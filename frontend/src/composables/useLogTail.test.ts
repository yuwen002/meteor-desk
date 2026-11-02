import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { MAX_TAIL_LINES, useLogTail } from './useLogTail'

function create(options?: { filePath?: string; encoding?: string }) {
  const filePath = ref(options?.filePath ?? 'C:/logs/app.log')
  const encodingMode = ref(options?.encoding ?? 'utf-8')
  const errorMessage = ref('')
  const tail = useLogTail({ filePath, encodingMode, errorMessage })
  return { filePath, encodingMode, errorMessage, ...tail }
}

function installBridge(readLogTail: ReturnType<typeof vi.fn>) {
  Object.defineProperty(window, 'go', { configurable: true, value: { main: { App: { ReadLogTail: readLogTail } } } })
}

function chunk(prefix: string, count: number) {
  return Array.from({ length: count }, (_, index) => prefix + index)
}

describe('live tail state', () => {
  let readLogTail: ReturnType<typeof vi.fn>

  beforeEach(() => {
    vi.useFakeTimers()
    readLogTail = vi.fn().mockResolvedValue({ lines: ['first'], offset: 7, fileSizeBytes: 7, encoding: 'utf-8', reset: true, truncated: false })
    installBridge(readLogTail)
  })

  afterEach(() => {
    vi.useRealTimers()
    delete (window as Window & { go?: unknown }).go
  })

  it('refuses to start without a selected file', () => {
    const tail = create({ filePath: '' })
    tail.startTail()
    expect(tail.errorMessage.value).toBe('请先选择日志文件。')
    expect(tail.tailEnabled.value).toBe(false)
    expect(readLogTail).not.toHaveBeenCalled()
  })

  it('reads from the start and then only asks for the new bytes', async () => {
    const tail = create()
    tail.startTail()
    await vi.advanceTimersByTimeAsync(1)
    expect(readLogTail).toHaveBeenNthCalledWith(1, 'C:/logs/app.log', -1, 'utf-8')
    expect(tail.tailOffset.value).toBe(7)
    expect(tail.tailLines.value).toEqual(['first'])

    readLogTail.mockResolvedValueOnce({ lines: ['second'], offset: 14, fileSizeBytes: 14, encoding: 'utf-8', reset: false, truncated: false })
    await vi.advanceTimersByTimeAsync(1000)
    expect(readLogTail).toHaveBeenNthCalledWith(2, 'C:/logs/app.log', 7, 'utf-8')
    expect(tail.tailLines.value).toEqual(['first', 'second'])
  })

  it('keeps only the newest lines and honours the reset flag', async () => {
    const tail = create()
    readLogTail.mockResolvedValue({ lines: chunk('a', 600), offset: 1, fileSizeBytes: 1, encoding: 'utf-8', reset: false, truncated: true })
    tail.startTail()
    await vi.advanceTimersByTimeAsync(1)
    expect(tail.tailLines.value).toHaveLength(600)
    expect(tail.tailTruncated.value).toBe(true)

    await vi.advanceTimersByTimeAsync(1000)
    expect(tail.tailLines.value).toHaveLength(MAX_TAIL_LINES)
    expect(tail.tailLines.value[0]).toBe('a200')
    expect(tail.tailLines.value[MAX_TAIL_LINES - 1]).toBe('a599')

    readLogTail.mockResolvedValueOnce({ lines: ['fresh'], offset: 3, fileSizeBytes: 3, encoding: 'gb18030', reset: true, truncated: false })
    await vi.advanceTimersByTimeAsync(1000)
    expect(tail.tailLines.value).toEqual(['fresh'])
    expect(tail.tailEncoding.value).toBe('gb18030')
  })

  it('drops an increment that arrives after tailing stopped', async () => {
    const tail = create()
    let resolveRead: (value: unknown) => void = () => {}
    readLogTail.mockImplementationOnce(() => new Promise(resolve => { resolveRead = resolve }))
    tail.startTail()
    tail.stopTail()
    resolveRead({ lines: ['late'], offset: 9, fileSizeBytes: 9, encoding: 'utf-8', reset: false, truncated: false })
    await vi.advanceTimersByTimeAsync(1)
    expect(tail.tailLines.value).toEqual([])
    expect(tail.tailEnabled.value).toBe(false)
  })

  it('leaves the scroll position alone when follow mode is off', async () => {
    const tail = create()
    const element = document.createElement('div')
    let scrollTop = 7
    Object.defineProperty(element, 'scrollTop', { configurable: true, get: () => scrollTop, set: (value: number) => { scrollTop = value } })
    tail.tailListElement.value = element
    tail.tailFollow.value = false
    tail.startTail()
    await vi.advanceTimersByTimeAsync(1)
    expect(scrollTop).toBe(7)

    tail.tailFollow.value = true
    readLogTail.mockResolvedValueOnce({ lines: ['more'], offset: 20, fileSizeBytes: 20, encoding: 'utf-8', reset: false, truncated: false })
    await vi.advanceTimersByTimeAsync(1000)
    // jsdom 里 scrollHeight 恒为 0，跟随滚动等价于回到顶部。
    expect(scrollTop).toBe(0)
  })

  it('stops polling and reports the failure when a read rejects', async () => {
    const tail = create()
    tail.startTail()
    await vi.advanceTimersByTimeAsync(1)
    readLogTail.mockRejectedValueOnce(new Error('file rotated'))
    await vi.advanceTimersByTimeAsync(1000)
    expect(tail.tailError.value).toContain('file rotated')
    expect(tail.tailEnabled.value).toBe(false)

    const calls = readLogTail.mock.calls.length
    await vi.advanceTimersByTimeAsync(3000)
    expect(readLogTail.mock.calls.length).toBe(calls)
  })

  it('toggles the tail and clears the buffered lines', async () => {
    const tail = create()
    tail.toggleTail()
    await vi.advanceTimersByTimeAsync(1)
    expect(tail.tailEnabled.value).toBe(true)
    expect(tail.tailLines.value).toEqual(['first'])

    tail.clearTail()
    expect(tail.tailLines.value).toEqual([])
    tail.toggleTail()
    expect(tail.tailEnabled.value).toBe(false)

    const calls = readLogTail.mock.calls.length
    await vi.advanceTimersByTimeAsync(3000)
    expect(readLogTail.mock.calls.length).toBe(calls)
  })

  it('does nothing without the desktop bridge', async () => {
    delete (window as Window & { go?: unknown }).go
    const tail = create()
    tail.startTail()
    await vi.advanceTimersByTimeAsync(1)
    expect(tail.tailEnabled.value).toBe(true)
    expect(tail.tailLines.value).toEqual([])

    tail.dispose()
    tail.stopTail()
  })
})
