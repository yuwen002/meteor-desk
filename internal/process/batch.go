package process

import (
	"fmt"
)

// StartBatch 自动包含所选服务的上游依赖，并按依赖优先顺序启动。
func (m *Manager) StartBatch(ids []string) ([]BatchResult, error) {
	m.mu.RLock()
	order, err := m.dependencyOrderLocked(ids, true)
	var needed map[string]bool
	if err == nil {
		needed = m.readinessIDsLocked(order)
	}
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}

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
		result := BatchResult{ID: id, Name: spec.Name, Action: "start"}
		switch {
		case blockedBy != "":
			result.Message = "依赖服务启动失败，已跳过：" + blockedBy
		case running:
			result.Success = true
			result.Message = "服务已在运行"
		default:
			if startErr := m.startOne(id); startErr != nil {
				// Another concurrent StartBatch call may have created the instance
				// after this batch checked the state. Treat that race as idempotent.
				m.mu.RLock()
				latest := m.instances[id]
				startedConcurrently := latest != nil && (latest.snapshot.State == StateRunning || latest.snapshot.State == StateStarting)
				m.mu.RUnlock()
				if startedConcurrently {
					result.Success = true
					result.Message = "服务已在运行"
				} else {
					result.Message = startErr.Error()
				}
			} else {
				result.Success = true
				result.Message = "启动成功"
			}
		}
		// 只有本次批量中确实被下游依赖的服务才需要等待就绪，避免无谓延迟。
		if result.Success && needed[id] {
			if timeout := readinessTimeout(spec); timeout > 0 {
				if message := m.waitReady(id, timeout); message != "" {
					result.Success = false
					result.Message = message
					m.appendOutput(id, "system", message)
				} else if result.Message == "启动成功" {
					result.Message = "启动成功，已通过健康检查"
				}
			}
		}
		resultsByID[id] = result
		results = append(results, result)
	}
	return results, nil
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

	// 反复扩张集合，把所有依赖目标服务的下游都纳进来。
	changed := true
	for changed {
		changed = false
		for id, spec := range m.specs {
			if _, selected := targets[id]; selected {
				continue
			}
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
	for id := range targets {
		targetIDs = append(targetIDs, id)
	}
	order, err := m.dependencyOrderLocked(targetIDs, true)
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	results := make([]BatchResult, 0, len(targets))
	// 停止顺序是启动顺序的逆序，先停依赖方再停被依赖者。
	for i := len(order) - 1; i >= 0; i-- {
		id := order[i]
		if _, wanted := targets[id]; !wanted {
			continue
		}
		m.mu.RLock()
		spec := m.specs[id]
		instance := m.instances[id]
		active := instance != nil && isActive(instance.snapshot.State)
		m.mu.RUnlock()

		result := BatchResult{ID: id, Name: spec.Name, Action: "stop"}
		if !active {
			// 已失败的实例可能还在等待自动重启，手动停止需要取消定时器。
			m.cancelRestart(id)
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

// dependencyOrderLocked 执行拓扑排序；includeDependencies 为 true 时递归包含上游依赖。
// 检出循环依赖时直接报错，不会给出顺序不确定的结果。调用方需持有读锁或写锁。
func (m *Manager) dependencyOrderLocked(ids []string, includeDependencies bool) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	state := make(map[string]uint8, len(m.specs))
	order := make([]string, 0, len(m.specs))
	var visit func(string) error
	visit = func(id string) error {
		spec, exists := m.specs[id]
		if !exists {
			return fmt.Errorf("未找到服务配置：%s", id)
		}
		// 1 表示正在递归访问，再次遇到即说明成环；2 表示已完成。
		if state[id] == 1 {
			return fmt.Errorf("检测到服务依赖循环，涉及服务：%s", spec.Name)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		if includeDependencies {
			for _, dep := range spec.DependsOn {
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		state[id] = 2
		order = append(order, id)
		return nil
	}
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return order, nil
}
