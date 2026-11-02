package logx

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func utcMilli(year int, month time.Month, day, hour, min, sec int) int64 {
	return time.Date(year, month, day, hour, min, sec, 0, time.UTC).UnixMilli()
}

func TestParseLeadingTimestampMillisFormats(t *testing.T) {
	cases := []struct {
		name   string
		line   string
		wantMs int64
		wantOK bool
	}{
		{"date only", "2026-01-02 something", utcMilli(2026, time.January, 2, 0, 0, 0), true},
		{"slash date time", "2026/01/02 03:04:05 ERR", utcMilli(2026, time.January, 2, 3, 4, 5), true},
		{"space date time", "2026-01-02 03:04:05 ERR", utcMilli(2026, time.January, 2, 3, 4, 5), true},
		{"ISO T millis", "2026-01-02T03:04:05.123Z ERR", utcMilli(2026, time.January, 2, 3, 4, 5), true},
		{"comma fraction", "2026-01-02 03:04:05,123 ERR", utcMilli(2026, time.January, 2, 3, 4, 5), true},
		{"leading bracket", "[2026-01-02 03:04:05] ERR", utcMilli(2026, time.January, 2, 3, 4, 5), true},
		{"positive offset", "2026-01-02T03:04:05+08:00 ERR", utcMilli(2026, time.January, 1, 19, 4, 5), true},
		{"negative offset compact", "2026-01-02 03:04:05-0500 ERR", utcMilli(2026, time.January, 2, 8, 4, 5), true},
		{"time only no date", "03:04:05 ERR", 0, false},
		{"no timestamp", "ERROR without any stamp", 0, false},
		{"invalid month", "2026-13-02 03:04:05 ERR", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseLeadingTimestampMillis([]byte(tc.line), time.UTC)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, 期望 %v", ok, tc.wantOK)
			}
			if tc.wantOK && got != tc.wantMs {
				t.Fatalf("毫秒 = %d, 期望 %d", got, tc.wantMs)
			}
		})
	}
}

func TestTimeWindowFilterBoundaries(t *testing.T) {
	// [10:00, 11:00)：下界含、上界不含；naive 时间按 UTC 参照。
	filter := timeWindowFilter{enabled: true, startMs: utcMilli(2026, time.January, 2, 10, 0, 0), endMs: utcMilli(2026, time.January, 2, 11, 0, 0), zone: time.UTC}

	if within, has := filter.allows([]byte("2026-01-02 10:00:00 a")); !within || !has {
		t.Fatalf("下界应命中: within=%v has=%v", within, has)
	}
	if within, _ := filter.allows([]byte("2026-01-02 11:00:00 a")); within {
		t.Fatal("上界不含，11:00 不应命中")
	}
	if within, _ := filter.allows([]byte("2026-01-02 09:59:59 a")); within {
		t.Fatal("窗口前不应命中")
	}
	if within, has := filter.allows([]byte("no stamp line")); within || has {
		t.Fatalf("无时间戳应 within=false has=false: within=%v has=%v", within, has)
	}

	// 未启用时恒通过。
	var disabled timeWindowFilter
	if within, has := disabled.allows([]byte("anything")); !within || !has {
		t.Fatal("未启用时间窗时任何内容都应通过")
	}
}

func TestNewTimeWindowFilterZoneMapping(t *testing.T) {
	utc := newTimeWindowFilter(SearchOptions{TimeWindowEnabled: true, TimeWindowZone: "utc"})
	if utc.zone != time.UTC {
		t.Fatalf("zone=utc 应映射到 time.UTC，实际 %v", utc.zone)
	}
	local := newTimeWindowFilter(SearchOptions{TimeWindowEnabled: true, TimeWindowZone: "local"})
	if local.zone != time.Local {
		t.Fatalf("zone=local 应映射到 time.Local，实际 %v", local.zone)
	}
	if newTimeWindowFilter(SearchOptions{}).enabled {
		t.Fatal("未显式启用时时间窗不应生效")
	}
}

// writeTempLog 在临时目录写入日志并返回路径。
func writeTempLog(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func windowOptions(start, end int64, extra SearchOptions) SearchOptions {
	extra.TimeWindowEnabled = true
	extra.TimeWindowStartMs = start
	extra.TimeWindowEndMs = end
	extra.TimeWindowZone = "utc"
	return extra
}

func TestSearchPlainRespectsTimeWindow(t *testing.T) {
	content := "2026-01-02 09:00:00 ERROR before\n" +
		"2026-01-02 10:30:00 ERROR inside\n" +
		"2026-01-02 12:00:00 ERROR after\n" +
		"ERROR no timestamp\n"
	path := writeTempLog(t, "win.log", content)

	start := utcMilli(2026, time.January, 2, 10, 0, 0)
	end := utcMilli(2026, time.January, 2, 11, 0, 0)
	got, err := SearchFileWithOptions(context.Background(), path, "ERROR", false, 10, windowOptions(start, end, SearchOptions{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 1 || got.Matches[0].LineNumber != 2 {
		t.Fatalf("时间窗应只保留窗口内命中：%+v", got.Matches)
	}
	if got.TimeWindowSkipped != 1 {
		t.Fatalf("无时间戳命中应计入跳过数 1，实际 %d", got.TimeWindowSkipped)
	}
}

func TestSearchEventContextRespectsTimeWindow(t *testing.T) {
	// 两个事件，各自以带时间戳的行起始；窗口只覆盖第二个。
	content := "2026-01-02 09:00:00 ERROR first\nstack A\n" +
		"2026-01-02 10:30:00 ERROR second\nstack B\n"
	path := writeTempLog(t, "evt.log", content)

	start := utcMilli(2026, time.January, 2, 10, 0, 0)
	end := utcMilli(2026, time.January, 2, 11, 0, 0)
	got, err := SearchFileWithOptions(context.Background(), path, "ERROR", false, 10,
		windowOptions(start, end, SearchOptions{EventContext: true}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 1 || got.Matches[0].LineNumber != 3 {
		t.Fatalf("事件时间窗应保留第二个事件：%+v", got.Matches)
	}
}

func TestSearchMultiLineJSONSkipsUndatedDocuments(t *testing.T) {
	// JSON 文档行首无日期时间戳，启用时间窗后应全部作为“无时间戳”跳过。
	content := "{\"level\":\"ERROR\",\"msg\":\"boom\"}\n"
	path := writeTempLog(t, "doc.log", content)

	start := utcMilli(2026, time.January, 2, 10, 0, 0)
	got, err := SearchFileWithOptions(context.Background(), path, "ERROR", false, 10,
		windowOptions(start, 0, SearchOptions{MultiLineJSON: true}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 0 {
		t.Fatalf("无日期文档在时间窗下不应入选：%+v", got.Matches)
	}
	if got.TimeWindowSkipped != 1 {
		t.Fatalf("无时间戳文档应计入跳过数，实际 %d", got.TimeWindowSkipped)
	}
}

func TestSearchTimeWindowDisabledKeepsAllMatches(t *testing.T) {
	content := "2026-01-02 09:00:00 ERROR a\nERROR plain\n"
	path := writeTempLog(t, "off.log", content)

	got, err := SearchFileWithOptions(context.Background(), path, "ERROR", false, 10, SearchOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 2 || got.TimeWindowSkipped != 0 {
		t.Fatalf("未启用时间窗时应保留全部命中：%+v skipped=%d", got.Matches, got.TimeWindowSkipped)
	}
}
