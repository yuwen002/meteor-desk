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
	"unicode/utf8"

	textencoding "golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	textunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const (
	defaultMaxResults = 500
	hardMaxResults    = 5000
	maxPreviewBytes   = 8 * 1024
	maxContextBytes   = 1024
	readerBufferSize  = 256 * 1024
	progressStepBytes = 256 * 1024
	// 正则表达式需要按整行匹配，因此为单行设置内存上限。
	maxRegexLineBytes = 4 * 1024 * 1024
)

const maxEncodingSampleBytes = 64 * 1024

// byteCountingReader 统计解码器从底层文件读取的原始字节数。
type byteCountingReader struct {
	source    io.Reader
	bytesRead int64
}

func (r *byteCountingReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	r.bytesRead += int64(n)
	return n, err
}

// resolveTextEncoding 支持 BOM、UTF-8 校验、无 BOM UTF-16 启发式检测，
// 无法识别的非 UTF-8 文本按中文 Windows 日志常见的 GB18030 处理。
func resolveTextEncoding(sample []byte, requested string, wholeFileSample bool) (string, int64, textencoding.Encoding, error) {
	mode := strings.ToLower(strings.TrimSpace(requested))
	if mode == "" {
		mode = "auto"
	}
	utf8BOM := []byte{0xEF, 0xBB, 0xBF}
	utf16LEBOM := []byte{0xFF, 0xFE}
	utf16BEBOM := []byte{0xFE, 0xFF}

	switch mode {
	case "auto":
		switch {
		case bytes.HasPrefix(sample, utf8BOM):
			return "UTF-8", int64(len(utf8BOM)), nil, nil
		case bytes.HasPrefix(sample, utf16LEBOM):
			return "UTF-16LE", int64(len(utf16LEBOM)), textunicode.UTF16(textunicode.LittleEndian, textunicode.IgnoreBOM), nil
		case bytes.HasPrefix(sample, utf16BEBOM):
			return "UTF-16BE", int64(len(utf16BEBOM)), textunicode.UTF16(textunicode.BigEndian, textunicode.IgnoreBOM), nil
		}
		if guessed := guessUTF16(sample); guessed != "" {
			return encodingByName(guessed, 0)
		}
		if validUTF8Sample(sample, wholeFileSample) {
			return "UTF-8", 0, nil, nil
		}
		return "GB18030", 0, simplifiedchinese.GB18030, nil
	case "utf-8", "utf8":
		if bytes.HasPrefix(sample, utf8BOM) {
			return "UTF-8", int64(len(utf8BOM)), nil, nil
		}
		return "UTF-8", 0, nil, nil
	case "gb18030", "gbk":
		return "GB18030", 0, simplifiedchinese.GB18030, nil
	case "utf-16le", "utf16le":
		var skip int64
		if bytes.HasPrefix(sample, utf16LEBOM) {
			skip = int64(len(utf16LEBOM))
		}
		return "UTF-16LE", skip, textunicode.UTF16(textunicode.LittleEndian, textunicode.IgnoreBOM), nil
	case "utf-16be", "utf16be":
		var skip int64
		if bytes.HasPrefix(sample, utf16BEBOM) {
			skip = int64(len(utf16BEBOM))
		}
		return "UTF-16BE", skip, textunicode.UTF16(textunicode.BigEndian, textunicode.IgnoreBOM), nil
	default:
		return "", 0, nil, fmt.Errorf("不支持的日志编码 %q；请选择自动识别、UTF-8、GB18030 或 UTF-16", requested)
	}
}

func decoderForEncoding(name string) textencoding.Encoding {
	switch name {
	case "GB18030", "GBK":
		return simplifiedchinese.GB18030
	case "UTF-16LE":
		return textunicode.UTF16(textunicode.LittleEndian, textunicode.IgnoreBOM)
	case "UTF-16BE":
		return textunicode.UTF16(textunicode.BigEndian, textunicode.IgnoreBOM)
	default:
		return nil
	}
}

func encodingByName(name string, skip int64) (string, int64, textencoding.Encoding, error) {
	switch name {
	case "UTF-16LE":
		return name, skip, textunicode.UTF16(textunicode.LittleEndian, textunicode.IgnoreBOM), nil
	case "UTF-16BE":
		return name, skip, textunicode.UTF16(textunicode.BigEndian, textunicode.IgnoreBOM), nil
	default:
		return "", 0, nil, fmt.Errorf("内部编码识别结果无效：%s", name)
	}
}

// validUTF8Sample 只容忍样本末尾被截断的合法 UTF-8 字符，不把任意非法尾字节当成截断。
func validUTF8Sample(sample []byte, wholeFile bool) bool {
	if utf8.Valid(sample) {
		return true
	}
	if wholeFile || len(sample) == 0 {
		return false
	}

	// UTF-8 字符最多占 4 字节，因此不完整字符的起始位置只能出现在末尾 3 字节内。
	startMin := max(0, len(sample)-utf8.UTFMax+1)
	for start := len(sample) - 1; start >= startMin; start-- {
		lead := sample[start]
		expected := utf8SequenceLength(lead)
		if expected == 0 || len(sample)-start >= expected || !utf8.Valid(sample[:start]) {
			continue
		}
		if isValidUTF8Prefix(sample[start:]) {
			return true
		}
	}
	return false
}

func utf8SequenceLength(lead byte) int {
	switch {
	case lead >= 0xC2 && lead <= 0xDF:
		return 2
	case lead >= 0xE0 && lead <= 0xEF:
		return 3
	case lead >= 0xF0 && lead <= 0xF4:
		return 4
	default:
		return 0
	}
}

// isValidUTF8Prefix 检查样本末尾是否是某个合法多字节字符的有效前缀。
func isValidUTF8Prefix(prefix []byte) bool {
	if len(prefix) == 0 {
		return false
	}
	lead := prefix[0]
	for i := 1; i < len(prefix); i++ {
		b := prefix[i]
		if b&0xC0 != 0x80 {
			return false
		}
		if i == 1 {
			switch lead {
			case 0xE0:
				if b < 0xA0 {
					return false
				}
			case 0xED:
				if b > 0x9F {
					return false
				}
			case 0xF0:
				if b < 0x90 {
					return false
				}
			case 0xF4:
				if b > 0x8F {
					return false
				}
			}
		}
	}
	return true
}

// guessUTF16 仅在样本包含明显的交错 NUL 字节时识别无 BOM UTF-16，
// 避免对任意非 UTF-8 二进制文件做过度猜测。
func guessUTF16(sample []byte) string {
	limit := min(len(sample), 512)
	if limit < 8 {
		return ""
	}
	var evenZeros, oddZeros int
	for i, value := range sample[:limit] {
		if value == 0 {
			if i%2 == 0 {
				evenZeros++
			} else {
				oddZeros++
			}
		}
	}
	half := float64(limit / 2)
	if half == 0 {
		return ""
	}
	if float64(oddZeros)/half >= 0.25 && float64(evenZeros)/half <= 0.05 {
		return "UTF-16LE"
	}
	if float64(evenZeros)/half >= 0.25 && float64(oddZeros)/half <= 0.05 {
		return "UTF-16BE"
	}
	return ""
}

// LineContext 是上下文窗口中的一行日志。
type LineContext struct {
	LineNumber     int64  `json:"lineNumber"`
	Content        string `json:"content"`
	ContentClipped bool   `json:"contentClipped"`
}

// Match 表示一条匹配结果，以及可选的前后文。
type Match struct {
	LineNumber int64  `json:"lineNumber"`
	Content    string `json:"content"`
	// FilePath 标注该命中的来源文件，仅多文件目录检索填写；单文件检索留空（阶段 6 批次 E）。
	FilePath       string        `json:"filePath,omitempty"`
	ContentClipped bool          `json:"contentClipped"`
	ContextBefore  []LineContext `json:"contextBefore"`
	ContextAfter   []LineContext `json:"contextAfter"`
	Event          *EventContext `json:"event,omitempty"`
}

// Progress 描述扫描进度，供桌面界面轮询展示。
type Progress struct {
	BytesScanned  int64 `json:"bytesScanned"`
	FileSizeBytes int64 `json:"fileSizeBytes"`
	ScannedLines  int64 `json:"scannedLines"`
	MatchesFound  int   `json:"matchesFound"`
	// Matches 只包含自上次进度通知后新出现的匹配，避免反复复制整个结果集。
	Matches []Match `json:"matches,omitempty"`
}

// SearchOptions 是一次检索的可选参数。
// ContextLines 会被限制在 0 到 10 之间，超过上限的值按 10 处理。
type SearchOptions struct {
	ContextLines int `json:"contextLines"`
	// Encoding 可选 auto、utf-8、gb18030、utf-16le、utf-16be；空值等同 auto。
	Encoding string `json:"encoding,omitempty"`
	// MultiLineJSON 将跨行 JSON 对象/数组作为一个文档匹配；单行日志仍按行检索。
	MultiLineJSON bool `json:"multiLineJSON,omitempty"`
	EventContext  bool `json:"eventContext,omitempty"`
	// EventBoundary 指定事件边界规则：timestamp（默认）、blank 或 custom。
	EventBoundary string `json:"eventBoundary,omitempty"`
	// EventBoundaryPattern 是 EventBoundary 为 custom 时使用的 Go 正则。
	EventBoundaryPattern string `json:"eventBoundaryPattern,omitempty"`
	// TimeWindowEnabled 为真时，只保留行首时间戳落在 [Start,End) 内的命中（阶段 6 批次 A）。
	TimeWindowEnabled bool `json:"timeWindowEnabled,omitempty"`
	// TimeWindowStartMs/EndMs 是窗口边界的 Unix 毫秒：下界含、上界不含；EndMs<=0 表示上界开放。
	TimeWindowStartMs int64 `json:"timeWindowStartMs,omitempty"`
	TimeWindowEndMs   int64 `json:"timeWindowEndMs,omitempty"`
	// TimeWindowZone 指定无时区后缀（naive）日志时间戳与窗口边界的解释参照："local"（默认）或 "utc"。
	TimeWindowZone string `json:"timeWindowZone,omitempty"`
	// StartOffset/StartLine 支持“从上次截断处继续”续扫（阶段 6 批次 B）：仅对无需字符解码器的
	// UTF-8/ASCII 文件生效。StartOffset 为续扫起始的原始字节偏移，StartLine 为已处理行数（保证行号连续）。
	// StartOffset<=0 表示从文件正文开头扫描。
	StartOffset int64 `json:"startOffset,omitempty"`
	StartLine   int64 `json:"startLine,omitempty"`
}

// SearchResponse 是一次日志检索的结果摘要。
type SearchResponse struct {
	FilePath string `json:"filePath"`
	// Encoding 是本次检索实际使用的文本编码。
	Encoding           string  `json:"encoding"`
	FileSizeBytes      int64   `json:"fileSizeBytes"`
	BytesScanned       int64   `json:"bytesScanned"`
	ScannedLines       int64   `json:"scannedLines"`
	Matches            []Match `json:"matches"`
	ResultLimitReached bool    `json:"resultLimitReached"`
	Canceled           bool    `json:"canceled"`
	// FromCache 为 true 表示本次结果命中了搜索缓存，直接从落盘 JSON 加载，未重新扫描文件。
	FromCache bool `json:"fromCache"`
	// TimeWindowSkipped 是启用时间窗时，因行首缺少可解析的完整日期时间戳而未计入结果的命中数。
	TimeWindowSkipped int64 `json:"timeWindowSkipped"`
	// ResumeOffset 是达到结果上限后可续扫的原始字节偏移；-1 表示当前不可续扫
	// （已扫描到文件末尾、被取消，或需要字符解码器的 UTF-16/GB18030 编码）。
	ResumeOffset int64 `json:"resumeOffset"`
	// ResumeLine 是续扫时应作为 SearchOptions.StartLine 传入的已处理行数。
	ResumeLine int64 `json:"resumeLine"`
}

// SearchFile 保留简单调用入口。
func SearchFile(path, query string, useRegex bool, maxResults int) (SearchResponse, error) {
	return SearchFileWithOptions(context.Background(), path, query, useRegex, maxResults, SearchOptions{}, nil)
}

// SearchFileContext 支持取消和进度回调，但不附加上下文行。
//
// onProgress 的 Matches 是自上一次通知后的增量结果；如需最终完整上下文，
// 仍应使用 SearchResponse.Matches。
func SearchFileContext(ctx context.Context, path, query string, useRegex bool, maxResults int, onProgress func(Progress)) (SearchResponse, error) {
	return SearchFileWithOptions(ctx, path, query, useRegex, maxResults, SearchOptions{}, onProgress)
}

// SearchFileWithOptions 使用固定大小的读取缓冲区扫描文件。
// onProgress 在扫描达到进度步长或搜索结束时调用，新发现的 Match 会随 Progress.Matches 增量发送。
// 普通文本匹配只需保留关键字长度相关的边界字节，不拼接整行；结果预览和上下文窗口有界。
// 正则模式按整行匹配，单行最多允许 4 MiB，避免异常日志造成不受控的内存占用。
func SearchFileWithOptions(ctx context.Context, path, query string, useRegex bool, maxResults int, options SearchOptions, onProgress func(Progress)) (SearchResponse, error) {
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
	contextLines := options.ContextLines
	if contextLines < 0 {
		contextLines = 0
	}
	if contextLines > 10 {
		contextLines = 10
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

	// 事件边界规则在读取文件前解析，非法的自定义正则可以直接失败。
	var eventRule eventBoundaryRule
	if options.EventContext {
		resolved, resolveErr := resolveEventBoundary(options.EventBoundary, options.EventBoundaryPattern)
		if resolveErr != nil {
			return SearchResponse{}, resolveErr
		}
		eventRule = resolved
	}

	file, err := os.Open(path)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("打开日志文件失败：%w", err)
	}
	// 只读打开，关闭失败无需上抛。
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return SearchResponse{}, fmt.Errorf("读取日志文件信息失败：%w", err)
	}

	// 编码检测缓存：size+mtime 未变时复用上次检测结果，跳过 64KB 样本读取与启发式检测。
	cachedEnc, cachedBom, cacheHit := defaultEncodingIndex.Lookup(path, info.Size(), info.ModTime())

	var encodingName string
	var bomBytes int64
	var decoder textencoding.Encoding

	if cacheHit && (options.Encoding == "" || strings.EqualFold(options.Encoding, "auto")) {
		encodingName = cachedEnc
		bomBytes = cachedBom
		decoder = decoderForEncoding(encodingName)
	} else {
		sampleSize := min(info.Size(), int64(maxEncodingSampleBytes))
		sample := make([]byte, int(sampleSize))
		if len(sample) > 0 {
			n, sampleErr := file.ReadAt(sample, 0)
			if sampleErr != nil && sampleErr != io.EOF {
				return SearchResponse{}, fmt.Errorf("读取日志编码样本失败：%w", sampleErr)
			}
			sample = sample[:n]
		}
		var encErr error
		encodingName, bomBytes, decoder, encErr = resolveTextEncoding(sample, options.Encoding, int64(len(sample)) == info.Size())
		if encErr != nil {
			return SearchResponse{}, encErr
		}
		defaultEncodingIndex.Store(path, info.Size(), info.ModTime(), encodingName, bomBytes)
	}
	if isParallelizable(path, useRegex, options, decoder != nil, info.Size()) {
		if err := file.Close(); err != nil {
			return SearchResponse{}, fmt.Errorf("关闭日志文件失败：%w", err)
		}
		return parallelSearchFile(ctx, path, query, maxResults, encodingName, info.Size(), bomBytes, options, onProgress)
	}

	// 续扫只在无需字符解码器（UTF-8/ASCII）时按原始字节偏移定位；解码器文件从头扫描，
	// 因为解码后的行边界无法映射回精确的原始偏移。
	resumable := decoder == nil
	startBytes := bomBytes
	if resumable && options.StartOffset > 0 {
		startBytes = options.StartOffset
	}
	if _, err := file.Seek(startBytes, io.SeekStart); err != nil {
		return SearchResponse{}, fmt.Errorf("定位日志正文失败：%w", err)
	}

	var rawCounter *byteCountingReader
	var scanInput io.Reader = file
	if decoder != nil {
		rawCounter = &byteCountingReader{source: file}
		scanInput = transform.NewReader(rawCounter, decoder.NewDecoder())
	}
	reader := bufio.NewReaderSize(scanInput, readerBufferSize)
	// 时间窗过滤器在三条匹配路径入口统一构造，未启用时 allows 恒通过。
	window := newTimeWindowFilter(options)
	response := SearchResponse{
		FilePath:      path,
		Encoding:      encodingName,
		FileSizeBytes: info.Size(),
		Matches:       make([]Match, 0, min(maxResults, 128)),
		ResumeOffset:  -1,
		ScannedLines:  options.StartLine,
	}
	if options.MultiLineJSON {
		return searchMultiLineJSON(ctx, reader, response, query, matcher, maxResults, window, startBytes, resumable, onProgress)
	}
	if options.EventContext {
		return searchEventContext(ctx, reader, response, query, matcher, maxResults, eventRule, window, startBytes, resumable, onProgress)
	}

	lineNumber := options.StartLine
	var lineBytes int64
	bytesScanned := startBytes
	var lastProgressBytes int64
	var reportedMatchCount int
	var preview []byte
	var regexLine []byte
	var carry []byte
	var pendingCR bool
	var lineOpen bool
	var lineMatched bool
	var reachedLimit bool
	history := make([]LineContext, 0, contextLines)
	pendingContext := make([]int, 0, 16)

	notify := func(force bool) {
		if onProgress == nil {
			return
		}
		if !force && bytesScanned-lastProgressBytes < progressStepBytes {
			return
		}
		lastProgressBytes = bytesScanned
		// 上报自上次通知以来新增的匹配；拷贝上下文切片，避免后续行继续补齐
		// ContextAfter 时修改已经交给调用方的快照。
		matchUpdates := make([]Match, len(response.Matches)-reportedMatchCount)
		for i, match := range response.Matches[reportedMatchCount:] {
			match.ContextBefore = append([]LineContext(nil), match.ContextBefore...)
			match.ContextAfter = append([]LineContext(nil), match.ContextAfter...)
			matchUpdates[i] = match
		}
		reportedMatchCount = len(response.Matches)
		onProgress(Progress{
			BytesScanned:  bytesScanned,
			FileSizeBytes: info.Size(),
			ScannedLines:  response.ScannedLines,
			MatchesFound:  len(response.Matches),
			Matches:       matchUpdates,
		})
	}

	// appendContent 持续更新当前行预览与匹配状态；普通文本模式不缓存整行。
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
				return errors.New("当前日志包含超过 4 MiB 的单行内容；为避免占用过多内存，正则模式已停止，请改用普通文本搜索")
			}
			regexLine = append(regexLine, part...)
			return nil
		}

		if !lineMatched {
			if bytes.Contains(part, needle) {
				lineMatched = true
			} else {
				// 只有跨块匹配才需要拼接边界：最多分配关键字长度左右的少量内存。
				keep := len(needle) - 1
				prefixLen := min(len(part), keep)
				if keep > 0 && len(carry) > 0 && prefixLen > 0 {
					cross := make([]byte, 0, len(carry)+prefixLen)
					cross = append(cross, carry...)
					cross = append(cross, part[:prefixLen]...)
					lineMatched = bytes.Contains(cross, needle)
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
		return nil
	}

	// finishLine 统一处理换行、匹配结果、前后文窗口以及每行临时缓冲区复用。
	finishLine := func() {
		lineNumber++
		response.ScannedLines = lineNumber
		if matcher != nil {
			lineMatched = matcher.Match(regexLine)
		}

		// 未命中且未请求上下文时，不转换预览为字符串，减少海量日志扫描的分配次数。
		needContent := contextLines > 0 || lineMatched
		content := ""
		clipped := lineBytes > maxPreviewBytes
		if needContent {
			content = strings.ToValidUTF8(string(preview), "�")
		}
		contextContent := content
		contextClipped := clipped
		if contextLines > 0 && len(contextContent) > maxContextBytes {
			contextContent = contextContent[:maxContextBytes]
			contextContent = strings.ToValidUTF8(contextContent, "�")
			contextClipped = true
		}
		currentLine := LineContext{LineNumber: lineNumber, Content: contextContent, ContentClipped: contextClipped}

		// 为之前命中的结果追加当前行作为后文；已收集够上下文的结果不再保留。
		if contextLines > 0 && len(pendingContext) > 0 {
			nextPending := pendingContext[:0]
			for _, matchIndex := range pendingContext {
				m := &response.Matches[matchIndex]
				if len(m.ContextAfter) < contextLines {
					m.ContextAfter = append(m.ContextAfter, currentLine)
				}
				if len(m.ContextAfter) < contextLines {
					nextPending = append(nextPending, matchIndex)
				}
			}
			pendingContext = nextPending
		}

		if lineMatched {
			// 时间窗只限定哪些命中入选，不影响上下文历史：历史仍逐行累积。
			inWindow := true
			if window.enabled {
				within, hasStamp := window.allows(preview)
				if !hasStamp {
					response.TimeWindowSkipped++
				}
				inWindow = within
			}
			if inWindow {
				before := append([]LineContext{}, history...)
				response.Matches = append(response.Matches, Match{
					LineNumber:     lineNumber,
					Content:        content,
					ContentClipped: clipped,
					ContextBefore:  before,
					ContextAfter:   []LineContext{},
				})
				if contextLines > 0 {
					pendingContext = append(pendingContext, len(response.Matches)-1)
				}
				if len(response.Matches) >= maxResults {
					response.ResultLimitReached = true
					reachedLimit = true
				}
			}
		}

		if contextLines > 0 {
			history = append(history, currentLine)
			if len(history) > contextLines {
				history = append(history[:0], history[len(history)-contextLines:]...)
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

	// 主循环以固定读取缓冲区向前扫描。取消在每次读取迭代检查，
	// 因此大文件可在下一检查点停止并保留此前已累计的结果。
	for {
		if ctx.Err() != nil {
			response.Canceled = true
			response.BytesScanned = bytesScanned
			notify(true)
			return response, nil
		}

		chunk, readErr := reader.ReadSlice('\n')
		if rawCounter != nil {
			// 解码器可能预读少量源字节，因此此处表示底层已读取的原始字节数。
			bytesScanned = bomBytes + rawCounter.bytesRead
		} else {
			bytesScanned += int64(len(chunk))
		}
		if bytesScanned > info.Size() {
			bytesScanned = info.Size()
		}
		hasNewline := len(chunk) > 0 && chunk[len(chunk)-1] == '\n'
		part := chunk
		if hasNewline {
			part = part[:len(part)-1]
		}
		if hasNewline && len(part) > 0 && part[len(part)-1] == '\r' {
			part = part[:len(part)-1]
		}

		// 兼容 CRLF 跨读取缓冲区边界的情况。
		if pendingCR {
			if !hasNewline || len(part) > 0 {
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
				if resumable && bytesScanned < info.Size() {
					response.ResumeOffset = bytesScanned
					response.ResumeLine = response.ScannedLines
				}
				notify(true)
				return response, nil
			}
		}
		if readErr == bufio.ErrBufferFull {
			notify(false)
			continue
		}
		if readErr == io.EOF {
			if lineOpen {
				finishLine()
			}
			break
		}
		if readErr != nil {
			return SearchResponse{}, fmt.Errorf("读取日志文件失败：%w", readErr)
		}
		notify(false)
	}

	// 成功到达 EOF 时，以原始文件大小作为最终进度，避免字符解码导致字节计数差异。
	bytesScanned = info.Size()
	response.BytesScanned = bytesScanned
	notify(true)
	return response, nil
}