<script setup lang="ts">
import { computed, ref } from 'vue'
import { Copy, Clock3, RefreshCw } from 'lucide-vue-next'
import { getDateTimeInput, zonedDateTimeToEpoch } from '../../utils/timezone'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

const { copyText } = useCopyFeedback()

const timestampInput = ref(String(Math.floor(Date.now() / 1000)))
const timestampUnit = ref<'seconds' | 'milliseconds'>('seconds')
const timeZone = ref('Asia/Shanghai')
const localDateTime = ref(getDateTimeInput(Date.now(), 'Asia/Shanghai'))

const timestampInfo = computed(() => {
  const raw = timestampInput.value.trim()
  if (!raw) return { error: '请输入时间戳。', formatted: '', iso: '', seconds: '', milliseconds: '' }
  const n = Number(raw)
  if (!Number.isFinite(n)) return { error: '时间戳必须是有效数字。', formatted: '', iso: '', seconds: '', milliseconds: '' }
  const epochMs = timestampUnit.value === 'seconds' ? n * 1000 : n
  const date = new Date(epochMs)
  if (!Number.isFinite(date.getTime())) return { error: '该时间戳超出 JavaScript Date 支持的范围。', formatted: '', iso: '', seconds: '', milliseconds: '' }
  try {
    return {
      error: '',
      formatted: new Intl.DateTimeFormat('zh-CN', { timeZone: timeZone.value, dateStyle: 'full', timeStyle: 'long' }).format(date),
      iso: date.toISOString(),
      seconds: String(Math.floor(epochMs / 1000)),
      milliseconds: String(Math.trunc(epochMs)),
    }
  } catch {
    return { error: '无法识别该时区，请使用有效的 IANA 时区名称，例如 Asia/Shanghai。', formatted: '', iso: '', seconds: '', milliseconds: '' }
  }
})

const localDateEpoch = computed(() => {
  try {
    return zonedDateTimeToEpoch(localDateTime.value, timeZone.value)
  } catch {
    return null
  }
})
const localDateError = computed(() => {
  if (!localDateTime.value) return '请选择日期和时间。'
  if (localDateEpoch.value === null) return '日期无效，或该时区的本地时间不存在（例如夏令时跳时）。'
  return ''
})

function useNow() {
  timestampInput.value = String(Math.floor(Date.now() / (timestampUnit.value === 'seconds' ? 1000 : 1)))
}
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>时间戳与时区转换</h2><p>支持 Unix 秒、毫秒与指定时区的本地日期时间互转。</p></div><Clock3 :size="18" /></div>
    <div class="toolkit-grid-two">
      <div>
        <label class="toolkit-label" for="timestamp-value">Unix 时间戳</label>
        <div class="toolkit-input-row"><input id="timestamp-value" v-model="timestampInput" class="toolkit-input mono" inputmode="decimal" placeholder="例如 1735689600" /><select v-model="timestampUnit" class="toolkit-select" aria-label="时间戳单位"><option value="seconds">秒（s）</option><option value="milliseconds">毫秒（ms）</option></select></div>
        <p class="toolkit-help">秒和毫秒可切换，结果会同步更新。</p>
        <label class="toolkit-label" for="timezone-name">IANA 时区名称</label>
        <input id="timezone-name" v-model="timeZone" class="toolkit-input mono" placeholder="Asia/Shanghai" list="common-timezones" />
        <datalist id="common-timezones"><option value="Asia/Shanghai" /><option value="Asia/Singapore" /><option value="UTC" /><option value="America/Los_Angeles" /><option value="Europe/London" /><option value="Asia/Tokyo" /></datalist>
        <div class="toolkit-action-row"><button class="button button-secondary" @click="useNow"><RefreshCw :size="14" />当前时间</button><button class="button button-quiet" @click="copyText(timestampInfo.seconds, '秒级时间戳')"><Copy :size="14" />复制秒级时间戳</button></div>
      </div>
      <div class="toolkit-result-panel">
        <div v-if="timestampInfo.error" class="toolkit-error">{{ timestampInfo.error }}</div>
        <template v-else>
          <span class="toolkit-result-kicker">指定时区时间</span><strong class="toolkit-time-result">{{ timestampInfo.formatted }}</strong>
          <div class="toolkit-result-line"><span>Unix 秒</span><code>{{ timestampInfo.seconds }}</code><button aria-label="复制秒级时间戳" @click="copyText(timestampInfo.seconds, '秒级时间戳')"><Copy :size="13" /></button></div>
          <div class="toolkit-result-line"><span>Unix 毫秒</span><code>{{ timestampInfo.milliseconds }}</code><button aria-label="复制毫秒时间戳" @click="copyText(timestampInfo.milliseconds, '毫秒时间戳')"><Copy :size="13" /></button></div>
          <div class="toolkit-result-line"><span>UTC ISO 8601</span><code>{{ timestampInfo.iso }}</code><button aria-label="复制 ISO 时间" @click="copyText(timestampInfo.iso, 'ISO 时间')"><Copy :size="13" /></button></div>
        </template>
      </div>
    </div>
    <div class="toolkit-divider"></div>
    <div class="toolkit-card-heading compact"><div><h3>本地日期时间转时间戳</h3><p>输入框里的日期时间将按上方时区解释。</p></div></div>
    <div class="toolkit-grid-two bottom-converter">
      <div><label class="toolkit-label" for="local-datetime">日期和时间（{{ timeZone || '请填写时区' }}）</label><input id="local-datetime" v-model="localDateTime" type="datetime-local" class="toolkit-input" step="1" /><p v-if="localDateError" class="toolkit-error small">{{ localDateError }}</p><p v-else class="toolkit-help">切换时区不会改写输入的墙上时间；重新选择即可换算。</p></div>
      <div class="toolkit-result-panel compact-result"><template v-if="!localDateError"><div class="toolkit-result-line"><span>Unix 秒</span><code>{{ Math.floor((localDateEpoch ?? 0) / 1000) }}</code><button @click="copyText(String(Math.floor((localDateEpoch ?? 0) / 1000)), '秒级时间戳')"><Copy :size="13" /></button></div><div class="toolkit-result-line"><span>Unix 毫秒</span><code>{{ localDateEpoch }}</code><button @click="copyText(String(localDateEpoch), '毫秒时间戳')"><Copy :size="13" /></button></div><div class="toolkit-result-line"><span>对应 UTC</span><code>{{ new Date(localDateEpoch ?? 0).toISOString() }}</code><button @click="copyText(new Date(localDateEpoch ?? 0).toISOString(), 'UTC 时间')"><Copy :size="13" /></button></div></template></div>
    </div>
  </div>
</template>
