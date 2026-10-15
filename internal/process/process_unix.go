//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func requestProcessStop(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil { return nil }
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}

func forceStopProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil { return nil }
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err == nil { return nil }
	return cmd.Process.Kill()
}
