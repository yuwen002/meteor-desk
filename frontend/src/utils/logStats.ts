export type LogSeverity = 'error' | 'warn' | 'info' | 'debug' | 'other'

export interface LogStatsInput {
  lineNumber: number
  content: string
  event?: { startLine: number; endLine: number; content: string; contentClipped: boolean }
}

export interface DuplicateError {
  signature: string
  message: string
  count: number
  firstLine: number
  lines: number[]
}

export interface LogStats {
  total: number
  errors: number
  warnings: number
  info: number
  debug: number
  other: number
  duplicates: DuplicateError[]
  trend: Array<{ bucket: string; errors: number; warnings: number }>
}

const timestampPrefix = /^\s*(?:\[[^\]]+\]\s*)?(?:\d{4}[-/]\d{1,2}[-/]\d{1,2}[ T]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?\s*)/i
const levelPattern = /\b(TRACE|DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL|PANIC)\b/i
const stackLine = /^(?:at\s+\S+|\s*at\s+|Caused by:|Suppressed:|\.\.\. \d+ more)/i

export function classifyLogSeverity(content: string): LogSeverity {
  const line = content.replace(timestampPrefix, '').trim()
  const match = line.match(levelPattern)
  if (!match) return 'other'
  const level = match[1].toUpperCase()
  if (level === 'ERROR' || level === 'FATAL' || level === 'PANIC') return 'error'
  if (level === 'WARN' || level === 'WARNING') return 'warn'
  if (level === 'INFO') return 'info'
  if (level === 'DEBUG' || level === 'TRACE') return 'debug'
  return 'other'
}

function eventMessage(input: LogStatsInput): string {
  const content = input.event?.content ?? input.content
  const lines = content.split(/\r?\n/).map(line => line.trim()).filter(Boolean)
  return (lines.find(line => !stackLine.test(line) && levelPattern.test(line)) ?? lines.find(line => !stackLine.test(line)) ?? lines[0] ?? '').replace(timestampPrefix, '').trim()
}

function normalizeSignature(message: string): string {
  return message
    .replace(timestampPrefix, '')
    .replace(/\b[0-9a-f]{8}-[0-9a-f-]{27,}\b/gi, '<UUID>')
    .replace(/\b0x[0-9a-f]+\b/gi, '<HEX>')
    .replace(/\b\d{4,}\b/g, '<N>')
    .replace(/\b(?:request|trace|span|session|user|order|id)[_-]?[=: ]+[\w-]+/gi, '$1=<ID>')
    .replace(/\s+/g, ' ')
    .trim()
    .toLowerCase()
}

function minuteBucket(content: string): string | null {
  const match = content.match(/(?:^|\s|\[)(\d{4}[-/]\d{1,2}[-/]\d{1,2}[ T]\d{2}:\d{2}|\d{2}:\d{2})/)
  if (!match) return null
  return match[1].replace('T', ' ').replace(/\//g, '-').slice(-5)
}

export function summarizeLogMatches(matches: LogStatsInput[]): LogStats {
  const stats: LogStats = { total: matches.length, errors: 0, warnings: 0, info: 0, debug: 0, other: 0, duplicates: [], trend: [] }
  const duplicateMap = new Map<string, DuplicateError>()
  const trendMap = new Map<string, { bucket: string; errors: number; warnings: number }>()

  for (const match of matches) {
    const content = match.event?.content ?? match.content
    const severity = classifyLogSeverity(content)
    if (severity === 'error') stats.errors++
    else if (severity === 'warn') stats.warnings++
    else if (severity === 'info') stats.info++
    else if (severity === 'debug') stats.debug++
    else stats.other++

    const bucket = minuteBucket(content)
    if (bucket && (severity === 'error' || severity === 'warn')) {
      const item = trendMap.get(bucket) ?? { bucket, errors: 0, warnings: 0 }
      if (severity === 'error') item.errors++
      else item.warnings++
      trendMap.set(bucket, item)
    }

    if (severity !== 'error') continue
    const message = eventMessage(match)
    const signature = normalizeSignature(message)
    if (!signature) continue
    const existing = duplicateMap.get(signature)
    if (existing) {
      existing.count++
      existing.lines.push(match.event?.startLine ?? match.lineNumber)
    } else {
      duplicateMap.set(signature, { signature, message, count: 1, firstLine: match.event?.startLine ?? match.lineNumber, lines: [match.event?.startLine ?? match.lineNumber] })
    }
  }

  stats.duplicates = [...duplicateMap.values()].filter(item => item.count > 1).sort((a, b) => b.count - a.count || a.firstLine - b.firstLine).slice(0, 8)
  stats.trend = [...trendMap.values()].sort((a, b) => a.bucket.localeCompare(b.bucket)).slice(-12)
  return stats
}
