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

type pendingJSONMatch struct {
	match *Match
	remaining int
}

// searchMultiLineJSON 将跨行 JSON 对象/数组视为一个匹配单元，并支持文档外日志上下文。
func searchMultiLineJSON(ctx context.Context, reader *bufio.Reader, response SearchResponse, query string, matcher *regexp.Regexp, maxResults, contextLines int, onProgress func(Progress)) (SearchResponse, error) {
	var document []byte
	var documentStart int64
	var depth int
	var inString, escaped bool
	var bytesScanned, lastProgressBytes int64
	reportedMatches := 0
	var reachedLimit bool
	var foundCount int
	history := make([]LineContext, 0, contextLines)
	pending := make([]pendingJSONMatch, 0, 16)

	notify := func(force bool) {
		if onProgress == nil || (!force && bytesScanned-lastProgressBytes < progressStepBytes) {
			return
		}
		lastProgressBytes = bytesScanned
		updates := make([]Match, len(response.Matches)-reportedMatches)
		for i, match := range response.Matches[reportedMatches:] {
			match.ContextBefore = append([]LineContext(nil), match.ContextBefore...)
			match.ContextAfter = append([]LineContext(nil), match.ContextAfter...)
			updates[i] = match
		}
		reportedMatches = len(response.Matches)
		onProgress(Progress{BytesScanned:min(bytesScanned, response.FileSizeBytes), FileSizeBytes:response.FileSizeBytes, ScannedLines:response.ScannedLines, MatchesFound:len(response.Matches), Matches:updates})
	}
	matchText := func(text []byte) bool {
		if matcher != nil { return matcher.Match(text) }
		return bytes.Contains(text, []byte(query))
	}
	flushReady := func(all bool) {
		n := 0
		for n < len(pending) && (all || pending[n].remaining <= 0) {
			response.Matches = append(response.Matches, *pending[n].match)
			n++
		}
		if n > 0 {
			pending = append(pending[:0], pending[n:]...)
			notify(true)
		}
	}
	addMatch := func(start int64, text []byte) {
		if reachedLimit || !matchText(text) { return }
		preview := text
		clipped := len(preview) > maxPreviewBytes
		if clipped { preview = preview[:maxPreviewBytes] }
		before := append([]LineContext(nil), history...)
		m := &Match{LineNumber:start, Content:strings.ToValidUTF8(string(preview), "�"), ContentClipped:clipped, ContextBefore:before, ContextAfter:[]LineContext{}}
		pending = append(pending, pendingJSONMatch{match:m, remaining:contextLines})
		foundCount++
		if foundCount >= maxResults { response.ResultLimitReached = true; reachedLimit = true }
		flushReady(false)
	}
	ordinaryLine := func(lineNumber int64, content []byte) {
		content = bytes.TrimSpace(content)
		if len(content) == 0 { return }
		context := LineContext{LineNumber:lineNumber, Content:strings.ToValidUTF8(string(content), "�"), ContentClipped:len(content)>maxContextBytes}
		if context.ContentClipped { context.Content = context.Content[:maxContextBytes] }
		// 当前普通日志行可作为已有命中的后文；JSON 文档内部行不会进入这里。
		for i := range pending {
			if pending[i].remaining > 0 {
				pending[i].match.ContextAfter = append(pending[i].match.ContextAfter, context)
				pending[i].remaining--
			}
		}
		flushReady(false)
		addMatch(lineNumber, content)
		if contextLines > 0 {
			history = append(history, context)
			if len(history) > contextLines { history = history[len(history)-contextLines:] }
		}
	}
	consumeJSONLine := func(line []byte) {
		for _, c := range line {
			if inString {
				if escaped { escaped = false } else if c == '\\' { escaped = true } else if c == '"' { inString = false }
				continue
			}
			switch c {
			case '"': inString = true
			case '{', '[': depth++
			case '}', ']': depth--
			}
		}
	}

	for {
		if ctx.Err() != nil {
			response.Canceled = true
			response.BytesScanned = min(bytesScanned, response.FileSizeBytes)
			flushReady(true)
			notify(true)
			return response, nil
		}
		line, readErr, lineErr := readBoundedLine(reader, maxRegexLineBytes)
		if lineErr != nil { return SearchResponse{}, lineErr }
		bytesScanned += int64(len(line))
		response.ScannedLines++
		trimmed := bytes.TrimSpace(line)

		if len(document) == 0 && (bytes.HasPrefix(trimmed, []byte("{")) || bytes.HasPrefix(trimmed, []byte("["))) {
			documentStart = response.ScannedLines
			document = append(document[:0], line...)
			depth, inString, escaped = 0, false, false
			consumeJSONLine(line)
			if depth <= 0 && !inString {
				addMatch(documentStart, document)
				document = document[:0]
			}
		} else if len(document) > 0 {
			if len(document)+len(line) > maxRegexLineBytes { return SearchResponse{}, errors.New("多行 JSON 文档超过 4 MiB；为避免内存占用失控，已停止检索") }
			document = append(document, line...)
			consumeJSONLine(line)
			if depth <= 0 && !inString {
				addMatch(documentStart, document)
				document = document[:0]
			}
		} else {
			ordinaryLine(response.ScannedLines, trimmed)
		}

		notify(false)
		if reachedLimit {
			// 命中达到上限后，不再继续寻找新结果；只读取足够的普通行补齐已命中项的后文。
			allReady := true
			for _, p := range pending { if p.remaining > 0 { allReady = false; break } }
			if allReady {
				response.BytesScanned = min(bytesScanned, response.FileSizeBytes)
				notify(true)
				return response, nil
			}
		}
		if readErr == io.EOF { break }
		if readErr != nil { return SearchResponse{}, fmt.Errorf("读取多行 JSON 日志失败：%w", readErr) }
	}
	// EOF 时文档仍未闭合则忽略；已有命中的后文允许不足 contextLines。
	response.BytesScanned = min(bytesScanned, response.FileSizeBytes)
	flushReady(true)
	notify(true)
	return response, nil
}
