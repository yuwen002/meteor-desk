package process

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// readStoredEvents 读取并解析 process-events.json，供断言持久化结果。
func readStoredEvents(t *testing.T, path string) map[string][]ProcessEvent {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取时间线文件失败：%v", err)
	}
	var stored map[string][]ProcessEvent
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("解析时间线文件失败：%v", err)
	}
	return stored
}

// waitForPersistedEvents 轮询直到落盘文件包含指定服务且至少一条事件，规避异步落盘竞态。
func waitForPersistedEvents(t *testing.T, path, id string) []ProcessEvent {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			stored := readStoredEvents(t, path)
			if events := stored[id]; len(events) > 0 {
				return events
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("等待时间线落盘超时：path=%s id=%s", path, id)
	return nil
}

// TestEventsPersistedAndReload 验证服务运行产生的时间线会落盘，并能在重载后回看。
func TestEventsPersistedAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "processes.json")
	eventsPath := filepath.Join(dir, eventsFileName)
	manager, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(helperDiskSpec("时间线服务", false))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	waitTerminalStatus(t, manager, view.Spec.ID)
	if err := manager.FlushEvents(); err != nil {
		t.Fatal(err)
	}

	events := waitForPersistedEvents(t, eventsPath, view.Spec.ID)
	var hasStart, hasExit bool
	for _, event := range events {
		switch event.Kind {
		case EventStart:
			hasStart = true
		case EventExit:
			hasExit = true
		}
	}
	if !hasStart || !hasExit {
		t.Fatalf("落盘时间线缺少启动或退出事件：%+v", events)
	}

	// 重载新管理器应从 process-events.json 恢复同一服务的时间线。
	reloaded, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	reloadedEvents := reloaded.Timeline(view.Spec.ID)
	if len(reloadedEvents) != len(events) {
		t.Fatalf("重载后事件数 %d 与落盘 %d 不一致", len(reloadedEvents), len(events))
	}
	if reloadedEvents[len(reloadedEvents)-1].Kind != events[len(events)-1].Kind {
		t.Fatalf("重载后最后一条事件类型不一致：%+v vs %+v", reloadedEvents[len(reloadedEvents)-1], events[len(events)-1])
	}
}

// TestLoadEventsTrimsToMax 验证载入时超过上限的历史被裁剪到最近 maxProcessEvents 条。
func TestLoadEventsTrimsToMax(t *testing.T) {
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, eventsFileName)
	const total = maxProcessEvents + 50
	stored := make([]ProcessEvent, 0, total)
	for i := 0; i < total; i++ {
		stored = append(stored, ProcessEvent{At: time.Unix(int64(i), 0).Format(time.RFC3339Nano), Kind: EventStart, Detail: "seed"})
	}
	data, err := json.Marshal(map[string][]ProcessEvent{"svc-id": stored})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eventsPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManagerAt(filepath.Join(dir, "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	events := manager.Timeline("svc-id")
	if len(events) != maxProcessEvents {
		t.Fatalf("载入后事件数 = %d, 期望 %d", len(events), maxProcessEvents)
	}
	// 保留的应是最近的条目：最后一条时间戳与写入的最后一条一致。
	if events[len(events)-1].At != stored[total-1].At {
		t.Fatalf("未保留最近事件：last=%q want=%q", events[len(events)-1].At, stored[total-1].At)
	}
	if events[0].At != stored[total-maxProcessEvents].At {
		t.Fatalf("裁剪起点不正确：first=%q want=%q", events[0].At, stored[total-maxProcessEvents].At)
	}
}

// TestDeleteRemovesPersistedEvents 验证删除服务后其历史时间线也从落盘文件移除。
func TestDeleteRemovesPersistedEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "processes.json")
	eventsPath := filepath.Join(dir, eventsFileName)
	manager, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(helperDiskSpec("删除时间线服务", false))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	waitTerminalStatus(t, manager, view.Spec.ID)
	if err := manager.FlushEvents(); err != nil {
		t.Fatal(err)
	}
	waitForPersistedEvents(t, eventsPath, view.Spec.ID)

	if err := manager.Delete(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	if err := manager.FlushEvents(); err != nil {
		t.Fatal(err)
	}
	// 异步落盘可能与同步 flush 交错，轮询直到该服务的事件从文件消失。
	deadline := time.Now().Add(3 * time.Second)
	for {
		stored := readStoredEvents(t, eventsPath)
		if len(stored[view.Spec.ID]) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("删除服务后仍残留时间线：%+v", stored[view.Spec.ID])
		}
		_ = manager.FlushEvents()
		time.Sleep(20 * time.Millisecond)
	}
}
