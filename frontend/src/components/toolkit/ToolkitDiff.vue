<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowLeftRight } from 'lucide-vue-next'
import { diffText } from '../../utils/textDiff'

const diffBefore = ref('server started\nport=8080\nmode=development')
const diffAfter = ref('server started\nport=8081\nmode=production')
const textDifference = computed(() => {
  try { return { error: '', rows: diffText(diffBefore.value, diffAfter.value) } }
  catch (error) { return { error: String(error), rows: [] } }
})
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>文本差异比较</h2><p>按行比较两段文本，适合配置、响应和日志片段。</p></div><ArrowLeftRight :size="18" /></div>
    <div class="toolkit-json-grid"><div class="toolkit-editor-pane"><label class="toolkit-label" for="diff-before">原始文本</label><textarea id="diff-before" v-model="diffBefore" class="toolkit-textarea tall mono" spellcheck="false"></textarea></div><div class="toolkit-editor-pane"><label class="toolkit-label" for="diff-after">修改后文本</label><textarea id="diff-after" v-model="diffAfter" class="toolkit-textarea tall mono" spellcheck="false"></textarea></div></div>
    <p v-if="textDifference.error" class="toolkit-error">{{ textDifference.error }}</p><div v-else class="text-diff-output"><div v-for="(row, index) in textDifference.rows" :key="index" :class="row.kind"><b>{{ row.kind === 'added' ? '+' : row.kind === 'removed' ? '-' : ' ' }}</b><code>{{ row.value }}</code></div></div>
  </div>
</template>
