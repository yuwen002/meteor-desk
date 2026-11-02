<script setup lang="ts">
import { ref } from 'vue'
import { ArrowLeftRight, Copy, FileJson } from 'lucide-vue-next'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

const { copyText } = useCopyFeedback()

const base64Source = ref('')
const base64Encoded = ref('')
const base64Decoded = ref('')
const base64Message = ref('')

function encodeBase64() {
  base64Message.value = ''
  try {
    const bytes = new TextEncoder().encode(base64Source.value)
    let binary = ''
    for (let offset = 0; offset < bytes.length; offset += 0x8000) {
      binary += String.fromCharCode(...bytes.subarray(offset, Math.min(offset + 0x8000, bytes.length)))
    }
    base64Encoded.value = btoa(binary)
    base64Message.value = '编码完成（UTF-8）。'
  } catch (error) { base64Message.value = '编码失败：' + String(error) }
}

function decodeBase64() {
  base64Message.value = ''
  try {
    const input = base64Encoded.value.replace(/\s+/g, '').replace(/-/g, '+').replace(/_/g, '/')
    if (!input || !/^[A-Za-z0-9+/]*={0,2}$/.test(input) || input.length % 4 === 1) throw new Error('Base64 字符串格式不正确')
    const normalized = input.replace(/=+$/, '')
    const padded = normalized + '='.repeat((4 - normalized.length % 4) % 4)
    const binary = atob(padded)
    const bytes = Uint8Array.from(binary, character => character.charCodeAt(0))
    base64Decoded.value = new TextDecoder('utf-8', { fatal: true }).decode(bytes)
    base64Message.value = '解码完成（UTF-8）。'
  } catch (error) { base64Message.value = '解码失败：' + String(error) }
}

function resetBase64() {
  base64Source.value = ''
  base64Encoded.value = ''
  base64Decoded.value = ''
  base64Message.value = ''
}
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>Base64 编码与解码</h2><p>按 UTF-8 处理文本，支持中文和常见多字节字符。</p></div><FileJson :size="18" /></div>
    <div class="toolkit-json-grid">
      <div class="toolkit-editor-pane"><label class="toolkit-label" for="base64-source">原始文本</label><textarea id="base64-source" v-model="base64Source" class="toolkit-textarea tall" placeholder="输入要编码的文本……"></textarea><div class="toolkit-action-row"><button class="button button-primary" @click="encodeBase64">编码为 Base64 <ArrowLeftRight :size="14" /></button><button class="button button-quiet" @click="resetBase64">清空</button></div></div>
      <div class="toolkit-editor-pane"><label class="toolkit-label" for="base64-result">Base64 字符串</label><textarea id="base64-result" v-model="base64Encoded" class="toolkit-textarea tall mono" placeholder="编码结果或粘贴待解码字符串……"></textarea><div class="toolkit-action-row"><button class="button button-primary" @click="decodeBase64">解码为文本 <ArrowLeftRight :size="14" /></button><button class="button button-quiet" @click="copyText(base64Encoded, 'Base64 字符串')"><Copy :size="14" />复制</button></div></div>
    </div>
    <div v-if="base64Message" class="toolkit-inline-message">{{ base64Message }}</div>
    <div v-if="base64Decoded !== ''" class="toolkit-output-block"><div class="toolkit-output-heading"><strong>解码结果</strong><button class="button button-quiet" @click="copyText(base64Decoded, '解码文本')"><Copy :size="13" />复制</button></div><pre>{{ base64Decoded }}</pre></div>
  </div>
</template>
