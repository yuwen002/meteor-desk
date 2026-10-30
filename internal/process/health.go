package process

import (
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

// startHealthProbe 为一次运行启动探测协程；没有配置健康检查时什么都不做。
// 调用方需持有 m.mu，保证 probeStop 在实例对外可见前完成设置。
func startHealthProbe(m *Manager, id string, instance *managedProcess, check *HealthCheck) {
	if instance == nil || check == nil {
		return
	}
	stop := make(chan struct{})
	instance.probeStop = stop
	go runHealthProbe(m, id, instance, *check, stop)
}

// runHealthProbe 按配置间隔探测服务，只在状态变化时追加输出，避免刷屏。
func runHealthProbe(m *Manager, id string, instance *managedProcess, check HealthCheck, stop <-chan struct{}) {
	interval := time.Duration(check.IntervalSeconds) * time.Second
	timeout := time.Duration(check.TimeoutSeconds) * time.Second
	threshold := check.FailureThreshold
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if threshold <= 0 {
		threshold = 3
	}

	failures := 0
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-stop:
			return
		case <-timer.C:
		}

		message := probeHealth(check.Type, check.Target, timeout)
		transition := ""
		m.mu.Lock()
		if m.instances[id] != instance {
			m.mu.Unlock()
			return
		}
		instance.snapshot.HealthCheckedAt = time.Now().Format(time.RFC3339Nano)
		if message == "" {
			failures = 0
			if instance.snapshot.Health == HealthUnhealthy {
				transition = "健康检查已恢复"
			}
			instance.snapshot.Health = HealthHealthy
			instance.snapshot.HealthMessage = ""
		} else {
			failures++
			if failures >= threshold {
				if instance.snapshot.Health != HealthUnhealthy {
					transition = "健康检查失败：" + message
				}
				instance.snapshot.Health = HealthUnhealthy
				instance.snapshot.HealthMessage = message
			}
		}
		m.mu.Unlock()

		if transition != "" {
			m.appendOutput(id, "system", transition)
		}
		timer.Reset(interval)
	}
}

// probeHealth 执行一次探测，返回空字符串表示健康。
func probeHealth(kind, target string, timeout time.Duration) string {
	switch kind {
	case HealthCheckTCP:
		conn, err := net.DialTimeout("tcp", target, timeout)
		if err != nil {
			return "TCP 连接失败：" + err.Error()
		}
		_ = conn.Close()
		return ""
	case HealthCheckHTTP:
		client := &http.Client{Timeout: timeout}
		response, err := client.Get(target)
		if err != nil {
			return "HTTP 请求失败：" + err.Error()
		}
		defer response.Body.Close()
		// 只读取少量正文，避免健康接口返回大内容时占用内存。
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 32*1024))
		if response.StatusCode >= 400 {
			return "HTTP 状态码 " + strconv.Itoa(response.StatusCode)
		}
		return ""
	}
	return "未知的健康检查类型：" + kind
}
