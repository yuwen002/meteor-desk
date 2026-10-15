package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessHelper(t *testing.T) {
	if os.Getenv("METEORDESK_PROCESS_HELPER") != "1" { return }
	fmt.Println("helper-output-ready")
	for { time.Sleep(200*time.Millisecond) }
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
	manager,err:=NewManagerAt(filepath.Join(t.TempDir(),"processes.json"));if err!=nil{t.Fatal(err)}
	view,err:=manager.Save(Spec{Name:"测试子进程",Executable:os.Args[0],Args:[]string{"-test.run=TestProcessHelper"},Env:map[string]string{"METEORDESK_PROCESS_HELPER":"1"}});if err!=nil{t.Fatal(err)}
	if err:=manager.Start(view.Spec.ID);err!=nil{t.Fatal(err)}
	deadline:=time.Now().Add(4*time.Second);found:=false
	for time.Now().Before(deadline){for _,line:=range manager.Logs(view.Spec.ID,0){if strings.Contains(line.Text,"helper-output-ready"){found=true}};if found{break};time.Sleep(25*time.Millisecond)}
	if !found{_ = manager.Stop(view.Spec.ID);t.Fatal("未采集到子进程标准输出")}
	if err:=manager.Stop(view.Spec.ID);err!=nil{t.Fatal(err)}
	if manager.List()[0].Status.State!=StateStopped{t.Fatalf("服务停止后状态异常：%+v",manager.List()[0].Status)}
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
