package logx

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
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


// Event 表示由多行原始日志组成的一条逻辑事件。
// StartLine 和 EndLine 均为从 1 开始的原始行号。
type Event struct {
	StartLine int
	EndLine   int
	Timestamp string
	Level     string
	Content   string
	Truncated bool
}

var (
	eventTimestampPattern = regexp.MustCompile(`(?i)^\s*(\d{4}[-/]\d{1,2}[-/]\d{1,2}[ T]\d{2}:\d{2}:\d{2}([.,]\d{1,9})?(Z|[+-]\d{2}:?\d{2})?|\d{2}:\d{2}:\d{2}([.,]\d{1,9})?)`)
	eventLevelPattern     = regexp.MustCompile(`(?i)^\s*(?:\[[^\]]+\]\s*)?(?:TRACE|DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL|PANIC)\b`)
	eventLevelWordPattern = regexp.MustCompile(`(?i)\b(TRACE|DEBUG|INFO|WARN(ING)?|ERROR|FATAL|PANIC)\b`)
)

// AggregateEvents 将原始日志行聚合为逻辑事件。
// 新时间戳行开启新事件；堆栈、普通续行和未闭合的多行 JSON 保留在当前事件。
// maxEventLines 限制单事件行数，小于 1 时默认 10000 行。
func AggregateEvents(lines []string, maxEventLines int) []Event {
	if len(lines) == 0 {
		return nil
	}
	if maxEventLines < 1 {
		maxEventLines = 10000
	}

	events := make([]Event, 0, min(len(lines), 256))
	var current *Event
	var jsonDepth int
	var inString, escaped bool

	flush := func() {
		if current == nil {
			return
		}
		events = append(events, *current)
		current = nil
		jsonDepth = 0
		inString, escaped = false, false
	}
	start := func(line string, lineNo int) {
		ts := strings.TrimSpace(eventTimestampPattern.FindString(line))
		level := ""
		if match := eventLevelWordPattern.FindStringSubmatch(line); len(match) > 1 {
			level = strings.ToUpper(match[1])
			if level == "WARNING" {
				level = "WARN"
			}
		}
		current = &Event{StartLine: lineNo, EndLine: lineNo, Timestamp: ts, Level: level, Content: line}
		jsonDepth, inString, escaped = 0, false, false
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			jsonDepth, inString, escaped = jsonBalance(line, 0, false, false)
		}
	}
	appendLine := func(line string, lineNo int) {
		if current == nil {
			start(line, lineNo)
			return
		}
		if current.EndLine-current.StartLine+1 >= maxEventLines {
			current.Truncated = true
			flush()
			start(line, lineNo)
			return
		}
		current.Content += "\n" + line
		current.EndLine = lineNo
		jsonDepth, inString, escaped = jsonBalance(line, jsonDepth, inString, escaped)
	}

	for i, line := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		if current == nil {
			start(line, lineNo)
			continue
		}

		// JSON 尚未闭合时，即使字符串中出现时间戳或日志级别，也不能切分事件。
		if jsonDepth > 0 || inString {
			appendLine(line, lineNo)
			continue
		}

		isTimestamp := eventTimestampPattern.MatchString(line)
		isLevelLine := eventLevelPattern.MatchString(line) &&
			!strings.HasPrefix(trimmed, "at ") &&
			!strings.HasPrefix(trimmed, "Caused by:") &&
			!strings.HasPrefix(trimmed, "... ")
		isJSONStart := strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
		currentText := strings.TrimSpace(current.Content)
		jsonContinuesCurrent := isJSONStart && (current.Timestamp != "" || strings.HasSuffix(currentText, ":"))
		if trimmed != "" && (isTimestamp || isLevelLine || (isJSONStart && !jsonContinuesCurrent)) {
			flush()
			start(line, lineNo)
			continue
		}
		appendLine(line, lineNo)
	}
	flush()
	return events
}

// jsonBalance 在字符串之外统计 JSON 对象/数组括号，并处理转义引号。
func jsonBalance(line string, depth int, inString, escaped bool) (int, bool, bool) {
	for _, r := range line {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch r {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth, inString, escaped
}

// ParseEventTimestamp 按指定时区解析事件时间戳；无时区值按 location 解释。
func ParseEventTimestamp(value string, location *time.Location) (time.Time, bool) {
	if location == nil {
		location = time.Local
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05.999999999-0700",
		"2006-01-02 15:04:05-0700",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05.999999999Z07:00",
		"2006/01/02 15:04:05Z07:00",
		"2006/01/02 15:04:05.999999999-0700",
		"2006/01/02 15:04:05-0700",
		"2006/01/02 15:04:05.999999999",
		"2006/01/02 15:04:05",
		"15:04:05.999999999",
		"15:04:05",
	}
	value = strings.ReplaceAll(value, ",", ".")
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
