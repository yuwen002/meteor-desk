import { ref, type Ref } from 'vue'
import type { LogSearchInput } from './useLogSearch'
import {
  NAMED_SEARCH_STORAGE_KEY,
  normalizeNamedSearchName,
  parseNamedSearches,
  removeNamedSearch,
  upsertNamedSearch,
  type NamedSearch,
} from '../utils/namedSearches'

// 命名检索式：把当前检索表单参数（不含文件路径）存为本机预设，可应用/覆盖/删除。
// 与收藏一致只写 localStorage，写入失败通过 errorMessage 回流到页面提示。
export function useNamedSearches(options: { input: LogSearchInput; errorMessage: Ref<string> }) {
  const namedSearches = ref<NamedSearch[]>(loadStoredNamedSearches())

  function hasName(name: string): boolean {
    const key = normalizeNamedSearchName(name)
    return !!key && namedSearches.value.some(item => normalizeNamedSearchName(item.name) === key)
  }

  // capture 读取当前表单快照生成一条检索式；同名会覆盖并沿用原 id。
  function capture(name: string): NamedSearch | null {
    const trimmed = name.trim()
    if (!trimmed) {
      options.errorMessage.value = '请为检索式命名。'
      return null
    }
    if (!options.input.query.value.trim()) {
      options.errorMessage.value = '当前没有关键字或正则，无法保存检索式。'
      return null
    }
    const i = options.input
    const entry: NamedSearch = {
      id: Date.now().toString(36) + Math.random().toString(36).slice(2, 8),
      name: trimmed,
      createdAt: new Date().toISOString(),
      query: i.query.value,
      useRegex: i.useRegex.value,
      encoding: i.encodingMode.value,
      contextLines: i.contextLines.value,
      resultLimit: i.resultLimit.value,
      multiLineJSON: i.multiLineJSON.value,
      eventContext: i.eventContext.value,
      eventBoundary: i.eventBoundary.value,
      eventBoundaryPattern: i.eventBoundaryPattern.value,
      timeWindowEnabled: i.timeWindowEnabled.value,
      timeWindowPreset: i.timeWindowPreset.value,
      timeWindowStart: i.timeWindowStart.value,
      timeWindowEnd: i.timeWindowEnd.value,
      timeWindowZone: i.timeWindowZone.value,
    }
    namedSearches.value = upsertNamedSearch(namedSearches.value, entry)
    persist()
    // 返回合并后的首位条目：同名覆盖会沿用原 id，调用方拿到的 id 与列表一致。
    return namedSearches.value[0] ?? null
  }

  // apply 只回填检索参数，保留当前选中的文件路径，便于换文件复用同一检索式。
  function apply(entry: NamedSearch) {
    const i = options.input
    i.query.value = entry.query
    i.useRegex.value = entry.useRegex
    i.encodingMode.value = entry.encoding
    i.contextLines.value = entry.contextLines
    i.resultLimit.value = entry.resultLimit
    i.multiLineJSON.value = entry.multiLineJSON
    i.eventContext.value = entry.eventContext
    i.eventBoundary.value = entry.eventBoundary
    i.eventBoundaryPattern.value = entry.eventBoundaryPattern
    i.timeWindowEnabled.value = entry.timeWindowEnabled
    i.timeWindowPreset.value = entry.timeWindowPreset
    i.timeWindowStart.value = entry.timeWindowStart
    i.timeWindowEnd.value = entry.timeWindowEnd
    i.timeWindowZone.value = entry.timeWindowZone
  }

  function remove(id: string) {
    namedSearches.value = removeNamedSearch(namedSearches.value, id)
    persist()
  }

  function persist() {
    try {
      window.localStorage.setItem(NAMED_SEARCH_STORAGE_KEY, JSON.stringify(namedSearches.value))
    } catch {
      options.errorMessage.value = '无法保存检索式，请检查本地存储权限。'
    }
  }

  return { namedSearches, hasName, capture, apply, remove }
}

function loadStoredNamedSearches(): NamedSearch[] {
  try {
    return parseNamedSearches(window.localStorage.getItem(NAMED_SEARCH_STORAGE_KEY))
  } catch {
    return []
  }
}
