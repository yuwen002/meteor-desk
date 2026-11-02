import { computed, nextTick, ref, watch, type Ref } from 'vue'
import { bookmarkKey } from '../utils/bookmarks'
import { summarizeMatches } from '../utils/aggregate'
import { buildTimeDistribution, filterMatchesByTime, type TimeBucket } from '../utils/timeDistribution'

// 结果列表是虚拟滚动的，行高与可视行数一起决定切片范围；改行高要同时改这里。
const ROW_HEIGHT = 42
const VISIBLE_ROWS = 24
const TIMELINE_MAX_HEIGHT = 64

/** 点击时间分布柱状图后叠加的时间段过滤条件。 */
export interface TimeRangeFilter {
  start: number
  stepMs: number
  label: string
}

/**
 * 日志检索结果的视图状态：过滤、排序、收藏筛选、时间段过滤、虚拟滚动与当前选中行。
 * 这些状态只消费后端已返回的结果集，不会重新读文件，因此可以与模板渲染分离后单测。
 */
export function useResultView(options: {
  matches: () => LogMatch[]
  filePath: Ref<string>
  bookmarkKeys: Ref<Set<string>>
}) {
  const resultFilter = ref('')
  const bookmarkOnly = ref(false)
  const resultSort = ref<'asc' | 'desc'>('asc')
  const timeFilter = ref<TimeRangeFilter | null>(null)
  const scrollTop = ref(0)
  const resultListElement = ref<HTMLElement | null>(null)
  const selectedMatch = ref<LogMatch | null>(null)
  const jsonFilter = ref('')

  // 先按收藏与关键字筛选，再按时间段筛选，最后排序；虚拟滚动只消费最终列表。
  const keywordFiltered = computed(() => {
    const keyword = resultFilter.value.trim().toLowerCase()
    let list = options.matches()
    if (bookmarkOnly.value) {
      const keys = options.bookmarkKeys.value
      list = list.filter(m => keys.has(bookmarkKey(options.filePath.value, m.lineNumber)))
    }
    if (keyword) list = list.filter(m => String(m.lineNumber).includes(keyword) || m.content.toLowerCase().includes(keyword))
    return list
  })
  const filteredMatches = computed(() => {
    const filter = timeFilter.value
    const list = filter ? filterMatchesByTime(keywordFiltered.value, filter.start, filter.stepMs) : keywordFiltered.value
    return resultSort.value === 'desc' ? [...list].reverse() : list
  })
  const resultSummary = computed(() => summarizeMatches(filteredMatches.value))
  // 时间分布只读关键字过滤后的结果：选中某个时间段时柱状图仍保留全貌，只是叠加过滤。
  const timeDistribution = computed(() => buildTimeDistribution(keywordFiltered.value))

  // 再次点击同一时间格表示取消该时间段过滤。
  function selectTimelineBucket(bucket: TimeBucket) {
    const current = timeFilter.value
    if (current && current.start === bucket.start && current.stepMs === timeDistribution.value.stepMs) {
      timeFilter.value = null
      return
    }
    timeFilter.value = { start: bucket.start, stepMs: timeDistribution.value.stepMs, label: bucket.label }
    scrollResultListToTop()
  }
  function timelineActive(bucket: TimeBucket) {
    const filter = timeFilter.value
    return !!filter && filter.start === bucket.start && filter.stepMs === timeDistribution.value.stepMs
  }
  function timelineHeight(count: number) {
    return count ? Math.max(3, Math.round(count * TIMELINE_MAX_HEIGHT / (timeDistribution.value.maxCount || 1))) : 0
  }
  function timelineTitle(bucket: TimeBucket) {
    const parts: string[] = []
    if (bucket.error) parts.push('ERROR ' + bucket.error)
    if (bucket.warn) parts.push('WARN ' + bucket.warn)
    return bucket.label + (parts.length ? ' · ' + parts.join(' / ') : ' · 无错误')
  }
  // 时间格多时按比例抽稀标签，保证首尾两端始终可见。
  function timelineLabelVisible(index: number, total: number) {
    return index % (Math.ceil(total / 6) || 1) === 0 || index === total - 1
  }

  const virtualStart = computed(() => Math.max(0, Math.floor(scrollTop.value / ROW_HEIGHT) - 3))
  const visibleMatches = computed(() => filteredMatches.value.slice(virtualStart.value, virtualStart.value + VISIBLE_ROWS))
  const virtualTop = computed(() => virtualStart.value * ROW_HEIGHT)
  const virtualBottom = computed(() => Math.max(0, (filteredMatches.value.length - virtualStart.value - visibleMatches.value.length) * ROW_HEIGHT))
  const selectedIndex = computed(() => filteredMatches.value.findIndex(m => m.lineNumber === selectedMatch.value?.lineNumber))

  function updateScroll(event: Event) {
    if (event.target instanceof HTMLElement) scrollTop.value = event.target.scrollTop
  }
  function scrollResultListToTop() {
    scrollTop.value = 0
    if (resultListElement.value) resultListElement.value.scrollTop = 0
  }
  watch([resultFilter, resultSort, bookmarkOnly], scrollResultListToTop)

  function showMatch(match: LogMatch) {
    selectedMatch.value = match
    jsonFilter.value = ''
  }
  async function moveMatch(delta: number) {
    const all = filteredMatches.value
    if (!all.length) return
    const from = selectedIndex.value
    const target = Math.max(0, Math.min(all.length - 1, from < 0 ? (delta > 0 ? 0 : all.length - 1) : from + delta))
    showMatch(all[target])
    await nextTick()
    if (resultListElement.value) resultListElement.value.scrollTop = Math.max(0, target * ROW_HEIGHT - 3 * ROW_HEIGHT)
  }

  // 换文件、套用历史记录或重新检索时统一复位视图状态，保留后端返回的结果集本身。
  function resetViewState() {
    selectedMatch.value = null
    jsonFilter.value = ''
    resultFilter.value = ''
    bookmarkOnly.value = false
    resultSort.value = 'asc'
    timeFilter.value = null
    scrollResultListToTop()
  }

  return {
    resultFilter, bookmarkOnly, resultSort, timeFilter,
    scrollTop, resultListElement, selectedMatch, jsonFilter,
    keywordFiltered, filteredMatches, resultSummary, timeDistribution,
    virtualStart, visibleMatches, virtualTop, virtualBottom, selectedIndex,
    selectTimelineBucket, timelineActive, timelineHeight, timelineTitle, timelineLabelVisible,
    updateScroll, showMatch, moveMatch, resetViewState,
  }
}
