// Package logx 提供适用于大体积日志文件的流式检索能力。
package logx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	defaultMaxResults = 500
	hardMaxResults    = 5000
	maxPreviewBytes   = 8 * 1024
	readerBufferSize  = 256 * 1024
	progressStepBytes = 256 * 1024
	// 正则表达式需要按整行匹配，因此为单行设置内存上限。
	maxRegexLineBytes = 4 * 1024 * 1024
)

// LineContext 是上下文窗口中的一行日志。
type LineContext struct {
	LineNumber int64  `json:"lineNumber"`
	Content string `json:"content"`
	ContentClipped bool `json:"contentClipped"`
}

// Match 表示一条匹配结果，以及可选的前后文。
type Match struct {
	LineNumber int64 `json:"lineNumber"`
	Content string `json:"content"`
	ContentClipped bool `json:"contentClipped"`
	ContextBefore []LineContext `json:"contextBefore,omitempty"`
	ContextAfter []LineContext `json:"contextAfter,omitempty"`
}

// Progress 描述扫描进度，供桌面界面轮询展示。
type Progress struct {
	BytesScanned int64 `json:"bytesScanned"`
	FileSizeBytes int64 `json:"fileSizeBytes"`
	ScannedLines int64 `json:"scannedLines"`
	MatchesFound int `json:"matchesFound"`
}

// SearchOptions 是一次检索的可选参数。
type SearchOptions struct {
	ContextLines int `json:"contextLines"`
}

// SearchResponse 是一次日志检索的结果摘要。
type SearchResponse struct {
	FilePath string `json:"filePath"`
	FileSizeBytes int64 `json:"fileSizeBytes"`
	BytesScanned int64 `json:"bytesScanned"`
	ScannedLines int64 `json:"scannedLines"`
	Matches []Match `json:"matches"`
	ResultLimitReached bool `json:"resultLimitReached"`
	Canceled bool `json:"canceled"`
}

// SearchFile 保留简单调用入口。
func SearchFile(path, query string, useRegex bool, maxResults int) (SearchResponse, error) {
	return SearchFileWithOptions(context.Background(), path, query, useRegex, maxResults, SearchOptions{}, nil)
}

// SearchFileContext 支持取消和进度回调，但不附加上下文行。
func SearchFileContext(ctx context.Context, path, query string, useRegex bool, maxResults int, onProgress func(Progress)) (SearchResponse, error) {
	return SearchFileWithOptions(ctx, path, query, useRegex, maxResults, SearchOptions{}, onProgress)
}

// SearchFileWithOptions 使用固定大小的读取缓冲区扫描文件。
// 普通文本匹配只需保留关键字长度相关的边界字节，不拼接整行；结果预览和上下文窗口有界。
// 正则模式按整行匹配，单行最多允许 4 MiB，避免异常日志造成不受控的内存占用。
func SearchFileWithOptions(ctx context.Context, path, query string, useRegex bool, maxResults int, options SearchOptions, onProgress func(Progress)) (SearchResponse, error) {
	if strings.TrimSpace(path) == "" {
		return SearchResponse{}, errors.New("请先选择日志文件")
	}
	if query == "" {
		return SearchResponse{}, errors.New("请输入搜索内容")
	}
	if len(query) > 4096 {
		return SearchResponse{}, errors.New("搜索内容不能超过 4096 字节")
	}
	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}
	if maxResults > hardMaxResults {
		maxResults = hardMaxResults
	}
	contextLines := options.ContextLines
	if contextLines < 0 {
		contextLines = 0
	}
	if contextLines > 10 {
		contextLines = 10
	}
	if err := ctx.Err(); err != nil {
		return SearchResponse{Canceled: true}, nil
	}

	var matcher *regexp.Regexp
	if useRegex {
		compiled, err := regexp.Compile(query)
		if err != nil {
			return SearchResponse{}, fmt.Errorf("正则表达式无效：%w", err)
		}
		matcher = compiled
	}
	needle := []byte(query)

	file, err := os.Open(path)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("打开日志文件失败：%w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return SearchResponse{}, fmt.Errorf("读取日志文件信息失败：%w", err)
	}

	response := SearchResponse{
		FilePath: path,
		FileSizeBytes: info.Size(),
		Matches: make([]Match, 0, min(maxResults, 128)),
	}
	reader := bufio.NewReaderSize(file, readerBufferSize)

	var lineNumber int64
	var lineBytes int64
	var bytesScanned int64
	var lastProgressBytes int64
	var preview []byte
	var regexLine []byte
	var carry []byte
	var pendingCR bool
	var lineOpen bool
	var lineMatched bool
	var reachedLimit bool
	history := make([]LineContext, 0, contextLines)
	pendingContext := make([]int, 0, 16)

	notify := func(force bool) {
		if onProgress == nil {
			return
		}
		if !force && bytesScanned-lastProgressBytes < progressStepBytes {
			return
		}
		lastProgressBytes = bytesScanned
		onProgress(Progress{
			BytesScanned: bytesScanned,
			FileSizeBytes: info.Size(),
			ScannedLines: response.ScannedLines,
			MatchesFound: len(response.Matches),
		})
	}

	appendContent := func(part []byte) error {
		if len(part) == 0 {
			return nil
		}
		lineOpen = true
		lineBytes += int64(len(part))

		if len(preview) < maxPreviewBytes {
			remaining := maxPreviewBytes - len(preview)
			if remaining > len(part) {
				remaining = len(part)
			}
			preview = append(preview, part[:remaining]...)
		}

		if matcher != nil {
			if len(regexLine)+len(part) > maxRegexLineBytes {
				return errors.New("当前日志包含超过 4 MiB 的单行内容；为避免占用过多内存，正则模式已停止，请改用普通文本搜索")
			}
			regexLine = append(regexLine, part...)
			return nil
		}

		if !lineMatched {
			if bytes.Contains(part, needle) {
				lineMatched = true
			} else {
				// 只有跨块匹配才需要拼接边界：最多分配关键字长度左右的少量内存。
				keep := len(needle) - 1
				prefixLen := min(len(part), keep)
				if keep > 0 && len(carry) > 0 && prefixLen > 0 {
					cross := make([]byte, 0, len(carry)+prefixLen)
					cross = append(cross, carry...)
					cross = append(cross, part[:prefixLen]...)
					lineMatched = bytes.Contains(cross, needle)
				}
				if !lineMatched && keep > 0 {
					if len(part) >= keep {
						carry = append(carry[:0], part[len(part)-keep:]...)
					} else {
						needed := keep - len(part)
						if needed > len(carry) {
							needed = len(carry)
						}
						tail := append([]byte(nil), carry[len(carry)-needed:]...)
						carry = append(tail, part...)
					}
				}
			}
		}
		return nil
	}

	finishLine := func() {
		lineNumber++
		response.ScannedLines = lineNumber
		content := strings.ToValidUTF8(string(preview), "�")
		clipped := lineBytes > maxPreviewBytes
		current := LineContext{LineNumber: lineNumber, Content: content, ContentClipped: clipped}

		// 为前面已经命中的结果追加当前行作为后文。
		if contextLines > 0 && len(pendingContext) > 0 {
			nextPending := pendingContext[:0]
			for _, matchIndex := range pendingContext {
				m := &response.Matches[matchIndex]
				if len(m.ContextAfter) < contextLines {
					m.ContextAfter = append(m.ContextAfter, current)
				}
				if len(m.ContextAfter) < contextLines {
					nextPending = append(nextPending, matchIndex)
				}
			}
			pendingContext = nextPending
		}

		if matcher != nil {
			lineMatched = matcher.Match(regexLine)
		}
		if lineMatched {
			before := append([]LineContext(nil), history...)
			response.Matches = append(response.Matches, Match{
				LineNumber: lineNumber,
				Content: content,
				ContentClipped: clipped,
				ContextBefore: before,
			})
			if contextLines > 0 {
				pendingContext = append(pendingContext, len(response.Matches)-1)
			}
			if len(response.Matches) >= maxResults {
				response.ResultLimitReached = true
				reachedLimit = true
			}
		}

		if contextLines > 0 {
			history = append(history, current)
			if len(history) > contextLines {
				history = append(history[:0], history[len(history)-contextLines:]...)
			}
		}
		lineBytes = 0
		preview = preview[:0]
		regexLine = regexLine[:0]
		carry = carry[:0]
		lineMatched = false
		lineOpen = false
		pendingCR = false
	}

	for {
		if ctx.Err() != nil {
			response.Canceled = true
			response.BytesScanned = bytesScanned
			notify(true)
			return response, nil
		}

		chunk, readErr := reader.ReadSlice('
')
		bytesScanned += int64(len(chunk))
		hasNewline := len(chunk) > 0 && chunk[len(chunk)-1] == '
'
		part := chunk
		if hasNewline {
			part = part[:len(part)-1]
		}
		if hasNewline && len(part) > 0 && part[len(part)-1] == '' {
			part = part[:len(part)-1]
		}

		// 兼容 CRLF 跨读取缓冲区边界的情况。
		if pendingCR {
			if !(hasNewline && len(part) == 0) {
				if err := appendContent([]byte{''}); err != nil {
					return SearchResponse{}, err
				}
			}
			pendingCR = false
		}
		if !hasNewline && len(part) > 0 && part[len(part)-1] == '' {
			part = part[:len(part)-1]
			pendingCR = true
			lineOpen = true
		}
		if len(part) > 0 {
			if err := appendContent(part); err != nil {
				return SearchResponse{}, err
			}
		}

		if hasNewline {
			finishLine()
			notify(false)
			if reachedLimit {
				response.BytesScanned = bytesScanned
				notify(true)
				return response, nil
			}
		}
		if readErr == bufio.ErrBufferFull {
			notify(false)
			continue
		}
		if readErr == io.EOF {
			if lineOpen {
				finishLine()
			}
			break
		}
		if readErr != nil {
			return SearchResponse{}, fmt.Errorf("读取日志文件失败：%w", readErr)
		}
		notify(false)
	}

	response.BytesScanned = bytesScanned
	notify(true)
	return response, nil
}
