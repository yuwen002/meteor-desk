export interface CronParseResult { error: string; description: string; nextRuns: Date[] }

interface CronField { values: Set<number>; any: boolean }
const ranges = [[0, 59], [0, 23], [1, 31], [1, 12], [0, 6]] as const
const labels = ['分钟', '小时', '日期', '月份', '星期']

function parseField(input: string, min: number, max: number): CronField | null {
  const values = new Set<number>()
  if (!input) return null
  for (const part of input.split(',')) {
    const slashParts = part.split('/')
    if (slashParts.length > 2) return null
    const rangePart = slashParts[0]
    const step = slashParts.length === 1 ? 1 : Number(slashParts[1])
    if (!Number.isInteger(step) || step <= 0) return null
    let start = min, end = max
    if (rangePart !== '*') {
      const bounds = rangePart.split('-').map(Number)
      if (bounds.some(value => !Number.isInteger(value)) || bounds.length > 2) return null
      start = bounds[0]
      if (bounds.length === 2) end = bounds[1]
      else end = slashParts.length === 2 ? max : start
    }
    if (start < min || end > max || start > end) return null
    for (let value = start; value <= end; value += step) values.add(value)
  }
  return { values, any: input === '*' }
}

function fieldDescription(source: string, label: string): string {
  if (source === '*') return `每${label}`
  if (/^\*\/\d+$/.test(source)) return `每 ${source.slice(2)} ${label}`
  return `${label} ${source}`
}

export function parseCron(expression: string, from = new Date()): CronParseResult {
  const parts = expression.trim().split(/\s+/)
  if (parts.length !== 5) return { error: 'Cron 表达式需要 5 个字段：分 时 日 月 周。', description: '', nextRuns: [] }
  const fields = parts.map((part, index) => parseField(part, ranges[index][0], ranges[index][1]))
  if (fields.some(field => field === null)) return { error: '字段包含无效范围、步长或格式。', description: '', nextRuns: [] }
  const parsed = fields as CronField[]
  const nextRuns: Date[] = []
  const cursor = new Date(from.getTime())
  cursor.setSeconds(0, 0); cursor.setMinutes(cursor.getMinutes() + 1)
  const limit = 5 * 366 * 24 * 60
  for (let index = 0; index < limit && nextRuns.length < 8; index++) {
    const dayOfMonth = parsed[2].values.has(cursor.getDate())
    const dayOfWeek = parsed[4].values.has(cursor.getDay())
    const dayMatches = parsed[2].any || parsed[4].any ? dayOfMonth && dayOfWeek : dayOfMonth || dayOfWeek
    if (parsed[0].values.has(cursor.getMinutes()) && parsed[1].values.has(cursor.getHours()) && dayMatches && parsed[3].values.has(cursor.getMonth() + 1)) nextRuns.push(new Date(cursor))
    cursor.setMinutes(cursor.getMinutes() + 1)
  }
  return { error: nextRuns.length ? '' : '未来五年内没有符合条件的执行时间。', description: parts.map((part, index) => fieldDescription(part, labels[index])).join('，'), nextRuns }
}
