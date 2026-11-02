import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ProcessManagement from './ProcessManagement.vue'

function processView(id: string, name: string, overrides: Partial<ProcessSpec> = {}, status: Partial<ProcessStatus> = {}): ProcessView {
  return {
    spec: {
      id,
      name,
      executable: name.toLowerCase(),
      args: [],
      workingDir: '',
      env: {},
      autoStart: false,
      dependsOn: [],
      ...overrides,
    },
    status: { id, name, state: 'stopped', ...status },
  }
}

function createBridge(views: ProcessView[] = []) {
  return {
    GetProcesses: vi.fn().mockResolvedValue(views),
    GetProcessLogs: vi.fn().mockResolvedValue([]),
    GetResourceHistory: vi.fn().mockResolvedValue([]),
    GetProcessTimeline: vi.fn().mockResolvedValue([]),
    GetPortDiagnostics: vi.fn().mockResolvedValue({ specId: '', declared: [], ports: [], listening: [], scannedAt: new Date().toISOString() }),
    SaveProcess: vi.fn(async (spec: ProcessSpec): Promise<ProcessView> => ({ spec, status: { id: spec.id, name: spec.name, state: 'stopped' } })),
    DeleteProcess: vi.fn().mockResolvedValue(undefined),
    StartProcess: vi.fn().mockResolvedValue(undefined),
    StopProcess: vi.fn().mockResolvedValue(undefined),
    RestartProcess: vi.fn().mockResolvedValue(undefined),
    StartProcesses: vi.fn().mockResolvedValue([]),
    StopProcesses: vi.fn().mockResolvedValue([]),
    ExportProcessConfigs: vi.fn().mockResolvedValue(''),
    ImportProcessConfigs: vi.fn().mockResolvedValue([]),
    SaveExportFile: vi.fn().mockResolvedValue(''),
    GetProcessRunHistory: vi.fn().mockResolvedValue([]),
    ListProcessGroups: vi.fn().mockResolvedValue([]),
    SaveProcessGroup: vi.fn(async (group: ProcessGroup): Promise<ProcessGroup> => ({ ...group, id: group.id || 'group-1' })),
    DeleteProcessGroup: vi.fn().mockResolvedValue(undefined),
    StartProcessGroup: vi.fn().mockResolvedValue([]),
    StopProcessGroup: vi.fn().mockResolvedValue([]),
  }
}

async function mountPanel(bridge: ReturnType<typeof createBridge>, props: Record<string, unknown> = {}) {
  Object.defineProperty(window, 'go', { configurable: true, value: { main: { App: bridge } } })
  const wrapper = mount(ProcessManagement, { props: { active: true, ...props }, attachTo: document.body })
  await flushPromises()
  return wrapper
}

async function fillRequiredFields(wrapper: Awaited<ReturnType<typeof mountPanel>>, name: string, executable: string) {
  await wrapper.get('input[placeholder="例如：后端 API"]').setValue(name)
  await wrapper.get('input[placeholder="例如：go、npm 或程序绝对路径"]').setValue(executable)
}

describe('process management', () => {
  afterEach(() => {
    delete (window as Window & { go?: unknown }).go
    document.body.innerHTML = ''
  })

  it('lists saved services and selects the first one', async () => {
    const bridge = createBridge([
      processView('api', 'API', {}, { state: 'running', pid: 4321, cpuPercent: 3.46, memoryBytes: 12 * 1024 * 1024, startedAt: new Date().toISOString() }),
      processView('worker', 'Worker'),
    ])
    const wrapper = await mountPanel(bridge)

    const items = wrapper.findAll('.process-item')
    expect(items).toHaveLength(2)
    expect(items[0].classes()).toContain('selected')
    expect(items[0].text()).toContain('内存 12.0 MB')
    expect(items[0].text()).toContain('CPU 3.5%')

    // 首次加载只选中列表项，点击后才把配置填充到表单。
    await items[0].trigger('click')
    await flushPromises()
    expect((wrapper.get('input[placeholder="例如：后端 API"]').element as HTMLInputElement).value).toBe('API')
    expect(wrapper.get('.process-runtime').text()).toContain('内存：12.0 MB')
    wrapper.unmount()
  })

  it('saves health check settings together with the readiness timeout', async () => {
    const bridge = createBridge()
    const wrapper = await mountPanel(bridge)
    await wrapper.findAll('button').find(button => button.text().includes('新建服务'))!.trigger('click')
    await fillRequiredFields(wrapper, '数据库', 'postgres')
    await wrapper.get('textarea.text-area').setValue('["-D","data"]')
    await wrapper.get('.env-toggle').trigger('click')
    await wrapper.findAll('textarea.text-area')[1].setValue('{"PGPORT":"5432"}')

    await wrapper.get('select[aria-label="健康检查类型"]').setValue('tcp')
    await wrapper.get('input[aria-label="健康检查目标"]').setValue('127.0.0.1:5432')
    await wrapper.get('input[aria-label="依赖就绪等待"]').setValue('45')
    await wrapper.get('.process-form').trigger('submit')
    await flushPromises()

    expect(bridge.SaveProcess).toHaveBeenCalledTimes(1)
    expect(bridge.SaveProcess.mock.calls[0][0]).toMatchObject({
      name: '数据库',
      executable: 'postgres',
      args: ['-D', 'data'],
      env: { PGPORT: '5432' },
      dependsOn: [],
      health: { type: 'tcp', target: '127.0.0.1:5432', intervalSeconds: 10, timeoutSeconds: 2, failureThreshold: 3 },
      readyTimeoutSeconds: 45,
    })
    wrapper.unmount()
  })

  it('resets the readiness timeout when health checking is off', async () => {
    const bridge = createBridge()
    const wrapper = await mountPanel(bridge)
    await fillRequiredFields(wrapper, '前端', 'npm')
    await wrapper.get('.process-form').trigger('submit')
    await flushPromises()

    expect(bridge.SaveProcess.mock.calls[0][0]).toMatchObject({ health: null, readyTimeoutSeconds: 0 })
    expect(wrapper.find('input[aria-label="依赖就绪等待"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('offers other services as dependencies and submits the selected ids', async () => {
    const bridge = createBridge([processView('db', '数据库'), processView('api', 'API')])
    const wrapper = await mountPanel(bridge)
    await wrapper.findAll('.process-item')[1].trigger('click')
    await flushPromises()

    const dependencies = wrapper.findAll('.dependency-item')
    expect(dependencies).toHaveLength(1)
    expect(dependencies[0].text()).toContain('数据库')
    await dependencies[0].get('input').setValue(true)
    await wrapper.get('.process-form').trigger('submit')
    await flushPromises()

    expect(bridge.SaveProcess.mock.calls[0][0]).toMatchObject({ id: 'api', dependsOn: ['db'] })
    wrapper.unmount()
  })

  it('starts the selected service through the desktop bridge', async () => {
    const bridge = createBridge([processView('api', 'API')])
    const wrapper = await mountPanel(bridge)
    const startButton = wrapper.findAll('.runtime-actions button').find(button => button.text() === '启动')
    if (!startButton) throw new Error('启动按钮未渲染')
    await startButton.trigger('click')
    await flushPromises()

    expect(bridge.StartProcess).toHaveBeenCalledWith('api')
    expect(bridge.GetProcesses.mock.calls.length).toBeGreaterThan(1)
    wrapper.unmount()
  })

  it('renders the resource history sparkline for the selected service', async () => {
    const bridge = createBridge([
      processView('api', 'API', {}, { state: 'running', pid: 4321, startedAt: new Date().toISOString(), memoryBytes: 16 * 1024 * 1024, metricsAt: '2026-10-31T00:00:02Z' }),
    ])
    bridge.GetResourceHistory.mockResolvedValue([
      { at: '2026-10-31T00:00:00Z', cpuPercent: 4, memoryBytes: 10 * 1024 * 1024 },
      { at: '2026-10-31T00:00:02Z', cpuPercent: 48, memoryBytes: 18 * 1024 * 1024 },
    ])
    const wrapper = await mountPanel(bridge)
    await wrapper.findAll('.process-item')[0].trigger('click')
    await flushPromises()

    expect(bridge.GetResourceHistory).toHaveBeenCalledWith('api')
    expect(wrapper.get('.resource-history-head').text()).toContain('2 个采样点')
    const charts = wrapper.findAll('.resource-chart')
    expect(charts).toHaveLength(2)
    expect(charts[0].text()).toContain('CPU 峰值 48.0%')
    expect(charts[1].text()).toContain('内存峰值 18.0 MB')
    expect(charts[1].text()).toContain('+8.0 MB')
    // 峰值缩放到顶边、最小值贴底边，两个采样点即一条对角线。
    expect(charts[0].get('polyline').attributes('points')).toBe('0.0,32.0 120.0,0.0')
    expect(wrapper.get('.resource-history').text()).toContain('最多保留 150 个点')
    wrapper.unmount()
  })

  it('shows declared port diagnostics with the occupying process', async () => {
    const bridge = createBridge([processView('api', 'API', { ports: [8080, 8081] })])
    bridge.GetPortDiagnostics.mockResolvedValue({
      specId: 'api',
      declared: [8080, 8081],
      ports: [
        { port: 8080, state: 'occupied', pid: 4321, process: 'nginx' },
        { port: 8081, state: 'free' },
      ],
      listening: [],
      scannedAt: '2026-10-31T00:00:05Z',
    })
    const wrapper = await mountPanel(bridge)
    await wrapper.findAll('.process-item')[0].trigger('click')
    await flushPromises()

    expect(bridge.GetPortDiagnostics).toHaveBeenCalledWith('api')
    const panel = wrapper.get('.port-diagnostics')
    expect(panel.text()).toContain('端口诊断')
    expect(panel.text()).toContain('被 PID 4321（nginx）占用')
    expect(panel.text()).toContain('空闲')
    expect(panel.text()).toContain('netstat/lsof/ss')
    wrapper.unmount()
  })

  it('draws the dependency graph in start order', async () => {
    const bridge = createBridge([
      processView('db', '数据库'),
      processView('api', 'API', { dependsOn: ['db'] }, { state: 'running' }),
    ])
    const wrapper = await mountPanel(bridge)

    expect(wrapper.get('.graph-head').text()).toContain('2 个服务 · 2 层')
    const nodes = wrapper.findAll('.graph-node')
    expect(nodes).toHaveLength(2)
    expect(nodes[0].text()).toContain('数据库')
    expect(nodes[1].text()).toContain('API')
    expect(nodes[1].classes()).toContain('running')
    // 被依赖的服务排在前一层，依赖方在其右侧。
    expect(Number(nodes[1].get('rect').attributes('x'))).toBeGreaterThan(Number(nodes[0].get('rect').attributes('x')))
    expect(wrapper.findAll('.graph-edge')).toHaveLength(1)
    expect(wrapper.findAll('.graph-edge.cyclic')).toHaveLength(0)

    await nodes[1].trigger('click')
    await flushPromises()
    expect(nodes[1].classes()).toContain('selected')
    expect((wrapper.get('input[placeholder="例如：后端 API"]').element as HTMLInputElement).value).toBe('API')
    wrapper.unmount()
  })

  it('marks circular dependencies in the graph', async () => {
    const bridge = createBridge([
      processView('a', 'A', { dependsOn: ['b'] }),
      processView('b', 'B', { dependsOn: ['a'] }),
    ])
    const wrapper = await mountPanel(bridge)

    expect(wrapper.get('.graph-head').text()).toContain('检测到循环依赖')
    expect(wrapper.findAll('.graph-node')).toHaveLength(2)
    expect(wrapper.findAll('.graph-edge.cyclic')).toHaveLength(2)
    expect(wrapper.get('.graph-hint').text()).toContain('虚线表示循环依赖')
    wrapper.unmount()
  })

  it('renders the lifecycle timeline with the latest startup duration', async () => {
    const bridge = createBridge([processView('api', 'API', {}, { state: 'running', pid: 4321 })])
    bridge.GetProcessTimeline.mockResolvedValue([
      { at: '2026-10-31T00:00:00Z', kind: 'start', detail: '手动启动' },
      { at: '2026-10-31T00:00:02Z', kind: 'ready', detail: '首次输出', durationMs: 1800 },
      { at: '2026-10-31T00:00:09Z', kind: 'exit', detail: '异常退出（退出码 7）', durationMs: 9000 },
      { at: '2026-10-31T00:00:09Z', kind: 'restart', detail: '第 1 次自动重启，等待 1s' },
      { at: '2026-10-31T00:00:10Z', kind: 'start', detail: '自动重启第 1 次' },
      { at: '2026-10-31T00:00:11Z', kind: 'ready', detail: '首次输出', durationMs: 640 },
    ])
    const wrapper = await mountPanel(bridge)
    await wrapper.findAll('.process-item')[0].trigger('click')
    await flushPromises()

    expect(bridge.GetProcessTimeline).toHaveBeenCalledWith('api')
    const head = wrapper.get('.process-timeline .timeline-head')
    expect(head.text()).toContain('6 条事件')
    expect(head.text()).toContain('最近启动耗时 640 毫秒')
    expect(head.text()).toContain('自动重启 1 次')
    const rows = wrapper.findAll('.event-row')
    expect(rows).toHaveLength(6)
    expect(rows[0].classes()).toContain('start')
    expect(rows[0].text()).toContain('启动')
    expect(rows[1].text()).toContain('就绪')
    expect(rows[1].text()).toContain('耗时 1.8 秒')
    expect(rows[2].classes()).toContain('exit')
    expect(rows[2].text()).toContain('异常退出（退出码 7）')
    expect(rows[2].text()).toContain('运行 9.0 秒')
    expect(wrapper.get('.timeline-hint').text()).toContain('首次输出或健康检查通过')
    wrapper.unmount()
  })

  it('emits open-log with the disk path so the log page can take over', async () => {
    const logPath = 'C:\\Users\\me\\AppData\\logs\\API.log'
    const bridge = createBridge([processView('api', 'API', { logToDisk: true }, { state: 'running', pid: 1, logFilePath: logPath })])
    const wrapper = await mountPanel(bridge)
    await wrapper.findAll('.process-item')[0].trigger('click')
    await flushPromises()

    const panel = wrapper.get('.disk-log-panel')
    expect(panel.text()).toContain('输出落盘')
    expect(panel.get('.disk-log-path').text()).toBe(logPath)
    await panel.get('button').trigger('click')
    expect(wrapper.emitted('open-log')?.[0]).toEqual([logPath])
    wrapper.unmount()
  })

  it('renders the persisted run history newest first with peak metrics', async () => {
    const bridge = createBridge([processView('api', 'API')])
    bridge.GetProcessRunHistory.mockResolvedValue([
      { serviceId: 'api', serviceName: 'API', startedAt: '2026-10-31T00:00:00Z', finishedAt: '2026-10-31T00:01:00Z', durationMs: 60000, exitReason: 'normal', exitCode: 0, peakCpuPercent: 12.5, peakMemoryBytes: 20 * 1024 * 1024 },
      { serviceId: 'api', serviceName: 'API', startedAt: '2026-10-31T01:00:00Z', finishedAt: '2026-10-31T01:00:05Z', durationMs: 5000, exitReason: 'nonzero', exitCode: 2, restarts: 3, peakCpuPercent: 88, peakMemoryBytes: 40 * 1024 * 1024 },
    ])
    const wrapper = await mountPanel(bridge)
    await wrapper.findAll('.process-item')[0].trigger('click')
    await flushPromises()

    expect(bridge.GetProcessRunHistory).toHaveBeenCalledWith('api')
    const head = wrapper.get('.run-history-head')
    expect(head.text()).toContain('共 2 次')
    expect(head.text()).toContain('异常退出 1 次')
    expect(head.text()).toContain('累计自动重启 3 次')
    const rows = wrapper.findAll('.run-row')
    expect(rows).toHaveLength(2)
    // 最新一次排在最前，并标记为异常退出。
    expect(rows[0].classes().length).toBeGreaterThan(0)
    expect(rows[0].text()).toContain('第 2 次')
    expect(rows[0].text()).toContain('非零退出（2）')
    expect(rows[0].text()).toContain('内存 40.0 MB')
    expect(rows[1].text()).toContain('正常退出（0）')
    wrapper.unmount()
  })

  it('lists groups and starts the whole group through the bridge', async () => {
    const bridge = createBridge([processView('db', '数据库'), processView('api', 'API', { dependsOn: ['db'] })])
    bridge.ListProcessGroups.mockResolvedValue([{ id: 'g1', name: '本地联调', description: '一起起', memberIds: ['db', 'api'] }])
    bridge.StartProcessGroup.mockResolvedValue([{ id: 'db', success: true }, { id: 'api', success: true }])
    const wrapper = await mountPanel(bridge)
    await flushPromises()

    const group = wrapper.get('.group-item')
    expect(group.text()).toContain('本地联调')
    expect(group.text()).toContain('数据库')
    expect(group.text()).toContain('API')
    const startButton = wrapper.findAll('.group-item-actions button').find(button => button.text().includes('整组启动'))
    if (!startButton) throw new Error('整组启动按钮未渲染')
    await startButton.trigger('click')
    await flushPromises()
    expect(bridge.StartProcessGroup).toHaveBeenCalledWith('g1')
    wrapper.unmount()
  })

  it('saves a new group from the editor with toggled members', async () => {
    const bridge = createBridge([processView('db', '数据库'), processView('api', 'API')])
    const wrapper = await mountPanel(bridge)
    await fillRequiredFields(wrapper, 'ignore', 'ignore')
    await wrapper.get('input[aria-label="分组名称"]').setValue('联调场景')
    const members = wrapper.findAll('.group-member-item')
    expect(members).toHaveLength(2)
    await members[0].get('input').setValue(true)
    await members[1].get('input').setValue(true)
    const saveGroupButton = wrapper.findAll('.group-editor-actions button').find(button => button.text().includes('新建分组'))
    if (!saveGroupButton) throw new Error('新建分组按钮未渲染')
    await saveGroupButton.trigger('click')
    await flushPromises()

    expect(bridge.SaveProcessGroup).toHaveBeenCalledTimes(1)
    expect(bridge.SaveProcessGroup.mock.calls[0][0]).toMatchObject({ name: '联调场景', memberIds: ['db', 'api'] })
    wrapper.unmount()
  })

  it('selects a service requested via focusId and reports the jump handled', async () => {
    const bridge = createBridge([processView('db', '数据库'), processView('api', 'API')])
    const wrapper = await mountPanel(bridge)
    await wrapper.setProps({ focusId: 'api' })
    await flushPromises()

    expect((wrapper.get('input[placeholder="例如：后端 API"]').element as HTMLInputElement).value).toBe('API')
    expect(wrapper.emitted('focus-handled')).toBeTruthy()
    wrapper.unmount()
  })

  it('stops the occupying managed service when a declared port is taken by it', async () => {
    const bridge = createBridge([processView('api', 'API', { ports: [8080] })])
    bridge.GetPortDiagnostics.mockResolvedValue({
      specId: 'api', declared: [8080],
      ports: [{ port: 8080, state: 'occupied', pid: 999, process: 'node', ownerSpecId: 'db', ownerName: '数据库' }],
      listening: [], scannedAt: '2026-10-31T00:00:05Z',
    })
    const wrapper = await mountPanel(bridge)
    await wrapper.findAll('.process-item')[0].trigger('click')
    await flushPromises()

    const panel = wrapper.get('.port-diagnostics')
    expect(panel.text()).toContain('被本工作台服务“数据库”占用')
    const stopButton = panel.find('.port-stop')
    expect(stopButton.exists()).toBe(true)
    await stopButton.trigger('click')
    await flushPromises()
    expect(bridge.StopProcess).toHaveBeenCalledWith('db')
    wrapper.unmount()
  })
})
