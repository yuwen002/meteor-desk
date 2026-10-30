package process

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 端口状态：free 表示暂时无人监听，owned 表示由本服务进程监听，
// occupied 表示被其他进程占用，unknown 表示扫描失败无法判断。
const (
	PortFree     = "free"
	PortOwned    = "owned"
	PortOccupied = "occupied"
	PortUnknown  = "unknown"
)

// portsScanTimeout 限制一次端口扫描的最长耗时；系统自带的查看命令通常远小于此值。
const portsScanTimeout = 5 * time.Second

// maxDeclaredPorts 限制单个服务可声明的端口数量，避免诊断面板过长。
const maxDeclaredPorts = 20

// PortStatus 描述一个端口的监听情况；只有有人监听时才填充 PID 与进程名。
type PortStatus struct {
	Port    int    `json:"port"`
	State   string `json:"state"`
	PID     int    `json:"pid,omitempty"`
	Process string `json:"process,omitempty"`
}

// PortReport 是端口诊断结果。Ports 对应配置中声明的端口，
// Listening 是本服务进程实际监听的端口（无需配置即可发现）。
type PortReport struct {
	SpecID    string       `json:"specId"`
	Declared  []int        `json:"declared"`
	Ports     []PortStatus `json:"ports"`
	Listening []PortStatus `json:"listening"`
	ScannedAt string       `json:"scannedAt"`
	ScanError string       `json:"scanError,omitempty"`
}

// listenOwner 是一条系统 TCP 监听记录。
type listenOwner struct {
	Port    int
	PID     int
	Process string
}

// portCommand 描述一个可用于读取监听端口的系统命令。
type portCommand struct {
	name  string
	args  []string
	parse func(string) []listenOwner
}

// PortDiagnostics 扫描本机 TCP 监听端口，返回声明端口的占用情况和本服务实际监听的端口。
// 扫描会调用系统命令，因此只在锁外执行；服务未运行时 Listening 为空。
func (m *Manager) PortDiagnostics(id string) (PortReport, error) {
	m.mu.RLock()
	spec, ok := m.specs[id]
	if !ok {
		m.mu.RUnlock()
		return PortReport{}, errors.New("未找到该服务配置")
	}
	declared := append([]int(nil), spec.Ports...)
	pid := 0
	if instance := m.instances[id]; instance != nil {
		pid = instance.snapshot.PID
	}
	m.mu.RUnlock()

	report := PortReport{
		SpecID: id, Declared: declared, Ports: []PortStatus{}, Listening: []PortStatus{},
		ScannedAt: time.Now().Format(time.RFC3339Nano),
	}
	listeners, err := scanListeners()
	if err != nil {
		report.ScanError = err.Error()
		for _, port := range declared {
			report.Ports = append(report.Ports, PortStatus{Port: port, State: PortUnknown})
		}
		return report, nil
	}
	byPort := firstOwnerByPort(listeners)
	for _, port := range declared {
		report.Ports = append(report.Ports, portStatus(port, byPort[port], pid))
	}
	if pid > 0 {
		for _, listener := range listeners {
			if listener.PID != pid {
				continue
			}
			report.Listening = append(report.Listening, PortStatus{Port: listener.Port, State: PortOwned, PID: listener.PID, Process: listener.Process})
		}
		sort.Slice(report.Listening, func(i, j int) bool { return report.Listening[i].Port < report.Listening[j].Port })
	}
	return report, nil
}

// warnPortConflicts 在服务启动后检查声明端口是否被其他进程占用，并写入运行输出。
// 扫描需要调用系统命令，因此放在后台协程执行，不阻塞启动；未声明端口时完全跳过。
func (m *Manager) warnPortConflicts(id string, ports []int, pid int) {
	if len(ports) == 0 {
		return
	}
	listeners, err := scanListeners()
	if err != nil {
		m.appendOutput(id, "system", "端口诊断失败："+err.Error())
		return
	}
	byPort := firstOwnerByPort(listeners)
	for _, port := range ports {
		owner := byPort[port]
		if owner.PID == 0 || owner.PID == pid {
			continue
		}
		name := owner.Process
		if name == "" {
			name = "未知进程"
		}
		m.appendOutput(id, "system", fmt.Sprintf("端口 %d 已被 PID %d（%s）占用，本服务可能无法监听；可在详情面板查看端口诊断", port, owner.PID, name))
	}
}

// scanListeners 依次尝试当前平台可用的端口查看命令，返回全部 TCP 监听记录。
func scanListeners() ([]listenOwner, error) {
	netstat := []string{"-ano", "-p", "TCP"}
	var commands []portCommand
	switch runtime.GOOS {
	case "windows":
		commands = []portCommand{{"netstat", netstat, parseNetstatListeners}}
	case "darwin":
		commands = []portCommand{
			{"lsof", []string{"-nP", "-iTCP", "-sTCP:LISTEN", "-Fpc"}, parseLsofListeners},
			{"netstat", []string{"-an", "-p", "tcp"}, parseNetstatListeners},
		}
	default:
		commands = []portCommand{
			{"ss", []string{"-ltnp"}, parseSSListeners},
			{"lsof", []string{"-nP", "-iTCP", "-sTCP:LISTEN", "-Fpc"}, parseLsofListeners},
			{"netstat", []string{"-ltn"}, parseNetstatListeners},
		}
	}
	var lastErr error
	for _, command := range commands {
		output, err := runPortCommand(command.name, command.args...)
		if err != nil {
			lastErr = fmt.Errorf("%s 执行失败：%w", command.name, err)
			continue
		}
		if listeners := command.parse(output); len(listeners) > 0 {
			return listeners, nil
		}
		lastErr = fmt.Errorf("%s 未返回监听端口", command.name)
	}
	if lastErr == nil {
		lastErr = errors.New("当前平台没有可用的端口查看命令")
	}
	return nil, lastErr
}

// runPortCommand 执行端口查看命令并限制总耗时，避免命令异常时阻塞界面。
func runPortCommand(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), portsScanTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// firstOwnerByPort 为每个端口保留第一条监听记录，避免双栈监听（0.0.0.0 与 ::）重复展示。
func firstOwnerByPort(listeners []listenOwner) map[int]listenOwner {
	byPort := make(map[int]listenOwner, len(listeners))
	for _, listener := range listeners {
		if _, exists := byPort[listener.Port]; !exists {
			byPort[listener.Port] = listener
		}
	}
	return byPort
}

// portStatus 把监听记录转换成端口状态；ownPID 为 0 时不会判定为“本服务监听”。
func portStatus(port int, owner listenOwner, ownPID int) PortStatus {
	if owner.PID == 0 {
		return PortStatus{Port: port, State: PortFree}
	}
	state := PortOccupied
	if ownPID > 0 && owner.PID == ownPID {
		state = PortOwned
	}
	return PortStatus{Port: port, State: state, PID: owner.PID, Process: owner.Process}
}

// parseNetstatListeners 解析 netstat 的 LISTENING/LISTEN 行。
// Windows 为“TCP 本地地址 外部地址 LISTENING PID”，Linux netstat 多两列队列长度，
// 因此按状态列的位置回溯本地地址与 PID，两种格式共用同一套逻辑。
func parseNetstatListeners(output string) []listenOwner {
	var listeners []listenOwner
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		protocol := strings.ToLower(fields[0])
		if protocol != "tcp" && protocol != "tcp6" {
			continue
		}
		stateIndex := -1
		for index, field := range fields {
			upper := strings.ToUpper(field)
			if upper == "LISTENING" || upper == "LISTEN" {
				stateIndex = index
				break
			}
		}
		if stateIndex < 2 || stateIndex+1 >= len(fields) {
			continue
		}
		port := portFromAddress(fields[stateIndex-2])
		pid := leadingInt(fields[stateIndex+1])
		if port == 0 || pid == 0 {
			continue
		}
		listeners = append(listeners, listenOwner{Port: port, PID: pid})
	}
	return listeners
}

// parseLsofListeners 解析 lsof -Fpc 输出：p 开头为进程号，c 开头为进程名，n 开头为监听地址。
func parseLsofListeners(output string) []listenOwner {
	var listeners []listenOwner
	pid, name := 0, ""
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 2 {
			continue
		}
		value := line[1:]
		switch line[0] {
		case 'p':
			pid = leadingInt(value)
		case 'c':
			name = value
		case 'n':
			port := portFromAddress(value)
			if port == 0 || pid == 0 {
				continue
			}
			listeners = append(listeners, listenOwner{Port: port, PID: pid, Process: name})
		}
	}
	return listeners
}

// ssListenPattern 从 ss -ltnp 的 users:(("node",pid=1234,fd=20)) 片段提取进程名与 PID。
var ssListenPattern = regexp.MustCompile(`\("([^"]+)",pid=(\d+)`)

// parseSSListeners 解析 ss -ltnp 输出：本地地址固定在第 4 列，进程信息在行尾。
func parseSSListeners(output string) []listenOwner {
	var listeners []listenOwner
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || strings.ToUpper(fields[0]) != "LISTEN" {
			continue
		}
		port := portFromAddress(fields[3])
		match := ssListenPattern.FindStringSubmatch(line)
		if port == 0 || len(match) != 3 {
			continue
		}
		pid := leadingInt(match[2])
		if pid == 0 {
			continue
		}
		listeners = append(listeners, listenOwner{Port: port, PID: pid, Process: match[1]})
	}
	return listeners
}

// portFromAddress 从多种本地地址写法中取出端口：0.0.0.0:8080、[::]:8080、*:8080、127.0.0.1.8080。
// 端口写作 2–5 位数字，因此纯 IP 末段（127.0.0.1、::1）不会被误认成端口。
func portFromAddress(address string) int {
	address = strings.TrimSpace(address)
	if address == "" {
		return 0
	}
	separator := strings.LastIndexByte(address, ':')
	if separator < 0 {
		separator = strings.LastIndexByte(address, '.')
	}
	if separator < 0 || separator+1 >= len(address) {
		return 0
	}
	value := address[separator+1:]
	if len(value) < 2 || len(value) > 5 {
		return 0
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0
	}
	return port
}

// leadingInt 解析字符串开头的十进制数字，忽略 "1234/node" 这类后缀。
func leadingInt(value string) int {
	end := 0
	for end < len(value) && value[end] >= '0' && value[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	parsed, err := strconv.Atoi(value[:end])
	if err != nil {
		return 0
	}
	return parsed
}
