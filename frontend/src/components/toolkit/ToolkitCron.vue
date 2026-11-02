<script setup lang="ts">
import { computed, ref } from 'vue'
import { Clock3, Copy } from 'lucide-vue-next'
import { parseCron } from '../../utils/cron'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

const { copyText } = useCopyFeedback()

const cronInput = ref('*/15 9-17 * * 1-5')
const cronResult = computed(() => parseCron(cronInput.value))
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>Cron 表达式解析</h2><p>使用标准五段格式：分 时 日 月 周；按本机时区预览后续执行时间。</p></div><Clock3 :size="18" /></div>
    <label class="toolkit-label" for="cron-input">Cron 表达式</label><input id="cron-input" v-model="cronInput" class="toolkit-input mono" spellcheck="false" placeholder="*/15 9-17 * * 1-5" />
    <p v-if="cronResult.error" class="toolkit-error">{{ cronResult.error }}</p>
    <template v-else><p class="toolkit-help">{{ cronResult.description }}</p><div class="toolkit-diff-heading"><strong>未来执行时间</strong><span>{{ cronResult.nextRuns.length }} 项</span></div><div class="toolkit-param-list"><div v-for="run in cronResult.nextRuns" :key="run.toISOString()"><code>{{ run.toLocaleString('zh-CN') }}</code><span>{{ run.toISOString() }}</span><button @click="copyText(run.toISOString(), '执行时间')"><Copy :size="13" /></button></div></div></template>
  </div>
</template>
