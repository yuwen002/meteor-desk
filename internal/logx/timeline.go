package logx

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	textencoding "golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

const (
	defaultTimelineMaxEntries = 5000
	hardTimelineMaxEntries    = 20000
	maxTimelineWorkers        = 8
	maxTimelineContentLen     = 1024
	maxTimelineLineBytes      = 4 * 1024 * 1024
)

var levelKeyword = regexp.MustCompile(`(?i)\b(fatal|panic|error|err|exception|warn(?:ing)?|info|debug|trace|notice)\b`)

type TimelineLevel string

const (
	TimelineLevelError TimelineLevel = "error"
	TimelineLevelWarn  TimelineLevel = "warn"
	TimelineLevelInfo  TimelineLevel = "info"
	TimelineLevelDebug TimelineLevel = "debug"
	TimelineLevelTrace TimelineLevel = "trace"
	TimelineLevelOther TimelineLevel = "other"
)

func AllTimelineLevels() []TimelineLevel {
	return []TimelineLevel{
		TimelineLevelError,
		TimelineLevelWarn,
		TimelineLevelInfo,
		TimelineLevelDebug,
		TimelineLevelTrace,
		TimelineLevelOther,
	}
}

func TimelineLevelPriority(level TimelineLevel) int {
	switch level {
	case TimelineLevelError:
		return 0
	case TimelineLevelWarn:
		return 1
	case TimelineLevelInfo:
		return 2
	case TimelineLevelDebug:
		return 3
	case TimelineLevelTrace:
		return 4
	}
	return 5
}

type TimelineEntry struct {
	TimestampMs    int64         `json:"timestampMs"`
	DateOnly       string        `json:"dateOnly,omitempty"`
	TimeOnly       string        `json:"timeOnly,omitempty"`
	Level          TimelineLevel `json:"level"`
	Content        string        `json:"content"`
	ContentClipped bool          `json:"contentClipped"`
	LineNumber     int64         `json:"lineNumber"`
	FilePath       string        `json:"filePath"`
	SourceLabel    string        `json:"sourceLabel,omitempty"`
}

type TimelineScanOptions struct {
	Encoding    string `json:"encoding,omitempty"`
	MaxEntries  int    `json:"maxEntries,omitempty"`
	SourceLabel string `json:"sourceLabel,omitempty"`
	Timezone    string `json:"timezone,omitempty"`
}

type TimelineFileSpec struct {
	Path        string `json:"path"`
	SourceLabel string `json:"sourceLabel,omitempty"`
}

type TimelineProgress struct {
	BytesScanned int64  `json:"bytesScanned"`
	TotalBytes   int64  `json:"totalBytes"`
	ScannedLines int64  `json:"scannedLines"`
	FilesTotal   int    `json:"filesTotal"`
	FilesDone    int    `json:"filesDone"`
	FilesFailed  int    `json:"filesFailed"`
	EntriesFound int    `json:"entriesFound"`
	CurrentFile  string `json:"currentFile,omitempty"`
}

type TimelineFileResult struct {
	Path        string `json:"path"`
	SourceLabel string `json:"sourceLabel,omitempty"`
	BytesRead   int64  `json:"bytesRead"`
	LinesRead   int64  `json:"linesRead"`
	Entries     int    `json:"entries"`
	Done        bool   `json:"done"`
	Failed      bool   `json:"failed"`
	Error       string `json:"error,omitempty"`
}

type TimelineScanResponse struct {
	Entries      []TimelineEntry      `json:"entries"`
	Files        []TimelineFileResult `json:"files"`
	TotalEntries int                  `json:"totalEntries"`
	LimitReached bool                 `json:"limitReached"`
	Canceled     bool                 `json:"canceled"`
	BytesScanned int64                `json:"bytesScanned"`
	ScannedLines int64                `json:"scannedLines"`
}

func detectLevelFromBytes(b []byte) TimelineLevel {
	idx := levelKeyword.FindIndex(b)
	if idx == nil {
		return TimelineLevelOther
	}
	matched := strings.ToLower(string(b[idx[0]:idx[1]]))
	switch matched {
	case "fatal", "panic", "error", "err", "exception":
		return TimelineLevelError
	case "warn", "warning":
		return TimelineLevelWarn
	case "info", "notice":
		return TimelineLevelInfo
	case "debug":
		return TimelineLevelDebug
	case "trace":
		return TimelineLevelTrace
	}
	return TimelineLevelOther
}

func ScanTimeline(ctx context.Context, files []TimelineFileSpec, options TimelineScanOptions, onProgress func(TimelineProgress)) (TimelineScanResponse, error) {
	if len(files) == 0 {
		return TimelineScanResponse{}, fmt.Errorf("没有可扫描的日志文件")
	}
	maxEntries := options.MaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultTimelineMaxEntries
	}
	if maxEntries > hardTimelineMaxEntries {
		maxEntries = hardTimelineMaxEntries
	}

	for i := range files {
		files[i].Path = strings.TrimSpace(files[i].Path)
		if files[i].Path == "" {
			return TimelineScanResponse{}, fmt.Errorf("存在空的文件路径")
		}
	}

	fileResults := make([]TimelineFileResult, len(files))
	for i, f := range files {
		fileResults[i].Path = f.Path
		fileResults[i].SourceLabel = f.SourceLabel
		if info, err := os.Stat(f.Path); err == nil {
			fileResults[i].BytesRead = info.Size()
		}
	}

	var totalBytes int64
	for _, fr := range fileResults {
		totalBytes += fr.BytesRead
	}

	collected := make([]TimelineEntry, 0, 256)
	var mu sync.Mutex
	var aggBytes int64
	var aggLines int64
	var filesDone int
	var filesFailed int
	var entriesCollected int64
	var limitReached atomic.Bool
	var canceled atomic.Bool
	var nextFileIdx int64 = -1

	workers := runtime.NumCPU()
	if workers > maxTimelineWorkers {
		workers = maxTimelineWorkers
	}
	if workers > len(files) {
		workers = len(files)
	}
	if workers < 1 {
		workers = 1
	}

	notifyProgress := func(currentFile string) {
		if onProgress == nil {
			return
		}
		mu.Lock()
		progress := TimelineProgress{
			BytesScanned: aggBytes,
			TotalBytes:   totalBytes,
			ScannedLines: aggLines,
			FilesTotal:   len(files),
			FilesDone:    filesDone,
			FilesFailed:  filesFailed,
			EntriesFound: int(atomic.LoadInt64(&entriesCollected)),
			CurrentFile:  currentFile,
		}
		mu.Unlock()
		onProgress(progress)
	}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if ctx.Err() != nil {
					canceled.Store(true)
					return
				}
				idx := int(atomic.AddInt64(&nextFileIdx, 1))
				if idx >= len(files) {
					return
				}
				if limitReached.Load() {
					return
				}

				spec := files[idx]
				remaining := maxEntries - int(atomic.LoadInt64(&entriesCollected))
				if remaining <= 0 {
					limitReached.Store(true)
					return
				}
				entries, scanErr, bytesRead, linesRead := scanSingleFileTimeline(ctx, spec, options, remaining)

				mu.Lock()
				fileResults[idx].BytesRead = bytesRead
				fileResults[idx].LinesRead = linesRead
				if scanErr != nil {
					fileResults[idx].Failed = true
					fileResults[idx].Error = scanErr.Error()
					filesFailed++
				} else {
					fileResults[idx].Entries = len(entries)
					fileResults[idx].Done = true
					filesDone++
					for _, e := range entries {
						if atomic.LoadInt64(&entriesCollected) >= int64(maxEntries) {
							limitReached.Store(true)
							break
						}
						collected = append(collected, e)
						atomic.AddInt64(&entriesCollected, 1)
					}
					aggBytes += bytesRead
					aggLines += linesRead
				}
				mu.Unlock()

				if scanErr == nil {
					notifyProgress(spec.Path)
				}
			}
		}()
	}
	wg.Wait()

	if atomic.LoadInt64(&entriesCollected) >= int64(maxEntries) {
		limitReached.Store(true)
	}

	sort.Slice(collected, func(i, j int) bool {
		if collected[i].TimestampMs != collected[j].TimestampMs {
			return collected[i].TimestampMs < collected[j].TimestampMs
		}
		if collected[i].FilePath != collected[j].FilePath {
			return collected[i].FilePath < collected[j].FilePath
		}
		return collected[i].LineNumber < collected[j].LineNumber
	})

	var finalBytes int64
	var finalLines int64
	for _, fr := range fileResults {
		if !fr.Failed {
			finalBytes += fr.BytesRead
			finalLines += fr.LinesRead
		}
	}

	return TimelineScanResponse{
		Entries:      collected,
		Files:        fileResults,
		TotalEntries: len(collected),
		LimitReached: limitReached.Load(),
		Canceled:     canceled.Load(),
		BytesScanned: finalBytes,
		ScannedLines: finalLines,
	}, nil
}

func scanSingleFileTimeline(ctx context.Context, spec TimelineFileSpec, options TimelineScanOptions, budget int) ([]TimelineEntry, error, int64, int64) {
	f, err := os.Open(spec.Path)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败：%w", err), 0, 0
	}
	defer f.Close()

	info, statErr := f.Stat()
	if statErr != nil {
		return nil, fmt.Errorf("读取文件信息失败：%w", statErr), 0, 0
	}

	cachedEnc, cachedBom, cacheHit := defaultEncodingIndex.Lookup(spec.Path, info.Size(), info.ModTime())

	var encodingName string
	var bomBytes int64
	var decoder textencoding.Encoding
	requestedEnc := strings.TrimSpace(options.Encoding)

	if cacheHit && (requestedEnc == "" || strings.EqualFold(requestedEnc, "auto")) {
		encodingName = cachedEnc
		bomBytes = cachedBom
		decoder = decoderForEncoding(encodingName)
	} else {
		sampleSize := min(info.Size(), int64(maxEncodingSampleBytes))
		sample := make([]byte, int(sampleSize))
		if len(sample) > 0 {
			n, sampleErr := f.ReadAt(sample, 0)
			if sampleErr != nil && sampleErr != io.EOF {
				return nil, fmt.Errorf("读取编码样本失败：%w", sampleErr), info.Size(), 0
			}
			sample = sample[:n]
		}
		var encErr error
		encodingName, bomBytes, decoder, encErr = resolveTextEncoding(sample, requestedEnc, int64(len(sample)) == info.Size())
		if encErr != nil {
			return nil, encErr, info.Size(), 0
		}
		defaultEncodingIndex.Store(spec.Path, info.Size(), info.ModTime(), encodingName, bomBytes)
	}

	if _, err := f.Seek(bomBytes, io.SeekStart); err != nil {
		return nil, fmt.Errorf("定位文件正文失败：%w", err), info.Size(), 0
	}

	var scanInput io.Reader = f
	if decoder != nil {
		scanInput = transform.NewReader(f, decoder.NewDecoder())
	}

	reader := bufio.NewReaderSize(scanInput, readerBufferSize)

	var lineNumber int64
	var totalBytes int64
	entries := make([]TimelineEntry, 0, 64)

	for {
		if ctx.Err() != nil {
			break
		}
		raw, readErr := reader.ReadBytes('\n')
		if len(raw) > 0 {
			totalBytes += int64(len(raw))
			lineNumber++

			if len(raw) > 0 && raw[len(raw)-1] == '\n' {
				raw = raw[:len(raw)-1]
				totalBytes++
			}
			if len(raw) > 0 && raw[len(raw)-1] == '\r' {
				raw = raw[:len(raw)-1]
			}

			if len(raw) == 0 {
				continue
			}

			if len(raw) > maxTimelineLineBytes {
				raw = raw[:maxTimelineLineBytes]
			}

			content := string(raw)
			var clipped bool
			if len(content) > maxTimelineContentLen {
				content = content[:maxTimelineContentLen]
				clipped = true
			}

			tsMs, _ := parseLeadingTimestampMillis(raw, timeLocalOr(options.Timezone))
			level := detectLevelFromBytes(raw)

			entries = append(entries, TimelineEntry{
				TimestampMs:    tsMs,
				Level:          level,
				Content:        content,
				ContentClipped: clipped,
				LineNumber:     lineNumber,
				FilePath:       spec.Path,
				SourceLabel:    spec.SourceLabel,
			})

			if budget > 0 && len(entries) >= budget {
				break
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return entries, readErr, totalBytes, lineNumber
		}
	}

	return entries, nil, totalBytes, lineNumber
}

func timeLocalOr(tz string) *time.Location {
	switch strings.ToLower(strings.TrimSpace(tz)) {
	case "utc":
		return time.UTC
	case "local", "":
		return time.Local
	}
	return time.Local
}

func FormatTimelineTimestamp(ms int64, tz *time.Location) string {
	if ms <= 0 {
		return ""
	}
	if tz == nil {
		tz = time.Local
	}
	return time.UnixMilli(ms).In(tz).Format("2006-01-02 15:04:05")
}

func FormatTimelineTimeOnly(ms int64, tz *time.Location) string {
	if ms <= 0 {
		return ""
	}
	if tz == nil {
		tz = time.Local
	}
	return time.UnixMilli(ms).In(tz).Format("15:04:05")
}

func FormatTimelineDateOnly(ms int64, tz *time.Location) string {
	if ms <= 0 {
		return ""
	}
	if tz == nil {
		tz = time.Local
	}
	return time.UnixMilli(ms).In(tz).Format("2006-01-02")
}