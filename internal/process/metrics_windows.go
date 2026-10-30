//go:build windows

package process

import (
	"syscall"
	"time"
	"unsafe"
)

// processQueryInformation 是 OpenProcess 的最小查询权限，避免要求更高权限。
const processQueryInformation = 0x0400

var (
	kernel32DLL         = syscall.NewLazyDLL("kernel32.dll")
	psapiDLL            = syscall.NewLazyDLL("psapi.dll")
	procGetProcessTimes = kernel32DLL.NewProc("GetProcessTimes")
	procGetMemoryInfo   = psapiDLL.NewProc("GetProcessMemoryInfo")
)

// processMemoryCounters 对应 Windows 的 PROCESS_MEMORY_COUNTERS 结构。
type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

// sampleProcess 通过 GetProcessTimes 与 GetProcessMemoryInfo 读取累计 CPU 时间与工作集内存。
func sampleProcess(pid int) (time.Duration, uint64, error) {
	handle, err := syscall.OpenProcess(processQueryInformation, false, uint32(pid))
	if err != nil {
		return 0, 0, err
	}
	defer syscall.CloseHandle(handle)

	var creation, exit, kernel, user syscall.Filetime
	ok, _, callErr := procGetProcessTimes.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&creation)),
		uintptr(unsafe.Pointer(&exit)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if ok == 0 {
		return 0, 0, callErr
	}

	var counters processMemoryCounters
	counters.cb = uint32(unsafe.Sizeof(counters))
	ok, _, callErr = procGetMemoryInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&counters)), uintptr(counters.cb))
	if ok == 0 {
		return 0, 0, callErr
	}

	// FILETIME 以 100 纳秒为单位，换算成 time.Duration 的纳秒。
	cpu := time.Duration(filetimeValue(kernel)+filetimeValue(user)) * 100 * time.Nanosecond
	return cpu, uint64(counters.workingSetSize), nil
}

func filetimeValue(value syscall.Filetime) uint64 {
	return uint64(value.HighDateTime)<<32 | uint64(value.LowDateTime)
}
