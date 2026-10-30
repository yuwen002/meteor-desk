package process

import (
	"fmt"
	"time"
)

// readinessPollInterval 是等待依赖就绪时读取状态快照的间隔。
const readinessPollInterval = 50 * time.Millisecond

// readinessTimeout 返回本次启动需要等待依赖就绪的时长。
// 只有启用健康检查且配置了等待秒数时才等待；0 表示保持原有行为，启动后直接继续。
func readinessTimeout(spec Spec) time.Duration {
	if spec.Health == nil || spec.ReadyTimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(spec.ReadyTimeoutSeconds) * time.Second
}

// readinessIDsLocked 找出本次启动顺序中作为上游被依赖的服务。
// 只有这些服务需要在启动下游之前确认健康，避免为没有下游的服务增加等待。
// 调用方需持有 m.mu。
func (m *Manager) readinessIDsLocked(order []string) map[string]bool {
	inOrder := make(map[string]bool, len(order))
	for _, id := range order {
		inOrder[id] = true
	}
	needed := make(map[string]bool, len(order))
	for _, id := range order {
		spec, exists := m.specs[id]
		if !exists {
			continue
		}
		for _, dependencyID := range spec.DependsOn {
			if inOrder[dependencyID] {
				needed[dependencyID] = true
			}
		}
	}
	return needed
}

// waitReady 等待服务通过健康检查，返回空字符串表示已就绪。
// 超时、健康检查失败或服务提前退出都会返回可直接展示给用户的原因。
func (m *Manager) waitReady(id string, timeout time.Duration) string {
	if timeout <= 0 {
		return ""
	}
	m.mu.RLock()
	instance := m.instances[id]
	m.mu.RUnlock()
	if instance == nil {
		return "等待就绪失败：服务没有运行中的实例，下游服务不会启动"
	}

	deadline := time.Now().Add(timeout)
	for {
		m.mu.RLock()
		current := m.instances[id]
		var state State
		var health, message string
		if current != nil {
			state = current.snapshot.State
			health = current.snapshot.Health
			message = current.snapshot.HealthMessage
		}
		m.mu.RUnlock()

		// 实例被替换或进程已经结束，继续等待没有意义。
		if current != instance || (state != StateRunning && state != StateStarting) {
			return "等待就绪失败：服务已停止，下游服务不会启动"
		}
		switch health {
		case HealthHealthy:
			return ""
		case HealthUnhealthy:
			if message == "" {
				message = "健康检查未通过"
			}
			return fmt.Sprintf("等待就绪失败：健康检查未通过（%s），下游服务不会启动", message)
		}
		if !time.Now().Before(deadline) {
			return fmt.Sprintf("等待就绪超时（%s），健康检查尚未通过，下游服务不会启动", timeout)
		}
		time.Sleep(readinessPollInterval)
	}
}
