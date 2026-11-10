<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { bytesToUnit, humanizeBytes, parseHumanized, estimateTransfer, bandwidthPresets, iecUnits, type ByteUnit, type UnitSystem } from '../../utils/byteSize'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

defineProps<{ show?: boolean }>()
useCopyFeedback()

const bytes = ref<number>(1024 * 1024 * 500)
const system = ref<UnitSystem>('iec')
const targetUnit = ref<ByteUnit>('MB')
const humanizedInput = ref('')

const parsedFromHumanized = computed(() => {
  if (!humanizedInput.value.trim()) return null
  try { return parseHumanized(humanizedInput.value, system.value) } catch { return null }
})

watch(humanizedInput, () => {
  if (parsedFromHumanized.value !== null && !isNaN(parsedFromHumanized.value)) {
    bytes.value = parsedFromHumanized.value
  }
})

const allConversions = computed(() => {
  const result: { unit: ByteUnit; value: string; humanized: string }[] = []
  for (const u of iecUnits) {
    result.push({
      unit: u,
      value: bytesToUnit(bytes.value, u, system.value).toFixed(2),
      humanized: humanizeBytes(bytes.value, system.value),
    })
  }
  return result
})

const transferRate = ref<number>(1250000)
const transferResult = computed(() => estimateTransfer(bytes.value, transferRate.value))

function selectPreset(bytesPerSecond: number) { transferRate.value = bytesPerSecond }
function applyHumanized() {
  if (parsedFromHumanized.value !== null) {
    bytes.value = parsedFromHumanized.value
    humanizedInput.value = ''
  }
}
</script>

<template>
  <div class="toolkit-card toolkit-byte-size" v-show="show !== false">
    <h3>字节 / 单位换算</h3>
    <p class="toolkit-hint">实时双向换算：输入任意单位的数值，其余单位即时同步；下方是带宽传输时长估算。</p>
    <div class="toolkit-grid-two">
      <div>
        <label class="toolkit-label">字节数（原始值）</label>
        <input type="number" class="toolkit-input" v-model.number="bytes" min="0" />
      </div>
      <div>
        <label class="toolkit-label">系统</label>
        <select class="toolkit-select" v-model="system">
          <option value="iec">IEC 1024（GiB/MiB/KiB）</option>
          <option value="si">SI 1000（GB/MB/KB）</option>
        </select>
      </div>
    </div>
    <div class="toolkit-subsection">
      <h4>快速解析（"2.5 GiB"、"500 MB"、"1024"）</h4>
      <div class="toolkit-inline-row">
        <input class="toolkit-input" v-model="humanizedInput" placeholder="例如 2.5 GiB" />
        <button class="button button-secondary" @click="applyHumanized">应用</button>
        <span v-if="parsedFromHumanized !== null && !isNaN(parsedFromHumanized)" class="toolkit-note">≈ {{ parsedFromHumanized.toLocaleString() }} 字节</span>
      </div>
    </div>
    <table class="toolkit-table">
      <thead><tr><th>单位</th><th>换算值</th><th>系统</th></tr></thead>
      <tbody>
        <tr v-for="conv in allConversions" :key="conv.unit" @click="targetUnit = conv.unit">
          <td><code>{{ system === 'iec' && conv.unit !== 'B' ? conv.unit + 'iB' : conv.unit }}</code></td>
          <td>{{ conv.value }}</td>
          <td><small>{{ system.toUpperCase() }}</small></td>
        </tr>
      </tbody>
    </table>
    <div class="toolkit-subsection">
      <h4>带宽传输估算</h4>
      <div class="toolkit-inline-row">
        <label class="toolkit-label">传输速率（字节/秒）</label>
        <input type="number" class="toolkit-input" v-model.number="transferRate" min="1" />
      </div>
      <div class="toolkit-result-group">
        <div class="toolkit-result-line"><span class="toolkit-result-key">传输时长</span><strong>{{ transferResult.formatted }}</strong><small>（{{ bytes.toLocaleString() }} 字节 @ {{ humanizeBytes(transferRate) }}/s）</small></div>
      </div>
      <div class="toolkit-preset-buttons">
        <button v-for="p in bandwidthPresets" :key="p.label" class="button button-quiet" @click="selectPreset(p.bytesPerSecond)">{{ p.label }}</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.toolkit-preset-buttons { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
</style>