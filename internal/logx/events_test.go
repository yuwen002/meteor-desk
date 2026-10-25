package logx

import (
	"strings"
	"testing"
	"time"
)

func TestAggregateEventsKeepsLongStackTrace(t *testing.T) {
	lines := []string{
		"2026-10-20 12:00:00.123 ERROR request failed",
		"java.lang.IllegalStateException: boom",
		strings.Repeat(" at service.Layer.call(Layer.java:10)", 100),
		"Caused by: java.io.IOException: closed",
		" at service.IO.read(IO.java:20)",
		"2026-10-20 12:00:01 INFO next request",
	}
	got := AggregateEvents(lines, 1000)
	if len(got) != 2 {
		t.Fatalf("事件数量=%d，期望 2：%+v", len(got), got)
	}
	if got[0].StartLine != 1 || got[0].EndLine != 5 || !strings.Contains(got[0].Content, "Caused by:") {
		t.Fatalf("堆栈边界错误：%+v", got[0])
	}
	if got[0].Level != "ERROR" || got[0].Timestamp != "2026-10-20 12:00:00.123" {
		t.Fatalf("事件元数据错误：%+v", got[0])
	}
}

func TestAggregateEventsKeepsMultilineJSONAsSingleEvent(t *testing.T) {
	lines := []string{
		"2026-10-20 12:00:00 INFO payload:",
		"{",
		"  \"message\": \"text with } and INFO inside\"",
		"  ,\"nested\": {\"ok\": true}",
		"}",
		"2026-10-20 12:00:01 WARN next",
	}
	got := AggregateEvents(lines, 100)
	if len(got) != 2 {
		t.Fatalf("事件数量=%d，期望 2：%+v", len(got), got)
	}
	if got[0].StartLine != 1 || got[0].EndLine != 5 || !strings.Contains(got[0].Content, "\"nested\"") {
		t.Fatalf("JSON 事件边界错误：%+v", got[0])
	}
}

func TestAggregateEventsSupportsTimestampFormatsAndPlainContinuation(t *testing.T) {
	lines := []string{
		"2026/10/20 12:00:00,123 INFO first",
		"continuation",
		"12:00:01.456 WARN second",
		"third continuation",
	}
	got := AggregateEvents(lines, 100)
	if len(got) != 2 || got[0].EndLine != 2 || got[1].StartLine != 3 {
		t.Fatalf("时间戳事件边界错误：%+v", got)
	}
	if got[1].Level != "WARN" {
		t.Fatalf("级别识别错误：%+v", got[1])
	}
}

func TestAggregateEventsSplitsOversizedEventWithoutLosingLines(t *testing.T) {
	lines := []string{"2026-10-20 12:00:00 ERROR first", "stack 1", "stack 2", "stack 3", "2026-10-20 12:00:01 INFO next"}
	got := AggregateEvents(lines, 3)
	if len(got) != 3 {
		t.Fatalf("事件数量=%d，期望 3：%+v", len(got), got)
	}
	if !got[0].Truncated || got[0].EndLine != 3 || got[1].StartLine != 4 || got[1].EndLine != 4 {
		t.Fatalf("超长事件截断标记或行号错误：%+v", got)
	}
	if got[2].StartLine != 5 {
		t.Fatalf("后续事件丢失：%+v", got[2])
	}
}

func TestParseEventTimestamp(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	got, ok := ParseEventTimestamp("2026-10-20 12:00:00.123", loc)
	if !ok || got.Hour() != 12 || got.Nanosecond() != 123000000 {
		t.Fatalf("时间戳解析错误：%v, %v", got, ok)
	}
	if _, ok := ParseEventTimestamp("not-a-time", loc); ok {
		t.Fatal("非法时间戳不应解析成功")
	}
}
