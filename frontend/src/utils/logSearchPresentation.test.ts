import { describe, expect, it } from 'vitest'
import { excludeLogResults, splitHighlightedText } from './logSearchPresentation'

describe('splitHighlightedText', () => {
  it('highlights literal keywords without interpreting regex characters', () => {
    expect(splitHighlightedText('error [db] failed', '[db]')).toEqual([
      { text: 'error ', matched: false },
      { text: '[db]', matched: true },
      { text: ' failed', matched: false },
    ])
  })

  it('highlights multiple matches case-insensitively', () => {
    expect(splitHighlightedText('ERROR then error', 'error')).toEqual([
      { text: 'ERROR', matched: true },
      { text: ' then ', matched: false },
      { text: 'error', matched: true },
    ])
  })

  it('supports regex and safely handles invalid patterns', () => {
    expect(splitHighlightedText('E12 E34', 'E\\d+', true).filter((part) => part.matched).map((part) => part.text)).toEqual(['E12', 'E34'])
    expect(splitHighlightedText('unchanged', '[', true)).toEqual([{ text: 'unchanged', matched: false }])
  })

  it('returns plain text when query is empty or unmatched', () => {
    expect(splitHighlightedText('plain', '')).toEqual([{ text: 'plain', matched: false }])
    expect(splitHighlightedText('plain', 'missing')).toEqual([{ text: 'plain', matched: false }])
  })
})

describe('excludeLogResults', () => {
  const rows = [{ content: 'INFO health check' }, { content: 'ERROR database failed' }, { content: 'WARN Retry' }]

  it('excludes any comma or newline separated term case-insensitively', () => {
    expect(excludeLogResults(rows, 'health check, retry')).toEqual([{ content: 'ERROR database failed' }])
    expect(excludeLogResults(rows, 'health check\\nretry')).toEqual([{ content: 'ERROR database failed' }])
  })

  it('keeps all results when no exclusion terms are provided', () => {
    expect(excludeLogResults(rows, ' , \\n ')).toEqual(rows)
  })
})
