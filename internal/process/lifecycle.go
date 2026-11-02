package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// 停止流程的时间预算。宽限期结束后强制结束进程树，再给收尾留出等待时间。
// processStopConcurrencyCushion 为并发停止的等待者预留外部命令与调度开销。
// 结束进程树需要在锁外调用 taskkill（Windows）或信号操作，慢机器上每次可能占用数百毫秒到数秒；
// 等待预算若不包含这部分余量，跟随 leader 收尾的协程会先超时，误报“等待并发停止操作完成超时”。
const processStopConcurrencyCushion = 4 * time.Second
const processStopGracePeriod = 1500 * time.Millisecond
const processForceStopWait = 2 * time.Second
const processStopFinalizeWait = 500 * time.Millisecond
const processConcurrentStopWait = processStopGracePeriod + processForceStopWait + processStopFinalizeWait + processStopConcurrencyCushion

// diagnoseStartError 将底层启动错误转成更可操作的本地诊断提示。
func diagnoseStartError(spec Spec, err error) string {
	base := fmt.Sprintf("启动可执行文件 %q 失败：%v", spec.Executable, err)
	if spec.WorkingDir != "" {
		info, statErr := os.Stat(spec.WorkingDir)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				return base + fmt.Sprintf("；配置的工作目录 %q 不存在，请检查路径。", spec.WorkingDir)
			}
			if errors.Is(statErr, os.ErrPermission) {
				return base + fmt.Sprintf("；无权访问工作目录 %q，请检查目录权限。", spec.WorkingDir)
			}
		} else if !info.IsDir() {
			return base + fmt.Sprintf("；配置的工作目录 %q 不是目录。", spec.WorkingDir)
		}
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, exec.ErrNotFound) {
		return base + "；请确认可执行文件存在，或命令已加入 PATH。"
	}
	if errors.Is(err, os.ErrPermission) {
		return base + "；请检查可执行权限和当前用户权限。"
	}
	return base + "；请检查可执行文件、参数、工作目录及所需环境变量。"
}

// startOne 启动服务并清零自动重启计数。
func (m *Manager) startOne(id string) error {
	return m.startOneCounted(id, 0)
}

// startOneCounted 在锁内完成状态检查和 cmd.Start，确保同一服务不会被两个调用同时启动。
// restarts 由自动重启流程传入，手动启动时为 0。
func (m *Manager) startOneCounted(id string, restarts int) error {
	m.mu.Lock()
	m.cancelRestartLocked(id)
	spec, ok := m.specs[id]
	if !ok {
		m.mu.Unlock()
		return errors.New("未找到该服务配置")
	}
	if instance := m.instances[id]; instance != nil && isActive(instance.snapshot.State) {
		m.mu.Unlock()
		return errors.New("该服务已经运行或正在启停")
	}
	// 直接传递可执行文件和参数，绝不隐式拼接或执行 Shell 字符串。
	cmd := exec.Command(spec.Executable, spec.Args...)
	configureProcess(cmd)
	if spec.WorkingDir != "" {
		cmd.Dir = spec.WorkingDir
	}
	cmd.Env = mergeEnv(os.Environ(), spec.Env)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.mu.Unlock()
		return fmt.Errorf("创建标准输出管道失败：%w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		m.mu.Unlock()
		return fmt.Errorf("创建标准错误管道失败：%w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		finishedAt := time.Now()
		reason := diagnoseStartError(spec, err)
		finished := &managedProcess{snapshot: Snapshot{
			ID: id, Name: spec.Name, State: StateFailed, ExitReason: ExitReasonStartFailed, Restarts: restarts,
			Error: reason, FinishedAt: finishedAt.Format(time.RFC3339Nano),
		}}
		m.instances[id] = finished
		m.appendEventLocked(id, ProcessEvent{At: finishedAt.Format(time.RFC3339Nano), Kind: EventExit, Detail: "启动失败：" + reason})
		m.appendRunRecordLocked(runRecordFromInstance(finished))
		_ = m.persistEventsLocked()
		_ = m.persistRunHistoryLocked()
		m.mu.Unlock()
		m.appendOutput(id, "stderr", reason)
		return fmt.Errorf("%s: %w", reason, err)
	}
	startedAt := time.Now()
	healthState := ""
	if spec.Health != nil {
		healthState = HealthChecking
	}
	instance := &managedProcess{
		cmd: cmd, done: make(chan struct{}), startedAt: startedAt, stdout: stdout, stderr: stderr,
		snapshot: Snapshot{ID: id, Name: spec.Name, State: StateRunning, PID: cmd.Process.Pid, StartedAt: startedAt.Format(time.RFC3339Nano), Restarts: restarts, Health: healthState},
	}
	m.instances[id] = instance
	// 开启落盘时创建写入器并把路径写进快照；创建不碰磁盘，首次输出时才真正打开文件。
	if spec.LogToDisk {
		sink := m.newOutputSink(spec.Name)
		instance.disk = sink
		instance.snapshot.LogFilePath = sink.Path()
	}
	startDetail := "手动启动"
	if restarts > 0 {
		startDetail = fmt.Sprintf("自动重启第 %d 次", restarts)
	}
	m.appendEventLocked(id, ProcessEvent{At: startedAt.Format(time.RFC3339Nano), Kind: EventStart, Detail: startDetail})
	startHealthProbe(m, id, instance, spec.Health)
	// 采样协程与健康探测一样在锁内启动，避免 stopSamplers 在锁内读到未初始化的 metricsStop。
	startMetricsSampler(m, id, instance, cmd.Process.Pid)
	// 启动事件在释锁前同步落盘，保证“快照可观测则时间线已落盘”。
	_ = m.persistEventsLocked()
	m.mu.Unlock()

	var readers sync.WaitGroup
	readers.Add(2)
	go func() {
		defer readers.Done()
		readOutput(stdout, func(line string) { m.appendOutput(id, "stdout", line) })
	}()
	go func() {
		defer readers.Done()
		readOutput(stderr, func(line string) { m.appendOutput(id, "stderr", line) })
	}()
	go func() {
		readers.Wait()
		m.waitForExit(id, instance, cmd)
	}()
	m.appendOutput(id, "system", fmt.Sprintf("服务已启动，进程号 %d", cmd.Process.Pid))
	// 端口扫描要调用系统命令，放在启动完成后后台执行，不阻塞 Start。
	go m.warnPortConflicts(id, append([]int(nil), spec.Ports...), cmd.Process.Pid)
	return nil
}

// waitForExit 在输出读取结束后等待进程退出，写回退出状态与时间线事件，并按策略登记自动重启。
// 退出与重启事件必须在同一把锁内与快照一起写入，保证时间线顺序与状态一致。
func (m *Manager) waitForExit(id string, instance *managedProcess, cmd *exec.Cmd) {
	waitErr := cmd.Wait()
	finishedAt := time.Now()
	durationMsValue := finishedAt.Sub(instance.startedAt).Milliseconds()
	if durationMsValue < 0 {
		durationMsValue = 0
	}
	durationMs := &durationMsValue
	exitCode, signaled := processExitStatus(cmd)
	restartAttempt := 0
	var restartDelay time.Duration
	restartExhausted := false
	m.mu.Lock()
	if current := m.instances[id]; current == instance {
		current.snapshot.ExitCode = exitCode
		current.snapshot.FinishedAt = finishedAt.Format(time.RFC3339Nano)
		current.snapshot.DurationMs = durationMs
		switch {
		case current.snapshot.State == StateStopping:
			current.snapshot.State = StateStopped
			current.snapshot.ExitReason = ExitReasonManualStop
			current.snapshot.Error = ""
		case waitErr == nil:
			current.snapshot.State = StateStopped
			current.snapshot.ExitReason = ExitReasonNormal
			current.snapshot.Error = ""
		case signaled:
			current.snapshot.State = StateFailed
			current.snapshot.ExitReason = ExitReasonSignal
			current.snapshot.Error = waitErr.Error()
		default:
			current.snapshot.State = StateFailed
			current.snapshot.ExitReason = ExitReasonNonzero
			current.snapshot.Error = waitErr.Error()
		}
		// 进程已结束：健康状态与资源占用失效，采样协程随实例一起退出。
		current.stopSamplers()
		current.snapshot.Health = ""
		current.snapshot.HealthMessage = ""
		current.snapshot.HealthCheckedAt = ""
		current.snapshot.CPUPercent = 0
		current.snapshot.MemoryBytes = 0
		current.snapshot.MetricsAt = ""
		if current.snapshot.State == StateFailed {
			if spec, ok := m.specs[id]; ok && spec.Restart.Mode == RestartModeOnFailure {
				if current.snapshot.Restarts < spec.Restart.MaxAttempts {
					current.snapshot.Restarts++
					restartAttempt = current.snapshot.Restarts
					restartDelay = restartBackoff(restartAttempt)
					current.snapshot.NextRestartAt = time.Now().Add(restartDelay).Format(time.RFC3339Nano)
				} else {
					restartExhausted = true
				}
			}
		}
		m.appendEventLocked(id, ProcessEvent{At: finishedAt.Format(time.RFC3339Nano), Kind: EventExit, Detail: exitEventDetail(current.snapshot.ExitReason, exitCode), DurationMs: durationMs})
		if restartAttempt > 0 {
			m.appendEventLocked(id, ProcessEvent{At: time.Now().Format(time.RFC3339Nano), Kind: EventRestart, Detail: fmt.Sprintf("第 %d 次自动重启，等待 %s", restartAttempt, restartDelay)})
		} else if restartExhausted {
			m.appendEventLocked(id, ProcessEvent{At: time.Now().Format(time.RFC3339Nano), Kind: EventRestart, Detail: "自动重启次数已达上限"})
		}
		// 本次运行结束：写入一条汇总运行历史（起止/退出/重启/资源峰值）。
		m.appendRunRecordLocked(runRecordFromInstance(current))
	}
	// 退出与重启事件在释锁前同步落盘；释锁后状态即可被 List 观测，此时时间线已持久化。
	_ = m.persistEventsLocked()
	_ = m.persistRunHistoryLocked()
	m.mu.Unlock()
	// 输出读取协程已全部结束，此时关闭落盘句柄不会再与写入竞争；系统消息不写磁盘。
	if instance.disk != nil {
		_ = instance.disk.Close()
	}
	if waitErr != nil {
		m.appendOutput(id, "system", "进程已退出："+waitErr.Error())
	} else {
		m.appendOutput(id, "system", "进程已正常退出")
	}
	if restartAttempt > 0 {
		m.appendOutput(id, "system", fmt.Sprintf("异常退出，将在 %s 后自动重启（第 %d 次）", restartDelay, restartAttempt))
		m.scheduleRestart(id, instance, restartAttempt, restartDelay)
	} else if restartExhausted {
		m.appendOutput(id, "system", "异常退出，自动重启次数已达上限，请检查日志后手动启动")
	}
	close(instance.done)
}

// stopOne 先请求优雅停止，超过宽限期后强制结束进程树，并保证输出管道有收尾期限。
func (m *Manager) stopOne(id string) error {
	m.mu.Lock()
	m.cancelRestartLocked(id)
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
		instance.snapshot.Restarts = 0
		instance.snapshot.NextRestartAt = ""
		instance.stopSamplers()
		m.mu.Unlock()
		return nil
	}
	if instance.snapshot.State == StateStopping {
		done := instance.done
		m.mu.Unlock()
		timer := time.NewTimer(processConcurrentStopWait)
		defer timer.Stop()
		select {
		case <-done:
			return nil
		case <-timer.C:
			// A descendant may keep inherited output handles open after the root exits.
			instance.closeOutputPipes()
			select {
			case <-done:
				return nil
			case <-time.After(processStopFinalizeWait):
				return errors.New("等待并发停止操作完成超时，已关闭进程输出管道")
			}
		}
	}
	instance.snapshot.State = StateStopping
	instance.stopSamplers()
	cmd, done := instance.cmd, instance.done
	m.mu.Unlock()

	if err := requestProcessStop(cmd); err != nil {
		select {
		case <-done:
			return nil
		default:
		}
		m.appendOutput(id, "system", "停止进程树请求失败，将尝试强制结束："+err.Error())
		if killErr := forceStopProcess(cmd); killErr != nil {
			instance.closeOutputPipes()
			select {
			case <-done:
				return nil
			case <-time.After(processStopFinalizeWait):
				return fmt.Errorf("停止进程树失败：%w（请求停止错误：%v）", killErr, err)
			}
		}
	}
	timer := time.NewTimer(processStopGracePeriod)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		m.appendOutput(id, "system", "进程未在宽限期内退出，正在强制结束进程树")
		if err := forceStopProcess(cmd); err != nil {
			instance.closeOutputPipes()
			select {
			case <-done:
				return nil
			case <-time.After(processStopFinalizeWait):
				return fmt.Errorf("强制结束进程树失败：%w", err)
			}
		}
		forceTimer := time.NewTimer(processForceStopWait)
		defer forceTimer.Stop()
		select {
		case <-done:
			return nil
		case <-forceTimer.C:
			// Avoid leaving the service permanently in "stopping" if an unrelated
			// descendant inherited an output pipe and survived the process-group kill.
			instance.closeOutputPipes()
			select {
			case <-done:
				return nil
			case <-time.After(processStopFinalizeWait):
				return errors.New("已发送强制结束请求并关闭输出管道，但进程仍未完成退出")
			}
		}
	}
}

// Start 启动单个服务，并通过批处理入口自动满足其上游依赖。
func (m *Manager) Start(id string) error {
	results, err := m.StartBatch([]string{id})
	if err != nil {
		return err
	}
	for _, result := range results {
		if result.ID == id && !result.Success {
			return errors.New(result.Message)
		}
	}
	return nil
}

// Stop 停止指定服务及依赖该服务的下游服务。
func (m *Manager) Stop(id string) error {
	results, err := m.StopBatch([]string{id})
	if err != nil {
		return err
	}
	for _, result := range results {
		if !result.Success {
			return errors.New(result.Message)
		}
	}
	return nil
}

// Restart 在 Stop 完成后再启动；停止失败时不会继续启动第二个实例。
func (m *Manager) Restart(id string) error {
	if err := m.Stop(id); err != nil {
		return err
	}
	return m.Start(id)
}

// StopAll 按逆依赖顺序停止所有已配置服务，并汇总失败项。
func (m *Manager) StopAll() error {
	m.mu.RLock()
	ids := make([]string, 0, len(m.specs))
	for id := range m.specs {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	results, err := m.StopBatch(ids)
	if err != nil {
		return err
	}
	var failures []string
	for _, result := range results {
		if !result.Success {
			failures = append(failures, result.Name+": "+result.Message)
		}
	}
	if len(failures) > 0 {
		return errors.New("停止部分服务失败：" + strings.Join(failures, "；"))
	}
	return nil
}

// isActive 判断状态是否仍占用着服务（含正在启停的中间态）。
func isActive(state State) bool {
	return state == StateStarting || state == StateRunning || state == StateStopping
}

// mergeEnv 以当前进程环境为基础叠加服务自定义变量，并按键名排序输出，
// 保证同样的配置得到稳定的环境数组，便于测试与诊断。
func mergeEnv(base []string, overrides map[string]string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	for _, item := range base {
		if i := strings.IndexByte(item, '='); i > 0 {
			values[item[:i]] = item[i+1:]
		}
	}
	for k, v := range overrides {
		values[k] = v
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}
