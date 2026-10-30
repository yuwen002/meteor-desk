package logx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func collectEventsWithRule(t *testing.T, mode, pattern string, lines ...string) []EventContext {
	t.Helper()
	rule, err := resolveEventBoundary(mode, pattern)
	if err != nil {
		t.Fatalf("unexpected boundary rule error: %v", err)
	}
	collector := &eventCollector{}
	var events []EventContext
	for index, line := range lines {
		isBoundary := rule.start([]byte(line))
		if isBoundary && !collector.empty() && (collector.jsonDepth != 0 || collector.inString) {
			isBoundary = false
		}
		if isBoundary {
			if !collector.empty() {
				events = append(events, collector.context())
			}
			collector = &eventCollector{}
		}
		if !isBoundary || rule.skip == nil {
			collector.append([]byte(line), int64(index+1))
		}
	}
	if !collector.empty() {
		events = append(events, collector.context())
	}
	return events
}

func collectEvents(t *testing.T, lines ...string) []EventContext {
	t.Helper()
	return collectEventsWithRule(t, "", "", lines...)
}

func TestSearchFileEventContextReturnsCompleteException(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.log")
	content := "2026-10-25 10:00:00 INFO ready\n2026-10-25 10:01:00 ERROR request failed\njava.lang.IllegalStateException: database unavailable\n\tat api.Handler.handle(Handler.java:42)\nCaused by: connection refused\n2026-10-25 10:01:02 INFO recovered\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := SearchFileWithOptions(context.Background(), path, "database unavailable", false, 10, SearchOptions{EventContext: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 1 || got.Matches[0].Event == nil {
		t.Fatalf("expected event context: %+v", got.Matches)
	}
	event := got.Matches[0].Event
	if event.StartLine != 2 || event.EndLine != 5 || !strings.Contains(event.Content, "Caused by") {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestEventAggregationSplitsAtTimestampBoundaries(t *testing.T) {
	events := collectEvents(t, "2026-10-25 10:00:00 INFO started\n", "2026-10-25T10:00:01.123Z ERROR failed\n", "retrying\n")
	if len(events) != 2 || events[0].StartLine != 1 || events[0].EndLine != 1 || events[1].StartLine != 2 || events[1].EndLine != 3 {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestEventAggregationKeepsLongStackTraceTogether(t *testing.T) {
	lines := []string{"2026-10-25 10:00:00 ERROR request failed\n", "java.lang.IllegalStateException: unavailable\n"}
	for i := 0; i < 400; i++ {
		lines = append(lines, "\tat service.Handler.handle(Handler.java:42)\n")
	}
	lines = append(lines, "Caused by: java.net.ConnectException: refused\n", "2026-10-25 10:00:03 INFO recovered\n")
	events := collectEvents(t, lines...)
	if len(events) != 2 || events[0].EndLine != int64(len(lines)-1) || !strings.Contains(events[0].Content, "Caused by:") {
		t.Fatalf("stack trace should remain one event: %+v", events)
	}
}

func TestEventAggregationKeepsMultiLineJSONTogether(t *testing.T) {
	events := collectEvents(t, "2026-10-25 10:00:00 ERROR payload follows\n", "{\n", "  \"message\": \"literal 2026-10-25 10:01:00 stays a string\",\n", "  \"nested\": [1, 2]\n", "}\n", "2026-10-25 10:02:00 INFO next\n")
	if len(events) != 2 || events[0].EndLine != 5 || !strings.Contains(events[0].Content, "nested") {
		t.Fatalf("JSON should remain in first event: %+v", events)
	}
}

func TestEventAggregationSplitsAtBlankLines(t *testing.T) {
	events := collectEventsWithRule(t, "blank", "", "INFO first\n", "detail\n", "\n", "ERROR second\n", "\n", "\n")
	if len(events) != 2 || events[0].StartLine != 1 || events[0].EndLine != 2 || events[1].StartLine != 4 || events[1].EndLine != 4 {
		t.Fatalf("unexpected blank-line events: %+v", events)
	}
	if strings.Contains(events[0].Content, "\n\n") {
		t.Fatalf("blank separator should not be kept: %q", events[0].Content)
	}
}

func TestEventAggregationSplitsAtCustomBoundary(t *testing.T) {
	events := collectEventsWithRule(t, "custom", `^--- `, "--- request 1\n", "status=500\n", "--- request 2\n", "status=200\n")
	if len(events) != 2 || events[0].EndLine != 2 || events[1].StartLine != 3 || events[1].EndLine != 4 {
		t.Fatalf("unexpected custom-boundary events: %+v", events)
	}
}

func TestResolveEventBoundaryRejectsInvalidInput(t *testing.T) {
	cases := []struct{ mode, pattern string }{
		{"custom", ""},
		{"custom", "("},
		{"custom", strings.Repeat("a", maxEventBoundaryPatternBytes+1)},
		{"unknown", ""},
	}
	for _, tc := range cases {
		if _, err := resolveEventBoundary(tc.mode, tc.pattern); err == nil {
			t.Fatalf("expected error for mode=%q pattern=%q", tc.mode, tc.pattern)
		}
	}
	if _, err := resolveEventBoundary("BLANK", ""); err != nil {
		t.Fatalf("mode should be case-insensitive: %v", err)
	}
}

func TestSearchFileCustomBoundaryRespectsPattern(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.log")
	content := "--- request 1\nstatus=500\n--- request 2\nstatus=500\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := SearchFileWithOptions(context.Background(), path, "status=500", false, 10, SearchOptions{EventContext: true, EventBoundary: "custom", EventBoundaryPattern: `^--- `}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 2 {
		t.Fatalf("expected one match per request event, got %+v", got.Matches)
	}
	for i, match := range got.Matches {
		if match.Event == nil {
			t.Fatalf("match %d missing event context: %+v", i, match)
		}
		if want := int64(2 * (i + 1)); match.Event.StartLine != want-1 || match.Event.EndLine != want {
			t.Fatalf("match %d should cover its own request only: %+v", i, match.Event)
		}
	}
}

func TestSearchFileRejectsInvalidCustomBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.log")
	if err := os.WriteFile(path, []byte("anything\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := SearchFileWithOptions(context.Background(), path, "anything", false, 10, SearchOptions{EventContext: true, EventBoundary: "custom", EventBoundaryPattern: "("}, nil)
	if err == nil || !strings.Contains(err.Error(), "事件边界正则无效") {
		t.Fatalf("expected invalid pattern error, got %v", err)
	}
}

func TestMultiLineJSONMatchAttachesCompleteDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.ndjson.log")
	content := "{\n  \"level\": \"error\",\n  \"message\": \"database unavailable\"\n}\nplain line without json\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := SearchFileWithOptions(context.Background(), path, "database unavailable", false, 10, SearchOptions{MultiLineJSON: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 1 || got.Matches[0].Event == nil {
		t.Fatalf("expected one document match with event context: %+v", got.Matches)
	}
	event := got.Matches[0].Event
	if event.StartLine != 1 || event.EndLine != 4 || !strings.Contains(event.Content, "\"level\"") {
		t.Fatalf("unexpected document context: %+v", event)
	}
}

func TestMultiLineJSONPlainLineHasNoEventContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed.log")
	content := "{\n  \"message\": \"first\"\n}\nplain database unavailable line\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := SearchFileWithOptions(context.Background(), path, "database unavailable", false, 10, SearchOptions{MultiLineJSON: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 1 || got.Matches[0].Event != nil {
		t.Fatalf("plain line should not carry document context: %+v", got.Matches)
	}
}
