package logx

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func buildLargeTestLog(t *testing.T, sizeMB int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "parallel.log")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	line := []byte("2026-11-09T12:00:00Z INFO service=api request=abcdef123456 latency_ms=17 status=200 payload=data-payload-here\n")
	block := bytes.Repeat(line, (1024*1024)/len(line))
	target := int64(sizeMB) * 1024 * 1024
	var written int64
	for written < target {
		chunk := len(block)
		if int64(chunk) > target-written {
			chunk = int(target - written)
		}
		n, writeErr := file.Write(block[:chunk])
		written += int64(n)
		if writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParallelSearchMatchesSerial(t *testing.T) {
	path := buildLargeTestLog(t, 96)

	serial, err := SearchFile(path, "service=api", false, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if len(serial.Matches) == 0 {
		t.Fatal("serial search returned zero matches for known pattern")
	}

	file, _ := os.Open(path)
	info, _ := file.Stat()
	_ = file.Close()

	var progress atomic.Int64
	onProgress := func(p Progress) {
		progress.Store(p.BytesScanned)
	}
	parallel, err := parallelSearchFile(context.Background(), path, "service=api", 5000, "UTF-8", info.Size(), 0, SearchOptions{}, onProgress)
	if err != nil {
		t.Fatal(err)
	}

	if len(parallel.Matches) != len(serial.Matches) {
		t.Errorf("match count mismatch: parallel=%d serial=%d", len(parallel.Matches), len(serial.Matches))
	}

	for i := 0; i < min(len(parallel.Matches), len(serial.Matches)); i++ {
		if parallel.Matches[i].LineNumber != serial.Matches[i].LineNumber {
			t.Errorf("line mismatch at index %d: parallel=%d serial=%d", i, parallel.Matches[i].LineNumber, serial.Matches[i].LineNumber)
		}
		if parallel.Matches[i].Content != serial.Matches[i].Content {
			t.Errorf("content mismatch at index %d", i)
		}
	}
}

func TestParallelSearchRespectsMaxResults(t *testing.T) {
	path := buildLargeTestLog(t, 96)

	file, _ := os.Open(path)
	info, _ := file.Stat()
	_ = file.Close()

	resp, err := parallelSearchFile(context.Background(), path, "service=api", 3, "UTF-8", info.Size(), 0, SearchOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Matches) != 3 {
		t.Errorf("expected 3 matches, got %d", len(resp.Matches))
	}
	if !resp.ResultLimitReached {
		t.Error("expected ResultLimitReached=true when maxResults hit")
	}
}

func TestParallelSearchCancel(t *testing.T) {
	path := buildLargeTestLog(t, 96)

	file, _ := os.Open(path)
	info, _ := file.Stat()
	_ = file.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := parallelSearchFile(ctx, path, "service=api", 5000, "UTF-8", info.Size(), 0, SearchOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Canceled {
		t.Error("expected Canceled=true on context cancellation")
	}
}

func TestParallelSearchSkipsSmallFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.log")
	content := "line one\nline two service=api\nline three\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	serial, err := SearchFile(path, "service=api", false, 5000)
	if err != nil {
		t.Fatal(err)
	}

	file, _ := os.Open(path)
	info, _ := file.Stat()
	_ = file.Close()

	resp, err := parallelSearchFile(context.Background(), path, "service=api", 5000, "UTF-8", info.Size(), 0, SearchOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Matches) != len(serial.Matches) {
		t.Errorf("small file match count mismatch: parallel=%d serial=%d", len(resp.Matches), len(serial.Matches))
	}
}

func TestIsParallelizableGate(t *testing.T) {
	fileSize := int64(100 * 1024 * 1024)

	if !isParallelizable("x.log", false, SearchOptions{}, false, fileSize) {
		t.Error("large utf-8 file with plain text should be parallelizable")
	}
	if isParallelizable("x.log", true, SearchOptions{}, false, fileSize) {
		t.Error("regex should block parallelism")
	}
	if isParallelizable("x.log", false, SearchOptions{}, true, fileSize) {
		t.Error("decoder (utf-16/gb18030) should block parallelism")
	}
	if isParallelizable("x.log", false, SearchOptions{MultiLineJSON: true}, false, fileSize) {
		t.Error("MultiLineJSON should block parallelism")
	}
	if isParallelizable("x.log", false, SearchOptions{EventContext: true}, false, fileSize) {
		t.Error("EventContext should block parallelism")
	}
	if !isParallelizable("x.log", false, SearchOptions{ContextLines: 3}, false, fileSize) {
		t.Error("ContextLines>0 should now be parallelizable (context filled post-merge)")
	}
	if isParallelizable("x.log", false, SearchOptions{TimeWindowEnabled: true}, false, fileSize) {
		t.Error("TimeWindow should block parallelism")
	}
	if isParallelizable("x.log", false, SearchOptions{}, false, 32*1024*1024) {
		t.Error("small file should block parallelism")
	}
	if !isParallelizable("x.log", false, SearchOptions{ContextLines: 0}, false, fileSize) {
		t.Error("ContextLines=0 should not block parallelism")
	}
}

func TestParallelSearchProgressCallback(t *testing.T) {
	path := buildLargeTestLog(t, 96)

	file, _ := os.Open(path)
	info, _ := file.Stat()
	_ = file.Close()

	var called int32
	var lastBytes int64
	onProgress := func(p Progress) {
		atomic.AddInt32(&called, 1)
		lastBytes = p.BytesScanned
	}

	_, err := parallelSearchFile(context.Background(), path, "NEVER_MATCH", 5000, "UTF-8", info.Size(), 0, SearchOptions{}, onProgress)
	if err != nil {
		t.Fatal(err)
	}

	if called <= 1 {
		t.Error("expected multiple progress callbacks")
	}
	if lastBytes != info.Size() {
		t.Errorf("final progress bytes=%d, want %d", lastBytes, info.Size())
	}
}

func TestParallelSearchProgressFromSearchFileWithOptions(t *testing.T) {
	path := buildLargeTestLog(t, 96)

	var totalMatches int
	onProgress := func(p Progress) {
		totalMatches += len(p.Matches)
	}
	resp, err := SearchFileWithOptions(context.Background(), path, "service=api", false, 5000, SearchOptions{}, onProgress)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Matches) == 0 {
		t.Fatal("expected matches")
	}

	serial, _ := SearchFile(path, "service=api", false, 5000)
	if len(resp.Matches) != len(serial.Matches) {
		t.Errorf("parallel through SearchFileWithOptions: got %d matches, serial got %d", len(resp.Matches), len(serial.Matches))
	}
}

func TestCountLinesBefore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lines.log")
	content := "alpha\nbeta\ngamma\ndelta\nepsilon\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	lines, err := countLinesBefore(path, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if lines != 0 {
		t.Errorf("offset=0 expected 0 lines, got %d", lines)
	}

	lines, err = countLinesBefore(path, int64(len(content)), 0)
	if err != nil {
		t.Fatal(err)
	}
	if lines != 5 {
		t.Errorf("offset=end expected 5 lines, got %d", lines)
	}
}