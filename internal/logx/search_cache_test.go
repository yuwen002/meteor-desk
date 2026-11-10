package logx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func initCacheForTest(t *testing.T) {
	t.Helper()
	_, _ = cacheDirectory()
	_ = ClearAllCache()
	WarmCacheIndex()
}

func TestCachePutAndHit(t *testing.T) {
	initCacheForTest(t)

	path := filepath.Join(t.TempDir(), "cache-test.log")
	content := "alpha\nbeta hello\ngamma\nbeta world\ndelta\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	resp := SearchResponse{
		FilePath:      path,
		Encoding:      "UTF-8",
		FileSizeBytes: info.Size(),
		BytesScanned:  int64(len(content)),
		ScannedLines:  5,
		Matches: []Match{
			{LineNumber: 2, Content: "beta hello"},
			{LineNumber: 4, Content: "beta world"},
		},
	}

	opts := SearchOptions{}
	if err := PutCachedSearch(path, "beta", false, "UTF-8", info.Size(), info.ModTime().Unix(), 0, opts, resp); err != nil {
		t.Fatalf("PutCachedSearch: %v", err)
	}

	hit, cached := CheckCacheHit(path, "beta", false, "UTF-8", info.Size(), info.ModTime().Unix(), 0, opts)
	if !hit {
		t.Fatal("expected cache hit")
	}
	if len(cached.Matches) != 2 {
		t.Errorf("got %d matches, want 2", len(cached.Matches))
	}
	if cached.Matches[0].LineNumber != 2 {
		t.Errorf("first match line=%d, want 2", cached.Matches[0].LineNumber)
	}
}

func TestCacheMissOnFileChanged(t *testing.T) {
	initCacheForTest(t)

	path := filepath.Join(t.TempDir(), "cache-miss.log")
	content := "alpha\nbeta\n"
	os.WriteFile(path, []byte(content), 0644)
	info, _ := os.Stat(path)

	opts := SearchOptions{}
	resp := SearchResponse{FilePath: path, Encoding: "UTF-8", FileSizeBytes: info.Size(), BytesScanned: info.Size()}
	PutCachedSearch(path, "beta", false, "UTF-8", info.Size(), info.ModTime().Unix(), 0, opts, resp)

	time.Sleep(1100 * time.Millisecond)
	content = "alpha\nbeta\ncharlie\n"
	os.WriteFile(path, []byte(content), 0644)
	newInfo, _ := os.Stat(path)

	hit, _ := CheckCacheHit(path, "beta", false, "UTF-8", newInfo.Size(), newInfo.ModTime().Unix(), 0, opts)
	if hit {
		t.Error("should miss after file mtime/size changed")
	}
}

func TestCacheKeyDiffers(t *testing.T) {
	initCacheForTest(t)

	path := filepath.Join(t.TempDir(), "key-test.log")
	os.WriteFile(path, []byte("hello world\nfoo bar\n"), 0644)
	info, _ := os.Stat(path)
	opts := SearchOptions{}
	resp := SearchResponse{FilePath: path, Encoding: "UTF-8", FileSizeBytes: info.Size(), BytesScanned: info.Size()}

	PutCachedSearch(path, "hello", false, "UTF-8", info.Size(), info.ModTime().Unix(), 0, opts, resp)

	hit, _ := CheckCacheHit(path, "foo", false, "UTF-8", info.Size(), info.ModTime().Unix(), 0, opts)
	if hit {
		t.Error("different query should not hit")
	}

	hit, _ = CheckCacheHit(path, "hello", true, "UTF-8", info.Size(), info.ModTime().Unix(), 0, opts)
	if hit {
		t.Error("different useRegex should not hit")
	}
}

func TestCacheSkipCanceledSearch(t *testing.T) {
	initCacheForTest(t)

	path := filepath.Join(t.TempDir(), "canceled.log")
	os.WriteFile(path, []byte("a\nb\n"), 0644)
	info, _ := os.Stat(path)
	opts := SearchOptions{}
	resp := SearchResponse{FilePath: path, Encoding: "UTF-8", FileSizeBytes: info.Size(), BytesScanned: info.Size(), Canceled: true}

	if err := PutCachedSearch(path, "a", false, "UTF-8", info.Size(), info.ModTime().Unix(), 0, opts, resp); err != nil {
		t.Fatal(err)
	}
	hit, _ := CheckCacheHit(path, "a", false, "UTF-8", info.Size(), info.ModTime().Unix(), 0, opts)
	if hit {
		t.Error("canceled search should not be cached")
	}
}

func TestCacheEncodingCaseInsensitive(t *testing.T) {
	initCacheForTest(t)

	path := filepath.Join(t.TempDir(), "case.log")
	os.WriteFile(path, []byte("test\n"), 0644)
	info, _ := os.Stat(path)
	opts := SearchOptions{}
	resp := SearchResponse{FilePath: path, Encoding: "utf-8", FileSizeBytes: info.Size()}

	PutCachedSearch(path, "test", false, "utf-8", info.Size(), info.ModTime().Unix(), 0, opts, resp)

	hit, _ := CheckCacheHit(path, "test", false, "UTF-8", info.Size(), info.ModTime().Unix(), 0, opts)
	if !hit {
		t.Error("encoding uppercase query should hit lowercase-stored cache")
	}
}

func TestCacheEndToEnd(t *testing.T) {
	initCacheForTest(t)

	path := filepath.Join(t.TempDir(), "e2e.log")
	line := []byte("service=api status=200 ok\n")
	block := strings.Repeat(string(line), 1024)
	os.WriteFile(path, []byte(block), 0644)
	info, _ := os.Stat(path)

	opts := SearchOptions{}
	resp1, err := SearchFileWithOptions(context.Background(), path, "service=api", false, 5000, opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp1.Matches) == 0 {
		t.Fatal("expected matches")
	}

	if err := PutCachedSearch(path, "service=api", false, resp1.Encoding, info.Size(), info.ModTime().Unix(), 0, opts, resp1); err != nil {
		t.Fatal(err)
	}

	hit, cached := CheckCacheHit(path, "service=api", false, resp1.Encoding, info.Size(), info.ModTime().Unix(), 0, opts)
	if !hit {
		t.Fatal("expected e2e cache hit, encoding used=", resp1.Encoding)
	}
	if len(cached.Matches) != len(resp1.Matches) {
		t.Errorf("cached matches=%d, original=%d", len(cached.Matches), len(resp1.Matches))
	}
}

func TestCacheStatistics(t *testing.T) {
	initCacheForTest(t)

	stats := GetCacheStatistics()
	if stats.CacheDir == "" {
		t.Error("cache dir should not be empty")
	}
	if stats.MaxEntries != maxCacheEntries {
		t.Errorf("max entries=%d, want %d", stats.MaxEntries, maxCacheEntries)
	}
}

func TestCacheListEntries(t *testing.T) {
	initCacheForTest(t)

	path := filepath.Join(t.TempDir(), "list.log")
	os.WriteFile(path, []byte("a\nb\nc\n"), 0644)
	info, _ := os.Stat(path)
	opts := SearchOptions{}

	PutCachedSearch(path, "a", false, "UTF-8", info.Size(), info.ModTime().Unix(), 0, opts, SearchResponse{FilePath: path, Encoding: "UTF-8", FileSizeBytes: info.Size(), Matches: []Match{{LineNumber: 1, Content: "a"}}})
	PutCachedSearch(path, "b", false, "UTF-8", info.Size(), info.ModTime().Unix(), 0, opts, SearchResponse{FilePath: path, Encoding: "UTF-8", FileSizeBytes: info.Size(), Matches: []Match{{LineNumber: 2, Content: "b"}}})

	time.Sleep(200 * time.Millisecond)

	entries := ListCacheEntries(10)
	if len(entries) < 2 {
		t.Fatalf("got %d entries, want >= 2", len(entries))
	}
	t.Logf("total entries: %d", len(entries))
}

func TestCacheDeleteAndClear(t *testing.T) {
	initCacheForTest(t)

	path := filepath.Join(t.TempDir(), "del.log")
	os.WriteFile(path, []byte("x\ny\n"), 0644)
	info, _ := os.Stat(path)
	opts := SearchOptions{}

	PutCachedSearch(path, "x", false, "UTF-8", info.Size(), info.ModTime().Unix(), 0, opts, SearchResponse{FilePath: path, Encoding: "UTF-8", FileSizeBytes: info.Size()})

	if err := ClearAllCache(); err != nil {
		t.Fatal(err)
	}
	stats := GetCacheStatistics()
	if stats.Entries != 0 {
		t.Errorf("after clear: %d entries remain", stats.Entries)
	}
}

func TestCacheEvictLRU(t *testing.T) {
	initCacheForTest(t)

	path := filepath.Join(t.TempDir(), "evict.log")
	os.WriteFile(path, []byte("x\n"), 0644)
	info, _ := os.Stat(path)

	for i := 0; i < maxCacheEntries+5; i++ {
		opts := SearchOptions{}
		q := "query-" + string(rune('a'+i))
		PutCachedSearch(path, q, false, "UTF-8", info.Size(), info.ModTime().Unix(), 0, opts, SearchResponse{FilePath: path, Encoding: "UTF-8", FileSizeBytes: info.Size()})
	}

	stats := GetCacheStatistics()
	if stats.Entries > maxCacheEntries {
		t.Errorf("entries=%d exceed max=%d", stats.Entries, maxCacheEntries)
	}
}