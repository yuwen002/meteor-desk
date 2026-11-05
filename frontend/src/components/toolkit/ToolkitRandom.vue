<script setup lang="ts">
import { ref } from 'vue'
import { Copy, Dices, RefreshCw } from 'lucide-vue-next'
import { generateRandomString, generateUUID } from '../../utils/random'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

const { copyText } = useCopyFeedback()

const length = ref(16)
const lowercase = ref(true)
const uppercase = ref(true)
const digits = ref(true)
const symbols = ref(false)
const randomOutput = ref('')
const randomMessage = ref('')

const uuidOutput = ref('')

function generate() {
  randomMessage.value = ''
  try {
    randomOutput.value = generateRandomString({ length: length.value, lowercase: lowercase.value, uppercase: uppercase.value, digits: digits.value, symbols: symbols.value })
    if (!randomOutput.value) randomMessage.value = '请至少选择一种字符类别。'
  } catch (error) { randomMessage.value = String(error) }
}

function regenerateUUID() {
  uuidOutput.value = generateUUID()
}

generate()
regenerateUUID()
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>随机字符串 / UUID</h2><p>使用加密安全随机源（crypto.getRandomValues），适合 token、密钥与测试数据。</p></div><Dices :size="18" /></div>

    <div class="toolkit-grid-two">
      <div>
        <label class="toolkit-label" for="rand-length">长度</label>
        <div class="toolkit-input-row"><input id="rand-length" v-model.number="length" type="number" min="1" max="4096" class="toolkit-input" /><button class="button button-secondary" @click="generate"><RefreshCw :size="14" />重新生成</button></div>
        <p class="toolkit-help">长度范围 1 – 4096 个字符。</p>
        <div class="toolkit-checkbox-grid">
          <label><input v-model="lowercase" type="checkbox" />小写 a–z</label>
          <label><input v-model="uppercase" type="checkbox" />大写 A–Z</label>
          <label><input v-model="digits" type="checkbox" />数字 0–9</label>
          <label><input v-model="symbols" type="checkbox" />特殊符号</label>
        </div>
        <div class="toolkit-action-row"><button class="button button-primary" @click="generate">生成字符串</button></div>
      </div>
      <div class="toolkit-result-panel">
        <span class="toolkit-result-kicker">生成结果</span>
        <textarea v-model="randomOutput" class="toolkit-textarea tall mono" readonly placeholder="点击生成" />
        <div class="toolkit-action-row">
          <button class="button button-quiet" @click="copyText(randomOutput, '随机字符串')"><Copy :size="13" />复制</button>
        </div>
        <p v-if="randomMessage" class="toolkit-inline-message">{{ randomMessage }}</p>
      </div>
    </div>

    <div class="toolkit-divider"></div>

    <div class="toolkit-card-heading compact"><div><h3>UUID v4</h3><p>每次点击生成标准 RFC 4122 UUID v4。</p></div></div>
    <div class="toolkit-input-row">
      <input v-model="uuidOutput" class="toolkit-input mono" placeholder="点击下方按钮生成" readonly />
      <button class="button button-primary" @click="regenerateUUID"><RefreshCw :size="14" />生成 UUID</button>
      <button class="button button-quiet" @click="copyText(uuidOutput, 'UUID')"><Copy :size="14" />复制</button>
    </div>
  </div>
</template>

<style scoped>
.toolkit-checkbox-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 4px 14px; margin-top: 6px; font-size: 12.5px; color: #374151; }
.toolkit-checkbox-grid label { display: inline-flex; align-items: center; gap: 5px; cursor: pointer; }
.toolkit-checkbox-grid input[type="checkbox"] { margin: 0; }
</style>