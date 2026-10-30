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

const (
	maxEventBytes = 512 * 1024
	// maxEventTotalBytes 限制一次检索为事件或完整文档上下文保留的内容总量。
	maxEventTotalBytes = 32 << 20
	// maxEventBoundaryPatternBytes 限制自定义事件边界正则的长度。
	maxEventBoundaryPatternBytes = 512
)

var eventTimestamp = regexp.MustCompile(`^\s*(?:\[)?\d{4}[-/]\d{1,2}[-/]\d{1,2}(?:[ T]\d{1,2}:\d{2}:\d{2}(?:[.,]\d{1,9})?(?:Z|[+-]\d{2}:?\d{2})?)?`)

// eventBoundaryRule 描述如何判断一行是否开启新的事件。
// skip 非空时，命中的行只作为分隔符，不会写入事件内容。
type eventBoundaryRule struct {
	start func(line []byte) bool
	skip  func(line []byte) bool
}

// resolveEventBoundary 解析事件边界模式：timestamp（默认）、blank 或 custom。
// custom 模式使用用户提供的 Go 正则，匹配去首尾空白后的整行。
func resolveEventBoundary(mode, pattern string) (eventBoundaryRule, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "timestamp":
		return eventBoundaryRule{start: func(line []byte) bool { return eventTimestamp.Match(bytes.TrimSpace(line)) }}, nil
	case "blank":
		blank := func(line []byte) bool { return len(bytes.TrimSpace(line)) == 0 }
		return eventBoundaryRule{start: blank, skip: blank}, nil
	case "custom":
		if len(pattern) == 0 || len(pattern) > maxEventBoundaryPatternBytes {
			return eventBoundaryRule{}, fmt.Errorf("事件边界正则长度必须在 1 到 %d 字节之间", maxEventBoundaryPatternBytes)
		}
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return eventBoundaryRule{}, fmt.Errorf("事件边界正则无效：%w", err)
		}
		return eventBoundaryRule{start: func(line []byte) bool { return compiled.Match(bytes.TrimSpace(line)) }}, nil
	default:
		return eventBoundaryRule{}, errors.New("未知的事件边界规则；可选 timestamp、blank 或 custom")
	}
}

// newEventContext 在单事件上限与总量预算内保留事件或完整文档内容。
// 预算耗尽时返回空内容并标记截断，前端会回退到行预览。
func newEventContext(startLine, endLine int64, content string, budget *int64) *EventContext {
	limit := int64(len(content))
	if limit > maxEventBytes {
		limit = maxEventBytes
	}
	if *budget < limit {
		limit = *budget
	}
	if limit < 0 {
		limit = 0
	}
	event := &EventContext{StartLine: startLine, EndLine: endLine, ContentClipped: limit < int64(len(content))}
	if limit > 0 {
		event.Content = strings.ToValidUTF8(content[:limit], "�")
		*budget -= limit
	}
	return event
}

// EventContext is the bounded, complete log event containing a search hit.
type EventContext struct {
	StartLine      int64  `json:"startLine"`
	EndLine        int64  `json:"endLine"`
	Content        string `json:"content"`
	ContentClipped bool   `json:"contentClipped"`
	// Kind 区分按边界聚合的事件（event）与多行 JSON 文档（document）。
	Kind string `json:"kind,omitempty"`
}

type eventCollector struct {
	startLine int64
	endLine   int64
	content   []byte
	clipped   bool
	jsonDepth int
	inString  bool
	escaped   bool
}

func (c *eventCollector) empty() bool { return c.startLine == 0 }

func (c *eventCollector) startsJSON(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
}

func (c *eventCollector) consumeJSON(line []byte) {
	for _, char := range line {
		if c.inString {
			if c.escaped {
				c.escaped = false
			} else if char == '\\' {
				c.escaped = true
			} else if char == '"' {
				c.inString = false
			}
			continue
		}
		switch char {
		case '"':
			c.inString = true
		case '{', '[':
			c.jsonDepth++
		case '}', ']':
			c.jsonDepth--
		}
	}
}

func (c *eventCollector) append(line []byte, lineNumber int64) {
	if c.empty() {
		c.startLine = lineNumber
	}
	c.endLine = lineNumber
	if len(c.content) < maxEventBytes {
		remaining := maxEventBytes - len(c.content)
		if remaining < len(line) {
			line = line[:remaining]
			c.clipped = true
		}
		c.content = append(c.content, line...)
	} else {
		c.clipped = true
	}
	if c.jsonDepth == 0 && c.startsJSON(line) {
		c.inString, c.escaped = false, false
		c.consumeJSON(line)
	} else if c.jsonDepth > 0 || c.inString {
		c.consumeJSON(line)
	}
}

func (c *eventCollector) context() EventContext {
	return EventContext{StartLine: c.startLine, EndLine: c.endLine, Content: strings.ToValidUTF8(string(c.content), "�"), ContentClipped: c.clipped}
}

func searchEventContext(ctx context.Context, reader *bufio.Reader, response SearchResponse, query string, matcher *regexp.Regexp, maxResults int, rule eventBoundaryRule, onProgress func(Progress)) (SearchResponse, error) {
	collector := &eventCollector{}
	budget := int64(maxEventTotalBytes)
	var bytesScanned, lastProgressBytes int64
	reportedMatches := 0
	matchEvent := func(event EventContext) {
		if event.StartLine == 0 || len(response.Matches) >= maxResults {
			return
		}
		matched := bytes.Contains([]byte(event.Content), []byte(query))
		if matcher != nil {
			matched = matcher.MatchString(event.Content)
		}
		if !matched {
			return
		}
		preview := event.Content
		contentClipped := len(preview) > maxPreviewBytes
		if contentClipped {
			preview = preview[:maxPreviewBytes]
		}
		eventContext := newEventContext(event.StartLine, event.EndLine, event.Content, &budget)
		eventContext.ContentClipped = eventContext.ContentClipped || event.ContentClipped
		eventContext.Kind = "event"
		response.Matches = append(response.Matches, Match{LineNumber: event.StartLine, Content: preview, ContentClipped: contentClipped, ContextBefore: []LineContext{}, ContextAfter: []LineContext{}, Event: eventContext})
	}
	notify := func(force bool) {
		if onProgress == nil || (!force && bytesScanned-lastProgressBytes < progressStepBytes) {
			return
		}
		lastProgressBytes = bytesScanned
		updates := append([]Match(nil), response.Matches[reportedMatches:]...)
		reportedMatches = len(response.Matches)
		onProgress(Progress{BytesScanned: min(bytesScanned, response.FileSizeBytes), FileSizeBytes: response.FileSizeBytes, ScannedLines: response.ScannedLines, MatchesFound: len(response.Matches), Matches: updates})
	}
	for {
		if ctx.Err() != nil {
			response.Canceled = true
			response.BytesScanned = min(bytesScanned, response.FileSizeBytes)
			notify(true)
			return response, nil
		}
		line, readErr, lineErr := readBoundedLine(reader, maxEventBytes)
		if lineErr != nil {
			return SearchResponse{}, fmt.Errorf("事件聚合读取失败：%w", lineErr)
		}
		// 文件末尾的换行符不构成额外的一行，避免事件被空的尾行拉长。
		if len(line) == 0 && readErr == io.EOF {
			break
		}
		bytesScanned += int64(len(line))
		response.ScannedLines++
		isBoundary := rule.start(line)
		if isBoundary && !collector.empty() && (collector.jsonDepth != 0 || collector.inString) {
			// 未闭合的 JSON 文档内部不按边界拆分。
			isBoundary = false
		}
		if isBoundary {
			matchEvent(collector.context())
			collector = &eventCollector{}
			if len(response.Matches) >= maxResults {
				response.ResultLimitReached = true
				response.BytesScanned = min(bytesScanned, response.FileSizeBytes)
				notify(true)
				return response, nil
			}
		}
		if !isBoundary || rule.skip == nil {
			collector.append(line, response.ScannedLines)
		}
		notify(false)
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return SearchResponse{}, fmt.Errorf("读取日志事件失败：%w", readErr)
		}
	}
	matchEvent(collector.context())
	if len(response.Matches) >= maxResults {
		response.ResultLimitReached = true
	}
	response.BytesScanned = response.FileSizeBytes
	notify(true)
	return response, nil
}
