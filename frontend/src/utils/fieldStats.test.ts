import { describe, expect, it } from 'vitest'
import {
  buildFieldFacet,
  buildFieldTrend,
  coerceNumber,
  discoverFields,
  filterMatchesByFacet,
  MAX_FACET_VALUES,
  rowFields,
} from './fieldStats'

// jsonRow 用纯 JSON 行作为 NDJSON 命中；字段路径以叶子键为准（如 level、meta.user）。
const jsonRow = (value: unknown) => ({ content: JSON.stringify(value) })

describe('coerceNumber', () => {
  it('accepts finite numbers and numeric strings only', () => {
    expect(coerceNumber(42)).toBe(42)
    expect(coerceNumber(-3.5)).toBe(-3.5)
    expect(coerceNumber('1e3')).toBe(1000)
    expect(coerceNumber(' 12 ')).toBe(12)
    expect(coerceNumber('ERROR')).toBeUndefined()
    expect(coerceNumber('12abc')).toBeUndefined()
    expect(coerceNumber(true)).toBeUndefined()
    expect(coerceNumber(null)).toBeUndefined()
    expect(coerceNumber(NaN)).toBeUndefined()
  })
})

describe('rowFields', () => {
  it('flattens leaf scalars with dotted and indexed paths', () => {
    const fields = rowFields(jsonRow({ level: 'ERROR', meta: { user: 'ann', codes: [7, 8] } }))
    expect(fields).toMatchObject({ level: 'ERROR', 'meta.user': 'ann', 'meta.codes[0]': 7, 'meta.codes[1]': 8 })
  })

  it('prefers the full event content over the row preview', () => {
    const fields = rowFields({ content: '{"a":1,"b"', contentClipped: false, event: { content: '{"a":1,"b":2}', contentClipped: false } })
    expect(fields).toMatchObject({ a: 1, b: 2 })
  })

  it('returns null for clipped or non-JSON content', () => {
    expect(rowFields({ content: '{"a":1', contentClipped: true })).toBeNull()
    expect(rowFields({ content: 'plain text ERROR' })).toBeNull()
    expect(rowFields({ content: '[1,2,3]' })).toMatchObject({ '$[0]': 1, '$[1]': 2, '$[2]': 3 })
  })
})

describe('discoverFields', () => {
  it('counts occurrences and numeric rows, ordered by count then path', () => {
    const fields = discoverFields([
      jsonRow({ level: 'ERROR', latency: 10 }),
      jsonRow({ level: 'WARN', latency: 'slow' }),
      jsonRow({ level: 'INFO' }),
    ])
    const level = fields.find(field => field.path === 'level')
    const latency = fields.find(field => field.path === 'latency')
    expect(level).toEqual({ path: 'level', count: 3, numeric: 0 })
    expect(latency).toEqual({ path: 'latency', count: 2, numeric: 1 })
    // level 出现在 3 行，应排在 latency（2 行）之前。
    expect(fields.indexOf(level!)).toBeLessThan(fields.indexOf(latency!))
  })

  it('skips rows that are not JSON', () => {
    expect(discoverFields([{ content: 'not json' }])).toEqual([])
  })
})

describe('buildFieldFacet', () => {
  it('groups values, keeps missing rows out of the distribution', () => {
    const facet = buildFieldFacet([
      jsonRow({ level: 'ERROR' }),
      jsonRow({ level: 'ERROR' }),
      jsonRow({ level: 'WARN' }),
      jsonRow({ msg: 'no level field' }),
      { content: 'plain text' },
    ], 'level')
    expect(facet.total).toBe(5)
    expect(facet.presentCount).toBe(3)
    expect(facet.missingCount).toBe(2)
    expect(facet.values).toEqual([{ value: 'ERROR', count: 2 }, { value: 'WARN', count: 1 }])
    expect(facet.otherCount).toBe(0)
  })

  it('splits the tail into “其他” beyond the top N', () => {
    const rows = []
    for (let i = 0; i < 25; i++) rows.push(jsonRow({ code: 'v' + i }))
    const facet = buildFieldFacet(rows, 'code')
    expect(facet.values).toHaveLength(MAX_FACET_VALUES)
    expect(facet.otherDistinct).toBe(25 - MAX_FACET_VALUES)
    expect(facet.otherCount).toBe(25 - MAX_FACET_VALUES)
    // 全部计数都为 1，按取值升序稳定排列。
    expect(facet.values[0].value).toBe('v0')
  })
})

describe('filterMatchesByFacet', () => {
  it('keeps only rows whose field value equals the key', () => {
    const rows = [jsonRow({ level: 'ERROR' }), jsonRow({ level: 'WARN' }), jsonRow({ msg: 'x' })]
    const kept = filterMatchesByFacet(rows, 'level', 'ERROR')
    expect(kept).toHaveLength(1)
    expect(kept[0]).toBe(rows[0])
  })
})

describe('buildFieldTrend', () => {
  it('sums numeric values into adaptive time buckets', () => {
    const trend = buildFieldTrend([
      jsonRow({ level: 'ERROR', ts: '2026-10-30 10:00:00', latency: 100 }),
      jsonRow({ level: 'ERROR', ts: '2026-10-30 10:00:02', latency: 50 }),
      jsonRow({ level: 'ERROR', ts: '2026-10-30 10:00:05', latency: 30 }),
    ], 'latency')
    expect(trend.stepMs).toBe(1000)
    expect(trend.buckets).toHaveLength(6)
    expect(trend.sumTotal).toBe(180)
    expect(trend.numericCount).toBe(3)
    expect(trend.maxMagnitude).toBe(100)
    expect(trend.buckets[0].sum).toBe(100)
    expect(trend.buckets[0].avg).toBe(100)
    expect(trend.buckets[2].sum).toBe(50)
    expect(trend.buckets[5].sum).toBe(30)
  })

  it('counts numeric rows without a recognizable timestamp as undated', () => {
    const trend = buildFieldTrend([
      jsonRow({ ts: '2026-10-30 10:00:00', latency: 10 }),
      jsonRow({ latency: 999 }),
    ], 'latency')
    expect(trend.numericCount).toBe(1)
    expect(trend.undated).toBe(1)
    expect(trend.sumTotal).toBe(10)
  })

  it('returns an empty trend when the field is missing or non-numeric', () => {
    const rows = [jsonRow({ level: 'ERROR', ts: '2026-10-30 10:00:00' })]
    expect(buildFieldTrend(rows, 'level').buckets).toEqual([])
    expect(buildFieldTrend(rows, 'absent').numericCount).toBe(0)
  })
})
