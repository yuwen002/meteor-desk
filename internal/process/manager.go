// Package process 管理本地开发服务，配置保存在用户目录，输出只保留有限缓存。
package process

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxOutputLines = 1500
const maxOutputLineBytes = 64 * 1024
const maxConfigImportBytes = 1 << 20
const maxServiceConfigs = 500

type OutputLine struct {
	Sequence int64 `json:"sequence"`
	Time string `json:"time"`
	Stream string `json:"stream"`
	Text string `json:"text"`
}
type View struct {
	Spec Spec `json:"spec"`
	Status Snapshot `json:"status"`
}
type managedProcess struct {
	cmd *exec.Cmd
	done chan struct{}
	snapshot Snapshot
}
type Manager struct {
	mu sync.RWMutex
	configPath string
	specs map[string]Spec
	instances map[string]*managedProcess
	outputs map[string][]OutputLine
	outputSequence map[string]int64
}

func NewManager() (*Manager, error) {
	base, err := os.UserConfigDir()
	if err != nil { return nil, err }
	return NewManagerAt(filepath.Join(base, "MeteorDesk", "processes.json"))
}

func NewManagerAt(path string) (*Manager, error) {
	m := &Manager{configPath:path, specs:map[string]Spec{}, instances:map[string]*managedProcess{}, outputs:map[string][]OutputLine{}, outputSequence:map[string]int64{}}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) { return nil, fmt.Errorf("读取进程配置失败：%w", err) }
	if err == nil {
		var specs []Spec
		if err := json.Unmarshal(data, &specs); err != nil { return nil, fmt.Errorf("解析进程配置失败：%w", err) }
		for _, spec := range specs { if spec.ID != "" { m.specs[spec.ID] = cloneSpec(spec) } }
	}
	return m, nil
}

func (m *Manager) List() []View {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.specs))
	for id := range m.specs { ids = append(ids, id) }
	sort.Strings(ids)
	items := make([]View, 0, len(ids))
	for _, id := range ids {
		spec := cloneSpec(m.specs[id])
		status := Snapshot{ID:id, Name:spec.Name, State:StateStopped}
		if instance := m.instances[id]; instance != nil { status = instance.snapshot }
		items = append(items, View{Spec:spec, Status:status})
	}
	return items
}


 // ExportConfig serializes service definitions. Sensitive environment values
 // are blanked when redaction is requested; the keys are retained for editing.
func (m *Manager) ExportConfig(redactSensitive bool) ([]byte, error) {
	m.mu.RLock()
	specs := make([]Spec, 0, len(m.specs))
	for _, spec := range m.specs { specs = append(specs, cloneSpec(spec)) }
	m.mu.RUnlock()
	sort.Slice(specs, func(i, j int) bool { return specs[i].ID < specs[j].ID })
	if redactSensitive {
		for i := range specs {
			for key := range specs[i].Env {
				if isSensitiveEnvKey(key) { specs[i].Env[key] = "" }
			}
		}
	}
	return json.MarshalIndent(specs, "", "  ")
}

// ImportConfig merges definitions as new, stopped services. IDs are regenerated
// and dependency references within the import file are rewritten.
func (m *Manager) ImportConfig(data []byte) ([]View, error) {
	if len(data) == 0 { return nil, errors.New("导入文件为空") }
	if len(data) > maxConfigImportBytes {
		return nil, fmt.Errorf("导入文件过大，最大支持 %d KiB", maxConfigImportBytes/1024)
	}
	var incoming []Spec
	if err := json.Unmarshal(data, &incoming); err != nil {
		return nil, fmt.Errorf("解析服务配置失败：%w", err)
	}
	if len(incoming) > maxServiceConfigs {
		return nil, fmt.Errorf("单次最多导入 %d 个服务配置", maxServiceConfigs)
	}
	if len(incoming) == 0 { return []View{}, nil }
	sourceIDs := make(map[string]struct{}, len(incoming))
	for i := range incoming {
		incoming[i].ID = strings.TrimSpace(incoming[i].ID)
		if incoming[i].ID == "" { return nil, fmt.Errorf("第 %d 个服务缺少配置 ID", i+1) }
		if _, exists := sourceIDs[incoming[i].ID]; exists {
			return nil, fmt.Errorf("导入文件中存在重复服务 ID：%s", incoming[i].ID)
		}
		sourceIDs[incoming[i].ID] = struct{}{}
		if err := normalizeSpec(&incoming[i]); err != nil {
			return nil, fmt.Errorf("服务“%s”配置无效：%w", incoming[i].Name, err)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.specs)+len(incoming) > maxServiceConfigs {
		return nil, fmt.Errorf("导入后服务配置总数不能超过 %d 个", maxServiceConfigs)
	}
	idMap := make(map[string]string, len(incoming))
	for _, spec := range incoming { idMap[spec.ID] = newID() }
	for i := range incoming {
		sourceID := incoming[i].ID
		incoming[i].ID = idMap[sourceID]
		for j, dependencyID := range incoming[i].DependsOn {
			if mappedID, exists := idMap[dependencyID]; exists {
				incoming[i].DependsOn[j] = mappedID
				continue
			}
			if _, exists := m.specs[dependencyID]; !exists {
				return nil, fmt.Errorf("服务“%s”依赖的服务 ID 不存在：%s", incoming[i].Name, dependencyID)
			}
		}
	}
	importedIDs := make([]string, 0, len(incoming))
	batchIDs := make(map[string]struct{}, len(incoming))
	for _, spec := range incoming {
		batchIDs[spec.ID] = struct{}{}
		importedIDs = append(importedIDs, spec.ID)
	}
	for _, spec := range incoming {
		for _, dependencyID := range spec.DependsOn {
			if dependencyID == spec.ID { return nil, fmt.Errorf("服务“%s”不能依赖自身", spec.Name) }
			if _, exists := m.specs[dependencyID]; !exists {
				if _, inBatch := batchIDs[dependencyID]; !inBatch {
					return nil, fmt.Errorf("服务“%s”依赖的服务不存在：%s", spec.Name, dependencyID)
				}
			}
		}
	}
	for _, spec := range incoming { m.specs[spec.ID] = cloneSpec(spec) }
	rollback := func() { for _, id := range importedIDs { delete(m.specs, id) } }
	if _, err := m.dependencyOrderLocked(importedIDs, true); err != nil {
		rollback()
		return nil, err
	}
	if err := m.persistLocked(); err != nil {
		rollback()
		return nil, err
	}
	views := make([]View, 0, len(incoming))
	for _, spec := range incoming {
		views = append(views, View{Spec: cloneSpec(spec), Status: Snapshot{ID:spec.ID, Name:spec.Name, State:StateStopped}})
	}
	return views, nil
}

func normalizeSpec(spec *Spec) error {
	spec.ID = strings.TrimSpace(spec.ID)
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Executable = strings.TrimSpace(spec.Executable)
	spec.WorkingDir = strings.TrimSpace(spec.WorkingDir)
	if spec.Name == "" { return errors.New("服务名称不能为空") }
	if spec.Executable == "" { return errors.New("可执行文件不能为空") }
	if strings.ContainsRune(spec.ID, 0) { return errors.New("服务 ID 包含非法字符") }
	if len(spec.Args) > 128 || len(spec.Env) > 128 || len(spec.DependsOn) > 64 {
		return errors.New("参数最多 128 个、环境变量最多 128 个、依赖最多 64 个")
	}
	if strings.ContainsRune(spec.Executable, 0) { return errors.New("可执行文件路径包含非法字符") }
	for _, arg := range spec.Args {
		if strings.ContainsRune(arg, 0) { return errors.New("命令参数包含非法字符") }
	}
	for key, value := range spec.Env {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return fmt.Errorf("环境变量名称或值无效：%q", key)
		}
	}
	spec.Args = append([]string(nil), spec.Args...)
	spec.DependsOn = uniqueIDs(spec.DependsOn)
	if spec.Env == nil { spec.Env = map[string]string{} }
	return nil
}

func isSensitiveEnvKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", ".", "").Replace(strings.ToLower(key))
	for _, fragment := range []string{"password", "passwd", "pwd", "secret", "token", "apikey", "accesskey", "privatekey", "credential", "authorization", "cookie", "signingkey", "clientsecret", "dsn", "databaseurl", "dburl", "redisurl", "mongouri", "postgresurl", "mysqlurl", "jdbcurl", "connectionstring", "connectionuri", "webhook"} {
		if strings.Contains(normalized, fragment) { return true }
	}
	return false
}

func (m *Manager) Save(spec Spec) (View, error) {
	if err := normalizeSpec(&spec); err != nil {
		return View{}, err
	}
	if spec.ID == "" {
		spec.ID = newID()
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if instance := m.instances[spec.ID]; instance != nil && isActive(instance.snapshot.State) { return View{}, errors.New("运行中的服务不能修改配置，请先停止服务") }
	for _, dependencyID := range spec.DependsOn {
		if dependencyID == spec.ID { return View{}, errors.New("服务不能依赖自身") }
		if _, exists := m.specs[dependencyID]; !exists { return View{}, fmt.Errorf("依赖服务不存在：%s", dependencyID) }
	}
	oldSpec, hadOldSpec := m.specs[spec.ID]
	m.specs[spec.ID] = cloneSpec(spec)
	if _, err := m.dependencyOrderLocked([]string{spec.ID}, true); err != nil {
		if hadOldSpec { m.specs[spec.ID] = oldSpec } else { delete(m.specs, spec.ID) }
		return View{}, err
	}
	if err := m.persistLocked(); err != nil {
		if hadOldSpec { m.specs[spec.ID] = oldSpec } else { delete(m.specs, spec.ID) }
		return View{}, err
	}
	status := Snapshot{ID:spec.ID, Name:spec.Name, State:StateStopped}
	if instance := m.instances[spec.ID]; instance != nil { status = instance.snapshot }
	return View{Spec:cloneSpec(spec), Status:status}, nil
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.specs[id]; !ok { return errors.New("未找到该服务配置") }
	if instance := m.instances[id]; instance != nil && isActive(instance.snapshot.State) { return errors.New("请先停止服务，再删除配置") }
	for otherID, spec := range m.specs {
		if otherID == id { continue }
		for _, dependencyID := range spec.DependsOn {
			if dependencyID == id {
				return fmt.Errorf("服务仍被“%s”依赖，请先移除依赖关系", spec.Name)
			}
		}
	}
	delete(m.specs,id); delete(m.instances,id); delete(m.outputs,id); delete(m.outputSequence,id)
	return m.persistLocked()
}

func (m *Manager) startOne(id string) error {
	m.mu.Lock()
	spec, ok := m.specs[id]
	if !ok { m.mu.Unlock(); return errors.New("未找到该服务配置") }
	if instance := m.instances[id]; instance != nil && isActive(instance.snapshot.State) { m.mu.Unlock(); return errors.New("该服务已经运行或正在启停") }
	// 直接传递可执行文件和参数，绝不隐式拼接或执行 Shell 字符串。
	cmd := exec.Command(spec.Executable, spec.Args...)
	configureProcess(cmd)
	if spec.WorkingDir != "" { cmd.Dir = spec.WorkingDir }
	cmd.Env = mergeEnv(os.Environ(), spec.Env)
	stdout, err := cmd.StdoutPipe()
	if err != nil { m.mu.Unlock(); return fmt.Errorf("创建标准输出管道失败：%w", err) }
	stderr, err := cmd.StderrPipe()
	if err != nil { m.mu.Unlock(); return fmt.Errorf("创建标准错误管道失败：%w", err) }
	if err := cmd.Start(); err != nil { m.mu.Unlock(); m.appendOutput(id,"stderr","启动失败："+err.Error()); return fmt.Errorf("启动服务失败：%w",err) }
	instance := &managedProcess{cmd:cmd, done:make(chan struct{}), snapshot:Snapshot{ID:id,Name:spec.Name,State:StateRunning,PID:cmd.Process.Pid,StartedAt:time.Now().Format(time.RFC3339)}}
	m.instances[id] = instance
	m.mu.Unlock()

	var readers sync.WaitGroup
	readers.Add(2)
	go func(){ defer readers.Done(); readOutput(stdout,func(line string){m.appendOutput(id,"stdout",line)}) }()
	go func(){ defer readers.Done(); readOutput(stderr,func(line string){m.appendOutput(id,"stderr",line)}) }()
	go func(){
		readers.Wait()
		waitErr := cmd.Wait()
		var exitCode *int
		if cmd.ProcessState != nil {
			code := cmd.ProcessState.ExitCode()
			if code >= 0 {
				exitCode = &code
			}
		}
		m.mu.Lock()
		if current := m.instances[id]; current == instance {
			current.snapshot.ExitCode = exitCode
			if current.snapshot.State == StateStopping || waitErr == nil { current.snapshot.State=StateStopped; current.snapshot.Error="" } else { current.snapshot.State=StateFailed; current.snapshot.Error=waitErr.Error() }
		}
		m.mu.Unlock()
		if waitErr != nil { m.appendOutput(id,"system","进程已退出："+waitErr.Error()) } else { m.appendOutput(id,"system","进程已正常退出") }
		close(instance.done)
	}()
	m.appendOutput(id,"system",fmt.Sprintf("服务已启动，进程号 %d",cmd.Process.Pid))
	return nil
}

func (m *Manager) stopOne(id string) error {
	m.mu.Lock()
	instance := m.instances[id]
	if instance == nil {
		_, configured := m.specs[id]
		m.mu.Unlock()
		if configured {
			return nil
		}
		return errors.New("未找到该服务配置")
	}
	if instance.snapshot.State == StateStopped || instance.snapshot.State == StateFailed {
		m.mu.Unlock()
		return nil
	}
	if instance.snapshot.State == StateStopping {
		done := instance.done
		m.mu.Unlock()
		<-done
		return nil
	}
	instance.snapshot.State = StateStopping
	cmd, done := instance.cmd, instance.done
	m.mu.Unlock()

	if err := requestProcessStop(cmd); err != nil {
		select { case <-done: return nil; default: }
		m.appendOutput(id, "system", "停止进程树请求失败，将尝试强制结束："+err.Error())
		if killErr := forceStopProcess(cmd); killErr != nil {
			select { case <-done: return nil; case <-time.After(500 * time.Millisecond):
				return fmt.Errorf("停止进程树失败：%w（请求停止错误：%v）", killErr, err)
			}
		}
	}
	timer := time.NewTimer(1500 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-done: return nil
	case <-timer.C:
		m.appendOutput(id, "system", "进程未在宽限期内退出，正在强制结束进程树")
		if err := forceStopProcess(cmd); err != nil {
			select { case <-done: return nil; case <-time.After(500 * time.Millisecond): return fmt.Errorf("强制结束进程树失败：%w", err) }
		}
		forceTimer := time.NewTimer(2 * time.Second)
		defer forceTimer.Stop()
		select { case <-done: return nil; case <-forceTimer.C: return errors.New("已发送强制结束请求，但进程输出管道仍未关闭") }
	}
}

func (m *Manager) Start(id string) error {
	results, err := m.StartBatch([]string{id})
	if err != nil { return err }
	for _, result := range results {
		if result.ID == id && !result.Success { return errors.New(result.Message) }
	}
	return nil
}

// StartBatch 自动包含所选服务的上游依赖，并按依赖优先顺序启动。
func (m *Manager) StartBatch(ids []string) ([]BatchResult, error) {
	m.mu.RLock()
	order, err := m.dependencyOrderLocked(ids, true)
	m.mu.RUnlock()
	if err != nil { return nil, err }

	resultsByID := make(map[string]BatchResult, len(order))
	results := make([]BatchResult, 0, len(order))
	for _, id := range order {
		m.mu.RLock()
		spec := cloneSpec(m.specs[id])
		instance := m.instances[id]
		running := instance != nil && (instance.snapshot.State == StateRunning || instance.snapshot.State == StateStarting)
		m.mu.RUnlock()

		blockedBy := ""
		for _, dep := range spec.DependsOn {
			if previous, exists := resultsByID[dep]; exists && !previous.Success {
				blockedBy = dep
				break
			}
		}
		result := BatchResult{ID:id, Name:spec.Name, Action:"start"}
		switch {
		case blockedBy != "":
			result.Message = "依赖服务启动失败，已跳过：" + blockedBy
		case running:
			result.Success = true
			result.Message = "服务已在运行"
		default:
			if startErr := m.startOne(id); startErr != nil {
				result.Message = startErr.Error()
			} else {
				result.Success = true
				result.Message = "启动成功"
			}
		}
		resultsByID[id] = result
		results = append(results, result)
	}
	return results, nil
}

func (m *Manager) Stop(id string) error {
	results, err := m.StopBatch([]string{id})
	if err != nil { return err }
	for _, result := range results {
		if !result.Success { return errors.New(result.Message) }
	}
	return nil
}

// StopBatch 先停止所选服务的下游依赖方，再停止目标服务。
func (m *Manager) StopBatch(ids []string) ([]BatchResult, error) {
	m.mu.RLock()
	targets := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := m.specs[id]; !exists {
			m.mu.RUnlock()
			return nil, fmt.Errorf("未找到服务配置：%s", id)
		}
		targets[id] = struct{}{}
	}
	if len(targets) == 0 {
		m.mu.RUnlock()
		return []BatchResult{}, nil
	}

	changed := true
	for changed {
		changed = false
		for id, spec := range m.specs {
			if _, selected := targets[id]; selected { continue }
			for _, dep := range spec.DependsOn {
				if _, dependsOnTarget := targets[dep]; dependsOnTarget {
					targets[id] = struct{}{}
					changed = true
					break
				}
			}
		}
	}
	targetIDs := make([]string, 0, len(targets))
	for id := range targets { targetIDs = append(targetIDs, id) }
	order, err := m.dependencyOrderLocked(targetIDs, true)
	m.mu.RUnlock()
	if err != nil { return nil, err }

	results := make([]BatchResult, 0, len(targets))
	for i := len(order)-1; i >= 0; i-- {
		id := order[i]
		if _, wanted := targets[id]; !wanted { continue }
		m.mu.RLock()
		spec := m.specs[id]
		instance := m.instances[id]
		active := instance != nil && isActive(instance.snapshot.State)
		m.mu.RUnlock()

		result := BatchResult{ID:id, Name:spec.Name, Action:"stop"}
		if !active {
			result.Success = true
			result.Message = "服务已停止"
		} else if stopErr := m.stopOne(id); stopErr != nil {
			result.Message = stopErr.Error()
		} else {
			result.Success = true
			result.Message = "停止成功"
		}
		results = append(results, result)
	}
	return results, nil
}

func (m *Manager) Restart(id string) error {
	if err := m.Stop(id); err != nil { return err }
	return m.Start(id)
}

// StopAll 按依赖的反向顺序停止所有服务。
func (m *Manager) StopAll() error {
	m.mu.RLock()
	ids := make([]string, 0, len(m.specs))
	for id := range m.specs { ids = append(ids, id) }
	m.mu.RUnlock()
	results, err := m.StopBatch(ids)
	if err != nil { return err }
	var failures []string
	for _, result := range results {
		if !result.Success { failures = append(failures, result.Name+": "+result.Message) }
	}
	if len(failures) > 0 { return errors.New("停止部分服务失败：" + strings.Join(failures, "；")) }
	return nil
}

// dependencyOrderLocked 执行拓扑排序；includeDependencies 为 true 时递归包含上游依赖。
func (m *Manager) dependencyOrderLocked(ids []string, includeDependencies bool) ([]string, error) {
	if len(ids) == 0 { return []string{}, nil }
	state := make(map[string]uint8, len(m.specs))
	order := make([]string, 0, len(m.specs))
	var visit func(string) error
	visit = func(id string) error {
		spec, exists := m.specs[id]
		if !exists { return fmt.Errorf("未找到服务配置：%s", id) }
		if state[id] == 1 { return fmt.Errorf("检测到服务依赖循环，涉及服务：%s", spec.Name) }
		if state[id] == 2 { return nil }
		state[id] = 1
		if includeDependencies {
			for _, dep := range spec.DependsOn {
				if err := visit(dep); err != nil { return err }
			}
		}
		state[id] = 2
		order = append(order, id)
		return nil
	}
	for _, id := range ids {
		if err := visit(id); err != nil { return nil, err }
	}
	return order, nil
}

func uniqueIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" { continue }
		if _, exists := seen[id]; exists { continue }
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func (m *Manager) Logs(id string, afterSequence int64) []OutputLine {
	m.mu.RLock(); defer m.mu.RUnlock()
	lines:=m.outputs[id]; start:=0
	for start<len(lines)&&lines[start].Sequence<=afterSequence {start++}
	return append([]OutputLine(nil),lines[start:]...)
}
func (m *Manager) appendOutput(id,stream,text string) {
	m.mu.Lock(); defer m.mu.Unlock()
	m.outputSequence[id]++
	line:=OutputLine{Sequence:m.outputSequence[id],Time:time.Now().Format("15:04:05.000"),Stream:stream,Text:strings.ToValidUTF8(text,"�")}
	lines:=append(m.outputs[id],line)
	if len(lines)>maxOutputLines { lines=append([]OutputLine(nil),lines[len(lines)-maxOutputLines:]...) }
	m.outputs[id]=lines
}
func readOutput(reader io.Reader,emit func(string)) {
	br:=bufio.NewReaderSize(reader,32*1024); line:=make([]byte,0,4096); clipped:=false
	for {
		part,err:=br.ReadSlice('\n'); newline:=len(part)>0&&part[len(part)-1]=='\n'
		if newline {part=part[:len(part)-1]}
		if newline&&len(part)>0&&part[len(part)-1]=='\r' {part=part[:len(part)-1]}
		room:=maxOutputLineBytes-len(line)
		if room>0 {take:=min(room,len(part));line=append(line,part[:take]...);if take<len(part){clipped=true}} else if len(part)>0 {clipped=true}
		if newline {if clipped{line=append(line,[]byte(" …[输出过长，已截断]")...)};emit(string(line));line=line[:0];clipped=false}
		if err==bufio.ErrBufferFull {continue}
		if err==io.EOF {if len(line)>0||clipped{if clipped{line=append(line,[]byte(" …[输出过长，已截断]")...)};emit(string(line))};return}
		if err!=nil {emit("读取进程输出失败："+err.Error());return}
	}
}
func (m *Manager) persistLocked() error {
	if err:=os.MkdirAll(filepath.Dir(m.configPath),0700);err!=nil{return err}
	specs:=make([]Spec,0,len(m.specs));for _,spec:=range m.specs{specs=append(specs,cloneSpec(spec))}
	sort.Slice(specs,func(i,j int)bool{return specs[i].ID<specs[j].ID})
	data,err:=json.MarshalIndent(specs,"","  ");if err!=nil{return err}
	tmp:=m.configPath+".tmp";if err:=os.WriteFile(tmp,data,0600);err!=nil{return err}
	if err:=os.Rename(tmp,m.configPath);err!=nil {
		if removeErr:=os.Remove(m.configPath);removeErr==nil {if retryErr:=os.Rename(tmp,m.configPath);retryErr==nil{return nil}}
		_ = os.Remove(tmp);return fmt.Errorf("保存服务配置失败：%w",err)
	}
	return nil
}
func mergeEnv(base []string,overrides map[string]string) []string {
	values:=make(map[string]string,len(base)+len(overrides))
	for _,item:=range base {if i:=strings.IndexByte(item,'=');i>0{values[item[:i]]=item[i+1:]}}
	for k,v:=range overrides{values[k]=v}
	keys:=make([]string,0,len(values));for key:=range values{keys=append(keys,key)};sort.Strings(keys)
	out:=make([]string,0,len(keys));for _,key:=range keys{out=append(out,key+"="+values[key])};return out
}
func cloneSpec(spec Spec) Spec {
	spec.Args=append([]string(nil),spec.Args...)
	if spec.Env!=nil{env:=make(map[string]string,len(spec.Env));for k,v:=range spec.Env{env[k]=v};spec.Env=env}
	return spec
}
func isActive(state State) bool {return state==StateStarting||state==StateRunning||state==StateStopping}
func newID() string {var data [16]byte;if _,err:=rand.Read(data[:]);err==nil{return hex.EncodeToString(data[:])};return fmt.Sprintf("service-%d",time.Now().UnixNano())}
