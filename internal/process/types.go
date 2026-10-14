// Package process 定义本地进程管理模块使用的数据类型。
package process

// Spec 描述一个明确配置的本地命令。可执行文件、参数和工作目录分开保存，
// 避免默认把任意输入当成 Shell 脚本解释。
type Spec struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Executable string            `json:"executable"`
	Args       []string          `json:"args"`
	WorkingDir string            `json:"workingDir"`
	Env        map[string]string `json:"env"`
	AutoStart  bool              `json:"autoStart"`
}

// State 表示托管进程的生命周期状态。
type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateFailed   State = "failed"
)

// Snapshot 是提供给界面展示的进程状态快照。
type Snapshot struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	State     State  `json:"state"`
	PID       int    `json:"pid,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	Error     string `json:"error,omitempty"`
}
