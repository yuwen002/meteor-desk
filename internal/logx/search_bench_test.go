package logx

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// 默认扫描约 64 MiB；设置 METEORDESK_BENCH_SIZE_MB=1024 可测约 1 GiB。
// 建文件阶段按 1 MiB 块写入，避免大量小 I/O 影响基准准备时间。
func BenchmarkSearchFileLargeLog(b *testing.B) {
	sizeMB := 64
	if value := os.Getenv("METEORDESK_BENCH_SIZE_MB"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 { b.Fatal("METEORDESK_BENCH_SIZE_MB 必须是正整数") }
		sizeMB = parsed
	}
	path := filepath.Join(b.TempDir(), "synthetic-large.log")
	file, err := os.Create(path)
	if err != nil { b.Fatal(err) }
	line := []byte("2026-10-15T12:00:00Z INFO service=api request=abcdef123456 latency_ms=17 status=200 payload=synthetic-benchmark-data\n")
	target := int64(sizeMB) * 1024 * 1024
	block := bytes.Repeat(line, (1024*1024)/len(line))
	var written int64
	for written < target {
		chunkSize := len(block)
		if int64(chunkSize) > target-written { chunkSize = int(target-written) }
		n, writeErr := file.Write(block[:chunkSize])
		written += int64(n)
		if writeErr != nil { b.Fatal(writeErr) }
	}
	if err := file.Close(); err != nil { b.Fatal(err) }
	b.SetBytes(written)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := SearchFile(path, "NEVER_MATCH", false, 500); err != nil { b.Fatal(err) }
	}
}
