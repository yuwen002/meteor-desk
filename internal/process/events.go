package process

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// maxProcessEvents 是每个服务保留的生命周期事件条数上限，跨重启累积并同时用于内存与落盘裁剪。
const maxProcessEvents = 200

// eventsFileName 是与 processes.json 同目录的时间线持久化文件名。
const eventsFileName = "process-events.json"

// appendEventLocked 追加生命周期事件并保留最近 maxProcessEvents 条，同时标记待落盘；调用方需持有 m.mu。
// 此处只做内存变更，落盘由各生命周期方法在释放 m.mu 前调用 persistEventsLocked 完成。
func (m *Manager) appendEventLocked(id string, event ProcessEvent) {
	events := append(m.events[id], event)
	if len(events) > maxProcessEvents {
		events = append([]ProcessEvent(nil), events[len(events)-maxProcessEvents:]...)
	}
	m.events[id] = events
	m.eventsDirty = true
}

// persistEventsLocked 在调用方持有 m.mu 时把变更的时间线同步落盘；无变更则跳过。
// 事件写入频率低（每次启停只数条），在锁内串行写入既可保证“状态已可观测则已落盘”，
// 又不会产生后台协程泄漏；这与 persistLocked 在锁内写配置的既有惯例一致。
// 写入失败时保留脏标记，交由下一次落盘或退出收尾重试。
func (m *Manager) persistEventsLocked() error {
	if !m.eventsDirty {
		return nil
	}
	snapshot := make(map[string][]ProcessEvent, len(m.events))
	for id, events := range m.events {
		if len(events) == 0 {
			continue
		}
		snapshot[id] = append([]ProcessEvent(nil), events...)
	}
	if err := writeJSONAtomic(m.eventsPath, snapshot); err != nil {
		return err
	}
	m.eventsDirty = false
	return nil
}

// FlushEvents 同步把待落盘的时间线写入磁盘，供应用退出收尾与测试排空调用。
// 事件通常在启停收尾时已在锁内落盘，这里兜底捕获仅标记脏但尚未落盘的变更。
func (m *Manager) FlushEvents() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.persistEventsLocked()
}

// loadEventsFromDisk 在构造期从 eventsPath 载入历史时间线；文件缺失视为空而非错误。
// 此时尚未有其他并发访问，不需要持锁。超过上限的条目会被裁剪到最近 maxProcessEvents 条。
func (m *Manager) loadEventsFromDisk() {
	data, err := os.ReadFile(m.eventsPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "读取历史时间线失败，将忽略：%v\n", err)
		}
		return
	}
	var stored map[string][]ProcessEvent
	if err := json.Unmarshal(data, &stored); err != nil {
		fmt.Fprintf(os.Stderr, "解析历史时间线失败，将忽略：%v\n", err)
		return
	}
	for id, events := range stored {
		if len(events) == 0 {
			continue
		}
		if len(events) > maxProcessEvents {
			events = append([]ProcessEvent(nil), events[len(events)-maxProcessEvents:]...)
		}
		m.events[id] = append([]ProcessEvent(nil), events...)
	}
}

// markReadyLocked 记录本次运行的就绪事件：首次输出与首次健康通过先到为准，只记一次。
// 调用方需持有 m.mu。
func (m *Manager) markReadyLocked(id, source string) {
	instance := m.instances[id]
	// 启动失败的实例没有启动时间，写它的 stderr 不能当成就绪信号。
	if instance == nil || instance.readyDone || instance.startedAt.IsZero() {
		return
	}
	instance.readyDone = true
	now := time.Now()
	durationMs := now.Sub(instance.startedAt).Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}
	m.appendEventLocked(id, ProcessEvent{At: now.Format(time.RFC3339Nano), Kind: EventReady, Detail: source, DurationMs: &durationMs})
}

// Timeline 返回指定服务最近的生命周期事件，按发生顺序排列，进程退出后仍可回看；返回副本。
func (m *Manager) Timeline(id string) []ProcessEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]ProcessEvent(nil), m.events[id]...)
}

// exitEventDetail 把退出原因转为可读说明；启动失败的原因由调用方拼在事件里。
func exitEventDetail(reason ExitReason, code *int) string {
	switch reason {
	case ExitReasonNormal:
		return "正常退出"
	case ExitReasonManualStop:
		return "已手动停止"
	case ExitReasonSignal:
		return "被信号终止"
	case ExitReasonStartFailed:
		return "启动失败"
	}
	if code != nil && *code != 0 {
		return fmt.Sprintf("异常退出（退出码 %d）", *code)
	}
	return "异常退出"
}
