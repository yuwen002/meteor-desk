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
	// LogToDisk 为 true 时，本次运行的 stdout/stderr 会按行追加写入配置目录 logs/ 下的文件，
	// 单文件超过大小上限后轮转并只保留最近几份；关闭应用后仍可回看与检索。
	LogToDisk bool `json:"logToDisk,omitempty"`
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
	// LogFilePath 是本次运行输出落盘的文件绝对路径；未开启落盘时为空。
	// 服务停止后仍保留最后一次的落盘路径，供详情面板与日志分析跳转使用。
	LogFilePath string `json:"logFilePath,omitempty"`
}

// ResourceSample 是一次资源采样点，按时间顺序保存在内存中供前端绘制短期曲线。
// 只保留最近 5 分钟（采样间隔 2 秒）；进程退出后保留本次运行的最后一段曲线，重启后重新积累。
type ResourceSample struct {
	At          string  `json:"at"`
	CPUPercent  float64 `json:"cpuPercent"`
	MemoryBytes uint64  `json:"memoryBytes"`
}

// EventKind 区分服务生命周期事件的类型，供前端时间线着色与命名。
type EventKind string

const (
	EventStart   EventKind = "start"
	EventReady   EventKind = "ready"
	EventRestart EventKind = "restart"
	EventExit    EventKind = "exit"
)

// ProcessEvent 是一次启停生命周期事件，只在内存中保留最近若干条。
// DurationMs 的含义随 Kind 变化：ready 为启动耗时（启动到首次输出或健康通过），exit 为本次运行时长。
type ProcessEvent struct {
	At         string    `json:"at"`
	Kind       EventKind `json:"kind"`
	Detail     string    `json:"detail,omitempty"`
	DurationMs *int64    `json:"durationMs,omitempty"`
}

// RunRecord 是一次服务运行的汇总记录，在进程退出时写入，落盘到 run-history.json。
// 它只保留每次运行的关键指标（起止、退出、重启次数与资源峰值），不含逐行输出与采样曲线。
// DurationMs 为本次运行时长；ExitCode 为 nil 表示未取到退出码（如启动前就失败）。
type RunRecord struct {
	ServiceID   string     `json:"serviceId"`
	ServiceName string     `json:"serviceName,omitempty"`
	StartedAt   string     `json:"startedAt,omitempty"`
	FinishedAt  string     `json:"finishedAt,omitempty"`
	DurationMs  *int64     `json:"durationMs,omitempty"`
	ExitReason  ExitReason `json:"exitReason,omitempty"`
	ExitCode    *int       `json:"exitCode,omitempty"`
	Restarts    int        `json:"restarts,omitempty"`
	// PeakCPUPercent 与 PeakMemoryBytes 是本次运行期间资源采样的最大值；未采样到时为零。
	PeakCPUPercent  float64 `json:"peakCpuPercent,omitempty"`
	PeakMemoryBytes uint64  `json:"peakMemoryBytes,omitempty"`
}

// Group 是一组命名服务引用的集合（一种“场景”），只按 ID 引用已有服务，
// 不复制服务配置；整组启停时按依赖顺序复用批量启停逻辑，落盘到 groups.json。
type Group struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	MemberIDs   []string `json:"memberIds"`
}

// BatchResult 描述批量操作中单个服务的执行结果。
type BatchResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Action  string `json:"action"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}
