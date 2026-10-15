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
type searchJob struct { cancel context.CancelFunc; progress LogSearchProgress }

type App struct {
	ctx context.Context
	mu sync.RWMutex
	searches map[string]*searchJob
	droppedFiles []string
	processes *process.Manager
	settings config.Settings
	startupErr error
}
func NewApp() *App { return &App{searches:map[string]*searchJob{},settings:config.DefaultSettings()} }
func (a *App) startup(ctx context.Context) {
	a.ctx=ctx
	if settings,err:=config.LoadSettings();err==nil{a.settings=settings}
	a.processes,a.startupErr=process.NewManager()
	runtime.OnFileDrop(ctx,func(x,y int,paths []string){
		a.mu.Lock();defer a.mu.Unlock()
		for _,path:=range paths{if strings.TrimSpace(path)!=""{a.droppedFiles=append(a.droppedFiles,path)}}
	})
}

// shutdown 在窗口关闭时取消检索并停止本应用启动的本地服务。
func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	for _, job := range a.searches {
		if job.cancel != nil {
			job.cancel()
		}
	}
	a.mu.Unlock()

	if a.processes != nil {
		if err := a.processes.StopAll(); err != nil {
			runtime.LogError(ctx, "退出时停止服务失败："+err.Error())
		}
	}
}

func (a *App) SelectLogFile()(string,error){
	if a.ctx==nil{return "",errors.New("桌面运行环境尚未就绪")}
	path,err:=runtime.OpenFileDialog(a.ctx,runtime.OpenDialogOptions{Title:"选择日志文件",Filters:[]runtime.FileFilter{
		{DisplayName:"日志和文本文件",Pattern:"*.log;*.txt;*.out;*.json;*.ndjson"},
		{DisplayName:"所有文件",Pattern:"*"},
	}})
	return path,err
}
func (a *App) PollDroppedFiles()[]string{
	a.mu.Lock();defer a.mu.Unlock();paths:=append([]string(nil),a.droppedFiles...);a.droppedFiles=a.droppedFiles[:0];return paths
}
func (a *App) SearchLogs(searchID,path,query string,useRegex bool,maxResults int)(logx.SearchResponse,error){
	return a.SearchLogsAdvanced(searchID,path,query,useRegex,maxResults,0)
}
func (a *App) SearchLogsAdvanced(searchID,path,query string,useRegex bool,maxResults,contextLines int)(logx.SearchResponse,error){
	if a.ctx==nil{return logx.SearchResponse{},errors.New("桌面运行环境尚未就绪")}
	if searchID==""{return logx.SearchResponse{},errors.New("缺少检索任务编号")}
	a.mu.Lock();a.pruneSearchesLocked()
	if _,exists:=a.searches[searchID];exists{a.mu.Unlock();return logx.SearchResponse{},errors.New("检索任务编号重复")}
	searchCtx,cancel:=context.WithCancel(a.ctx)
	a.searches[searchID]=&searchJob{cancel:cancel,progress:LogSearchProgress{SearchID:searchID,Running:true}}
	a.mu.Unlock()
	response,err:=logx.SearchFileWithOptions(searchCtx,path,query,useRegex,maxResults,logx.SearchOptions{ContextLines:contextLines},func(p logx.Progress){a.updateSearchProgress(searchID,p)})
	a.mu.Lock()
	if current,ok:=a.searches[searchID];ok{
		current.cancel=nil;current.progress.Running=false;current.progress.Canceled=response.Canceled
		if err!=nil{current.progress.Error=err.Error()}
		if response.FileSizeBytes>0{current.progress.FileSizeBytes=response.FileSizeBytes}
		if response.BytesScanned>current.progress.BytesScanned{current.progress.BytesScanned=response.BytesScanned}
		if response.ScannedLines>current.progress.ScannedLines{current.progress.ScannedLines=response.ScannedLines}
		current.progress.MatchesFound=len(response.Matches)
		current.progress.Percent=calcPercent(current.progress.BytesScanned,current.progress.FileSizeBytes,false,response.Canceled||err!=nil)
	}
	if err==nil&&!response.Canceled&&path!=""{config.AddRecentFile(&a.settings,path)}
	settingsSnapshot:=a.settings
	a.mu.Unlock()
	cancel()
	if err==nil&&!response.Canceled{_ = config.SaveSettings(settingsSnapshot)}
	return response,err
}
func (a *App) GetLogSearchProgress(id string)(LogSearchProgress,error){
	a.mu.RLock();defer a.mu.RUnlock();job,ok:=a.searches[id];if !ok{return LogSearchProgress{},errors.New("未找到该检索任务")};return job.progress,nil
}
func (a *App) CancelLogSearch(id string)bool{
	a.mu.RLock();job,ok:=a.searches[id];if !ok||!job.progress.Running||job.cancel==nil{a.mu.RUnlock();return false};cancel:=job.cancel;a.mu.RUnlock();cancel();return true
}
func (a *App) GetAppSettings()config.Settings{a.mu.RLock();defer a.mu.RUnlock();return a.settings}
func (a *App) SaveAppSettings(settings config.Settings)error{
	config.NormalizeSettings(&settings);if err:=config.SaveSettings(settings);err!=nil{return err};a.mu.Lock();a.settings=settings;a.mu.Unlock();return nil
}
func (a *App) GetProcesses()([]process.View,error){if a.startupErr!=nil{return nil,a.startupErr};if a.processes==nil{return nil,errors.New("进程管理器尚未初始化")};return a.processes.List(),nil}
func (a *App) SaveProcess(spec process.Spec)(process.View,error){if a.startupErr!=nil{return process.View{},a.startupErr};if a.processes==nil{return process.View{},errors.New("进程管理器尚未初始化")};return a.processes.Save(spec)}
func (a *App) DeleteProcess(id string)error{if a.startupErr!=nil{return a.startupErr};if a.processes==nil{return errors.New("进程管理器尚未初始化")};return a.processes.Delete(id)}
func (a *App) StartProcess(id string)error{if a.startupErr!=nil{return a.startupErr};if a.processes==nil{return errors.New("进程管理器尚未初始化")};return a.processes.Start(id)}
func (a *App) StopProcess(id string)error{if a.startupErr!=nil{return a.startupErr};if a.processes==nil{return errors.New("进程管理器尚未初始化")};return a.processes.Stop(id)}
func (a *App) RestartProcess(id string)error{if a.startupErr!=nil{return a.startupErr};if a.processes==nil{return errors.New("进程管理器尚未初始化")};return a.processes.Restart(id)}
func (a *App) StartProcesses(ids []string)([]process.BatchResult,error){if a.startupErr!=nil{return nil,a.startupErr};if a.processes==nil{return nil,errors.New("进程管理器尚未初始化")};return a.processes.StartBatch(ids)}
func (a *App) StopProcesses(ids []string)([]process.BatchResult,error){if a.startupErr!=nil{return nil,a.startupErr};if a.processes==nil{return nil,errors.New("进程管理器尚未初始化")};return a.processes.StopBatch(ids)}

func (a *App) ExportProcessConfigs(redactSensitive bool) (string, error) {
	if a.ctx == nil { return "", errors.New("桌面运行环境尚未就绪") }
	if a.startupErr != nil { return "", a.startupErr }
	if a.processes == nil { return "", errors.New("进程管理器尚未初始化") }
	data, err := a.processes.ExportConfig(redactSensitive)
	if err != nil { return "", err }
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: "导出服务配置", DefaultFilename: "meteordesk-processes.json",
		Filters: []runtime.FileFilter{{DisplayName: "JSON 配置", Pattern: "*.json"}},
	})
	if err != nil || path == "" { return "", err }
	if err := os.WriteFile(path, data, 0600); err != nil { return "", err }
	_ = os.Chmod(path, 0600)
	if redactSensitive { return "已导出脱敏服务配置；敏感环境变量值为空，请导入后检查并补齐。", nil }
	return "已导出完整配置，文件包含真实环境变量值，请妥善保管。", nil
}

func (a *App) ImportProcessConfigs() ([]process.View, error) {
	if a.ctx == nil { return nil, errors.New("桌面运行环境尚未就绪") }
	if a.startupErr != nil { return nil, a.startupErr }
	if a.processes == nil { return nil, errors.New("进程管理器尚未初始化") }
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "导入服务配置",
		Filters: []runtime.FileFilter{{DisplayName: "JSON 配置", Pattern: "*.json"}},
	})
	if err != nil || path == "" { return []process.View{}, err }
	info, err := os.Stat(path)
	if err != nil { return nil, err }
	if info.Size() > 1<<20 { return nil, errors.New("导入文件过大，最大支持 1024 KiB") }
	data, err := os.ReadFile(path)
	if err != nil { return nil, err }
	return a.processes.ImportConfig(data)
}

func (a *App) GetProcessLogs(id string,after int64)([]process.OutputLine,error){if a.processes==nil{return nil,errors.New("进程管理器尚未初始化")};return a.processes.Logs(id,after),nil}
func (a *App) updateSearchProgress(id string,p logx.Progress){
	a.mu.Lock();defer a.mu.Unlock();job,ok:=a.searches[id];if !ok{return}
	job.progress.BytesScanned=p.BytesScanned;job.progress.FileSizeBytes=p.FileSizeBytes;job.progress.ScannedLines=p.ScannedLines;job.progress.MatchesFound=p.MatchesFound
	job.progress.Percent=calcPercent(p.BytesScanned,p.FileSizeBytes,job.progress.Running,false)
}
func (a *App) pruneSearchesLocked(){if len(a.searches)<32{return};for id,job:=range a.searches{if !job.progress.Running{delete(a.searches,id)};if len(a.searches)<24{return}}}
func calcPercent(scanned,total int64,running,canceled bool)int{
	if total<=0{if !running&&!canceled{return 100};return 0};percent:=int(float64(scanned)*100/float64(total));if percent>100{percent=100};if running&&percent>=100{return 99};return percent
}
func (a *App) UserDataDirectory()string{base,err:=os.UserConfigDir();if err!=nil{return ""};path:=filepath.Join(base,"MeteorDesk");_ = os.MkdirAll(path,0700);return path}
