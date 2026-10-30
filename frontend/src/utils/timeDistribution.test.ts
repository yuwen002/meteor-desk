import { describe, expect, it } from 'vitest'
import { buildTimeDistribution, extractLogTime, filterMatchesByTime, MAX_TIME_BUCKETS } from './timeDistribution'

const match = (content:string) => ({ content })

describe('extractLogTime', () => {
  it('parses full timestamps, ISO timestamps and clock-only lines', () => {
    expect(extractLogTime('2026-10-30 10:00:00 ERROR boom')).toBe(Date.UTC(2026, 9, 30, 10, 0, 0))
    expect(extractLogTime('2026-10-30T10:00:00.250Z WARN slow')).toBe(Date.UTC(2026, 9, 30, 10, 0, 0, 250))
    expect(extractLogTime('10:00:02 ERROR boom')).toBe(Date.UTC(1970, 0, 1, 10, 0, 2))
  })

  it('returns undefined without a valid timestamp', () => {
    expect(extractLogTime('ERROR boom')).toBeUndefined()
    expect(extractLogTime('2026-10-30 25:00:00 ERROR boom')).toBeUndefined()
    expect(extractLogTime('request id 12345 done')).toBeUndefined()
  })
})

describe('buildTimeDistribution', () => {
  it('buckets error and warn lines by second', () => {
    const distribution = buildTimeDistribution([
      match('2026-10-30 10:00:00 ERROR connection refused'),
      match('2026-10-30 10:00:01 WARN slow request'),
      match('2026-10-30 10:00:02 ERROR connection refused'),
      match('2026-10-30 10:00:03 INFO ready'),
    ])
    expect(distribution.stepMs).toBe(1000)
    expect(distribution.stepLabel).toBe('1 秒')
    // 10:00:03 的 INFO 行不参与分桶，最后一桶截止到 10:00:02。
    expect(distribution.buckets).toHaveLength(3)
    expect(distribution.buckets.map(bucket => bucket.error)).toEqual([1, 0, 1])
    expect(distribution.buckets.map(bucket => bucket.warn)).toEqual([0, 1, 0])
    expect(distribution.errorTotal).toBe(2)
    expect(distribution.warnTotal).toBe(1)
    expect(distribution.maxCount).toBe(1)
    // 秒级分桶保留秒，完整时间戳额外带上日期。
    expect(distribution.buckets[0].label).toBe('10-30 10:00:00')
  })

  it('counts error lines without timestamps separately', () => {
    const distribution = buildTimeDistribution([
      match('ERROR no timestamp'),
      match('2026-10-30 10:00:00 ERROR with timestamp'),
    ])
    expect(distribution.undated).toBe(1)
    expect(distribution.errorTotal).toBe(1)
  })

  it('ignores non error/warn lines and reports an empty distribution', () => {
    const distribution = buildTimeDistribution([match('2026-10-30 10:00:00 INFO ready'), match('2026-10-30 10:00:01 DEBUG tick')])
    expect(distribution.buckets).toEqual([])
    expect(distribution.errorTotal).toBe(0)
    expect(distribution.warnTotal).toBe(0)
    expect(distribution.undated).toBe(0)
  })

  it('widens the bucket for long time spans', () => {
    const distribution = buildTimeDistribution([
      match('2026-10-30 10:00:00 ERROR first'),
      match('2026-10-30 12:00:00 ERROR last'),
    ])
    expect(distribution.stepMs).toBe(600000)
    expect(distribution.stepLabel).toBe('10 分钟')
    expect(distribution.buckets).toHaveLength(13)
    expect(distribution.buckets[0].label).toBe('10-30 10:00')
    expect(distribution.errorTotal).toBe(2)
  })

  it('caps the bucket count', () => {
    const distribution = buildTimeDistribution([
      match('2026-09-01 00:00:00 ERROR first'),
      match('2026-10-01 00:00:00 ERROR last'),
    ])
    expect(distribution.buckets).toHaveLength(MAX_TIME_BUCKETS)
    expect(distribution.errorTotal).toBe(2)
  })
})

describe('filterMatchesByTime', () => {
  it('keeps only lines inside the bucket window', () => {
    const matches = [
      match('2026-10-30 10:00:00 ERROR first'),
      match('2026-10-30 10:00:01 WARN second'),
      match('2026-10-30 10:00:02 INFO third'),
      match('ERROR no timestamp'),
    ]
    const start = Date.UTC(2026, 9, 30, 10, 0, 1)
    // 上边界开放：10:00:02 属于下一格，没有时间戳的行不参与过滤。
    expect(filterMatchesByTime(matches, start, 1000).map(item => item.content)).toEqual(['2026-10-30 10:00:01 WARN second'])
  })

  it('matches clock-only lines against a clock-only anchor', () => {
    const matches = [match('10:00:02 ERROR boom'), match('10:00:05 error other')]
    const start = Date.UTC(1970, 0, 1, 10, 0, 0)
    expect(filterMatchesByTime(matches, start, 5000)).toHaveLength(1)
  })
})
