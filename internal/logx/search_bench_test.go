package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// BenchmarkSearchFileLargeLog 默认生成 64 MiB 日志；设置 METEORDESK_BENCH_SIZE_MB=1024 可测约 1 GiB。
// 可用 go test ./internal/logx -run '^$' -bench BenchmarkSearchFileLargeLog -benchtime=1x -benchmem 执行。
func BenchmarkSearchFileLargeLog(b *testing.B) {
	sizeMB := 64
	if value := os.Getenv("METEORDESK_BENCH_SIZE_MB"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			b.Fatalf("METEORDESK_BENCH_SIZE_MB 必须是正整数")
		}
		sizeMB = parsed
	}
	path := filepath.Join(b.TempDir(), "synthetic-large.log")
	file, err := os.Create(path)
	if err != nil { b.Fatal(err) }
	line := []byte("2026-10-15T12:00:00Z INFO service=api request=abcdef123456 latency_ms=17 status=200 payload=synthetic-benchmark-data\\n")
	target := int64(sizeMB) * 1024 * 1024
	var written int64
	for written < target {
		n, writeErr := file.Write(line)
		written += int64(n)
		if writeErr != nil { b.Fatal(writeErr) }
	}
	if err := file.Close(); err != nil { b.Fatal(err) }

	b.SetBytes(written)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 使用不存在的关键字，确保每轮都扫描完整文件而非命中结果上限。
		if _, err := SearchFile(path, fmt.Sprintf("NEVER_MATCH_%d", i), false, 500); err != nil {
			b.Fatal(err)
		}
	}
}
