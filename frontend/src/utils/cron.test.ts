import { describe, expect, it } from 'vitest'
import { parseCron } from './cron'

describe('parseCron', () => {
  it('parses steps and finds future execution times', () => {
    const result = parseCron('*/15 9-17 * * 1-5', new Date(2026, 9, 25, 8, 58))
    expect(result.error).toBe('')
    expect(result.nextRuns[0]?.getHours()).toBe(9)
    expect(result.nextRuns[0]?.getMinutes()).toBe(0)
    expect(result.nextRuns[0]?.getDay()).toBe(1)
  })
  it('rejects invalid field counts and ranges', () => {
    expect(parseCron('* * *').error).toContain('5 个字段')
    expect(parseCron('60 * * * *').error).toContain('无效')
  })
  it('uses the usual day-of-month or day-of-week rule when both are specified', () => {
    const result = parseCron('0 9 1 * 1', new Date(2026, 9, 25, 8, 58))
    expect(result.nextRuns[0]?.getDay()).toBe(1)
    expect(result.nextRuns[0]?.getDate()).toBe(26)
  })
})
