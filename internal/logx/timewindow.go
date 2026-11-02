package logx

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// leadingTimestamp 匹配日志行首部常见的时间戳，用于绝对/相对时间窗过滤。
// 支持：
//   - 仅日期：2026-10-30、2026/10/30
//   - 日期+时间：2026-10-30 10:00:00、ISO 的 T 分隔、可选毫秒/微秒（. 或 , 分隔）
//   - 可选时区后缀：Z 或 +08:00 / -0800
//   - 可选的前导空白与左方括号（如 "[2026-10-30 ..."）
//
// 与事件边界所用的 eventTimestamp 不同，这里需要捕获各分量以换算成瞬间。
// 只有时间没有日期（如 HH:mm:ss）的行无法落进绝对日期区间，按“无可用时间戳”处理。
var leadingTimestamp = regexp.MustCompile(`^[\s\[]*(\d{4})[-/](\d{1,2})[-/](\d{1,2})(?:[ T](\d{1,2}):(\d{1,2}):(\d{1,2})(?:[.,](\d{1,9}))?(Z|[+-]\d{2}:?\d{2})?)?`)

// timeWindowFilter 在启用时间窗时把命中限定到行首时间戳落在 [startMs,endMs) 区间内的结果。
// zone 是无时区后缀（naive）时间戳与窗口边界的共同解释参照：time.Local 或 time.UTC。
type timeWindowFilter struct {
	enabled bool
	startMs int64 // 含下界（Unix 毫秒）
	endMs   int64 // 不含上界；<=0 表示上界开放
	zone    *time.Location
}

// newTimeWindowFilter 依据检索参数构造时间窗过滤器。未启用时 allows 恒通过，主逻辑无需分支。
func newTimeWindowFilter(options SearchOptions) timeWindowFilter {
	if !options.TimeWindowEnabled {
		return timeWindowFilter{enabled: false}
	}
	zone := time.Local
	if strings.EqualFold(strings.TrimSpace(options.TimeWindowZone), "utc") {
		zone = time.UTC
	}
	return timeWindowFilter{enabled: true, startMs: options.TimeWindowStartMs, endMs: options.TimeWindowEndMs, zone: zone}
}

// allows 判断给定内容（单行、事件或文档的起始文本）是否落在时间窗内。
// within 为 false 表示该内容不在窗口内（或无时间戳）；hasStamp 为 false 表示行首
// 无法解析出带日期的时间戳，调用方据此把它计入“因缺少时间戳而被跳过”的数量。
func (f timeWindowFilter) allows(content []byte) (within bool, hasStamp bool) {
	if !f.enabled {
		return true, true
	}
	ms, ok := parseLeadingTimestampMillis(content, f.zone)
	if !ok {
		return false, false
	}
	if ms < f.startMs {
		return false, true
	}
	if f.endMs > 0 && ms >= f.endMs {
		return false, true
	}
	return true, true
}

// parseLeadingTimestampMillis 把行首时间戳换算为 Unix 毫秒。
// 带显式时区后缀（Z 或 ±偏移）的按其瞬间换算；naive 时间戳按 naiveZone 解释。
// 无法匹配或缺少日期时返回 ok=false。
func parseLeadingTimestampMillis(content []byte, naiveZone *time.Location) (int64, bool) {
	groups := leadingTimestamp.FindSubmatch(content)
	if groups == nil {
		return 0, false
	}
	atoi := func(b []byte) int {
		n, _ := strconv.Atoi(string(b))
		return n
	}
	year, month, day := atoi(groups[1]), atoi(groups[2]), atoi(groups[3])
	hour, minute, second := 0, 0, 0
	if len(groups[4]) > 0 {
		hour, minute, second = atoi(groups[4]), atoi(groups[5]), atoi(groups[6])
	}
	// 拒绝明显越界的分量（time.Date 会归一化而非报错，这里主动拦截异常输入）。
	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 || second > 60 {
		return 0, false
	}
	location := naiveZone
	// 捕获组 7 是可选的小数秒，8 才是时区后缀（Z / ±hh:mm / ±hhmm）。
	if offset := groups[8]; len(offset) > 0 {
		seconds, ok := parseZoneOffsetSeconds(offset)
		if !ok {
			return 0, false
		}
		location = time.FixedZone("", seconds)
	}
	moment := time.Date(year, time.Month(month), day, hour, minute, second, 0, location)
	return moment.UnixMilli(), true
}

// parseZoneOffsetSeconds 解析 Z、+hh:mm、-hhmm、+hh 形式的时区后缀为相对 UTC 的秒数。
func parseZoneOffsetSeconds(offset []byte) (int, bool) {
	text := string(offset)
	if text == "Z" || text == "z" {
		return 0, true
	}
	sign := 1
	switch text[0] {
	case '+':
		text = text[1:]
	case '-':
		sign = -1
		text = text[1:]
	default:
		return 0, false
	}
	var hour, minute int
	if len(text) == 4 {
		hour = mustAtoi(text[:2])
		minute = mustAtoi(text[2:])
	} else if colon := strings.IndexByte(text, ':'); colon >= 0 {
		hour = mustAtoi(text[:colon])
		minute = mustAtoi(text[colon+1:])
	} else {
		hour = mustAtoi(text)
	}
	if hour > 23 || minute > 59 {
		return 0, false
	}
	return sign * (hour*3600 + minute*60), true
}

func mustAtoi(text string) int {
	n, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return n
}
