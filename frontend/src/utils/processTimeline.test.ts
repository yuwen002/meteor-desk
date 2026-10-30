import { describe, expect, it } from 'vitest'
import { countEvents, eventLabel, formatEventDuration, latestStartupMs } from './processTimeline'

describe('eventLabel', () => {
  it('maps known kinds and keeps unknown kinds readable', () => {
    expect(eventLabel('start')).toBe('启动')
    expect(eventLabel('ready')).toBe('就绪')
    expect(eventLabel('restart')).toBe('自动重启')
    expect(eventLabel('exit')).toBe('退出')
    expect(eventLabel('crashed')).toBe('crashed')
  })
})

describe('formatEventDuration', () => {
  it('scales units with the duration', () => {
    expect(formatEventDuration(undefined)).toBe('—')
    expect(formatEventDuration(-1)).toBe('—')
    expect(formatEventDuration(420)).toBe('420 毫秒')
    expect(formatEventDuration(2400)).toBe('2.4 秒')
    expect(formatEventDuration(45000)).toBe('45 秒')
    expect(formatEventDuration(65000)).toBe('1 分 05 秒')
    expect(formatEventDuration(3 * 3600 * 1000 + 720000)).toBe('3 小时 12 分')
  })
})

describe('latestStartupMs', () => {
  it('returns the newest ready duration and ignores exits', () => {
    const events = [
      { kind: 'start', at: '2026-10-31T00:00:00Z' },
      { kind: 'ready', at: '2026-10-31T00:00:01Z', durationMs: 900 },
      { kind: 'exit', at: '2026-10-31T00:00:05Z', durationMs: 4000 },
      { kind: 'start', at: '2026-10-31T00:01:00Z' },
      { kind: 'ready', at: '2026-10-31T00:01:02Z', durationMs: 1800 },
    ]
    expect(latestStartupMs(events)).toBe(1800)
    expect(latestStartupMs([{ kind: 'start', at: '2026-10-31T00:00:00Z' }])).toBeNull()
  })
})

describe('countEvents', () => {
  it('counts events of a single kind', () => {
    const events = [{ kind: 'restart', at: 'a' }, { kind: 'start', at: 'b' }, { kind: 'restart', at: 'c' }]
    expect(countEvents(events, 'restart')).toBe(2)
    expect(countEvents(events, 'exit')).toBe(0)
  })
})
