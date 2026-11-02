import { computed, ref, type Ref } from 'vue'
import { BOOKMARK_STORAGE_KEY, bookmarkKey, parseBookmarks, toggleBookmark, type BookmarkEntry } from '../utils/bookmarks'

// 检索结果收藏状态：以「文件路径 + 行号」为唯一键，只写入本机 localStorage。
// 视图只消费 bookmarkKeys / fileBookmarks，写入失败通过 errorMessage 回流到页面提示。
export function useBookmarks(options: { filePath: Ref<string>; query: Ref<string>; errorMessage: Ref<string> }) {
  const bookmarks = ref<BookmarkEntry[]>(loadStoredBookmarks())

  const bookmarkKeys = computed(() => new Set(bookmarks.value.map(item => bookmarkKey(item.path, item.lineNumber))))
  const fileBookmarks = computed(() => bookmarks.value.filter(item => item.path.toLowerCase() === options.filePath.value.toLowerCase()).length)

  function isBookmarked(match: LogMatch) {
    return bookmarkKeys.value.has(bookmarkKey(options.filePath.value, match.lineNumber))
  }

  function toggleMatchBookmark(match: LogMatch) {
    bookmarks.value = toggleBookmark(bookmarks.value, {
      path: options.filePath.value,
      lineNumber: match.lineNumber,
      content: match.content.slice(0, 512),
      query: options.query.value,
      createdAt: new Date().toISOString(),
    })
    persist()
  }

  function persist() {
    try {
      window.localStorage.setItem(BOOKMARK_STORAGE_KEY, JSON.stringify(bookmarks.value))
    } catch {
      options.errorMessage.value = '无法保存收藏，请检查本地存储权限。'
    }
  }

  return { bookmarks, bookmarkKeys, fileBookmarks, isBookmarked, toggleMatchBookmark }
}

function loadStoredBookmarks(): BookmarkEntry[] {
  try {
    return parseBookmarks(window.localStorage.getItem(BOOKMARK_STORAGE_KEY))
  } catch {
    return []
  }
}
