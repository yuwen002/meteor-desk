package logx

import (
	"errors"
	"io"
	"os"
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
// 文件被截断时会从头读取；单次读取有上限，避免大文件增长时占用过多内存。
func ReadTailChunk(path string, offset int64) (TailChunk, error) {
	if offset < -1 {
		return TailChunk{}, errors.New("日志偏移量不能小于 -1")
	}
	f, err := os.Open(path)
	if err != nil {
		return TailChunk{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return TailChunk{}, err
	}
	if !info.Mode().IsRegular() {
		return TailChunk{}, errors.New("实时跟踪仅支持普通文件")
	}

	if offset == -1 {
		return TailChunk{Offset: info.Size(), FileSize: info.Size()}, nil
	}
	reset := offset > info.Size()
	if reset {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return TailChunk{}, err
	}

	remaining := info.Size() - offset
	if remaining <= 0 {
		return TailChunk{Offset: offset, Reset: reset, FileSize: info.Size()}, nil
	}
	if remaining > maxTailChunkBytes {
		remaining = maxTailChunkBytes
	}
	buf := make([]byte, int(remaining))
	n, readErr := io.ReadFull(f, buf)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return TailChunk{}, readErr
	}
	return TailChunk{
		Content:  string(buf[:n]),
		Offset:   offset + int64(n),
		Reset:    reset,
		FileSize: info.Size(),
	}, nil
}
