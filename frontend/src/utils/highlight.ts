// 结果高亮：只把命中区间切成纯文本片段，由模板渲染，不拼接 HTML，避免日志内容被当作标记执行。
export interface HighlightSegment {
  text: string
  hit: boolean
}

const MAX_SEGMENTS = 200

function textRanges(text: string, needle: string, max: number): [number, number][] {
  if (!needle) return []
  const ranges: [number, number][] = []
  let from = 0
  while (ranges.length < max) {
    // 与后端文本匹配保持一致：区分大小写。
    const at = text.indexOf(needle, from)
    if (at < 0) break
    ranges.push([at, at + needle.length])
    from = at + needle.length
  }
  return ranges
}

function regexRanges(text: string, pattern: string, max: number): [number, number][] {
  let source = pattern
  let flags = 'g'
  // Go 正则常用行首内联标志，JavaScript 只支持在 flags 中表达，这里转换最常见的 (?i)。
  if (source.startsWith('(?i)')) {
    source = source.slice(4)
    flags += 'i'
  }
  let re: RegExp
  try {
    re = new RegExp(source, flags)
  } catch {
    // Go 与 JavaScript 的正则语法不完全一致；无法编译时只展示原文，不猜测命中位置。
    return []
  }
  const ranges: [number, number][] = []
  let match: RegExpExecArray | null
  while ((match = re.exec(text)) && ranges.length < max) {
    if (!match[0].length) {
      re.lastIndex++
      continue
    }
    ranges.push([match.index, match.index + match[0].length])
  }
  return ranges
}

function buildSegments(text: string, ranges: [number, number][]): HighlightSegment[] {
  const segments: HighlightSegment[] = []
  let cursor = 0
  for (const [start, end] of ranges) {
    if (start > cursor) segments.push({ text: text.slice(cursor, start), hit: false })
    segments.push({ text: text.slice(start, end), hit: true })
    cursor = end
  }
  if (cursor < text.length) segments.push({ text: text.slice(cursor), hit: false })
  return segments
}

export function highlightSegments(text: string, query: string, useRegex: boolean, maxSegments = MAX_SEGMENTS): HighlightSegment[] {
  if (!text || !query) return [{ text, hit: false }]
  const ranges = useRegex ? regexRanges(text, query, maxSegments) : textRanges(text, query, maxSegments)
  if (!ranges.length) return [{ text, hit: false }]
  return buildSegments(text, ranges)
}
