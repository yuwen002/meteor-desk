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
	if err := os.WriteFile(path, []byte("启动\nINFO 已登录\nWARN 超时\nINFO 就绪"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := SearchFile(path, "INFO", false, 10)
	if err != nil { t.Fatal(err) }
	if got.ScannedLines != 4 { t.Fatalf("扫描行数 = %d，期望 4", got.ScannedLines) }
	if len(got.Matches) != 2 { t.Fatalf("匹配数量 = %d，期望 2", len(got.Matches)) }
	if got.Matches[0].LineNumber != 2 || got.Matches[1].LineNumber != 4 {
		t.Fatalf("匹配行号不符合预期：%+v", got.Matches)
	}
}

func TestSearchFileRegexAndResultLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.log")
	if err := os.WriteFile(path, []byte("error 100\nerror 200\ninfo 300\nerror 400\n"), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "^error [0-9]+$", true, 2)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 2 || !got.ResultLimitReached { t.Fatalf("结果数量上限处理异常：%+v", got) }
}

func TestSearchFileClipsVeryLongLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "long.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxPreviewBytes+100)), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "xxx", false, 1)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 1 || !got.Matches[0].ContentClipped { t.Fatalf("应当返回被截断的预览：%+v", got) }
}

func TestSearchFileMatchesAcrossBufferBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "boundary.log")
	content := strings.Repeat("a", readerBufferSize-3) + "MAGIC" + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil { t.Fatal(err) }
	got, err := SearchFile(path, "MAGIC", false, 10)
	if err != nil { t.Fatal(err) }
	if len(got.Matches) != 1 || got.Matches[0].LineNumber != 1 {
		t.Fatalf("跨缓冲区边界的关键字未被正确匹配：%+v", got.Matches)
	}
}

func TestSearchFileCanCancelAndReportProgress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 3*1024*1024)), 0600); err != nil { t.Fatal(err) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reported := false
	got, err := SearchFileContext(ctx, path, "not-found", false, 10, func(p Progress) {
		if p.BytesScanned >= progressStepBytes && !reported {
			reported = true
			cancel()
		}
	})
	if err != nil { t.Fatal(err) }
	if !reported || !got.Canceled { t.Fatalf("检索未按预期取消：progress=%v response=%+v", reported, got) }
	if got.BytesScanned >= got.FileSizeBytes { t.Fatalf("取消后不应扫描完整个文件：%+v", got) }
}

func TestSearchFileRejectsOversizedRegexLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "regex-long.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxRegexLineBytes+1)), 0600); err != nil { t.Fatal(err) }
	_, err := SearchFile(path, "x+", true, 10)
	if err == nil || !strings.Contains(err.Error(), "正则模式") {
		t.Fatalf("超长行的正则检索应返回中文说明，实际错误：%v", err)
	}
}
