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
type TailFileState struct {
	Info os.FileInfo
	Anchor []byte
	AnchorOffset int64
}

// ReadTailChunk 从指定字节偏移读取一小段新增内容。
func ReadTailChunk(path string, offset int64) (TailChunk, error) {
	chunk, _, err := ReadTailChunkWithPrevious(path, offset, nil)
	return chunk, err
}

// ReadTailChunkWithPrevious 返回下一段内容及校验状态。
// Anchor 用于识别同一文件在两次轮询之间被截断并迅速重写为更大文件的情况。
func ReadTailChunkWithPrevious(path string, offset int64, previous *TailFileState) (TailChunk, *TailFileState, error) {
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
		state, stateErr := makeTailFileState(f, info, info.Size())
		if stateErr != nil { return TailChunk{}, nil, stateErr }
		return TailChunk{Offset: info.Size(), FileSize: info.Size()}, state, nil
	}
	// os.SameFile 在各支持平台比较文件身份；仅比较大小无法发现更大的轮转替换文件。
	reset := offset > info.Size()
	if previous != nil {
		if previous.Info == nil || !os.SameFile(previous.Info, info) || info.Size() < previous.Info.Size() {
			reset = true
		} else if len(previous.Anchor) > 0 && !tailAnchorMatches(f, previous) {
			reset = true
		}
	}
	if reset {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return TailChunk{}, nil, err
	}

	remaining := info.Size() - offset
	if remaining <= 0 {
		state, stateErr := makeTailFileState(f, info, offset)
		if stateErr != nil { return TailChunk{}, nil, stateErr }
		return TailChunk{Offset: offset, Reset: reset, FileSize: info.Size()}, state, nil
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
	newOffset := offset + int64(n)
	state, stateErr := makeTailFileState(f, info, newOffset)
	if stateErr != nil { return TailChunk{}, nil, stateErr }
	return TailChunk{
		Content:  string(buf[:n]),
		Offset:   newOffset,
		Reset:    reset,
		FileSize: info.Size(),
	}, state, nil
}

const tailAnchorBytes = 64

func makeTailFileState(f *os.File, info os.FileInfo, end int64) (*TailFileState, error) {
	start := end - tailAnchorBytes
	if start < 0 { start = 0 }
	anchor := make([]byte, int(end-start))
	if len(anchor) > 0 {
		n, err := f.ReadAt(anchor, start)
		if err != nil && err != io.EOF { return nil, err }
		anchor = anchor[:n]
	}
	return &TailFileState{Info: info, Anchor: anchor, AnchorOffset: start}, nil
}

func tailAnchorMatches(f *os.File, previous *TailFileState) bool {
	if previous == nil || len(previous.Anchor) == 0 { return true }
	current := make([]byte, len(previous.Anchor))
	n, err := f.ReadAt(current, previous.AnchorOffset)
	if err != nil && err != io.EOF { return false }
	return n == len(previous.Anchor) && bytes.Equal(current, previous.Anchor)
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
