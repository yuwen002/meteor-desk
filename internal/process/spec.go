package process

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"
)

// sensitiveEnvFragments 用于识别疑似存放凭据的环境变量名，命中即在做脱敏导出时清空取值。
// 比较时先把键名转小写并去掉 _ - . ，因此 PASSWORD、db_url、API-KEY 等写法都能覆盖。
var sensitiveEnvFragments = []string{
	"password", "passwd", "pwd", "secret", "token", "apikey", "accesskey", "privatekey",
	"credential", "authorization", "cookie", "signingkey", "clientsecret", "dsn",
	"databaseurl", "dburl", "redisurl", "mongouri", "postgresurl", "mysqlurl",
	"jdbcurl", "connectionstring", "connectionuri", "webhook",
}

// normalizeSpec 就地整理并校验服务定义：去除首尾空白、补齐默认值、拒绝非法输入。
// 校验失败时返回可直接展示给用户的中文原因，调用方不应保存该配置。
func normalizeSpec(spec *Spec) error {
	spec.ID = strings.TrimSpace(spec.ID)
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Executable = strings.TrimSpace(spec.Executable)
	spec.WorkingDir = strings.TrimSpace(spec.WorkingDir)
	if spec.Name == "" {
		return errors.New("服务名称不能为空")
	}
	if spec.Executable == "" {
		return errors.New("可执行文件不能为空")
	}
	if strings.ContainsRune(spec.ID, 0) {
		return errors.New("服务 ID 包含非法字符")
	}
	if len(spec.Args) > 128 || len(spec.Env) > 128 || len(spec.DependsOn) > 64 {
		return errors.New("参数最多 128 个、环境变量最多 128 个、依赖最多 64 个")
	}
	if strings.ContainsRune(spec.Executable, 0) {
		return errors.New("可执行文件路径包含非法字符")
	}
	for _, arg := range spec.Args {
		if strings.ContainsRune(arg, 0) {
			return errors.New("命令参数包含非法字符")
		}
	}
	for key, value := range spec.Env {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return fmt.Errorf("环境变量名称或值无效：%q", key)
		}
	}
	spec.Args = append([]string(nil), spec.Args...)
	spec.DependsOn = uniqueIDs(spec.DependsOn)
	if len(spec.Ports) > maxDeclaredPorts {
		return fmt.Errorf("监听端口最多声明 %d 个", maxDeclaredPorts)
	}
	ports := make([]int, 0, len(spec.Ports))
	seenPorts := make(map[int]struct{}, len(spec.Ports))
	for _, port := range spec.Ports {
		if port < 1 || port > 65535 {
			return fmt.Errorf("监听端口需在 1 到 65535 之间：%d", port)
		}
		if _, exists := seenPorts[port]; exists {
			continue
		}
		seenPorts[port] = struct{}{}
		ports = append(ports, port)
	}
	// 统一升序去重，保证配置文件与端口诊断面板的顺序稳定。
	sort.Ints(ports)
	spec.Ports = ports
	if spec.Env == nil {
		spec.Env = map[string]string{}
	}
	spec.Restart.Mode = strings.ToLower(strings.TrimSpace(spec.Restart.Mode))
	if spec.Restart.Mode == "" {
		spec.Restart.Mode = RestartModeNone
	}
	if spec.Restart.Mode != RestartModeNone && spec.Restart.Mode != RestartModeOnFailure {
		return errors.New("自动重启模式只支持 none 或 on-failure")
	}
	if spec.Restart.Mode == RestartModeOnFailure {
		if spec.Restart.MaxAttempts == 0 {
			spec.Restart.MaxAttempts = 3
		}
		if spec.Restart.MaxAttempts < 1 || spec.Restart.MaxAttempts > 20 {
			return errors.New("自动重启次数需在 1 到 20 之间")
		}
	} else {
		spec.Restart.MaxAttempts = 0
	}
	if spec.Health != nil {
		check := *spec.Health
		check.Type = strings.ToLower(strings.TrimSpace(check.Type))
		check.Target = strings.TrimSpace(check.Target)
		switch check.Type {
		case HealthCheckTCP:
			if _, _, err := net.SplitHostPort(check.Target); err != nil {
				return errors.New("TCP 健康检查目标需为 host:port，例如 127.0.0.1:8080")
			}
		case HealthCheckHTTP:
			parsed, err := url.Parse(check.Target)
			if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return errors.New("HTTP 健康检查目标需为 http:// 或 https:// 地址")
			}
		default:
			return errors.New("健康检查类型只支持 tcp 或 http")
		}
		if check.IntervalSeconds == 0 {
			check.IntervalSeconds = 10
		}
		if check.IntervalSeconds < 1 || check.IntervalSeconds > 300 {
			return errors.New("健康检查间隔需在 1 到 300 秒之间")
		}
		if check.TimeoutSeconds == 0 {
			check.TimeoutSeconds = 2
		}
		if check.TimeoutSeconds < 1 || check.TimeoutSeconds > 30 {
			return errors.New("健康检查超时需在 1 到 30 秒之间")
		}
		// 超时不应超过间隔，否则一次探测会拖到下一个周期之后。
		if check.TimeoutSeconds > check.IntervalSeconds {
			check.TimeoutSeconds = check.IntervalSeconds
		}
		if check.FailureThreshold == 0 {
			check.FailureThreshold = 3
		}
		if check.FailureThreshold < 1 || check.FailureThreshold > 10 {
			return errors.New("健康检查失败阈值需在 1 到 10 次之间")
		}
		spec.Health = &check
	}
	if spec.ReadyTimeoutSeconds < 0 || spec.ReadyTimeoutSeconds > 600 {
		return errors.New("依赖就绪等待需在 0 到 600 秒之间")
	}
	// 没有健康检查时无法判断就绪，统一归零，避免导入文件里残留无意义的值。
	if spec.Health == nil {
		spec.ReadyTimeoutSeconds = 0
	}
	return nil
}

// isSensitiveEnvKey 判断环境变量名是否疑似存放凭据。
func isSensitiveEnvKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", ".", "").Replace(strings.ToLower(key))
	for _, fragment := range sensitiveEnvFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

// uniqueIDs 去除空白与重复项，保持首次出现的顺序。
func uniqueIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// cloneSpec 深拷贝切片与映射字段，避免调用方拿到内部可变对象。
func cloneSpec(spec Spec) Spec {
	spec.Args = append([]string(nil), spec.Args...)
	spec.Ports = append([]int(nil), spec.Ports...)
	if spec.Env != nil {
		env := make(map[string]string, len(spec.Env))
		for k, v := range spec.Env {
			env[k] = v
		}
		spec.Env = env
	}
	if spec.Health != nil {
		check := *spec.Health
		spec.Health = &check
	}
	return spec
}

// newID 生成服务配置的唯一 ID；随机数不可用时退化为时间戳。
func newID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err == nil {
		return hex.EncodeToString(data[:])
	}
	return fmt.Sprintf("service-%d", time.Now().UnixNano())
}
