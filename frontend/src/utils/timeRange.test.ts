import { describe, expect, it } from 'vitest'
import { extractLogTimestamp, filterLogLinesByTimeRange, isWithinTimeRange } from './timeRange'

describe('extractLogTimestamp', () => {
  it('reads ISO timestamps with UTC offsets and fractional seconds', () => {
    expect(extractLogTimestamp('2026-10-16T10:20:30.250+08:00 INFO ready'))
      .toBe(Date.parse('2026-10-16T10:20:30.250+08:00'))
  })

  it('reads common local timestamps and Apache timestamps', () => {
    expect(extractLogTimestamp('2026-10-16 10:20:30 WARN retry')).not.toBeNull()
    expect(extractLogTimestamp('[16/Oct/2026:10:20:30 +0800] GET /health')).toBe(Date.parse('16 Oct 2026 10:20:30 GMT+0800'))
  })

  it('reads timestamp fields in JSON and ignores unrelated dates in message text', () => {
    expect(extractLogTimestamp('{"@timestamp":"2026-10-16T10:20:30Z","level":"INFO"}'))
      .toBe(Date.parse('2026-10-16T10:20:30Z'))
    expect(extractLogTimestamp('user mentioned 2026-10-16T10:20:30Z')).toBeNull()
  })
})

describe('isWithinTimeRange', () => {
  it('uses inclusive range boundaries and keeps all lines when unset', () => {
    const line = '2026-10-16T10:20:30Z INFO ready'
    expect(isWithinTimeRange(line, { from: '2026-10-16T10:20:30Z', to: '2026-10-16T10:20:30Z' })).toBe(true)
    expect(isWithinTimeRange('undated detail', {})).toBe(true)
  })

  it('includes the full selected end minute for datetime-local inputs', () => {
    const end = new Date(2026, 9, 16, 12, 30, 59, 999)
    const stamp = [end.getFullYear(), String(end.getMonth() + 1).padStart(2, '0'), String(end.getDate()).padStart(2, '0')].join('-')
      + 'T' + [String(end.getHours()).padStart(2, '0'), String(end.getMinutes()).padStart(2, '0'), String(end.getSeconds()).padStart(2, '0')].join(':')
    expect(isWithinTimeRange(stamp + ' INFO last second', { to: stamp.slice(0, 16) })).toBe(true)
  })

  it('excludes undated lines and rejects invalid or reversed ranges', () => {
    expect(isWithinTimeRange('undated detail', { from: '2026-10-16T00:00' })).toBe(false)
    expect(isWithinTimeRange('2026-10-16T10:20:30Z INFO ready', { from: 'bad-date' })).toBe(false)
    expect(isWithinTimeRange('2026-10-16T10:20:30Z INFO ready', { from: '2026-10-17T00:00Z', to: '2026-10-16T00:00Z' })).toBe(false)
  })
})

describe('filterLogLinesByTimeRange', () => {
  it('filters result rows without changing their order', () => {
    const rows = [
      { content: '2026-10-16T10:00:00Z INFO before', id: 1 },
      { content: '2026-10-16T11:00:00Z INFO inside', id: 2 },
      { content: '2026-10-16T12:00:00Z INFO after', id: 3 },
    ]
    expect(filterLogLinesByTimeRange(rows, { from: '2026-10-16T10:30:00Z', to: '2026-10-16T11:30:00Z' })).toEqual([rows[1]])
  })
})
