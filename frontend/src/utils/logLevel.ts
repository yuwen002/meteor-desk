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

const tokenPattern = /(?:^|[\s\[({,:])(?:level|severity)\s*[=:]\s*["']?(INFO|INFORMATION|WARN|WARNING|ERROR|FATAL|CRITICAL)\b/i
const bracketPattern = /(?:^|\s)\[(INFO|INFORMATION|WARN|WARNING|ERROR|FATAL|CRITICAL)\](?:\s|$)/i
const prefixPattern = /^\s*(?:\S+\s+){0,3}(INFO|INFORMATION|WARN|WARNING|ERROR|FATAL|CRITICAL)\b/i

export function classifyLogLevel(line: string): LogLevel | null {
  const match = line.match(tokenPattern) ?? line.match(bracketPattern) ?? line.match(prefixPattern)
  return match ? levelAliases[match[1].toUpperCase()] ?? null : null
}

export function filterTailLines(text: string, filter: TailLevelFilter): string {
  if (filter === 'ALL' || !text) return text
  return text.split('\n').filter((line) => classifyLogLevel(line) === filter).join('\n')
}
