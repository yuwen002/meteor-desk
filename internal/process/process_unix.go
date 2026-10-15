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

// processExitStatus reports a conventional exit code and whether the process
// was terminated by a Unix signal.
func processExitStatus(cmd *exec.Cmd) (*int, bool) {
	if cmd == nil || cmd.ProcessState == nil {
		return nil, false
	}
	if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return nil, true
	}
	code := cmd.ProcessState.ExitCode()
	if code < 0 {
		return nil, false
	}
	return &code, false
}
