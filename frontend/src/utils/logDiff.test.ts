import { describe, expect, it } from 'vitest'
import type { DiffLine } from './textDiff'
import { isOneSidedAlert, summarizeOneSidedAlerts } from './logDiff'

function line(kind: DiffLine['kind'], value: string): DiffLine {
  return { kind, value }
}

describe('isOneSidedAlert', () => {
  it('flags only one-sided error/warn lines', () => {
    expect(isOneSidedAlert(line('equal', 'ERROR shared'))).toBe(false)
    expect(isOneSidedAlert(line('removed', 'ERROR only here'))).toBe(true)
    expect(isOneSidedAlert(line('added', 'WARN only there'))).toBe(true)
    expect(isOneSidedAlert(line('added', 'INFO routine'))).toBe(false)
    expect(isOneSidedAlert(line('removed', 'plain text'))).toBe(false)
  })
})

describe('summarizeOneSidedAlerts', () => {
  const diff: DiffLine[] = [
    line('equal', 'INFO boot'),
    line('removed', 'ERROR db down'), // 仅左侧 error
    line('removed', 'warning slow query'), // 仅左侧 warn
    line('added', 'FATAL crash'), // 仅右侧 error（fatal 归入 error）
    line('added', 'INFO new line'), // 非告警
    line('removed', 'debug noise'), // 非告警
  ]

  it('counts one-sided error/warn per side', () => {
    const summary = summarizeOneSidedAlerts(diff)
    expect(summary).toMatchObject({ leftError: 1, leftWarn: 1, rightError: 1, rightWarn: 0, total: 3 })
  })

  it('collects samples tagged with side and level', () => {
    const summary = summarizeOneSidedAlerts(diff)
    expect(summary.samples).toHaveLength(3)
    expect(summary.samples[0]).toMatchObject({ side: 'left', level: 'error', text: 'ERROR db down' })
    expect(summary.samples[2]).toMatchObject({ side: 'right', level: 'error', text: 'FATAL crash' })
  })

  it('caps the number of samples but still counts everything', () => {
    const summary = summarizeOneSidedAlerts(diff, 1)
    expect(summary.samples).toHaveLength(1)
    expect(summary.total).toBe(3)
  })

  it('returns an empty summary when there are no alerts', () => {
    const summary = summarizeOneSidedAlerts([line('equal', 'a'), line('added', 'INFO b')])
    expect(summary.total).toBe(0)
    expect(summary.samples).toEqual([])
  })
})
