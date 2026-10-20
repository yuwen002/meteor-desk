package logx

import (
	"regexp"
	"strings"
	"time"
)

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
	eventTimestampPattern = regexp.MustCompile(`(?i)^\s*(\d{4}[-/]\d{2}[-/]\d{2}[ T]\d{2}:\d{2}:\d{2}([.,]\d{1,9})?(Z|[+-]\d{2}:?\d{2})?|\d{2}:\d{2}:\d{2}([.,]\d{1,9})?)`)
	eventLevelPattern = regexp.MustCompile(`(?i)\b(TRACE|DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL|PANIC)\b`)
)

// AggregateEvents 将原始行聚合为逻辑日志事件。
// 新时间戳行通常开启新事件；堆栈、缩进文本和多行 JSON 保留在当前事件中。
// maxEventLines 用于限制单个事件的内存占用，传入小于 1 的值时默认 10000 行。
func AggregateEvents(lines []string, maxEventLines int) []Event {
	if len(lines) == 0 {
		return nil
	}
	if maxEventLines < 1 {
		maxEventLines = 10000
	}

	events := make([]Event, 0, len(lines))
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
		if match := eventLevelPattern.FindStringSubmatch(line); len(match) > 1 {
			level = strings.ToUpper(match[1])
			if level == "WARNING" {
				level = "WARN"
			}
		}
		current = &Event{StartLine: lineNo, EndLine: lineNo, Timestamp: ts, Level: level, Content: line}
		jsonDepth, inString, escaped = jsonBalance(line, 0, false, false)
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
		// JSON 未闭合时，不把内部带时间戳或 INFO/ERROR 的字符串误判为新事件。
		if jsonDepth > 0 {
			appendLine(line, lineNo)
			continue
		}

		isTimestamp := eventTimestampPattern.MatchString(line)
		isLevelLine := eventLevelPattern.MatchString(line) && !strings.HasPrefix(trimmed, "at ") &&
			!strings.HasPrefix(trimmed, "Caused by:") && !strings.HasPrefix(trimmed, "... ")
		isJSONStart := strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
		currentText := strings.TrimSpace(current.Content)
		jsonContinuesCurrent := isJSONStart && (current.Timestamp != "" || strings.HasSuffix(currentText, ":"))
		if (isTimestamp || isLevelLine || (isJSONStart && !jsonContinuesCurrent)) && trimmed != "" {
			flush()
			start(line, lineNo)
			continue
		}
		appendLine(line, lineNo)
	}
	flush()
	return events
}

// jsonBalance 在字符串之外统计 JSON 对象/数组括号，正确处理转义引号。
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

// ParseEventTimestamp 尝试将事件时间戳解析为本地可显示的时间。
// 对不带时区的日志时间戳按调用方指定的时区解释。
func ParseEventTimestamp(value string, location *time.Location) (time.Time, bool) {
	if location == nil {
		location = time.Local
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05.999999999",
		"2006/01/02 15:04:05",
		"15:04:05.999999999",
		"15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
