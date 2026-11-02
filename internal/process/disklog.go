package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// 输出落盘的默认约束：单文件 64 MiB，轮转后连同当前文件最多保留 3 份。
// 这些值刻意做小，避免长时间刷屏的服务把配置目录占满。
const (
	defaultOutputFileMaxBytes = 64 << 20
	defaultOutputFileKeep     = 3
	// outputLogDirName 是配置目录下存放服务输出的子目录名。
	outputLogDirName = "logs"
)

// diskRotator 把单个服务的输出按行追加到一个本地文件，并在超过大小上限时轮转。
// 轮转采用命名位移：当前文件 name.log → name.log.1 → name.log.2 …，超过保留份数的最旧文件被删除。
// 并发写（stdout、stderr、系统消息可能来自不同协程）由内部 mu 保护，跨流的先后不保证强一致，
// 但每条记录自带时间戳与流标记，足以事后排查；同一文件内的写入不会交错出半行。
type diskRotator struct {
	mu       sync.Mutex
	dir      string
	base     string // 已清理过的文件名（不含轮转后缀），例如 “api-server.log”
	maxBytes int64
	keep     int
	file     *os.File
	size     int64
}

// newDiskRotator 构造一个落盘写入器；此处不创建目录或文件，首次 Write 时才打开，
// 以便把落盘失败（权限、磁盘）推迟到服务真正产生输出时再暴露。
func newDiskRotator(dir, base string, maxBytes int64, keep int) *diskRotator {
	if maxBytes <= 0 {
		maxBytes = defaultOutputFileMaxBytes
	}
	if keep < 1 {
		keep = 1
	}
	return &diskRotator{dir: dir, base: base, maxBytes: maxBytes, keep: keep}
}

// Path 返回当前活动输出文件的绝对路径；即使尚未打开也能给出，供详情面板展示。
func (d *diskRotator) Path() string {
	return filepath.Join(d.dir, d.base)
}

// activePathLocked 是当前正在写入的文件路径（不带轮转后缀）。
func (d *diskRotator) activePathLocked() string {
	return filepath.Join(d.dir, d.base)
}

// backupPathLocked 返回第 index（从 1 开始）个历史备份文件路径。
func (d *diskRotator) backupPathLocked(index int) string {
	return d.activePathLocked() + "." + strconv.Itoa(index)
}

// ensureOpenLocked 按需创建目录并以追加方式打开当前文件；调用方需持有 d.mu。
func (d *diskRotator) ensureOpenLocked() error {
	if d.file != nil {
		return nil
	}
	if err := os.MkdirAll(d.dir, 0700); err != nil {
		return err
	}
	path := d.activePathLocked()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	size := int64(0)
	if info, statErr := file.Stat(); statErr == nil {
		size = info.Size()
	}
	d.file = file
	d.size = size
	return nil
}

// Write 追加一条记录（自动补换行）；写入前按需打开文件，超过上限时先轮转。
// entry 由调用方格式化（含时间与流标记），这里只负责落盘与轮转，便于独立单元测试。
func (d *diskRotator) Write(entry string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.ensureOpenLocked(); err != nil {
		return err
	}
	line := strings.ReplaceAll(entry, "\n", "\u2028") + "\n" // 单行落盘，内嵌换行用行分隔符替换，避免破坏按行读取
	n := int64(len(line))
	// 只有文件已有内容且再写会超限时才轮转，保证空文件不会因单条超长记录反复触发轮转。
	if d.size > 0 && d.size+n > d.maxBytes {
		if err := d.rotateLocked(); err != nil {
			return err
		}
	}
	if _, err := d.file.WriteString(line); err != nil {
		return err
	}
	d.size += n
	return nil
}

// rotateLocked 关闭当前文件，把历史备份整体后移一位、丢弃超出保留份数的最旧文件，
// 再将当前文件转为 name.log.1，最后打开一个全新的当前文件继续写入。调用方需持有 d.mu。
func (d *diskRotator) rotateLocked() error {
	if d.file != nil {
		_ = d.file.Close()
		d.file = nil
	}
	if d.keep == 1 {
		// 只保留当前文件：直接清空重开，等价于就地截断。
		file, err := os.OpenFile(d.activePathLocked(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		d.file = file
		d.size = 0
		return nil
	}
	// 丢弃会掉出保留窗口的最旧备份。
	_ = os.Remove(d.backupPathLocked(d.keep - 1))
	// 从高位向低位位移，避免覆盖尚未搬走的备份。
	for index := d.keep - 2; index >= 1; index-- {
		src := d.backupPathLocked(index)
		dst := d.backupPathLocked(index + 1)
		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(src, dst)
		}
	}
	if err := os.Rename(d.activePathLocked(), d.backupPathLocked(1)); err != nil && !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(d.activePathLocked(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	d.file = file
	d.size = 0
	return nil
}

// Close 关闭底层文件句柄；重复调用安全。落盘写入在每次 Write 时直接进行，
// 因此这里不需要额外 flush，退出收尾只要确保句柄关闭即可。
func (d *diskRotator) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.file == nil {
		return nil
	}
	err := d.file.Close()
	d.file = nil
	return err
}

// outputLogDir 返回服务输出落盘目录（配置目录下的 logs/）。configPath 构造后不再变更，
// 因此无需持锁读取。
func (m *Manager) outputLogDir() string {
	return filepath.Join(filepath.Dir(m.configPath), outputLogDirName)
}

// newOutputSink 为一次运行创建落盘写入器；此处只计算路径、不碰磁盘。
func (m *Manager) newOutputSink(name string) *diskRotator {
	return newDiskRotator(m.outputLogDir(), sanitizeLogName(name), defaultOutputFileMaxBytes, defaultOutputFileKeep)
}

// removeOutputLogFiles 删除某个服务名对应的当前及历史输出文件；文件不存在时静默忽略。
func (m *Manager) removeOutputLogFiles(base string) {
	dir := m.outputLogDir()
	_ = os.Remove(filepath.Join(dir, base))
	for index := 1; index <= defaultOutputFileKeep; index++ {
		_ = os.Remove(filepath.Join(dir, base+"."+strconv.Itoa(index)))
	}
}

// logBaseNameOf 暴露清理后的文件名，供删除服务时判断是否有其他配置共用同一个落盘文件。
func logBaseNameOf(name string) string {
	return sanitizeLogName(name)
}

// sanitizeLogName 把服务名转成可安全用作文件名的形式：去掉路径分隔符与控制字符、
// 非法文件名字符替换为连字符，压缩空白；为空时回退到占位名，并强制 .log 后缀。
func sanitizeLogName(name string) string {
	trimmed := strings.TrimSpace(name)
	if idx := strings.LastIndexAny(trimmed, `\/`); idx >= 0 {
		trimmed = trimmed[idx+1:]
	}
	var b strings.Builder
	for _, r := range trimmed {
		if r < 0x20 || strings.ContainsRune(`:*?"<>|\/`, r) {
			b.WriteRune('-')
			continue
		}
		b.WriteRune(r)
	}
	base := strings.Trim(b.String(), ".- ")
	if base == "" {
		base = "service"
	}
	if !strings.EqualFold(filepath.Ext(base), ".log") {
		base += ".log"
	}
	return base
}
