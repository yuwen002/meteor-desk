import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ProcessManagement from '../components/ProcessManagement.vue'
import { getMaskedEnvironmentPreview, isSensitiveEnvKey, parseProcessForm } from './processConfig'

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
})
