import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import packageJson from '../package.json'
import App from './App.vue'

function createBridge() {
  return {
    SelectLogFile: vi.fn().mockResolvedValue('C:/logs/app.log'),
    PollDroppedFiles: vi.fn().mockResolvedValue([]),
    SearchLogsAdvanced: vi.fn().mockResolvedValue({
      filePath: 'C:/logs/app.log',
      encoding: 'utf-8',
      fileSizeBytes: 12,
      bytesScanned: 12,
      scannedLines: 3,
      matches: [],
      resultLimitReached: false,
      canceled: false,
    }),
    GetLogSearchProgress: vi.fn().mockResolvedValue({
      searchId: '',
      bytesScanned: 12,
      fileSizeBytes: 12,
      scannedLines: 3,
      matchesFound: 0,
      percent: 100,
      running: false,
      canceled: false,
    }),
    GetLogSearchMatches: vi.fn().mockResolvedValue([]),
    CancelLogSearch: vi.fn().mockResolvedValue(true),
    GetAppSettings: vi.fn().mockResolvedValue({ recentFiles: [], searchHistory: [], contextLines: 2, resultLimit: 500 }),
    SaveAppSettings: vi.fn().mockResolvedValue(undefined),
    GetProcesses: vi.fn().mockResolvedValue([]),
    GetProcessLogs: vi.fn().mockResolvedValue([]),
    GetRuntimeInfo: vi.fn().mockResolvedValue({
      appVersion: '0.5.23',
      goVersion: 'go1.23.5',
      platform: 'windows/amd64',
      dataDir: 'C:/Users/test/AppData/Roaming/MeteorDesk',
    }),
    SaveExportFile: vi.fn().mockResolvedValue('C:/exports/results.csv'),
    ReadLogTail: vi.fn().mockResolvedValue({
      lines: ['tail line'],
      offset: 9,
      fileSizeBytes: 9,
      encoding: 'utf-8',
      reset: true,
      truncated: false,
    }),
  }
}

async function mountApp() {
  const bridge = createBridge()
  Object.defineProperty(window, 'go', { configurable: true, value: { main: { App: bridge } } })
  const wrapper = mount(App, { attachTo: document.body })
  await flushPromises()
  const chooseButton = wrapper.findAll('button').find(button => button.text().includes('选择日志'))
  if (!chooseButton) throw new Error('选择日志按钮未渲染')
  await chooseButton.trigger('click')
  await flushPromises()
  return { wrapper, bridge }
}

function boundarySelect(wrapper: Awaited<ReturnType<typeof mountApp>>['wrapper']) {
  const target = wrapper.findAll('.option-select').find(item => item.text().includes('事件边界'))
  if (!target) throw new Error('事件边界选项未渲染')
  return target.get('select')
}

describe('log search options', () => {
  afterEach(() => {
    delete (window as Window & { go?: unknown }).go
    document.body.innerHTML = ''
    window.localStorage.clear()
  })

  it('passes the custom event boundary to the desktop bridge', async () => {
    const { wrapper, bridge } = await mountApp()
    await wrapper.get('#search-query').setValue('boom')
    await boundarySelect(wrapper).setValue('custom')
    await wrapper.get('.option-input input').setValue('^--- ')
    await wrapper.get('button[type="submit"]').trigger('click')
    await flushPromises()

    expect(bridge.SearchLogsAdvanced).toHaveBeenCalledWith(expect.any(String), 'C:/logs/app.log', 'boom', false, 500, 2, 'auto', false, true, 'custom', '^--- ')
    wrapper.unmount()
  })

  it('rejects an empty custom boundary before calling the bridge', async () => {
    const { wrapper, bridge } = await mountApp()
    await wrapper.get('#search-query').setValue('boom')
    await boundarySelect(wrapper).setValue('custom')
    await wrapper.get('button[type="submit"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('.notice-error').text()).toContain('请输入自定义事件边界正则')
    expect(bridge.SearchLogsAdvanced).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('shows package version and backend build info in the about dialog', async () => {
    const { wrapper, bridge } = await mountApp()
    expect(wrapper.get('.version-tag').text()).toBe('v' + packageJson.version)

    await wrapper.get('.version-tag').trigger('click')
    await flushPromises()

    const dialog = wrapper.get('.about-dialog')
    expect(dialog.text()).toContain('v' + packageJson.version)
    expect(dialog.text()).toContain('go1.23.5')
    expect(dialog.text()).toContain('windows/amd64')
    expect(dialog.text()).toContain('MeteorDesk')
    expect(bridge.GetRuntimeInfo).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('exports the current results with context through the desktop bridge', async () => {
    const { wrapper, bridge } = await mountApp()
    bridge.SearchLogsAdvanced.mockResolvedValue({
      filePath: 'C:/logs/app.log',
      encoding: 'utf-8',
      fileSizeBytes: 12,
      bytesScanned: 12,
      scannedLines: 3,
      resultLimitReached: false,
      canceled: false,
      matches: [{
        lineNumber: 2,
        content: 'boom happened',
        contentClipped: false,
        contextBefore: [{ lineNumber: 1, content: 'before', contentClipped: false }],
        contextAfter: [],
      }],
    })
    await wrapper.get('#search-query').setValue('boom')
    await wrapper.get('button[type="submit"]').trigger('click')
    await flushPromises()

    const csvButton = wrapper.findAll('button').find(button => button.text() === 'CSV')
    if (!csvButton) throw new Error('导出按钮未渲染')
    await csvButton.trigger('click')
    await flushPromises()

    expect(bridge.SaveExportFile).toHaveBeenCalledTimes(1)
    const [name, format, content] = bridge.SaveExportFile.mock.calls[0]
    expect(format).toBe('csv')
    expect(name).toMatch(/^meteordesk-search-app\.log-\d{8}-\d{6}\.csv$/)
    expect(content).toContain('context-before,1,,boom,before')
    expect(content).toContain('boom happened')
    expect(wrapper.get('.notice-success').text()).toContain('已导出 1 条检索结果')
    wrapper.unmount()
  })

  it('streams appended lines while live tail is enabled', async () => {
    const { wrapper, bridge } = await mountApp()
    const startButton = wrapper.findAll('button').find(button => button.text().includes('开始跟踪'))
    if (!startButton) throw new Error('跟踪按钮未渲染')
    await startButton.trigger('click')
    await flushPromises()

    expect(bridge.ReadLogTail).toHaveBeenCalledWith('C:/logs/app.log', -1, 'auto')
    expect(wrapper.get('.tail-output').text()).toContain('tail line')

    const stopButton = wrapper.findAll('button').find(button => button.text().includes('停止跟踪'))
    if (!stopButton) throw new Error('停止跟踪按钮未渲染')
    await stopButton.trigger('click')
    await flushPromises()

    const calls = bridge.ReadLogTail.mock.calls.length
    await new Promise(resolve => setTimeout(resolve, 1100))
    await flushPromises()
    expect(bridge.ReadLogTail.mock.calls.length).toBe(calls)
    wrapper.unmount()
  })

  it('filters, sorts and highlights the current results', async () => {
    const { wrapper, bridge } = await mountApp()
    bridge.SearchLogsAdvanced.mockResolvedValue({
      filePath: 'C:/logs/app.log',
      encoding: 'utf-8',
      fileSizeBytes: 120,
      bytesScanned: 120,
      scannedLines: 12,
      resultLimitReached: false,
      canceled: false,
      matches: [
        { lineNumber: 2, content: 'boom happened', contentClipped: false, contextBefore: [], contextAfter: [] },
        { lineNumber: 5, content: 'timeout waiting', contentClipped: false, contextBefore: [], contextAfter: [] },
        { lineNumber: 9, content: 'boom retry', contentClipped: false, contextBefore: [], contextAfter: [] },
      ],
    })
    await wrapper.get('#search-query').setValue('boom')
    await wrapper.get('button[type="submit"]').trigger('click')
    await flushPromises()

    expect(wrapper.findAll('.result-row')).toHaveLength(3)
    expect(wrapper.findAll('.result-row mark').map(mark => mark.text())).toEqual(['boom', 'boom'])
    expect(wrapper.get('.filter-summary').text()).toBe('3 / 3 条')

    await wrapper.get('.result-filter input').setValue('timeout')
    await flushPromises()
    expect(wrapper.findAll('.result-row')).toHaveLength(1)
    expect(wrapper.get('.result-row').text()).toContain('timeout waiting')
    expect(wrapper.get('.filter-summary').text()).toBe('1 / 3 条')

    await wrapper.get('.result-filter input').setValue('')
    await flushPromises()
    await wrapper.get('.sort-button').trigger('click')
    await flushPromises()
    expect(wrapper.get('.sort-button').text()).toContain('行号降序')
    expect(wrapper.findAll('.result-row .line-number').map(item => item.text())).toEqual(['00009', '00005', '00002'])
    wrapper.unmount()
  })

  it('stores bookmarks locally and can show only bookmarked results', async () => {
    const { wrapper, bridge } = await mountApp()
    bridge.SearchLogsAdvanced.mockResolvedValue({
      filePath: 'C:/logs/app.log',
      encoding: 'utf-8',
      fileSizeBytes: 120,
      bytesScanned: 120,
      scannedLines: 12,
      resultLimitReached: false,
      canceled: false,
      matches: [
        { lineNumber: 2, content: 'boom happened', contentClipped: false, contextBefore: [], contextAfter: [] },
        { lineNumber: 5, content: 'timeout waiting', contentClipped: false, contextBefore: [], contextAfter: [] },
      ],
    })
    await wrapper.get('#search-query').setValue('boom')
    await wrapper.get('button[type="submit"]').trigger('click')
    await flushPromises()

    const bookmarkButtons = wrapper.findAll('.bookmark-button')
    expect(bookmarkButtons).toHaveLength(2)
    await bookmarkButtons[1].trigger('click')
    await flushPromises()
    const stored = JSON.parse(window.localStorage.getItem('meteordesk.bookmarks.v1') ?? '[]')
    expect(stored).toHaveLength(1)
    expect(stored[0]).toMatchObject({ path: 'C:/logs/app.log', lineNumber: 5, query: 'boom' })
    expect(bookmarkButtons[1].classes()).toContain('active')

    await wrapper.get('.toolbar-toggle input').setValue(true)
    await flushPromises()
    expect(wrapper.findAll('.result-row')).toHaveLength(1)
    expect(wrapper.get('.result-row').text()).toContain('timeout waiting')
    wrapper.unmount()
  })

  it('aggregates levels and error signatures from the current results', async () => {
    const clipboard = { writeText: vi.fn().mockResolvedValue(undefined) }
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: clipboard })
    const { wrapper, bridge } = await mountApp()
    bridge.SearchLogsAdvanced.mockResolvedValue({
      filePath: 'C:/logs/app.log',
      encoding: 'utf-8',
      fileSizeBytes: 120,
      bytesScanned: 120,
      scannedLines: 12,
      resultLimitReached: false,
      canceled: false,
      matches: [
        { lineNumber: 2, content: '2026-10-30 10:00:00 ERROR connection refused', contentClipped: false, contextBefore: [], contextAfter: [] },
        { lineNumber: 5, content: '2026-10-30 10:00:01 WARN slow request 3000ms', contentClipped: false, contextBefore: [], contextAfter: [] },
        { lineNumber: 9, content: '2026-10-30 10:00:02 ERROR connection refused', contentClipped: false, contextBefore: [], contextAfter: [] },
        { lineNumber: 12, content: '2026-10-30 10:00:03 INFO ready', contentClipped: false, contextBefore: [], contextAfter: [] },
      ],
    })
    await wrapper.get('#search-query').setValue('boom')
    await wrapper.get('button[type="submit"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('.summary-panel').exists()).toBe(false)
    const toggle = wrapper.findAll('button').find(button => button.text() === '统计')
    if (!toggle) throw new Error('统计按钮未渲染')
    await toggle.trigger('click')
    await flushPromises()

    expect(wrapper.get('.summary-head').text()).toContain('共 4 条，行号 2–12')
    expect(wrapper.get('.level-chip.level-error').text()).toContain('ERROR')
    expect(wrapper.get('.level-chip.level-error').text()).toContain('2')
    expect(wrapper.get('.level-chip.level-info').text()).toContain('INFO')
    const signatures = wrapper.findAll('.summary-signatures li')
    expect(signatures).toHaveLength(2)
    expect(signatures[0].text()).toContain('connection refused')
    expect(signatures[0].text()).toContain('2 次')
    // 时间分布只统计 error/warn，10:00:03 的 INFO 行不占时间格。
    const timeline = wrapper.get('.summary-timeline')
    expect(timeline.get('.summary-subtitle').text()).toContain('每格 1 秒')
    expect(timeline.get('.summary-subtitle').text()).toContain('ERROR 2')
    expect(timeline.get('.summary-subtitle').text()).toContain('WARN 1')
    const timelineBuckets = timeline.findAll('.timeline-bucket')
    expect(timelineBuckets).toHaveLength(3)
    expect(timelineBuckets[0].get('.timeline-bar').attributes('title')).toBe('10-30 10:00:00 · ERROR 1')
    expect(timelineBuckets[1].get('.timeline-bar').attributes('title')).toBe('10-30 10:00:01 · WARN 1')
    expect(timelineBuckets[0].get('.timeline-bar').attributes('style')).toContain('height: 64px')
    expect(timelineBuckets[0].find('.timeline-error').exists()).toBe(true)
    expect(timelineBuckets[0].find('.timeline-warn').exists()).toBe(false)
    expect(timelineBuckets[1].find('.timeline-warn').exists()).toBe(true)
    expect(timelineBuckets[1].find('.timeline-error').exists()).toBe(false)
    expect(timelineBuckets[0].get('.timeline-label').text()).toBe('10-30 10:00:00')

    const copyButton = wrapper.findAll('button').find(button => button.text().includes('复制摘要'))
    if (!copyButton) throw new Error('复制摘要按钮未渲染')
    await copyButton.trigger('click')
    await flushPromises()
    expect(clipboard.writeText).toHaveBeenCalledWith(expect.stringContaining('检索结果摘要\n总计：4 条（行号 2–12）'))
    expect(wrapper.get('.summary-note').text()).toContain('摘要已复制到剪贴板')
    wrapper.unmount()
  })

  it('flags error lines without timestamps in the time distribution', async () => {
    const { wrapper, bridge } = await mountApp()
    bridge.SearchLogsAdvanced.mockResolvedValue({
      filePath: 'C:/logs/app.log',
      encoding: 'utf-8',
      fileSizeBytes: 120,
      bytesScanned: 120,
      scannedLines: 5,
      resultLimitReached: false,
      canceled: false,
      matches: [
        { lineNumber: 2, content: '2026-10-30 10:00:00 ERROR connection refused', contentClipped: false, contextBefore: [], contextAfter: [] },
        { lineNumber: 4, content: 'ERROR bare message without timestamp', contentClipped: false, contextBefore: [], contextAfter: [] },
      ],
    })
    await wrapper.get('#search-query').setValue('ERROR')
    await wrapper.get('button[type="submit"]').trigger('click')
    await flushPromises()
    const toggle = wrapper.findAll('button').find(button => button.text() === '统计')
    if (!toggle) throw new Error('统计按钮未渲染')
    await toggle.trigger('click')
    await flushPromises()

    expect(wrapper.findAll('.timeline-bucket')).toHaveLength(1)
    expect(wrapper.get('.summary-panel').text()).toContain('1 条错误/警告行缺少可识别的时间戳')
    wrapper.unmount()
  })
})
