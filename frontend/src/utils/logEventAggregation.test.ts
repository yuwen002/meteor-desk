import { describe, expect, it } from 'vitest'
import { aggregateLogEvents } from './logEventAggregation'

describe('aggregateLogEvents', () => {
  it('attaches stack trace lines to the preceding error event', () => {
    const events = aggregateLogEvents([
      '2026-10-20T10:00:00Z ERROR request failed',
      '    at main.run (main.go:42)',
      '    at main.start (main.go:18)',
      '2026-10-20T10:00:01Z INFO next request started',
    ].join('\n'))
    expect(events).toHaveLength(2)
    expect(events[0]).toMatchObject({ startLine: 1, endLine: 3, level: 'ERROR', timestamp: '2026-10-20T10:00:00Z' })
    expect(events[0].content).toContain('at main.run')
    expect(events[1].level).toBe('INFO')
  })

  it('groups Python traceback continuation lines', () => {
    const events = aggregateLogEvents([
      'ERROR Traceback (most recent call last):',
      '  File "worker.py", line 42, in run',
      '    raise RuntimeError("failed")',
    ].join('\n'))
    expect(events).toHaveLength(1)
    expect(events[0].endLine).toBe(3)
  })

  it('keeps unrecognized lines and bounds event content', () => {
    const events = aggregateLogEvents('ERROR ' + 'x'.repeat(100), { maxEventBytes: 20 })
    expect(events[0].content).toHaveLength(20)
    expect(events[0].truncated).toBe(true)
  })

  it('supports a custom timestamp expression and tolerates an invalid one', () => {
    const input = '2026/10/20 10:00:00 ERROR first\n2026/10/20 10:00:01 ERROR second'
    const events = aggregateLogEvents(input, { timestampPattern: '^(\\d{4}/\\d{2}/\\d{2} \\d{2}:\\d{2}:\\d{2})' })
    expect(events).toHaveLength(2)
    expect(events[1].timestamp).toBe('2026/10/20 10:00:01')
    expect(aggregateLogEvents(input, { timestampPattern: '[' })).toHaveLength(1)
  })

  it('returns no events for empty input', () => {
    expect(aggregateLogEvents('')).toEqual([])
  })
})
