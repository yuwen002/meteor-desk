import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ProcessManagement from '../components/ProcessManagement.vue'
import { getMaskedEnvironmentPreview, isSensitiveEnvKey, parsePortList, parseProcessForm } from './processConfig'

describe('process configuration helpers', () => {
  it('parses JSON args and environment variables without shell concatenation', () => {
    expect(parseProcessForm('["run","dev","--port=8000"]', '{"APP_MODE":"dev"}')).toEqual({
      args: ['run', 'dev', '--port=8000'],
      env: { APP_MODE: 'dev' },
    })
  })

  it('rejects non-string command arguments and environment values', () => {
    expect(() => parseProcessForm('[1,true]', '{}')).toThrow('参数必须是 JSON 字符串数组')
    expect(() => parseProcessForm('[]', '{"PORT":8000}')).toThrow('环境变量必须是 JSON 字符串对象')
    expect(() => parseProcessForm('{', '{}')).toThrow('JSON 格式无效')
  })

  it('masks credential-like keys while keeping ordinary values readable', () => {
    expect(isSensitiveEnvKey('DATABASE_URL')).toBe(true)
    expect(isSensitiveEnvKey('app_mode')).toBe(false)
    const preview = getMaskedEnvironmentPreview(JSON.stringify({
      API_TOKEN: 'secret-value',
      APP_MODE: 'development',
      EMPTY_PASSWORD: '',
    }))
    expect(preview).toContain('"API_TOKEN": "••••••••"')
    expect(preview).toContain('"APP_MODE": "development"')
    expect(preview).toContain('"EMPTY_PASSWORD": ""')
    expect(preview).not.toContain('secret-value')
  })

  it('shows a repair hint for malformed environment JSON', () => {
    expect(getMaskedEnvironmentPreview('{')).toContain('环境变量 JSON 无效')
    expect(getMaskedEnvironmentPreview('[]')).toContain('环境变量 JSON 无效')
  })

  it('parses declared ports with de-duplication and range validation', () => {
    expect(parsePortList('8080, 80，8080')).toEqual([80, 8080])
    expect(parsePortList('')).toEqual([])
    expect(() => parsePortList('8080, abc')).toThrow('1 到 65535')
    expect(() => parsePortList('70000')).toThrow('1 到 65535')
  })
})

describe('process management form interactions', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    delete (window as Window & { go?: unknown }).go
  })

  it('shows validation feedback for invalid args before calling save', async () => {
    const saveProcess = vi.fn()
    const appBridge = {
      GetProcesses: vi.fn().mockResolvedValue([]),
      GetProcessLogs: vi.fn().mockResolvedValue([]),
      SaveProcess: saveProcess,
    }
    Object.defineProperty(window, 'go', { configurable: true, value: { main: { App: appBridge } } })
    const wrapper = mount(ProcessManagement, { props: { active: true }, attachTo: document.body })
    await wrapper.get('input[placeholder="例如：后端 API"]').setValue('Demo service')
    await wrapper.get('input[placeholder="例如：go、npm 或程序绝对路径"]').setValue('go')
    await wrapper.get('.process-form textarea').setValue('[1, "run"]')
    await wrapper.get('.process-form').trigger('submit')
    expect(wrapper.text()).toContain('参数必须是 JSON 字符串数组')
    expect(saveProcess).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('saves restart policy and health check settings', async () => {
    const saveProcess = vi.fn().mockImplementation(spec => Promise.resolve({
      spec,
      status: { id: spec.id || 'service-1', name: spec.name, state: 'stopped' },
    }))
    const appBridge = {
      GetProcesses: vi.fn().mockResolvedValue([]),
      GetProcessLogs: vi.fn().mockResolvedValue([]),
      SaveProcess: saveProcess,
    }
    Object.defineProperty(window, 'go', { configurable: true, value: { main: { App: appBridge } } })
    const wrapper = mount(ProcessManagement, { props: { active: true }, attachTo: document.body })
    await wrapper.get('input[placeholder="例如：后端 API"]').setValue('Demo API')
    await wrapper.get('input[placeholder="例如：go、npm 或程序绝对路径"]').setValue('go')
    await wrapper.get('select[aria-label="自动重启"]').setValue('on-failure')
    await wrapper.get('input[aria-label="自动重启次数"]').setValue('5')
    await wrapper.get('select[aria-label="健康检查类型"]').setValue('http')
    await wrapper.get('input[aria-label="健康检查目标"]').setValue('http://127.0.0.1:8080/health')
    await wrapper.get('.process-form').trigger('submit')
    await flushPromises()

    expect(saveProcess).toHaveBeenCalledTimes(1)
    const spec = saveProcess.mock.calls[0][0]
    expect(spec.restart).toEqual({ mode: 'on-failure', maxAttempts: 5 })
    expect(spec.health).toEqual({
      type: 'http',
      target: 'http://127.0.0.1:8080/health',
      intervalSeconds: 10,
      timeoutSeconds: 2,
      failureThreshold: 3,
    })
    wrapper.unmount()
  })
})
