<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowLeftRight, Braces, CheckCircle2, Copy } from 'lucide-vue-next'
import { appendJSONPath } from '../../utils/jsonPath'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

interface JsonDiffRow { path: string; kind: '新增' | '删除' | '修改'; before: unknown; after: unknown }

const { copyText } = useCopyFeedback()

const jsonInput = ref('')
const jsonCompareInput = ref('')
const jsonOutput = ref('')
const jsonMessage = ref('')

function parseJSON(value: string): { valid: true; value: unknown } | { valid: false; error: string } {
  try { return { valid: true, value: JSON.parse(value) as unknown } }
  catch (error) { return { valid: false, error: error instanceof Error ? error.message : String(error) } }
}
function prettyJSON(source: string, compact = false): string {
  const parsed = parseJSON(source)
  if (!parsed.valid) throw new Error(parsed.error)
  return JSON.stringify(parsed.value, null, compact ? undefined : 2)
}
function formatJSON(target: 'left' | 'right' | 'compact') {
  jsonMessage.value = ''
  const source = target === 'right' ? jsonCompareInput.value : jsonInput.value
  try {
    const formatted = prettyJSON(source, target === 'compact')
    if (target === 'right') jsonCompareInput.value = formatted
    else jsonInput.value = formatted
    jsonOutput.value = formatted
    jsonMessage.value = target === 'compact' ? 'JSON 已压缩。' : 'JSON 格式有效，已完成格式化。'
  } catch (error) {
    jsonMessage.value = 'JSON 解析失败：' + String(error)
  }
}
function flattenJSON(value: unknown, path = '$', target = new Map<string, unknown>()): Map<string, unknown> {
  if (Array.isArray(value)) {
    if (!value.length) target.set(path, [])
    value.forEach((item, index) => flattenJSON(item, path + '[' + index + ']', target))
  } else if (value !== null && typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
    if (!entries.length) target.set(path, {})
    entries.forEach(([key, child]) => flattenJSON(child, appendJSONPath(path, key), target))
  } else {
    target.set(path, value)
  }
  return target
}
const jsonComparison = computed((): { error: string; rows: JsonDiffRow[] } => {
  if (!jsonInput.value.trim() || !jsonCompareInput.value.trim()) return { error: '在左右两侧输入 JSON 后，即可按字段路径比较差异。', rows: [] }
  const before = parseJSON(jsonInput.value)
  if (!before.valid) return { error: '左侧 JSON 无效：' + before.error, rows: [] }
  const after = parseJSON(jsonCompareInput.value)
  if (!after.valid) return { error: '右侧 JSON 无效：' + after.error, rows: [] }
  try {
    const a = flattenJSON(before.value)
    const b = flattenJSON(after.value)
    if (a.size + b.size > 20000) return { error: '字段数量过多，当前版本最多对比约 20,000 个叶子字段。', rows: [] }
    const paths = Array.from(new Set([...a.keys(), ...b.keys()])).sort()
    const rows: JsonDiffRow[] = []
    for (const path of paths) {
      const hasBefore = a.has(path)
      const hasAfter = b.has(path)
      if (!hasBefore) rows.push({ path, kind: '新增', before: undefined, after: b.get(path) })
      else if (!hasAfter) rows.push({ path, kind: '删除', before: a.get(path), after: undefined })
      else if (JSON.stringify(a.get(path)) !== JSON.stringify(b.get(path))) rows.push({ path, kind: '修改', before: a.get(path), after: b.get(path) })
    }
    return { error: '', rows }
  } catch (error) {
    return { error: '对比失败：' + String(error), rows: [] }
  }
})
function showJSONValue(value: unknown): string {
  if (value === undefined) return '—'
  const result = JSON.stringify(value)
  return result === undefined ? String(value) : result
}
function resetJSON() {
  jsonInput.value = ''
  jsonCompareInput.value = ''
  jsonOutput.value = ''
  jsonMessage.value = ''
}
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>JSON 格式化与结构化对比</h2><p>格式化、压缩 JSON，并按字段路径识别新增、删除和修改项。</p></div><Braces :size="18" /></div>
    <div class="toolkit-action-row json-global-actions"><button class="button button-primary" @click="formatJSON('left')"><Braces :size="14" />格式化左侧</button><button class="button button-secondary" @click="formatJSON('right')"><Braces :size="14" />格式化右侧</button><button class="button button-secondary" @click="formatJSON('compact')">压缩左侧</button><button class="button button-quiet" @click="copyText(jsonOutput || jsonInput, 'JSON')"><Copy :size="14" />复制最近输出</button><button class="button button-quiet" @click="resetJSON">清空</button></div>
    <div class="toolkit-json-grid">
      <div class="toolkit-editor-pane"><label class="toolkit-label" for="json-left">原始 JSON（左侧）</label><textarea id="json-left" v-model="jsonInput" class="toolkit-textarea mono" spellcheck="false" placeholder='{"name":"MeteorDesk","version":1}'></textarea></div>
      <div class="toolkit-editor-pane"><label class="toolkit-label" for="json-right">对比 JSON（右侧）</label><textarea id="json-right" v-model="jsonCompareInput" class="toolkit-textarea mono" spellcheck="false" placeholder='{"name":"MeteorDesk","version":2}'></textarea></div>
    </div>
    <p v-if="jsonMessage" class="toolkit-inline-message">{{ jsonMessage }}</p>
    <div class="toolkit-diff-heading"><strong>结构化差异</strong><span v-if="!jsonComparison.error">发现 {{ jsonComparison.rows.length }} 项差异</span></div>
    <p v-if="jsonComparison.error" class="toolkit-help">{{ jsonComparison.error }}</p>
    <div v-else-if="!jsonComparison.rows.length" class="toolkit-empty"><CheckCircle2 :size="17" />左右 JSON 内容一致。</div>
    <div v-else class="toolkit-diff-list"><div v-for="row in jsonComparison.rows" :key="row.path" class="toolkit-diff-row" :class="row.kind"><div class="toolkit-diff-path"><span>{{ row.kind }}</span><code>{{ row.path }}</code></div><div class="toolkit-diff-values"><code>{{ showJSONValue(row.before) }}</code><ArrowLeftRight :size="13" /><code>{{ showJSONValue(row.after) }}</code></div></div></div>
    <p class="toolkit-help">比较基于解析后的字段路径；数组按下标比较。大于约 20,000 个叶子字段时会停止差异展开。</p>
  </div>
</template>
