package logx

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// pageScan 以固定每页上限反复续扫，直到某页不再触发上限或不可续扫，
// 返回合并后的结果；用于验证“分页续扫拼接 == 单次全量扫描”。
func pageScan(t *testing.T, path, query string, useRegex bool, pageSize int, base SearchOptions) SearchResponse {
	t.Helper()
	merged := SearchResponse{Matches: []Match{}}
	var off, line int64
	for {
		options := base
		options.StartOffset = off
		options.StartLine = line
		resp, err := SearchFileWithOptions(context.Background(), path, query, useRegex, pageSize, options, nil)
		if err != nil {
			t.Fatal(err)
		}
		merged.Matches = append(merged.Matches, resp.Matches...)
		merged.TimeWindowSkipped += resp.TimeWindowSkipped
		if !resp.ResultLimitReached || resp.ResumeOffset < 0 {
			return merged
		}
		if off == resp.ResumeOffset && line == resp.ResumeLine {
			t.Fatalf("续扫位置未推进，可能死循环：offset=%d line=%d", off, line)
		}
		off, line = resp.ResumeOffset, resp.ResumeLine
	}
}

// assertMatchesEqual 比对两次扫描返回的命中行号序列是否完全一致。
func assertMatchesEqual(t *testing.T, want, got SearchResponse) {
	t.Helper()
	if len(got.Matches) != len(want.Matches) {
		t.Fatalf("命中数量不一致：全量 %d，分页 %d", len(want.Matches), len(got.Matches))
	}
	for i := range want.Matches {
		if want.Matches[i].LineNumber != got.Matches[i].LineNumber {
			t.Fatalf("第 %d 条行号不一致：全量 %d，分页 %d", i, want.Matches[i].LineNumber, got.Matches[i].LineNumber)
		}
		if want.Matches[i].Content != got.Matches[i].Content {
			t.Fatalf("第 %d 条内容不一致：全量 %q，分页 %q", i, want.Matches[i].Content, got.Matches[i].Content)
		}
	}
}

func TestResumePlainPagesEqualSingleScan(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 20; i++ {
		if i%2 == 0 {
			fmt.Fprintf(&sb, "2026-01-02 10:%02d:00 ERROR boom %d\n", i, i)
		} else {
			fmt.Fprintf(&sb, "2026-01-02 10:%02d:00 INFO quiet %d\n", i, i)
		}
	}
	path := writeTempLog(t, "plain.log", sb.String())

	base := SearchOptions{ContextLines: 0}
	full, err := SearchFileWithOptions(context.Background(), path, "ERROR", false, 1000, base, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Matches) != 10 {
		t.Fatalf("全量应命中 10 行，实际 %d", len(full.Matches))
	}
	if full.ResumeOffset != -1 {
		t.Fatalf("全量扫描不应给出续扫点，实际 %d", full.ResumeOffset)
	}

	paged := pageScan(t, path, "ERROR", false, 3, base)
	assertMatchesEqual(t, full, paged)
}

func TestResumePlainReportsProgressiveOffset(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&sb, "line %d ERROR hit\n", i)
	}
	path := writeTempLog(t, "prog.log", sb.String())

	first, err := SearchFileWithOptions(context.Background(), path, "ERROR", false, 4, SearchOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !first.ResultLimitReached || first.ResumeOffset < 0 {
		t.Fatalf("首页应给出可续扫点：limit=%v offset=%d", first.ResultLimitReached, first.ResumeOffset)
	}
	if len(first.Matches) != 4 {
		t.Fatalf("首页应有 4 条，实际 %d", len(first.Matches))
	}

	// 每页同样受 maxResults 限制：第二页应继续返回接下来的 4 条（第 5-8 行）并可再次续扫。
	second, err := SearchFileWithOptions(context.Background(), path, "ERROR", false, 4, SearchOptions{
		StartOffset: first.ResumeOffset, StartLine: first.ResumeLine,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Matches) != 4 || second.Matches[0].LineNumber != 5 {
		t.Fatalf("第二页应为第 5-8 行共 4 条，实际 %d 条首行 %d", len(second.Matches), second.Matches[0].LineNumber)
	}
	if !second.ResultLimitReached || second.ResumeOffset < 0 {
		t.Fatal("第二页应仍达上限并给出续扫点")
	}

	// 第三页扫描到文件末尾，返回剩余 2 条（第 9-10 行），不再截断。
	third, err := SearchFileWithOptions(context.Background(), path, "ERROR", false, 4, SearchOptions{
		StartOffset: second.ResumeOffset, StartLine: second.ResumeLine,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Matches) != 2 || third.Matches[0].LineNumber != 9 {
		t.Fatalf("第三页应为第 9-10 行共 2 条，实际 %d 条首行 %d", len(third.Matches), third.Matches[0].LineNumber)
	}
	if third.ResultLimitReached || third.ResumeOffset != -1 {
		t.Fatal("第三页已扫描到末尾，不应截断或给出续扫点")
	}
}

func TestResumeDisabledForDecodedEncoding(t *testing.T) {
	// 0xC4 0xE3 不是合法 UTF-8 序列，自动识别回退到 GB18030，解码器路径不可续扫。
	var sb strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&sb, "ERROR \xC4\xE3 hit %d\n", i)
	}
	path := writeTempLog(t, "gb.log", sb.String())

	resp, err := SearchFileWithOptions(context.Background(), path, "ERROR", false, 3, SearchOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.ResultLimitReached {
		t.Fatal("应因结果上限而截断")
	}
	if resp.ResumeOffset != -1 {
		t.Fatalf("解码器编码不应可续扫，实际 offset=%d", resp.ResumeOffset)
	}
}

func TestResumeEventPagesEqualSingleScan(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&sb, "2026-01-02 11:%02d:00 ERROR event %d\n", i, i)
		sb.WriteString("\tat Frame()\n")
	}
	path := writeTempLog(t, "evt.log", sb.String())

	base := SearchOptions{EventContext: true}
	full, err := SearchFileWithOptions(context.Background(), path, "ERROR", false, 1000, base, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Matches) != 12 {
		t.Fatalf("全量事件应命中 12 个，实际 %d", len(full.Matches))
	}

	paged := pageScan(t, path, "ERROR", false, 5, base)
	assertMatchesEqual(t, full, paged)
}

func TestResumeMultiLineJSONPagesEqualSingleScan(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 9; i++ {
		fmt.Fprintf(&sb, "{\"level\":\"ERROR\",\"msg\":\"boom %d\"}\n", i)
	}
	path := writeTempLog(t, "docs.log", sb.String())

	base := SearchOptions{MultiLineJSON: true}
	full, err := SearchFileWithOptions(context.Background(), path, "ERROR", false, 1000, base, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Matches) != 9 {
		t.Fatalf("全量文档应命中 9 个，实际 %d", len(full.Matches))
	}

	paged := pageScan(t, path, "ERROR", false, 4, base)
	assertMatchesEqual(t, full, paged)
}
