package logx

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const maxEventBytes = 512 * 1024

var eventTimestamp = regexp.MustCompile(`^\s*(?:\[)?\d{4}[-/]\d{1,2}[-/]\d{1,2}(?:[ T]\d{1,2}:\d{2}:\d{2}(?:[.,]\d{1,9})?(?:Z|[+-]\d{2}:?\d{2})?)?`)

// EventContext is the bounded, complete log event containing a search hit.
type EventContext struct {
	StartLine      int64  `json:"startLine"`
	EndLine        int64  `json:"endLine"`
	Content        string `json:"content"`
	ContentClipped bool   `json:"contentClipped"`
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

func (c *eventCollector) eventBoundary(line []byte) bool {
	return !c.empty() && c.jsonDepth == 0 && !c.inString && eventTimestamp.Match(bytes.TrimSpace(line))
}

func searchEventContext(ctx context.Context, reader *bufio.Reader, response SearchResponse, query string, matcher *regexp.Regexp, maxResults int, onProgress func(Progress)) (SearchResponse, error) {
	collector := &eventCollector{}
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
		response.Matches = append(response.Matches, Match{LineNumber: event.StartLine, Content: preview, ContentClipped: contentClipped, ContextBefore: []LineContext{}, ContextAfter: []LineContext{}, Event: &event})
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
		bytesScanned += int64(len(line))
		response.ScannedLines++
		if collector.eventBoundary(line) {
			matchEvent(collector.context())
			collector = &eventCollector{}
			if len(response.Matches) >= maxResults {
				response.ResultLimitReached = true
				response.BytesScanned = min(bytesScanned, response.FileSizeBytes)
				notify(true)
				return response, nil
			}
		}
		collector.append(line, response.ScannedLines)
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
