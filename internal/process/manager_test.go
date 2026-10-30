package process

import (
	"fmt"
	"encoding/json"
	"os/exec"
	"sync"
	"strconv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessHelper(t *testing.T) {
	if os.Getenv("METEORDESK_PROCESS_HELPER") != "1" { return }
	fmt.Println("helper-output-ready")
	fmt.Fprintln(os.Stderr, "helper-error-ready")
	if raw := os.Getenv("METEORDESK_PROCESS_HELPER_EXIT_CODE"); raw != "" {
		if code, err := strconv.Atoi(raw); err == nil {
			os.Exit(code)
		}
	}
	if os.Getenv("METEORDESK_PROCESS_HELPER_BUSY") == "1" {
		busyUntil := time.Now().Add(4 * time.Second)
		for time.Now().Before(busyUntil) {
		}
	}
	if os.Getenv("METEORDESK_PROCESS_TREE_PARENT") == "1" {
		marker := os.Getenv("METEORDESK_PROCESS_TREE_MARKER")
		child := exec.Command(os.Args[0], "-test.run=TestProcessTreeGrandchild")
		child.Env = append(os.Environ(), "METEORDESK_PROCESS_TREE_CHILD=1", "METEORDESK_PROCESS_TREE_MARKER="+marker)
		if os.Getenv("METEORDESK_PROCESS_TREE_PARENT_EXIT") == "1" {
			configureProcess(child)
		}
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil { fmt.Fprintln(os.Stderr, "grandchild-start-error:", err); return }
		fmt.Printf("grandchild-pid=%d\n", child.Process.Pid)
		if os.Getenv("METEORDESK_PROCESS_TREE_PARENT_EXIT") == "1" {
			return
		}
	}
	for { time.Sleep(200*time.Millisecond) }
}

func TestProcessTreeGrandchild(t *testing.T) {
	if os.Getenv("METEORDESK_PROCESS_TREE_CHILD") != "1" { return }
	marker := os.Getenv("METEORDESK_PROCESS_TREE_MARKER")
	for {
		_ = os.WriteFile(marker, []byte(time.Now().Format(time.RFC3339Nano)), 0600)
		time.Sleep(80 * time.Millisecond)
	}
}

func TestManagerPersistsSpecsAndRejectsUnsafeSpecs(t *testing.T) {
	path:=filepath.Join(t.TempDir(),"nested","processes.json")
	manager,err:=NewManagerAt(path);if err!=nil{t.Fatal(err)}
	view,err:=manager.Save(Spec{Name:"测试服务",Executable:os.Args[0],Args:[]string{"-test.run=TestProcessHelper"},Env:map[string]string{"METEORDESK_PROCESS_HELPER":"1","SAMPLE_VALUE":"本地"}})
	if err!=nil{t.Fatal(err)}
	reloaded,err:=NewManagerAt(path);if err!=nil{t.Fatal(err)}
	if len(reloaded.List())!=1||reloaded.List()[0].Spec.ID!=view.Spec.ID{t.Fatalf("服务配置未持久化：%+v",reloaded.List())}
	if _,err:=manager.Save(Spec{Name:"不安全配置",Executable:"sh",Args:[]string{"-c","echo dangerous\x00"}});err==nil{t.Fatal("应拒绝包含 NUL 字符的参数")}
}
func TestManagerStartsCollectsAndStopsProcess(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil { t.Fatal(err) }
	view, err := manager.Save(Spec{Name:"测试子进程", Executable:os.Args[0], Args:[]string{"-test.run=TestProcessHelper"}, Env:map[string]string{"METEORDESK_PROCESS_HELPER":"1"}})
	if err != nil { t.Fatal(err) }
	if err := manager.Start(view.Spec.ID); err != nil { t.Fatal(err) }

	deadline := time.Now().Add(4*time.Second)
	foundStdout, foundStderr := false, false
	for time.Now().Before(deadline) {
		for _, line := range manager.Logs(view.Spec.ID, 0) {
			if line.Stream == "stdout" && strings.Contains(line.Text, "helper-output-ready") { foundStdout = true }
			if line.Stream == "stderr" && strings.Contains(line.Text, "helper-error-ready") { foundStderr = true }
		}
		if foundStdout && foundStderr { break }
		time.Sleep(25*time.Millisecond)
	}
	if !foundStdout || !foundStderr {
		_ = manager.Stop(view.Spec.ID)
		t.Fatalf("未同时采集标准输出和标准错误：stdout=%v stderr=%v", foundStdout, foundStderr)
	}
	if err := manager.Stop(view.Spec.ID); err != nil { t.Fatal(err) }
	stopped := manager.List()[0].Status
	if stopped.State != StateStopped { t.Fatalf("服务停止后状态异常：%+v", stopped) }
	if stopped.ExitReason != ExitReasonManualStop { t.Fatalf("手动停止原因未记录：%+v", stopped) }

	// The prior process has fully finalized, so starting again should be immediate.
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatalf("停止后立即重新启动失败：%v", err)
	}
	if status := manager.List()[0].Status; status.State != StateRunning {
		t.Fatalf("重新启动后的状态不正确：%+v", status)
	}
	if err := manager.Stop(view.Spec.ID); err != nil {
		t.Fatalf("第二次停止失败：%v", err)
	}
}


func TestManagerRecordsNonzeroExitCode(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(Spec{
		Name: "非零退出码测试",
		Executable: os.Args[0],
		Args: []string{"-test.run=TestProcessHelper"},
		Env: map[string]string{
			"METEORDESK_PROCESS_HELPER": "1",
			"METEORDESK_PROCESS_HELPER_EXIT_CODE": "7",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status := manager.List()[0].Status
		if status.State == StateFailed {
			if status.ExitCode == nil {
				t.Fatal("进程失败后应记录退出码")
			}
			if *status.ExitCode != 7 {
				t.Fatalf("退出码=%d，期望 7", *status.ExitCode)
			}
			if status.ExitReason != ExitReasonNonzero {
				t.Fatalf("非零退出原因不正确：%+v", status)
			}
			if status.FinishedAt == "" {
				t.Fatal("进程结束后应记录结束时间")
			}
			if _, err := time.Parse(time.RFC3339Nano, status.FinishedAt); err != nil {
				t.Fatalf("结束时间格式无效：%q：%v", status.FinishedAt, err)
			}
			if status.DurationMs == nil || *status.DurationMs < 0 {
				t.Fatalf("进程结束后应记录非负运行时长：%+v", status.DurationMs)
			}
			if status.StartedAt == "" {
				t.Fatal("进程启动后应记录开始时间")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待进程退出超时")
}


func TestManagerRecordsNormalExit(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(Spec{
		Name: "正常退出测试",
		Executable: os.Args[0],
		Args: []string{"-test.run=TestProcessHelper"},
		Env: map[string]string{
			"METEORDESK_PROCESS_HELPER": "1",
			"METEORDESK_PROCESS_HELPER_EXIT_CODE": "0",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status := manager.List()[0].Status
		if status.State == StateStopped && status.ExitReason != "" {
			if status.ExitReason != ExitReasonNormal {
				t.Fatalf("正常退出原因不正确：%+v", status)
			}
			if status.ExitCode == nil || *status.ExitCode != 0 {
				t.Fatalf("正常退出应记录退出码 0：%+v", status)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待正常退出超时")
}

func TestManagerRecordsStartFailure(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(Spec{Name: "启动失败测试", Executable: filepath.Join(t.TempDir(), "missing-executable")})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err == nil {
		t.Fatal("可执行文件不存在时应返回启动错误")
	}
	status := manager.List()[0].Status
	if status.State != StateFailed || status.ExitReason != ExitReasonStartFailed || status.Error == "" {
		t.Fatalf("启动失败状态未记录完整：%+v", status)
	}
	if !strings.Contains(status.Error, "PATH") {
		t.Fatalf("启动失败应提供可操作的诊断提示，实际错误：%s", status.Error)
	}
}

func TestManagerStartFailureDiagnosesMissingWorkingDirectory(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	missingDir := filepath.Join(t.TempDir(), "missing-workdir")
	view, err := manager.Save(Spec{
		Name: "工作目录诊断测试",
		Executable: os.Args[0],
		Args: []string{"-test.run=TestProcessHelper"},
		WorkingDir: missingDir,
		Env: map[string]string{"METEORDESK_PROCESS_HELPER": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err == nil {
		t.Fatal("工作目录不存在时应返回启动错误")
	}
	status := manager.List()[0].Status
	if status.State != StateFailed || !strings.Contains(status.Error, "工作目录") || !strings.Contains(status.Error, "不存在") {
		t.Fatalf("应指出缺失的工作目录：%+v", status)
	}
}


func TestManagerConcurrentStartsDoNotDuplicateProcess(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(Spec{Name: "并发启动测试", Executable: os.Args[0], Args: []string{"-test.run=TestProcessHelper"}, Env: map[string]string{"METEORDESK_PROCESS_HELPER": "1"}})
	if err != nil {
		t.Fatal(err)
	}

	const callers = 8
	startResults := make(chan error, callers)
	var starters sync.WaitGroup
	starters.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer starters.Done()
			startResults <- manager.Start(view.Spec.ID)
		}()
	}
	starters.Wait()
	close(startResults)
	for err := range startResults {
		if err != nil {
			_ = manager.StopAll()
			t.Fatalf("并发启动不应产生重复启动错误：%v", err)
		}
	}

	startMessages := 0
	for _, line := range manager.Logs(view.Spec.ID, 0) {
		if line.Stream == "system" && strings.Contains(line.Text, "服务已启动") {
			startMessages++
		}
	}
	if startMessages != 1 {
		_ = manager.StopAll()
		t.Fatalf("并发启动应只创建一个进程，启动日志数=%d", startMessages)
	}
	if err := manager.Stop(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
}

func TestManagerConcurrentStopsComplete(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(Spec{Name: "并发停止测试", Executable: os.Args[0], Args: []string{"-test.run=TestProcessHelper"}, Env: map[string]string{"METEORDESK_PROCESS_HELPER": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}

	const callers = 4
	stopResults := make(chan error, callers)
	var stoppers sync.WaitGroup
	stoppers.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer stoppers.Done()
			stopResults <- manager.Stop(view.Spec.ID)
		}()
	}
	stoppers.Wait()
	close(stopResults)
	for err := range stopResults {
		if err != nil {
			t.Fatalf("并发停止应全部完成：%v", err)
		}
	}
	if status := manager.List()[0].Status; status.State != StateStopped {
		t.Fatalf("并发停止后状态不正确：%+v", status)
	}
}

// 并发停止的等待预算必须覆盖 leader 的完整收尾路径（宽限期 + 强制结束 + 收尾），
// 否则外部命令稍慢就会让等待者先超时，表现为偶发失败。
func TestConcurrentStopWaitCoversLeaderBudget(t *testing.T) {
	leaderBudget := processStopGracePeriod + processForceStopWait + processStopFinalizeWait
	if processConcurrentStopWait <= leaderBudget {
		t.Fatalf("并发停止等待预算 %s 未超过 leader 收尾预算 %s", processConcurrentStopWait, leaderBudget)
	}
	if processConcurrentStopWait-leaderBudget < time.Second {
		t.Fatalf("并发停止等待余量不足 1 秒：%s", processConcurrentStopWait-leaderBudget)
	}
}

func TestManagerStopFinalizesWhenDescendantKeepsOutputPipeOpen(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "detached-grandchild.txt")
	view, err := manager.Save(Spec{
		Name: "输出管道继承测试",
		Executable: os.Args[0],
		Args: []string{"-test.run=TestProcessHelper"},
		Env: map[string]string{
			"METEORDESK_PROCESS_HELPER": "1",
			"METEORDESK_PROCESS_TREE_PARENT": "1",
			"METEORDESK_PROCESS_TREE_PARENT_EXIT": "1",
			"METEORDESK_PROCESS_TREE_MARKER": marker,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}

	var childPID int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, line := range manager.Logs(view.Spec.ID, 0) {
			if strings.HasPrefix(line.Text, "grandchild-pid=") {
				childPID, _ = strconv.Atoi(strings.TrimPrefix(line.Text, "grandchild-pid="))
				break
			}
		}
		if childPID > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID <= 0 {
		t.Fatalf("未获取到子进程 PID：%+v", manager.Logs(view.Spec.ID, 0))
	}
	child, err := os.FindProcess(childPID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Kill() }()

	done := make(chan error, 1)
	go func() { done <- manager.Stop(view.Spec.ID) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("输出管道被后代进程持有时，停止仍应完成：%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("停止操作在后代进程继承输出管道时卡住")
	}

	status := manager.List()[0].Status
	if status.State != StateStopped || status.ExitReason != ExitReasonManualStop {
		t.Fatalf("关闭输出管道后应完成停止状态收尾：%+v", status)
	}
}

func TestReadOutputTruncatesVeryLongLine(t *testing.T) {
	input:=strings.Repeat("x",maxOutputLineBytes+500)+"\nsecond\n";var lines []string
	readOutput(strings.NewReader(input),func(line string){lines=append(lines,line)})
	if len(lines)!=2{t.Fatalf("输出行数=%d，期望2",len(lines))}
	if !strings.Contains(lines[0],"输出过长")||lines[1]!="second"{t.Fatalf("超长输出截断异常：%q",lines)}
}

 
func TestRestartConfiguredButStoppedService(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil { t.Fatal(err) }
	view, err := manager.Save(Spec{Name:"重启测试", Executable:os.Args[0], Args:[]string{"-test.run=TestProcessHelper"}, Env:map[string]string{"METEORDESK_PROCESS_HELPER":"1"}})
	if err != nil { t.Fatal(err) }
	if err := manager.Restart(view.Spec.ID); err != nil { t.Fatalf("重启已保存但尚未运行的服务应成功：%v", err) }
	if manager.List()[0].Status.State != StateRunning { t.Fatalf("重启后状态不正确：%+v", manager.List()[0].Status) }
	if err := manager.Stop(view.Spec.ID); err != nil { t.Fatal(err) }
}

func TestStopAllStopsRunningServices(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil { t.Fatal(err) }
	view, err := manager.Save(Spec{Name:"退出清理测试", Executable:os.Args[0], Args:[]string{"-test.run=TestProcessHelper"}, Env:map[string]string{"METEORDESK_PROCESS_HELPER":"1"}})
	if err != nil { t.Fatal(err) }
	if err := manager.Start(view.Spec.ID); err != nil { t.Fatal(err) }
	if err := manager.StopAll(); err != nil { t.Fatal(err) }
	if manager.List()[0].Status.State != StateStopped { t.Fatalf("StopAll 后服务仍未停止：%+v", manager.List()[0].Status) }
}


func TestManagerStartsDependenciesInOrder(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil { t.Fatal(err) }
	dependency, err := manager.Save(Spec{Name:"基础服务", Executable:os.Args[0], Args:[]string{"-test.run=TestProcessHelper"}, Env:map[string]string{"METEORDESK_PROCESS_HELPER":"1"}})
	if err != nil { t.Fatal(err) }
	dependent, err := manager.Save(Spec{Name:"API 服务", Executable:os.Args[0], Args:[]string{"-test.run=TestProcessHelper"}, Env:map[string]string{"METEORDESK_PROCESS_HELPER":"1"}, DependsOn:[]string{dependency.Spec.ID}})
	if err != nil { t.Fatal(err) }
	results, err := manager.StartBatch([]string{dependent.Spec.ID})
	if err != nil { t.Fatal(err) }
	if len(results) != 2 || results[0].ID != dependency.Spec.ID || results[1].ID != dependent.Spec.ID {
		_ = manager.StopAll()
		t.Fatalf("启动顺序未遵循依赖关系：%+v", results)
	}
	for _, result := range results {
		if !result.Success { _ = manager.StopAll(); t.Fatalf("批量启动失败：%+v", result) }
	}
	if err := manager.Stop(dependency.Spec.ID); err != nil { t.Fatal(err) }
	for _, view := range manager.List() {
		if view.Status.State != StateStopped { t.Fatalf("停止依赖时应先停止依赖方：%+v", view) }
	}
}

func TestManagerRejectsDependencyCycle(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil { t.Fatal(err) }
	first, err := manager.Save(Spec{Name:"服务 A", Executable:os.Args[0]})
	if err != nil { t.Fatal(err) }
	second, err := manager.Save(Spec{Name:"服务 B", Executable:os.Args[0], DependsOn:[]string{first.Spec.ID}})
	if err != nil { t.Fatal(err) }
	first.Spec.DependsOn = []string{second.Spec.ID}
	if _, err := manager.Save(first.Spec); err == nil || !strings.Contains(err.Error(), "循环") {
		t.Fatalf("应拒绝依赖循环，实际错误：%v", err)
	}

	// 更新失败必须回滚内存中的原配置，不能留下非法依赖。
	items := manager.List()
	for _, item := range items {
		if item.Spec.ID == first.Spec.ID {
			if len(item.Spec.DependsOn) != 0 {
				t.Fatalf("循环依赖保存失败后，原配置应保持不变：%+v", item.Spec.DependsOn)
			}
		}
	}
	// 同时检查磁盘快照，确保失败更新没有污染后续启动加载的配置。
	reloaded, err := NewManagerAt(manager.configPath)
	if err != nil { t.Fatal(err) }
	for _, item := range reloaded.List() {
		if item.Spec.ID == first.Spec.ID && len(item.Spec.DependsOn) != 0 {
			t.Fatalf("循环依赖保存失败后，磁盘配置应保持不变：%+v", item.Spec.DependsOn)
		}
	}
}

func TestManagerStopBatchIgnoresStoppedServices(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil { t.Fatal(err) }
	view, err := manager.Save(Spec{Name:"未运行服务", Executable:os.Args[0]})
	if err != nil { t.Fatal(err) }
	results, err := manager.StopBatch([]string{view.Spec.ID})
	if err != nil { t.Fatal(err) }
	if len(results) != 1 || !results[0].Success { t.Fatalf("停止已停止服务应为幂等操作：%+v", results) }
}


func TestManagerRejectsDeletingReferencedDependency(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil { t.Fatal(err) }
	base, err := manager.Save(Spec{Name:"数据库", Executable:os.Args[0]})
	if err != nil { t.Fatal(err) }
	_, err = manager.Save(Spec{Name:"API", Executable:os.Args[0], DependsOn:[]string{base.Spec.ID}})
	if err != nil { t.Fatal(err) }
	if err := manager.Delete(base.Spec.ID); err == nil || !strings.Contains(err.Error(), "仍被") {
		t.Fatalf("仍有依赖方时应拒绝删除，实际错误：%v", err)
	}
}


func TestExportConfigRedactsSensitiveEnvironmentValues(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json")); if err != nil { t.Fatal(err) }
	_, err = manager.Save(Spec{Name:"导出测试", Executable:os.Args[0], Env:map[string]string{
		"API_TOKEN":"token-secret-value", "DB_PASSWORD":"password-secret-value", "LOG_LEVEL":"debug",
	}})
	if err != nil { t.Fatal(err) }
	redacted, err := manager.ExportConfig(true); if err != nil { t.Fatal(err) }
	if strings.Contains(string(redacted), "token-secret-value") || strings.Contains(string(redacted), "password-secret-value") { t.Fatal("脱敏导出不应包含敏感环境变量原值") }
	var exported []Spec; if err := json.Unmarshal(redacted, &exported); err != nil { t.Fatal(err) }
	if len(exported)!=1 || exported[0].Env["API_TOKEN"]!="" || exported[0].Env["DB_PASSWORD"]!="" || exported[0].Env["LOG_LEVEL"]!="debug" { t.Fatalf("脱敏导出结果异常：%+v", exported) }
	full, err := manager.ExportConfig(false); if err != nil { t.Fatal(err) }
	if !strings.Contains(string(full),"token-secret-value") || !strings.Contains(string(full),"password-secret-value") { t.Fatal("完整导出应保留环境变量值") }
}

func TestImportConfigRemapsIDsAndDependencies(t *testing.T) {
	path:=filepath.Join(t.TempDir(),"processes.json")
	manager,err:=NewManagerAt(path);if err!=nil{t.Fatal(err)}
	existing,err:=manager.Save(Spec{Name:"现有服务",Executable:os.Args[0]});if err!=nil{t.Fatal(err)}
	data,err:=json.Marshal([]Spec{
		{ID:"source-db",Name:"导入数据库",Executable:os.Args[0]},
		{ID:"source-api",Name:"导入 API",Executable:os.Args[0],DependsOn:[]string{"source-db"}},
	});if err!=nil{t.Fatal(err)}
	imported,err:=manager.ImportConfig(data);if err!=nil{t.Fatal(err)}
	if len(imported)!=2{t.Fatalf("导入数量=%d，期望 2",len(imported))}
	idsByName:=map[string]string{}
	for _,view:=range imported{
		if view.Spec.ID=="source-db"||view.Spec.ID=="source-api"{t.Fatalf("导入服务应分配新 ID：%q",view.Spec.ID)}
		if view.Status.State!=StateStopped{t.Fatalf("导入服务不应自动启动：%+v",view.Status)}
		idsByName[view.Spec.Name]=view.Spec.ID
	}
	var importedAPI Spec;for _,view:=range imported{if view.Spec.Name=="导入 API"{importedAPI=view.Spec}}
	if len(importedAPI.DependsOn)!=1||importedAPI.DependsOn[0]!=idsByName["导入数据库"]{t.Fatalf("依赖未重映射：%+v",importedAPI.DependsOn)}
	list:=manager.List();if len(list)!=3{t.Fatalf("导入应合并而非覆盖：%+v",list)}
	foundExisting:=false;for _,view:=range list{if view.Spec.ID==existing.Spec.ID{foundExisting=true}}
	if !foundExisting{t.Fatal("导入不应覆盖已有服务")}
	reloaded,err:=NewManagerAt(path);if err!=nil{t.Fatal(err)}
	if len(reloaded.List())!=3{t.Fatalf("导入配置未持久化：%+v",reloaded.List())}
}

func TestImportConfigRejectsCyclesWithoutPartialChanges(t *testing.T) {
	path:=filepath.Join(t.TempDir(),"processes.json");manager,err:=NewManagerAt(path);if err!=nil{t.Fatal(err)}
	data,err:=json.Marshal([]Spec{
		{ID:"cycle-a",Name:"循环 A",Executable:os.Args[0],DependsOn:[]string{"cycle-b"}},
		{ID:"cycle-b",Name:"循环 B",Executable:os.Args[0],DependsOn:[]string{"cycle-a"}},
	});if err!=nil{t.Fatal(err)}
	if _,err:=manager.ImportConfig(data);err==nil||!strings.Contains(err.Error(),"循环"){t.Fatalf("应拒绝循环依赖导入，实际错误：%v",err)}
	if len(manager.List())!=0{t.Fatalf("失败导入不应留下部分配置：%+v",manager.List())}
	reloaded,err:=NewManagerAt(path);if err!=nil{t.Fatal(err)}
	if len(reloaded.List())!=0{t.Fatalf("失败导入不应持久化部分配置：%+v",reloaded.List())}
}

func TestImportConfigRejectsMissingDependenciesWithoutPartialChanges(t *testing.T) {
	manager,err:=NewManagerAt(filepath.Join(t.TempDir(),"processes.json"));if err!=nil{t.Fatal(err)}
	data,err:=json.Marshal([]Spec{{ID:"orphan",Name:"孤立服务",Executable:os.Args[0],DependsOn:[]string{"missing"}}});if err!=nil{t.Fatal(err)}
	if _,err:=manager.ImportConfig(data);err==nil||!strings.Contains(err.Error(),"不存在"){t.Fatalf("应拒绝缺失依赖，实际错误：%v",err)}
	if len(manager.List())!=0{t.Fatalf("失败导入不应留下部分配置：%+v",manager.List())}
}

func TestStopTerminatesProcessTree(t *testing.T) {
	manager,err:=NewManagerAt(filepath.Join(t.TempDir(),"processes.json"));if err!=nil{t.Fatal(err)}
	marker:=filepath.Join(t.TempDir(),"grandchild-heartbeat.txt")
	view,err:=manager.Save(Spec{Name:"父子进程测试",Executable:os.Args[0],Args:[]string{"-test.run=TestProcessHelper"},Env:map[string]string{
		"METEORDESK_PROCESS_HELPER":"1","METEORDESK_PROCESS_TREE_PARENT":"1","METEORDESK_PROCESS_TREE_MARKER":marker,
	}});if err!=nil{t.Fatal(err)}
	if err:=manager.Start(view.Spec.ID);err!=nil{t.Fatal(err)}
	defer func(){_ = manager.StopAll()}()
	deadline:=time.Now().Add(5*time.Second)
	for time.Now().Before(deadline){if _,err:=os.Stat(marker);err==nil{break};time.Sleep(25*time.Millisecond)}
	if _,err:=os.Stat(marker);err!=nil{t.Fatalf("孙进程未启动并创建心跳文件：%v",err)}
	if err:=manager.Stop(view.Spec.ID);err!=nil{t.Fatal(err)}
	before,err:=os.ReadFile(marker);if err!=nil{t.Fatal(err)}
	time.Sleep(350*time.Millisecond)
	after,err:=os.ReadFile(marker);if err!=nil{t.Fatal(err)}
	if string(before)!=string(after){t.Fatal("停止服务后孙进程仍在运行，进程树未完全退出")}
}
