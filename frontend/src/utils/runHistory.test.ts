import { describe, expect, it } from 'vitest'
import {
  formatRunBytes,
  formatRunDuration,
  formatRunPeakCPU,
  runExitLabel,
  runExitSummary,
  summarizeRunRecords,
} from './runHistory'

describe('run history helpers', () => {
  it('labels exit reasons and falls back to the raw value', () => {
    expect(runExitLabel('nonzero')).toBe('非零退出')
    expect(runExitLabel('manual-stop')).toBe('手动停止')
    expect(runExitLabel('mystery')).toBe('mystery')
    expect(runExitLabel(undefined)).toBe('')
  })

  it('combines exit reason with the code when present', () => {
    expect(runExitSummary('nonzero', 2)).toBe('非零退出（2）')
    expect(runExitSummary('normal', 0)).toBe('正常退出（0）')
    expect(runExitSummary('signal')).toBe('信号终止')
    expect(runExitSummary(undefined, 1)).toBe('')
  })

  it('formats durations across unit boundaries', () => {
    expect(formatRunDuration(640)).toBe('640 毫秒')
    expect(formatRunDuration(1800)).toBe('1.8 秒')
    expect(formatRunDuration(60000)).toBe('1 分 00 秒')
    expect(formatRunDuration(-1)).toBe('—')
    expect(formatRunDuration(undefined)).toBe('—')
  })

  it('formats peak bytes and peak cpu', () => {
    expect(formatRunBytes(20 * 1024 * 1024)).toBe('20.0 MB')
    expect(formatRunBytes(0)).toBe('—')
    expect(formatRunPeakCPU(88)).toBe('88.0%')
    expect(formatRunPeakCPU(0)).toBe('—')
  })

  it('summarizes counts, abnormal exits and restarts', () => {
    const summary = summarizeRunRecords([
      { exitReason: 'normal', restarts: 0 },
      { exitReason: 'nonzero', restarts: 3 },
      { exitReason: 'start-error', restarts: 1 },
    ])
    expect(summary).toEqual({ count: 3, nonzeroExits: 2, totalRestarts: 4 })
  })
})
