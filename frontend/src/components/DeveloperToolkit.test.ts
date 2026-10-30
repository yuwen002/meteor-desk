import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import DeveloperToolkit from './DeveloperToolkit.vue'

interface HTTPBridge { SendHTTPRequest: ReturnType<typeof vi.fn> }

function mountHTTPPanel(bridge: HTTPBridge) {
  Object.defineProperty(window, 'go', { configurable: true, value: { main: { App: bridge } } })
  const wrapper = mount(DeveloperToolkit)
  const tab = wrapper.findAll('.toolkit-tab').find(item => item.text().includes('HTTP 调试'))
  if (!tab) throw new Error('HTTP 调试标签页未渲染')
  return { wrapper, tab }
}

async function openHTTPPanel(bridge: HTTPBridge) {
  const { wrapper, tab } = mountHTTPPanel(bridge)
  await tab.trigger('click')
  return wrapper
}

describe('HTTP debugger panel', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    delete (window as Window & { go?: unknown }).go
  })

  it('rejects malformed header JSON before calling the desktop bridge', async () => {
    const send = vi.fn()
    const wrapper = await openHTTPPanel({ SendHTTPRequest: send })
    await wrapper.get('input[aria-label="请求 URL"]').setValue('https://example.com/health')
    await wrapper.get('#http-headers').setValue('{')
    await wrapper.get('.http-request-line .button-primary').trigger('click')
    expect(wrapper.get('.toolkit-error').text()).toContain('请求头 JSON 无效')
    expect(send).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('requires a URL before sending', async () => {
    const send = vi.fn()
    const wrapper = await openHTTPPanel({ SendHTTPRequest: send })
    await wrapper.get('.http-request-line .button-primary').trigger('click')
    expect(wrapper.get('.toolkit-error').text()).toContain('请输入请求 URL')
    expect(send).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('renders status, timing and received bytes from the backend response', async () => {
    const send = vi.fn().mockResolvedValue({
      status: 201,
      statusText: 'Created',
      headers: { 'Content-Type': 'application/json' },
      body: '{"ok":true}',
      bodyTruncated: false,
      sizeBytes: 11,
      durationMs: 12,
    })
    const wrapper = await openHTTPPanel({ SendHTTPRequest: send })
    await wrapper.get('input[aria-label="请求 URL"]').setValue(' https://example.com/items ')
    await wrapper.get('.http-request-line .button-primary').trigger('click')
    await vi.waitFor(() => expect(wrapper.find('.http-response-summary').exists()).toBe(true))
    const summary = wrapper.get('.http-response-summary').text()
    expect(summary).toContain('201 Created')
    expect(summary).not.toContain('201 201')
    expect(summary).toContain('12 ms')
    expect(summary).toContain('11 B')
    expect(send).toHaveBeenCalledWith({ method: 'GET', url: 'https://example.com/items', headers: { Accept: 'application/json' }, body: '' })
    wrapper.unmount()
  })
})
