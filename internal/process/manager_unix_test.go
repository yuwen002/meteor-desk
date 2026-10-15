//go:build !windows

package process

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagerRecordsSignalTermination(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := manager.Save(Spec{
		Name: "信号终止测试",
		Executable: "/bin/sh",
		Args: []string{"-c", "kill -TERM $$"},
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
			if status.ExitReason != ExitReasonSignal {
				t.Fatalf("信号终止原因不正确：%+v", status)
			}
			if status.ExitCode != nil {
				t.Fatalf("信号终止不应伪造常规退出码：%+v", status)
			}
			if !strings.Contains(status.Error, "signal") {
				t.Fatalf("应保留信号终止错误详情：%+v", status)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待信号终止超时")
}
