package main

import (
	"context"
	"errors"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/yuwen002/meteor-desk/internal/logx"
)

// LogSearchProgress 是界面展示检索进度所需的信息。
type LogSearchProgress struct {
	SearchID      string `json:"searchId"`
	BytesScanned  int64  `json:"bytesScanned"`
	FileSizeBytes int64  `json:"fileSizeBytes"`
	ScannedLines  int64  `json:"scannedLines"`
	MatchesFound  int    `json:"matchesFound"`
	Percent       int    `json:"percent"`
	Running       bool   `json:"running"`
	Canceled      bool   `json:"canceled"`
	Error         string `json:"error,omitempty"`
}

type searchJob struct {
	cancel   context.CancelFunc
	progress LogSearchProgress
}

// App 负责在 Vue 界面与桌面原生能力之间进行交互。
type App struct {
	ctx      context.Context
	mu       sync.RWMutex
	searches map[string]*searchJob
}

func NewApp() *App {
	return &App{searches: make(map[string]*searchJob)}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// SelectLogFile 打开系统原生文件选择器。
func (a *App) SelectLogFile() (string, error) {
	if a.ctx == nil {
		return "", errors.New("桌面运行环境尚未就绪")
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择日志文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "日志和文本文件", Pattern: "*.log;*.txt;*.out;*.json;*.ndjson"},
			{DisplayName: "所有文件", Pattern: "*"},
		},
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// SearchLogs 启动流式检索。searchID 用于查询进度或取消当前任务。
func (a *App) SearchLogs(searchID, path, query string, useRegex bool, maxResults int) (logx.SearchResponse, error) {
	if a.ctx == nil {
		return logx.SearchResponse{}, errors.New("桌面运行环境尚未就绪")
	}
	if searchID == "" {
		return logx.SearchResponse{}, errors.New("缺少检索任务编号")
	}

	a.mu.Lock()
	a.pruneSearchesLocked()
	if _, exists := a.searches[searchID]; exists {
		a.mu.Unlock()
		return logx.SearchResponse{}, errors.New("检索任务编号重复")
	}
	searchCtx, cancel := context.WithCancel(a.ctx)
	job := &searchJob{
		cancel: cancel,
		progress: LogSearchProgress{SearchID: searchID, Running: true},
	}
	a.searches[searchID] = job
	a.mu.Unlock()

	response, err := logx.SearchFileContext(searchCtx, path, query, useRegex, maxResults, func(p logx.Progress) {
		a.updateSearchProgress(searchID, p)
	})

	a.mu.Lock()
	if current, ok := a.searches[searchID]; ok {
		current.cancel = nil
		current.progress.Running = false
		current.progress.Canceled = response.Canceled
		if err != nil {
			current.progress.Error = err.Error()
		}
		if response.FileSizeBytes > 0 {
			current.progress.FileSizeBytes = response.FileSizeBytes
		}
		if response.BytesScanned > current.progress.BytesScanned {
			current.progress.BytesScanned = response.BytesScanned
		}
		if response.ScannedLines > current.progress.ScannedLines {
			current.progress.ScannedLines = response.ScannedLines
		}
		current.progress.MatchesFound = len(response.Matches)
		current.progress.Percent = calcPercent(current.progress.BytesScanned, current.progress.FileSizeBytes, false, response.Canceled || err != nil)
	}
	a.mu.Unlock()
	cancel()
	return response, err
}

// GetLogSearchProgress 返回检索进度，界面可定时轮询。
func (a *App) GetLogSearchProgress(searchID string) (LogSearchProgress, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	job, ok := a.searches[searchID]
	if !ok {
		return LogSearchProgress{}, errors.New("未找到该检索任务")
	}
	return job.progress, nil
}

// CancelLogSearch 请求取消指定的检索任务。
func (a *App) CancelLogSearch(searchID string) bool {
	a.mu.RLock()
	job, ok := a.searches[searchID]
	if !ok || !job.progress.Running || job.cancel == nil {
		a.mu.RUnlock()
		return false
	}
	cancel := job.cancel
	a.mu.RUnlock()
	cancel()
	return true
}

func (a *App) updateSearchProgress(searchID string, p logx.Progress) {
	a.mu.Lock()
	defer a.mu.Unlock()
	job, ok := a.searches[searchID]
	if !ok {
		return
	}
	job.progress.BytesScanned = p.BytesScanned
	job.progress.FileSizeBytes = p.FileSizeBytes
	job.progress.ScannedLines = p.ScannedLines
	job.progress.MatchesFound = p.MatchesFound
	job.progress.Percent = calcPercent(p.BytesScanned, p.FileSizeBytes, job.progress.Running, false)
}

func (a *App) pruneSearchesLocked() {
	if len(a.searches) < 32 {
		return
	}
	for id, job := range a.searches {
		if !job.progress.Running {
			delete(a.searches, id)
		}
		if len(a.searches) < 24 {
			return
		}
	}
}

func calcPercent(scanned, total int64, running, canceled bool) int {
	if total <= 0 {
		if !running && !canceled {
			return 100
		}
		return 0
	}
	percent := int(float64(scanned) * 100 / float64(total))
	if percent > 100 {
		percent = 100
	}
	if running && percent >= 100 {
		return 99
	}
	return percent
}
