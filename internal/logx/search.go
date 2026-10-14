// Package logx 提供适用于大体积日志文件的流式检索能力。
package logx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	defaultMaxResults = 500
	hardMaxResults    = 5000
	maxPreviewBytes   = 8 * 1024
	readerBufferSize  = 256 * 1024
	progressStepBytes = 256 * 1024
	// 正则表达式需要按整行匹配，因此为单行设置内存上限。
	maxRegexLineBytes = 4 * 1024 * 1024
)

// Match 表示一条匹配结果。超长日志行只返回开头的有限预览。
type Match struct {
	LineNumber     int64  `json:"lineNumber"`
	Content        string `json:"content"`
	ContentClipped bool   `json:"contentClipped"`
}

// Progress 描述扫描进度，供桌面界面轮询展示。
type Progress struct {
	BytesScanned  int64 `json:"bytesScanned"`
	FileSizeBytes int64 `json:"fileSizeBytes"`
	ScannedLines  int64 `json:"scannedLines"`
	MatchesFound  int   `json:"matchesFound"`
}

// SearchResponse 是一次日志检索的结果摘要。
type SearchResponse struct {
	FilePath           string  `json:"filePath"`
	FileSizeBytes      int64   `json:"fileSizeBytes"`
	BytesScanned       int64   `json:"bytesScanned"`
	ScannedLines       int64   `json:"scannedLines"`
	Matches            []Match `json:"matches"`
	ResultLimitReached bool    `json:"resultLimitReached"`
	Canceled           bool    `json:"canceled"`
}

// SearchFile 保留简单调用入口；需要取消和进度信息时请调用 SearchFileContext。
func SearchFile(path, query string, useRegex bool, maxResults int) (SearchResponse, error) {
	return SearchFileContext(context.Background(), path, query, useRegex, maxResults, nil)
}

// SearchFileContext 使用固定大小的读取缓冲区扫描文件。
// 普通文本模式会跨缓冲区边界匹配，不会为了搜索而拼接整行；只保存有限的结果预览。
// 正则模式按整行匹配，但单行最多允许 4 MiB，避免异常日志行造成不受控的内存占用。
func SearchFileContext(ctx context.Context, path, query string, useRegex bool, maxResults int, onProgress func(Progress)) (SearchResponse, error) {
	if strings.TrimSpace(path) == "" {
		return SearchResponse{}, errors.New("请先选择日志文件")
	}
	if query == "" {
		return SearchResponse{}, errors.New("请输入搜索内容")
	}
	if len(query) > 4096 {
		return SearchResponse{}, errors.New("搜索内容不能超过 4096 字节")
	}
	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}
	if maxResults > hardMaxResults {
		maxResults = hardMaxResults
	}
	if err := ctx.Err(); err != nil {
		return SearchResponse{Canceled: true}, nil
	}

	var matcher *regexp.Regexp
	if useRegex {
		compiled, err := regexp.Compile(query)
		if err != nil {
			return SearchResponse{}, fmt.Errorf("正则表达式无效：%w", err)
		}
		matcher = compiled
	}
	needle := []byte(query)

	file, err := os.Open(path)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("打开日志文件失败：%w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return SearchResponse{}, fmt.Errorf("读取日志文件信息失败：%w", err)
	}

	response := SearchResponse{
		FilePath: path, FileSizeBytes: info.Size(),
		Matches: make([]Match, 0, min(maxResults, 128)),
	}
	reader := bufio.NewReaderSize(file, readerBufferSize)

	var lineNumber int64
	var lineBytes int64
	var bytesScanned int64
	var lastProgressBytes int64
	var preview []byte
	var regexLine []byte
	var carry []byte
	var pendingCR bool
	var lineOpen bool
	var lineMatched bool
	var reachedLimit bool

	notify := func(force bool) {
		if onProgress == nil {
			return
		}
		if !force && bytesScanned-lastProgressBytes < progressStepBytes {
			return
		}
		lastProgressBytes = bytesScanned
		onProgress(Progress{
			BytesScanned: bytesScanned, FileSizeBytes: info.Size(),
			ScannedLines: response.ScannedLines, MatchesFound: len(response.Matches),
		})
	}

	appendContent := func(part []byte) error {
		if len(part) == 0 {
			return nil
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

		if matcher != nil {
			if len(regexLine)+len(part) > maxRegexLineBytes {
				return fmt.Errorf("当前日志包含超过 4 MiB 的单行内容；为避免占用过多内存，正则模式已停止，请改用普通文本搜索")
			}
			regexLine = append(regexLine, part...)
			return nil
		}

		if !lineMatched {
			window := make([]byte, 0, len(carry)+len(part))
			window = append(window, carry...)
			window = append(window, part...)
			lineMatched = bytes.Contains(window, needle)

			keep := len(needle) - 1
			if keep > len(window) {
				keep = len(window)
			}
			if keep > 0 {
				carry = append(carry[:0], window[len(window)-keep:]...)
			} else {
				carry = carry[:0]
			}
		}
		return nil
	}

	finishLine := func() {
		lineNumber++
		response.ScannedLines = lineNumber
		if matcher != nil {
			lineMatched = matcher.Match(regexLine)
		}
		if lineMatched {
			content := strings.ToValidUTF8(string(preview), "�")
			response.Matches = append(response.Matches, Match{
				LineNumber: lineNumber,
				Content: content,
				ContentClipped: lineBytes > maxPreviewBytes,
			})
			if len(response.Matches) >= maxResults {
				response.ResultLimitReached = true
				reachedLimit = true
			}
		}

		lineBytes = 0
		preview = preview[:0]
		regexLine = regexLine[:0]
		carry = carry[:0]
		lineMatched = false
		lineOpen = false
		pendingCR = false
	}

	for {
		if ctx.Err() != nil {
			response.Canceled = true
			response.BytesScanned = bytesScanned
			notify(true)
			return response, nil
		}

		chunk, readErr := reader.ReadSlice('\n')
		bytesScanned += int64(len(chunk))
		hasNewline := len(chunk) > 0 && chunk[len(chunk)-1] == '\n'
		part := chunk
		if hasNewline {
			part = part[:len(part)-1]
		}

		// 兼容 CRLF；如果回车符恰好落在缓冲区边界，先暂存再判断。
		if hasNewline && len(part) > 0 && part[len(part)-1] == '\r' {
			part = part[:len(part)-1]
		}
		if pendingCR {
			if !(hasNewline && len(part) == 0) {
				if err := appendContent([]byte{'\r'}); err != nil {
					return SearchResponse{}, err
				}
			}
			pendingCR = false
		}
		if !hasNewline && len(part) > 0 && part[len(part)-1] == '\r' {
			part = part[:len(part)-1]
			pendingCR = true
			lineOpen = true
		}
		if len(part) > 0 {
			if err := appendContent(part); err != nil {
				return SearchResponse{}, err
			}
		}

		if hasNewline {
			finishLine()
			notify(false)
			if reachedLimit {
				response.BytesScanned = bytesScanned
				notify(true)
				return response, nil
			}
		}

		if readErr == bufio.ErrBufferFull {
			notify(false)
			continue
		}
		if readErr == io.EOF {
			// EOF 前没有换行符的最后一行也算一行；末尾孤立的回车符按行结束符处理。
			if lineOpen {
				pendingCR = false
				finishLine()
			}
			break
		}
		if readErr != nil {
			return SearchResponse{}, fmt.Errorf("读取日志文件失败：%w", readErr)
		}
		notify(false)
	}

	response.BytesScanned = bytesScanned
	notify(true)
	return response, nil
}
