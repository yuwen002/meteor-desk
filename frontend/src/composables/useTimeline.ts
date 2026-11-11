import { computed, ref } from 'vue'
import { getBridge } from '../utils/bridge'

export type TimelineLevelFilter = 'all' | 'error' | 'warn' | 'info' | 'debug' | 'trace' | 'other'

const LEVEL_ORDER: Record<string, number> = {
  error: 0,
  warn: 1,
  info: 2,
  debug: 3,
  trace: 4,
  other: 5,
}

export function useTimeline() {
  const busy = ref(false)
  const errorMessage = ref('')
  const response = ref<TimelineScanResponse | null>(null)
  const selectedEntry = ref<TimelineEntry | null>(null)
  const levelFilter = ref<TimelineLevelFilter>('all')
  const sourceFilter = ref<string>('all')

  const sourceColors = computed(() => {
    const set = new Set<string>()
    for (const e of response.value?.entries ?? []) {
      if (e.sourceLabel) set.add(e.sourceLabel)
    }
    const palette = [
      '#4f46e5', '#0891b2', '#16a34a', '#ca8a04',
      '#dc2626', '#db2777', '#7c3aed', '#059669',
      '#ea580c', '#9333ea', '#2563eb', '#be123c',
    ]
    const result: Record<string, string> = {}
    let idx = 0
    for (const s of set) {
      result[s] = palette[idx % palette.length]
      idx++
    }
    return result
  })

  const availableSources = computed(() => {
    const sources = new Set<string>()
    for (const f of response.value?.files ?? []) {
      if (f.sourceLabel) sources.add(f.sourceLabel)
    }
    return Array.from(sources).sort()
  })

  const filteredEntries = computed<TimelineEntry[]>(() => {
    const entries = response.value?.entries ?? []
    const lv = levelFilter.value
    const src = sourceFilter.value
    return entries.filter(e => {
      if (lv !== 'all' && e.level !== lv) return false
      if (src !== 'all' && e.sourceLabel !== src) return false
      return true
    })
  })

  const levelCounts = computed(() => {
    const counts: Record<string, number> = {}
    for (const e of response.value?.entries ?? []) {
      counts[e.level] = (counts[e.level] ?? 0) + 1
    }
    return counts
  })

  const filesInfo = computed<TimelineFileResult[]>(() => response.value?.files ?? [])

  async function runScan(files: TimelineFileSpec[], options: TimelineScanOptions = {}) {
    const a = getBridge()
    if (!a) {
      errorMessage.value = '请通过 Wails 桌面应用运行'
      return
    }
    if (!files.length) {
      errorMessage.value = '未指定任何日志文件'
      return
    }
    errorMessage.value = ''
    busy.value = true
    selectedEntry.value = null
    try {
      const resp = await a.ScanLogTimeline(files, options)
      response.value = resp
      if (resp.canceled) {
        errorMessage.value = '扫描已取消'
      } else if (resp.files.some(f => f.failed)) {
        const failed = resp.files.filter(f => f.failed).map(f => `${f.path.split(/[\\/]/).pop()}: ${f.error ?? '未知错误'}`)
        errorMessage.value = `部分文件扫描失败：${failed.join('；')}`
      }
    } catch (e) {
      errorMessage.value = '时间线扫描失败：' + String(e)
    } finally {
      busy.value = false
    }
  }

  function selectEntry(entry: TimelineEntry | null) {
    selectedEntry.value = entry
  }

  function reset() {
    response.value = null
    selectedEntry.value = null
    errorMessage.value = ''
    levelFilter.value = 'all'
    sourceFilter.value = 'all'
  }

  function filterByLevel(level: TimelineLevelFilter) {
    levelFilter.value = level
  }

  function filterBySource(source: string) {
    sourceFilter.value = source
  }

  function levelBadgeClass(level: string): string {
    switch (level) {
      case 'error': return 'tlvl-error'
      case 'warn': return 'tlvl-warn'
      case 'info': return 'tlvl-info'
      case 'debug': return 'tlvl-debug'
      case 'trace': return 'tlvl-trace'
      default: return 'tlvl-other'
    }
  }

  function formatLevel(level: string): string {
    const map: Record<string, string> = {
      error: '错误', warn: '警告', info: '信息',
      debug: '调试', trace: '跟踪', other: '其他',
    }
    return map[level] ?? level
  }

  function sortKeyForLevel(level: string): number {
    return LEVEL_ORDER[level] ?? 99
  }

  return {
    busy, errorMessage, response, selectedEntry,
    levelFilter, sourceFilter,
    sourceColors, availableSources, filteredEntries, levelCounts, filesInfo,
    runScan, selectEntry, reset, filterByLevel, filterBySource,
    levelBadgeClass, formatLevel, sortKeyForLevel,
  }
}