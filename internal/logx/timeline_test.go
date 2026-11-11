package logx

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func writeTimelineFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("写入 %s 失败：%v", path, err)
	}
	return path
}

func TestDetectLevelFromBytesNormal(t *testing.T) {
	cases := []struct {
		line string
		want TimelineLevel
	}{
		{"2026-10-30 10:00:00 ERROR something broke", TimelineLevelError},
		{"2026-10-30 10:00:00 error: fatal crash", TimelineLevelError},
		{"2026-10-30 10:00:00 [WARN] disk almost full", TimelineLevelWarn},
		{"2026-10-30 10:00:00 WARNING slow query", TimelineLevelWarn},
		{"2026-10-30 10:00:00 INFO server started", TimelineLevelInfo},
		{"2026-10-30 10:00:00 DEBUG request body", TimelineLevelDebug},
		{"2026-10-30 10:00:00 TRACE enter func", TimelineLevelTrace},
		{"2026-10-30 10:00:00 NOTICE cron job ran", TimelineLevelInfo},
		{"2026-10-30 10:00:00 panic: nil pointer", TimelineLevelError},
		{"2026-10-30 10:00:00 exception at line 42", TimelineLevelError},
		{"2026-10-30 10:00:00 just a line with no level", TimelineLevelOther},
	}
	for _, c := range cases {
		got := detectLevelFromBytes([]byte(c.line))
		if got != c.want {
			t.Errorf("detectLevelFromBytes(%q) = %s, 想要 %s", c.line, got, c.want)
		}
	}
}

func TestScanTimelineSingleFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTimelineFile(t, dir, "single.log",
		"2026-10-30 10:00:00 INFO server started\n"+
			"2026-10-30 10:00:01 WARN port 8080 already in use\n"+
			"2026-10-30 10:00:02 ERROR failed to bind port\n"+
			"2026-10-30 10:00:03 INFO retrying on 8081\n")

	resp, err := ScanTimeline(context.Background(),
		[]TimelineFileSpec{{Path: path, SourceLabel: "test"}},
		TimelineScanOptions{MaxEntries: 100},
		nil,
	)
	if err != nil {
		t.Fatalf("ScanTimeline 失败：%v", err)
	}
	if resp.Canceled {
		t.Fatal("不应该被取消")
	}
	if resp.TotalEntries != 4 {
		t.Fatalf("想要 4 条，得到 %d", resp.TotalEntries)
	}
	if len(resp.Files) != 1 {
		t.Fatalf("想要 1 个文件结果，得到 %d", len(resp.Files))
	}
	fr := resp.Files[0]
	if !fr.Done {
		t.Error("文件应该标记为 Done")
	}
	if fr.Entries != 4 {
		t.Errorf("文件条目数想要 4，得到 %d", fr.Entries)
	}

	levels := make(map[TimelineLevel]int)
	for _, e := range resp.Entries {
		levels[e.Level]++
	}
	if levels[TimelineLevelError] != 1 {
		t.Errorf("想要 1 个 error，得到 %d", levels[TimelineLevelError])
	}
	if levels[TimelineLevelWarn] != 1 {
		t.Errorf("想要 1 个 warn，得到 %d", levels[TimelineLevelWarn])
	}
	if levels[TimelineLevelInfo] != 2 {
		t.Errorf("想要 2 个 info，得到 %d", levels[TimelineLevelInfo])
	}

	for i := 1; i < len(resp.Entries); i++ {
		if resp.Entries[i].TimestampMs < resp.Entries[i-1].TimestampMs {
			t.Errorf("时间戳应该单调递增，位置 %d 有 %d < %d",
				i, resp.Entries[i].TimestampMs, resp.Entries[i-1].TimestampMs)
		}
	}
}

func TestScanTimelineMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	p1 := writeTimelineFile(t, dir, "api.log",
		"2026-10-30 10:00:00 INFO api started\n"+
			"2026-10-30 10:00:05 ERROR api timeout\n")
	p2 := writeTimelineFile(t, dir, "db.log",
		"2026-10-30 10:00:02 INFO db connected\n"+
			"2026-10-30 10:00:10 WARN slow query\n")

	resp, err := ScanTimeline(context.Background(),
		[]TimelineFileSpec{
			{Path: p1, SourceLabel: "api"},
			{Path: p2, SourceLabel: "db"},
		},
		TimelineScanOptions{MaxEntries: 100},
		nil,
	)
	if err != nil {
		t.Fatalf("ScanTimeline 失败：%v", err)
	}
	if resp.TotalEntries != 4 {
		t.Fatalf("想要 4 条，得到 %d", resp.TotalEntries)
	}
	if len(resp.Files) != 2 {
		t.Fatalf("想要 2 个文件结果，得到 %d", len(resp.Files))
	}

	timeline := make([]int64, 0, 4)
	for _, e := range resp.Entries {
		timeline = append(timeline, e.TimestampMs)
	}
	if !sort.SliceIsSorted(timeline, func(i, j int) bool { return timeline[i] < timeline[j] }) {
		t.Error("合并后的时间线应该按时间戳排序")
	}

	labels := make(map[string]int)
	for _, e := range resp.Entries {
		labels[e.SourceLabel]++
	}
	if labels["api"] != 2 {
		t.Errorf("想要 api 贡献 2 条，得到 %d", labels["api"])
	}
	if labels["db"] != 2 {
		t.Errorf("想要 db 贡献 2 条，得到 %d", labels["db"])
	}
}

func TestScanTimelineNoTimestampLines(t *testing.T) {
	dir := t.TempDir()
	path := writeTimelineFile(t, dir, "mix.log",
		"2026-10-30 10:00:00 INFO with timestamp\n"+
			"INFO without any timestamp prefix\n"+
			"2026-10-30 10:00:01 ERROR with ts\n")

	resp, err := ScanTimeline(context.Background(),
		[]TimelineFileSpec{{Path: path}},
		TimelineScanOptions{MaxEntries: 100},
		nil,
	)
	if err != nil {
		t.Fatalf("ScanTimeline 失败：%v", err)
	}
	if resp.TotalEntries != 3 {
		t.Fatalf("想要 3 条，得到 %d", resp.TotalEntries)
	}
	foundNoTs := false
	for _, e := range resp.Entries {
		if e.Content == "INFO without any timestamp prefix" {
			if e.TimestampMs != 0 {
				t.Errorf("无时间戳行 TimestampMs 应该为 0，得到 %d", e.TimestampMs)
			}
			foundNoTs = true
		}
	}
	if !foundNoTs {
		t.Error("没有找到无时间戳的那行")
	}
}

func TestScanTimelineLimitReached(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sec := i % 60
		sb.WriteString("2026-10-30 10:00:")
		if sec < 10 {
			sb.WriteString("0")
		}
		sb.WriteString(strconv.Itoa(sec))
		sb.WriteString(" INFO line ")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("\n")
	}
	path := writeTimelineFile(t, dir, "big.log", sb.String())

	resp, err := ScanTimeline(context.Background(),
		[]TimelineFileSpec{{Path: path}},
		TimelineScanOptions{MaxEntries: 50},
		nil,
	)
	if err != nil {
		t.Fatalf("ScanTimeline 失败：%v", err)
	}
	if !resp.LimitReached {
		t.Error("应该报告 LimitReached=true")
	}
	if resp.TotalEntries > 50 {
		t.Errorf("不应该超过 50 条，得到 %d", resp.TotalEntries)
	}
}

func TestScanTimelineEmptyInput(t *testing.T) {
	_, err := ScanTimeline(context.Background(), nil, TimelineScanOptions{}, nil)
	if err == nil {
		t.Fatal("空输入应该报错")
	}
}

func TestScanTimelineProgressCallback(t *testing.T) {
	dir := t.TempDir()
	p1 := writeTimelineFile(t, dir, "a.log", "2026-10-30 10:00:00 INFO a\n")
	p2 := writeTimelineFile(t, dir, "b.log", "2026-10-30 10:00:01 INFO b\n")

	var lastProgress TimelineProgress
	called := 0
	onProgress := func(p TimelineProgress) {
		lastProgress = p
		called++
	}

	resp, err := ScanTimeline(context.Background(),
		[]TimelineFileSpec{
			{Path: p1},
			{Path: p2},
		},
		TimelineScanOptions{MaxEntries: 100},
		onProgress,
	)
	if err != nil {
		t.Fatalf("ScanTimeline 失败：%v", err)
	}
	if resp.TotalEntries != 2 {
		t.Fatalf("想要 2 条，得到 %d", resp.TotalEntries)
	}
	if called == 0 {
		t.Error("进度回调应该被调用")
	}
	if lastProgress.FilesDone != 2 {
		t.Errorf("最后进度应该显示 2 个文件完成，得到 %d", lastProgress.FilesDone)
	}
}

func TestScanTimelineCancel(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for i := 0; i < 10000; i++ {
		sb.WriteString("2026-10-30 10:00:00 INFO line ")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("\n")
	}
	path := writeTimelineFile(t, dir, "huge.log", sb.String())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := ScanTimeline(ctx,
		[]TimelineFileSpec{{Path: path}},
		TimelineScanOptions{MaxEntries: 20000},
		nil,
	)
	if err != nil {
		t.Fatalf("ScanTimeline 失败：%v", err)
	}
	if !resp.Canceled {
		t.Error("取消的 ctx 应该产生 Canceled=true")
	}
}

func TestScanTimelineMissingFile(t *testing.T) {
	resp, err := ScanTimeline(context.Background(),
		[]TimelineFileSpec{{Path: filepath.Join(t.TempDir(), "nonexistent.log")}},
		TimelineScanOptions{},
		nil,
	)
	if err != nil {
		t.Fatalf("ScanTimeline 不应整体失败：%v", err)
	}
	if len(resp.Files) != 1 || !resp.Files[0].Failed {
		t.Error("不存在的文件应该标记为 Failed")
	}
}

func TestFormatTimelineTimestamp(t *testing.T) {
	ms := int64(1698652800000)
	got := FormatTimelineTimestamp(ms, nil)
	if got == "" {
		t.Error("不应该返回空字符串")
	}
	got = FormatTimelineTimestamp(0, nil)
	if got != "" {
		t.Errorf("ms=0 应该返回空字符串，得到 %q", got)
	}
}