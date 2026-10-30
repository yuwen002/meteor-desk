import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
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
  beforeEach(() => {
    window.localStorage.clear()
  })

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
    expect(send).toHaveBeenCalledWith({ method: 'GET', url: 'https://example.com/items', headers: { Accept: 'application/json' }, body: '', timeoutMs: 30000, followRedirects: true, insecureSkipVerify: false })
    wrapper.unmount()
  })

  it('sends timeout, redirect and TLS options chosen in the panel', async () => {
    const send = vi.fn().mockResolvedValue({ status: 200, statusText: 'OK', headers: {}, body: '', bodyTruncated: false, sizeBytes: 0, durationMs: 1 })
    const wrapper = await openHTTPPanel({ SendHTTPRequest: send })
    await wrapper.get('input[aria-label="请求 URL"]').setValue('https://localhost:8443/health')
    const [followRedirects, insecureSkipVerify] = wrapper.findAll('.http-options input[type="checkbox"]')
    await followRedirects.setValue(false)
    await insecureSkipVerify.setValue(true)
    await wrapper.get('.http-options select').setValue('60000')
    await wrapper.get('.http-request-line .button-primary').trigger('click')
    await vi.waitFor(() => expect(send).toHaveBeenCalled())
    expect(send).toHaveBeenCalledWith(expect.objectContaining({ timeoutMs: 60000, followRedirects: false, insecureSkipVerify: true }))
    wrapper.unmount()
  })

  it('shows the redirect count, final URL and binary warning', async () => {
    const send = vi.fn().mockResolvedValue({
      status: 200,
      statusText: 'OK',
      headers: { 'Content-Type': 'application/octet-stream' },
      body: '\u0000\u0001',
      bodyTruncated: false,
      sizeBytes: 2,
      durationMs: 8,
      finalUrl: 'https://example.com/final',
      redirects: 2,
      binary: true,
    })
    const wrapper = await openHTTPPanel({ SendHTTPRequest: send })
    await wrapper.get('input[aria-label="请求 URL"]').setValue('https://example.com/start')
    await wrapper.get('.http-request-line .button-primary').trigger('click')
    await vi.waitFor(() => expect(wrapper.find('.http-response-summary').exists()).toBe(true))
    expect(wrapper.get('.http-response-summary').text()).toContain('重定向 2 次')
    expect(wrapper.get('.http-final-url').text()).toBe('https://example.com/final')
    expect(wrapper.text()).toContain('不是有效 UTF-8')
    wrapper.unmount()
  })

  it('records recent requests in localStorage and restores them on click', async () => {
    const send = vi.fn().mockResolvedValue({ status: 200, statusText: 'OK', headers: {}, body: '', bodyTruncated: false, sizeBytes: 0, durationMs: 1 })
    const wrapper = await openHTTPPanel({ SendHTTPRequest: send })
    const urlInput = wrapper.get('input[aria-label="请求 URL"]')
    await urlInput.setValue('https://example.com/history')
    await wrapper.get('.http-request-line .button-primary').trigger('click')
    await vi.waitFor(() => expect(wrapper.findAll('.http-history-item').length).toBe(1))
    expect(window.localStorage.getItem('meteordesk.httpHistory.v1')).toContain('https://example.com/history')

    await urlInput.setValue('https://example.com/other')
    await wrapper.get('.http-history-item').trigger('click')
    expect((urlInput.element as HTMLInputElement).value).toBe('https://example.com/history')

    await wrapper.get('.http-history button').trigger('click')
    expect(wrapper.find('.http-history').exists()).toBe(false)
    expect(window.localStorage.getItem('meteordesk.httpHistory.v1')).toBe('[]')
    wrapper.unmount()
  })
})
