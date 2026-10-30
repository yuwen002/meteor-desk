package main

import (
	"context"
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
