package process

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func helperSpec(name string, exitCode string, restart RestartPolicy, health *HealthCheck) Spec {
	env := map[string]string{"METEORDESK_PROCESS_HELPER": "1"}
	if exitCode != "" {
		env["METEORDESK_PROCESS_HELPER_EXIT_CODE"] = exitCode
	}
	return Spec{
		Name:       name,
		Executable: os.Args[0],
		Args:       []string{"-test.run=TestProcessHelper"},
		Env:        env,
		Restart:    restart,
		Health:     health,
	}
}

func countLogs(manager *Manager, id, fragment string) int {
	count := 0
	for _, line := range manager.Logs(id, 0) {
		if strings.Contains(line.Text, fragment) {
			count++
		}
	}
	return count
}

func waitForStatus(t *testing.T, manager *Manager, id string, timeout time.Duration, done func(Snapshot) bool) Snapshot {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, view := range manager.List() {
			if view.Spec.ID == id && done(view.Status) {
				return view.Status
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("等待进程状态超时：%+v", manager.List())
	return Snapshot{}
}

func TestManagerAutoRestartsFailedProcessUntilLimit(t *testing.T) {
	previousBase := restartBackoffBase
	restartBackoffBase = 20 * time.Millisecond
	defer func() { restartBackoffBase = previousBase }()

	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(helperSpec("自动重启测试", "9", RestartPolicy{Mode: RestartModeOnFailure, MaxAttempts: 2}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}

	status := waitForStatus(t, manager, view.Spec.ID, 10*time.Second, func(snapshot Snapshot) bool {
		return snapshot.State == StateFailed && snapshot.Restarts == 2
	})
	if status.ExitCode == nil || *status.ExitCode != 9 {
		t.Fatalf("自动重启后应保留退出码 9：%+v", status)
	}
	// 首次启动加两次自动重启，共三次启动记录。
	// 慢机器或并行跑测试时单次拉起 helper 进程可能耗时数秒，等待窗口需留出余量。
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if starts := countLogs(manager, view.Spec.ID, "服务已启动"); starts == 3 && countLogs(manager, view.Spec.ID, "自动重启次数已达上限") == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("自动重启次数或上限提示不正确：启动 %d 次，日志 %+v", countLogs(manager, view.Spec.ID, "服务已启动"), manager.Logs(view.Spec.ID, 0))
}

func TestManagerStopsPendingAutoRestart(t *testing.T) {
	previousBase := restartBackoffBase
	restartBackoffBase = 300 * time.Millisecond
	defer func() { restartBackoffBase = previousBase }()

	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(helperSpec("取消重启测试", "4", RestartPolicy{Mode: RestartModeOnFailure, MaxAttempts: 5}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, manager, view.Spec.ID, 5*time.Second, func(snapshot Snapshot) bool {
		return snapshot.State == StateFailed && snapshot.NextRestartAt != "" && snapshot.Restarts == 1
	})

	startsBeforeStop := countLogs(manager, view.Spec.ID, "服务已启动")
	if err := manager.Stop(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(700 * time.Millisecond)
	status := waitForStatus(t, manager, view.Spec.ID, time.Second, func(Snapshot) bool { return true })
	if status.Restarts != 0 || status.NextRestartAt != "" {
		t.Fatalf("手动停止后应清除重启计数：%+v", status)
	}
	if starts := countLogs(manager, view.Spec.ID, "服务已启动"); starts != startsBeforeStop {
		t.Fatalf("手动停止后不应再次启动：停止前 %d 次，现在 %d 次", startsBeforeStop, starts)
	}
}

func TestManagerDoesNotRestartNormalExit(t *testing.T) {
	previousBase := restartBackoffBase
	restartBackoffBase = 20 * time.Millisecond
	defer func() { restartBackoffBase = previousBase }()

	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(helperSpec("正常退出不重启", "0", RestartPolicy{Mode: RestartModeOnFailure, MaxAttempts: 3}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, manager, view.Spec.ID, 5*time.Second, func(snapshot Snapshot) bool {
		return snapshot.State == StateStopped && snapshot.ExitReason == ExitReasonNormal
	})
	time.Sleep(200 * time.Millisecond)
	status := manager.List()[0].Status
	if status.Restarts != 0 {
		t.Fatalf("正常退出不应自动重启：%+v", status)
	}
	if count := countLogs(manager, view.Spec.ID, "自动重启"); count != 0 {
		t.Fatalf("正常退出不应输出自动重启日志：%+v", manager.Logs(view.Spec.ID, 0))
	}
}

func TestManagerHealthCheckTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	health := &HealthCheck{Type: HealthCheckTCP, Target: listener.Addr().String(), IntervalSeconds: 1, TimeoutSeconds: 1, FailureThreshold: 1}
	view, err := manager.Save(helperSpec("TCP 健康检查", "", RestartPolicy{}, health))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop(view.Spec.ID)

	waitForStatus(t, manager, view.Spec.ID, 5*time.Second, func(snapshot Snapshot) bool {
		return snapshot.Health == HealthHealthy
	})
	_ = listener.Close()
	status := waitForStatus(t, manager, view.Spec.ID, 5*time.Second, func(snapshot Snapshot) bool {
		return snapshot.Health == HealthUnhealthy
	})
	if !strings.Contains(status.HealthMessage, "TCP 连接失败") {
		t.Fatalf("健康失败信息不正确：%+v", status)
	}
}

func TestManagerHealthCheckHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	health := &HealthCheck{Type: HealthCheckHTTP, Target: server.URL + "/health", IntervalSeconds: 1, TimeoutSeconds: 1, FailureThreshold: 1}
	view, err := manager.Save(helperSpec("HTTP 健康检查", "", RestartPolicy{}, health))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop(view.Spec.ID)

	status := waitForStatus(t, manager, view.Spec.ID, 5*time.Second, func(snapshot Snapshot) bool {
		return snapshot.Health == HealthUnhealthy
	})
	if !strings.Contains(status.HealthMessage, "503") {
		t.Fatalf("HTTP 健康检查应记录状态码：%+v", status)
	}
}

func TestNormalizeSpecRestartAndHealth(t *testing.T) {
	spec := Spec{Name: "默认值", Executable: "demo"}
	if err := normalizeSpec(&spec); err != nil {
		t.Fatal(err)
	}
	if spec.Restart.Mode != RestartModeNone || spec.Restart.MaxAttempts != 0 {
		t.Fatalf("默认不应启用自动重启：%+v", spec.Restart)
	}

	spec = Spec{Name: "默认次数", Executable: "demo", Restart: RestartPolicy{Mode: " On-Failure "}}
	if err := normalizeSpec(&spec); err != nil {
		t.Fatal(err)
	}
	if spec.Restart.Mode != RestartModeOnFailure || spec.Restart.MaxAttempts != 3 {
		t.Fatalf("on-failure 默认重启次数应为 3：%+v", spec.Restart)
	}

	badRestart := []RestartPolicy{{Mode: "always"}, {Mode: RestartModeOnFailure, MaxAttempts: 21}}
	for _, policy := range badRestart {
		if err := normalizeSpec(&Spec{Name: "非法重启", Executable: "demo", Restart: policy}); err == nil {
			t.Fatalf("应拒绝自动重启配置：%+v", policy)
		}
	}

	badHealth := []HealthCheck{
		{Type: "udp", Target: "127.0.0.1:1"},
		{Type: HealthCheckTCP, Target: "127.0.0.1"},
		{Type: HealthCheckHTTP, Target: "127.0.0.1:8080"},
		{Type: HealthCheckTCP, Target: "127.0.0.1:1", IntervalSeconds: 400},
	}
	for _, check := range badHealth {
		health := check
		if err := normalizeSpec(&Spec{Name: "非法健康检查", Executable: "demo", Health: &health}); err == nil {
			t.Fatalf("应拒绝健康检查配置：%+v", check)
		}
	}

	health := &HealthCheck{Type: HealthCheckTCP, Target: "127.0.0.1:8080", TimeoutSeconds: 30}
	spec = Spec{Name: "健康检查默认值", Executable: "demo", Health: health}
	if err := normalizeSpec(&spec); err != nil {
		t.Fatal(err)
	}
	if spec.Health.IntervalSeconds != 10 || spec.Health.TimeoutSeconds != 10 || spec.Health.FailureThreshold != 3 {
		t.Fatalf("健康检查默认值错误：%+v", spec.Health)
	}

	path := filepath.Join(t.TempDir(), "processes.json")
	manager, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := manager.Save(Spec{
		Name: "持久化策略", Executable: "demo",
		Restart: RestartPolicy{Mode: RestartModeOnFailure, MaxAttempts: 7},
		Health:  &HealthCheck{Type: HealthCheckHTTP, Target: "http://127.0.0.1:8080/health"},
	})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	views := reloaded.List()
	if len(views) != 1 || views[0].Spec.ID != saved.Spec.ID {
		t.Fatalf("配置持久化失败：%+v", views)
	}
	if views[0].Spec.Restart.MaxAttempts != 7 || views[0].Spec.Health == nil || views[0].Spec.Health.Target != "http://127.0.0.1:8080/health" {
		t.Fatalf("重启或健康检查配置未持久化：%+v", views[0].Spec)
	}
}
