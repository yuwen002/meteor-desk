package logx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	parallelChunkBytes    = 64 * 1024 * 1024
	parallelMinFileBytes  = 64 * 1024 * 1024
	parallelMaxWorkers    = 8
	parallelReaderBufSize = 256 * 1024
)

type chunkMatches struct {
	chunkIndex   int
	chunkStart   int64
	chunkEnd     int64
	matches      []Match
	linesBefore  int64
	scannedLines int64
	bytesScanned int64
	err          error
}

func isParallelizable(path string, useRegex bool, options SearchOptions, hasDecoder bool, fileSize int64) bool {
	_ = path
	if useRegex || hasDecoder {
		return false
	}
	if options.MultiLineJSON || options.EventContext {
		return false
	}
	if options.TimeWindowEnabled {
		return false
	}
	if options.StartOffset > 0 || options.StartLine > 0 {
		return false
	}
	if fileSize < parallelMinFileBytes {
		return false
	}
	return true
}

func countLinesBefore(path string, offset int64, bomBytes int64) (int64, error) {
	if offset <= bomBytes {
		return 0, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Seek(bomBytes, io.SeekStart); err != nil {
		return 0, err
	}
	var total int64
	remaining := offset - bomBytes
	buf := make([]byte, 64*1024)
	for remaining > 0 {
		readSize := int64(len(buf))
		if readSize > remaining {
			readSize = remaining
		}
		n, err := file.Read(buf[:readSize])
		if n > 0 {
			remaining -= int64(n)
			total += int64(bytes.Count(buf[:n], []byte{'\n'}))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

func scanChunk(ctx context.Context, path string, query []byte, maxResults int, chunkStart, chunkEnd, bomBytes int64, linesBefore int64, isLastChunk bool) chunkMatches {
	cm := chunkMatches{chunkStart: chunkStart, chunkEnd: chunkEnd, linesBefore: linesBefore}

	file, err := os.Open(path)
	if err != nil {
		cm.err = err
		return cm
	}
	defer func() { _ = file.Close() }()

	actualStart := chunkStart
	if chunkStart > bomBytes {
		if _, err := file.Seek(chunkStart, io.SeekStart); err != nil {
			cm.err = err
			return cm
		}
		bufFile := bufio.NewReader(file)
		b, readErr := bufFile.ReadByte()
		if readErr != nil && readErr != io.EOF {
			cm.err = readErr
			return cm
		}
		if b == '\n' {
			actualStart = chunkStart + 1
		} else {
			for {
				b, readErr := bufFile.ReadByte()
				if readErr != nil {
					break
				}
				actualStart++
				if b == '\n' {
					break
				}
			}
		}
	}

	if actualStart >= chunkEnd {
		return cm
	}

	if _, err := file.Seek(actualStart, io.SeekStart); err != nil {
		cm.err = err
		return cm
	}

	reader := bufio.NewReaderSize(file, parallelReaderBufSize)
	var lineNumber int64 = linesBefore
	var bytesRead int64 = 0
	targetBytes := chunkEnd - actualStart
	var chunkExpanded bool

	var preview []byte
	var carry []byte
	var pendingCR bool
	var lineOpen bool
	var lineMatched bool
	var lineBytes int64

	appendContent := func(part []byte) {
		if len(part) == 0 {
			return
		}
		lineOpen = true
		lineBytes += int64(len(part))
		if len(preview) < maxPreviewBytes {
			remaining := maxPreviewBytes - len(preview)
			if remaining > len(part) {
				remaining = len(part)
			}
			preview = append(preview, part[:remaining]...)
		}
		if !lineMatched {
			if bytes.Contains(part, query) {
				lineMatched = true
			} else {
				keep := len(query) - 1
				prefixLen := min(len(part), keep)
				if keep > 0 && len(carry) > 0 && prefixLen > 0 {
					cross := make([]byte, 0, len(carry)+prefixLen)
					cross = append(cross, carry...)
					cross = append(cross, part[:prefixLen]...)
					lineMatched = bytes.Contains(cross, query)
				}
				if !lineMatched && keep > 0 {
					if len(part) >= keep {
						carry = append(carry[:0], part[len(part)-keep:]...)
					} else {
						needed := keep - len(part)
						if needed > len(carry) {
							needed = len(carry)
						}
						tail := append([]byte(nil), carry[len(carry)-needed:]...)
						carry = append(tail, part...)
					}
				}
			}
		}
	}

	finishLine := func() {
		lineNumber++
		cm.scannedLines = lineNumber - linesBefore
		if lineMatched {
			content := strings.ToValidUTF8(string(preview), "�")
			clipped := lineBytes > maxPreviewBytes
			cm.matches = append(cm.matches, Match{
				LineNumber:     lineNumber,
				Content:        content,
				ContentClipped: clipped,
			})
		}
		lineBytes = 0
		preview = preview[:0]
		carry = carry[:0]
		lineMatched = false
		lineOpen = false
		pendingCR = false
	}

	for {
		if ctx.Err() != nil {
			cm.err = ctx.Err()
			cm.bytesScanned = bytesRead
			return cm
		}
		if maxResults > 0 && len(cm.matches) >= maxResults {
			cm.bytesScanned = bytesRead
			return cm
		}
		if isLastChunk && bytesRead >= targetBytes {
			if lineOpen {
				finishLine()
			}
			cm.bytesScanned = bytesRead
			return cm
		}

		chunk, readErr := reader.ReadSlice('\n')
		n := int64(len(chunk))
		bytesRead += n

		hasNewline := len(chunk) > 0 && chunk[len(chunk)-1] == '\n'
		part := chunk
		if hasNewline {
			part = part[:len(part)-1]
		}
		if hasNewline && len(part) > 0 && part[len(part)-1] == '\r' {
			part = part[:len(part)-1]
		}

		if pendingCR {
			if !hasNewline || len(part) > 0 {
				appendContent([]byte{'\r'})
			}
			pendingCR = false
		}
		if !hasNewline && len(part) > 0 && part[len(part)-1] == '\r' {
			part = part[:len(part)-1]
			pendingCR = true
			lineOpen = true
		}
		if len(part) > 0 {
			appendContent(part)
		}

		if hasNewline {
			finishLine()
			if !isLastChunk && !chunkExpanded && bytesRead >= targetBytes {
				chunkExpanded = true
				cm.bytesScanned = bytesRead
				return cm
			}
		}

		if readErr == bufio.ErrBufferFull {
			continue
		}
		if readErr == io.EOF {
			if lineOpen {
				finishLine()
			}
			cm.bytesScanned = bytesRead
			return cm
		}
		if readErr != nil {
			cm.err = fmt.Errorf("读取日志文件失败：%w", readErr)
			cm.bytesScanned = bytesRead
			return cm
		}
	}
}

func parallelSearchFile(ctx context.Context, path string, query string, maxResults int, encodingName string, fileSize int64, bomBytes int64, options SearchOptions, onProgress func(Progress)) (SearchResponse, error) {
	needle := []byte(query)

	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}
	if maxResults > hardMaxResults {
		maxResults = hardMaxResults
	}

	workerCount := runtime.NumCPU()
	if workerCount > parallelMaxWorkers {
		workerCount = parallelMaxWorkers
	}

	type chunkTask struct {
		index       int
		start       int64
		end         int64
		linesBefore int64
		isLast      bool
	}

	tasks := make([]chunkTask, 0)
	var offset int64 = bomBytes
	var idx int
	for offset < fileSize {
		end := offset + parallelChunkBytes
		if end > fileSize {
			end = fileSize
		}
		lines, err := countLinesBefore(path, offset, bomBytes)
		if err != nil {
			return SearchResponse{}, err
		}
		isLast := end == fileSize
		tasks = append(tasks, chunkTask{index: idx, start: offset, end: end, linesBefore: lines, isLast: isLast})
		offset = end
		idx++
	}
	if len(tasks) == 0 {
		tasks = append(tasks, chunkTask{index: 0, start: bomBytes, end: fileSize, linesBefore: 0, isLast: true})
	}

	results := make([]chunkMatches, len(tasks))

	var totalMatchesAtomic int64
	var aggBytesAtomic int64
	var aggLinesAtomic int64
	var canceled atomic.Bool
	var limitReached atomic.Bool

	var (
		progressMu   sync.Mutex
		notifiedBytes int64
	)

	maxPerChunk := maxResults / workerCount
	if maxPerChunk < maxResults {
		maxPerChunk = maxResults
	}

	taskCh := make(chan chunkTask, len(tasks))
	for _, t := range tasks {
		taskCh <- t
	}
	close(taskCh)

	var wg sync.WaitGroup
	for w := 0; w < workerCount && w < len(tasks); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskCh {
				if canceled.Load() {
					return
				}
				if ctx.Err() != nil {
					canceled.Store(true)
					return
				}
				if limitReached.Load() {
					return
				}

				result := scanChunk(ctx, path, needle, maxPerChunk, task.start, task.end, bomBytes, task.linesBefore, task.isLast)
				result.chunkIndex = task.index
				results[task.index] = result

				if result.err != nil {
					if errors.Is(result.err, context.Canceled) {
						canceled.Store(true)
						continue
					}
					continue
				}

				currentMatches := atomic.LoadInt64(&totalMatchesAtomic)
				if maxResults > 0 && currentMatches >= int64(maxResults) {
					limitReached.Store(true)
					continue
				}
				if int64(len(result.matches)) > int64(maxResults)-currentMatches {
					allowed := int(maxResults) - int(currentMatches)
					if allowed > 0 {
						result.matches = result.matches[:allowed]
						results[task.index] = result
					}
				}

				atomic.AddInt64(&totalMatchesAtomic, int64(len(result.matches)))
				atomic.AddInt64(&aggBytesAtomic, result.bytesScanned)
				atomic.AddInt64(&aggLinesAtomic, result.scannedLines)

				progressMu.Lock()
				if onProgress != nil {
					scanned := atomic.LoadInt64(&aggBytesAtomic)
					if scanned > fileSize {
						scanned = fileSize
					}
					if scanned-notifiedBytes >= progressStepBytes || scanned >= fileSize {
						notifiedBytes = scanned
						onProgress(Progress{
							BytesScanned:  scanned,
							FileSizeBytes: fileSize,
							ScannedLines:  atomic.LoadInt64(&aggLinesAtomic),
							MatchesFound:  int(atomic.LoadInt64(&totalMatchesAtomic)),
							Matches:       result.matches,
						})
					}
				}
				progressMu.Unlock()

				if maxResults > 0 && atomic.LoadInt64(&totalMatchesAtomic) >= int64(maxResults) {
					limitReached.Store(true)
				}
			}
		}()
	}
	wg.Wait()

	allMatches := make([]Match, 0)
	var totalBytes int64
	var totalLines int64
	for _, r := range results {
		if r.err == nil {
			allMatches = append(allMatches, r.matches...)
			totalBytes += r.bytesScanned
			totalLines += r.scannedLines
		}
	}

	if maxResults > 0 && len(allMatches) > maxResults {
		allMatches = allMatches[:maxResults]
		limitReached.Store(true)
	}

	sort.Slice(allMatches, func(i, j int) bool { return allMatches[i].LineNumber < allMatches[j].LineNumber })

	contextLines := options.ContextLines
	if contextLines > 0 && len(allMatches) > 0 {
		if fillErr := fillMatchContexts(path, allMatches, contextLines); fillErr != nil {
			return SearchResponse{}, fillErr
		}
	}

	if totalBytes > fileSize {
		totalBytes = fileSize
	}

	response := SearchResponse{
		FilePath:      path,
		Encoding:      encodingName,
		FileSizeBytes: fileSize,
		BytesScanned:  totalBytes,
		ScannedLines:  totalLines,
		Matches:       allMatches,
		ResumeOffset:  -1,
		ResumeLine:    0,
	}

	if limitReached.Load() {
		response.ResultLimitReached = true
	}
	if canceled.Load() {
		response.Canceled = true
	}

	if onProgress != nil {
		onProgress(Progress{
			BytesScanned:  totalBytes,
			FileSizeBytes: fileSize,
			ScannedLines:  totalLines,
			MatchesFound:  len(allMatches),
			Matches:       []Match{},
		})
	}

	return response, nil
}

func fillMatchContexts(path string, matches []Match, contextLines int) error {
	if contextLines <= 0 || len(matches) == 0 {
		return nil
	}

	matchedLines := make(map[int64]int, len(matches))
	for i, m := range matches {
		matchedLines[m.LineNumber] = i
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	reader := bufio.NewReaderSize(file, parallelReaderBufSize)
	var lineNumber int64
	history := make([]LineContext, 0, contextLines)
	pendingContext := make([]int, 0, 16)
	pendingCounts := make([]int, 0, 16)

	var preview []byte
	var lineBytes int64
	var lineOpen bool
	var pendingCR bool

	appendContent := func(part []byte) {
		if len(part) == 0 {
			return
		}
		lineOpen = true
		lineBytes += int64(len(part))
		if len(preview) < maxPreviewBytes+maxContextBytes {
			remaining := maxPreviewBytes + maxContextBytes - len(preview)
			if remaining > len(part) {
				remaining = len(part)
			}
			preview = append(preview, part[:remaining]...)
		}
	}

	finishLine := func() {
		lineNumber++
		content := strings.ToValidUTF8(string(preview), "")
		clipped := lineBytes > maxPreviewBytes+maxContextBytes
		currentLine := LineContext{LineNumber: lineNumber, Content: content, ContentClipped: clipped}

		if mi, found := matchedLines[lineNumber]; found {
			matches[mi].ContextBefore = append([]LineContext(nil), history...)
			pendingContext = append(pendingContext, mi)
			pendingCounts = append(pendingCounts, 0)
		}

		for idx := len(pendingContext) - 1; idx >= 0; idx-- {
			mi := pendingContext[idx]
			matches[mi].ContextAfter = append(matches[mi].ContextAfter, currentLine)
			pendingCounts[idx]++
			if pendingCounts[idx] >= contextLines {
				pendingContext = append(pendingContext[:idx], pendingContext[idx+1:]...)
				pendingCounts = append(pendingCounts[:idx], pendingCounts[idx+1:]...)
			}
		}

		history = append(history, currentLine)
		if len(history) > contextLines {
			history = append(history[:0], history[len(history)-contextLines:]...)
		}

		lineBytes = 0
		preview = preview[:0]
		lineOpen = false
		pendingCR = false
	}

	for {
		chunk, readErr := reader.ReadSlice('\n')
		n := int64(len(chunk))
		_ = n

		hasNewline := len(chunk) > 0 && chunk[len(chunk)-1] == '\n'
		part := chunk
		if hasNewline {
			part = part[:len(part)-1]
		}
		if hasNewline && len(part) > 0 && part[len(part)-1] == '\r' {
			part = part[:len(part)-1]
		}

		if pendingCR {
			if !hasNewline || len(part) > 0 {
				appendContent([]byte{'\r'})
			}
			pendingCR = false
		}
		if !hasNewline && len(part) > 0 && part[len(part)-1] == '\r' {
			part = part[:len(part)-1]
			pendingCR = true
			lineOpen = true
		}
		if len(part) > 0 {
			appendContent(part)
		}

		if hasNewline {
			finishLine()
		}

		if readErr == bufio.ErrBufferFull {
			continue
		}
		if readErr == io.EOF {
			if lineOpen {
				finishLine()
			}
			break
		}
		if readErr != nil {
			return fmt.Errorf("读取日志文件失败：%w", readErr)
		}
	}

	return nil
}