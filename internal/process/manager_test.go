package process

import (
	"fmt"
	"encoding/json"
	"os/exec"
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
	if os.Getenv("METEORDESK_PROCESS_TREE_PARENT") == "1" {
		marker := os.Getenv("METEORDESK_PROCESS_TREE_MARKER")
		child := exec.Command(os.Args[0], "-test.run=TestProcessTreeGrandchild")
		child.Env = append(os.Environ(), "METEORDESK_PROCESS_TREE_CHILD=1", "METEORDESK_PROCESS_TREE_MARKER="+marker)
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil { fmt.Fprintln(os.Stderr, "grandchild-start-error:", err); return }
		fmt.Printf("grandchild-pid=%d\n", child.Process.Pid)
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
	if manager.List()[0].Status.State != StateStopped { t.Fatalf("服务停止后状态异常：%+v", manager.List()[0].Status) }
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
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待进程退出超时")
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
