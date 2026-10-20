import { describe, expect, it } from 'vitest'
import { summarizeLogLines } from './logStats'

describe('summarizeLogLines', () => {
  it('counts recognized and unknown levels without guessing from JSON message text', () => {
    const stats = summarizeLogLines([
      { content: 'INFO startup complete' },
      { content: '[WARN] slow request' },
      { content: '{"level":"error","message":"request failed"}' },
      { content: '{"message":"ERROR mentioned as plain text"}' },
      { content: 'plain line' },
    ])
    expect([stats.total, stats.info, stats.warn, stats.error, stats.unknown]).toEqual([5, 1, 1, 1, 2])
  })

  it('groups repeated errors after normalizing timestamps and long numeric IDs', () => {
    const stats = summarizeLogLines([
      { content: '2026-10-20 10:00:01 ERROR request 123456 failed' },
      { content: '2026-10-20 10:00:02 ERROR request 987654 failed' },
    ])
    expect(stats.repeatedErrors).toHaveLength(1)
    expect(stats.repeatedErrors[0].count).toBe(2)
    expect(stats.repeatedErrors[0].message).toContain('<N>')
  })

  it('builds minute buckets from text and JSON timestamps in chronological order', () => {
    const stats = summarizeLogLines([
      { content: '{"timestamp":"2026-10-20T10:02:15Z","level":"ERROR","message":"bad"}' },
      { content: '2026-10-20 10:01:20 INFO ready' },
      { content: '{"ts":1792490400,"level":"WARN"}' },
    ])
    expect(stats.trend.length).toBeGreaterThanOrEqual(2)
    expect(stats.trend[0].label).toBe('2026-10-20 10:00')
    expect(stats.trend.some(bucket => bucket.error === 1)).toBe(true)
  })

  it('limits the trend to the newest 60 minutes and repeated errors to ten groups', () => {
    const lines = Array.from({ length: 80 }, (_, i) => ({
      content: `2026-10-20T10:${String(i).padStart(2, '0')}:00Z ERROR unique failure ${i + 10000}`,
    }))
    const stats = summarizeLogLines(lines)
    expect(stats.trend.length).toBeLessThanOrEqual(60)
    expect(stats.repeatedErrors.length).toBeLessThanOrEqual(10)
  })
})
