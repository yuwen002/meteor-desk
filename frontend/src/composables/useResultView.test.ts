import { describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import { bookmarkKey } from '../utils/bookmarks'
import { useResultView } from './useResultView'

function match(lineNumber: number, content: string): LogMatch {
  return { lineNumber, content, contentClipped: false, contextBefore: [], contextAfter: [] }
}

const SAMPLE = [
  match(2, 'boom happened'),
  match(5, 'timeout waiting'),
  match(9, 'BOOM retry'),
]

function createView(matches: LogMatch[], options?: { filePath?: string; bookmarks?: number[] }) {
  const filePath = ref(options?.filePath ?? 'C:/logs/app.log')
  const bookmarkKeys = ref(new Set((options?.bookmarks ?? []).map(line => bookmarkKey(filePath.value, line))))
  const view = useResultView({ matches: () => matches, filePath, bookmarkKeys })
  return { filePath, bookmarkKeys, ...view }
}

// jsdom 不做布局，原生 scrollTop 恒为 0；用可读写替身才能断言滚动位置。
function scrollableElement() {
  const element = document.createElement('div')
  let value = 0
  Object.defineProperty(element, 'scrollTop', { configurable: true, get: () => value, set: (next: number) => { value = next } })
  return element
}

describe('result view state', () => {
  it('filters by content or line number without touching the source list', () => {
    const view = createView(SAMPLE)
    expect(view.filteredMatches.value.map(item => item.lineNumber)).toEqual([2, 5, 9])

    view.resultFilter.value = 'BOOM'
    expect(view.filteredMatches.value.map(item => item.lineNumber)).toEqual([2, 9])

    view.resultFilter.value = '5'
    expect(view.filteredMatches.value.map(item => item.content)).toEqual(['timeout waiting'])
    // 关键字过滤读的是原始结果集，反复修改不会污染它。
    expect(view.keywordFiltered.value).toHaveLength(1)
    view.resultFilter.value = ''
    expect(view.filteredMatches.value).toHaveLength(3)
  })

  it('shows only bookmarked lines of the current file', () => {
    const view = createView(SAMPLE, { bookmarks: [5] })
    view.bookmarkOnly.value = true
    expect(view.filteredMatches.value.map(item => item.lineNumber)).toEqual([5])
  })

  it('reverses a copy when sorting descending', () => {
    const view = createView(SAMPLE)
    view.resultSort.value = 'desc'
    expect(view.filteredMatches.value.map(item => item.lineNumber)).toEqual([9, 5, 2])
    expect(view.keywordFiltered.value.map(item => item.lineNumber)).toEqual([2, 5, 9])
  })

  it('applies and cancels the time bucket filter', () => {
    const view = createView([
      match(2, '2026-10-30 10:00:00 ERROR connection refused'),
      match(5, '2026-10-30 10:00:01 WARN slow request'),
      match(9, '2026-10-30 10:00:02 ERROR connection refused'),
    ])
    const buckets = view.timeDistribution.value.buckets
    expect(buckets).toHaveLength(3)

    view.selectTimelineBucket(buckets[1])
    expect(view.timeFilter.value?.label).toBe(buckets[1].label)
    expect(view.filteredMatches.value.map(item => item.lineNumber)).toEqual([5])
    expect(view.timelineActive(buckets[1])).toBe(true)
    // 叠加过滤后柱状图仍保留全貌。
    expect(view.timeDistribution.value.buckets).toHaveLength(3)

    view.selectTimelineBucket(buckets[1])
    expect(view.timeFilter.value).toBeNull()
    expect(view.filteredMatches.value).toHaveLength(3)
  })

  it('titles buckets with their error and warn counts', () => {
    const view = createView([match(2, '2026-10-30 10:00:00 ERROR connection refused')])
    const bucket = view.timeDistribution.value.buckets[0]
    expect(view.timelineTitle(bucket)).toBe(bucket.label + ' · ERROR 1')
    expect(view.timelineTitle({ start: bucket.start, label: 'empty', error: 0, warn: 0 })).toBe('empty · 无错误')
    expect(view.timelineHeight(0)).toBe(0)
    expect(view.timelineHeight(view.timeDistribution.value.maxCount)).toBe(64)
  })

  it('keeps first and last timeline labels while thinning the rest', () => {
    const view = createView(SAMPLE)
    const visible = Array.from({ length: 12 }, (_, index) => view.timelineLabelVisible(index, 12))
    // 每 2 格留一个标签，再加末格，共 7 个可见标签。
    expect(visible.filter(Boolean)).toHaveLength(7)
    expect(visible[0] && visible[11]).toBe(true)
  })

  it('windows the visible rows with an overscan of three', () => {
    const many = Array.from({ length: 100 }, (_, index) => match(index + 1, 'line ' + (index + 1)))
    const view = createView(many)
    expect(view.virtualStart.value).toBe(0)
    expect(view.visibleMatches.value).toHaveLength(24)
    expect(view.virtualBottom.value).toBe((100 - 24) * 42)

    view.scrollTop.value = 42 * 10
    expect(view.virtualStart.value).toBe(7)
    expect(view.visibleMatches.value[0].lineNumber).toBe(8)
    expect(view.virtualTop.value).toBe(294)
    expect(view.virtualBottom.value).toBe((100 - 7 - 24) * 42)
  })

  it('scrolls the list back to the top when the view filters change', async () => {
    const view = createView(SAMPLE)
    const element = scrollableElement()
    element.scrollTop = 420
    view.resultListElement.value = element
    view.updateScroll({ target: element } as unknown as Event)
    expect(view.scrollTop.value).toBe(420)

    view.resultFilter.value = 'boom'
    await nextTick()
    expect(view.scrollTop.value).toBe(0)
    expect(element.scrollTop).toBe(0)
  })

  it('moves the selection inside the filtered list and clamps at both ends', async () => {
    const rows = Array.from({ length: 10 }, (_, index) => match(index + 1, 'line ' + (index + 1)))
    const view = createView(rows)
    const element = scrollableElement()
    view.resultListElement.value = element

    // 未选中时先落到首条，再往前仍然停在首条。
    await view.moveMatch(1)
    expect(view.selectedMatch.value?.lineNumber).toBe(1)
    expect(view.selectedIndex.value).toBe(0)
    await view.moveMatch(-1)
    expect(view.selectedMatch.value?.lineNumber).toBe(1)

    view.showMatch(rows[5])
    view.jsonFilter.value = 'trace'
    await view.moveMatch(1)
    expect(view.selectedMatch.value?.lineNumber).toBe(7)
    expect(view.jsonFilter.value).toBe('')
    expect(element.scrollTop).toBe((6 - 3) * 42)

    // 末条之后仍然停在末条。
    view.showMatch(rows[9])
    await view.moveMatch(1)
    expect(view.selectedMatch.value?.lineNumber).toBe(10)
  })

  it('aggregates the summary from the filtered list only', () => {
    const view = createView([
      match(2, 'ERROR one'),
      match(5, 'WARN two'),
      match(9, 'INFO three'),
    ])
    expect(view.resultSummary.value.total).toBe(3)
    view.resultFilter.value = 'ERROR'
    expect(view.resultSummary.value.total).toBe(1)
    expect(view.resultSummary.value.levels[0].level).toBe('error')
  })

  it('resets every view state when a new search starts', () => {
    const view = createView(SAMPLE, { bookmarks: [5] })
    view.resultFilter.value = 'boom'
    view.bookmarkOnly.value = true
    view.resultSort.value = 'desc'
    view.timeFilter.value = { start: 0, stepMs: 1000, label: 'now' }
    view.selectedMatch.value = SAMPLE[0]
    view.jsonFilter.value = 'trace'
    view.scrollTop.value = 200

    view.resetViewState()
    expect(view.resultFilter.value).toBe('')
    expect(view.bookmarkOnly.value).toBe(false)
    expect(view.resultSort.value).toBe('asc')
    expect(view.timeFilter.value).toBeNull()
    expect(view.selectedMatch.value).toBeNull()
    expect(view.jsonFilter.value).toBe('')
    expect(view.scrollTop.value).toBe(0)
  })

  it('ignores the virtual window scroll handler for non-element targets', () => {
    const view = createView(SAMPLE)
    const setter = vi.fn()
    view.updateScroll({ set scrollTop(value: number) { setter(value) }, get scrollTop() { return 0 } } as unknown as Event)
    expect(setter).not.toHaveBeenCalled()
    expect(view.scrollTop.value).toBe(0)
  })
})
