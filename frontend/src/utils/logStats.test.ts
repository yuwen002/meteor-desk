import { describe, expect, it } from 'vitest'
import { classifyLogSeverity, summarizeLogMatches } from './logStats'

describe('classifyLogSeverity', () => {
  it('recognizes common levels after timestamps and logger labels', () => {
    expect(classifyLogSeverity('2026-10-26 10:00:00 ERROR connection refused')).toBe('error')
    expect(classifyLogSeverity('2026-10-26T10:00:00Z WARNING slow query')).toBe('warn')
    expect(classifyLogSeverity('[service] INFO ready')).toBe('info')
    expect(classifyLogSeverity('2026/10/26 10:00:00,123 [worker-1] WARN retry')).toBe('warn')
    expect(classifyLogSeverity('12:00:01.456 DEBUG cache miss')).toBe('debug')
    expect(classifyLogSeverity('random text')).toBe('other')
  })

  it('does not classify words that only occur inside message text or identifiers', () => {
    expect(classifyLogSeverity('requestId=INFO-123 ordinary message')).toBe('other')
    expect(classifyLogSeverity('received INFO response from upstream')).toBe('other')
    expect(classifyLogSeverity('payload={"level":"ERROR","message":"failed"}')).toBe('other')
    expect(classifyLogSeverity('Caused by: ERROR is part of exception text')).toBe('other')
  })
})

describe('summarizeLogMatches', () => {
  it('counts levels, trends and duplicate errors within the supplied result set', () => {
    const stats = summarizeLogMatches([
      { lineNumber: 1, content: '2026-10-26 10:00:00 ERROR request failed requestId=123456' },
      { lineNumber: 2, content: '2026-10-26 10:00:30 ERROR request failed requestId=987654' },
      { lineNumber: 3, content: '2026-10-26 10:01:00 WARN retrying' },
      { lineNumber: 4, content: '2026-10-26 10:02:00 INFO ready' },
    ])
    expect(stats.total).toBe(4)
    expect(stats.errors).toBe(2)
    expect(stats.warnings).toBe(1)
    expect(stats.info).toBe(1)
    expect(stats.duplicates).toHaveLength(1)
    expect(stats.duplicates[0].count).toBe(2)
    expect(stats.trend).toEqual([
      { bucket: '2026-10-26 10:00', errors: 2, warnings: 0 },
      { bucket: '2026-10-26 10:01', errors: 0, warnings: 1 },
    ])
  })

  it('keeps same clock minutes on different dates in separate trend buckets', () => {
    const stats = summarizeLogMatches([
      { lineNumber: 1, content: '2026-10-25 10:00:00 ERROR yesterday' },
      { lineNumber: 2, content: '2026-10-26 10:00:00 ERROR today' },
    ])
    expect(stats.trend).toEqual([
      { bucket: '2026-10-25 10:00', errors: 1, warnings: 0 },
      { bucket: '2026-10-26 10:00', errors: 1, warnings: 0 },
    ])
  })

  it('uses aggregated event content and keeps stack traces out of duplicate signatures', () => {
    const stats = summarizeLogMatches([
      { lineNumber: 10, content: 'ERROR preview', event: { startLine: 10, endLine: 12, content: '2026-10-26 10:00:00 ERROR failed requestId=abc123\n at service.A.call(A.go:10)\nCaused by: closed', contentClipped: false } },
      { lineNumber: 20, content: 'ERROR preview', event: { startLine: 20, endLine: 22, content: '2026-10-26 10:01:00 ERROR failed requestId=xyz789\n at service.B.call(B.go:20)\nCaused by: closed', contentClipped: false } },
    ])
    expect(stats.duplicates[0].count).toBe(2)
    expect(stats.duplicates[0].lines).toEqual([10, 20])
  })

  it('does not count unmatched text as a severity', () => {
    const stats = summarizeLogMatches([{ lineNumber: 1, content: 'requestId=INFO-123 ordinary message' }])
    expect(stats.other).toBe(1)
    expect(stats.errors).toBe(0)
  })

  it('handles empty results and missing timestamps without inventing trend buckets', () => {
    expect(summarizeLogMatches([])).toMatchObject({ total: 0, errors: 0, warnings: 0, other: 0, trend: [], duplicates: [] })
    const stats = summarizeLogMatches([
      { lineNumber: 1, content: 'ERROR failed without timestamp' },
      { lineNumber: 2, content: 'WARN retry without timestamp' },
    ])
    expect(stats.trend).toEqual([])
  })
})
