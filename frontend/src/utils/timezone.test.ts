import { describe, expect, it } from 'vitest'
import { getDateTimeInput, partsInTimeZone, zonedDateTimeToEpoch } from './timezone'

// 2026-01-01T00:00:00Z
const NEW_YEAR_UTC = Date.UTC(2026, 0, 1, 0, 0, 0)

describe('partsInTimeZone', () => {
  it('按指定时区取名下时间，不受本机时区影响', () => {
    const shanghai = partsInTimeZone(NEW_YEAR_UTC, 'Asia/Shanghai')
    expect(shanghai).toEqual({ year: 2026, month: 1, day: 1, hour: 8, minute: 0, second: 0 })
    expect(partsInTimeZone(NEW_YEAR_UTC, 'UTC').hour).toBe(0)
  })

  it('使用 24 小时制，午夜显示为 0 点', () => {
    expect(partsInTimeZone(NEW_YEAR_UTC, 'UTC').hour).toBe(0)
  })
})

describe('getDateTimeInput', () => {
  it('补零成 datetime-local 需要的格式', () => {
    expect(getDateTimeInput(NEW_YEAR_UTC, 'Asia/Shanghai')).toBe('2026-01-01T08:00')
    expect(getDateTimeInput(Date.UTC(2026, 8, 9, 23, 5), 'UTC')).toBe('2026-09-09T23:05')
  })

  it('与 zonedDateTimeToEpoch 互为逆运算', () => {
    const wallTime = getDateTimeInput(NEW_YEAR_UTC + 1234 * 1000, 'Europe/London')
    expect(zonedDateTimeToEpoch(wallTime, 'Europe/London')).toBe(NEW_YEAR_UTC + (1234 - 34) * 1000)
  })
})

describe('zonedDateTimeToEpoch', () => {
  it('换算东八区墙上时间', () => {
    expect(zonedDateTimeToEpoch('2026-01-01T08:00', 'Asia/Shanghai')).toBe(NEW_YEAR_UTC)
  })

  it('支持可选的秒字段', () => {
    expect(zonedDateTimeToEpoch('2026-01-01T00:00:30', 'UTC')).toBe(NEW_YEAR_UTC + 30_000)
  })

  it('按目标时区的夏令时偏移换算，而不是本机偏移', () => {
    // 伦敦 1 月为 UTC+0，7 月为 UTC+1（夏令时）。
    expect(zonedDateTimeToEpoch('2026-01-15T12:00', 'Europe/London')).toBe(Date.UTC(2026, 0, 15, 12, 0))
    expect(zonedDateTimeToEpoch('2026-07-15T12:00', 'Europe/London')).toBe(Date.UTC(2026, 6, 15, 11, 0))
  })

  it('夏令时跳时内不存在的本地时间返回 null', () => {
    // 2026-03-08 纽约 02:00 直接跳到 03:00，02:30 不存在。
    expect(zonedDateTimeToEpoch('2026-03-08T02:30', 'America/New_York')).toBeNull()
  })

  it('拒绝格式错误与日历上不存在的日期', () => {
    expect(zonedDateTimeToEpoch('', 'UTC')).toBeNull()
    expect(zonedDateTimeToEpoch('2026-01-01 08:00', 'UTC')).toBeNull()
    expect(zonedDateTimeToEpoch('2026-02-30T10:00', 'UTC')).toBeNull()
    expect(zonedDateTimeToEpoch('2026-01-01T24:00', 'UTC')).toBeNull()
  })

  it('无效时区名称抛 RangeError，由调用方的 try/catch 转成提示', () => {
    expect(() => zonedDateTimeToEpoch('2026-01-01T08:00', 'Not/AZone')).toThrow(RangeError)
  })
})
