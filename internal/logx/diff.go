package logx

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/text/transform"
)

// maxDiffReadBytes 限制两份日志逐行对比时单侧读取的原始字节数（阶段 6 批次 D）。
const maxDiffReadBytes = 4 * 1024 * 1024

// DecodedFileHead 是读取文件开头并解码为 UTF-8 的结果，供逐行差异对比使用。
type DecodedFileHead struct {
	Text      string `json:"text"`
	Encoding  string `json:"encoding"`
	SizeBytes int64  `json:"sizeBytes"`
	// Truncated 表示文件正文超过读取上限，这里只包含开头部分。
	Truncated bool `json:"truncated"`
}

// ReadDecodedFileHead 读取文件开头至多 maxBytes 原始字节，并按识别编码解码为 UTF-8 文本。
// 与检索、实时跟踪共用同一套编码识别逻辑；maxBytes<=0 或超过 4 MiB 时一律按 4 MiB 上限处理，
// 因此返回值最多承载一侧 4 MiB 的内容，避免为对比把整份大文件读进内存。
func ReadDecodedFileHead(path, encoding string, maxBytes int64) (DecodedFileHead, error) {
	if strings.TrimSpace(path) == "" {
		return DecodedFileHead{}, errors.New("请先选择日志文件")
	}
	if maxBytes <= 0 || maxBytes > maxDiffReadBytes {
		maxBytes = maxDiffReadBytes
	}

	file, err := os.Open(path)
	if err != nil {
		return DecodedFileHead{}, fmt.Errorf("打开日志文件失败：%w", err)
	}
	// 只读打开，关闭失败无需上抛。
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return DecodedFileHead{}, fmt.Errorf("读取日志文件信息失败：%w", err)
	}

	sampleSize := min(info.Size(), int64(maxEncodingSampleBytes))
	sample := make([]byte, int(sampleSize))
	if len(sample) > 0 {
		n, sampleErr := file.ReadAt(sample, 0)
		if sampleErr != nil && sampleErr != io.EOF {
			return DecodedFileHead{}, fmt.Errorf("读取日志编码样本失败：%w", sampleErr)
		}
		sample = sample[:n]
	}
	encodingName, bomBytes, decoder, err := resolveTextEncoding(sample, encoding, int64(len(sample)) == info.Size())
	if err != nil {
		return DecodedFileHead{}, err
	}
	if _, err := file.Seek(bomBytes, io.SeekStart); err != nil {
		return DecodedFileHead{}, fmt.Errorf("定位日志正文失败：%w", err)
	}

	// 先按原始字节限额截断，再解码；解码输出同样截断到 maxBytes，
	// 防止 UTF-16/GB18030 解码后字符膨胀造成失控的内存占用。
	raw := io.LimitReader(file, maxBytes)
	if decoder != nil {
		raw = transform.NewReader(io.LimitReader(file, maxBytes), decoder.NewDecoder())
	}
	data, readErr := io.ReadAll(io.LimitReader(raw, maxBytes))
	// 截断的多字节序列可能在末尾抛出意外 EOF；只要已读到内容就按最佳努力返回，
	// 只有完全读不到任何字节时才视为真正的错误。
	if readErr != nil && len(data) == 0 {
		return DecodedFileHead{}, fmt.Errorf("读取日志内容失败：%w", readErr)
	}

	return DecodedFileHead{
		Text:      strings.ToValidUTF8(string(data), "\uFFFD"),
		Encoding:  encodingName,
		SizeBytes: info.Size(),
		Truncated: info.Size()-bomBytes > maxBytes,
	}, nil
}
