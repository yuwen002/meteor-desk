//go:build darwin

package process

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// sampleProcess 调用系统 ps 读取累计 CPU 时间与常驻内存（KB）。
// macOS 没有可移植的 /proc，ps 是无需额外依赖的稳定来源。
func sampleProcess(pid int) (time.Duration, uint64, error) {
	out, err := exec.Command("ps", "-o", "time=", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return 0, 0, fmt.Errorf("无法解析 ps 输出：%q", string(out))
	}
	cpu, err := parsePSTime(fields[0])
	if err != nil {
		return 0, 0, err
	}
	rssKB, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	return cpu, rssKB * 1024, nil
}
