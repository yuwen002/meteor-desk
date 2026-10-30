<script setup lang="ts">
import { ref } from 'vue'
import { X } from 'lucide-vue-next'
import { DEFAULT_SHORTCUTS, SHORTCUT_OPTIONS, validateShortcutMap, type ShortcutAction, type ShortcutMap } from '../utils/shortcuts'

const props = defineProps<{ config: ShortcutMap }>()
const emit = defineEmits<{ close: []; change: [map: ShortcutMap] }>()
const draft = ref<ShortcutMap>({ ...props.config }), error = ref('')

const shortcutActions: { key: ShortcutAction; label: string; description: string }[] = [
  { key: 'openFile', label: '打开日志文件', description: '打开系统文件选择器' },
  { key: 'focusSearch', label: '聚焦搜索框', description: '切换到日志分析并聚焦搜索输入' },
  { key: 'runSearch', label: '开始检索', description: '在日志分析页面启动检索' },
  { key: 'logs', label: '日志分析页面', description: '切换到日志分析' },
  { key: 'processes', label: '进程管理页面', description: '切换到本地服务管理' },
  { key: 'toolkit', label: '开发工具页面', description: '切换到开发工具箱' },
]

function save() {
  const validationError = validateShortcutMap(draft.value)
  if (validationError) { error.value = validationError; return }
  try {
    window.localStorage.setItem('meteordesk.shortcuts.v1', JSON.stringify(draft.value))
    error.value = ''
    emit('change', { ...draft.value })
    emit('close')
  } catch { error.value = '无法保存快捷键设置，请检查本地存储权限。' }
}

function reset() { draft.value = { ...DEFAULT_SHORTCUTS }; error.value = '' }
</script>

<template>
 <div class="settings-overlay" @click.self="emit('close')">
  <section class="settings-dialog" role="dialog" aria-modal="true" aria-labelledby="shortcuts-title">
   <div class="settings-dialog-heading"><div><span class="eyebrow">PREFERENCES</span><h2 id="shortcuts-title">快捷键设置</h2><p>快捷键使用 Ctrl（Windows/Linux）或 ⌘（macOS）。修改后立即生效。</p></div><button class="icon-button" type="button" aria-label="关闭" @click="emit('close')"><X :size="17"/></button></div>
   <div class="shortcut-settings-list"><label v-for="action in shortcutActions" :key="action.key" class="shortcut-setting-row"><span><strong>{{action.label}}</strong><small>{{action.description}}</small></span><select v-model="draft[action.key]" :aria-label="action.label+'快捷键'"><option v-for="option in SHORTCUT_OPTIONS" :key="option.value" :value="option.value">{{option.label}}</option></select></label></div>
   <p v-if="error" class="settings-error" role="alert">{{error}}</p>
   <div class="settings-dialog-actions"><button class="button button-secondary" type="button" @click="reset">恢复默认</button><span class="option-spacer"></span><button class="button button-secondary" type="button" @click="emit('close')">取消</button><button class="button button-primary" type="button" @click="save">保存设置</button></div>
  </section>
 </div>
</template>
