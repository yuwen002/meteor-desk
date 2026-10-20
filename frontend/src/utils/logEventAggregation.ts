export type LogEvent = {
  startLine: number
  endLine: number
  level: 'INFO' | 'WARN' | 'ERROR' | 'UNKNOWN'
  timestamp: string | null
  content: string
  truncated: boolean
}
export type LogEventOptions = {
  maxEventBytes?: number
  /** A custom timestamp expression matched at the beginning of a line. */
  timestampPattern?: string
  /** Start a new event when a recognized level token appears at the beginning. */
  splitOnLevel?: boolean
}
const DEFAULT_MAX_EVENT_BYTES = 64 * 1024
const LEVEL = /^\s*(?:\[[^\]]+\]\s*)?(?:\d{4}-\d{2}-\d{2}[T ][^ ]+\s+)?(INFO|INFORMATION|WARN|WARNING|ERROR|FATAL|CRITICAL|DEBUG|TRACE)\b/i
const TIMESTAMP = /^\s*(\d{4}-\d{2}-\d{2}[T ][0-9:.+-]+Z?)/
const CONTINUATION = /^\s+(?:at\s+|\.\.\. \d+ more|Caused by:|Suppressed:|goroutine \d+|File ".+", line \d+|Traceback \(most recent call last\):)|^\s*\^/
function levelOf(line: string): LogEvent['level'] {
  const match = line.match(LEVEL)
  if (!match) return 'UNKNOWN'
  const level = match[1].toUpperCase()
  if (level === 'INFO' || level === 'INFORMATION') return 'INFO'
  if (level === 'WARN' || level === 'WARNING') return 'WARN'
  if (level === 'ERROR' || level === 'FATAL' || level === 'CRITICAL') return 'ERROR'
  return 'UNKNOWN'
}
function timestampOf(line: string, pattern?: RegExp): string | null {
  const match = pattern ? line.match(pattern) : line.match(TIMESTAMP)
  return match?.[1] ?? null
}
/**
 * Group line-oriented logs into bounded events. A new timestamped record or
 * recognized level header starts a new event; stack traces and indented lines
 * remain attached to the preceding event. Original line numbers are preserved.
 */
export function aggregateLogEvents(input: string, options: LogEventOptions = {}): LogEvent[] {
  if (!input) return []
  const maxBytes = Number.isFinite(options.maxEventBytes) && (options.maxEventBytes ?? 0) > 0
    ? Math.floor(options.maxEventBytes!)
    : DEFAULT_MAX_EVENT_BYTES
  let timestampPattern: RegExp | undefined
  if (options.timestampPattern?.trim()) {
    try { timestampPattern = new RegExp(options.timestampPattern) } catch { timestampPattern = undefined }
  }
  const splitOnLevel = options.splitOnLevel ?? true
  const lines = input.split(/\r?\n/)
  const events: LogEvent[] = []
  let current: LogEvent | null = null
  const flush = () => {
    if (current) events.push(current)
    current = null
  }
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    if (i === lines.length - 1 && line === '') break
    const level = levelOf(line)
    const timestamp = timestampOf(line, timestampPattern)
    const startsNew = current === null || (timestamp !== null && current.content.length > 0) ||
      (splitOnLevel && level !== 'UNKNOWN' && current.level !== 'UNKNOWN' && !CONTINUATION.test(line))
    if (startsNew) {
      flush()
      current = { startLine: i + 1, endLine: i + 1, level, timestamp, content: '', truncated: false }
    }
    if (!current) continue
    current.endLine = i + 1
    const separator = current.content ? '\n' : ''
    const next = separator + line
    const remaining = maxBytes - current.content.length
    if (remaining <= 0) current.truncated = true
    else if (next.length > remaining) {
      current.content += next.slice(0, remaining)
      current.truncated = true
    } else current.content += next
  }
  flush()
  return events
}
