import { appendJSONPath } from './jsonPath'
import {
  MAX_TIME_BUCKETS,
  chooseStep,
  extractLogTime,
  formatBucketLabel,
  formatStep,
} from './timeDistribution'

// 结构化字段分布与字段趋势（阶段 6 批次 C）：只消费后端已返回的检索结果，
// 在前端逐行解析 JSON，按选定字段统计取值分布或按时间桶汇总数值，不重新读文件。

// 字段展平上限与详情面板保持一致，避免深嵌套/超大 JSON 拖慢统计。
const MAX_JSON_FIELDS = 300
const MAX_JSON_DEPTH = 8

// facets 最多展示Top N取值，其余合并进“其他”。
export const MAX_FACET_VALUES = 20

export type JSONScalar = string | number | boolean | null

/** 统计所需的最小行结构：LogMatch 天然可赋值给它，测试也可传最简对象。 */
export interface FieldRow {
  content: string
  contentClipped?: boolean
  event?: { content?: string; contentClipped?: boolean }
}

/** 单个取值的命中计数。 */
export interface FacetValue { value: string; count: number }

/** 选定字段的取值分布：Top N + “其他”，并区分含字段/缺字段的行数。 */
export interface FieldFacet {
  path: string
  values: FacetValue[]
  otherCount: number
  otherDistinct: number
  presentCount: number
  missingCount: number
  total: number
}

/** 结果集中出现过的字段：出现行数与其中可解析为数值的行数。 */
export interface FieldInfo { path: string; count: number; numeric: number }

/** 趋势图单个时间桶：该字段数值之和、参与行数与均值。 */
export interface TrendBucket { start: number; label: string; sum: number; count: number; avg: number }

/** 选定数值字段的按时间桶汇总趋势。 */
export interface FieldTrend {
  path: string
  buckets: TrendBucket[]
  stepMs: number
  stepLabel: string
  maxMagnitude: number
  sumTotal: number
  numericCount: number
  undated: number
}

// coerceNumber 只接受 JSON number 与纯数字字符串，其余（布尔/文本/NaN）视为无数值。
export function coerceNumber(value: unknown): number | undefined {
  if (typeof value === 'number') return Number.isFinite(value) ? value : undefined
  if (typeof value === 'string') {
    const text = value.trim()
    if (text && /^-?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?$/.test(text)) {
      const num = Number(text)
      return Number.isFinite(num) ? num : undefined
    }
  }
  return undefined
}

// toFacetKey 把标量取值规范化为分组/比较用的字符串键。
function toFacetKey(value: JSONScalar): string {
  return value === null ? 'null' : String(value)
}

// flattenScalars 把对象/数组展平成“字段路径 → 叶子标量”，与详情面板同源但只保留标量叶子。
function flattenScalars(root: unknown): Record<string, JSONScalar> {
  const out: Record<string, JSONScalar> = {}
  let count = 0
  const add = (value: unknown, path: string, depth: number): void => {
    if (count >= MAX_JSON_FIELDS || depth > MAX_JSON_DEPTH) return
    if (Array.isArray(value)) {
      value.forEach((item, index) => add(item, path + '[' + index + ']', depth + 1))
    } else if (value && typeof value === 'object') {
      for (const [key, item] of Object.entries(value as Record<string, unknown>)) add(item, appendJSONPath(path, key), depth + 1)
    } else {
      out[path] = value as JSONScalar
      count++
    }
  }
  if (root && typeof root === 'object') add(root, '$', 0)
  return out
}

// rowFields 解析单条命中为字段→标量映射；被截断或非对象内容返回 null，不参与统计。
// 内容取用优先级与详情面板一致：优先完整事件/文档，其次行预览。
export function rowFields(match: FieldRow): Record<string, JSONScalar> | null {
  const event = match.event
  const text = event?.content || match.content
  const clipped = event?.content ? event.contentClipped : (match.contentClipped ?? false)
  if (clipped || !text) return null
  try {
    const parsed: unknown = JSON.parse(text)
    if (parsed && typeof parsed === 'object') return flattenScalars(parsed)
  } catch {
    return null
  }
  return null
}

// discoverFields 扫描结果集，列出出现过的字段及其行数、数值行数，按行数降序（同数按路径升序）。
export function discoverFields(matches: FieldRow[]): FieldInfo[] {
  const stats = new Map<string, { count: number; numeric: number }>()
  for (const match of matches) {
    const fields = rowFields(match)
    if (!fields) continue
    for (const [path, value] of Object.entries(fields)) {
      const entry = stats.get(path) ?? { count: 0, numeric: 0 }
      entry.count++
      if (coerceNumber(value) !== undefined) entry.numeric++
      stats.set(path, entry)
    }
  }
  return [...stats.entries()]
    .map(([path, entry]) => ({ path, count: entry.count, numeric: entry.numeric }))
    .sort((left, right) => right.count - left.count || left.path.localeCompare(right.path))
}

// buildFieldFacet 统计选定字段的取值分布：Top N + “其他”，并统计缺失该字段的行数。
export function buildFieldFacet(matches: FieldRow[], path: string, topN = MAX_FACET_VALUES): FieldFacet {
  const counts = new Map<string, number>()
  let present = 0
  let missing = 0
  for (const match of matches) {
    const fields = rowFields(match)
    if (fields && path in fields) {
      present++
      const key = toFacetKey(fields[path])
      counts.set(key, (counts.get(key) ?? 0) + 1)
    } else {
      missing++
    }
  }
  const sorted = [...counts.entries()]
    .map(([value, count]) => ({ value, count }))
    .sort((left, right) => right.count - left.count || left.value.localeCompare(right.value))
  const values = sorted.slice(0, topN)
  const rest = sorted.slice(topN)
  const otherCount = rest.reduce((sum, item) => sum + item.count, 0)
  return {
    path,
    values,
    otherCount,
    otherDistinct: rest.length,
    presentCount: present,
    missingCount: missing,
    total: matches.length,
  }
}

// filterMatchesByFacet 保留“该字段取值等于 key”的行，用于点击取值形成的 facets 式过滤。
// 泛型保留入参行类型（如 LogMatch），以便调用方拿到过滤后仍可访问行号等字段。
export function filterMatchesByFacet<T extends FieldRow>(matches: T[], path: string, key: string): T[] {
  return matches.filter((match) => {
    const fields = rowFields(match)
    return !!fields && path in fields && toFacetKey(fields[path]) === key
  })
}

// buildFieldTrend 把选定字段的数值按自适应时间桶求和，复用时间分布的步长档位与格数上限。
// 无该字段、值非数值或行首无可识别时间戳的行都不参与分桶（后者计入 undated）。
export function buildFieldTrend(matches: FieldRow[], path: string, maxBuckets = MAX_TIME_BUCKETS): FieldTrend {
  const entries: { time: number; value: number }[] = []
  let undated = 0
  for (const match of matches) {
    const fields = rowFields(match)
    if (!fields || !(path in fields)) continue
    const num = coerceNumber(fields[path])
    if (num === undefined) continue
    const time = extractLogTime(match.content)
    if (time === undefined) {
      undated++
      continue
    }
    entries.push({ time, value: num })
  }
  if (!entries.length) {
    return { path, buckets: [], stepMs: 0, stepLabel: '', maxMagnitude: 0, sumTotal: 0, numericCount: 0, undated }
  }

  let minTime = entries[0].time
  let maxTime = entries[0].time
  for (const entry of entries) {
    if (entry.time < minTime) minTime = entry.time
    if (entry.time > maxTime) maxTime = entry.time
  }
  const step = chooseStep(maxTime - minTime, maxBuckets)
  const firstStart = Math.floor(minTime / step) * step
  const count = Math.min(maxBuckets, Math.floor((maxTime - firstStart) / step) + 1)
  const buckets: TrendBucket[] = Array.from({ length: count }, (_, index) => {
    const start = firstStart + index * step
    return { start, label: formatBucketLabel(start, step), sum: 0, count: 0, avg: 0 }
  })

  let sumTotal = 0
  let maxMagnitude = 0
  for (const entry of entries) {
    const index = Math.min(count - 1, Math.floor((entry.time - firstStart) / step))
    if (index < 0) continue
    const bucket = buckets[index]
    bucket.sum += entry.value
    bucket.count++
    sumTotal += entry.value
    maxMagnitude = Math.max(maxMagnitude, Math.abs(bucket.sum))
  }
  for (const bucket of buckets) bucket.avg = bucket.count ? bucket.sum / bucket.count : 0
  return {
    path,
    buckets,
    stepMs: step,
    stepLabel: formatStep(step),
    maxMagnitude,
    sumTotal,
    numericCount: entries.length,
    undated,
  }
}
