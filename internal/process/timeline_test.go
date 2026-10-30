package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newTimelineManager(t *testing.T) *Manager {
	t.Helper()
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func waitForEvent(t *testing.T, manager *Manager, id string, kind EventKind) ProcessEvent {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, event := range manager.Timeline(id) {
			if event.Kind == kind {
				return event
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待 %s 事件超时，当前时间线：%+v", kind, manager.Timeline(id))
	return ProcessEvent{}
}

func TestManagerRecordsStartAndReadyEvents(t *testing.T) {
	manager := newTimelineManager(t)
	view, err := manager.Save(Spec{
		Name:       "时间线测试",
		Executable: os.Args[0],
		Args:       []string{"-test.run=TestProcessHelper"},
		Env:        map[string]string{"METEORDESK_PROCESS_HELPER": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Stop(view.Spec.ID) }()

	events := manager.Timeline(view.Spec.ID)
	if len(events) == 0 || events[0].Kind != EventStart || events[0].Detail != "手动启动" {
		t.Fatalf("启动后应先写入一条启动事件：%+v", events)
	}
	if _, err := time.Parse(time.RFC3339Nano, events[0].At); err != nil {
		t.Fatalf("事件时间格式无效：%q：%v", events[0].At, err)
	}

	ready := waitForEvent(t, manager, view.Spec.ID, EventReady)
	if ready.Detail != "首次输出" {
		t.Fatalf("就绪事件应说明来源：%+v", ready)
	}
	if ready.DurationMs == nil || *ready.DurationMs < 0 {
		t.Fatalf("就绪事件应记录非负启动耗时：%+v", ready.DurationMs)
	}
}

func TestManagerTimelineRecordsExitAndSurvivesStop(t *testing.T) {
	manager := newTimelineManager(t)
	view, err := manager.Save(Spec{
		Name:       "退出事件测试",
		Executable: os.Args[0],
		Args:       []string{"-test.run=TestProcessHelper"},
		Env: map[string]string{
			"METEORDESK_PROCESS_HELPER":           "1",
			"METEORDESK_PROCESS_HELPER_EXIT_CODE": "7",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	exit := waitForEvent(t, manager, view.Spec.ID, EventExit)
	if !strings.Contains(exit.Detail, "退出码 7") {
		t.Fatalf("退出事件应包含退出码：%+v", exit)
	}
	if exit.DurationMs == nil || *exit.DurationMs < 0 {
		t.Fatalf("退出事件应记录非负运行时长：%+v", exit.DurationMs)
	}
	// 再次启动后，上一轮事件仍保留在时间线中。
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = manager.Stop(view.Spec.ID) }()
	starts := 0
	for _, event := range manager.Timeline(view.Spec.ID) {
		if event.Kind == EventStart {
			starts++
		}
	}
	if starts < 2 {
		t.Fatalf("重启后时间线应保留历史启动事件：%+v", manager.Timeline(view.Spec.ID))
	}
}

func TestManagerTimelineRecordsStartFailure(t *testing.T) {
	manager := newTimelineManager(t)
	view, err := manager.Save(Spec{Name: "启动失败时间线", Executable: filepath.Join(t.TempDir(), "missing-executable")})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err == nil {
		t.Fatal("可执行文件不存在时应返回启动错误")
	}
	events := manager.Timeline(view.Spec.ID)
	if len(events) != 1 || events[0].Kind != EventExit || !strings.Contains(events[0].Detail, "启动失败") {
		t.Fatalf("启动失败应写入一条退出事件：%+v", events)
	}
	if err := manager.Delete(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	if len(manager.Timeline(view.Spec.ID)) != 0 {
		t.Fatal("删除服务后应清空时间线")
	}
}

func TestManagerEventWindowIsBounded(t *testing.T) {
	manager := newTimelineManager(t)
	const total = maxProcessEvents + 5
	manager.mu.Lock()
	for i := 0; i < total; i++ {
		manager.appendEventLocked("svc", ProcessEvent{Kind: EventStart, Detail: strconv.Itoa(i)})
	}
	manager.mu.Unlock()
	events := manager.Timeline("svc")
	if len(events) != maxProcessEvents {
		t.Fatalf("事件窗口应限制在 %d 条，实际 %d", maxProcessEvents, len(events))
	}
	if last := events[len(events)-1].Detail; last != strconv.Itoa(total-1) {
		t.Fatalf("环形窗口应保留最新事件，实际最后一条：%q", last)
	}
	if first := events[0].Detail; first != strconv.Itoa(total-maxProcessEvents) {
		t.Fatalf("环形窗口应丢弃最早事件，实际第一条：%q", first)
	}
}
