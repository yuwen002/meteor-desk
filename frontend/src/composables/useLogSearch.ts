import { computed, ref, type Ref } from 'vue'
import { getBridge } from '../utils/bridge'
import { resolveTimeWindow, type TimeWindowZone } from '../utils/timeWindow'
import type { SearchExportMeta } from '../utils/exporters'

// 进度轮询间隔：与 Wails 事件推送并存，两者共享同一个匹配项游标避免重复追加。
const PROGRESS_POLL_INTERVAL_MS = 250

/** 一次检索提交的参数，全部来自页面上的输入控件。 */
export interface LogSearchInput {
  filePath: Ref<string>
  query: Ref<string>
  useRegex: Ref<boolean>
  contextLines: Ref<number>
  resultLimit: Ref<number>
  encodingMode: Ref<string>
  multiLineJSON: Ref<boolean>
  eventContext: Ref<boolean>
  eventBoundary: Ref<string>
  eventBoundaryPattern: Ref<string>
  // 时间窗（阶段 6 批次 A）：绝对起止、相对快捷项与解释参照。
  timeWindowEnabled: Ref<boolean>
  timeWindowPreset: Ref<string>
  timeWindowStart: Ref<string>
  timeWindowEnd: Ref<string>
  timeWindowZone: Ref<TimeWindowZone>
  // 多文件目录检索（阶段 6 批次 E）：开关、根目录与通配模式；开启时忽略单文件路径。
  multiFile: Ref<boolean>
  rootDir: Ref<string>
  rootPattern: Ref<string>
}

/**
 * 导出元数据来自实际执行的检索参数，避免检索期间修改输入框后导出错误信息。
 */
export function searchMetaFrom(input: { path: string; query: string; useRegex: boolean; encoding: string; contextLines: number; multiLineJSON: boolean; eventContext: boolean }, current: LogSearchResponse): SearchExportMeta {
  return {
    filePath: input.path,
    query: input.query,
    useRegex: input.useRegex,
    encoding: current.encoding || input.encoding,
    contextLines: input.contextLines,
    multiLineJSON: input.multiLineJSON,
    eventContext: input.eventContext,
    resultLimitReached: current.resultLimitReached,
    canceled: current.canceled,
  }
}

/**
 * 流式检索的状态机：任务标识、进度轮询、事件订阅、取消与增量结果合并。
 * 结果集 ref 由页面持有（模板与结果视图都在读它），这里只负责写入；
 * 设置持久化、拖拽等待队列等页面职责由调用方通过回调注入。
 */
export function useLogSearch(options: {
  input: LogSearchInput
  response: Ref<LogSearchResponse | null>
  errorMessage: Ref<string>
  resetViewState: () => void
  onSearchStart: () => void
  recordSearch: (record: SearchRecord) => Promise<void>
  onSearchFinish: () => void
}) {
  const busy = ref(false)
  const cancelRequested = ref(false)
  const searchProgress = ref<LogSearchProgress | null>(null)
  const searchID = ref('')
  const lastSearchMeta = ref<SearchExportMeta | null>(null)
  // 续扫快照（阶段 6 批次 B）：记录上一次因结果上限截断且后端给出续扫点时的字节位置、
  // 已处理行数，以及当时的输入指纹；只有输入未变且非相对时间窗时才允许“从这里继续”。
  const resumeState = ref<{ offset: number; line: number; signature: string } | null>(null)

  function currentSignature() {
    const i = options.input
    return JSON.stringify([i.filePath.value, i.query.value, i.useRegex.value, i.contextLines.value, i.encodingMode.value, i.multiLineJSON.value, i.eventContext.value, i.eventBoundary.value, i.eventBoundaryPattern.value, i.timeWindowEnabled.value, i.timeWindowPreset.value, i.timeWindowStart.value, i.timeWindowEnd.value, i.timeWindowZone.value])
  }
  const canResume = computed(() => {
    const s = resumeState.value
    return !!s && !busy.value && s.offset >= 0 && s.signature === currentSignature()
  })

  let progressTimer: number | undefined
  let partialMatchCursor = 0
  let progressPollInFlight = false
  let completedSearchID = ''
  let stopSearchUpdates: (() => void) | undefined

  function clearProgressTimer() {
    if (progressTimer !== undefined) {
      clearInterval(progressTimer)
      progressTimer = undefined
    }
  }

  // 每轮只追加尚未展示的匹配项，并同步进度元数据；不重复拉取完整结果集。
  async function pollProgress(id: string) {
    const a = getBridge()
    if (!a || progressPollInFlight || completedSearchID === id) return
    progressPollInFlight = true
    try {
      const p = await a.GetLogSearchProgress(id)
      if (searchID.value !== id || completedSearchID === id) return
      searchProgress.value = p
      const streaming = options.response.value
      if (streaming) {
        options.response.value = { ...streaming, fileSizeBytes: p.fileSizeBytes || streaming.fileSizeBytes, bytesScanned: p.bytesScanned, scannedLines: p.scannedLines, canceled: p.canceled }
      }
      const cursor = partialMatchCursor
      const updates = await a.GetLogSearchMatches(id, cursor)
      if (searchID.value !== id || completedSearchID === id || cursor !== partialMatchCursor || !updates.length) return
      partialMatchCursor += updates.length
      const current = options.response.value
      if (current) {
        options.response.value = { ...current, fileSizeBytes: p.fileSizeBytes || current.fileSizeBytes, bytesScanned: p.bytesScanned, scannedLines: p.scannedLines, matches: [...current.matches, ...updates] }
      }
    } catch { /* 刚创建任务时可能尚未登记，下一轮重试 */ }
    finally { progressPollInFlight = false }
  }

  // Wails 事件用于低延迟追加结果；保留轮询作为兼容回退，并共享游标防止重复追加。
  function onSearchUpdate(event: SearchMatchesEvent) {
    if (event.searchId !== searchID.value || completedSearchID === event.searchId) return
    searchProgress.value = event.progress
    const current = options.response.value
    if (!current) return
    const updates = event.matches ?? []
    if (!updates.length) {
      options.response.value = { ...current, fileSizeBytes: event.progress.fileSizeBytes || current.fileSizeBytes, bytesScanned: event.progress.bytesScanned, scannedLines: event.progress.scannedLines, canceled: event.progress.canceled }
      return
    }
    partialMatchCursor += updates.length
    options.response.value = { ...current, fileSizeBytes: event.progress.fileSizeBytes || current.fileSizeBytes, bytesScanned: event.progress.bytesScanned, scannedLines: event.progress.scannedLines, canceled: event.progress.canceled, matches: [...current.matches, ...updates] }
  }

  // 事件订阅需要桌面运行时就绪，由调用方在 onMounted 里 attach、onBeforeUnmount 里 dispose。
  function attachEventUpdates() {
    stopSearchUpdates = window.runtime?.EventsOn('log-search:update', onSearchUpdate)
  }
  function dispose() {
    stopSearchUpdates?.()
    stopSearchUpdates = undefined
    clearProgressTimer()
  }

  async function cancelSearch() {
    const a = getBridge()
    if (!a || !searchID.value) return
    cancelRequested.value = true
    try {
      await a.CancelLogSearch(searchID.value)
    } catch (e) {
      options.errorMessage.value = '取消检索失败：' + String(e)
    }
  }

  async function execute(resume: boolean) {
    options.errorMessage.value = ''
    if (busy.value) return
    const prior = options.response.value
    const snapshot = resumeState.value
    const multiFile = options.input.multiFile.value
    if (resume) {
      // 续扫只适用于单文件；多文件检索不维护续扫点。
      if (multiFile || !prior || !snapshot || snapshot.offset < 0 || snapshot.signature !== currentSignature()) return
    } else {
      resumeState.value = null
    }
    if (multiFile) {
      if (!options.input.rootDir.value) {
        options.errorMessage.value = '请先选择要检索的日志目录。'
        return
      }
    } else if (!options.input.filePath.value) {
      options.errorMessage.value = '请先选择日志文件。'
      return
    }
    if (!options.input.query.value) {
      options.errorMessage.value = '请输入关键字或正则表达式。'
      return
    }
    const a = getBridge()
    if (!a) {
      options.errorMessage.value = '未检测到桌面桥接服务，请通过 wails dev 启动。'
      return
    }
    const eventContextActive = options.input.eventContext.value && !options.input.multiLineJSON.value
    if (eventContextActive && options.input.eventBoundary.value === 'custom' && !options.input.eventBoundaryPattern.value.trim()) {
      options.errorMessage.value = '请输入自定义事件边界正则。'
      return
    }
    // 时间窗在后端扫描时过滤（结果有上限，不能等回传后再筛），输入非法要直接阻止检索。
    const timeWindow = resolveTimeWindow(
      {
        enabled: options.input.timeWindowEnabled.value,
        preset: options.input.timeWindowPreset.value,
        start: options.input.timeWindowStart.value,
        end: options.input.timeWindowEnd.value,
        zone: options.input.timeWindowZone.value,
      },
      Date.now(),
      Intl.DateTimeFormat().resolvedOptions().timeZone,
    )
    if (timeWindow.error) {
      options.errorMessage.value = timeWindow.error
      return
    }
    const targetPath = multiFile ? options.input.rootDir.value : options.input.filePath.value
    const input = {
      path: targetPath,
      query: options.input.query.value,
      useRegex: options.input.useRegex.value,
      resultLimit: options.input.resultLimit.value,
      contextLines: options.input.contextLines.value,
      encoding: options.input.encodingMode.value,
      multiLineJSON: options.input.multiLineJSON.value,
      eventContext: eventContextActive,
      eventBoundary: eventContextActive ? options.input.eventBoundary.value : 'timestamp',
      eventBoundaryPattern: eventContextActive ? options.input.eventBoundaryPattern.value : '',
    }
    // 快照续扫所用的输入指纹，扫描期间即使输入被改动，完成时也以此判定是否仍可续扫。
    const baseSignature = currentSignature()
    clearProgressTimer()
    const id = Date.now() + '-' + Math.random().toString(16).slice(2)
    searchID.value = id
    completedSearchID = ''
    cancelRequested.value = false
    busy.value = true
    partialMatchCursor = 0
    lastSearchMeta.value = searchMetaFrom(input, { filePath: input.path, encoding: '', fileSizeBytes: 0, bytesScanned: 0, scannedLines: 0, matches: [], resultLimitReached: false, canceled: false })
    if (resume && prior) {
      // 追加模式保留既有结果供流式增量拼接，并清除上一轮的截断/续扫标记。
      options.response.value = { ...prior, canceled: false, resultLimitReached: false, resumeOffset: -1 }
    } else {
      options.response.value = { filePath: targetPath, encoding: '', fileSizeBytes: 0, bytesScanned: 0, scannedLines: 0, matches: [], resultLimitReached: false, canceled: false }
    }
    options.onSearchStart()
    if (!resume) options.resetViewState()
    searchProgress.value = { searchId: id, bytesScanned: resume && prior ? prior.bytesScanned : 0, fileSizeBytes: resume && prior ? prior.fileSizeBytes : 0, scannedLines: resume && prior ? prior.scannedLines : 0, matchesFound: resume && prior ? prior.matches.length : 0, percent: 0, running: true, canceled: false }
    progressTimer = window.setInterval(() => void pollProgress(id), PROGRESS_POLL_INTERVAL_MS)
    try {
      const searchOptions: LogSearchOptions = {
        contextLines: input.contextLines,
        encoding: input.encoding,
        multiLineJSON: input.multiLineJSON,
        eventContext: input.eventContext,
        eventBoundary: input.eventBoundary,
        eventBoundaryPattern: input.eventBoundaryPattern,
        timeWindowEnabled: timeWindow.enabled,
        timeWindowStartMs: timeWindow.startMs,
        timeWindowEndMs: timeWindow.endMs,
        timeWindowZone: options.input.timeWindowZone.value,
      }
      if (resume && snapshot) {
        searchOptions.startOffset = snapshot.offset
        searchOptions.startLine = snapshot.line
      }
      // 多文件目录检索与单文件检索共用同一任务登记与轮询机制，只在这里按开关分流到不同桥接方法。
      const completed = multiFile
        ? await a.SearchLogsMultiFile(id, options.input.rootDir.value, options.input.rootPattern.value, input.query, input.useRegex, input.resultLimit, searchOptions)
        : await a.SearchLogsAdvanced(id, input.path, input.query, input.useRegex, input.resultLimit, searchOptions)
      if (resume && prior) {
        options.response.value = { ...completed, matches: [...prior.matches, ...completed.matches], timeWindowSkipped: (prior.timeWindowSkipped ?? 0) + (completed.timeWindowSkipped ?? 0) }
      } else {
        options.response.value = completed
      }
      if (lastSearchMeta.value) {
        lastSearchMeta.value = { ...lastSearchMeta.value, encoding: completed.encoding || lastSearchMeta.value.encoding, resultLimitReached: completed.resultLimitReached, canceled: completed.canceled }
      }
      partialMatchCursor = completed.matches.length
      completedSearchID = id
      let finalPercent = 100
      if (completed.canceled || completed.resultLimitReached) {
        finalPercent = completed.fileSizeBytes > 0 ? Math.min(99, Math.floor(completed.bytesScanned * 100 / completed.fileSizeBytes)) : 0
      }
      searchProgress.value = { searchId: id, bytesScanned: completed.bytesScanned, fileSizeBytes: completed.fileSizeBytes, scannedLines: completed.scannedLines, matchesFound: options.response.value?.matches.length ?? 0, percent: finalPercent, running: false, canceled: completed.canceled }
      await pollProgress(id)
      if (options.response.value?.canceled) {
        options.errorMessage.value = '检索已取消，以下保留已找到的部分结果。'
      } else {
        // 记录续扫可用位置：仅在因上限截断、后端给出续扫点、且未使用会随当前时间漂移的相对时间窗时启用。
        const usesRelativeWindow = options.input.timeWindowEnabled.value && !!options.input.timeWindowPreset.value
        const nextOffset = completed.resumeOffset ?? -1
        resumeState.value = completed.resultLimitReached && nextOffset >= 0 && !usesRelativeWindow
          ? { offset: nextOffset, line: completed.resumeLine ?? 0, signature: baseSignature }
          : null
        if (!resume) {
          await options.recordSearch({ query: input.query, path: input.path, useRegex: input.useRegex, contextLines: input.contextLines, encoding: input.encoding, createdAt: new Date().toISOString() })
        }
      }
    } catch (e) {
      options.errorMessage.value = String(e)
    } finally {
      clearProgressTimer()
      busy.value = false
      cancelRequested.value = false
      options.onSearchFinish()
    }
  }

  // runSearch 启动一次全新检索；resumeSearch 从上次截断处续扫并追加结果。
  // 两者都忽略调用方（表单/快捷键）可能传入的事件对象参数。
  function runSearch() {
    return execute(false)
  }
  function resumeSearch() {
    return execute(true)
  }

  return { busy, cancelRequested, searchProgress, searchID, lastSearchMeta, runSearch, resumeSearch, canResume, cancelSearch, pollProgress, onSearchUpdate, attachEventUpdates, dispose }
}
