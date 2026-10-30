// 检索结果收藏：按文件路径与行号去重，只保存在本机 localStorage。
export interface BookmarkEntry {
  path: string
  lineNumber: number
  content: string
  query: string
  createdAt: string
}

export const MAX_BOOKMARKS = 500
export const BOOKMARK_STORAGE_KEY = 'meteordesk.bookmarks.v1'

export function bookmarkKey(path: string, lineNumber: number): string {
  return path.toLowerCase() + '#' + lineNumber
}

export function parseBookmarks(raw: string | null): BookmarkEntry[] {
  if (!raw) return []
  try {
    const data: unknown = JSON.parse(raw)
    if (!Array.isArray(data)) return []
    const entries: BookmarkEntry[] = []
    for (const item of data) {
      if (!item || typeof item !== 'object') continue
      const value = item as Record<string, unknown>
      const path = typeof value.path === 'string' ? value.path : ''
      const lineNumber = typeof value.lineNumber === 'number' ? Math.floor(value.lineNumber) : 0
      if (!path || !Number.isFinite(lineNumber) || lineNumber <= 0) continue
      entries.push({
        path,
        lineNumber,
        content: typeof value.content === 'string' ? value.content : '',
        query: typeof value.query === 'string' ? value.query : '',
        createdAt: typeof value.createdAt === 'string' ? value.createdAt : '',
      })
      if (entries.length >= MAX_BOOKMARKS) break
    }
    return entries
  } catch {
    return []
  }
}

export function toggleBookmark(list: BookmarkEntry[], entry: BookmarkEntry, limit = MAX_BOOKMARKS): BookmarkEntry[] {
  const key = bookmarkKey(entry.path, entry.lineNumber)
  if (list.some(item => bookmarkKey(item.path, item.lineNumber) === key)) {
    return list.filter(item => bookmarkKey(item.path, item.lineNumber) !== key)
  }
  return [entry, ...list].slice(0, limit)
}
