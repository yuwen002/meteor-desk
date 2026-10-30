package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yuwen002/meteor-desk/internal/logx"
)

func TestSearchLogsRejectsDuplicateTaskID(t *testing.T) {
	app := NewApp()
	app.ctx = context.Background()
	app.searches["duplicate"] = &searchJob{
		progress: LogSearchProgress{SearchID: "duplicate", Running: false},
		completedAt: time.Now(),
	}

	_, err := app.SearchLogsAdvanced("duplicate", "unused.log", "needle", false, 10, 0, "auto", false, false, "", "")
	if err == nil || err.Error() != "检索任务编号重复" {
		t.Fatalf("重复任务 ID 应被拒绝，实际错误：%v", err)
	}
}

func TestCanceledSearchKeepsIncrementalResultsUntilExpiry(t *testing.T) {
	app := NewApp()
	match := logx.Match{LineNumber: 7, Content: "needle found"}
	app.searches["canceled"] = &searchJob{
		progress: LogSearchProgress{SearchID: "canceled", Running: true},
		matches: []logx.Match{match},
	}

	response := logx.SearchResponse{
		Canceled: true,
		Matches: []logx.Match{match},
		FileSizeBytes: 100,
		BytesScanned: 50,
		ScannedLines: 7,
	}
	app.finishSearchJob("canceled", "", response, nil)

	got, err := app.GetLogSearchMatches("canceled", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].LineNumber != 7 || got[0].Content != "needle found" {
		t.Fatalf("取消后应保留已发现的增量结果：%+v", got)
	}
	progress, err := app.GetLogSearchProgress("canceled")
	if err != nil {
		t.Fatal(err)
	}
	if progress.Running || !progress.Canceled {
		t.Fatalf("取消任务状态错误：%+v", progress)
	}
}

func TestSearchMatchesCursorBoundaries(t *testing.T) {
	app := NewApp()
	app.searches["cursor"] = &searchJob{
		progress: LogSearchProgress{SearchID: "cursor"},
		matches: []logx.Match{
			{LineNumber: 1, Content: "first"},
			{LineNumber: 2, Content: "second"},
		},
	}

	tests := []struct {
		name string
		after int
		want []int64
	}{
		{name: "negative starts at beginning", after: -1, want: []int64{1, 2}},
		{name: "zero starts at beginning", after: 0, want: []int64{1, 2}},
		{name: "middle cursor", after: 1, want: []int64{2}},
		{name: "exact end", after: 2, want: []int64{}},
		{name: "past end", after: 99, want: []int64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := app.GetLogSearchMatches("cursor", tt.after)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("结果数量=%d，期望=%d：%+v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i].LineNumber != tt.want[i] {
					t.Fatalf("游标结果不匹配：%+v", got)
				}
			}
		})
	}
}

func TestConcurrentSearchPollingIsSafe(t *testing.T) {
	app := NewApp()
	app.searches["poll"] = &searchJob{
		progress: LogSearchProgress{SearchID: "poll", Running: true},
		matches: []logx.Match{{LineNumber: 1, Content: "first"}},
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if _, err := app.GetLogSearchMatches("poll", 0); err != nil {
					t.Errorf("并发读取增量结果失败：%v", err)
					return
				}
				if _, err := app.GetLogSearchProgress("poll"); err != nil {
					t.Errorf("并发读取进度失败：%v", err)
					return
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			app.updateSearchProgress("poll", logx.Progress{
				BytesScanned: int64(i),
				FileSizeBytes: 100,
				ScannedLines: int64(i),
				MatchesFound: i + 1,
				Matches: []logx.Match{{LineNumber: int64(i + 2), Content: "incremental"}},
			})
		}
	}()
	wg.Wait()
}

func TestPruneSearchesExpiresCompletedJobsButKeepsRunningJobs(t *testing.T) {
	app := NewApp()
	app.searches["expired"] = &searchJob{
		progress: LogSearchProgress{SearchID: "expired", Running: false},
		completedAt: time.Now().Add(-searchJobRetention - time.Second),
	}
	app.searches["running"] = &searchJob{
		progress: LogSearchProgress{SearchID: "running", Running: true},
	}
	app.mu.Lock()
	app.pruneSearchesLocked()
	app.mu.Unlock()

	if _, ok := app.searches["expired"]; ok {
		t.Fatal("超过保留期的已完成任务应被清理")
	}
	if _, ok := app.searches["running"]; !ok {
		t.Fatal("清理任务不能移除运行中的任务")
	}
}

func TestGetRuntimeInfoReportsVersionAndBuildEnvironment(t *testing.T) {
	info := NewApp().GetRuntimeInfo()
	if info.AppVersion == "" {
		t.Fatal("应用版本不应为空")
	}
	if !strings.HasPrefix(info.GoVersion, "go") {
		t.Fatalf("Go 运行时版本异常：%q", info.GoVersion)
	}
	if !strings.Contains(info.Platform, "/") {
		t.Fatalf("平台信息应包含 GOOS/GOARCH：%q", info.Platform)
	}
	if !strings.HasSuffix(info.DataDir, "MeteorDesk") {
		t.Fatalf("配置目录应位于 MeteorDesk 子目录：%q", info.DataDir)
	}
}

func TestSaveExportFileValidatesContentAndFormat(t *testing.T) {
	app := NewApp()
	if _, err := app.SaveExportFile("result.csv", "csv", "  "); err == nil {
		t.Fatal("空内容应被拒绝")
	}
	if _, err := app.SaveExportFile("result.txt", "txt", "a,b"); err == nil {
		t.Fatal("不支持的格式应被拒绝")
	}
	if _, err := app.SaveExportFile("result.csv", "csv", strings.Repeat("x", maxExportContentBytes+1)); err == nil {
		t.Fatal("超过上限的内容应被拒绝")
	}
	if _, err := app.SaveExportFile("result.csv", "csv", "a,b"); err == nil || err.Error() != "桌面运行环境尚未就绪" {
		t.Fatalf("未初始化的桌面环境应返回明确错误：%v", err)
	}
}

func TestExportFileNameStripsPathsAndFixesExtension(t *testing.T) {
	tests := []struct{ in, format, want string }{
		{in: `C:\logs\app.log`, format: "csv", want: "app.log.csv"},
		{in: "../../etc/passwd", format: "json", want: "passwd.json"},
		{in: "report.JSON", format: "json", want: "report.JSON"},
		{in: "   ", format: "csv", want: "meteordesk-export.csv"},
		{in: "a:b*.csv", format: "csv", want: "a-b-.csv"},
	}
	for _, tt := range tests {
		if got := exportFileName(tt.in, tt.format); got != tt.want {
			t.Fatalf("exportFileName(%q, %q)=%q，期望 %q", tt.in, tt.format, got, tt.want)
		}
	}
}
