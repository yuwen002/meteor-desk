package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSanitizeLogName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"api-server", "api-server.log"},
		{"  My Service  ", "My Service.log"},
		{"C:\\path\\to\\app.exe", "app.exe.log"},
		{"weird:*?\"<>|name", "weird-------name.log"},
		{"already.log", "already.log"},
		{"   ", "service.log"},
		{"../../etc/passwd", "passwd.log"},
	}
	for _, tc := range cases {
		if got := sanitizeLogName(tc.in); got != tc.want {
			t.Errorf("sanitizeLogName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDiskRotatorWritesAndAppends(t *testing.T) {
	dir := t.TempDir()
	w := newDiskRotator(dir, "svc.log", 1<<20, 3)
	if err := w.Write("one"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Write("two"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "svc.log"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := string(data); got != "one\ntwo\n" {
		t.Fatalf("file content = %q, want %q", got, "one\ntwo\n")
	}
}

func TestDiskRotatorReopensAndKeepsExistingSize(t *testing.T) {
	dir := t.TempDir()
	w := newDiskRotator(dir, "svc.log", 1<<20, 3)
	if err := w.Write("first\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// 重新打开同一服务应续写而不是覆盖。
	w2 := newDiskRotator(dir, "svc.log", 1<<20, 3)
	if err := w2.Write("second"); err != nil {
		t.Fatalf("write2: %v", err)
	}
	if err := w2.Close(); err != nil {
		t.Fatalf("close2: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "svc.log"))
	if !strings.HasPrefix(string(data), "first") || !strings.Contains(string(data), "second") {
		t.Fatalf("expected append across reopen, got %q", string(data))
	}
}

func TestDiskRotatorRotatesAndDropsOldest(t *testing.T) {
	dir := t.TempDir()
	// 每条记录（含换行）3 字节，上限 12 字节 → 每 4 条写满触发一次轮转；写 10 条足以产生三个保留文件。
	w := newDiskRotator(dir, "svc.log", 12, 3)
	for i := 1; i <= 10; i++ {
		if err := w.Write(fmt.Sprintf("r%d", i)); err != nil { // "r1"=2 + "\n"=1 → 每条 3 字节
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// keep=3 意味着最多存在 svc.log、svc.log.1、svc.log.2 三个文件。
	for _, name := range []string{"svc.log", "svc.log.1", "svc.log.2"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected retained file %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "svc.log.3")); !os.IsNotExist(err) {
		t.Fatalf("svc.log.3 should have been dropped, stat err = %v", err)
	}
	// 每个保留文件都非空，且单文件不超过上限。
	for _, name := range []string{"svc.log", "svc.log.1", "svc.log.2"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if info.Size() == 0 {
			t.Fatalf("%s is empty after rotation", name)
		}
		if info.Size() > 12 {
			t.Fatalf("%s size %d exceeds max 12", name, info.Size())
		}
	}
}

func TestDiskRotatorKeepOneTruncatesInPlace(t *testing.T) {
	dir := t.TempDir()
	w := newDiskRotator(dir, "svc.log", 6, 1)
	for _, entry := range []string{"aaaa", "bbbb", "cccc"} {
		if err := w.Write(entry); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "svc.log.1")); !os.IsNotExist(err) {
		t.Fatalf("keep=1 must not create backups, stat err = %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "svc.log"))
	if len(data) > 6 {
		t.Fatalf("active file grew beyond cap: %d bytes, content %q", len(data), string(data))
	}
}

func TestDiskRotatorConcurrentWritesDoNotInterleave(t *testing.T) {
	dir := t.TempDir()
	w := newDiskRotator(dir, "svc.log", 1<<20, 3)
	var wg sync.WaitGroup
	const per = 200
	for writer := 0; writer < 4; writer++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < per; i++ {
				_ = w.Write(fmt.Sprintf("w%d-%03d", id, i))
			}
		}(writer)
	}
	wg.Wait()
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "svc.log"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 4*per {
		t.Fatalf("expected %d lines, got %d", 4*per, len(lines))
	}
	// 每一行都必须是完整的一条记录，说明并发写没有交错出半行。
	for _, line := range lines {
		if len(line) != 6 { // "wX-NNN" 固定 6 字节
			t.Fatalf("corrupted line %q (len %d)", line, len(line))
		}
	}
}
