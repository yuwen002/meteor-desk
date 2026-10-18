import { describe, expect, it } from 'vitest'
import { classifyLogLevel, filterTailLines } from './logLevel'

describe('classifyLogLevel', () => {
  it.each([
    ['2026-10-16T10:00:00Z INFO service ready', 'INFO'],
    ['2026-10-16 10:00:00.123 [INFO] request completed', 'INFO'],
    ['[WARN] retrying request', 'WARN'],
    ['2026-10-16T10:00:00Z WARNING upstream timeout', 'WARN'],
    ['{"severity":"warning","message":"slow"}', 'WARN'],
    ['{"level":"ERROR","message":"failed"}', 'ERROR'],
    ['FATAL unable to start', 'ERROR'],
    ['{"level":"critical","message":"unavailable"}', 'ERROR'],
    ['message mentions ERROR in its body', null],
    ['{"message":"the level=ERROR is only part of the message"}', null],
    ['plain log line', null],
  ] as const)('classifies %s as %s', (line, expected) => {
    expect(classifyLogLevel(line)).toBe(expected)
  })
})

describe('filterTailLines', () => {
  it('keeps all lines when ALL is selected', () => {
    const text = 'INFO ready\nplain detail\nERROR failed'
    expect(filterTailLines(text, 'ALL')).toBe(text)
  })

  it('keeps only lines matching the selected quick filter', () => {
    expect(filterTailLines('INFO ready\n[WARN] retry\nERROR failed', 'WARN')).toBe('[WARN] retry')
  })
})
