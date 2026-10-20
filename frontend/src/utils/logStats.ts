import { classifyLogLevel, type LogLevel } from './logLevel'

export type LogStatsInput = { content: string }
export type LogTrendBucket = { label: string; info: number; warn: number; error: number }
export type RepeatedError = { key: string; message: string; count: number }
export type LogStats = {
  total: number
  info: number
  warn: number
  error: number
  unknown: number
  trend: LogTrendBucket[]
  repeatedErrors: RepeatedError[]
}

function timestampOf(line: string): Date | null {
  let candidate = ''
  const trimmed = line.trim()
  if (trimmed.startsWith('{')) {
    try {
      const parsed: unknown = JSON.parse(trimmed)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        const record = parsed as Record<string, unknown>
        const raw = record.timestamp ?? record.time ?? record.ts ?? record['@timestamp']
        if (typeof raw === 'string' || typeof raw === 'number') {
          const value = typeof raw === 'number' && raw > 0 && raw < 1e12 ? raw * 1000 : raw
          const date = new Date(value)
          if (!Number.isNaN(date.getTime())) return date
        }
      }
    } catch { /* fall through to common text timestamp formats */ }
  }
  const match = trimmed.match(/\b(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:Z|[+-]\d{2}:?\d{2})?)/)
  if (match) candidate = match[1]
  if (!candidate) return null
  const date = new Date(candidate)
  return Number.isNaN(date.getTime()) ? null : date
}

function normalizeError(line: string): string {
  let source = line.trim()
  // JSON logs often put the changing timestamp and metadata outside the actual error.
  // Prefer the message field so identical failures can be grouped across requests.
  if (source.startsWith('{')) {
    try {
      const parsed: unknown = JSON.parse(source)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        const record = parsed as Record<string, unknown>
        const message = record.message ?? record.msg ?? record.error ?? record.exception
        if (typeof message === 'string') source = message
        else if (message && typeof message === 'object') source = JSON.stringify(message)
      }
    } catch { /* Keep the original line for malformed JSON-like log messages. */ }
  }

  return source
    .replace(/^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?\s*/, '')
    .replace(/\b[0-9a-f]{8}-[0-9a-f-]{27,}\b/gi, '<UUID>')
    .replace(/\b0x[0-9a-f]+\b/gi, '<HEX>')
    .replace(/\b\d{4,}\b/g, '<N>')
    .replace(/\s+/g, ' ')
    .slice(0, 240)
}

function levelOf(line: string): LogLevel | null {
  return classifyLogLevel(line)
}

/** Summarize the supplied preview/result set only; this does not rescan the source file. */
export function summarizeLogLines(lines: LogStatsInput[]): LogStats {
  const stats: LogStats = { total: lines.length, info: 0, warn: 0, error: 0, unknown: 0, trend: [], repeatedErrors: [] }
  const minuteBuckets = new Map<number, LogTrendBucket>()
  const errors = new Map<string, RepeatedError>()

  for (const item of lines) {
    const line = item.content ?? ''
    const level = levelOf(line)
    if (level === 'INFO') stats.info++
    else if (level === 'WARN') stats.warn++
    else if (level === 'ERROR') stats.error++
    else stats.unknown++

    const timestamp = timestampOf(line)
    if (timestamp) {
      const minute = Math.floor(timestamp.getTime() / 60000) * 60000
      let bucket = minuteBuckets.get(minute)
      if (!bucket) {
        bucket = { label: new Date(minute).toISOString().slice(0, 16).replace('T', ' '), info: 0, warn: 0, error: 0 }
        minuteBuckets.set(minute, bucket)
      }
      if (level === 'INFO') bucket.info++
      else if (level === 'WARN') bucket.warn++
      else if (level === 'ERROR') bucket.error++
    }

    if (level === 'ERROR') {
      const message = normalizeError(line)
      if (message) {
        const key = message.toLowerCase()
        const existing = errors.get(key)
        if (existing) existing.count++
        else errors.set(key, { key, message, count: 1 })
      }
    }
  }

  stats.trend = [...minuteBuckets.entries()].sort((a, b) => a[0] - b[0]).slice(-60).map(([, bucket]) => bucket)
  stats.repeatedErrors = [...errors.values()].sort((a, b) => b.count - a.count || a.message.localeCompare(b.message)).slice(0, 10)
  return stats
}
