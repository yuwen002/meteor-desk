package process

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseNetstatListeners(t *testing.T) {
	windows := `活动连接

  协议  本地地址          外部地址        状态           PID
  TCP    0.0.0.0:135            0.0.0.0:0              LISTENING       1234
  TCP    127.0.0.1:8080         0.0.0.0:0              LISTENING       5678
  TCP    [::]:445               [::]:0                 LISTENING       4
  UDP    0.0.0.0:500            *:*                                    99
`
	listeners := parseNetstatListeners(windows)
	if len(listeners) != 3 {
		t.Fatalf("应解析出 3 条 TCP 监听记录，实际 %d：%+v", len(listeners), listeners)
	}
	want := map[int]int{135: 1234, 8080: 5678, 445: 4}
	for _, listener := range listeners {
		if want[listener.Port] != listener.PID {
			t.Fatalf("端口 %d 的 PID 不正确：%+v", listener.Port, listener)
		}
	}

	linux := `Active Internet connections (only servers)
Proto Recv-Q Send-Q Local Address           Foreign Address         State       PID/Program name
tcp        0      0 0.0.0.0:22              0.0.0.0:*               LISTEN      901/sshd
`
	listeners = parseNetstatListeners(linux)
	if len(listeners) != 1 || listeners[0].Port != 22 || listeners[0].PID != 901 {
		t.Fatalf("Linux netstat 解析结果不正确：%+v", listeners)
	}
}

func TestParseLsofListeners(t *testing.T) {
	output := `p4321
cnode
n*:3000
p99
cnginx
n127.0.0.1:8080
n[::1]:8080
`
	listeners := parseLsofListeners(output)
	if len(listeners) != 3 {
		t.Fatalf("应解析出 3 条监听记录，实际 %d：%+v", len(listeners), listeners)
	}
	first := listeners[0]
	if first.Port != 3000 || first.PID != 4321 || first.Process != "node" {
		t.Fatalf("首条监听记录不正确：%+v", first)
	}
	for _, listener := range listeners[1:] {
		if listener.Port != 8080 || listener.PID != 99 || listener.Process != "nginx" {
			t.Fatalf("双栈监听记录不正确：%+v", listener)
		}
	}
}

func TestParseSSListeners(t *testing.T) {
	output := `State  Recv-Q Send-Q Local Address:Port  Peer Address:Port Process
LISTEN 0      511          127.0.0.1:8081       0.0.0.0:*     users:(("node",pid=2222,fd=22))
LISTEN 0      4096               [::]:22              [::]:*     users:(("sshd",pid=777,fd=3))
LISTEN 0      4096                 *:9090                 *:*
`
	listeners := parseSSListeners(output)
	if len(listeners) != 2 {
		t.Fatalf("应解析出带进程信息的 2 条记录，实际 %d：%+v", len(listeners), listeners)
	}
	if listeners[0].Port != 8081 || listeners[0].PID != 2222 || listeners[0].Process != "node" {
		t.Fatalf("ss 解析结果不正确：%+v", listeners[0])
	}
	if listeners[1].Port != 22 || listeners[1].PID != 777 {
		t.Fatalf("IPv6 监听解析结果不正确：%+v", listeners[1])
	}
}

func TestPortFromAddress(t *testing.T) {
	cases := map[string]int{
		"0.0.0.0:8080":     8080,
		"[::]:135":         135,
		"*:3000":           3000,
		"127.0.0.1.5432":   5432,
		"127.0.0.1:65535":  65535,
		"127.0.0.1:70000":  0,
		"[::]":             0,
		"":                 0,
		"127.0.0.1":        0,
	}
	for address, want := range cases {
		if got := portFromAddress(address); got != want {
			t.Fatalf("portFromAddress(%q) = %d，期望 %d", address, got, want)
		}
	}
}

func TestNormalizeSpecDeclaredPorts(t *testing.T) {
	spec := Spec{Name: "端口服务", Executable: "demo", Ports: []int{8080, 80, 8080}}
	if err := normalizeSpec(&spec); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(spec.Ports, []int{80, 8080}) {
		t.Fatalf("声明端口应去重升序：%+v", spec.Ports)
	}

	for _, bad := range [][]int{{0}, {65536}, {-1}} {
		if err := normalizeSpec(&Spec{Name: "非法端口", Executable: "demo", Ports: bad}); err == nil {
			t.Fatalf("应拒绝非法端口：%+v", bad)
		}
	}
	tooMany := make([]int, maxDeclaredPorts+1)
	for i := range tooMany {
		tooMany[i] = 10000 + i
	}
	if err := normalizeSpec(&Spec{Name: "端口过多", Executable: "demo", Ports: tooMany}); err == nil {
		t.Fatal("超过上限的声明端口应被拒绝")
	}

	empty := Spec{Name: "无端口", Executable: "demo"}
	if err := normalizeSpec(&empty); err != nil {
		t.Fatal(err)
	}
	if empty.Ports == nil || len(empty.Ports) != 0 {
		t.Fatalf("未声明端口时应初始化为空切片：%+v", empty.Ports)
	}
}

func TestManagerPortDiagnosticsReportsOccupiedPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(Spec{ID: "api", Name: "API", Executable: "demo", Ports: []int{port}}); err != nil {
		t.Fatal(err)
	}
	report, err := manager.PortDiagnostics("api")
	if err != nil {
		t.Fatal(err)
	}
	if report.ScanError != "" {
		t.Skipf("当前环境无法扫描监听端口：%s", report.ScanError)
	}
	if !reflect.DeepEqual(report.Declared, []int{port}) {
		t.Fatalf("诊断结果应带回声明端口：%+v", report)
	}
	if len(report.Ports) != 1 {
		t.Fatalf("应返回一个端口状态：%+v", report.Ports)
	}
	status := report.Ports[0]
	if status.State != PortOccupied {
		t.Fatalf("测试监听端口应判定为被占用：%+v", status)
	}
	if status.PID != os.Getpid() {
		t.Fatalf("占用进程应是测试进程 %d：%+v", os.Getpid(), status)
	}
	if len(report.Listening) != 0 {
		t.Fatalf("服务未运行时不应报告自身监听端口：%+v", report.Listening)
	}
}

func TestManagerPortDiagnosticsUnknownService(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PortDiagnostics("missing"); err == nil {
		t.Fatal("未知服务应返回错误")
	}
}

func TestLeadingInt(t *testing.T) {
	cases := map[string]int{"1234": 1234, "1234/node": 1234, "": 0, "node": 0, "0": 0}
	for value, want := range cases {
		if got := leadingInt(value); got != want {
			t.Fatalf("leadingInt(%q) = %d，期望 %d", value, got, want)
		}
	}
	if got := leadingInt("007"); got != 7 {
		t.Fatalf("前导零应解析为 7：%d", got)
	}
}
