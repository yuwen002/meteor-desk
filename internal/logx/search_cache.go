package logx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxCacheEntries     = 128
	maxCacheBytes       = 500 << 20
	cacheMetaFilename   = "cache-index.json"
	cacheFileExt        = ".json"
	cacheLockWaitMillis = 500
)

type cacheEntry struct {
	Key         string    `json:"key"`
	Path        string    `json:"path"`
	Query       string    `json:"query"`
	UseRegex    bool      `json:"useRegex"`
	Encoding    string    `json:"encoding,omitempty"`
	FileSize    int64     `json:"fileSize"`
	FileMtime   int64     `json:"fileMtime"`
	Matches     int       `json:"matches"`
	ResultLimit bool      `json:"resultLimitReached"`
	MatchFile   string    `json:"matchFile"`
	SizeBytes   int64     `json:"sizeBytes"`
	CreatedAt   time.Time `json:"createdAt"`
	AccessedAt  time.Time `json:"accessedAt"`
}

type cacheIndex struct {
	Version int          `json:"version"`
	Entries []cacheEntry `json:"entries"`
}

type CacheEntryMeta struct {
	Path        string `json:"path"`
	Query       string `json:"query"`
	UseRegex    bool   `json:"useRegex"`
	Encoding    string `json:"encoding,omitempty"`
	FileSize    int64  `json:"fileSize"`
	FileMtime   int64  `json:"fileMtime"`
	Matches     int    `json:"matches"`
	ResultLimit bool   `json:"resultLimitReached"`
	CreatedAt   string `json:"createdAt"`
	AccessedAt  string `json:"accessedAt"`
}

var (
	cacheDirOnce sync.Once
	cacheDir     string
	cacheErr     error
	cacheMu      sync.Mutex
)

func cacheDirectory() (string, error) {
	cacheDirOnce.Do(func() {
		base, err := os.UserConfigDir()
		if err != nil {
			cacheErr = err
			return
		}
		dir := filepath.Join(base, "MeteorDesk", "search-cache")
		if err := os.MkdirAll(dir, 0700); err != nil {
			cacheErr = err
			return
		}
		cacheDir = dir
	})
	return cacheDir, cacheErr
}

func cacheIndexPath() (string, error) {
	dir, err := cacheDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, cacheMetaFilename), nil
}

func loadCacheIndex() (*cacheIndex, error) {
	p, err := cacheIndexPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return &cacheIndex{Version: 1, Entries: []cacheEntry{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var idx cacheIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	if idx.Entries == nil {
		idx.Entries = []cacheEntry{}
	}
	return &idx, nil
}

func saveCacheIndex(idx *cacheIndex) error {
	p, err := cacheIndexPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(p)
	tmp, err := os.CreateTemp(dir, ".cache-index-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, p); err != nil {
		if rmErr := os.Remove(p); rmErr == nil {
			return os.Rename(tmpName, p)
		}
		return err
	}
	return nil
}

func computeCacheKey(path, query string, useRegex bool, encoding string, fileSize, fileMtime int64, contextLines int, multiLineJSON, eventContext, timeWindowEnabled bool, startOffset, startLine int64) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%t|%s|%d|%d|%d|%t|%t|%t|%d|%d",
		path, query, useRegex, strings.ToUpper(encoding), fileSize, fileMtime,
		contextLines, multiLineJSON, eventContext, timeWindowEnabled,
		startOffset, startLine)
	return hex.EncodeToString(h.Sum(nil))
}

func cacheDataPath(key string) (string, error) {
	dir, err := cacheDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, key+cacheFileExt), nil
}

func saveCachedResponse(key string, resp SearchResponse) (int64, error) {
	data, err := json.Marshal(resp)
	if err != nil {
		return 0, err
	}
	p, err := cacheDataPath(key)
	if err != nil {
		return 0, err
	}
	dir := filepath.Dir(p)
	tmp, err := os.CreateTemp(dir, ".cache-*.tmp")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmpName, p); err != nil {
		if rmErr := os.Remove(p); rmErr == nil {
			if retryErr := os.Rename(tmpName, p); retryErr == nil {
				return int64(len(data)), nil
			}
		}
		return 0, err
	}
	return int64(len(data)), nil
}

func loadCachedResponse(key string) (SearchResponse, error) {
	p, err := cacheDataPath(key)
	if err != nil {
		return SearchResponse{}, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return SearchResponse{}, err
	}
	var resp SearchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return SearchResponse{}, err
	}
	return resp, nil
}

func lookupCacheEntry(idx *cacheIndex, key string) *cacheEntry {
	for i := range idx.Entries {
		if idx.Entries[i].Key == key {
			return &idx.Entries[i]
		}
	}
	return nil
}

func verifyFileFreshness(path string, wantSize, wantMtime int64) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.Size() != wantSize {
		return false
	}
	if info.ModTime().Unix() != wantMtime {
		return false
	}
	return true
}

func CheckCacheHit(path, query string, useRegex bool, encoding string, fileSize, fileMtime int64, contextLines int, opts SearchOptions) (bool, SearchResponse) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	idx, err := loadCacheIndex()
	if err != nil {
		return false, SearchResponse{}
	}
	key := computeCacheKey(path, query, useRegex, encoding, fileSize, fileMtime,
		contextLines, opts.MultiLineJSON, opts.EventContext, opts.TimeWindowEnabled,
		opts.StartOffset, opts.StartLine)
	entry := lookupCacheEntry(idx, key)
	if entry == nil {
		return false, SearchResponse{}
	}
	if !verifyFileFreshness(path, entry.FileSize, entry.FileMtime) {
		removeEntryLocked(idx, entry)
		_ = saveCacheIndex(idx)
		return false, SearchResponse{}
	}
	resp, err := loadCachedResponse(key)
	if err != nil {
		removeEntryLocked(idx, entry)
		_ = saveCacheIndex(idx)
		return false, SearchResponse{}
	}
	now := time.Now()
	entry.AccessedAt = now
	_ = saveCacheIndex(idx)
	return true, resp
}

func PutCachedSearch(path, query string, useRegex bool, encoding string, fileSize, fileMtime int64, contextLines int, opts SearchOptions, resp SearchResponse) error {
	if resp.Canceled {
		return nil
	}
	key := computeCacheKey(path, query, useRegex, encoding, fileSize, fileMtime,
		contextLines, opts.MultiLineJSON, opts.EventContext, opts.TimeWindowEnabled,
		opts.StartOffset, opts.StartLine)

	cacheMu.Lock()
	defer cacheMu.Unlock()

	idx, err := loadCacheIndex()
	if err != nil {
		return err
	}

	removeEntryLocked(idx, lookupCacheEntry(idx, key))

	dataSize, err := saveCachedResponse(key, resp)
	if err != nil {
		return err
	}

	now := time.Now()
	entry := cacheEntry{
		Key:         key,
		Path:        path,
		Query:       query,
		UseRegex:    useRegex,
		Encoding:    encoding,
		FileSize:    fileSize,
		FileMtime:   fileMtime,
		Matches:     len(resp.Matches),
		ResultLimit: resp.ResultLimitReached,
		MatchFile:   key + cacheFileExt,
		SizeBytes:   dataSize,
		CreatedAt:   now,
		AccessedAt:  now,
	}
	idx.Entries = append(idx.Entries, entry)

	evictLocked(idx)

	return saveCacheIndex(idx)
}

func removeEntryLocked(idx *cacheIndex, e *cacheEntry) {
	if e == nil {
		return
	}
	for i := range idx.Entries {
		if idx.Entries[i].Key == e.Key {
			idx.Entries = append(idx.Entries[:i], idx.Entries[i+1:]...)
			break
		}
	}
	if e.MatchFile != "" {
		dir, _ := cacheDirectory()
		_ = os.Remove(filepath.Join(dir, e.MatchFile))
	}
}

func evictLocked(idx *cacheIndex) {
	var total int64
	for _, e := range idx.Entries {
		total += e.SizeBytes
	}
	if len(idx.Entries) <= maxCacheEntries && total <= maxCacheBytes {
		return
	}

	sort.Slice(idx.Entries, func(i, j int) bool {
		return idx.Entries[i].AccessedAt.Before(idx.Entries[j].AccessedAt)
	})

	for len(idx.Entries) > maxCacheEntries || total > maxCacheBytes {
		if len(idx.Entries) == 0 {
			break
		}
		victim := idx.Entries[0]
		idx.Entries = idx.Entries[1:]
		total -= victim.SizeBytes
		dir, _ := cacheDirectory()
		if victim.MatchFile != "" {
			_ = os.Remove(filepath.Join(dir, victim.MatchFile))
		}
	}
}

func ListCacheEntries(limit int) []CacheEntryMeta {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	idx, err := loadCacheIndex()
	if err != nil {
		return nil
	}
	sort.Slice(idx.Entries, func(i, j int) bool {
		return idx.Entries[i].AccessedAt.After(idx.Entries[j].AccessedAt)
	})
	if limit > 0 && len(idx.Entries) > limit {
		idx.Entries = idx.Entries[:limit]
	}
	out := make([]CacheEntryMeta, 0, len(idx.Entries))
	for _, e := range idx.Entries {
		out = append(out, CacheEntryMeta{
			Path:        e.Path,
			Query:       e.Query,
			UseRegex:    e.UseRegex,
			Encoding:    e.Encoding,
			FileSize:    e.FileSize,
			FileMtime:   e.FileMtime,
			Matches:     e.Matches,
			ResultLimit: e.ResultLimit,
			CreatedAt:   e.CreatedAt.Format(time.RFC3339Nano),
			AccessedAt:  e.AccessedAt.Format(time.RFC3339Nano),
		})
	}
	return out
}

func DeleteCacheEntryByKey(key string) error {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	idx, err := loadCacheIndex()
	if err != nil {
		return err
	}
	for i := range idx.Entries {
		if idx.Entries[i].Key == key {
			dir, _ := cacheDirectory()
			if idx.Entries[i].MatchFile != "" {
				_ = os.Remove(filepath.Join(dir, idx.Entries[i].MatchFile))
			}
			idx.Entries = append(idx.Entries[:i], idx.Entries[i+1:]...)
			return saveCacheIndex(idx)
		}
	}
	return nil
}

func ClearAllCache() error {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	dir, err := cacheDirectory()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if name == cacheMetaFilename {
			continue
		}
		if strings.HasSuffix(name, cacheFileExt) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
	return saveCacheIndex(&cacheIndex{Version: 1, Entries: []cacheEntry{}})
}

type CacheStatistics struct {
	Entries    int    `json:"entries"`
	TotalBytes int64  `json:"totalBytes"`
	CacheDir   string `json:"cacheDir"`
	MaxEntries int    `json:"maxEntries"`
	MaxBytes   int64  `json:"maxBytes"`
}

func GetCacheStatistics() CacheStatistics {
	dir, _ := cacheDirectory()

	cacheMu.Lock()
	defer cacheMu.Unlock()

	idx, err := loadCacheIndex()
	if err != nil {
		return CacheStatistics{CacheDir: dir, MaxEntries: maxCacheEntries, MaxBytes: maxCacheBytes}
	}
	var total int64
	for _, e := range idx.Entries {
		total += e.SizeBytes
	}
	return CacheStatistics{
		Entries:    len(idx.Entries),
		TotalBytes: total,
		CacheDir:   dir,
		MaxEntries: maxCacheEntries,
		MaxBytes:   maxCacheBytes,
	}
}

func WarmCacheIndex() {
	_, _ = cacheDirectory()
	idx, err := loadCacheIndex()
	if err != nil || idx == nil {
		return
	}

	cleaned := make([]cacheEntry, 0, len(idx.Entries))
	for _, e := range idx.Entries {
		info, err := os.Stat(e.Path)
		if err != nil {
			continue
		}
		if info.Size() != e.FileSize || info.ModTime().Unix() != e.FileMtime {
			dir, _ := cacheDirectory()
			if e.MatchFile != "" {
				_ = os.Remove(filepath.Join(dir, e.MatchFile))
			}
			continue
		}
		cleaned = append(cleaned, e)
	}

	if len(cleaned) != len(idx.Entries) {
		idx.Entries = cleaned
		_ = saveCacheIndex(idx)
	}
}

func ComputeCacheKeyForTest(path, query string, useRegex bool, encoding string, fileSize, fileMtime int64, contextLines int, multiLineJSON, eventContext, timeWindowEnabled bool, startOffset, startLine int64) string {
	return computeCacheKey(path, query, useRegex, encoding, fileSize, fileMtime, contextLines, multiLineJSON, eventContext, timeWindowEnabled, startOffset, startLine)
}