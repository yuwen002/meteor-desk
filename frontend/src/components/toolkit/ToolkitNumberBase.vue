<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { parseNumber, allBases, extractBitRange, constantsForWidth, bitWidths, numberBaseLabels, type NumericBase, type BitWidth, type NumberRepresentation } from '../../utils/numberBase'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

defineProps<{ show?: boolean }>()
const { copyText } = useCopyFeedback()

const inputText = ref('42')
const base = ref<NumericBase>(10)
const width = ref<BitWidth>(32)
const signed = ref(true)
const error = ref('')
const rep = ref<NumberRepresentation | null>(null)

function parseRep() {
  if (!inputText.value.trim()) { rep.value = null; error.value = ''; return }
  try {
    error.value = ''
    rep.value = parseNumber(inputText.value.trim(), base.value, width.value, signed.value)
  } catch (e) {
    error.value = String(e)
    rep.value = null
  }
}
watch([inputText, base, width, signed], parseRep, { immediate: true })

const bases = computed(() => rep.value ? allBases(rep.value, signed.value) : null)

const high = ref(15)
const low = ref(0)
const bitExtraction = computed(() => {
  if (!rep.value || error.value) return null
  try {
    return extractBitRange(rep.value, Math.min(high.value, width.value - 1), low.value)
  } catch {
    return null
  }
})

const constants = computed(() => constantsForWidth(width.value))
</script>

<template>
  <div class="toolkit-card toolkit-number-base" v-show="show !== false">
    <h3>进制转换</h3>
    <p class="toolkit-hint">同一数字在十 / 二 / 八 / 十六进制之间实时互转，可选有符号/无符号与位宽；下方是位段提取与常见常量。</p>
    <div class="toolkit-grid-two">
      <div>
        <label class="toolkit-label">输入数字</label>
        <input class="toolkit-input" v-model="inputText" placeholder="例如 42 / 0xFF / 101010" />
      </div>
      <div class="toolkit-inline-row">
        <div>
          <label class="toolkit-label">输入进制</label>
          <select v-model="base" class="toolkit-select">
            <option v-for="b in ([2, 8, 10, 16] as NumericBase[])" :key="b" :value="b">{{ numberBaseLabels[b] }}</option>
          </select>
        </div>
        <div>
          <label class="toolkit-label">位宽</label>
          <select v-model="width" class="toolkit-select">
            <option v-for="w in bitWidths" :key="w" :value="w">{{ w }} 位</option>
          </select>
        </div>
        <label class="toolkit-checkbox"><input type="checkbox" v-model="signed" /> 有符号</label>
      </div>
    </div>
    <p v-if="error" class="toolkit-error">{{ error }}</p>
    <div v-if="bases" class="toolkit-result-group">
      <div v-for="b in ([2, 8, 10, 16] as NumericBase[])" :key="b" class="toolkit-result-line">
        <span class="toolkit-result-key">{{ numberBaseLabels[b] }}</span>
        <code class="toolkit-result-value">{{ bases[b] }}</code>
        <button class="toolkit-copy" :data-key="'base-' + b" @click="copyText(bases[b], 'base-' + b)">复制</button>
      </div>
    </div>
    <div class="toolkit-subsection">
      <h4>位段提取</h4>
      <div class="toolkit-inline-row">
        <label class="toolkit-label">高位</label>
        <input type="number" class="toolkit-input" v-model.number="high" :max="width - 1" :min="0" />
        <label class="toolkit-label">低位</label>
        <input type="number" class="toolkit-input" v-model.number="low" :max="width - 1" :min="0" />
      </div>
      <div v-if="bitExtraction" class="toolkit-result-group">
        <div class="toolkit-result-line"><span class="toolkit-result-key">十进制</span><code>{{ bitExtraction.decimal }}</code></div>
        <div class="toolkit-result-line"><span class="toolkit-result-key">二进制</span><code>{{ bitExtraction.binary }}</code></div>
      </div>
    </div>
    <div class="toolkit-subsection">
      <h4>{{ width }} 位常用常量</h4>
      <table class="toolkit-table">
        <thead><tr><th>名称</th><th>十进制</th><th>十六进制</th></tr></thead>
        <tbody>
          <tr v-for="c in constants" :key="c.name">
            <td>{{ c.name }}<small v-if="c.note" class="toolkit-note">（{{ c.note }}）</small></td>
            <td><code>{{ c.decimal }}</code></td>
            <td><code>{{ c.hex }}</code></td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>