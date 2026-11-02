// 命名检索式：把一组检索参数（不含文件路径）保存为本机预设，可一键应用、覆盖与删除。
// 只写入 localStorage，上限 100 条；纯函数便于单测，视图与持久化在 composable 里组合。
export type NamedSearchZone = 'local' | 'utc'

/** 可保存的一组检索参数快照，字段与检索表单一一对应。 */
export interface NamedSearchParams {
  query: string
  useRegex: boolean
  encoding: string
  contextLines: number
  resultLimit: number
  multiLineJSON: boolean
  eventContext: boolean
  eventBoundary: string
  eventBoundaryPattern: string
  timeWindowEnabled: boolean
  timeWindowPreset: string
  timeWindowStart: string
  timeWindowEnd: string
  timeWindowZone: NamedSearchZone
}

export interface NamedSearch extends NamedSearchParams {
  id: string
  name: string
  createdAt: string
}

export const MAX_NAMED_SEARCHES = 100
export const NAMED_SEARCH_STORAGE_KEY = 'meteordesk.namedSearches.v1'

// normalizeNamedSearchName 用于「同名即覆盖」的比较：去首尾空白并大小写不敏感。
export function normalizeNamedSearchName(name: string): string {
  return name.trim().toLowerCase()
}

function clampInt(value: unknown, min: number, max: number, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback
  const rounded = Math.floor(value)
  if (rounded < min) return min
  if (rounded > max) return max
  return rounded
}

function toBool(value: unknown, fallback: boolean): boolean {
  return typeof value === 'boolean' ? value : fallback
}

function toStr(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

// parseNamedSearches 容错解析本地存储：非法条目跳过，字段缺失回退默认值，条数截断到上限。
export function parseNamedSearches(raw: string | null): NamedSearch[] {
  if (!raw) return []
  try {
    const data: unknown = JSON.parse(raw)
    if (!Array.isArray(data)) return []
    const entries: NamedSearch[] = []
    for (const item of data) {
      if (!item || typeof item !== 'object') continue
      const value = item as Record<string, unknown>
      const name = toStr(value.name).trim()
      const query = toStr(value.query)
      // 名称为空的条目无法定位「覆盖」，直接丢弃；检索式没有关键字也无意义。
      if (!name || !query) continue
      entries.push({
        id: toStr(value.id) || name,
        name,
        createdAt: toStr(value.createdAt),
        query,
        useRegex: toBool(value.useRegex, false),
        encoding: toStr(value.encoding) || 'auto',
        contextLines: clampInt(value.contextLines, 0, 10, 0),
        resultLimit: clampInt(value.resultLimit, 1, 5000, 500),
        multiLineJSON: toBool(value.multiLineJSON, false),
        eventContext: toBool(value.eventContext, false),
        eventBoundary: toStr(value.eventBoundary) || 'timestamp',
        eventBoundaryPattern: toStr(value.eventBoundaryPattern),
        timeWindowEnabled: toBool(value.timeWindowEnabled, false),
        timeWindowPreset: toStr(value.timeWindowPreset),
        timeWindowStart: toStr(value.timeWindowStart),
        timeWindowEnd: toStr(value.timeWindowEnd),
        timeWindowZone: value.timeWindowZone === 'utc' ? 'utc' : 'local',
      })
      if (entries.length >= MAX_NAMED_SEARCHES) break
    }
    return entries
  } catch {
    return []
  }
}

// upsertNamedSearch 按名称去重：同名覆盖并沿用原 id、提到列表最前；新名称插入最前，整体截断到上限。
export function upsertNamedSearch(list: NamedSearch[], entry: NamedSearch, limit = MAX_NAMED_SEARCHES): NamedSearch[] {
  const key = normalizeNamedSearchName(entry.name)
  const existing = list.find(item => normalizeNamedSearchName(item.name) === key)
  const merged: NamedSearch = existing ? { ...entry, id: existing.id } : entry
  const rest = list.filter(item => normalizeNamedSearchName(item.name) !== key)
  return [merged, ...rest].slice(0, limit)
}

export function removeNamedSearch(list: NamedSearch[], id: string): NamedSearch[] {
  return list.filter(item => item.id !== id)
}
