// Package process defines domain types for the local process manager.
package process

// Spec is an explicit command definition: the runner need not interpret arbitrary shell text.
type Spec struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Executable string `json:"executable"`
	Args []string `json:"args"`
	WorkingDir string `json:"workingDir"`
	Env map[string]string `json:"env"`
	AutoStart bool `json:"autoStart"`
}

type State string
const (
	StateStopped State = "stopped"
	StateStarting State = "starting"
	StateRunning State = "running"
	StateStopping State = "stopping"
	StateFailed State = "failed"
)

type Snapshot struct {
	ID string `json:"id"`
	Name string `json:"name"`
	State State `json:"state"`
	PID int `json:"pid,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	Error string `json:"error,omitempty"`
}
