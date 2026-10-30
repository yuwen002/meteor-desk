import { describe, expect, it } from 'vitest'
import { detectLevel, normalizeSignature, summarizeMatches, summaryText } from './aggregate'

describe('detectLevel', () => {
  it('recognizes common log levels', () => {
    expect(detectLevel('2026-10-30 12:00:00 ERROR boom')).toBe('error')
    expect(detectLevel('{"level":"warn","msg":"slow"}')).toBe('warn')
    expect(detectLevel('[info] started')).toBe('info')
    expect(detectLevel('FATAL crash')).toBe('error')
    expect(detectLevel('trace detail')).toBe('trace')
    expect(detectLevel('2026-10-30 DEBUG cache hit')).toBe('debug')
  })

  it('does not treat words containing level names as levels', () => {
    expect(detectLevel('information only')).toBe('other')
    expect(detectLevel('debugging session')).toBe('other')
    expect(detectLevel('plain line')).toBe('other')
  })
})

describe('normalizeSignature', () => {
  it('strips timestamp and level prefix', () => {
    expect(normalizeSignature('2026-10-30 12:00:00.123 ERROR connection refused')).toBe('connection refused')
  })

  it('normalizes UUIDs and numbers', () => {
    expect(normalizeSignature('ERROR order 550e8400-e29b-41d4-a716-446655440000 failed, retry 3')).toBe(
      'order <id> failed, retry <num>',
    )
  })

  it('truncates long signatures', () => {
    const signature = normalizeSignature('ERROR ' + 'x'.repeat(120))
    expect(signature).toHaveLength(81)
    expect(signature.endsWith('…')).toBe(true)
  })
})

describe('summarizeMatches', () => {
  const matches = [
    { lineNumber: 10, content: '2026-10-30 10:00:00 ERROR connection refused' },
    { lineNumber: 20, content: '2026-10-30 10:00:01 WARN slow request 3000ms' },
    { lineNumber: 30, content: '2026-10-30 10:00:02 ERROR connection refused' },
    { lineNumber: 40, content: '2026-10-30 10:00:03 INFO ready' },
    { lineNumber: 50, content: '2026-10-30 10:00:04 info ready' },
  ]

  it('counts levels and line range', () => {
    const summary = summarizeMatches(matches)
    expect(summary.total).toBe(5)
    expect(summary.firstLine).toBe(10)
    expect(summary.lastLine).toBe(50)
    expect(summary.levels).toEqual([
      { level: 'error', count: 2 },
      { level: 'warn', count: 1 },
      { level: 'info', count: 2 },
    ])
  })

  it('ranks error signatures by frequency', () => {
    const summary = summarizeMatches(matches)
    expect(summary.signatures).toEqual([
      { text: 'connection refused', count: 2 },
      { text: 'slow request <num>ms', count: 1 },
    ])
  })

  it('honors the signature limit', () => {
    expect(summarizeMatches(matches, 1).signatures).toHaveLength(1)
  })

  it('handles an empty result list', () => {
    expect(summarizeMatches([])).toEqual({ total: 0, firstLine: 0, lastLine: 0, levels: [], signatures: [] })
  })
})

describe('summaryText', () => {
  it('renders a copyable summary', () => {
    const text = summaryText(summarizeMatches([
      { lineNumber: 5, content: 'ERROR boom' },
      { lineNumber: 9, content: 'INFO ok' },
    ]))
    expect(text).toContain('检索结果摘要')
    expect(text).toContain('总计：2 条（行号 5–9）')
    expect(text).toContain('级别：ERROR 1 | INFO 1')
    expect(text).toContain('1. boom — 1 次')
  })
})
