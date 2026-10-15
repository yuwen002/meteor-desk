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

// LogSearchProgress 是前端轮询使用的搜索任务状态 DTO。
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
	matches []logx.Match
}

// App 是 Wails 暴露给前端的后端桥接对象；公开方法即本地桌面接口。
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

// SelectLogFile 打开系统文件选择器，返回用户选中的日志路径。
func (a *App) SelectLogFile()(string,error){
	if a.ctx==nil{return "",errors.New("桌面运行环境尚未就绪")}
	path,err:=runtime.OpenFileDialog(a.ctx,runtime.OpenDialogOptions{Title:"选择日志文件",Filters:[]runtime.FileFilter{
		{DisplayName:"日志和文本文件",Pattern:"*.log;*.txt;*.out;*.json;*.ndjson"},
		{DisplayName:"所有文件",Pattern:"*"},
	}})
	return path,err
}
// PollDroppedFiles 取出并清空桌面拖放事件中积累的文件路径。
func (a *App) PollDroppedFiles()[]string{
	a.mu.Lock();defer a.mu.Unlock();paths:=append([]string(nil),a.droppedFiles...);a.droppedFiles=a.droppedFiles[:0];return paths
}
// SearchLogs 执行不带上下文行的日志搜索，保留旧版桥接调用入口。
func (a *App) SearchLogs(searchID,path,query string,useRegex bool,maxResults int)(logx.SearchResponse,error){
	return a.SearchLogsAdvanced(searchID,path,query,useRegex,maxResults,0)
}
// SearchLogsAdvanced 注册一个可取消的搜索任务；扫描期间通过进度和增量匹配接口读取状态。
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
// GetLogSearchProgress 返回任务当前的字节数、行数、匹配数和运行状态。
// GetLogSearchMatches 返回 afterIndex 之后新到达的匹配结果，避免重复传输完整结果集。
func (a *App) GetLogSearchMatches(id string, afterIndex int)([]logx.Match,error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	job, ok := a.searches[id]
	if !ok {
		return nil, errors.New("未找到该检索任务")
	}
	if afterIndex < 0 {
		afterIndex = 0
	}
	if afterIndex >= len(job.matches) {
		return []logx.Match{}, nil
	}
	matches := make([]logx.Match, len(job.matches)-afterIndex)
	for i, match := range job.matches[afterIndex:] {
		match.ContextBefore = append([]logx.LineContext(nil), match.ContextBefore...)
		match.ContextAfter = append([]logx.LineContext(nil), match.ContextAfter...)
		matches[i] = match
	}
	return matches, nil
}

// GetLogSearchProgress 返回任务当前的字节数、行数、匹配数和运行状态。
func (a *App) GetLogSearchProgress(id string)(LogSearchProgress,error){
	a.mu.RLock();defer a.mu.RUnlock();job,ok:=a.searches[id];if !ok{return LogSearchProgress{},errors.New("未找到该检索任务")};return job.progress,nil
}
// CancelLogSearch 请求取消仍在运行的搜索；返回 false 表示任务不存在或已结束。
func (a *App) CancelLogSearch(id string)bool{
	a.mu.RLock();job,ok:=a.searches[id];if !ok||!job.progress.Running||job.cancel==nil{a.mu.RUnlock();return false};cancel:=job.cancel;a.mu.RUnlock();cancel();return true
}
// GetAppSettings 返回当前本地设置的副本。
func (a *App) GetAppSettings()config.Settings{a.mu.RLock();defer a.mu.RUnlock();return a.settings}
// SaveAppSettings 规范化并持久化界面偏好和搜索历史。
func (a *App) SaveAppSettings(settings config.Settings)error{
	config.NormalizeSettings(&settings);if err:=config.SaveSettings(settings);err!=nil{return err};a.mu.Lock();a.settings=settings;a.mu.Unlock();return nil
}
// GetProcesses 返回所有已配置服务及其运行快照。
func (a *App) GetProcesses()([]process.View,error){if a.startupErr!=nil{return nil,a.startupErr};if a.processes==nil{return nil,errors.New("进程管理器尚未初始化")};return a.processes.List(),nil}
// SaveProcess 新建或更新服务配置；配置保存本身不会执行命令。
func (a *App) SaveProcess(spec process.Spec)(process.View,error){if a.startupErr!=nil{return process.View{},a.startupErr};if a.processes==nil{return process.View{},errors.New("进程管理器尚未初始化")};return a.processes.Save(spec)}
// DeleteProcess 删除已停止且未被其他服务引用的配置。
func (a *App) DeleteProcess(id string)error{if a.startupErr!=nil{return a.startupErr};if a.processes==nil{return errors.New("进程管理器尚未初始化")};return a.processes.Delete(id)}
// StartProcess 启动单个服务及其尚未运行的上游依赖。
func (a *App) StartProcess(id string)error{if a.startupErr!=nil{return a.startupErr};if a.processes==nil{return errors.New("进程管理器尚未初始化")};return a.processes.Start(id)}
// StopProcess 停止目标服务，并按依赖关系先停止依赖它的服务。
func (a *App) StopProcess(id string)error{if a.startupErr!=nil{return a.startupErr};if a.processes==nil{return errors.New("进程管理器尚未初始化")};return a.processes.Stop(id)}
// RestartProcess 在旧实例退出后重新启动服务。
func (a *App) RestartProcess(id string)error{if a.startupErr!=nil{return a.startupErr};if a.processes==nil{return errors.New("进程管理器尚未初始化")};return a.processes.Restart(id)}
// StartProcesses 按拓扑顺序批量启动所选服务及其依赖，并逐项返回执行结果。
func (a *App) StartProcesses(ids []string)([]process.BatchResult,error){if a.startupErr!=nil{return nil,a.startupErr};if a.processes==nil{return nil,errors.New("进程管理器尚未初始化")};return a.processes.StartBatch(ids)}
// StopProcesses 按逆依赖顺序批量停止所选服务和依赖方。
func (a *App) StopProcesses(ids []string)([]process.BatchResult,error){if a.startupErr!=nil{return nil,a.startupErr};if a.processes==nil{return nil,errors.New("进程管理器尚未初始化")};return a.processes.StopBatch(ids)}

// ExportProcessConfigs 通过系统保存对话框导出服务定义；可选择脱敏环境变量值。
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

// ImportProcessConfigs 通过系统文件选择器导入 JSON 服务配置，不会自动启动导入项。
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

// GetProcessLogs 返回序号大于 after 的新输出行，用于前端增量轮询日志。
func (a *App) GetProcessLogs(id string,after int64)([]process.OutputLine,error){if a.processes==nil{return nil,errors.New("进程管理器尚未初始化")};return a.processes.Logs(id,after),nil}
// updateSearchProgress 在锁内更新进度快照，并按到达顺序缓存新发现的匹配项。
func (a *App) updateSearchProgress(id string,p logx.Progress){
	a.mu.Lock();defer a.mu.Unlock();job,ok:=a.searches[id];if !ok{return}
	job.progress.BytesScanned=p.BytesScanned;job.progress.FileSizeBytes=p.FileSizeBytes;job.progress.ScannedLines=p.ScannedLines;job.progress.MatchesFound=p.MatchesFound
	if len(p.Matches) > 0 {
		job.matches = append(job.matches, p.Matches...)
	}
	job.progress.Percent=calcPercent(p.BytesScanned,p.FileSizeBytes,job.progress.Running,false)
}
func (a *App) pruneSearchesLocked(){if len(a.searches)<32{return};for id,job:=range a.searches{if !job.progress.Running{delete(a.searches,id)};if len(a.searches)<24{return}}}
func calcPercent(scanned,total int64,running,canceled bool)int{
	if total<=0{if !running&&!canceled{return 100};return 0};percent:=int(float64(scanned)*100/float64(total));if percent>100{percent=100};if running&&percent>=100{return 99};return percent
}
// UserDataDirectory 返回应用本地配置目录，目录会在需要时创建。
func (a *App) UserDataDirectory()string{base,err:=os.UserConfigDir();if err!=nil{return ""};path:=filepath.Join(base,"MeteorDesk");_ = os.MkdirAll(path,0700);return path}
