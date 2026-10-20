import { describe, expect, it } from 'vitest'
import { filterLogResults } from './logResultFilters'
import { excludeLogResults } from './logSearchPresentation'

describe('filterLogResults', () => {
  const matches = [
    { lineNumber: 1, content: '2026-10-16T10:00:00Z INFO ready' },
    { lineNumber: 2, content: '2026-10-16T10:30:00Z WARN retry' },
    { lineNumber: 3, content: '{"timestamp":"2026-10-16T11:00:00Z","level":"ERROR"}' },
    { lineNumber: 4, content: 'undated detail' },
  ]

  it('keeps every result when no filters are selected', () => {
    expect(filterLogResults(matches, {}, 'ALL')).toEqual(matches)
  })

  it('combines the selected level and time range', () => {
    expect(filterLogResults(matches, {
      from: '2026-10-16T10:15:00Z',
      to: '2026-10-16T10:45:00Z',
    }, 'WARN')).toEqual([matches[1]])
  })

  it('composes time, level, and exclusion filters without changing result order', () => {
    const expanded = [
      ...matches,
      { lineNumber: 5, content: '{"timestamp":"2026-10-16T11:30:00Z","level":"ERROR","message":"permission denied"}' },
    ]
    const inWindow = filterLogResults(expanded, {
      from: '2026-10-16T10:15:00Z',
      to: '2026-10-16T12:00:00Z',
    }, 'ERROR')
    expect(excludeLogResults(inWindow, 'permission denied')).toEqual([matches[2]])
    expect(expanded.map(item => item.lineNumber)).toEqual([1, 2, 3, 4, 5])
  })

  it('shows undated lines when only a level filter is active', () => {
    expect(filterLogResults(matches, {}, 'ALL')).toContain(matches[3])
  })

  it('excludes unknown levels when a specific level is selected', () => {
    expect(filterLogResults(matches, {}, 'ERROR')).toEqual([matches[2]])
  })

  it('does not reorder or mutate the original matches', () => {
    const source = matches.slice()
    expect(filterLogResults(matches, {}, 'INFO')).toEqual([matches[0]])
    expect(matches).toEqual(source)
  })
})
