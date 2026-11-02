package process

import (
	"io"
	"os/exec"
	"sync"
	"time"
)

// View 将服务配置与当前运行快照组合，作为前端列表项的数据结构。
type View struct {
	Spec   Spec     `json:"spec"`
	Status Snapshot `json:"status"`
}

// managedProcess 是服务的一次运行实例：进程句柄、输出管道，以及探测与采样协程的
// 停止句柄和该次运行的状态快照。每次启动都会新建实例，自动重启产生的是新实例，
// 因此“实例是否还是原来那个”可以用来判断操作是否已经过期。
// 所有字段只在 m.mu 下读写（cmd、done 等在启动后仅由退出协程按约定使用）。
type managedProcess struct {
	cmd             *exec.Cmd
	done            chan struct{}
	startedAt       time.Time
	stdout          io.Closer
	stderr          io.Closer
	closeOutputOnce sync.Once
	probeStop       chan struct{}
	probeOnce       sync.Once
	metricsStop     chan struct{}
	metricsOnce     sync.Once
	// history 是资源采样的环形窗口，只在 m.mu 下读写。
	history []ResourceSample
	// disk 是本次运行的输出落盘写入器；nil 表示未开启落盘。
	// 它在 startOne 锁内创建、在 waitForExit 输出读取结束后关闭；写入自带内部锁，不占用 m.mu。
	disk *diskRotator
	// peakCPUPercent 与 peakMemoryBytes 是本次运行期间资源采样的最大值，退出时写入运行历史。
	peakCPUPercent  float64
	peakMemoryBytes uint64
	// readyDone 标记本次运行是否已记录就绪事件（首次输出或健康通过，先到为准）。
	readyDone bool
	snapshot  Snapshot
}

// stopProbe 结束健康检查协程；重复调用是安全的。
func (p *managedProcess) stopProbe() {
	if p == nil || p.probeStop == nil {
		return
	}
	p.probeOnce.Do(func() { close(p.probeStop) })
}

// stopSamplers 结束健康检查与资源采样协程；重复调用是安全的。
func (p *managedProcess) stopSamplers() {
	p.stopProbe()
	if p == nil || p.metricsStop == nil {
		return
	}
	p.metricsOnce.Do(func() { close(p.metricsStop) })
}

// closeOutputPipes 关闭输出读取端。后代进程可能继承了管道句柄，
// 根进程退出后管道仍未结束，靠它保证停止流程不会永远停在“停止中”。
func (p *managedProcess) closeOutputPipes() {
	if p == nil {
		return
	}
	p.closeOutputOnce.Do(func() {
		if p.stdout != nil {
			_ = p.stdout.Close()
		}
		if p.stderr != nil {
			_ = p.stderr.Close()
		}
	})
}
