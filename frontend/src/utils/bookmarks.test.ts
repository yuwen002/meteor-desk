import { describe, expect, it } from 'vitest'
import { MAX_BOOKMARKS, parseBookmarks, toggleBookmark, type BookmarkEntry } from './bookmarks'

function entry(path: string, lineNumber: number): BookmarkEntry {
  return { path, lineNumber, content: 'boom', query: 'boom', createdAt: '2026-10-31T00:00:00.000Z' }
}

describe('bookmarks', () => {
  it('parses stored bookmarks and drops invalid entries', () => {
    const raw = JSON.stringify([
      entry('C:/logs/app.log', 3),
      { path: '', lineNumber: 4 },
      { path: 'C:/logs/app.log', lineNumber: 0 },
      { path: 'C:/logs/other.log', lineNumber: 7, content: 1 },
      'invalid',
    ])
    expect(parseBookmarks(raw)).toEqual([
      entry('C:/logs/app.log', 3),
      { path: 'C:/logs/other.log', lineNumber: 7, content: '', query: '', createdAt: '' },
    ])
    expect(parseBookmarks('not-json')).toEqual([])
    expect(parseBookmarks(null)).toEqual([])
  })

  it('toggles by path and line number regardless of path case', () => {
    const added = toggleBookmark([], entry('C:/Logs/app.log', 3))
    expect(added).toHaveLength(1)
    expect(toggleBookmark(added, entry('c:/logs/APP.log', 3))).toEqual([])
  })

  it('keeps the newest bookmarks under the limit', () => {
    const list = toggleBookmark([], entry('C:/logs/app.log', 1), 2)
    const second = toggleBookmark(list, entry('C:/logs/app.log', 2), 2)
    const third = toggleBookmark(second, entry('C:/logs/app.log', 3), 2)
    expect(third.map(item => item.lineNumber)).toEqual([3, 2])
    expect(MAX_BOOKMARKS).toBeGreaterThan(2)
  })
})
