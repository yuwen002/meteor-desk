package main

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/yuwen002/meteor-desk/internal/logx"
)

type App struct { ctx context.Context }

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// SelectLogFile opens the native file picker and returns the chosen path.
func (a *App) SelectLogFile() (string, error) {
	if a.ctx == nil { return "", errors.New("desktop runtime is not ready") }
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Open a log file",
		Filters: []runtime.FileFilter{
			{DisplayName: "Log and text files", Pattern: "*.log;*.txt;*.out;*.json;*.ndjson"},
			{DisplayName: "All files", Pattern: "*"},
		},
	})
	if err != nil { return "", err }
	return path, nil
}

// SearchLogs searches incrementally and retains a bounded number of previews.
func (a *App) SearchLogs(path, query string, useRegex bool, maxResults int) (logx.SearchResponse, error) {
	return logx.SearchFile(path, query, useRegex, maxResults)
}
