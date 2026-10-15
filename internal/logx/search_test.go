package logx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchFileLiteralAndLineNumbers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.log")
	if err := os.WriteFile(path, []byte("启动\nINFO 已登录\nWARN 超时\nINFO 就绪"), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "INFO", false, 10)
	if err != nil { t.Fatal(err) }
	if got.ScannedLines != 4 { t.Fatalf("扫描行数 = %d，期望 4", got.ScannedLines) }
	if len(got.Matches) != 2 { t.Fatalf("匹配数量 = %d，期望 2", len(got.Matches)) }
	if got.Matches[0].LineNumber != 2 || got.Matches[1].LineNumber != 4 { t.Fatalf("匹配行号异常：%+v", got.Matches) }
}

func TestSearchFileRegexAndResultLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.log")
	if err := os.WriteFile(path, []byte("error 100\nerror 200\ninfo 300\nerror 400\n"), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "^error [0-9]+$", true, 2)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 2 || !got.ResultLimitReached { t.Fatalf("结果上限处理异常：%+v", got) }
}

func TestSearchFileClipsVeryLongLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "long.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxPreviewBytes+100)), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "xxx", false, 1)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 1 || !got.Matches[0].ContentClipped { t.Fatalf("应当返回截断预览：%+v", got.Matches) }
}

func TestSearchFileMatchesAcrossBufferBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boundary.log")
	content := strings.Repeat("a", readerBufferSize-3) + "MAGIC\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "MAGIC", false, 10)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 1 || got.Matches[0].LineNumber != 1 { t.Fatalf("跨缓冲区匹配失败：%+v", got.Matches) }
}

func TestSearchWithContextLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "context.log")
	content := "before-1\nbefore-2\nERROR target\nafter-1\nafter-2\nafter-3\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFileWithOptions(context.Background(), path, "ERROR", false, 10, SearchOptions{ContextLines:2}, nil)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 1 { t.Fatalf("匹配数量异常：%+v", got.Matches) }
	match := got.Matches[0]
	if len(match.ContextBefore) != 2 || match.ContextBefore[0].Content != "before-1" || match.ContextBefore[1].Content != "before-2" {
		t.Fatalf("前文异常：%+v", match.ContextBefore)
	}
	if len(match.ContextAfter) != 2 || match.ContextAfter[0].Content != "after-1" || match.ContextAfter[1].Content != "after-2" {
		t.Fatalf("后文异常：%+v", match.ContextAfter)
	}
}

func TestSearchFileCanCancelAndReportProgress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 3*1024*1024)), 0600); err != nil { t.Fatal(err) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reported := false
	got, err := SearchFileContext(ctx, path, "not-found", false, 10, func(p Progress) {
		if p.BytesScanned >= progressStepBytes && !reported { reported = true; cancel() }
	})
	if err != nil { t.Fatal(err) }
	if !reported || !got.Canceled { t.Fatalf("未按预期取消：reported=%v response=%+v", reported, got) }
	if got.BytesScanned >= got.FileSizeBytes { t.Fatalf("取消后不应扫描完整文件：%+v", got) }
}

func TestSearchFileRejectsOversizedRegexLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "regex-long.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxRegexLineBytes+1)), 0600); err != nil { t.Fatal(err) }
	_, err := SearchFile(path, "x+", true, 10)
	if err == nil || !strings.Contains(err.Error(), "正则模式") { t.Fatalf("应返回超限提示，实际错误：%v", err) }
}

func TestSearchFileHandlesCRLFAndBlankLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crlf.log")
	if err := os.WriteFile(path, []byte("\r\nalpha\r\nbeta\r\n"), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "alpha", false, 10)
	if err != nil { t.Fatal(err) }
	if got.ScannedLines != 3 { t.Fatalf("扫描行数 = %d，期望 3", got.ScannedLines) }
	if len(got.Matches) != 1 || got.Matches[0].LineNumber != 2 { t.Fatalf("CRLF 处理异常：%+v", got) }
}
