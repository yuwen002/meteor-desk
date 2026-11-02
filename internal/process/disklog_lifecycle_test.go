package process

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helperDiskSpec 构造一个会打印标准输出/错误后以退出码 0 立即结束的服务定义，
// 便于在落盘生命周期测试里快速跑完“启动→产生输出→退出收尾”的完整链路。
func helperDiskSpec(name string, logToDisk bool) Spec {
	return Spec{
		Name:       name,
		Executable: os.Args[0],
		Args:       []string{"-test.run=TestProcessHelper"},
		LogToDisk:  logToDisk,
		Env: map[string]string{
			"METEORDESK_PROCESS_HELPER":           "1",
			"METEORDESK_PROCESS_HELPER_EXIT_CODE": "0",
		},
	}
}

// waitTerminalStatus 轮询直到服务进入停止/失败等终态，返回最后一次快照；超时则失败。
func waitTerminalStatus(t *testing.T, manager *Manager, id string) Snapshot {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		for _, view := range manager.List() {
			if view.Spec.ID != id {
				continue
			}
			if view.Status.State == StateStopped || view.Status.State == StateFailed {
				return view.Status
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("等待服务 %s 退出超时", id)
	return Snapshot{}
}

// TestManagerDiskOutputLifecycleWritesAndCloses 验证开启落盘后：
// 快照暴露 logs/<服务名>.log 路径，输出确实写进文件，进程退出收尾后句柄关闭且路径仍保留。
func TestManagerDiskOutputLifecycleWritesAndCloses(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewManagerAt(filepath.Join(dir, "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(helperDiskSpec("落盘服务", true))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	status := waitTerminalStatus(t, manager, view.Spec.ID)

	wantPath := filepath.Join(dir, "logs", sanitizeLogName("落盘服务"))
	if status.LogFilePath != wantPath {
		t.Fatalf("快照落盘路径 = %q, 期望 %q", status.LogFilePath, wantPath)
	}
	// 退出收尾已关闭句柄，文件内容应完整可读。
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("读取落盘文件失败：%v", err)
	}
	content := string(data)
	if !strings.Contains(content, "helper-output-ready") || !strings.Contains(content, "[stdout]") {
		t.Fatalf("落盘文件缺少预期的标准输出记录：%q", content)
	}
	if !strings.Contains(content, "helper-error-ready") || !strings.Contains(content, "[stderr]") {
		t.Fatalf("落盘文件缺少预期的标准错误记录：%q", content)
	}
	// 系统消息不应写盘。
	if strings.Contains(content, "[system]") {
		t.Fatalf("系统消息不应写入落盘文件：%q", content)
	}
}

// TestManagerDiskOutputDisabledWritesNothing 验证未开启落盘时既无路径暴露也不产生文件。
func TestManagerDiskOutputDisabledWritesNothing(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewManagerAt(filepath.Join(dir, "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(helperDiskSpec("不落盘服务", false))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	status := waitTerminalStatus(t, manager, view.Spec.ID)
	if status.LogFilePath != "" {
		t.Fatalf("未开启落盘却暴露了路径：%q", status.LogFilePath)
	}
	logPath := filepath.Join(dir, "logs", sanitizeLogName("不落盘服务"))
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("未开启落盘不应生成文件，stat err = %v", err)
	}
}

// TestManagerDeleteCleansDiskLog 验证删除唯一服务时一并清理其落盘文件。
func TestManagerDeleteCleansDiskLog(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewManagerAt(filepath.Join(dir, "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(helperDiskSpec("待删服务", true))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	waitTerminalStatus(t, manager, view.Spec.ID)
	logPath := filepath.Join(dir, "logs", sanitizeLogName("待删服务"))
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("删除前落盘文件应存在：%v", err)
	}
	if err := manager.Delete(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("删除服务后落盘文件应被清理，stat err = %v", err)
	}
}

// TestFindServiceByLogPath 验证可根据落盘文件路径反查所属服务（跨模块跳转），无关路径返回 false。
func TestFindServiceByLogPath(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewManagerAt(filepath.Join(dir, "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(helperDiskSpec("反查服务", true))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	status := waitTerminalStatus(t, manager, view.Spec.ID)
	if status.LogFilePath == "" {
		t.Fatal("开启落盘后应暴露落盘路径")
	}

	found, ok := manager.FindServiceByLogPath(status.LogFilePath)
	if !ok || found.ID != view.Spec.ID {
		t.Fatalf("反查落盘路径应命中服务：%+v ok=%v", found, ok)
	}
	if _, ok := manager.FindServiceByLogPath(filepath.Join(dir, "other", "unrelated.log")); ok {
		t.Fatal("无关路径不应命中任何服务")
	}
	if _, ok := manager.FindServiceByLogPath("   "); ok {
		t.Fatal("空白路径不应命中任何服务")
	}
}

// TestManagerDeleteKeepsSharedDiskLog 验证同名落盘文件被其它开启落盘的服务共用时，
// 删除其中一个不应误删仍在使用的文件。
func TestManagerDeleteKeepsSharedDiskLog(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewManagerAt(filepath.Join(dir, "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.Save(helperDiskSpec("共用名称", true))
	if err != nil {
		t.Fatal(err)
	}
	// 第二个配置使用相同名称，同样开启落盘，二者会写入同一个文件。
	if _, err := manager.Save(helperDiskSpec("共用名称", true)); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(first.Spec.ID); err != nil {
		t.Fatal(err)
	}
	waitTerminalStatus(t, manager, first.Spec.ID)
	logPath := filepath.Join(dir, "logs", sanitizeLogName("共用名称"))
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("共用文件在删除前应存在：%v", err)
	}
	if err := manager.Delete(first.Spec.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("文件被其它落盘服务共用，删除单个配置不应清理它：%v", err)
	}
}
