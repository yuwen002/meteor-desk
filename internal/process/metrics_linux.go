//go:build linux

package process

import (
	"fmt"
	"os"
	"time"
)

// sampleProcess 读取 /proc/<pid>/stat，用用户态与内核态 ticks、常驻页数作为采样结果。
// Linux 的用户态时钟固定为 100 Hz，因此 ticks 除以 100 得到秒。
func sampleProcess(pid int) (time.Duration, uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, 0, err
	}
	ticks, pages, err := parseProcStat(string(data))
	if err != nil {
		return 0, 0, err
	}
	return time.Duration(ticks) * time.Second / 100, uint64(pages) * uint64(os.Getpagesize()), nil
}
