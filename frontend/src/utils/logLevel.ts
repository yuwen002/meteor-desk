export type LogLevel = 'INFO' | 'WARN' | 'ERROR'
export type TailLevelFilter = 'ALL' | LogLevel

const levelAliases: Record<string, LogLevel> = {
  INFO: 'INFO',
  INFORMATION: 'INFO',
  WARN: 'WARN',
  WARNING: 'WARN',
  ERROR: 'ERROR',
  FATAL: 'ERROR',
  CRITICAL: 'ERROR',
}

const recognizedLevels = new Set(Object.keys(levelAliases))
const tokenPattern = /(?:^|[\\s\\[({,:])"?(?:level|severity)"?\\s*[=:]\\s*["']?(INFO|INFORMATION|WARN|WARNING|ERROR|FATAL|CRITICAL)\\b/i
const bracketPattern = /(?:^|\\s)\\[(INFO|INFORMATION|WARN|WARNING|ERROR|FATAL|CRITICAL)\\](?:\\s|$)/i
const prefixPattern = /^\\s*(?:\\d{4}-\\d{2}-\\d{2}[T ]\\S+\\s+)?(INFO|INFORMATION|WARN|WARNING|ERROR|FATAL|CRITICAL)\\b/i

function normalizeLevel(value: unknown): LogLevel | null {
  if (typeof value !== 'string') return null
  const normalized = value.trim().toUpperCase()
  return recognizedLevels.has(normalized) ? levelAliases[normalized] : null
}

function classifyJSONLevel(line: string): LogLevel | null | undefined {
  const trimmed = line.trim()
  if (!trimmed.startsWith('{')) return undefined

  try {
    const parsed: unknown = JSON.parse(trimmed)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null

    const record = parsed as Record<string, unknown>
    // A valid JSON object is classified only from its actual level/severity fields.
    // Never scan message text for level-like tokens: that creates false positives.
    return normalizeLevel(record.level) ?? normalizeLevel(record.severity)
  } catch {
    return undefined
  }
}

export function classifyLogLevel(line: string): LogLevel | null {
  const jsonLevel = classifyJSONLevel(line)
  if (jsonLevel !== undefined) return jsonLevel

  const match = line.match(tokenPattern) ?? line.match(bracketPattern) ?? line.match(prefixPattern)
  return match ? levelAliases[match[1].toUpperCase()] ?? null : null
}

export function filterTailLines(text: string, filter: TailLevelFilter): string {
  if (filter === 'ALL' || !text) return text
  return text.split('\\n').filter((line) => classifyLogLevel(line) === filter).join('\\n')
}
