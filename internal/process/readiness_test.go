package process

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadinessTimeoutRequiresHealthCheck(t *testing.T) {
	if got := readinessTimeout(Spec{ReadyTimeoutSeconds: 5}); got != 0 {
		t.Fatalf("没有健康检查时不应等待：%s", got)
	}
	health := &HealthCheck{Type: HealthCheckTCP, Target: "127.0.0.1:8080", IntervalSeconds: 1, TimeoutSeconds: 1, FailureThreshold: 1}
	if got := readinessTimeout(Spec{Health: health}); got != 0 {
		t.Fatalf("等待秒数为 0 时不应等待：%s", got)
	}
	if got := readinessTimeout(Spec{Health: health, ReadyTimeoutSeconds: 3}); got != 3*time.Second {
		t.Fatalf("就绪等待时长错误：%s", got)
	}
}

func TestNormalizeSpecReadyTimeout(t *testing.T) {
	health := &HealthCheck{Type: HealthCheckTCP, Target: "127.0.0.1:8080"}
	spec := Spec{Name: "就绪等待", Executable: "demo", Health: health, ReadyTimeoutSeconds: 30}
	if err := normalizeSpec(&spec); err != nil {
		t.Fatal(err)
	}
	if spec.ReadyTimeoutSeconds != 30 {
		t.Fatalf("合法就绪等待被改写：%d", spec.ReadyTimeoutSeconds)
	}
	// 导入文件可能残留等待秒数但没有健康检查，此时应归零而不是拒绝。
	spec = Spec{Name: "无健康检查", Executable: "demo", ReadyTimeoutSeconds: 30}
	if err := normalizeSpec(&spec); err != nil {
		t.Fatal(err)
	}
	if spec.ReadyTimeoutSeconds != 0 {
		t.Fatalf("没有健康检查时应归零：%d", spec.ReadyTimeoutSeconds)
	}
	for _, seconds := range []int{-1, 601} {
		if err := normalizeSpec(&Spec{Name: "非法等待", Executable: "demo", Health: &HealthCheck{Type: HealthCheckTCP, Target: "127.0.0.1:8080"}, ReadyTimeoutSeconds: seconds}); err == nil {
			t.Fatalf("应拒绝越界的就绪等待：%d", seconds)
		}
	}
}

func TestStartBatchWaitsForDependencyReadiness(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	health := &HealthCheck{Type: HealthCheckTCP, Target: listener.Addr().String(), IntervalSeconds: 1, TimeoutSeconds: 1, FailureThreshold: 1}
	dependencySpec := helperSpec("基础服务", "", RestartPolicy{}, health)
	dependencySpec.ReadyTimeoutSeconds = 5
	dependency, err := manager.Save(dependencySpec)
	if err != nil {
		t.Fatal(err)
	}
	dependent, err := manager.Save(Spec{
		Name: "API 服务", Executable: os.Args[0], Args: []string{"-test.run=TestProcessHelper"},
		Env: map[string]string{"METEORDESK_PROCESS_HELPER": "1"}, DependsOn: []string{dependency.Spec.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.StopAll()

	started := time.Now()
	results, err := manager.StartBatch([]string{dependent.Spec.ID})
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	if len(results) != 2 || results[0].ID != dependency.Spec.ID || results[1].ID != dependent.Spec.ID {
		t.Fatalf("批量启动顺序不正确：%+v", results)
	}
	for _, result := range results {
		if !result.Success {
			t.Fatalf("依赖就绪后批量启动应成功：%+v", results)
		}
	}
	if !strings.Contains(results[0].Message, "已通过健康检查") {
		t.Fatalf("上游应提示已通过健康检查：%+v", results[0])
	}
	// 健康探测在启动 1 秒后进行，等待生效时批量启动不会立刻返回。
	if elapsed < 900*time.Millisecond {
		t.Fatalf("未等待上游就绪就启动了下游：耗时 %s", elapsed)
	}
	status := waitForStatus(t, manager, dependent.Spec.ID, 3*time.Second, func(snapshot Snapshot) bool {
		return snapshot.State == StateRunning
	})
	if status.PID == 0 {
		t.Fatalf("下游服务应处于运行中：%+v", status)
	}
}

func TestStartBatchBlocksDownstreamWhenDependencyUnhealthy(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// 关闭监听器，让健康检查持续得到连接失败。
	target := probe.Addr().String()
	_ = probe.Close()

	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	health := &HealthCheck{Type: HealthCheckTCP, Target: target, IntervalSeconds: 1, TimeoutSeconds: 1, FailureThreshold: 1}
	dependencySpec := helperSpec("未就绪服务", "", RestartPolicy{}, health)
	dependencySpec.ReadyTimeoutSeconds = 10
	dependency, err := manager.Save(dependencySpec)
	if err != nil {
		t.Fatal(err)
	}
	dependent, err := manager.Save(Spec{
		Name: "下游服务", Executable: os.Args[0], Args: []string{"-test.run=TestProcessHelper"},
		Env: map[string]string{"METEORDESK_PROCESS_HELPER": "1"}, DependsOn: []string{dependency.Spec.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.StopAll()

	started := time.Now()
	results, err := manager.StartBatch([]string{dependent.Spec.ID})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("健康检查未通过时应尽快返回：耗时 %s", elapsed)
	}
	if len(results) != 2 {
		t.Fatalf("批量启动结果数量不正确：%+v", results)
	}
	if results[0].Success || !strings.Contains(results[0].Message, "健康检查未通过") {
		t.Fatalf("上游未通过健康检查时应标记失败：%+v", results[0])
	}
	if results[1].Success || !strings.Contains(results[1].Message, "依赖服务启动失败，已跳过") {
		t.Fatalf("上游未就绪时应跳过下游：%+v", results[1])
	}
	if count := countLogs(manager, dependency.Spec.ID, "下游服务不会启动"); count != 1 {
		t.Fatalf("就绪失败应写入运行输出：%+v", manager.Logs(dependency.Spec.ID, 0))
	}
}
