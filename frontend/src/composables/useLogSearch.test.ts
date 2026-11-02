import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { searchMetaFrom, useLogSearch } from './useLogSearch'

type Bridge = {
  SearchLogsAdvanced: ReturnType<typeof vi.fn>
  SearchLogsMultiFile: ReturnType<typeof vi.fn>
  GetLogSearchProgress: ReturnType<typeof vi.fn>
  GetLogSearchMatches: ReturnType<typeof vi.fn>
  CancelLogSearch: ReturnType<typeof vi.fn>
}

function match(lineNumber: number, content: string): LogMatch {
  return { lineNumber, content, contentClipped: false, contextBefore: [], contextAfter: [] }
}

function progress(searchId: string, over: Partial<LogSearchProgress> = {}): LogSearchProgress {
  return { searchId, bytesScanned: 30, fileSizeBytes: 30, scannedLines: 3, matchesFound: 0, percent: 100, running: false, canceled: false, ...over }
}

function result(over: Partial<LogSearchResponse> = {}): LogSearchResponse {
  return { filePath: 'C:/logs/app.log', encoding: 'utf-8', fileSizeBytes: 30, bytesScanned: 30, scannedLines: 3, matches: [], resultLimitReached: false, canceled: false, ...over }
}

let bridge: Bridge
function installBridge() {
  bridge = {
    SearchLogsAdvanced: vi.fn().mockResolvedValue(result()),
    SearchLogsMultiFile: vi.fn().mockResolvedValue(result({ filePath: 'C:/logs', encoding: '多文件' })),
    GetLogSearchProgress: vi.fn().mockResolvedValue(progress('any')),
    GetLogSearchMatches: vi.fn().mockResolvedValue([]),
    CancelLogSearch: vi.fn().mockResolvedValue(true),
  }
  Object.defineProperty(window, 'go', { configurable: true, value: { main: { App: bridge } } })
  return bridge
}

function create(options: { filePath?: string; query?: string } = {}) {
  const input = {
    filePath: ref(options.filePath === undefined ? 'C:/logs/app.log' : options.filePath),
    query: ref(options.query === undefined ? 'boom' : options.query),
    useRegex: ref(false),
    contextLines: ref(2),
    resultLimit: ref(500),
    encodingMode: ref('auto'),
    multiLineJSON: ref(false),
    eventContext: ref(true),
    eventBoundary: ref('timestamp'),
    eventBoundaryPattern: ref(''),
    timeWindowEnabled: ref(false),
    timeWindowPreset: ref(''),
    timeWindowStart: ref(''),
    timeWindowEnd: ref(''),
    timeWindowZone: ref<'local' | 'utc'>('local'),
    multiFile: ref(false),
    rootDir: ref('C:/logs'),
    rootPattern: ref('*.log;*.txt'),
  }
  const response = ref<LogSearchResponse | null>(null)
  const errorMessage = ref('')
  const hooks = {
    resetViewState: vi.fn(),
    onSearchStart: vi.fn(),
    recordSearch: vi.fn().mockResolvedValue(undefined),
    onSearchFinish: vi.fn(),
  }
  const search = useLogSearch({
    input,
    response,
    errorMessage,
    resetViewState: hooks.resetViewState,
    onSearchStart: hooks.onSearchStart,
    recordSearch: hooks.recordSearch,
    onSearchFinish: hooks.onSearchFinish,
  })
  return { input, response, errorMessage, hooks, ...search }
}

describe('log search state machine', () => {
  afterEach(() => {
    delete (window as Window & { go?: unknown }).go
    delete (window as Window & { runtime?: unknown }).runtime
  })

  it('refuses to scan without a file, a query or the desktop bridge', async () => {
    installBridge()
    const missingFile = create({ filePath: '' })
    await missingFile.runSearch()
    expect(missingFile.errorMessage.value).toBe('请先选择日志文件。')
    expect(bridge.SearchLogsAdvanced).not.toHaveBeenCalled()

    const missingQuery = create({ query: '' })
    await missingQuery.runSearch()
    expect(missingQuery.errorMessage.value).toBe('请输入关键字或正则表达式。')
    expect(bridge.SearchLogsAdvanced).not.toHaveBeenCalled()

    delete (window as Window & { go?: unknown }).go
    const noBridge = create()
    await noBridge.runSearch()
    expect(noBridge.errorMessage.value).toBe('未检测到桌面桥接服务，请通过 wails dev 启动。')

    installBridge()
    const emptyBoundary = create()
    emptyBoundary.input.eventBoundary.value = 'custom'
    await emptyBoundary.runSearch()
    expect(emptyBoundary.errorMessage.value).toBe('请输入自定义事件边界正则。')
    expect(bridge.SearchLogsAdvanced).not.toHaveBeenCalled()
  })

  it('passes the effective options and keeps the exported metadata in sync', async () => {
    installBridge()
    const search = create()
    search.input.useRegex.value = true
    search.input.multiLineJSON.value = true
    await search.runSearch()

    const [id, path, query, useRegex, resultLimit, options] = bridge.SearchLogsAdvanced.mock.calls[0]
    expect([path, query, useRegex, resultLimit]).toEqual(['C:/logs/app.log', 'boom', true, 500])
    // 上下文、编码等改由选项结构体传递；多行 JSON 文档模式会强制关闭事件上下文。
    expect([options.contextLines, options.encoding, options.multiLineJSON, options.eventContext, options.eventBoundary, options.eventBoundaryPattern]).toEqual([2, 'auto', true, false, 'timestamp', ''])
    expect(search.lastSearchMeta.value).toMatchObject({ filePath: path, query, useRegex: true, encoding: 'utf-8', eventContext: false, resultLimitReached: false, canceled: false })
    expect(search.searchID.value).toBe(id)
    expect(search.hooks.resetViewState).toHaveBeenCalledOnce()
    expect(search.hooks.onSearchStart).toHaveBeenCalledOnce()
    expect(search.hooks.recordSearch).toHaveBeenCalledWith(expect.objectContaining({ query: 'boom', path: 'C:/logs/app.log' }))
    expect(search.hooks.onSearchFinish).toHaveBeenCalledOnce()
    expect(search.busy.value).toBe(false)
    expect(search.searchProgress.value).toMatchObject({ percent: 100, running: false, matchesFound: 0 })
  })

  it('routes a multi-file scan to the directory bridge and labels the source', async () => {
    installBridge()
    const search = create()
    search.input.multiFile.value = true
    await search.runSearch()

    expect(bridge.SearchLogsAdvanced).not.toHaveBeenCalled()
    expect(bridge.SearchLogsMultiFile).toHaveBeenCalledOnce()
    const [id, rootDir, pattern, query] = bridge.SearchLogsMultiFile.mock.calls[0]
    expect([rootDir, pattern, query]).toEqual(['C:/logs', '*.log;*.txt', 'boom'])
    expect(search.searchID.value).toBe(id)
    // 目录检索的“已打开位置”承载根目录，且回填文件级明细供总览面板使用。
    expect(search.response.value?.filePath).toBe('C:/logs')
    expect(search.response.value?.encoding).toBe('多文件')
    expect(search.hooks.recordSearch).toHaveBeenCalledWith(expect.objectContaining({ path: 'C:/logs' }))
  })

  it('requires a directory before a multi-file scan and never offers resume', async () => {
    installBridge()
    const search = create()
    search.input.multiFile.value = true
    search.input.rootDir.value = ''
    await search.runSearch()
    expect(search.errorMessage.value).toBe('请先选择要检索的日志目录。')
    expect(bridge.SearchLogsMultiFile).not.toHaveBeenCalled()
  })

  it('resolves the time window and forwards it to the backend options', async () => {
    installBridge()
    const search = create()
    search.input.timeWindowEnabled.value = true
    search.input.timeWindowPreset.value = '30m'
    await search.runSearch()

    const options = bridge.SearchLogsAdvanced.mock.calls[0][5]
    expect(options.timeWindowEnabled).toBe(true)
    expect(options.timeWindowZone).toBe('local')
    expect(options.timeWindowEndMs - options.timeWindowStartMs).toBe(30 * 60_000)
  })

  it('blocks the search when the time window input is invalid', async () => {
    installBridge()
    const search = create()
    search.input.timeWindowEnabled.value = true
    search.input.timeWindowStart.value = '2026-13-45T99:99'
    await search.runSearch()
    expect(search.errorMessage.value).toBeTruthy()
    expect(bridge.SearchLogsAdvanced).not.toHaveBeenCalled()
  })

  it('keeps partial results when the scan is cancelled or hits the limit', async () => {
    installBridge()
    bridge.SearchLogsAdvanced.mockResolvedValue(result({ canceled: true, bytesScanned: 10, fileSizeBytes: 100, matches: [match(2, 'boom')] }))
    const cancelled = create()
    await cancelled.runSearch()
    expect(cancelled.errorMessage.value).toContain('检索已取消')
    expect(cancelled.hooks.recordSearch).not.toHaveBeenCalled()
    expect(cancelled.searchProgress.value?.percent).toBe(10)

    installBridge()
    bridge.SearchLogsAdvanced.mockResolvedValue(result({ resultLimitReached: true, bytesScanned: 99, fileSizeBytes: 100 }))
    const limited = create()
    await limited.runSearch()
    expect(limited.searchProgress.value?.percent).toBe(99)
    expect(limited.lastSearchMeta.value?.resultLimitReached).toBe(true)
    expect(limited.errorMessage.value).toBe('')
  })

  it('continues from the truncation offset and appends without dropping prior results', async () => {
    installBridge()
    bridge.SearchLogsAdvanced.mockResolvedValueOnce(result({
      resultLimitReached: true, resumeOffset: 4096, resumeLine: 100,
      bytesScanned: 4096, fileSizeBytes: 10000, scannedLines: 100,
      matches: [match(2, 'boom one'), match(5, 'boom two')],
    }))
    const search = create()
    await search.runSearch()
    expect(search.canResume.value).toBe(true)

    bridge.SearchLogsAdvanced.mockResolvedValueOnce(result({
      resultLimitReached: false, resumeOffset: -1,
      bytesScanned: 10000, fileSizeBytes: 10000, scannedLines: 200,
      matches: [match(120, 'boom three')],
    }))
    await search.resumeSearch()

    const resumeOptions = bridge.SearchLogsAdvanced.mock.calls[1][5]
    expect(resumeOptions).toMatchObject({ startOffset: 4096, startLine: 100 })
    expect(search.response.value?.matches.map(item => item.lineNumber)).toEqual([2, 5, 120])
    // 续扫页扫描到末尾不再截断，续扫入口应自动隐藏。
    expect(search.canResume.value).toBe(false)
    // 历史只在首次检索时记录，续扫不应重复写入。
    expect(search.hooks.recordSearch).toHaveBeenCalledOnce()
  })

  it('hides the resume option once the search inputs change', async () => {
    installBridge()
    bridge.SearchLogsAdvanced.mockResolvedValueOnce(result({
      resultLimitReached: true, resumeOffset: 4096, resumeLine: 100, matches: [match(2, 'boom one')],
    }))
    const search = create()
    await search.runSearch()
    expect(search.canResume.value).toBe(true)
    search.input.query.value = 'other'
    expect(search.canResume.value).toBe(false)
  })

  it('does not offer resume when a relative time window is active', async () => {
    installBridge()
    bridge.SearchLogsAdvanced.mockResolvedValueOnce(result({
      resultLimitReached: true, resumeOffset: 4096, resumeLine: 100, matches: [match(2, 'boom one')],
    }))
    const search = create()
    search.input.timeWindowEnabled.value = true
    search.input.timeWindowPreset.value = '30m'
    await search.runSearch()
    expect(search.canResume.value).toBe(false)
  })

  it('surfaces bridge failures and still releases the busy flag', async () => {
    installBridge()
    bridge.SearchLogsAdvanced.mockRejectedValue(new Error('磁盘不可读'))
    const search = create()
    await search.runSearch()
    expect(search.errorMessage.value).toBe('Error: 磁盘不可读')
    expect(search.busy.value).toBe(false)
    expect(search.hooks.onSearchFinish).toHaveBeenCalledOnce()
  })

  it('shares the match cursor between event pushes and progress polling', async () => {
    installBridge()
    const search = create()
    let finish: (value: LogSearchResponse) => void = () => {}
    bridge.SearchLogsAdvanced.mockImplementation(() => new Promise<LogSearchResponse>(resolve => { finish = resolve }))

    const running = search.runSearch()
    const id = search.searchID.value
    expect(search.busy.value).toBe(true)
    expect(search.response.value?.matches).toEqual([])

    search.onSearchUpdate({ searchId: id, progress: progress(id, { matchesFound: 2 }), matches: [match(2, 'boom one'), match(3, 'boom two')] })
    expect(search.response.value?.matches).toHaveLength(2)

    bridge.GetLogSearchMatches.mockResolvedValue([match(9, 'boom three')])
    await search.pollProgress(id)
    expect(bridge.GetLogSearchMatches).toHaveBeenCalledWith(id, 2)
    expect(search.response.value?.matches.map(item => item.lineNumber)).toEqual([2, 3, 9])

    // 属于其他任务的事件不能污染当前结果集。
    search.onSearchUpdate({ searchId: 'stale-task', progress: progress('stale-task'), matches: [match(99, 'late')] })
    expect(search.response.value?.matches).toHaveLength(3)

    finish(result({ matches: [match(2, 'boom one'), match(3, 'boom two'), match(9, 'boom three')] }))
    await running
    expect(search.response.value?.matches).toHaveLength(3)
    // 任务完成后迟到的增量一律丢弃。
    search.onSearchUpdate({ searchId: id, progress: progress(id), matches: [match(100, 'after done')] })
    expect(search.response.value?.matches).toHaveLength(3)
  })

  it('cancels the running task through the bridge', async () => {
    installBridge()
    const search = create()
    bridge.SearchLogsAdvanced.mockImplementation(() => new Promise<LogSearchResponse>(() => {}))
    void search.runSearch()
    await search.cancelSearch()
    expect(bridge.CancelLogSearch).toHaveBeenCalledWith(search.searchID.value)
    expect(search.cancelRequested.value).toBe(true)

    const idle = create()
    await idle.cancelSearch()
    expect(bridge.CancelLogSearch).toHaveBeenCalledOnce()
    search.dispose()
  })

  it('subscribes to desktop events and unsubscribes on dispose', async () => {
    installBridge()
    const unsubscribe = vi.fn()
    const eventsOn = vi.fn().mockReturnValue(unsubscribe)
    Object.defineProperty(window, 'runtime', { configurable: true, value: { EventsOn: eventsOn } })
    const search = create()
    search.attachEventUpdates()
    expect(eventsOn).toHaveBeenCalledWith('log-search:update', expect.any(Function))

    // 事件回调在任务登记之前到达也只是被忽略，不会改写结果集。
    search.response.value = result({ matches: [] })
    eventsOn.mock.calls[0][1]({ searchId: search.searchID.value, progress: progress('unknown') })
    expect(search.response.value?.matches).toEqual([])

    search.dispose()
    expect(unsubscribe).toHaveBeenCalledOnce()
  })

  it('derives the export metadata from the response encoding with a fallback to the request', () => {
    const input = { path: 'C:/logs/app.log', query: 'boom', useRegex: false, encoding: 'auto', contextLines: 2, multiLineJSON: false, eventContext: true }
    expect(searchMetaFrom(input, result({ encoding: 'gb18030' })).encoding).toBe('gb18030')
    expect(searchMetaFrom(input, result({ encoding: '' })).encoding).toBe('auto')
    expect(searchMetaFrom(input, result({ resultLimitReached: true, canceled: true }))).toMatchObject({ resultLimitReached: true, canceled: true, contextLines: 2 })
  })
})
