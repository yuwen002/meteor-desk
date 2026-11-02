import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { BOOKMARK_STORAGE_KEY } from '../utils/bookmarks'
import { useBookmarks } from './useBookmarks'

function create(options?: { filePath?: string; query?: string; stored?: unknown[] }) {
  const filePath = ref(options?.filePath ?? 'C:/logs/app.log')
  const query = ref(options?.query ?? 'boom')
  const errorMessage = ref('')
  if (options?.stored) window.localStorage.setItem(BOOKMARK_STORAGE_KEY, JSON.stringify(options.stored))
  const bookmarks = useBookmarks({ filePath, query, errorMessage })
  return { filePath, query, errorMessage, ...bookmarks }
}

function stored(): { path: string; lineNumber: number; content: string; query: string }[] {
  return JSON.parse(window.localStorage.getItem(BOOKMARK_STORAGE_KEY) ?? '[]')
}

function match(lineNumber: number, content: string): LogMatch {
  return { lineNumber, content, contentClipped: false, contextBefore: [], contextAfter: [] }
}

describe('bookmark view state', () => {
  afterEach(() => {
    window.localStorage.clear()
  })

  it('loads stored entries and counts only the current file', () => {
    const { fileBookmarks, bookmarkKeys } = create({
      stored: [
        { path: 'C:/logs/app.log', lineNumber: 3, content: 'boom', query: 'boom', createdAt: '' },
        { path: 'C:/logs/other.log', lineNumber: 8, content: 'boom', query: 'boom', createdAt: '' },
        { path: '', lineNumber: 9, content: 'boom', query: '', createdAt: '' },
      ],
    })
    expect(fileBookmarks.value).toBe(1)
    // 非法条目（空路径）在解析阶段就被丢掉。
    expect(bookmarkKeys.value.size).toBe(2)
  })

  it('adds and removes a bookmark and persists it locally', () => {
    const { isBookmarked, toggleMatchBookmark } = create()
    const row = match(7, 'timeout waiting')
    expect(isBookmarked(row)).toBe(false)

    toggleMatchBookmark(row)
    expect(isBookmarked(row)).toBe(true)
    expect(stored()).toHaveLength(1)
    expect(stored()[0]).toMatchObject({ path: 'C:/logs/app.log', lineNumber: 7, content: 'timeout waiting', query: 'boom' })

    toggleMatchBookmark(row)
    expect(isBookmarked(row)).toBe(false)
    expect(stored()).toEqual([])
  })

  it('matches the file path case-insensitively and caps the stored content length', () => {
    const { filePath, isBookmarked, toggleMatchBookmark } = create({ stored: [{ path: 'C:/LOGS/app.log', lineNumber: 4, content: 'x', query: '', createdAt: '' }] })
    expect(isBookmarked(match(4, 'boom'))).toBe(true)

    const row = match(9, 'a'.repeat(900))
    toggleMatchBookmark(row)
    filePath.value = 'c:/logs/APP.log'
    expect(isBookmarked(row)).toBe(true)
    expect(stored()[0].content).toHaveLength(512)
  })

  it('reports a storage failure without breaking the in-memory list', () => {
    const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('quota exceeded') })
    const { bookmarks, errorMessage, toggleMatchBookmark } = create()
    toggleMatchBookmark(match(3, 'boom'))
    expect(errorMessage.value).toBe('无法保存收藏，请检查本地存储权限。')
    expect(bookmarks.value).toHaveLength(1)
    setItem.mockRestore()
  })
})
