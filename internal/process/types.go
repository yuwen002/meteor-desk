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
	DependsOn  []string          `json:"dependsOn"`
	Restart    RestartPolicy     `json:"restart,omitempty"`
	Health     *HealthCheck      `json:"health,omitempty"`
	// ReadyTimeoutSeconds 控制作为上游依赖时等待健康检查通过的最长秒数；0 表示不等待。
	ReadyTimeoutSeconds int `json:"readyTimeoutSeconds,omitempty"`
	// Ports 是服务期望监听的 TCP 端口，用于启动时的占用提示与详情面板诊断；
	// 为空表示不参与端口诊断。端口由服务自己决定，这里只做检查，不会修改启动参数。
	Ports []int `json:"ports,omitempty"`
}

// RestartPolicy 控制服务异常退出后的自动重启。
// Mode 只支持 none 和 on-failure；正常退出和手动停止不会触发重启。
type RestartPolicy struct {
	Mode        string `json:"mode"`
	MaxAttempts int    `json:"maxAttempts"`
}

const (
	RestartModeNone      = "none"
	RestartModeOnFailure = "on-failure"
)

// HealthCheck 通过 TCP 连接或 HTTP 请求判断服务是否可用。
// 健康检查只用于展示状态，不会因为探测失败自动重启服务。
type HealthCheck struct {
	Type             string `json:"type"`
	Target           string `json:"target"`
	IntervalSeconds  int    `json:"intervalSeconds"`
	TimeoutSeconds   int    `json:"timeoutSeconds"`
	FailureThreshold int    `json:"failureThreshold"`
}

const (
	HealthCheckTCP  = "tcp"
	HealthCheckHTTP = "http"
)

// 健康状态：checking 表示已启动但尚未得到探测结果。
const (
	HealthChecking  = "checking"
	HealthHealthy   = "healthy"
	HealthUnhealthy = "unhealthy"
)

// State 表示托管进程的生命周期状态。
type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateFailed   State = "failed"
)

// ExitReason 说明进程最近一次运行结束的原因。
type ExitReason string

const (
	ExitReasonNormal      ExitReason = "normal"
	ExitReasonNonzero     ExitReason = "nonzero"
	ExitReasonSignal      ExitReason = "signal"
	ExitReasonManualStop  ExitReason = "manual-stop"
	ExitReasonStartFailed ExitReason = "start-error"
)

// Snapshot 是提供给界面展示的进程状态快照。
type Snapshot struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	State      State      `json:"state"`
	PID        int        `json:"pid,omitempty"`
	StartedAt  string     `json:"startedAt,omitempty"`
	Error      string     `json:"error,omitempty"`
	ExitCode   *int       `json:"exitCode,omitempty"`
	ExitReason ExitReason `json:"exitReason,omitempty"`
	FinishedAt string     `json:"finishedAt,omitempty"`
	DurationMs *int64     `json:"durationMs,omitempty"`
	// Health 为 checking、healthy 或 unhealthy；未配置健康检查时为空。
	Health          string `json:"health,omitempty"`
	HealthMessage   string `json:"healthMessage,omitempty"`
	HealthCheckedAt string `json:"healthCheckedAt,omitempty"`
	// Restarts 是本次运行周期内的自动重启次数，手动启动会清零。
	Restarts      int    `json:"restarts,omitempty"`
	NextRestartAt string `json:"nextRestartAt,omitempty"`
	// CPUPercent、MemoryBytes 与 MetricsAt 是后台资源采样结果，进程退出后清空。
	CPUPercent  float64 `json:"cpuPercent,omitempty"`
	MemoryBytes uint64  `json:"memoryBytes,omitempty"`
	MetricsAt   string  `json:"metricsAt,omitempty"`
}

// ResourceSample 是一次资源采样点，按时间顺序保存在内存中供前端绘制短期曲线。
// 只保留最近 5 分钟（采样间隔 2 秒）；进程退出后保留本次运行的最后一段曲线，重启后重新积累。
type ResourceSample struct {
	At          string  `json:"at"`
	CPUPercent  float64 `json:"cpuPercent"`
	MemoryBytes uint64  `json:"memoryBytes"`
}

// BatchResult 描述批量操作中单个服务的执行结果。
type BatchResult struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Action string `json:"action"`
	Success bool `json:"success"`
	Message string `json:"message"`
}
