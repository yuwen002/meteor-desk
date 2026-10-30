export type LogLevel = 'error' | 'warn' | 'info' | 'debug' | 'trace' | 'other'

export interface LevelCount { level: LogLevel; count: number }
export interface SignatureCount { text: string; count: number }
export interface MatchSummary {
  total: number
  firstLine: number
  lastLine: number
  levels: LevelCount[]
  signatures: SignatureCount[]
}

export const MAX_SIGNATURES = 5

const LEVEL_ORDER: LogLevel[] = ['error', 'warn', 'info', 'debug', 'trace', 'other']
const LEVEL_WORD = /\b(fatal|error|err|warn(?:ing)?|info|debug|trace)\b/i
const TIMESTAMP = /\b\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?\b/g
const TIME = /\b\d{2}:\d{2}:\d{2}(?:[.,]\d+)?\b/g
const UUID = /\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b/gi

// detectLevel 按行内第一个级别词判断日志级别；fatal 和 err 归入 error。
export function detectLevel(line: string): LogLevel {
  const match = LEVEL_WORD.exec(line)
  if (!match) return 'other'
  const word = match[1].toLowerCase()
  if (word === 'fatal' || word === 'err') return 'error'
  if (word.startsWith('warn')) return 'warn'
  return word as LogLevel
}

// normalizeSignature 去掉时间戳、级别前缀、UUID 与数字，保留可比较的错误文本。
export function normalizeSignature(line: string): string {
  let text = line.replace(TIMESTAMP, ' ').replace(TIME, ' ')
  const match = LEVEL_WORD.exec(text)
  if (match) text = text.slice(match.index + match[0].length)
  // 数字不带尾部边界，让 "3000ms" 之类的耗时也归并为同一签名。
  text = text.replace(UUID, '<id>').replace(/\b\d+(?:\.\d+)*/g, '<num>')
  text = text.replace(/\s+/g, ' ').trim()
  if (!text) text = line.replace(/\s+/g, ' ').trim()
  return text.length > 80 ? text.slice(0, 80).trimEnd() + '…' : text
}

// summarizeMatches 汇总当前结果列表：级别分布、行号范围与错误/警告的高频签名。
export function summarizeMatches(
  matches: Array<{ content: string; lineNumber: number }>,
  signatureLimit = MAX_SIGNATURES,
): MatchSummary {
  const levelCounts = new Map<LogLevel, number>()
  const signatures = new Map<string, number>()
  let firstLine = 0
  let lastLine = 0

  for (const match of matches) {
    const level = detectLevel(match.content)
    levelCounts.set(level, (levelCounts.get(level) ?? 0) + 1)
    if (!firstLine || match.lineNumber < firstLine) firstLine = match.lineNumber
    if (match.lineNumber > lastLine) lastLine = match.lineNumber
    if (level !== 'error' && level !== 'warn') continue
    const signature = normalizeSignature(match.content)
    if (signature) signatures.set(signature, (signatures.get(signature) ?? 0) + 1)
  }

  const levels = LEVEL_ORDER
    .filter(level => (levelCounts.get(level) ?? 0) > 0)
    .map(level => ({ level, count: levelCounts.get(level) ?? 0 }))
  const topSignatures = [...signatures.entries()]
    .map(([text, count]) => ({ text, count }))
    .sort((left, right) => right.count - left.count || left.text.localeCompare(right.text))
    .slice(0, signatureLimit)

  return { total: matches.length, firstLine, lastLine, levels, signatures: topSignatures }
}

export const LEVEL_LABEL: Record<LogLevel, string> = {
  error: 'ERROR', warn: 'WARN', info: 'INFO', debug: 'DEBUG', trace: 'TRACE', other: '其他',
}

// summaryText 生成可直接粘贴到工单或聊天窗口的纯文本摘要。
export function summaryText(summary: MatchSummary): string {
  const lines = ['检索结果摘要', `总计：${summary.total} 条（行号 ${summary.firstLine}–${summary.lastLine}）`]
  if (summary.levels.length) {
    lines.push('级别：' + summary.levels.map(item => `${LEVEL_LABEL[item.level]} ${item.count}`).join(' | '))
  }
  if (summary.signatures.length) {
    lines.push('高频错误：')
    summary.signatures.forEach((item, index) => lines.push(`${index + 1}. ${item.text} — ${item.count} 次`))
  }
  return lines.join('\n')
}
