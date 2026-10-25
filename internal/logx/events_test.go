package logx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func collectEvents(lines ...string) []EventContext {
	collector := &eventCollector{}
	var events []EventContext
	for index, line := range lines {
		if collector.eventBoundary([]byte(line)) {
			events = append(events, collector.context())
			collector = &eventCollector{}
		}
		collector.append([]byte(line), int64(index+1))
	}
	if !collector.empty() {
		events = append(events, collector.context())
	}
	return events
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
	events := collectEvents("2026-10-25 10:00:00 INFO started\n", "2026-10-25T10:00:01.123Z ERROR failed\n", "retrying\n")
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
	events := collectEvents(lines...)
	if len(events) != 2 || events[0].EndLine != int64(len(lines)-1) || !strings.Contains(events[0].Content, "Caused by:") {
		t.Fatalf("stack trace should remain one event: %+v", events)
	}
}

func TestEventAggregationKeepsMultiLineJSONTogether(t *testing.T) {
	events := collectEvents("2026-10-25 10:00:00 ERROR payload follows\n", "{\n", "  \"message\": \"literal 2026-10-25 10:01:00 stays a string\",\n", "  \"nested\": [1, 2]\n", "}\n", "2026-10-25 10:02:00 INFO next\n")
	if len(events) != 2 || events[0].EndLine != 5 || !strings.Contains(events[0].Content, "nested") {
		t.Fatalf("JSON should remain in first event: %+v", events)
	}
}
