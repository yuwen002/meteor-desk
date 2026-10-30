package logx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func writeTailFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("写入测试文件失败：%v", err)
	}
}

func TestTailFileReadsFromEndAndSkipsPartialLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	writeTailFile(t, path, "line1\nline2\nline3\nline4\n")

	result, err := TailFile(path, TailOptions{Offset: -1, MaxBytes: 12})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Reset {
		t.Fatal("首次从末尾跟踪应标记 Reset")
	}
	if len(result.Lines) != 2 || result.Lines[0] != "line3" || result.Lines[1] != "line4" {
		t.Fatalf("应从末尾返回完整行：%+v", result.Lines)
	}
	if result.Offset != 24 {
		t.Fatalf("偏移应停在文件末尾，实际 %d", result.Offset)
	}

	// 回退点落在行中间时必须丢弃残行，避免输出半行。
	result, err = TailFile(path, TailOptions{Offset: -1, MaxBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 1 || result.Lines[0] != "line4" {
		t.Fatalf("残行应被跳过：%+v", result.Lines)
	}
}

func TestTailFileReturnsOnlyNewLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	writeTailFile(t, path, "a\nb\n")

	first, err := TailFile(path, TailOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Lines) != 2 || first.Offset != 4 {
		t.Fatalf("首次读取结果错误：%+v", first)
	}

	writeTailFile(t, path, "a\nb\nc\n")
	second, err := TailFile(path, TailOptions{Offset: first.Offset})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Lines) != 1 || second.Lines[0] != "c" {
		t.Fatalf("只应返回新增行：%+v", second.Lines)
	}
	if second.Reset {
		t.Fatal("文件增长不应触发 Reset")
	}
}

func TestTailFileResetsWhenFileShrinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	writeTailFile(t, path, "a\nb\n")
	writeTailFile(t, path, "x\n")

	result, err := TailFile(path, TailOptions{Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Reset {
		t.Fatal("文件变小应触发 Reset")
	}
	if len(result.Lines) != 1 || result.Lines[0] != "x" || result.Offset != 2 {
		t.Fatalf("重置后应从头读取：%+v", result)
	}
}

func TestTailFileWaitsForCompleteLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	writeTailFile(t, path, "partial")

	waiting, err := TailFile(path, TailOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(waiting.Lines) != 0 || waiting.Offset != 0 {
		t.Fatalf("未完成行不应输出，也不应前进偏移：%+v", waiting)
	}

	writeTailFile(t, path, "partial\nnext")
	result, err := TailFile(path, TailOptions{Offset: waiting.Offset})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 1 || result.Lines[0] != "partial" || result.Offset != 8 {
		t.Fatalf("应只输出已完成行：%+v", result)
	}
}

func TestTailFileDecodesGB18030(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	encoded, err := simplifiedchinese.GB18030.NewEncoder().String("中文日志\n第二行\n")
	if err != nil {
		t.Fatal(err)
	}
	writeTailFile(t, path, encoded)

	result, err := TailFile(path, TailOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Encoding != "GB18030" {
		t.Fatalf("编码识别结果错误：%s", result.Encoding)
	}
	if len(result.Lines) != 2 || result.Lines[0] != "中文日志" || result.Lines[1] != "第二行" {
		t.Fatalf("GB18030 行解码错误：%+v", result.Lines)
	}
}

func TestTailFileRejectsUTF16(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	content := []byte{0xFF, 0xFE}
	for _, r := range "hello\n" {
		content = append(content, byte(r), 0)
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}

	_, err := TailFile(path, TailOptions{})
	if err == nil || !strings.Contains(err.Error(), "UTF-16") {
		t.Fatalf("UTF-16 应返回明确的编码限制提示：%v", err)
	}
}

func TestTailFileEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	writeTailFile(t, path, "")

	result, err := TailFile(path, TailOptions{Offset: -1})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 0 || result.Offset != 0 || result.FileSizeBytes != 0 {
		t.Fatalf("空文件应返回空增量：%+v", result)
	}
}
