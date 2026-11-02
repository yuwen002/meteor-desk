package process

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// maxRunHistory 是每个服务保留的运行记录条数上限，跨重启与跨进程累积。
const maxRunHistory = 100

// runHistoryFileName 是与 processes.json 同目录的运行历史持久化文件名。
const runHistoryFileName = "run-history.json"

// appendRunRecordLocked 追加一条运行记录并保留最近 maxRunHistory 条，同时标记待落盘；调用方需持有 m.mu。
// 与事件一致，此处只做内存变更，落盘由各生命周期方法在释放 m.mu 前调用 persistRunHistoryLocked 完成。
func (m *Manager) appendRunRecordLocked(record RunRecord) {
	if record.ServiceID == "" {
		return
	}
	records := append(m.runHistory[record.ServiceID], record)
	if len(records) > maxRunHistory {
		records = append([]RunRecord(nil), records[len(records)-maxRunHistory:]...)
	}
	m.runHistory[record.ServiceID] = records
	m.runHistoryDirty = true
}

// persistRunHistoryLocked 在调用方持有 m.mu 时把变更的运行历史同步落盘；无变更则跳过。
// 写入失败时保留脏标记，交由下一次落盘或退出收尾重试。
func (m *Manager) persistRunHistoryLocked() error {
	if !m.runHistoryDirty {
		return nil
	}
	snapshot := make(map[string][]RunRecord, len(m.runHistory))
	for id, records := range m.runHistory {
		if len(records) == 0 {
			continue
		}
		snapshot[id] = append([]RunRecord(nil), records...)
	}
	if err := writeJSONAtomic(m.runHistoryPath, snapshot); err != nil {
		return err
	}
	m.runHistoryDirty = false
	return nil
}

// FlushRunHistory 同步把待落盘的运行历史写入磁盘，供应用退出收尾与测试排空调用。
func (m *Manager) FlushRunHistory() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.persistRunHistoryLocked()
}

// loadRunHistoryFromDisk 在构造期从 runHistoryPath 载入历史运行记录；文件缺失视为空而非错误。
// 此时尚未有其他并发访问，不需要持锁。超过上限的记录会被裁剪到最近 maxRunHistory 次。
func (m *Manager) loadRunHistoryFromDisk() {
	data, err := os.ReadFile(m.runHistoryPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "读取运行历史失败，将忽略：%v\n", err)
		}
		return
	}
	var stored map[string][]RunRecord
	if err := json.Unmarshal(data, &stored); err != nil {
		fmt.Fprintf(os.Stderr, "解析运行历史失败，将忽略：%v\n", err)
		return
	}
	for id, records := range stored {
		if len(records) == 0 {
			continue
		}
		if len(records) > maxRunHistory {
			records = append([]RunRecord(nil), records[len(records)-maxRunHistory:]...)
		}
		m.runHistory[id] = append([]RunRecord(nil), records...)
	}
}

// RunHistory 返回指定服务最近的运行记录，按发生顺序排列，进程退出与重启后仍可回看；返回副本。
func (m *Manager) RunHistory(id string) []RunRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]RunRecord(nil), m.runHistory[id]...)
}

// runRecordFromInstance 依据退出收尾时的实例状态构造一条运行记录。
// 调用方需持有 m.mu，并且实例的 snapshot 已写入最终退出信息。
func runRecordFromInstance(instance *managedProcess) RunRecord {
	if instance == nil {
		return RunRecord{}
	}
	snapshot := instance.snapshot
	return RunRecord{
		ServiceID:       snapshot.ID,
		ServiceName:     snapshot.Name,
		StartedAt:       snapshot.StartedAt,
		FinishedAt:      snapshot.FinishedAt,
		DurationMs:      snapshot.DurationMs,
		ExitReason:      snapshot.ExitReason,
		ExitCode:        snapshot.ExitCode,
		Restarts:        snapshot.Restarts,
		PeakCPUPercent:  instance.peakCPUPercent,
		PeakMemoryBytes: instance.peakMemoryBytes,
	}
}
