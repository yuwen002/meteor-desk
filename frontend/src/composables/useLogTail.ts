import { nextTick, ref, type Ref } from 'vue'
import { getBridge } from '../utils/bridge'

// 实时跟踪只在内存里保留最近 1,000 行，定时读取新增内容而不重新扫描整份文件。
export const MAX_TAIL_LINES = 1000
const TAIL_POLL_INTERVAL_MS = 1000

/**
 * 日志实时跟踪状态：增量读取、跟随滚动、轮询定时器与失败提示。
 * 定时器由调用方在 onBeforeUnmount 里通过 dispose 收尾，composable 自身不注册生命周期钩子。
 */
export function useLogTail(options: { filePath: Ref<string>; encodingMode: Ref<string>; errorMessage: Ref<string> }) {
  const tailEnabled = ref(false)
  const tailLines = ref<string[]>([])
  const tailOffset = ref(-1)
  const tailEncoding = ref('')
  const tailError = ref('')
  const tailTruncated = ref(false)
  const tailFollow = ref(true)
  const tailBusy = ref(false)
  const tailListElement = ref<HTMLElement | null>(null)
  let timer: number | undefined

  function clearTail() {
    tailLines.value = []
    tailTruncated.value = false
  }
  function stopTail() {
    tailEnabled.value = false
    clearTimer()
  }
  function startTail() {
    if (!options.filePath.value) {
      options.errorMessage.value = '请先选择日志文件。'
      return
    }
    tailLines.value = []
    tailOffset.value = -1
    tailError.value = ''
    tailTruncated.value = false
    tailEnabled.value = true
    clearTimer()
    timer = window.setInterval(() => void pollTail(), TAIL_POLL_INTERVAL_MS)
    void pollTail()
  }
  function toggleTail() {
    if (tailEnabled.value) stopTail()
    else startTail()
  }
  async function pollTail() {
    const a = getBridge()
    if (!a || !options.filePath.value || tailBusy.value) return
    tailBusy.value = true
    try {
      const result = await a.ReadLogTail(options.filePath.value, tailOffset.value, options.encodingMode.value)
      // 读取期间已停止或切换文件：丢弃这次增量，避免清空后又被写回。
      if (!tailEnabled.value) return
      tailEncoding.value = result.encoding
      tailTruncated.value = result.truncated
      if (result.reset) tailLines.value = []
      tailOffset.value = result.offset
      if (result.lines.length) {
        tailLines.value = [...tailLines.value, ...result.lines].slice(-MAX_TAIL_LINES)
        if (tailFollow.value) {
          await nextTick()
          if (tailListElement.value) tailListElement.value.scrollTop = tailListElement.value.scrollHeight
        }
      }
      tailError.value = ''
    } catch (e) {
      tailError.value = '跟踪日志失败：' + String(e)
      stopTail()
    } finally {
      tailBusy.value = false
    }
  }
  function clearTimer() {
    if (timer !== undefined) {
      clearInterval(timer)
      timer = undefined
    }
  }

  return { tailEnabled, tailLines, tailOffset, tailEncoding, tailError, tailTruncated, tailFollow, tailBusy, tailListElement, clearTail, startTail, stopTail, toggleTail, pollTail, dispose: clearTimer }
}
