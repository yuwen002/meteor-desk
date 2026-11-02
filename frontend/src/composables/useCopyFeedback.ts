import { ref } from 'vue'

// 复制提示在工具箱的九个面板之间共享同一条状态栏：
// 面板组件负责写入，DeveloperToolkit 负责展示与关闭。
// 模块级 ref 保证跨组件复用同一份状态，DeveloperToolkit 挂载时会主动清空，
// 避免上一次页面停留期间的提示在重新进入工具箱后残留。
const copyStatus = ref('')

export function useCopyFeedback() {
  async function copyText(value: string, label = '内容') {
    copyStatus.value = ''
    if (!value) {
      copyStatus.value = '没有可复制的' + label + '。'
      return
    }
    try {
      await navigator.clipboard.writeText(value)
      copyStatus.value = label + '已复制。'
    } catch {
      copyStatus.value = '复制失败，请检查桌面应用的剪贴板权限。'
    }
  }

  function dismissCopyStatus() {
    copyStatus.value = ''
  }

  return { copyStatus, copyText, dismissCopyStatus }
}
