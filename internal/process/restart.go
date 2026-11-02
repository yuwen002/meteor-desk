package process

import "time"

// restartBackoffBase 是首次自动重启前的等待时间；测试中会调小。
var restartBackoffBase = time.Second

// maxRestartBackoff 是指数退避的单次等待上限。
const maxRestartBackoff = 30 * time.Second

// cancelRestartLocked 取消待执行的重启定时器；调用方需持有 m.mu。
func (m *Manager) cancelRestartLocked(id string) {
	if timer := m.restartTimers[id]; timer != nil {
		timer.Stop()
		delete(m.restartTimers, id)
	}
}

// cancelRestart 取消待执行的重启并清零重启计数，用于手动停止已失败的服务。
func (m *Manager) cancelRestart(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelRestartLocked(id)
	if instance := m.instances[id]; instance != nil {
		instance.snapshot.Restarts = 0
		instance.snapshot.NextRestartAt = ""
	}
}

// scheduleRestart 在退避时间后尝试自动重启；只对当前实例有效。
func (m *Manager) scheduleRestart(id string, previous *managedProcess, attempt int, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.instances[id] != previous {
		return
	}
	m.cancelRestartLocked(id)
	m.restartTimers[id] = time.AfterFunc(delay, func() { m.autoRestart(id, previous, attempt) })
}

// autoRestart 在定时器到期后重新启动服务；已被停止、删除或替换时放弃。
func (m *Manager) autoRestart(id string, previous *managedProcess, attempt int) {
	m.mu.Lock()
	_, pending := m.restartTimers[id]
	if !pending || m.instances[id] != previous {
		m.mu.Unlock()
		return
	}
	delete(m.restartTimers, id)
	m.mu.Unlock()
	m.appendOutput(id, "system", "正在自动重启服务")
	if err := m.startOneCounted(id, attempt); err != nil {
		m.appendOutput(id, "stderr", "自动重启失败："+err.Error())
	}
}

// restartBackoff 返回指数退避时间：1s、2s、4s……最长 30s。
func restartBackoff(attempt int) time.Duration {
	delay := restartBackoffBase
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= maxRestartBackoff {
			return maxRestartBackoff
		}
	}
	if delay > maxRestartBackoff {
		return maxRestartBackoff
	}
	return delay
}
