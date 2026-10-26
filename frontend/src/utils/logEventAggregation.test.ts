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

  it('supports custom timestamps and falls back to built-in parsing for invalid expressions', () => {
    const input = '2026/10/20 10:00:00 ERROR first\n2026/10/20 10:00:01 ERROR second'
    const events = aggregateLogEvents(input, { timestampPattern: '^(\\d{4}/\\d{2}/\\d{2} \\d{2}:\\d{2}:\\d{2})' })
    expect(events).toHaveLength(2)
    expect(events[1].timestamp).toBe('2026/10/20 10:00:01')
    expect(aggregateLogEvents(input, { timestampPattern: '[' })).toHaveLength(2)
  })

  it('returns no events for empty input', () => {
    expect(aggregateLogEvents('')).toEqual([])
  })

  it('starts a new event at a level header after unclassified lines', () => {
    const events = aggregateLogEvents([
      'service process initialized',
      'background worker waiting',
      'ERROR database unavailable',
      'Caused by: connection refused',
      '    at store.open (store.go:17)',
      'INFO recovery complete',
    ].join('\n'))
    expect(events).toHaveLength(3)
    expect(events[0]).toMatchObject({ startLine: 1, endLine: 2, level: 'UNKNOWN' })
    expect(events[1]).toMatchObject({ startLine: 3, endLine: 5, level: 'ERROR' })
    expect(events[1].content).toContain('Caused by: connection refused')
    expect(events[2]).toMatchObject({ startLine: 6, endLine: 6, level: 'INFO' })
  })

  it('enforces maxEventBytes using UTF-8 bytes without splitting a Unicode code point', () => {
    const event = aggregateLogEvents('ERROR 错误🙂', { maxEventBytes: 10 })[0]
    expect(new TextEncoder().encode(event.content).length).toBeLessThanOrEqual(10)
    expect(event.content).toBe('ERROR 错')
    expect(event.truncated).toBe(true)
  })

  it('recognizes slash-separated timestamps and keeps payload text attached', () => {
    const events = aggregateLogEvents([
      '2026/10/20 10:00:00,123 ERROR request failed',
      'payload says INFO but is not a header',
      '2026/10/20 10:00:01,456 INFO recovered',
    ].join('\n'))
    expect(events).toHaveLength(2)
    expect(events[0].timestamp).toBe('2026/10/20 10:00:00,123')
    expect(events[0].endLine).toBe(2)
    expect(events[1].level).toBe('INFO')
  })

  it('does not split an unindented cause line into a separate event', () => {
    const events = aggregateLogEvents([
      'ERROR request failed',
      'Caused by: ERROR token appears in explanation',
      'Traceback (most recent call last):',
      '  File "worker.py", line 12, in run',
    ].join('\n'))
    expect(events).toHaveLength(1)
    expect(events[0].endLine).toBe(4)
  })


  it('keeps multiline JSON payloads together when strings contain timestamps and level words', () => {
    const events = aggregateLogEvents([
      '2026-10-20T10:00:00Z ERROR response payload:',
      '{',
      '  "message": "2026-10-20T10:01:00Z ERROR inside JSON text",',
      '  "nested": { "status": "INFO", "items": [1, 2] }',
      '}',
      '2026-10-20T10:00:02Z INFO next event',
    ].join('\n'))
    expect(events).toHaveLength(2)
    expect(events[0]).toMatchObject({ startLine: 1, endLine: 5, level: 'ERROR' })
    expect(events[0].content).toContain('inside JSON text')
    expect(events[1]).toMatchObject({ startLine: 6, endLine: 6, level: 'INFO' })
  })

  it('starts a separate event for standalone JSON after unrelated text', () => {
    const events = aggregateLogEvents('plain startup line\n{"ok":true}\nERROR failed')
    expect(events).toHaveLength(3)
    expect(events[1].content).toBe('{"ok":true}')
    expect(events[2].level).toBe('ERROR')
  })

  it('bounds long stack traces while preserving the next timestamped event', () => {
    const stack = Array.from({ length: 80 }, (_, i) => `    at worker.step${i} (worker.go:${i + 1})`)
    const events = aggregateLogEvents([
      '2026-10-20T10:00:00Z ERROR request failed',
      ...stack,
      '2026-10-20T10:00:01Z INFO recovered',
    ].join('\\n'), { maxEventBytes: 128 })
    expect(events).toHaveLength(2)
    expect(events[0].truncated).toBe(true)
    expect(new TextEncoder().encode(events[0].content).length).toBeLessThanOrEqual(128)
    expect(events[0].endLine).toBe(81)
    expect(events[1]).toMatchObject({ startLine: 82, level: 'INFO' })
  })

})
