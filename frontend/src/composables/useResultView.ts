import { computed, nextTick, ref, watch, type Ref } from 'vue'
import { bookmarkKey } from '../utils/bookmarks'
import { summarizeMatches } from '../utils/aggregate'
import { buildTimeDistribution, filterMatchesByTime, type TimeBucket } from '../utils/timeDistribution'
import {
  buildFieldFacet,
  buildFieldTrend,
  discoverFields,
  filterMatchesByFacet,
  type TrendBucket,
} from '../utils/fieldStats'

// 结果列表是虚拟滚动的，行高与可视行数一起决定切片范围；改行高要同时改这里。
const ROW_HEIGHT = 42
const VISIBLE_ROWS = 24
const TIMELINE_MAX_HEIGHT = 64
const TREND_MAX_HEIGHT = 64

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
  // 结构化字段分布与趋势（阶段 6 批次 C）：facetPath/trendPath 选择字段，facetFilter 叠加取值过滤。
  const facetPath = ref('')
  const facetFilter = ref<{ path: string; value: string } | null>(null)
  const trendPath = ref('')
  // 多文件检索（阶段 6 批次 E）：文件过滤把结果收窄到某个来源文件，等价于“按文件分组查看”。
  const fileFilter = ref<string | null>(null)

  // matchKey 同时兼顾行号与来源文件：单文件时 filePath 为空，退化为按行号唯一。
  function matchKey(match: LogMatch): string {
    return (match.filePath ?? '') + ':' + match.lineNumber
  }

  // 先按文件收窄（多文件），再按收藏与关键字筛选，最后叠加时间段与 facets；虚拟滚动只消费最终列表。
  const keywordFiltered = computed(() => {
    const keyword = resultFilter.value.trim().toLowerCase()
    let list = options.matches()
    if (fileFilter.value) list = list.filter(m => (m.filePath ?? '') === fileFilter.value)
    if (bookmarkOnly.value) {
      const keys = options.bookmarkKeys.value
      list = list.filter(m => keys.has(bookmarkKey(options.filePath.value, m.lineNumber)))
    }
    if (keyword) list = list.filter(m => String(m.lineNumber).includes(keyword) || m.content.toLowerCase().includes(keyword))
    return list
  })
  const filteredMatches = computed(() => {
    const filter = timeFilter.value
    const list = filter ? filterMatchesByTime(facetFiltered.value, filter.start, filter.stepMs) : facetFiltered.value
    return resultSort.value === 'desc' ? [...list].reverse() : list
  })
  // facets：取值过滤基于关键字过滤后的结果，与时间段过滤叠加。
  const facetFiltered = computed(() => {
    const filter = facetFilter.value
    return filter ? filterMatchesByFacet(keywordFiltered.value, filter.path, filter.value) : keywordFiltered.value
  })
  // 字段发现与统计只读关键字过滤后的结果：选中某取值时分布图仍保留全貌，只是叠加过滤。
  const fieldInfos = computed(() => discoverFields(keywordFiltered.value))
  const trendCandidates = computed(() => fieldInfos.value.filter(field => field.numeric > 0))
  const activeFacet = computed(() => (facetPath.value ? buildFieldFacet(keywordFiltered.value, facetPath.value) : null))
  const activeTrend = computed(() => (trendPath.value ? buildFieldTrend(keywordFiltered.value, trendPath.value) : null))
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

  // 点击某个取值即把结果过滤到该值，再次点击同一取值取消。
  function selectFacetValue(value: string) {
    const current = facetFilter.value
    if (current && current.path === facetPath.value && current.value === value) {
      facetFilter.value = null
      return
    }
    facetFilter.value = { path: facetPath.value, value }
    scrollResultListToTop()
  }
  function facetValueActive(value: string) {
    const current = facetFilter.value
    return !!current && current.path === facetPath.value && current.value === value
  }
  function clearFacet() {
    facetFilter.value = null
  }
  // 点击某个来源文件即把结果收窄到该文件，再次点击同一文件取消。
  function selectFileFilter(path: string) {
    fileFilter.value = fileFilter.value === path ? null : path
    scrollResultListToTop()
  }
  function clearFileFilter() {
    fileFilter.value = null
  }
  // 切换分布字段时，旧的取值过滤不再适用，直接清除。
  watch(facetPath, () => { facetFilter.value = null })

  // 趋势柱按最大桶量级（含负值取绝对值）归一化高度。
  function trendBarHeight(sum: number) {
    const magnitude = activeTrend.value?.maxMagnitude || 1
    return sum ? Math.max(3, Math.round(Math.abs(sum) * TREND_MAX_HEIGHT / magnitude)) : 0
  }
  function trendTitle(bucket: TrendBucket) {
    return bucket.label + ' · 合计 ' + bucket.sum + '（' + bucket.count + ' 行，均值 ' + (Math.round(bucket.avg * 100) / 100) + '）'
  }
  function trendLabelVisible(index: number, total: number) {
    return index % (Math.ceil(total / 6) || 1) === 0 || index === total - 1
  }

  const virtualStart = computed(() => Math.max(0, Math.floor(scrollTop.value / ROW_HEIGHT) - 3))
  const visibleMatches = computed(() => filteredMatches.value.slice(virtualStart.value, virtualStart.value + VISIBLE_ROWS))
  const virtualTop = computed(() => virtualStart.value * ROW_HEIGHT)
  const virtualBottom = computed(() => Math.max(0, (filteredMatches.value.length - virtualStart.value - visibleMatches.value.length) * ROW_HEIGHT))
  const selectedIndex = computed(() => {
    const key = selectedMatch.value ? matchKey(selectedMatch.value) : ''
    return filteredMatches.value.findIndex(m => matchKey(m) === key)
  })

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
    facetPath.value = ''
    facetFilter.value = null
    trendPath.value = ''
    fileFilter.value = null
    scrollResultListToTop()
  }

  return {
    resultFilter, bookmarkOnly, resultSort, timeFilter,
    scrollTop, resultListElement, selectedMatch, jsonFilter,
    keywordFiltered, filteredMatches, resultSummary, timeDistribution,
    facetPath, facetFilter, trendPath, fieldInfos, trendCandidates, activeFacet, activeTrend,
    selectFacetValue, facetValueActive, clearFacet, trendBarHeight, trendTitle, trendLabelVisible,
    fileFilter, selectFileFilter, clearFileFilter, matchKey,
    virtualStart, visibleMatches, virtualTop, virtualBottom, selectedIndex,
    selectTimelineBucket, timelineActive, timelineHeight, timelineTitle, timelineLabelVisible,
    updateScroll, showMatch, moveMatch, resetViewState,
  }
}
