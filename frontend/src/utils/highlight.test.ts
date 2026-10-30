import { describe, expect, it } from 'vitest'
import { highlightSegments } from './highlight'

describe('highlightSegments', () => {
  it('splits every case-sensitive text hit', () => {
    expect(highlightSegments('boom at 1, boom at 2', 'boom', false)).toEqual([
      { text: 'boom', hit: true },
      { text: ' at 1, ', hit: false },
      { text: 'boom', hit: true },
      { text: ' at 2', hit: false },
    ])
    expect(highlightSegments('BOOM', 'boom', false)).toEqual([{ text: 'BOOM', hit: false }])
  })

  it('returns the original text when nothing matches or the query is empty', () => {
    expect(highlightSegments('plain text', 'boom', false)).toEqual([{ text: 'plain text', hit: false }])
    expect(highlightSegments('plain text', '', false)).toEqual([{ text: 'plain text', hit: false }])
    expect(highlightSegments('', 'boom', false)).toEqual([{ text: '', hit: false }])
  })

  it('highlights regular expression hits and skips zero-width matches', () => {
    const segments = highlightSegments('id=AB12 id=CD34', 'id=[A-Z]{2}\\d{2}', true)
    expect(segments.filter(segment => segment.hit).map(segment => segment.text)).toEqual(['id=AB12', 'id=CD34'])
    expect(highlightSegments('abc', 'x*', true)).toEqual([{ text: 'abc', hit: false }])
  })

  it('supports the Go (?i) prefix and falls back for patterns JavaScript cannot compile', () => {
    expect(highlightSegments('BOOM', '(?i)boom', true)).toEqual([{ text: 'BOOM', hit: true }])
    expect(highlightSegments('value 1', '(?P<name>value)', true)).toEqual([{ text: 'value 1', hit: false }])
  })

  it('caps the number of highlighted segments', () => {
    const text = 'a a a a a a'
    const segments = highlightSegments(text, 'a', false, 2)
    expect(segments.filter(segment => segment.hit)).toHaveLength(2)
    expect(segments.map(segment => segment.text).join('')).toBe(text)
  })
})
