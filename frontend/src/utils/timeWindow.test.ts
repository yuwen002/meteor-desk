import { describe, expect, it } from 'vitest'
import { resolveTimeWindow, type TimeWindowInput } from './timeWindow'

const base: TimeWindowInput = { enabled: false, preset: '', start: '', end: '', zone: 'utc' }

describe('resolveTimeWindow', () => {
  it('未启用时直接返回不启用', () => {
    expect(resolveTimeWindow({ ...base, enabled: false }, 0, 'Asia/Shanghai').enabled).toBe(false)
  })

  it('相对快捷项以提交时刻为基准锚定', () => {
    const now = 1_700_000_000_000
    const got = resolveTimeWindow({ ...base, enabled: true, preset: '30m' }, now, 'UTC')
    expect(got.enabled).toBe(true)
    expect(got.endMs).toBe(now)
    expect(got.startMs).toBe(now - 30 * 60_000)
  })

  it('绝对起止按 UTC 参照换算成瞬间', () => {
    const got = resolveTimeWindow({ ...base, enabled: true, start: '2026-01-02T10:00', end: '2026-01-02T11:00', zone: 'utc' }, 0, 'UTC')
    expect(got.enabled).toBe(true)
    // 2026-01-02T10:00Z
    expect(got.startMs).toBe(Date.UTC(2026, 0, 2, 10, 0, 0))
    expect(got.endMs).toBe(Date.UTC(2026, 0, 2, 11, 0, 0))
  })

  it('仅填开始时间时下界生效、上界开放', () => {
    const got = resolveTimeWindow({ ...base, enabled: true, start: '2026-01-02T10:00', zone: 'utc' }, 0, 'UTC')
    expect(got.enabled).toBe(true)
    expect(got.startMs).toBe(Date.UTC(2026, 0, 2, 10, 0, 0))
    expect(got.endMs).toBe(0)
  })

  it('勾选但没有任何边界时按未启用处理', () => {
    const got = resolveTimeWindow({ ...base, enabled: true, zone: 'utc' }, 0, 'UTC')
    expect(got.enabled).toBe(false)
  })

  it('开始时间不早于结束时间时报错', () => {
    const got = resolveTimeWindow({ ...base, enabled: true, start: '2026-01-02T12:00', end: '2026-01-02T11:00', zone: 'utc' }, 0, 'UTC')
    expect(got.enabled).toBe(false)
    expect(got.error).toContain('必须早于')
  })

  it('非法时间格式报错而非静默忽略', () => {
    const got = resolveTimeWindow({ ...base, enabled: true, start: '2026-13-45T99:99', zone: 'utc' }, 0, 'UTC')
    expect(got.enabled).toBe(false)
    expect(got.error).toBeTruthy()
  })

  it('本地参照与 UTC 参照得到不同瞬间', () => {
    const local = resolveTimeWindow({ ...base, enabled: true, start: '2026-01-02T10:00', zone: 'local' }, 0, 'Asia/Shanghai')
    const utc = resolveTimeWindow({ ...base, enabled: true, start: '2026-01-02T10:00', zone: 'utc' }, 0, 'UTC')
    // Asia/Shanghai 比 UTC 早 8 小时，同一墙上时间在本地参照下瞬间更小。
    expect(utc.startMs - local.startMs).toBe(8 * 60 * 60_000)
  })
})
