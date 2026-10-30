//go:build !windows && !linux && !darwin

package process

import (
	"errors"
	"time"
)

// sampleProcess 在其他平台不采集资源，快照保持为空。
func sampleProcess(int) (time.Duration, uint64, error) {
	return 0, 0, errors.New("当前平台不支持进程资源采样")
}
