package logx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupIndex(t *testing.T) (*encodingIndex, time.Time) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, indexesFile)
	idx := &encodingIndex{path: p}
	return idx, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
}

func TestEncodingIndexStoreAndLookup(t *testing.T) {
	idx, mtime := setupIndex(t)

	if _, _, ok := idx.Lookup("C:\\logs\\app.log", 12345, mtime); ok {
		t.Fatal("expected miss on empty cache")
	}

	idx.Store("C:\\logs\\app.log", 12345, mtime, "UTF-8", 0)

	enc, bom, ok := idx.Lookup("C:\\logs\\app.log", 12345, mtime)
	if !ok {
		t.Fatal("expected hit after store")
	}
	if enc != "UTF-8" || bom != 0 {
		t.Fatalf("encoding mismatch: got %q bom=%d", enc, bom)
	}
}

func TestEncodingIndexMismatchMisses(t *testing.T) {
	idx, mtime := setupIndex(t)
	idx.Store("/var/log/app.log", 1000, mtime, "GB18030", 0)

	if _, _, ok := idx.Lookup("/var/log/app.log", 2000, mtime); ok {
		t.Fatal("size change should miss")
	}
	if _, _, ok := idx.Lookup("/var/log/app.log", 1000, mtime.Add(time.Minute)); ok {
		t.Fatal("mtime change should miss")
	}
	if _, _, ok := idx.Lookup("/var/log/other.log", 1000, mtime); ok {
		t.Fatal("different path should miss")
	}
}

func TestEncodingIndexUpdateExisting(t *testing.T) {
	idx, mtime1 := setupIndex(t)
	mtime2 := mtime1.Add(time.Hour)

	idx.Store("app.log", 1000, mtime1, "UTF-8", 0)
	idx.Store("app.log", 1500, mtime2, "UTF-16LE", 2)

	enc, bom, ok := idx.Lookup("app.log", 1500, mtime2)
	if !ok || enc != "UTF-16LE" || bom != 2 {
		t.Fatalf("update failed: got ok=%v enc=%q bom=%d", ok, enc, bom)
	}

	idx.mu.RLock()
	count := len(idx.items)
	idx.mu.RUnlock()
	if count != 1 {
		t.Fatalf("expected 1 entry after update, got %d", count)
	}
}

func TestEncodingIndexEvictionAtCap(t *testing.T) {
	idx, mtime := setupIndex(t)

	for i := 0; i < maxCachedFiles+5; i++ {
		path := filepath.Join("C:\\logs", "file"+string(rune('A'+i))+".log")
		idx.Store(path, int64(100+i), mtime.Add(time.Duration(i)*time.Second), "UTF-8", 0)
	}

	idx.mu.RLock()
	count := len(idx.items)
	idx.mu.RUnlock()
	if count > maxCachedFiles {
		t.Fatalf("cache grew beyond max: %d", count)
	}
}

func TestEncodingIndexEvictExpired(t *testing.T) {
	idx, _ := setupIndex(t)
	oldNow := time.Now().Add(-cacheTTL - time.Hour)
	freshNow := time.Now()

	idx.items = []cachedEncoding{
		{Path: "old.log", Size: 100, Mtime: 1, Encoding: "UTF-8", EncodedAt: oldNow, LastAccess: oldNow},
		{Path: "fresh.log", Size: 200, Mtime: 2, Encoding: "GB18030", EncodedAt: freshNow, LastAccess: freshNow},
	}

	idx.evictExpiredLocked()

	idx.mu.RLock()
	count := len(idx.items)
	idx.mu.RUnlock()
	if count != 1 {
		t.Fatalf("expected 1 fresh entry, got %d", count)
	}
}

func TestEncodingIndexPersistAndReload(t *testing.T) {
	idx, mtime := setupIndex(t)
	idx.Store("C:\\data\\events.ndjson", 50000, mtime, "UTF-8", 3)

	if err := idx.persist(); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	data, err := os.ReadFile(idx.path)
	if err != nil {
		t.Fatalf("cannot read persisted file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("persisted file is empty")
	}

	var loaded []cachedEncoding
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("json load failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 item loaded, got %d", len(loaded))
	}
	if loaded[0].Encoding != "UTF-8" || loaded[0].BomSkip != 3 {
		t.Fatalf("persisted content mismatch: %+v", loaded[0])
	}
}

func TestEncodingIndexPathNormalization(t *testing.T) {
	idx, mtime := setupIndex(t)

	idx.Store("logs/../logs/app.log", 1000, mtime, "UTF-8", 0)

	enc, _, ok := idx.Lookup(filepath.Clean("logs/app.log"), 1000, mtime)
	if !ok {
		t.Fatal("should match after filepath.Clean normalization")
	}
	if enc != "UTF-8" {
		t.Fatalf("got wrong encoding: %q", enc)
	}
}

func TestEncodingIndexConcurrentAccess(t *testing.T) {
	idx, mtime := setupIndex(t)

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				path := filepath.Join("C:\\logs", "file.log")
				idx.Lookup(path, int64(n*100+j), mtime)
				idx.Store(path, int64(n*100+j), mtime, "UTF-8", 0)
			}
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
