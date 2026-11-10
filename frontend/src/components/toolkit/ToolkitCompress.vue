<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { compressText, decompressToText, compressionSupported, bytesToBase64, base64ToBytes, type CompressionAlgorithm, compressionAlgorithms } from '../../utils/compress'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

defineProps<{ show?: boolean }>()
const { copyText } = useCopyFeedback()

const inputText = ref('Hello, this is some repetitive text. Hello, hello, hello world!')

const algo = ref<CompressionAlgorithm>('gzip')
const mode = ref<'compress' | 'decompress'>('compress')
const compressedData = ref<Uint8Array | null>(null)
const error = ref('')
const supported = compressionSupported()

async function run() {
  error.value = ''
  try {
    if (mode.value === 'compress') {
      compressedData.value = await compressText(inputText.value, algo.value)
    } else {
      const data = base64ToBytes(inputText.value.trim())
      const text = await decompressToText(data, algo.value)
      compressedData.value = null
      inputText.value = text
    }
  } catch (e) {
    error.value = String(e)
  }
}

const inputSize = computed(() => new TextEncoder().encode(inputText.value).length)
const outputBase64Value = computed(() => compressedData.value ? bytesToBase64(compressedData.value) : '')
const outputSize = computed(() => compressedData.value?.length ?? 0)
const ratio = computed(() => {
  if (!inputSize.value || !outputSize.value) return null
  return ((outputSize.value / inputSize.value) * 100).toFixed(1) + '%'
})

watch([inputText, algo, mode], () => run(), { immediate: true })
</script>

<template>
  <div class="toolkit-card toolkit-compress" v-show="show !== false">
    <h3>gzip / brotli 压缩解压</h3>
    <p class="toolkit-hint">浏览器原生 CompressionStream / DecompressionStream；支持 gzip、deflate、brotli 三种算法。</p>
    <div v-if="!supported.gzip" class="toolkit-error">当前浏览器不支持 CompressionStream API。</div>
    <template v-else>
      <div class="toolkit-inline-row">
        <label class="toolkit-label">算法</label>
        <select class="toolkit-select" v-model="algo">
          <option v-for="a in compressionAlgorithms" :key="a" :value="a">{{ a }}</option>
        </select>
        <label class="toolkit-checkbox"><input type="radio" v-model="mode" value="compress" /> 压缩</label>
        <label class="toolkit-checkbox"><input type="radio" v-model="mode" value="decompress" /> 解压</label>
      </div>
      <div class="toolkit-textarea-group">
        <label class="toolkit-label">{{ mode === 'compress' ? '输入文本' : '输入 Base64' }}</label>
        <textarea class="toolkit-textarea" v-model="inputText" rows="6" :placeholder="mode === 'compress' ? '输入要压缩的文本...' : '输入要解压的 Base64...'" />
        <div class="toolkit-textarea-meta">
          <small>原始：{{ inputSize.toLocaleString() }} 字节</small>
          <button class="button button-secondary" @click="run">立即{{ mode === 'compress' ? '压缩' : '解压' }}</button>
        </div>
      </div>
      <p v-if="error" class="toolkit-error">{{ error }}</p>
      <div v-if="compressedData && mode === 'compress'" class="toolkit-result-group">
        <div class="toolkit-result-line"><span class="toolkit-result-key">压缩后大小</span><strong>{{ outputSize.toLocaleString() }} 字节</strong><small v-if="ratio">（{{ ratio }}）</small></div>
        <div class="toolkit-subsection">
          <h4>压缩结果（Base64）</h4>
          <div class="toolkit-result-line"><code class="toolkit-result-value">{{ outputBase64Value }}</code><button class="toolkit-copy" @click="copyText(outputBase64Value, 'b64')">复制</button></div>
        </div>
      </div>
    </template>
  </div>
</template>