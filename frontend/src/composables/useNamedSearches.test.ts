import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { NAMED_SEARCH_STORAGE_KEY, type NamedSearch } from '../utils/namedSearches'
import { useNamedSearches } from './useNamedSearches'
import type { LogSearchInput } from './useLogSearch'

function createInput(): LogSearchInput {
  return {
    filePath: ref('C:/logs/app.log'),
    query: ref('ERROR pay'),
    useRegex: ref(false),
    contextLines: ref(2),
    resultLimit: ref(500),
    encodingMode: ref('utf-8'),
    multiLineJSON: ref(false),
    eventContext: ref(true),
    eventBoundary: ref('timestamp'),
    eventBoundaryPattern: ref(''),
    timeWindowEnabled: ref(true),
    timeWindowPreset: ref(''),
    timeWindowStart: ref('2026-01-02T10:00:00'),
    timeWindowEnd: ref('2026-01-02T11:00:00'),
    timeWindowZone: ref('utc'),
    multiFile: ref(false),
    rootDir: ref(''),
    rootPattern: ref('*.log;*.txt;*.out;*.json;*.ndjson'),
  }
}

function create(stored?: NamedSearch[]) {
  if (stored) window.localStorage.setItem(NAMED_SEARCH_STORAGE_KEY, JSON.stringify(stored))
  const input = createInput()
  const errorMessage = ref('')
  const api = useNamedSearches({ input, errorMessage })
  return { input, errorMessage, ...api }
}

function stored(): NamedSearch[] {
  return JSON.parse(window.localStorage.getItem(NAMED_SEARCH_STORAGE_KEY) ?? '[]')
}

describe('useNamedSearches', () => {
  afterEach(() => window.localStorage.clear())

  it('refuses to capture without a name or a query', () => {
    const { errorMessage, capture } = create()
    expect(capture('  ')).toBeNull()
    expect(errorMessage.value).toBe('请为检索式命名。')

    const second = create()
    second.input.query.value = '   '
    expect(second.capture('空查询')).toBeNull()
    expect(second.errorMessage.value).toContain('无法保存检索式')
  })

  it('captures the current form and persists it, then reports the name as taken', () => {
    const { capture, hasName, namedSearches } = create()
    const entry = capture('支付排查')
    expect(entry).not.toBeNull()
    expect(namedSearches.value).toHaveLength(1)
    expect(hasName('支付排查')).toBe(true)
    expect(hasName(' 支付排查 ')).toBe(true) // 大小写/空白归一
    expect(hasName('不存在')).toBe(false)
    expect(stored()[0]).toMatchObject({ name: '支付排查', query: 'ERROR pay', encoding: 'utf-8', timeWindowEnabled: true, timeWindowZone: 'utc' })
  })

  it('overwrites a same-name capture while keeping the id', () => {
    const existing: NamedSearch = { id: 'keep', name: 'same', createdAt: '', query: 'old', useRegex: false, encoding: 'auto', contextLines: 0, resultLimit: 500, multiLineJSON: false, eventContext: false, eventBoundary: 'timestamp', eventBoundaryPattern: '', timeWindowEnabled: false, timeWindowPreset: '', timeWindowStart: '', timeWindowEnd: '', timeWindowZone: 'local' }
    const { capture, namedSearches } = create([existing])
    const entry = capture('SAME')
    expect(namedSearches.value).toHaveLength(1)
    expect(entry?.id).toBe('keep')
    expect(namedSearches.value[0].query).toBe('ERROR pay')
  })

  it('applies an entry to the form refs without touching the file path', () => {
    const entry: NamedSearch = { id: 'x', name: 'preset', createdAt: '', query: 'WARN slow', useRegex: true, encoding: 'gb18030', contextLines: 5, resultLimit: 1000, multiLineJSON: true, eventContext: false, eventBoundary: 'blank', eventBoundaryPattern: '', timeWindowEnabled: true, timeWindowPreset: '1h', timeWindowStart: '', timeWindowEnd: '', timeWindowZone: 'utc' }
    const { input, apply } = create()
    apply(entry)
    expect(input.filePath.value).toBe('C:/logs/app.log') // 文件路径保持不变
    expect(input.query.value).toBe('WARN slow')
    expect(input.useRegex.value).toBe(true)
    expect(input.encodingMode.value).toBe('gb18030')
    expect(input.contextLines.value).toBe(5)
    expect(input.resultLimit.value).toBe(1000)
    expect(input.multiLineJSON.value).toBe(true)
    expect(input.eventBoundary.value).toBe('blank')
    expect(input.timeWindowPreset.value).toBe('1h')
    expect(input.timeWindowZone.value).toBe('utc')
  })

  it('removes by id and persists the change', () => {
    const { capture, remove, namedSearches } = create()
    const entry = capture('临时')
    expect(namedSearches.value).toHaveLength(1)
    remove(entry!.id)
    expect(namedSearches.value).toHaveLength(0)
    expect(stored()).toEqual([])
  })

  it('survives a storage write failure while keeping the in-memory list', () => {
    const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('quota') })
    const { capture, errorMessage, namedSearches } = create()
    capture('写盘失败')
    expect(errorMessage.value).toBe('无法保存检索式，请检查本地存储权限。')
    expect(namedSearches.value).toHaveLength(1)
    setItem.mockRestore()
  })
})
