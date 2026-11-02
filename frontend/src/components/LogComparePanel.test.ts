import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import LogComparePanel from './LogComparePanel.vue'

function setBridge(bridge: { SelectLogFile: ReturnType<typeof vi.fn>; ReadLogFileHead: ReturnType<typeof vi.fn> }) {
  Object.defineProperty(window, 'go', { configurable: true, value: { main: { App: bridge } } })
}

function head(text: string, truncated = false) {
  return { text, encoding: 'UTF-8', sizeBytes: text.length, truncated }
}

async function pickBothSides(wrapper: ReturnType<typeof mount>) {
  const buttons = wrapper.findAll('.compare-file button')
  await buttons[0].trigger('click') // 基准
  await buttons[1].trigger('click') // 对比
}

describe('LogComparePanel', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    delete (window as Window & { go?: unknown }).go
  })

  it('renders the line diff and flags one-sided error/warn rows', async () => {
    const select = vi.fn().mockResolvedValueOnce('C:/a.log').mockResolvedValueOnce('C:/b.log')
    const readHead = vi.fn().mockImplementation((path: string) =>
      Promise.resolve(path === 'C:/a.log' ? head('INFO boot\nERROR db down\n') : head('INFO boot\nWARN slow query\n')))
    setBridge({ SelectLogFile: select, ReadLogFileHead: readHead })
    const wrapper = mount(LogComparePanel)

    await pickBothSides(wrapper)
    await wrapper.get('.compare-files .button-primary').trigger('click')
    await vi.waitFor(() => expect(wrapper.find('.compare-summary').exists()).toBe(true))

    expect(select).toHaveBeenCalledTimes(2)
    expect(readHead).toHaveBeenCalledTimes(2)
    const stats = wrapper.get('.compare-stats').text()
    expect(stats).toContain('ERROR 1')
    expect(stats).toContain('WARN 1')
    // ERROR 只在左侧、WARN 只在右侧，各生成一条单侧告警。
    expect(wrapper.findAll('.compare-alert')).toHaveLength(2)
    expect(wrapper.findAll('.compare-row.alert')).toHaveLength(2)
    wrapper.unmount()
  })

  it('reports identical files without any diff rows', async () => {
    const select = vi.fn().mockResolvedValue('C:/x.log')
    const readHead = vi.fn().mockResolvedValue(head('same line\n'))
    setBridge({ SelectLogFile: select, ReadLogFileHead: readHead })
    const wrapper = mount(LogComparePanel)

    await pickBothSides(wrapper)
    await wrapper.get('.compare-files .button-primary').trigger('click')
    await vi.waitFor(() => expect(wrapper.find('.compare-summary').exists()).toBe(true))

    expect(wrapper.find('.compare-empty').text()).toContain('没有差异')
    expect(wrapper.find('.compare-diff').exists()).toBe(false)
    wrapper.unmount()
  })

  it('notes when a side is truncated at the read cap', async () => {
    const select = vi.fn().mockResolvedValueOnce('C:/left.log').mockResolvedValueOnce('C:/right.log')
    const readHead = vi.fn().mockImplementation((path: string) =>
      Promise.resolve(path === 'C:/left.log'
        ? { text: 'ERROR only left\n', encoding: 'UTF-8', sizeBytes: 100, truncated: true }
        : { text: 'other\n', encoding: 'UTF-8', sizeBytes: 6, truncated: false }))
    setBridge({ SelectLogFile: select, ReadLogFileHead: readHead })
    const wrapper = mount(LogComparePanel)

    await pickBothSides(wrapper)
    await wrapper.get('.compare-files .button-primary').trigger('click')
    await vi.waitFor(() => expect(wrapper.find('.compare-note').exists()).toBe(true))
    expect(wrapper.get('.compare-note').text()).toContain('仅对比开头部分')
    wrapper.unmount()
  })

  it('explains that the file picker needs the desktop bridge', async () => {
    const wrapper = mount(LogComparePanel)
    await wrapper.findAll('.compare-file button')[0].trigger('click')
    expect(wrapper.get('.notice-error').text()).toContain('请通过 Wails 桌面应用运行')
    wrapper.unmount()
  })
})
