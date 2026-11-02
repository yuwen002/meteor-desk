// Package process 管理本地开发服务，配置保存在用户目录，输出只保留有限缓存。
//
// 文件按职责划分，行为约定集中在 Manager 的锁纪律上：
//   - manager.go：管理器状态、配置加载与持久化、快照查询、配置增删。
//   - spec.go：服务定义的校验、克隆与 ID 生成。
//   - transfer.go：配置导入导出（含敏感变量脱敏）。
//   - lifecycle.go：单个服务的启动、退出收尾与停止，以及对外启停入口。
//   - batch.go：批量启停与依赖拓扑排序。
//   - restart.go：异常退出的自动重启与指数退避。
//   - health.go / readiness.go / metrics.go / ports.go：健康探测、就绪等待、资源采样与端口诊断。
//   - output.go：输出缓存与按行读取；events.go：生命周期事件时间线。
//   - instance.go：单次运行实例的内部结构。
package process

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Manager 持有服务配置、受管理的进程实例及有界的输出缓存。
// 所有共享状态均通过 mu 保护；调用耗时操作前会尽量释放锁。
type Manager struct {
	mu             sync.RWMutex
	configPath     string
	specs          map[string]Spec
	instances      map[string]*managedProcess
	outputs        map[string][]OutputLine
	outputSequence map[string]int64
	restartTimers  map[string]*time.Timer
	// events 按服务保存最近 maxProcessEvents 条生命周期事件，跨重启保留并落盘到 eventsPath。
	events map[string][]ProcessEvent
	// eventsPath 是与 processes.json 同目录的 process-events.json 全路径，构造后不再变更。
	eventsPath string
	// eventsDirty 标记时间线自上次落盘后是否有变更，由 m.mu 保护；
	// 落盘在各生命周期方法释放 m.mu 前于锁内串行完成（见 persistEventsLocked）。
	eventsDirty bool
	// runHistory 按服务保存最近 maxRunHistory 条运行记录，跨重启保留并落盘到 runHistoryPath。
	runHistory map[string][]RunRecord
	// runHistoryPath 是与 processes.json 同目录的 run-history.json 全路径，构造后不再变更。
	runHistoryPath string
	// runHistoryDirty 标记运行历史自上次落盘后是否有变更，由 m.mu 保护。
	runHistoryDirty bool
	// groups 保存命名服务分组（场景），只按 ID 引用服务，落盘到与 processes.json 同目录的 groups.json。
	groups map[string]Group
	// groupsPath 构造后不再变更，为 groups.json 的绝对路径。
	groupsPath string
}

// NewManager 在当前用户配置目录中创建进程管理器。
func NewManager() (*Manager, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return NewManagerAt(filepath.Join(base, "MeteorDesk", "processes.json"))
}

// NewManagerAt 从指定文件加载服务定义，主要用于隔离测试或自定义本地路径。
// 配置文件不存在时返回空管理器，不会视为错误。
func NewManagerAt(path string) (*Manager, error) {
	m := &Manager{
		configPath:     path,
		specs:          map[string]Spec{},
		instances:      map[string]*managedProcess{},
		outputs:        map[string][]OutputLine{},
		outputSequence: map[string]int64{},
		restartTimers:  map[string]*time.Timer{},
		events:         map[string][]ProcessEvent{},
		eventsPath:     filepath.Join(filepath.Dir(path), eventsFileName),
		runHistory:     map[string][]RunRecord{},
		runHistoryPath: filepath.Join(filepath.Dir(path), runHistoryFileName),
		groups:         map[string]Group{},
		groupsPath:     filepath.Join(filepath.Dir(path), groupsFileName),
	}
	m.loadEventsFromDisk()
	m.loadRunHistoryFromDisk()
	m.loadGroupsFromDisk()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("读取进程配置失败：%w", err)
	}
	if err == nil {
		var specs []Spec
		if err := json.Unmarshal(data, &specs); err != nil {
			return nil, fmt.Errorf("解析进程配置失败：%w", err)
		}
		for _, spec := range specs {
			if spec.ID != "" {
				m.specs[spec.ID] = cloneSpec(spec)
			}
		}
	}
	return m, nil
}

// List 返回按服务 ID 排序的配置与状态快照；返回的配置切片不会暴露内部可变对象。
func (m *Manager) List() []View {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.specs))
	for id := range m.specs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	items := make([]View, 0, len(ids))
	for _, id := range ids {
		spec := cloneSpec(m.specs[id])
		status := Snapshot{ID: id, Name: spec.Name, State: StateStopped}
		if instance := m.instances[id]; instance != nil {
			status = instance.snapshot
		}
		items = append(items, View{Spec: spec, Status: status})
	}
	return items
}

// FindServiceByLogPath 根据日志文件绝对路径反查归属的本工作台服务，用于日志分析向进程详情的跳转。
// 匹配已落盘服务的当前落盘路径（服务停止后仍保留最后一次路径）；找不到时返回 false。
// 路径比较先做 Clean 归一，Windows 下不区分大小写。
func (m *Manager) FindServiceByLogPath(path string) (Snapshot, bool) {
	if strings.TrimSpace(path) == "" {
		return Snapshot{}, false
	}
	target := filepath.Clean(path)
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, instance := range m.instances {
		if instance == nil || instance.snapshot.LogFilePath == "" {
			continue
		}
		candidate := filepath.Clean(instance.snapshot.LogFilePath)
		matched := candidate == target
		if !matched && runtime.GOOS == "windows" {
			matched = strings.EqualFold(candidate, target)
		}
		if matched {
			return instance.snapshot, true
		}
	}
	return Snapshot{}, false
}

// Save 校验并持久化一个服务定义。运行中的服务不能直接修改配置。
func (m *Manager) Save(spec Spec) (View, error) {
	if err := normalizeSpec(&spec); err != nil {
		return View{}, err
	}
	if spec.ID == "" {
		spec.ID = newID()
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if instance := m.instances[spec.ID]; instance != nil && isActive(instance.snapshot.State) {
		return View{}, errors.New("运行中的服务不能修改配置，请先停止服务")
	}
	for _, dependencyID := range spec.DependsOn {
		if dependencyID == spec.ID {
			return View{}, errors.New("服务不能依赖自身")
		}
		if _, exists := m.specs[dependencyID]; !exists {
			return View{}, fmt.Errorf("依赖服务不存在：%s", dependencyID)
		}
	}
	oldSpec, hadOldSpec := m.specs[spec.ID]
	m.specs[spec.ID] = cloneSpec(spec)
	// 依赖成环或落盘失败都要把配置回滚到修改前，避免留下无效状态。
	if _, err := m.dependencyOrderLocked([]string{spec.ID}, true); err != nil {
		if hadOldSpec {
			m.specs[spec.ID] = oldSpec
		} else {
			delete(m.specs, spec.ID)
		}
		return View{}, err
	}
	if err := m.persistLocked(); err != nil {
		if hadOldSpec {
			m.specs[spec.ID] = oldSpec
		} else {
			delete(m.specs, spec.ID)
		}
		return View{}, err
	}
	// 配置变更后放弃待执行的重启，避免用旧策略继续拉起服务。
	m.cancelRestartLocked(spec.ID)
	if instance := m.instances[spec.ID]; instance != nil {
		instance.snapshot.Restarts = 0
		instance.snapshot.NextRestartAt = ""
		instance.stopSamplers()
	}
	status := Snapshot{ID: spec.ID, Name: spec.Name, State: StateStopped}
	if instance := m.instances[spec.ID]; instance != nil {
		status = instance.snapshot
	}
	return View{Spec: cloneSpec(spec), Status: status}, nil
}

// Delete 删除已停止且未被其他服务引用的配置及其输出缓存。
// 若服务开启了落盘且没有其它配置共用同名文件，会一并删除其输出文件。
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	spec, ok := m.specs[id]
	if !ok {
		m.mu.Unlock()
		return errors.New("未找到该服务配置")
	}
	if instance := m.instances[id]; instance != nil && isActive(instance.snapshot.State) {
		m.mu.Unlock()
		return errors.New("请先停止服务，再删除配置")
	}
	for otherID, other := range m.specs {
		if otherID == id {
			continue
		}
		for _, dependencyID := range other.DependsOn {
			if dependencyID == id {
				m.mu.Unlock()
				return fmt.Errorf("服务仍被“%s”依赖，请先移除依赖关系", other.Name)
			}
		}
	}
	// 只在落盘文件名不被其它服务共用时才清理，避免误删他人在写的日志。
	cleanupBase := ""
	if spec.LogToDisk {
		base := logBaseNameOf(spec.Name)
		shared := false
		for otherID, other := range m.specs {
			if otherID == id || !other.LogToDisk {
				continue
			}
			if logBaseNameOf(other.Name) == base {
				shared = true
				break
			}
		}
		if !shared {
			cleanupBase = base
		}
	}
	delete(m.specs, id)
	delete(m.instances, id)
	delete(m.outputs, id)
	delete(m.outputSequence, id)
	delete(m.events, id)
	delete(m.runHistory, id)
	// 删除服务后其历史时间线也不应残留，标脏并在锁内与配置一并落盘。
	m.eventsDirty = true
	m.runHistoryDirty = true
	m.cancelRestartLocked(id)
	err := m.persistLocked()
	if ferr := m.persistEventsLocked(); err == nil {
		err = ferr
	}
	if ferr := m.persistRunHistoryLocked(); err == nil {
		err = ferr
	}
	// 被删除的服务不应再残留于任何分组，顺带清理并落盘分组。
	if ferr := m.purgeGroupMembershipLocked(id); err == nil {
		err = ferr
	}
	m.mu.Unlock()
	if cleanupBase != "" {
		m.removeOutputLogFiles(cleanupBase)
	}
	return err
}

// persistLocked 把当前全部配置写入文件；先写临时文件再重命名，
// 避免中途失败留下半份配置。调用方需持有 m.mu。
func (m *Manager) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(m.configPath), 0700); err != nil {
		return err
	}
	specs := make([]Spec, 0, len(m.specs))
	for _, spec := range m.specs {
		specs = append(specs, cloneSpec(spec))
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].ID < specs[j].ID })
	data, err := json.MarshalIndent(specs, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.configPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, m.configPath); err != nil {
		// Windows 上目标文件被占用时重命名会失败，先删后重试一次。
		if removeErr := os.Remove(m.configPath); removeErr == nil {
			if retryErr := os.Rename(tmp, m.configPath); retryErr == nil {
				return nil
			}
		}
		_ = os.Remove(tmp)
		return fmt.Errorf("保存服务配置失败：%w", err)
	}
	return nil
}
