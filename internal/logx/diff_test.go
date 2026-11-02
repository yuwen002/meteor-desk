package logx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	textunicode "golang.org/x/text/encoding/unicode"
)

func TestReadDecodedFileHeadUTF8(t *testing.T) {
	path := writeTempLog(t, "head.log", "INFO start\nERROR boom\nWARN slow\n")
	got, err := ReadDecodedFileHead(path, "auto", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Encoding != "UTF-8" {
		t.Fatalf("编码识别异常：%q", got.Encoding)
	}
	if got.Text != "INFO start\nERROR boom\nWARN slow\n" {
		t.Fatalf("文本内容异常：%q", got.Text)
	}
	if got.Truncated {
		t.Fatal("小文件不应被截断")
	}
	if got.SizeBytes != int64(len("INFO start\nERROR boom\nWARN slow\n")) {
		t.Fatalf("文件大小异常：%d", got.SizeBytes)
	}
}

func TestReadDecodedFileHeadTruncatesToMaxBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.log")
	content := strings.Repeat("a", 1024) + "\n" + strings.Repeat("b", 1024) + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDecodedFileHead(path, "utf-8", 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated {
		t.Fatal("超过 maxBytes 应标记截断")
	}
	if int64(len(got.Text)) > 1024 {
		t.Fatalf("读取文本应受限：%d", len(got.Text))
	}
	if !strings.HasPrefix(got.Text, "aaa") {
		t.Fatalf("应保留文件开头：%q", got.Text[:8])
	}
}

func TestReadDecodedFileHeadDecodesUTF16(t *testing.T) {
	path := filepath.Join(t.TempDir(), "utf16.log")
	encoded, err := textunicode.UTF16(textunicode.LittleEndian, textunicode.UseBOM).NewEncoder().Bytes([]byte("INFO 中文\nERROR 错误\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDecodedFileHead(path, "auto", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Encoding, "UTF-16") {
		t.Fatalf("应识别 UTF-16：%q", got.Encoding)
	}
	if got.Text != "INFO 中文\nERROR 错误\n" {
		t.Fatalf("UTF-16 解码异常：%q", got.Text)
	}
}

func TestReadDecodedFileHeadClampsMaxBytes(t *testing.T) {
	path := writeTempLog(t, "clamp.log", "line one\nline two\n")
	// maxBytes 为负数时按 4 MiB 上限处理，仍应正常读取全部内容。
	got, err := ReadDecodedFileHead(path, "auto", -1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "line one\nline two\n" || got.Truncated {
		t.Fatalf("负数上限应回退到默认值：%+v", got)
	}
}

func TestReadDecodedFileHeadRejectsMissingFile(t *testing.T) {
	if _, err := ReadDecodedFileHead(filepath.Join(t.TempDir(), "nope.log"), "auto", 0); err == nil {
		t.Fatal("文件不存在时应返回错误")
	}
	if _, err := ReadDecodedFileHead("  ", "auto", 0); err == nil {
		t.Fatal("空路径应返回错误")
	}
}
