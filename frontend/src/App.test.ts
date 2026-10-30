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
})
