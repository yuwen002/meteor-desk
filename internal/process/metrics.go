package process

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// metricsSampleInterval 是资源采样间隔；前端按 1 秒轮询快照。
const metricsSampleInterval = 2 * time.Second

// metricsHistoryLimit 限制每个实例保留的采样点数：2 秒 × 150 = 5 分钟。
const metricsHistoryLimit = 150

// startMetricsSampler 为一次运行启动资源采样协程。
// 调用方必须持有 m.mu（与 startHealthProbe 同在实例注册处调用）：
// stopSamplers 在多处持锁读取 metricsStop，锁外写入会被 -race 判定为数据竞争。
func startMetricsSampler(m *Manager, id string, instance *managedProcess, pid int) {
	if instance == nil || pid <= 0 {
		return
	}
	stop := make(chan struct{})
	instance.metricsStop = stop
	go runMetricsSampler(m, id, instance, pid, stop)
}

// runMetricsSampler 每 2 秒采样一次累计 CPU 时间与常驻内存。
// CPUPercent 由两次采样的差值换算，首次采样只提供内存占用。
func runMetricsSampler(m *Manager, id string, instance *managedProcess, pid int, stop <-chan struct{}) {
	ticker := time.NewTicker(metricsSampleInterval)
	defer ticker.Stop()
	var lastCPU time.Duration
	var lastAt time.Time
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		cpuTime, memoryBytes, err := sampleProcess(pid)
		if err != nil {
			continue
		}
		now := time.Now()
		percent := 0.0
		if !lastAt.IsZero() && cpuTime >= lastCPU {
			if elapsed := now.Sub(lastAt); elapsed > 0 {
				percent = float64(cpuTime-lastCPU) / float64(elapsed) * 100
			}
		}
		lastCPU, lastAt = cpuTime, now

		m.mu.Lock()
		if m.instances[id] != instance {
			m.mu.Unlock()
			return
		}
		instance.snapshot.CPUPercent = math.Round(percent*10) / 10
		instance.snapshot.MemoryBytes = memoryBytes
		instance.snapshot.MetricsAt = now.Format(time.RFC3339Nano)
		instance.history = appendResourceSample(instance.history, ResourceSample{
			At: instance.snapshot.MetricsAt, CPUPercent: instance.snapshot.CPUPercent, MemoryBytes: memoryBytes,
		})
		m.mu.Unlock()
	}
}

// appendResourceSample 追加采样点并只保留最近 metricsHistoryLimit 个。
// 超出上限时复制到新切片，避免持续 append 复用过的底层数组无限增长。
func appendResourceSample(history []ResourceSample, sample ResourceSample) []ResourceSample {
	history = append(history, sample)
	if len(history) <= metricsHistoryLimit {
		return history
	}
	trimmed := make([]ResourceSample, metricsHistoryLimit)
	copy(trimmed, history[len(history)-metricsHistoryLimit:])
	return trimmed
}

// parseProcStat 从 /proc/<pid>/stat 中取出用户态与内核态 CPU ticks、常驻内存页数。
// comm 字段可能包含空格和右括号，因此从最后一个 ')' 之后开始解析。
func parseProcStat(raw string) (cpuTicks int64, rssPages int64, err error) {
	end := strings.LastIndex(raw, ")")
	if end < 0 || end+2 >= len(raw) {
		return 0, 0, errors.New("无法解析进程状态")
	}
	// fields[0] 是 state（第 3 个字段），utime/stime 是第 14/15 个字段，rss 是第 24 个字段。
	fields := strings.Fields(raw[end+2:])
	if len(fields) < 22 {
		return 0, 0, errors.New("进程状态字段不足")
	}
	utime, err := strconv.ParseInt(fields[11], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("解析 utime 失败：%w", err)
	}
	stime, err := strconv.ParseInt(fields[12], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("解析 stime 失败：%w", err)
	}
	rss, err := strconv.ParseInt(fields[21], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("解析 rss 失败：%w", err)
	}
	if rss < 0 {
		rss = 0
	}
	return utime + stime, rss, nil
}

// parsePSTime 解析 ps 输出的累计 CPU 时间，支持 [[dd-]hh:]mm:ss[.ss] 形式。
func parsePSTime(raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, errors.New("CPU 时间为空")
	}
	days := int64(0)
	if index := strings.IndexByte(value, '-'); index > 0 {
		parsed, err := strconv.ParseInt(value[:index], 10, 64)
		if err != nil {
			return 0, err
		}
		days = parsed
		value = value[index+1:]
	}
	parts := strings.Split(value, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("无法解析 CPU 时间：%q", raw)
	}
	var hours int64
	if len(parts) == 3 {
		parsed, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return 0, err
		}
		hours = parsed
	}
	minutes, err := strconv.ParseInt(parts[len(parts)-2], 10, 64)
	if err != nil {
		return 0, err
	}
	seconds, err := strconv.ParseFloat(parts[len(parts)-1], 64)
	if err != nil {
		return 0, err
	}
	total := float64(days*24*3600+hours*3600+minutes*60) + seconds
	return time.Duration(total * float64(time.Second)), nil
}
