package logx

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	// MaxMultiFiles 是单次目录/通配多文件检索允许处理的文件数量上限（阶段 6 批次 E）。
	MaxMultiFiles = 200
	// maxEnumerateVisits 限制一次目录遍历访问的条目数，避免在异常庞大的目录树上失控。
	maxEnumerateVisits = 50000
	// maxMultiFileWorkers 限制并发扫描的 goroutine 数，防止同时打开过多大文件耗尽句柄与内存。
	maxMultiFileWorkers = 8
)

// defaultLogPatterns 是未指定通配时收集日志文件的常见扩展名，与单文件选择器的过滤一致。
var defaultLogPatterns = []string{"*.log", "*.txt", "*.out", "*.json", "*.ndjson"}

// FileSearchState 记录单个文件在多文件检索中的最终处理状态，供前端展示进度与失败原因。
type FileSearchState struct {
	Path          string `json:"path"`
	BytesScanned  int64  `json:"bytesScanned"`
	FileSizeBytes int64  `json:"fileSizeBytes"`
	ScannedLines  int64  `json:"scannedLines"`
	MatchesFound  int    `json:"matchesFound"`
	Done          bool   `json:"done"`
	Failed        bool   `json:"failed"`
	// Error 是失败文件的错误原因；成功或被跳过的文件留空。
	Error string `json:"error,omitempty"`
}

// MultiProgress 汇总多文件检索的跨文件进度与自上次通知后新增的匹配。
// 与单文件 Progress 一样，Matches 只包含增量，避免反复复制整个结果集。
type MultiProgress struct {
	BytesScanned int64   `json:"bytesScanned"`
	TotalBytes   int64   `json:"fileSizeBytes"`
	ScannedLines int64   `json:"scannedLines"`
	FilesTotal   int     `json:"filesTotal"`
	FilesDone    int     `json:"filesDone"`
	FilesFailed  int     `json:"filesFailed"`
	MatchesFound int     `json:"matchesFound"`
	CurrentFile  string  `json:"currentFile,omitempty"`
	Matches      []Match `json:"matches,omitempty"`
}

// MultiSearchResponse 是一次多文件检索的完整结果。字段命名与 SearchResponse 对齐，
// 使前端可沿用同一套进度轮询与结果渲染，只额外携带文件级明细。
type MultiSearchResponse struct {
	// FilePath 承载本次检索的根目录，供「已打开位置」与导出元信息使用。
	FilePath string `json:"filePath"`
	Encoding string `json:"encoding"`
	// FileSizeBytes 是所有文件大小的合计，BytesScanned 是已处理字节合计。
	FileSizeBytes int64   `json:"fileSizeBytes"`
	BytesScanned  int64   `json:"bytesScanned"`
	ScannedLines  int64   `json:"scannedLines"`
	Matches       []Match `json:"matches"`
	// Files 按枚举顺序给出每个文件的处理状态，前端据此分组与列出失败原因。
	Files          []FileSearchState `json:"files"`
	FilesTotal     int               `json:"filesTotal"`
	FilesSucceeded int               `json:"filesSucceeded"`
	FilesFailed    int               `json:"filesFailed"`
	FilesSkipped   int               `json:"filesSkipped"`
	// FilesTruncated 表示目录中匹配文件超过 MaxMultiFiles，仅检索了排序后的前若干文件。
	FilesTruncated     bool  `json:"filesTruncated"`
	ResultLimitReached bool  `json:"resultLimitReached"`
	Canceled           bool  `json:"canceled"`
	TimeWindowSkipped  int64 `json:"timeWindowSkipped,omitempty"`
}

// normalizePatterns 清洗用户提供的通配列表：去空、转小写；为空时回退到默认日志扩展名。
func normalizePatterns(patterns []string) []string {
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		trimmed := strings.ToLower(strings.TrimSpace(pattern))
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), defaultLogPatterns...)
	}
	return out
}

// matchesAnyPattern 用文件名（基名）逐一尝试 glob 匹配；非法模式按不匹配处理，不让单个坏模式中断遍历。
func matchesAnyPattern(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if ok, err := filepath.Match(pattern, name); err == nil && ok {
			return true
		}
	}
	return false
}

// EnumerateLogFiles 递归遍历 root，收集文件名匹配任一 pattern 的普通文件，
// 按路径升序返回，最多 max 个（<=0 或超过上限时按 MaxMultiFiles）。
// 第二个返回值表示因为超过上限或访问条目数上限而截断。
func EnumerateLogFiles(root string, patterns []string, max int) ([]string, bool, error) {
	if strings.TrimSpace(root) == "" {
		return nil, false, errors.New("请先选择日志目录")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, false, fmt.Errorf("读取目录信息失败：%w", err)
	}
	if !info.IsDir() {
		return nil, false, errors.New("请选择目录而不是单个文件；单文件检索请沿用原有入口")
	}
	if max <= 0 || max > MaxMultiFiles {
		max = MaxMultiFiles
	}
	normalized := normalizePatterns(patterns)

	var collected []string
	var visits int
	hitVisitCap := false
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			// 跳过无法访问的条目（权限、竞态删除等），不让局部问题中断整体遍历。
			return nil
		}
		visits++
		if visits > maxEnumerateVisits {
			hitVisitCap = true
			return fs.SkipAll
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		if !matchesAnyPattern(strings.ToLower(entry.Name()), normalized) {
			return nil
		}
		collected = append(collected, path)
		return nil
	})
	if walkErr != nil {
		return nil, false, fmt.Errorf("遍历日志目录失败：%w", walkErr)
	}
	sort.Strings(collected)
	truncated := hitVisitCap
	if len(collected) > max {
		truncated = true
		collected = collected[:max]
	}
	return collected, truncated, nil
}

// SearchFilesConcurrently 以有界并发扫描多个文件并把结果合并成一份全局有序的结果集。
//
// 设计要点（阶段 6 批次 E）：
//   - 并发模型：min(NumCPU, maxMultiFileWorkers, 文件数) 个 goroutine 从原子索引领取文件，
//     每个文件复用成熟且自身无共享状态的 SearchFileWithOptions，避免为并发重造扫描内核。
//   - 全局上限：maxResults 作为所有文件合计的命中上限；按文件完成顺序填充剩余预算，
//     预算耗尽后停止领取新文件并把未扫描文件计为跳过，整体规模可控。
//   - 取消：ctx 取消后停止领取新文件，已扫描文件（含中途取消的部分）结果全部保留。
//   - 顺序：最终 Matches 按枚举顺序（路径升序）、文件内按行号顺序拼接，保证结果确定。
//
// onProgress 在每个文件完成时以文件粒度回调一次，携带该文件的新增匹配与跨文件聚合进度。
func SearchFilesConcurrently(ctx context.Context, files []string, query string, useRegex bool, maxResults int, options SearchOptions, onProgress func(MultiProgress)) (MultiSearchResponse, error) {
	if len(files) == 0 {
		return MultiSearchResponse{}, errors.New("没有可检索的日志文件")
	}
	if strings.TrimSpace(query) == "" {
		return MultiSearchResponse{}, errors.New("请输入搜索内容")
	}
	if len(query) > 4096 {
		return MultiSearchResponse{}, errors.New("搜索内容不能超过 4096 字节")
	}
	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}
	if maxResults > hardMaxResults {
		maxResults = hardMaxResults
	}
	// 一次性校验整体检索参数，非法正则或事件边界直接整体失败，避免每个文件重复报同一个错。
	if useRegex {
		if _, err := regexp.Compile(query); err != nil {
			return MultiSearchResponse{}, fmt.Errorf("正则表达式无效：%w", err)
		}
	}
	if options.EventContext {
		if _, err := resolveEventBoundary(options.EventBoundary, options.EventBoundaryPattern); err != nil {
			return MultiSearchResponse{}, err
		}
	}

	states := make([]FileSearchState, len(files))
	results := make([][]Match, len(files))
	var totalBytes int64
	for i, path := range files {
		states[i].Path = path
		if info, err := os.Stat(path); err == nil {
			states[i].FileSizeBytes = info.Size()
			totalBytes += info.Size()
		}
	}

	var (
		mu           sync.Mutex
		remaining    = maxResults
		aggBytes     int64
		aggLines     int64
		filesDone    int
		filesFailed  int
		collected    int
		twSkipped    int64
		limitReached bool
		canceled     bool
	)
	// 用原子索引向有限的 worker 分配文件，领取即消费，天然跳过已完成的任务。
	var nextIndex int64 = -1

	notify := func(delta []Match, current string) {
		if onProgress == nil {
			return
		}
		mu.Lock()
		progress := MultiProgress{
			BytesScanned: aggBytes,
			TotalBytes:   totalBytes,
			ScannedLines: aggLines,
			FilesTotal:   len(files),
			FilesDone:    filesDone,
			FilesFailed:  filesFailed,
			MatchesFound: collected,
			CurrentFile:  current,
		}
		mu.Unlock()
		progress.Matches = delta
		onProgress(progress)
	}

	workers := runtime.NumCPU()
	if workers > maxMultiFileWorkers {
		workers = maxMultiFileWorkers
	}
	if workers > len(files) {
		workers = len(files)
	}
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				idx := int(atomic.AddInt64(&nextIndex, 1))
				if idx >= len(files) {
					return
				}
				// 已达全局上限时不再领取新文件；这些文件在收尾时统一计为跳过。
				mu.Lock()
				stopPicking := limitReached || remaining <= 0
				mu.Unlock()
				if stopPicking {
					return
				}

				path := files[idx]
				response, err := SearchFileWithOptions(ctx, path, query, useRegex, maxResults, options, nil)

				mu.Lock()
				if response.BytesScanned > 0 {
					states[idx].BytesScanned = response.BytesScanned
				}
				if response.FileSizeBytes > 0 {
					states[idx].FileSizeBytes = response.FileSizeBytes
				}
				states[idx].ScannedLines = response.ScannedLines
				if response.Canceled {
					canceled = true
				}
				var delta []Match
				if err != nil {
					states[idx].Failed = true
					states[idx].Error = err.Error()
					filesFailed++
				} else {
					for _, match := range response.Matches {
						if remaining <= 0 {
							limitReached = true
							break
						}
						match.FilePath = path
						results[idx] = append(results[idx], match)
						remaining--
						collected++
					}
					if len(response.Matches) > len(results[idx]) {
						limitReached = true
					}
					states[idx].MatchesFound = len(results[idx])
					states[idx].Done = true
					filesDone++
					aggBytes += response.BytesScanned
					aggLines += response.ScannedLines
					twSkipped += response.TimeWindowSkipped
					delta = append([]Match(nil), results[idx]...)
				}
				mu.Unlock()

				notify(delta, path)
			}
		}()
	}
	wg.Wait()

	allMatches := make([]Match, 0, collected)
	for _, perFile := range results {
		allMatches = append(allMatches, perFile...)
	}
	return MultiSearchResponse{
		FileSizeBytes:      totalBytes,
		BytesScanned:       aggBytes,
		ScannedLines:       aggLines,
		Matches:            allMatches,
		Files:              states,
		FilesTotal:         len(files),
		FilesSucceeded:     filesDone,
		FilesFailed:        filesFailed,
		FilesSkipped:       len(files) - filesDone - filesFailed,
		ResultLimitReached: limitReached,
		Canceled:           canceled,
		TimeWindowSkipped:  twSkipped,
	}, nil
}
