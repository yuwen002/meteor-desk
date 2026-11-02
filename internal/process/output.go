package process

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

// 输出缓存只保留在内存：单行最多 64 KiB，每个服务最多最近 1,500 条。
const maxOutputLines = 1500
const maxOutputLineBytes = 64 * 1024

// truncateMark 是单行超长被截断时附加的提示。
const truncateMark = " …[输出过长，已截断]"

// OutputLine 表示某个服务的一条本机输出或生命周期消息。
type OutputLine struct {
	Sequence int64  `json:"sequence"`
	Time     string `json:"time"`
	Stream   string `json:"stream"`
	Text     string `json:"text"`
}

// Logs 返回序号大于 afterSequence 的缓存输出，适合前端按游标轮询。
func (m *Manager) Logs(id string, afterSequence int64) []OutputLine {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lines := m.outputs[id]
	start := 0
	for start < len(lines) && lines[start].Sequence <= afterSequence {
		start++
	}
	return append([]OutputLine(nil), lines[start:]...)
}

// appendOutput 追加一条输出或系统消息，并在超出上限时丢弃最早的行。
// stdout/stderr 会顺带尝试登记就绪事件，用于测量启动耗时；开启落盘的服务还会把这两类行写入本地文件。
// 落盘写入在释放 m.mu 后进行，避免磁盘 I/O 阻塞启停；同一协程内的行序仍与内存序号一致。
func (m *Manager) appendOutput(id, stream, text string) {
	now := time.Now()
	m.mu.Lock()
	m.outputSequence[id]++
	line := OutputLine{
		Sequence: m.outputSequence[id],
		Time:     now.Format("15:04:05.000"),
		Stream:   stream,
		Text:     strings.ToValidUTF8(text, "\ufffd"),
	}
	lines := append(m.outputs[id], line)
	if len(lines) > maxOutputLines {
		lines = append([]OutputLine(nil), lines[len(lines)-maxOutputLines:]...)
	}
	m.outputs[id] = lines
	var sink *diskRotator
	var entry string
	// 首行进程输出视为启动完成信号；没有健康检查时也能量出启动耗时。
	if stream == "stdout" || stream == "stderr" {
		m.markReadyLocked(id, "首次输出")
		if instance := m.instances[id]; instance != nil && instance.disk != nil {
			sink = instance.disk
			entry = now.Format(time.RFC3339Nano) + " [" + stream + "] " + line.Text
		}
	}
	m.mu.Unlock()
	if sink != nil {
		_ = sink.Write(entry)
	}
}

// readOutput 按行读取管道并回调 emit，单行超过 maxOutputLineBytes 时截断并附加提示。
// 不拼接无上限的超长行，避免一个刷屏的服务把内存吃掉。
func readOutput(reader io.Reader, emit func(string)) {
	br := bufio.NewReaderSize(reader, 32*1024)
	line := make([]byte, 0, 4096)
	clipped := false
	for {
		part, err := br.ReadSlice('\n')
		newline := len(part) > 0 && part[len(part)-1] == '\n'
		if newline {
			part = part[:len(part)-1]
		}
		if newline && len(part) > 0 && part[len(part)-1] == '\r' {
			part = part[:len(part)-1]
		}
		room := maxOutputLineBytes - len(line)
		if room > 0 {
			take := min(room, len(part))
			line = append(line, part[:take]...)
			if take < len(part) {
				clipped = true
			}
		} else if len(part) > 0 {
			clipped = true
		}
		if newline {
			if clipped {
				line = append(line, truncateMark...)
			}
			emit(string(line))
			line = line[:0]
			clipped = false
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == io.EOF {
			if len(line) > 0 || clipped {
				if clipped {
					line = append(line, truncateMark...)
				}
				emit(string(line))
			}
			return
		}
		if err != nil {
			// 停止流程会主动关闭管道，这不是错误，安静结束即可。
			if errors.Is(err, os.ErrClosed) {
				return
			}
			emit("读取进程输出失败：" + err.Error())
			return
		}
	}
}
