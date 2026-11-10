<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { hexToRgb, rgbToHex, rgbToHsl, hslToRgb, tailwindPalette } from '../../utils/color'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

defineProps<{ show?: boolean }>()
const { copyText } = useCopyFeedback()

const hexInput = ref('#3b82f6')

const rgbR = ref(59), rgbG = ref(130), rgbB = ref(246)
const hslH = ref(217), hslS = ref(91), hslL = ref(60)

const error = ref('')

function syncFromHex() {
  error.value = ''
  try {
    const rgb = hexToRgb(hexInput.value)
    rgbR.value = rgb.r
    rgbG.value = rgb.g
    rgbB.value = rgb.b
    const hsl = rgbToHsl(rgb.r, rgb.g, rgb.b)
    hslH.value = hsl.h
    hslS.value = hsl.s
    hslL.value = hsl.l
  } catch (e) { error.value = String(e) }
}
function syncFromRgb() {
  const hex = rgbToHex(rgbR.value, rgbG.value, rgbB.value)
  hexInput.value = hex
  const hsl = rgbToHsl(rgbR.value, rgbG.value, rgbB.value)
  hslH.value = hsl.h
  hslS.value = hsl.s
  hslL.value = hsl.l
  error.value = ''
}
function syncFromHsl() {
  const rgb = hslToRgb(hslH.value, hslS.value, hslL.value)
  rgbR.value = rgb.r; rgbG.value = rgb.g; rgbB.value = rgb.b
  hexInput.value = rgbToHex(rgb.r, rgb.g, rgb.b)
  error.value = ''
}

watch(hexInput, () => syncFromHex())
watch([rgbR, rgbG, rgbB], () => syncFromRgb())
watch([hslH, hslS, hslL], () => syncFromHsl())

const previewStyle = computed(() => ({ background: hexInput.value }))
const paletteNames = ['sky', 'emerald', 'amber', 'red', 'violet', 'slate']
</script>

<template>
  <div class="toolkit-card toolkit-color" v-show="show !== false">
    <h3>颜色工具</h3>
    <p class="toolkit-hint">HEX / RGB / HSL 三种色彩空间实时互转；点击调色板任意色块可快速填入；右侧预览当前颜色。</p>
    <div class="toolkit-color-row">
      <div class="toolkit-color-preview" :style="previewStyle" :title="hexInput"></div>
      <div class="toolkit-color-inputs">
        <label class="toolkit-label">HEX</label>
        <input class="toolkit-input" v-model="hexInput" placeholder="#3b82f6" />
        <p v-if="error" class="toolkit-error">{{ error }}</p>
      </div>
    </div>
    <div class="toolkit-grid-three">
      <div class="toolkit-color-space">
        <h4>RGB</h4>
        <div class="toolkit-inline-row">
          <label>R <input type="number" class="toolkit-input" v-model.number="rgbR" min="0" max="255" /></label>
          <label>G <input type="number" class="toolkit-input" v-model.number="rgbG" min="0" max="255" /></label>
          <label>B <input type="number" class="toolkit-input" v-model.number="rgbB" min="0" max="255" /></label>
        </div>
      </div>
      <div class="toolkit-color-space">
        <h4>HSL</h4>
        <div class="toolkit-inline-row">
          <label>H <input type="number" class="toolkit-input" v-model.number="hslH" min="0" max="360" />°</label>
          <label>S <input type="number" class="toolkit-input" v-model.number="hslS" min="0" max="100" />%</label>
          <label>L <input type="number" class="toolkit-input" v-model.number="hslL" min="0" max="100" />%</label>
        </div>
      </div>
      <div class="toolkit-result-group">
        <div class="toolkit-result-line"><span class="toolkit-result-key">HEX</span><code>{{ hexInput.toUpperCase() }}</code><button class="toolkit-copy" @click="copyText(hexInput, 'hex')">复制</button></div>
        <div class="toolkit-result-line"><span class="toolkit-result-key">RGB</span><code>rgb({{ rgbR }}, {{ rgbG }}, {{ rgbB }})</code><button class="toolkit-copy" @click="copyText(`rgb(${rgbR}, ${rgbG}, ${rgbB})`, 'rgb')">复制</button></div>
        <div class="toolkit-result-line"><span class="toolkit-result-key">HSL</span><code>hsl({{ hslH }}, {{ hslS }}%, {{ hslL }}%)</code><button class="toolkit-copy" @click="copyText(`hsl(${hslH}, ${hslS}%, ${hslL}%)`, 'hsl')">复制</button></div>
      </div>
    </div>
    <div class="toolkit-subsection">
      <h4>Tailwind 调色板</h4>
      <div class="toolkit-palette">
        <div v-for="name in paletteNames" :key="name" class="toolkit-palette-group">
          <span class="toolkit-palette-name">{{ name }}</span>
          <div class="toolkit-palette-row">
            <button v-for="scale in [50,100,200,300,400,500,600,700,800,900]" :key="scale" class="toolkit-swatch" :style="{background: tailwindPalette[name][scale]}" :title="tailwindPalette[name][scale]" @click="hexInput = tailwindPalette[name][scale]"></button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.toolkit-color-row { display: flex; gap: 16px; align-items: flex-start; margin-bottom: 12px; }
.toolkit-color-preview { width: 64px; height: 64px; border-radius: 8px; border: 1px solid rgba(0,0,0,0.1); flex-shrink: 0; }
.toolkit-color-inputs { flex: 1; }
.toolkit-color-space h4 { margin: 0 0 6px 0; font-size: 13px; color: var(--muted, #64748b); }
.toolkit-palette { display: flex; flex-direction: column; gap: 10px; }
.toolkit-palette-group { display: flex; flex-direction: column; gap: 4px; }
.toolkit-palette-name { font-size: 12px; color: var(--muted, #64748b); text-transform: lowercase; }
.toolkit-palette-row { display: flex; gap: 2px; }
.toolkit-swatch { width: 100%; height: 18px; border: none; border-radius: 2px; cursor: pointer; padding: 0; }
.toolkit-swatch:hover { outline: 2px solid var(--accent, #3b82f6); }
.toolkit-grid-three { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; }
@media (max-width: 720px) { .toolkit-grid-three { grid-template-columns: 1fr; } }
</style>