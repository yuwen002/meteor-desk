package logx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// readBoundedLine 在累积整行前执行大小检查，避免异常超长日志行导致无界分配。
func readBoundedLine(reader *bufio.Reader, limit int) ([]byte, error, error) {
	var line []byte
	for {
		part, readErr := reader.ReadSlice('\n')
		if len(line)+len(part) > limit {
			return nil, readErr, errors.New("多行 JSON 检索遇到超过 4 MiB 的单行内容；请改用普通检索或拆分文件")
		}
		line = append(line, part...)
		if readErr == bufio.ErrBufferFull {
			continue
		}
		return line, readErr, nil
	}
}

// searchMultiLineJSON 将跨行 JSON 对象/数组视为一个匹配单元。
// 单个文档限制为 4 MiB，避免格式错误的输入无限占用内存。
func searchMultiLineJSON(ctx context.Context, reader *bufio.Reader, response SearchResponse, query string, matcher *regexp.Regexp, maxResults int, onProgress func(Progress)) (SearchResponse, error) {
	var document []byte
	var documentStart int64
	var depth int
	var inString, escaped bool
	var bytesScanned, lastProgressBytes int64
	reportedMatches := 0
	var reachedLimit bool

	notify := func(force bool) {
		if onProgress == nil || (!force && bytesScanned-lastProgressBytes < progressStepBytes) {
			return
		}
		lastProgressBytes = bytesScanned
		updates := append([]Match(nil), response.Matches[reportedMatches:]...)
		reportedMatches = len(response.Matches)
		onProgress(Progress{
			BytesScanned: min(bytesScanned, response.FileSizeBytes),
			FileSizeBytes: response.FileSizeBytes,
			ScannedLines: response.ScannedLines,
			MatchesFound: len(response.Matches),
			Matches: updates,
		})
	}

	matchText := func(text []byte) bool {
		if matcher != nil {
			return matcher.Match(text)
		}
		return bytes.Contains(text, []byte(query))
	}
	addMatch := func(start int64, text []byte) {
		if matchText(text) {
			preview := text
			clipped := len(preview) > maxPreviewBytes
			if clipped {
				preview = preview[:maxPreviewBytes]
			}
			response.Matches = append(response.Matches, Match{
				LineNumber: start,
				Content: strings.ToValidUTF8(string(preview), "�"),
				ContentClipped: clipped,
				ContextBefore: []LineContext{},
				ContextAfter: []LineContext{},
			})
			if len(response.Matches) >= maxResults {
				response.ResultLimitReached = true
				reachedLimit = true
			}
		}
	}

	consumeJSONLine := func(line []byte) {
		for _, c := range line {
			if inString {
				if escaped {
					escaped = false
				} else if c == '\\' {
					escaped = true
				} else if c == '"' {
					inString = false
				}
				continue
			}
			switch c {
			case '"':
				inString = true
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
	}

	for {
		if ctx.Err() != nil {
			response.Canceled = true
			response.BytesScanned = min(bytesScanned, response.FileSizeBytes)
			notify(true)
			return response, nil
		}

		line, readErr := reader.ReadString('\n')
		if len(line) > maxRegexLineBytes {
			return SearchResponse{}, errors.New("多行 JSON 检索遇到超过 4 MiB 的单行内容；请改用普通检索或拆分文件")
		}
		bytesScanned += int64(len(line))
		response.ScannedLines++
		trimmed := bytes.TrimSpace([]byte(line))

		if len(document) == 0 && (bytes.HasPrefix(trimmed, []byte("{")) || bytes.HasPrefix(trimmed, []byte("["))) {
			documentStart = response.ScannedLines
			document = append(document[:0], line...)
			depth, inString, escaped = 0, false, false
			consumeJSONLine([]byte(line))
			if depth <= 0 {
				addMatch(documentStart, document)
				document = document[:0]
			}
		} else if len(document) > 0 {
			if len(document)+len(line) > maxRegexLineBytes {
				return SearchResponse{}, errors.New("多行 JSON 文档超过 4 MiB；为避免内存占用失控，已停止检索")
			}
			document = append(document, line...)
			consumeJSONLine([]byte(line))
			if depth <= 0 && !inString {
				addMatch(documentStart, document)
				document = document[:0]
			}
		} else if len(trimmed) > 0 {
			// 混合日志中的普通行仍可检索，不要求整个文件都是 JSON。
			addMatch(response.ScannedLines, trimmed)
		}

		notify(false)
		if reachedLimit {
			response.BytesScanned = min(bytesScanned, response.FileSizeBytes)
			notify(true)
			return response, nil
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return SearchResponse{}, fmt.Errorf("读取多行 JSON 日志失败：%w", readErr)
		}
	}
	if len(document) > 0 {
		addMatch(documentStart, document)
	}
	response.BytesScanned = response.FileSizeBytes
	notify(true)
	return response, nil
}
