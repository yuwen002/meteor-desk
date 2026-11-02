package process

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// runLoopSpec 构造一个会持续运行直到被停止的服务定义（不设退出码则 helper 内部循环）。
func runLoopSpec(name string) Spec {
	return Spec{
		Name:       name,
		Executable: os.Args[0],
		Args:       []string{"-test.run=TestProcessHelper"},
		Env:        map[string]string{"METEORDESK_PROCESS_HELPER": "1"},
	}
}

// TestRunHistoryRecordedAndReloaded 验证正常退出会写入一条汇总运行历史，并可重载回看。
func TestRunHistoryRecordedAndReloaded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "processes.json")
	historyPath := filepath.Join(dir, runHistoryFileName)
	manager, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(helperDiskSpec("运行历史服务", false))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	waitTerminalStatus(t, manager, view.Spec.ID)

	records := manager.RunHistory(view.Spec.ID)
	if len(records) != 1 {
		t.Fatalf("一次运行应产生一条记录，实际 %d：%+v", len(records), records)
	}
	record := records[0]
	if record.ServiceID != view.Spec.ID || record.ServiceName != "运行历史服务" {
		t.Fatalf("记录标识不正确：%+v", record)
	}
	if record.ExitReason != ExitReasonNormal {
		t.Fatalf("退出原因 = %q, 期望 %q", record.ExitReason, ExitReasonNormal)
	}
	if record.StartedAt == "" || record.FinishedAt == "" {
		t.Fatalf("记录应包含起止时间：%+v", record)
	}
	if record.DurationMs == nil || *record.DurationMs < 0 {
		t.Fatalf("记录应包含非负运行时长：%+v", record.DurationMs)
	}
	if _, err := os.Stat(historyPath); err != nil {
		t.Fatalf("运行历史文件应已落盘：%v", err)
	}

	reloaded, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	reloadedRecords := reloaded.RunHistory(view.Spec.ID)
	if len(reloadedRecords) != 1 || reloadedRecords[0].ExitReason != ExitReasonNormal {
		t.Fatalf("重载后运行历史不一致：%+v", reloadedRecords)
	}
}

// TestRunHistoryRecordsNonzeroExit 验证非零退出记录退出码与原因。
func TestRunHistoryRecordsNonzeroExit(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(Spec{
		Name:       "非零退出历史",
		Executable: os.Args[0],
		Args:       []string{"-test.run=TestProcessHelper"},
		Env: map[string]string{
			"METEORDESK_PROCESS_HELPER":           "1",
			"METEORDESK_PROCESS_HELPER_EXIT_CODE": "9",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	waitTerminalStatus(t, manager, view.Spec.ID)

	records := manager.RunHistory(view.Spec.ID)
	if len(records) != 1 {
		t.Fatalf("应写入一条运行历史：%+v", records)
	}
	record := records[0]
	if record.ExitReason != ExitReasonNonzero {
		t.Fatalf("退出原因 = %q, 期望 %q", record.ExitReason, ExitReasonNonzero)
	}
	if record.ExitCode == nil || *record.ExitCode != 9 {
		t.Fatalf("退出码 = %v, 期望 9", record.ExitCode)
	}
}

// TestRunHistoryRecordsStartFailure 验证启动失败也记录一条运行历史（退出原因 start-error、无退出码）。
func TestRunHistoryRecordsStartFailure(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(Spec{Name: "启动失败历史", Executable: filepath.Join(t.TempDir(), "missing-exe")})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err == nil {
		t.Fatal("可执行文件不存在时应返回启动错误")
	}
	records := manager.RunHistory(view.Spec.ID)
	if len(records) != 1 || records[0].ExitReason != ExitReasonStartFailed {
		t.Fatalf("启动失败应记录一条 start-error 运行历史：%+v", records)
	}
	if records[0].ExitCode != nil {
		t.Fatalf("启动失败不应有退出码：%+v", records[0].ExitCode)
	}
}

// TestRunHistoryAccumulatesAcrossRuns 验证多次运行累积多条记录，且删除服务后一并清理。
func TestRunHistoryAccumulatesAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "processes.json")
	manager, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(helperDiskSpec("多次运行", false))
	if err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 3; run++ {
		if err := manager.Start(view.Spec.ID); err != nil {
			t.Fatal(err)
		}
		waitTerminalStatus(t, manager, view.Spec.ID)
	}
	if records := manager.RunHistory(view.Spec.ID); len(records) != 3 {
		t.Fatalf("三次运行应累积三条记录：%+v", records)
	}
	if err := manager.Delete(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	if records := manager.RunHistory(view.Spec.ID); len(records) != 0 {
		t.Fatalf("删除服务后运行历史应清空：%+v", records)
	}
	reloaded, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if records := reloaded.RunHistory(view.Spec.ID); len(records) != 0 {
		t.Fatalf("删除后重载运行历史仍残留：%+v", records)
	}
}

// TestRunHistoryWindowIsBounded 验证运行历史窗口只保留最近 maxRunHistory 条。
func TestRunHistoryWindowIsBounded(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	const total = maxRunHistory + 7
	manager.mu.Lock()
	for i := 0; i < total; i++ {
		manager.appendRunRecordLocked(RunRecord{ServiceID: "svc", ServiceName: strconv.Itoa(i)})
	}
	manager.mu.Unlock()
	records := manager.RunHistory("svc")
	if len(records) != maxRunHistory {
		t.Fatalf("运行历史窗口应限制在 %d 条，实际 %d", maxRunHistory, len(records))
	}
	if last := records[len(records)-1].ServiceName; last != strconv.Itoa(total-1) {
		t.Fatalf("窗口应保留最新记录，实际最后一条：%q", last)
	}
	if first := records[0].ServiceName; first != strconv.Itoa(total-maxRunHistory) {
		t.Fatalf("窗口应丢弃最早记录，实际第一条：%q", first)
	}
}

// TestLoadRunHistoryTrimsToMax 验证载入时超过上限的历史被裁剪到最近 maxRunHistory 次。
func TestLoadRunHistoryTrimsToMax(t *testing.T) {
	dir := t.TempDir()
	historyPath := filepath.Join(dir, runHistoryFileName)
	const total = maxRunHistory + 50
	stored := make([]RunRecord, 0, total)
	for i := 0; i < total; i++ {
		stored = append(stored, RunRecord{ServiceID: "svc-id", ServiceName: strconv.Itoa(i)})
	}
	data, err := json.Marshal(map[string][]RunRecord{"svc-id": stored})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(historyPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManagerAt(filepath.Join(dir, "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	records := manager.RunHistory("svc-id")
	if len(records) != maxRunHistory {
		t.Fatalf("载入后记录数 = %d, 期望 %d", len(records), maxRunHistory)
	}
	if records[0].ServiceName != strconv.Itoa(total-maxRunHistory) || records[maxRunHistory-1].ServiceName != strconv.Itoa(total-1) {
		t.Fatalf("未保留最近的运行记录：first=%q last=%q", records[0].ServiceName, records[maxRunHistory-1].ServiceName)
	}
}

// TestRunHistoryRecordsPeakMemory 验证持续运行的服务在停止后记录了内存峰值。
func TestRunHistoryRecordsPeakMemory(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(runLoopSpec("峰值采集服务"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	// 至少跨越一个采样周期，确保有一次资源采样写入峰值。
	time.Sleep(metricsSampleInterval + 800*time.Millisecond)
	if err := manager.Stop(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	waitTerminalStatus(t, manager, view.Spec.ID)
	records := manager.RunHistory(view.Spec.ID)
	if len(records) != 1 {
		t.Fatalf("应写入一条运行历史：%+v", records)
	}
	if records[0].PeakMemoryBytes == 0 {
		t.Fatalf("持续运行后应记录非零内存峰值：%+v", records[0])
	}
}
