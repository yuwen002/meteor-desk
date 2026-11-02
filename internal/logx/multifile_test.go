package logx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// writeFile 在指定路径写入内容，并按需创建父目录，供多文件测试搭建目录树。
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestEnumerateLogFilesCollectsMatchesSortedWithDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "b.log"), "x")
	writeFile(t, filepath.Join(dir, "a.txt"), "x")
	writeFile(t, filepath.Join(dir, "skip.md"), "x")
	writeFile(t, filepath.Join(dir, "nested", "c.ndjson"), "x")

	files, truncated, err := EnumerateLogFiles(dir, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatal("少量文件不应被截断")
	}
	if len(files) != 3 {
		t.Fatalf("应收集 3 个日志文件，实际 %d：%v", len(files), files)
	}
	base := []string{}
	for _, path := range files {
		base = append(base, filepath.Base(path))
	}
	if base[0] != "a.txt" || base[1] != "b.log" || base[2] != "c.ndjson" {
		t.Fatalf("收集结果未按路径升序或缺少嵌套文件：%v", base)
	}
}

func TestEnumerateLogFilesCustomPatternAndCap(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"app.log", "worker.log", "trace.json", "notes.txt"} {
		writeFile(t, filepath.Join(dir, name), "x")
	}
	files, _, err := EnumerateLogFiles(dir, []string{"*.log"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("自定义模式应只收集 .log，实际 %v", files)
	}

	capped, truncated, err := EnumerateLogFiles(dir, []string{"*"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(capped) != 2 {
		t.Fatalf("上限生效异常：truncated=%v len=%d", truncated, len(capped))
	}
}

func TestEnumerateLogFilesRejectsFileNotDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "single.log")
	writeFile(t, path, "x")
	if _, _, err := EnumerateLogFiles(path, nil, 0); err == nil {
		t.Fatal("传入单个文件应报错，提示改用单文件检索入口")
	}
}

func TestSearchFilesConcurrentlyLabelsSourceAndMerges(t *testing.T) {
	dir := t.TempDir()
	left := filepath.Join(dir, "left.log")
	right := filepath.Join(dir, "right.log")
	writeFile(t, left, "INFO boot\nERROR alpha\nplain\nERROR beta\n")
	writeFile(t, right, "ERROR gamma\ninfo ok\n")

	got, err := SearchFilesConcurrently(context.Background(), []string{left, right}, "ERROR", false, 100, SearchOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 3 {
		t.Fatalf("应合并 3 条命中，实际 %d：%+v", len(got.Matches), got.Matches)
	}
	if got.FilesSucceeded != 2 || got.FilesTotal != 2 || got.FilesFailed != 0 {
		t.Fatalf("文件状态统计异常：%+v", got)
	}
	// 结果按枚举顺序（left 在前）拼接，并标注来源文件。
	if got.Matches[0].FilePath != left || got.Matches[0].LineNumber != 2 {
		t.Fatalf("首条命中来源或行号异常：%+v", got.Matches[0])
	}
	if got.Matches[2].FilePath != right || got.Matches[2].LineNumber != 1 {
		t.Fatalf("末条命中应来自 right 第 1 行：%+v", got.Matches[2])
	}
	for _, m := range got.Matches {
		if m.FilePath == "" {
			t.Fatal("多文件命中必须标注来源文件")
		}
	}
}

func TestSearchFilesConcurrentlyEnforcesGlobalCap(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for i := 0; i < 4; i++ {
		path := filepath.Join(dir, fmt.Sprintf("f%d.log", i))
		writeFile(t, path, strings.Repeat("ERROR hit\n", 10))
		files = append(files, path)
	}
	got, err := SearchFilesConcurrently(context.Background(), files, "ERROR", false, 15, SearchOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 15 {
		t.Fatalf("全局上限应为 15，实际 %d", len(got.Matches))
	}
	if !got.ResultLimitReached {
		t.Fatal("命中达到全局上限应标记截断")
	}
	// 每个成功文件的 matchesFound 之和应等于合并后的总数。
	var sum int
	for _, f := range got.Files {
		sum += f.MatchesFound
	}
	if sum != len(got.Matches) {
		t.Fatalf("文件级命中数之和 %d 与合并结果数 %d 不一致", sum, len(got.Matches))
	}
}

func TestSearchFilesConcurrentlyListsFailedFiles(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.log")
	writeFile(t, good, "ERROR here\n")
	missing := filepath.Join(dir, "gone.log") // 不存在，打开失败

	got, err := SearchFilesConcurrently(context.Background(), []string{missing, good}, "ERROR", false, 100, SearchOptions{}, nil)
	if err != nil {
		t.Fatalf("单个文件失败不应整体报错：%v", err)
	}
	if got.FilesFailed != 1 || got.FilesSucceeded != 1 {
		t.Fatalf("失败/成功计数异常：%+v", got.Files)
	}
	if !got.Files[0].Failed || got.Files[0].Error == "" {
		t.Fatalf("失败文件应记录原因：%+v", got.Files[0])
	}
	if got.Files[1].MatchesFound != 1 {
		t.Fatalf("成功文件结果应保留：%+v", got.Files[1])
	}
}

func TestSearchFilesConcurrentlyCancelKeepsCompleted(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for i := 0; i < 6; i++ {
		path := filepath.Join(dir, fmt.Sprintf("big%d.log", i))
		// 每个文件足够大，使取消能在文件之间生效。
		writeFile(t, path, strings.Repeat("ERROR needle padding padding padding\n", 4000))
		files = append(files, path)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var progressCalls int32
	got, err := SearchFilesConcurrently(ctx, files, "ERROR", false, 1<<30, SearchOptions{}, func(p MultiProgress) {
		if atomic.AddInt32(&progressCalls, 1) == 1 {
			cancel() // 第一个文件完成后取消，验证已完成结果被保留
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.FilesSucceeded < 1 {
		t.Fatal("取消前应至少完成并保留一个文件的结果")
	}
	if len(got.Matches) == 0 {
		t.Fatal("取消后应保留已完成文件的匹配")
	}
	// 不变量：成功 + 失败 + 跳过 == 总数。
	if got.FilesSucceeded+got.FilesFailed+got.FilesSkipped != got.FilesTotal {
		t.Fatalf("文件状态计数不闭合：%+v", got)
	}
	for _, m := range got.Matches {
		if m.FilePath == "" {
			t.Fatal("保留结果同样需要来源文件标注")
		}
	}
}

// TestSearchFilesConcurrentlyDataRace 在 CI 的 `go test -race` 下重点覆盖并发合并、
// 全局预算扣减与进度回调的共享状态，确保多文件编排没有数据竞争。
func TestSearchFilesConcurrentlyDataRace(t *testing.T) {
	dir := t.TempDir()
	const fileCount = 24
	files := make([]string, 0, fileCount)
	for i := 0; i < fileCount; i++ {
		path := filepath.Join(dir, fmt.Sprintf("race%d.log", i))
		var b strings.Builder
		for line := 0; line < 50; line++ {
			fmt.Fprintf(&b, "2026-01-02T10:00:%02d ERROR service=api code=%d msg=boom\n", line%60, line)
		}
		writeFile(t, path, b.String())
		files = append(files, path)
	}

	var mu sync.Mutex
	var observed int
	got, err := SearchFilesConcurrently(context.Background(), files, "code=5", true, 5000, SearchOptions{ContextLines: 2}, func(p MultiProgress) {
		mu.Lock()
		observed += len(p.Matches)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	// 断言非空、不超上限，且与增量回调送达的总数一致（并发合并无丢失/重复）。
	if len(got.Matches) == 0 {
		t.Fatal("正则多文件检索应有命中")
	}
	if len(got.Matches) > 5000 {
		t.Fatalf("超过全局上限：%d", len(got.Matches))
	}
	if observed != len(got.Matches) {
		t.Fatalf("增量回调送达数 %d 与最终结果数 %d 不一致", observed, len(got.Matches))
	}
	if got.FilesSucceeded != fileCount {
		t.Fatalf("全部文件应成功：%d/%d", got.FilesSucceeded, fileCount)
	}
}
