import { describe, expect, it } from 'vitest'
import {
  MAX_NAMED_SEARCHES,
  normalizeNamedSearchName,
  parseNamedSearches,
  removeNamedSearch,
  upsertNamedSearch,
  type NamedSearch,
} from './namedSearches'

function base(over: Partial<NamedSearch> = {}): NamedSearch {
  return {
    id: 'id-1',
    name: '支付错误',
    createdAt: '2026-01-01T00:00:00.000Z',
    query: 'ERROR pay',
    useRegex: false,
    encoding: 'auto',
    contextLines: 2,
    resultLimit: 500,
    multiLineJSON: false,
    eventContext: true,
    eventBoundary: 'timestamp',
    eventBoundaryPattern: '',
    timeWindowEnabled: false,
    timeWindowPreset: '',
    timeWindowStart: '',
    timeWindowEnd: '',
    timeWindowZone: 'local',
    ...over,
  }
}

describe('parseNamedSearches', () => {
  it('returns empty for missing or malformed payloads', () => {
    expect(parseNamedSearches(null)).toEqual([])
    expect(parseNamedSearches('not json')).toEqual([])
    expect(parseNamedSearches('{"a":1}')).toEqual([])
  })

  it('keeps valid entries, applies defaults and drops incomplete ones', () => {
    const raw = JSON.stringify([
      base({ id: 'a', name: 'A' }),
      { name: '  ', query: 'x' }, // 名称为空 → 丢弃
      { name: 'B', query: '' }, // 关键字为空 → 丢弃
      { name: 'C', query: 'boom', contextLines: 99, resultLimit: -5, timeWindowZone: 'weird' },
    ])
    const parsed = parseNamedSearches(raw)
    expect(parsed.map(item => item.name)).toEqual(['A', 'C'])
    const c = parsed[1]
    expect(c.contextLines).toBe(10) // 夹到上限
    expect(c.resultLimit).toBe(1) // 夹到下限
    expect(c.encoding).toBe('auto') // 缺省回退
    expect(c.timeWindowZone).toBe('local') // 非法值回退
  })

  it('caps the number of stored entries', () => {
    const many = Array.from({ length: MAX_NAMED_SEARCHES + 30 }, (_, i) => base({ id: 'x' + i, name: 'n' + i }))
    expect(parseNamedSearches(JSON.stringify(many))).toHaveLength(MAX_NAMED_SEARCHES)
  })
})

describe('upsertNamedSearch', () => {
  it('prepends a brand new entry', () => {
    const list = upsertNamedSearch([], base({ id: 'a', name: 'A' }))
    expect(list).toHaveLength(1)
    expect(list[0].name).toBe('A')
  })

  it('overwrites a same-name entry, reuses its id and moves it to the front', () => {
    const initial = [base({ id: 'a', name: 'A', query: 'old' }), base({ id: 'b', name: 'B' })]
    const result = upsertNamedSearch(initial, base({ id: 'fresh', name: ' a ', query: 'new' }))
    expect(result).toHaveLength(2)
    expect(result[0].name.trim().toLowerCase()).toBe('a')
    expect(result[0].id).toBe('a') // 沿用原 id
    expect(result[0].query).toBe('new')
    expect(result[1].name).toBe('B')
  })

  it('truncates to the cap after inserting', () => {
    const initial = Array.from({ length: MAX_NAMED_SEARCHES }, (_, i) => base({ id: 'x' + i, name: 'n' + i }))
    const result = upsertNamedSearch(initial, base({ id: 'top', name: 'brand-new' }))
    expect(result).toHaveLength(MAX_NAMED_SEARCHES)
    expect(result[0].name).toBe('brand-new')
  })
})

describe('removeNamedSearch', () => {
  it('drops only the matching id', () => {
    const initial = [base({ id: 'a', name: 'A' }), base({ id: 'b', name: 'B' })]
    expect(removeNamedSearch(initial, 'a').map(item => item.id)).toEqual(['b'])
  })
})

describe('normalizeNamedSearchName', () => {
  it('trims and lowercases for case-insensitive matching', () => {
    expect(normalizeNamedSearchName('  Pay Error ')).toBe('pay error')
  })
})
