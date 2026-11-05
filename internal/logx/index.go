package logx

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxCachedFiles     = 50
	maxCacheFileBytes  = 64 << 20
	indexesFile        = "file-indexes.json"
	cacheTTL           = 24 * time.Hour
)

type cachedEncoding struct {
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	Mtime       int64     `json:"mtime"`
	Encoding    string    `json:"encoding"`
	BomSkip     int64     `json:"bomSkip"`
	EncodedAt   time.Time `json:"encodedAt"`
	LastAccess  time.Time `json:"lastAccess"`
}

type encodingIndex struct {
	mu    sync.RWMutex
	items []cachedEncoding
	dirty bool
	path  string
}

var defaultEncodingIndex *encodingIndex

func init() {
	defaultEncodingIndex = &encodingIndex{}
}

func encodingCachePath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "MeteorDesk", indexesFile), nil
}

func loadEncodingIndex() *encodingIndex {
	idx := &encodingIndex{}
	p, err := encodingCachePath()
	if err != nil {
		idx.path = ""
		return idx
	}
	idx.path = p
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return idx
	}
	if err != nil || len(data) == 0 {
		return idx
	}
	var items []cachedEncoding
	if err := json.Unmarshal(data, &items); err != nil {
		return idx
	}
	idx.items = items
	idx.evictExpiredLocked()
	return idx
}

func (idx *encodingIndex) persist() error {
	idx.mu.Lock()
	if !idx.dirty {
		idx.mu.Unlock()
		return nil
	}
	items := make([]cachedEncoding, len(idx.items))
	copy(items, idx.items)
	idx.dirty = false
	idx.mu.Unlock()

	if idx.path == "" {
		p, err := encodingCachePath()
		if err != nil {
			return err
		}
		idx.path = p
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxCacheFileBytes {
		idx.shrinkToFit(maxCachedFiles / 2)
		idx.mu.RLock()
		items = make([]cachedEncoding, len(idx.items))
		copy(items, idx.items)
		idx.mu.RUnlock()
		data, _ = json.MarshalIndent(items, "", "  ")
	}
	if err := os.MkdirAll(filepath.Dir(idx.path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(idx.path), ".index-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	_ = tmp.Chmod(0600)
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
	if err := os.Rename(tmpName, idx.path); err != nil {
		if removeErr := os.Remove(idx.path); removeErr == nil {
			return os.Rename(tmpName, idx.path)
		}
		return err
	}
	return nil
}

func (idx *encodingIndex) evictExpiredLocked() {
	now := time.Now()
	alive := idx.items[:0]
	for _, item := range idx.items {
		if now.Sub(item.EncodedAt) <= cacheTTL {
			alive = append(alive, item)
		}
	}
	idx.items = alive
}

func (idx *encodingIndex) shrinkToFit(target int) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if len(idx.items) <= target {
		return
	}
	for i := 0; i < len(idx.items); i++ {
		for j := i + 1; j < len(idx.items); j++ {
			if idx.items[j].LastAccess.Before(idx.items[i].LastAccess) {
				idx.items[i], idx.items[j] = idx.items[j], idx.items[i]
			}
		}
	}
	idx.items = idx.items[:target]
	idx.dirty = true
}

func (idx *encodingIndex) Lookup(path string, size int64, mtime time.Time) (encoding string, bomSkip int64, ok bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	needle := filepath.Clean(path)
	for i := range idx.items {
		it := &idx.items[i]
		if filepath.Clean(it.Path) == needle && it.Size == size && it.Mtime == mtime.UnixNano() {
			it.LastAccess = time.Now()
			return it.Encoding, it.BomSkip, true
		}
	}
	return "", 0, false
}

func (idx *encodingIndex) Store(path string, size int64, mtime time.Time, encoding string, bomSkip int64) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	needle := filepath.Clean(path)
	for i := range idx.items {
		if filepath.Clean(idx.items[i].Path) == needle {
			idx.items[i].Size = size
			idx.items[i].Mtime = mtime.UnixNano()
			idx.items[i].Encoding = encoding
			idx.items[i].BomSkip = bomSkip
			idx.items[i].EncodedAt = time.Now()
			idx.items[i].LastAccess = time.Now()
			idx.dirty = true
			return
		}
	}
	if len(idx.items) >= maxCachedFiles {
		idx.evictExpiredLocked()
		if len(idx.items) >= maxCachedFiles {
			oldest := 0
			for i := 1; i < len(idx.items); i++ {
				if idx.items[i].LastAccess.Before(idx.items[oldest].LastAccess) {
					oldest = i
				}
			}
			idx.items = append(idx.items[:oldest], idx.items[oldest+1:]...)
		}
	}
	idx.items = append(idx.items, cachedEncoding{
		Path:       needle,
		Size:       size,
		Mtime:      mtime.UnixNano(),
		Encoding:   encoding,
		BomSkip:    bomSkip,
		EncodedAt:  time.Now(),
		LastAccess: time.Now(),
	})
	idx.dirty = true
}

func FlushEncodingIndex() {
	defaultEncodingIndex.persist()
}

func WarmEncodingCache() {
	defaultEncodingIndex = loadEncodingIndex()
}