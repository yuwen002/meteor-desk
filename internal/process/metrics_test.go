//go:build windows || linux || darwin

package process

import (
	"path/filepath"
	"testing"
	"time"
)

func TestParseProcStat(t *testing.T) {
	// comm 字段包含空格和右括号，解析必须从最后一个 ')' 之后开始。
	raw := "123 (demo (x)) S 1 123 123 0 -1 4194560 100 0 0 0 25 10 0 0 20 0 1 0 1000 12345678 500 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0\n"
	ticks, pages, err := parseProcStat(raw)
	if err != nil {
		t.Fatal(err)
	}
	if ticks != 35 {
		t.Fatalf("CPU ticks 应为 utime+stime=35，实际 %d", ticks)
	}
	if pages != 500 {
		t.Fatalf("常驻页数应为 500，实际 %d", pages)
	}
	for _, broken := range []string{"broken", "1 (x) S 1"} {
		if _, _, err := parseProcStat(broken); err == nil {
			t.Fatalf("应拒绝无法解析的进程状态：%q", broken)
		}
	}
}

func TestParsePSTime(t *testing.T) {
	cases := map[string]time.Duration{
		"00:01":       time.Second,
		"1:02":        62 * time.Second,
		"02:03:04":    2*time.Hour + 3*time.Minute + 4*time.Second,
		"2-03:04:05":  2*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second,
		"00:01.5":     1500 * time.Millisecond,
		"  0:00:02  ": 2 * time.Second,
	}
	for input, want := range cases {
		got, err := parsePSTime(input)
		if err != nil {
			t.Fatalf("解析 %q 失败：%v", input, err)
		}
		if got != want {
			t.Fatalf("解析 %q 得到 %s，期望 %s", input, got, want)
		}
	}
	for _, broken := range []string{"", "abc", "1:2:3:4"} {
		if _, err := parsePSTime(broken); err == nil {
			t.Fatalf("应拒绝无法解析的 CPU 时间：%q", broken)
		}
	}
}

func TestMetricsSamplerReportsResources(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(helperSpec("资源采样", "", RestartPolicy{}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Stop(view.Spec.ID) })

	status := waitForStatus(t, manager, view.Spec.ID, 10*time.Second, func(snapshot Snapshot) bool {
		return snapshot.MemoryBytes > 0 && snapshot.MetricsAt != ""
	})
	if status.MemoryBytes == 0 {
		t.Fatalf("应采集到常驻内存：%+v", status)
	}
	if status.MetricsAt == "" {
		t.Fatalf("应记录采样时间：%+v", status)
	}

	if err := manager.Stop(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	stopped := manager.List()[0].Status
	if stopped.MemoryBytes != 0 || stopped.CPUPercent != 0 || stopped.MetricsAt != "" {
		t.Fatalf("进程停止后应清空资源占用：%+v", stopped)
	}
}

func TestMetricsSamplerReportsCPU(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	spec := helperSpec("CPU 采样", "", RestartPolicy{}, nil)
	spec.Env["METEORDESK_PROCESS_HELPER_BUSY"] = "1"
	view, err := manager.Save(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(view.Spec.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Stop(view.Spec.ID) })

	// 第一次采样只能得到累计时间，第二次采样（约 4 秒）算出忙等期间的 CPU 占用。
	status := waitForStatus(t, manager, view.Spec.ID, 10*time.Second, func(snapshot Snapshot) bool {
		return snapshot.CPUPercent > 50
	})
	if status.CPUPercent <= 50 {
		t.Fatalf("忙等进程应产生明显的 CPU 占用：%+v", status)
	}
}
