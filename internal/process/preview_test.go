package process

import (
	"strings"
	"testing"
)

func TestRenderCommandSimple(t *testing.T) {
	p := RenderCommand(Spec{Executable: "/usr/bin/python3", Args: []string{"-m", "http.server", "8080"}})
	if strings.ContainsAny(p.CommandLine, `"`) {
		t.Fatalf("简单参数不应出现引号：%s", p.CommandLine)
	}
	want := `/usr/bin/python3 -m http.server 8080`
	if p.CommandLine != want {
		t.Fatalf("got %q want %q", p.CommandLine, want)
	}
}

func TestRenderCommandNeedsQuotes(t *testing.T) {
	p := RenderCommand(Spec{Executable: "C:\\Program Files\\App\\run.exe", Args: []string{"--name=my service", "port=8080"}})
	if !strings.Contains(p.CommandLine, `"C:\Program Files\App\run.exe"`) {
		t.Fatalf("可执行文件含空格应加引号：%s", p.CommandLine)
	}
	if !strings.Contains(p.CommandLine, `"--name=my service"`) {
		t.Fatalf("参数含空格应加引号：%s", p.CommandLine)
	}
	if strings.Contains(p.CommandLine, `"port=8080"`) {
		t.Fatalf("不含特殊字符的参数不应加引号：%s", p.CommandLine)
	}
}

func TestRenderCommandBackslashNoForcedQuote(t *testing.T) {
	p := RenderCommand(Spec{Executable: `C:\path\to\app.exe`, Args: []string{"--conf", `D:\cfg\settings.json`}})
	if strings.Contains(p.CommandLine, `"`) {
		t.Fatalf("路径分隔符反斜杠不应强制加引号：%s", p.CommandLine)
	}
}

func TestRenderCommandEscapesQuoteAndDollar(t *testing.T) {
	p := RenderCommand(Spec{Executable: "cmd", Args: []string{`--name="special"`, `$HOME`, "正常"}})
	want := `cmd "--name=\"special\"" "$HOME" 正常`
	if p.CommandLine != want {
		t.Fatalf("got %q want %q", p.CommandLine, want)
	}
}

func TestRenderCommandEmptyArg(t *testing.T) {
	p := RenderCommand(Spec{Executable: "echo", Args: []string{"hello", "", "world"}})
	want := `echo hello "" world`
	if p.CommandLine != want {
		t.Fatalf("空参数应渲染为 \"\"：got %q want %q", p.CommandLine, want)
	}
}

func TestRenderCommandWorkingDir(t *testing.T) {
	p := RenderCommand(Spec{Executable: "ls", WorkingDir: "/var/log"})
	if p.WorkingDir != "/var/log" {
		t.Fatalf("工作目录未保留：%s", p.WorkingDir)
	}
}

func TestRenderCommandEnvSensitive(t *testing.T) {
	p := RenderCommand(Spec{
		Executable: "app",
		Env:        map[string]string{"DB_PASSWORD": "hunter2", "LOG_LEVEL": "debug", "API_TOKEN": "abc", "PRIVATE_KEY": "xyz"},
	})
	if len(p.Env) != 4 {
		t.Fatalf("环境变量数量不对：%v", p.Env)
	}
	for _, item := range p.Env {
		kv := strings.SplitN(item, "=", 2)
		if len(kv) != 2 {
			t.Fatalf("环境变量格式不对：%s", item)
		}
		key, val := kv[0], kv[1]
		switch key {
		case "DB_PASSWORD", "API_TOKEN", "PRIVATE_KEY":
			if val != "***" {
				t.Fatalf("敏感变量 %s 未脱敏：%s", key, item)
			}
		case "LOG_LEVEL":
			if val != "debug" {
				t.Fatalf("非敏感变量 %s 不应被脱敏：%s", key, item)
			}
		}
	}
}

func TestRenderCommandEnvSorted(t *testing.T) {
	p := RenderCommand(Spec{
		Executable: "app",
		Env:        map[string]string{"ZEBRA": "1", "APPLE": "2", "MANGO": "3"},
	})
	if len(p.Env) != 3 {
		t.Fatalf("环境变量数量不对：%v", p.Env)
	}
	if !strings.HasPrefix(p.Env[0], "APPLE=") || !strings.HasPrefix(p.Env[1], "MANGO=") || !strings.HasPrefix(p.Env[2], "ZEBRA=") {
		t.Fatalf("环境变量未按 key 排序：%v", p.Env)
	}
}

func TestRenderCommandNoEnvOmitsField(t *testing.T) {
	p := RenderCommand(Spec{Executable: "ls"})
	if len(p.Env) != 0 {
		t.Fatalf("无环境变量应返回空列表：%v", p.Env)
	}
}

func TestRenderCommandSpecialShellChars(t *testing.T) {
	specials := []string{`a b`, `ab"c`, `$HOME`, "`cmd`", `pipe|here`, `redirect>out`, `amp&er`, `(parens)`, `semi;`, `hash#tag`, `bang!`, `star*`, `tilde~`, `q?mark`}
	for _, s := range specials {
		p := RenderCommand(Spec{Executable: "cmd", Args: []string{s}})
		if !strings.Contains(p.CommandLine, `"`) {
			t.Fatalf("特殊字符 %q 应触发加引号，实际渲染：%s", s, p.CommandLine)
		}
	}
}