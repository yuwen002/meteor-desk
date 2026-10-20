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

  const rawISO = line.match(isoPrefix)?.[1]
  if (rawISO) {
    const normalized = rawISO.replace(',', '.').replace(/([+-]\d{2})(\d{2})$/, '$1:$2')
    const parsed = Date.parse(normalized)
    return Number.isFinite(parsed) ? parsed : null
  }

  const apache = line.match(apachePrefix)?.[1]?.match(/^(\d{2})\/([A-Za-z]{3})\/(\d{4}):(\d{2}):(\d{2}):(\d{2}) ([+-])(\d{2})(\d{2})$/)
  if (!apache) return null
  const months: Record<string, string> = { Jan: '01', Feb: '02', Mar: '03', Apr: '04', May: '05', Jun: '06', Jul: '07', Aug: '08', Sep: '09', Oct: '10', Nov: '11', Dec: '12' }
  const [, day, monthName, year, hour, minute, second, sign, zoneHour, zoneMinute] = apache
  const month = months[monthName]
  if (!month) return null
  const parsed = Date.parse(year + '-' + month + '-' + day + 'T' + hour + ':' + minute + ':' + second + sign + zoneHour + ':' + zoneMinute)
  return Number.isFinite(parsed) ? parsed : null
}

function boundary(value: string, isEnd: boolean): number {
  const timestamp = new Date(value).getTime()
  if (!Number.isFinite(timestamp)) return Number.NaN
  return isEnd && value.length === 16 && value[10] === 'T'
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
