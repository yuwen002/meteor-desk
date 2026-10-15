//go:build windows

package process

import (
	"os/exec"
	"strconv"
)

func configureProcess(cmd *exec.Cmd) {
	// taskkill /T locates descendants from the root PID on Windows.
}

func requestProcessStop(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil { return nil }
	return exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T").Run()
}

func forceStopProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil { return nil }
	err := exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run()
	if err != nil { return cmd.Process.Kill() }
	return nil
}

// Windows reports process termination as an exit code rather than a Unix signal.
func processExitStatus(cmd *exec.Cmd) (*int, bool) {
	if cmd == nil || cmd.ProcessState == nil {
		return nil, false
	}
	code := cmd.ProcessState.ExitCode()
	if code < 0 {
		return nil, false
	}
	return &code, false
}
