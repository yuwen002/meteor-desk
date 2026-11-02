package process

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestFlushOnCleanManagerWritesNothing 验证没有任何事件或运行记录时，
// 同步落盘是空操作，不会凭空创建 process-events.json / run-history.json。
func TestFlushOnCleanManagerWritesNothing(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewManagerAt(filepath.Join(dir, "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.FlushEvents(); err != nil {
		t.Fatalf("干净状态 flush 时间线不应报错：%v", err)
	}
	if err := manager.FlushRunHistory(); err != nil {
		t.Fatalf("干净状态 flush 运行历史不应报错：%v", err)
	}
	for _, name := range []string{eventsFileName, runHistoryFileName} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("未发生变更时不应写出 %s，stat err = %v", name, err)
		}
	}
}

// TestPersistenceSurviveConcurrentLifecycle 并发驱动多个服务的启停，验证时间线与运行历史
// 的写入被正确串行化：落盘文件在任何时刻都是合法 JSON，且各服务记录数与内存一致。
func TestPersistenceSurviveConcurrentLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "processes.json")
	manager, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		view, err := manager.Save(helperDiskSpec("并发服务", false))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, view.Spec.ID)
	}

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			// 反复启停制造并发的事件与运行历史写入。
			for round := 0; round < 3; round++ {
				if err := manager.Start(id); err != nil {
					continue // 可能因“已运行”被拒绝，忽略
				}
				time.Sleep(30 * time.Millisecond)
				_ = manager.Stop(id)
			}
		}(id)
	}
	wg.Wait()
	// 全部退出后确保收尾落盘已完成。
	if err := manager.FlushEvents(); err != nil {
		t.Fatal(err)
	}
	if err := manager.FlushRunHistory(); err != nil {
		t.Fatal(err)
	}

	// 落盘文件必须是合法 JSON（串行化写入不会产生半截文件）。
	eventsData, err := os.ReadFile(filepath.Join(dir, eventsFileName))
	if err != nil {
		t.Fatalf("读取时间线文件失败：%v", err)
	}
	var storedEvents map[string][]ProcessEvent
	if err := json.Unmarshal(eventsData, &storedEvents); err != nil {
		t.Fatalf("时间线文件不是合法 JSON：%v", err)
	}
	historyData, err := os.ReadFile(filepath.Join(dir, runHistoryFileName))
	if err != nil {
		t.Fatalf("读取运行历史文件失败：%v", err)
	}
	var storedHistory map[string][]RunRecord
	if err := json.Unmarshal(historyData, &storedHistory); err != nil {
		t.Fatalf("运行历史文件不是合法 JSON：%v", err)
	}

	// 每个服务落盘的记录数应与内存中的时间线/运行历史一致且非空。
	for _, id := range ids {
		if got, want := len(storedEvents[id]), len(manager.Timeline(id)); got != want {
			t.Fatalf("服务 %s 落盘时间线数 %d 与内存 %d 不一致", id, got, want)
		}
		if len(storedEvents[id]) == 0 {
			t.Fatalf("服务 %s 应至少有一条时间线事件落盘", id)
		}
		if got, want := len(storedHistory[id]), len(manager.RunHistory(id)); got != want {
			t.Fatalf("服务 %s 落盘运行历史数 %d 与内存 %d 不一致", id, got, want)
		}
	}
}

// TestReloadRestoresTimelineAndRunHistory 验证重启进程（新建 Manager）后时间线与运行历史均可回看，
// 且落盘目录下的历史文件被正确载入。
func TestReloadRestoresTimelineAndRunHistory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "processes.json")
	manager, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(helperDiskSpec("重载回看服务", false))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	waitTerminalStatus(t, manager, view.Spec.ID)

	reloaded, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Timeline(view.Spec.ID)) == 0 {
		t.Fatal("重载后应能回看历史时间线")
	}
	records := reloaded.RunHistory(view.Spec.ID)
	if len(records) != 1 || records[0].ExitReason != ExitReasonNormal {
		t.Fatalf("重载后运行历史不正确：%+v", records)
	}
}
