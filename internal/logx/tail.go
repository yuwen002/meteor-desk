package logx

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// TailOptions 控制一次增量读取。
type TailOptions struct {
	// Offset 是上一次结果返回的 Offset；负值表示从文件末尾开始跟踪。
	Offset int64
	// MaxBytes 限制单次读取的原始字节数，默认 256 KiB，上限 2 MiB。
	MaxBytes int64
	// MaxLines 限制单次返回的行数，默认 500，上限 5,000。
	MaxLines int
	// Encoding 与检索一致，支持 auto、utf-8、gb18030；UTF-16 暂不支持。
	Encoding string
}

// TailResult 是实时跟踪的一次增量。
//
// Offset 只前进到最后一个完整行之后，因此未写完的行会留到下一次读取；
// Reset 表示文件被截断、替换或首次跟踪，调用方应清空已有行。
type TailResult struct {
	Lines         []string `json:"lines"`
	Offset        int64    `json:"offset"`
	FileSizeBytes int64    `json:"fileSizeBytes"`
	Encoding      string   `json:"encoding"`
	Reset         bool     `json:"reset"`
	// Truncated 表示单次读取达到上限、存在未完成行或跳过了超长行。
	Truncated bool `json:"truncated"`
}

const (
	defaultTailMaxBytes = 256 * 1024
	hardTailMaxBytes    = 2 * 1024 * 1024
	defaultTailMaxLines = 500
	hardTailMaxLines    = 5000
	// maxTailLineBytes 限制单行长度；超过该长度仍未遇到换行时放弃该段落，
	// 避免异常日志或二进制内容把增量读取拖入无界缓冲。
	maxTailLineBytes = 1 << 20
	// maxTailPreviewBytes 限制单行返回长度，避免前端渲染超长行。
	maxTailPreviewBytes = 64 * 1024
)

// TailFile 读取日志文件在指定偏移之后的新内容，用于实时跟踪。
// 只按换行切分并在完整行之后前进偏移，因此不会输出半行或重复行。
func TailFile(path string, options TailOptions) (TailResult, error) {
	if strings.TrimSpace(path) == "" {
		return TailResult{}, errors.New("请先选择日志文件")
	}
	maxBytes := options.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultTailMaxBytes
	}
	if maxBytes > hardTailMaxBytes {
		maxBytes = hardTailMaxBytes
	}
	maxLines := options.MaxLines
	if maxLines <= 0 {
		maxLines = defaultTailMaxLines
	}
	if maxLines > hardTailMaxLines {
		maxLines = hardTailMaxLines
	}

	file, err := os.Open(path)
	if err != nil {
		return TailResult{}, fmt.Errorf("打开日志文件失败：%w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return TailResult{}, fmt.Errorf("读取日志文件信息失败：%w", err)
	}

	sampleSize := min(info.Size(), int64(maxEncodingSampleBytes))
	sample := make([]byte, int(sampleSize))
	if len(sample) > 0 {
		n, sampleErr := file.ReadAt(sample, 0)
		if sampleErr != nil && sampleErr != io.EOF {
			return TailResult{}, fmt.Errorf("读取日志编码样本失败：%w", sampleErr)
		}
		sample = sample[:n]
	}
	encodingName, bomBytes, decoder, err := resolveTextEncoding(sample, options.Encoding, int64(len(sample)) == info.Size())
	if err != nil {
		return TailResult{}, err
	}
	// UTF-16 的换行由两个字节组成，按字节切分会在字符中间断开；
	// 实时跟踪要求单字节换行，因此在这里明确拒绝而不是返回乱码。
	if strings.HasPrefix(encodingName, "UTF-16") {
		return TailResult{}, fmt.Errorf("实时跟踪暂不支持 %s 编码的日志；请将日志转换为 UTF-8 或 GB18030 后重试", encodingName)
	}

	result := TailResult{FileSizeBytes: info.Size(), Encoding: encodingName, Lines: []string{}}
	start := options.Offset
	skipPartial := false
	switch {
	case start < 0:
		// 首次跟踪从文件末尾回退，避免为了显示最后几百行而扫描整份大文件。
		start = info.Size() - maxBytes
		if start < bomBytes {
			start = bomBytes
		}
		// 回退点通常落在行中间；如果前一个字节就是换行，则从行首开始，不丢首行。
		if start > bomBytes {
			previous := make([]byte, 1)
			if _, err := file.ReadAt(previous, start-1); err == nil {
				skipPartial = previous[0] != '\n'
			}
		}
		result.Reset = true
	case start > info.Size():
		// 文件被截断或替换：重新从头读取，调用方需要清空旧行。
		start = bomBytes
		result.Reset = true
	case start < bomBytes:
		start = bomBytes
	}

	if start >= info.Size() {
		result.Offset = info.Size()
		return result, nil
	}

	buf := make([]byte, maxBytes)
	n, readErr := file.ReadAt(buf, start)
	if readErr != nil && readErr != io.EOF {
		return TailResult{}, fmt.Errorf("读取日志增量失败：%w", readErr)
	}
	chunk := buf[:n]
	var cursor int64
	if skipPartial {
		idx := bytes.IndexByte(chunk, '\n')
		if idx < 0 {
			// 回退起点所在的整段都在同一行内，直接跳过，避免反复读取同一块。
			result.Offset = start + int64(n)
			result.Truncated = true
			return result, nil
		}
		cursor = int64(idx + 1)
	}
	body := chunk[cursor:]
	last := bytes.LastIndexByte(body, '\n')
	if last < 0 {
		if len(body) >= maxTailLineBytes {
			result.Offset = start + int64(n)
			result.Truncated = true
			return result, nil
		}
		// 还没有完整行：偏移停在行首，等下一次读取补齐。
		result.Offset = start + cursor
		return result, nil
	}
	complete := body[:last+1]
	result.Offset = start + cursor + int64(last+1)
	lines := splitTailLines(complete)
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
		result.Truncated = true
	}
	if decoder != nil {
		for i, line := range lines {
			decoded, decodeErr := decoder.NewDecoder().Bytes([]byte(line))
			if decodeErr != nil {
				return TailResult{}, fmt.Errorf("解码日志增量失败：%w", decodeErr)
			}
			lines[i] = string(decoded)
		}
	}
	result.Lines = lines
	return result, nil
}

// splitTailLines 把完整行数据切成文本行，保留空行并去掉行尾 CR。
func splitTailLines(data []byte) []string {
	trimmed := bytes.TrimSuffix(data, []byte("\n"))
	if len(trimmed) == 0 {
		return nil
	}
	parts := bytes.Split(trimmed, []byte("\n"))
	lines := make([]string, len(parts))
	for i, part := range parts {
		part = bytes.TrimSuffix(part, []byte("\r"))
		if len(part) > maxTailPreviewBytes {
			lines[i] = string(part[:maxTailPreviewBytes]) + " [本行过长，已截断]"
			continue
		}
		lines[i] = string(part)
	}
	return lines
}
