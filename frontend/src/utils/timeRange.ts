export interface TimeRange {
  from?: string
  to?: string
}

const isoPrefix = /^\s*(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?)/i
const apachePrefix = /^\s*\[?(\d{2}\/[A-Za-z]{3}\/\d{4}:\d{2}:\d{2}:\d{2} [+-]\d{4})\]?/
const jsonTimestamp = /["'](?:@timestamp|timestamp|time)["']\s*:\s*["']([^"']+)["']/i

export function extractLogTimestamp(line: string): number | null {
  const jsonValue = line.match(jsonTimestamp)?.[1]
  if (jsonValue) {
    const parsed = Date.parse(jsonValue)
    if (Number.isFinite(parsed)) return parsed
  }

  const raw = line.match(isoPrefix)?.[1] ?? line.match(apachePrefix)?.[1]
  if (!raw) return null
  const normalized = raw.replace(',', '.').replace(/([+-]\d{2})(\d{2})$/, '$1:$2')
  const parsed = Date.parse(normalized)
  return Number.isFinite(parsed) ? parsed : null
}

function boundary(value: string, isEnd: boolean): number {
  const timestamp = new Date(value).getTime()
  if (!Number.isFinite(timestamp)) return Number.NaN
  return isEnd && /^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}$/.test(value)
    ? timestamp + 60_000 - 1
    : timestamp
}

export function isWithinTimeRange(line: string, range: TimeRange): boolean {
  if (!range.from && !range.to) return true
  const timestamp = extractLogTimestamp(line)
  if (timestamp === null) return false

  const from = range.from ? boundary(range.from, false) : Number.NEGATIVE_INFINITY
  const to = range.to ? boundary(range.to, true) : Number.POSITIVE_INFINITY
  if ((range.from && !Number.isFinite(from)) || (range.to && !Number.isFinite(to)) || from > to) return false
  return timestamp >= from && timestamp <= to
}

export function filterLogLinesByTimeRange<T extends { content: string }>(
  lines: readonly T[],
  range: TimeRange,
): T[] {
  if (!range.from && !range.to) return [...lines]
  return lines.filter((line) => isWithinTimeRange(line.content, range))
}
