package logx

import (
	"errors"
	"io"
	"os"
	"unicode/utf8"
)

const maxTailChunkBytes = 256 * 1024

// TailChunk 是一次增量读取的快照。Offset 始终表示已读取的文件字节位置。
type TailChunk struct {
	Content  string `json:"content"`
	Offset   int64  `json:"offset"`
	Reset    bool   `json:"reset"`
	FileSize int64  `json:"fileSize"`
}

// ReadTailChunk 从指定字节偏移读取一小段新增内容。
func ReadTailChunk(path string, offset int64) (TailChunk, error) {
	chunk, _, err := ReadTailChunkWithPrevious(path, offset, nil)
	return chunk, err
}

// ReadTailChunkWithPrevious 同时返回文件元信息，以便调用方识别路径指向的文件是否被替换。
func ReadTailChunkWithPrevious(path string, offset int64, previous os.FileInfo) (TailChunk, os.FileInfo, error) {
	if offset < -1 {
		return TailChunk{}, nil, errors.New("日志偏移量不能小于 -1")
	}
	f, err := os.Open(path)
	if err != nil {
		return TailChunk{}, nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return TailChunk{}, nil, err
	}
	if !info.Mode().IsRegular() {
		return TailChunk{}, nil, errors.New("实时跟踪仅支持普通文件")
	}

	if offset == -1 {
		return TailChunk{Offset: info.Size(), FileSize: info.Size()}, info, nil
	}
	// os.SameFile 在各支持平台比较文件身份；仅比较大小无法发现更大的轮转替换文件。
	reset := offset > info.Size() || (previous != nil && !os.SameFile(previous, info))
	if reset {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return TailChunk{}, nil, err
	}

	remaining := info.Size() - offset
	if remaining <= 0 {
		return TailChunk{Offset: offset, Reset: reset, FileSize: info.Size()}, info, nil
	}
	if remaining > maxTailChunkBytes {
		remaining = maxTailChunkBytes
	}
	buf := make([]byte, int(remaining))
	n, readErr := io.ReadFull(f, buf)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return TailChunk{}, nil, readErr
	}
	// Keep UTF-8 code points intact across chunk boundaries. The byte offset
	// advances only through bytes returned to the caller, so the next poll
	// rereads any incomplete trailing rune instead of replacing it with �.
	if offset+int64(n) < info.Size() {
		n = completeUTF8Prefix(buf[:n])
	}
	return TailChunk{
		Content:  string(buf[:n]),
		Offset:   offset + int64(n),
		Reset:    reset,
		FileSize: info.Size(),
	}, info, nil
}

func completeUTF8Prefix(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	start := len(data) - 1
	for start > 0 && !utf8.RuneStart(data[start]) {
		start--
	}
	if !utf8.FullRune(data[start:]) {
		return start
	}
	return len(data)
}
