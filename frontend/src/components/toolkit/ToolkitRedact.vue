<script setup lang="ts">
import { computed, ref } from 'vue'
import { Copy, ShieldAlert } from 'lucide-vue-next'
import { redactSensitiveText } from '../../utils/redact'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

const { copyText } = useCopyFeedback()

const redactInput = ref('Authorization: Bearer secret-token\n{"db_password":"example","service":"api"}')
const redactOutput = computed(() => redactSensitiveText(redactInput.value))
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>日志脱敏</h2><p>敏感字段只在浏览器内替换；原始文本不会离开本机。</p></div><ShieldAlert :size="18" /></div>
    <div class="toolkit-json-grid">
      <div class="toolkit-editor-pane"><label class="toolkit-label" for="redact-input">原始日志或配置</label><textarea id="redact-input" v-model="redactInput" class="toolkit-textarea tall mono" spellcheck="false" placeholder="粘贴日志、JSON、Header 或 URL……"></textarea><div class="toolkit-action-row"><button class="button button-quiet" @click="redactInput = ''">清空</button></div></div>
      <div class="toolkit-editor-pane"><div class="toolkit-output-heading"><label class="toolkit-label">脱敏结果</label><button class="button button-quiet" :disabled="!redactOutput.text" @click="copyText(redactOutput.text, '脱敏结果')"><Copy :size="13" />复制</button></div><pre class="toolkit-json-output redact-output">{{ redactOutput.text }}</pre><p class="toolkit-help">已替换 {{ redactOutput.replacements }} 处值。覆盖 JSON、键值对、Authorization、URL 查询参数与私钥块。</p></div>
    </div>
  </div>
</template>
