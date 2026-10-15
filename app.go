package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/yuwen002/meteor-desk/internal/config"
	"github.com/yuwen002/meteor-desk/internal/logx"
	"github.com/yuwen002/meteor-desk/internal/process"
)

type LogSearchProgress struct {
	SearchID string `json:"searchId"`
	BytesScanned int64 `json:"bytesScanned"`
	FileSizeBytes int64 `json:"fileSizeBytes"`
	ScannedLines int64 `json:"scannedLines"`
	MatchesFound int `json:"matchesFound"`
	Percent int `json:"percent"`
	Running bool `json:"running"`
	Canceled bool `json:"canceled"`
	Error string `json:"error,omitempty"`
}

type searchJob struct {
	cancel context.CancelFunc
	progress LogSearchProgress
}

// App 是界面与原生桌面能力之间的桥接层。
type App struct {
	ctx context.Context
	mu sync.RWMutex
	searches map[string]*searchJob
	droppedFiles []string
	processes *process.Manager
	settings config.Settings
	startupErr error
}

func NewApp() *App {
	return &App{searches:make(map[string]*searchJob), settings:config.DefaultSettings()}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.settings, a.startupErr = config.LoadSettings()
	if a.startupErr != nil { a.settings = config.DefaultSettings() }
	a.processes, a.startupErr = process.NewManager()
	// Wails 会返回丢入窗口的绝对文件路径，避免前端尝试猜测本地文件路径。
	runtime.OnFileDrop(ctx, func(x, y int, paths []string) {
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, path := range paths {
			if strings.TrimSpace(path) != "" { a.droppedFiles = append(a.droppedFiles, path) }
		}
	})
}

// SelectLogFile 打开系统原生文件选择器。
func (a *App) SelectLogFile() (string, error) {
	if a.ctx == nil { return "", errors.New("桌面运行环境尚未就绪") }
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:"选择日志文件",
		Filters:[]runtime.FileFilter{
			{DisplayName:"日志和文本文件", Pattern:"*.log;*.txt;*.out;*.json;*.ndjson"},
			{DisplayName:"所有文件", Pattern:"*"},
		},
	})
	if err != nil { return "", err }
	return path, nil
}

// PollDroppedFiles 返回新拖入窗口的文件路径；路径由 Wails 原生拖放接口提供。
func (a *App) PollDroppedFiles() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	paths := append([]string(nil), a.droppedFiles...)
	a.droppedFiles = a.droppedFiles[:0]
	return paths
}

func (a *App) SearchLogs(searchID, path, query string, useRegex bool, maxResults int) (logx.SearchResponse, error) {
	return a.SearchLogsAdvanced(searchID, path, query, useRegex, maxResults, 0)
}

// SearchLogsAdvanced 支持上下文行的流式检索。
func (a *App) SearchLogsAdvanced(searchID, path, query string, useRegex bool, maxResults, contextLines int) (logx.SearchResponse, error) {
	if a.ctx == nil { return logx.SearchResponse{}, errors.New("桌面运行环境尚未就绪") }
	if searchID == "" { return logx.SearchResponse{}, errors.New("缺少检索任务编号") }

	a.mu.Lock()
	a.pruneSearchesLocked()
	if _, exists := a.searches[searchID]; exists {
		a.mu.Unlock()
		return logx.SearchResponse{}, errors.New("检索任务编号重复")
	}
	searchCtx, cancel := context.WithCancel(a.ctx)
	job := &searchJob{cancel:cancel, progress:LogSearchProgress{SearchID:searchID, Running:true}}
	a.searches[searchID] = job
	a.mu.Unlock()

	response, err := logx.SearchFileWithOptions(searchCtx, path, query, useRegex, maxResults,
		logx.SearchOptions{ContextLines:contextLines},
		func(p logx.Progress) { a.updateSearchProgress(searchID, p) })

	a.mu.Lock()
	if current, ok := a.searches[searchID]; ok {
		current.cancel = nil
		current.progress.Running = false
		current.progress.Canceled = response.Canceled
		if err != nil { current.progress.Error = err.Error() }
		if response.FileSizeBytes > 0 { current.progress.FileSizeBytes = response.FileSizeBytes }
		if response.BytesScanned > current.progress.BytesScanned { current.progress.BytesScanned = response.BytesScanned }
		if response.ScannedLines > current.progress.ScannedLines { current.progress.ScannedLines = response.ScannedLines }
		current.progress.MatchesFound = len(response.Matches)
		current.progress.Percent = calcPercent(current.progress.BytesScanned, current.progress.FileSizeBytes, false, response.Canceled || err != nil)
	}
	a.mu.Unlock()
	cancel()

	// 将最近文件记录持久化；设置读写与检索任务锁分离。
	if err == nil && !response.Canceled && path != "" {
		a.mu.Lock()
		config.AddRecentFile(&a.settings, path)
		settingsSnapshot := a.settings
		a.mu.Unlock()
		_ = config.SaveSettings(settingsSnapshot)
	}
	return response, err
}

func (a *App) GetLogSearchProgress(searchID string) (LogSearchProgress, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	job, ok := a.searches[searchID]
	if !ok { return LogSearchProgress{}, errors.New("未找到该检索任务") }
	return job.progress, nil
}

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

func (a *App) GetAppSettings() config.Settings {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.settings
}

func (a *App) SaveAppSettings(settings config.Settings) error {
	config.NormalizeSettings(&settings)
	if err := config.SaveSettings(settings); err != nil { return err }
	a.mu.Lock()
	a.settings = settings
	a.mu.Unlock()
	return nil
}

func (a *App) GetProcesses() ([]process.View, error) {
	if a.startupErr != nil { return nil, a.startupErr }
	if a.processes == nil { return nil, errors.New("进程管理器尚未初始化") }
	return a.processes.List(), nil
}

func (a *App) SaveProcess(spec process.Spec) (process.View, error) {
	if a.startupErr != nil { return process.View{}, a.startupErr }
	if a.processes == nil { return process.View{}, errors.New("进程管理器尚未初始化") }
	return a.processes.Save(spec)
}

func (a *App) DeleteProcess(id string) error {
	if a.startupErr != nil { return a.startupErr }
	if a.processes == nil { return errors.New("进程管理器尚未初始化") }
	return a.processes.Delete(id)
}

func (a *App) StartProcess(id string) error {
	if a.startupErr != nil { return a.startupErr }
	if a.processes == nil { return errors.New("进程管理器尚未初始化") }
	return a.processes.Start(id)
}

func (a *App) StopProcess(id string) error {
	if a.startupErr != nil { return a.startupErr }
	if a.processes == nil { return errors.New("进程管理器尚未初始化") }
	return a.processes.Stop(id)
}

func (a *App) RestartProcess(id string) error {
	if a.startupErr != nil { return a.startupErr }
	if a.processes == nil { return errors.New("进程管理器尚未初始化") }
	return a.processes.Restart(id)
}

func (a *App) GetProcessLogs(id string, afterSequence int64) ([]process.OutputLine, error) {
	if a.processes == nil { return nil, errors.New("进程管理器尚未初始化") }
	return a.processes.Logs(id, afterSequence), nil
}

func (a *App) updateSearchProgress(searchID string, p logx.Progress) {
	a.mu.Lock()
	defer a.mu.Unlock()
	job, ok := a.searches[searchID]
	if !ok { return }
	job.progress.BytesScanned = p.BytesScanned
	job.progress.FileSizeBytes = p.FileSizeBytes
	job.progress.ScannedLines = p.ScannedLines
	job.progress.MatchesFound = p.MatchesFound
	job.progress.Percent = calcPercent(p.BytesScanned, p.FileSizeBytes, job.progress.Running, false)
}

func (a *App) pruneSearchesLocked() {
	if len(a.searches) < 32 { return }
	for id, job := range a.searches {
		if !job.progress.Running { delete(a.searches, id) }
		if len(a.searches) < 24 { return }
	}
}

func calcPercent(scanned, total int64, running, canceled bool) int {
	if total <= 0 {
		if !running && !canceled { return 100 }
		return 0
	}
	percent := int(float64(scanned) * 100 / float64(total))
	if percent > 100 { percent = 100 }
	if running && percent >= 100 { return 99 }
	return percent
}

func (a *App) UserDataDirectory() string {
	base, err := os.UserConfigDir()
	if err != nil { return "" }
	path := filepath.Join(base, "MeteorDesk")
	_ = os.MkdirAll(path, 0700)
	return path
}
