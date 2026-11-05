package process

import (
	"sort"
	"strings"
)

// CommandPreview 把 Spec 渲染成用户可以核对的命令行预览：
// CommandLine 带引号与转义，WorkingDir 是启动目录，Env 按 KEY=VALUE 形式列出。
// 仅用于展示，不会真的拼接 Shell 执行。
type CommandPreview struct {
	CommandLine string   `json:"commandLine"`
	WorkingDir  string   `json:"workingDir,omitempty"`
	Env         []string `json:"env,omitempty"`
}

// shellSpecialChars 列出需要触发 quoting 的 Shell 元字符：
// 空格会触发，双引号会在内部被转义；反斜杠不作为触发字符（Windows 路径分隔符），
// 但一旦进入引号内部，反斜杠也会被转义以正确还原。
var shellSpecialChars = " \"()<>|&;'$`#!{[]}~*?"

// RenderCommand 按给定 Spec 渲染预览。
func RenderCommand(spec Spec) CommandPreview {
	parts := []string{shellQuote(spec.Executable)}
	for _, arg := range spec.Args {
		parts = append(parts, shellQuote(arg))
	}
	preview := CommandPreview{
		CommandLine: strings.Join(parts, " "),
		WorkingDir:  spec.WorkingDir,
	}
	if len(spec.Env) > 0 {
		keys := make([]string, 0, len(spec.Env))
		for k := range spec.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := spec.Env[k]
			if isSensitiveEnvKey(k) {
				v = "***"
			} else {
				v = shellQuote(v)
			}
			preview.Env = append(preview.Env, k+"="+v)
		}
	}
	return preview
}

// shellQuote 跨平台命令行 quoting：包含空格或 Shell 特殊字符时用双引号包裹，
// 内部双引号加反斜杠转义；反斜杠作为路径分隔符不额外转义。不含特殊字符则原样返回。
func shellQuote(s string) string {
	if s == "" {
		return `""`
	}
	needsQuote := strings.ContainsAny(s, shellSpecialChars)
	if !needsQuote {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		if r == '"' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}