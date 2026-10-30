import { describe, expect, it } from 'vitest'
import { sparklinePoints, summarizeHistory } from './resourceHistory'

describe('summarizeHistory', () => {
  it('collects series and peaks from samples', () => {
    const summary = summarizeHistory([
      { cpuPercent: 10.5, memoryBytes: 1000 },
      { cpuPercent: 42, memoryBytes: 1600 },
      { cpuPercent: 3, memoryBytes: 1300 },
    ])
    expect(summary.count).toBe(3)
    expect(summary.peakCPU).toBe(42)
    expect(summary.peakMemory).toBe(1600)
    expect(summary.memoryDelta).toBe(300)
    expect(summary.cpuValues).toEqual([10.5, 42, 3])
  })

  it('treats missing or non-finite values as zero', () => {
    const summary = summarizeHistory([{ memoryBytes: Number.NaN }, { cpuPercent: Number.POSITIVE_INFINITY, memoryBytes: 2048 }])
    expect(summary.cpuValues).toEqual([0, 0])
    expect(summary.memoryValues).toEqual([0, 2048])
    expect(summary.peakCPU).toBe(0)
    expect(summary.memoryDelta).toBe(2048)
  })

  it('returns an empty summary without samples', () => {
    expect(summarizeHistory([])).toMatchObject({ count: 0, peakCPU: 0, peakMemory: 0, memoryDelta: 0 })
  })
})

describe('sparklinePoints', () => {
  it('maps the minimum to the bottom edge and the maximum to the top edge', () => {
    expect(sparklinePoints([10, 20], 120, 32)).toBe('0.0,32.0 120.0,0.0')
  })

  it('draws a flat midline for constant series and single samples', () => {
    expect(sparklinePoints([5, 5, 5], 100, 20)).toBe('0.0,10.0 50.0,10.0 100.0,10.0')
    expect(sparklinePoints([7], 120, 32)).toBe('0,16.0 120,16.0')
  })

  it('returns an empty string without values', () => {
    expect(sparklinePoints([], 120, 32)).toBe('')
  })
})
