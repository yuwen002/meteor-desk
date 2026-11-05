<script setup lang="ts">
import { ref, watchEffect } from 'vue'
import { Copy, Fingerprint, ShieldCheck } from 'lucide-vue-next'
import { digestText, hmacText, hashAlgorithms } from '../../utils/hash'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

const { copyText } = useCopyFeedback()

const hashText = ref('')
const hashResults = ref<Record<string, string>>({})
const hashError = ref('')
const hmacBody = ref('')
const hmacSecret = ref('')
const hmacAlgo = ref<'SHA-256' | 'SHA-384' | 'SHA-512'>('SHA-256')
const hmacResult = ref('')
const hmacError = ref('')

let hashToken = 0
watchEffect(async () => {
  const current = ++hashToken
  hashError.value = ''
  const input = hashText.value
  if (!input.trim()) { hashResults.value = {}; return }
  const entries: Record<string, string> = {}
  await Promise.all(hashAlgorithms.map(async (algo) => {
    try { entries[algo] = await digestText(input, algo) }
    catch { hashError.value = '当前环境不支持 Web Crypto，无法计算哈希。' }
  }))
  if (current === hashToken) hashResults.value = entries
})

let hmacToken = 0
watchEffect(async () => {
  const current = ++hmacToken
  hmacError.value = ''
  if (!hmacBody.value.trim() || !hmacSecret.value.trim()) { hmacResult.value = ''; return }
  try {
    const value = await hmacText(hmacBody.value, hmacSecret.value, hmacAlgo.value)
    if (current === hmacToken) hmacResult.value = value
  } catch { hmacError.value = '当前环境不支持 Web Crypto，无法计算 HMAC。' }
})

const hmacAlgorithms: ('SHA-256' | 'SHA-384' | 'SHA-512')[] = ['SHA-256', 'SHA-384', 'SHA-512']
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>哈希摘要</h2><p>使用浏览器原生 Web Crypto，全部在本机计算，不上传任何内容。</p></div><Fingerprint :size="18" /></div>

    <label class="toolkit-label" for="hash-input">输入文本</label>
    <textarea id="hash-input" v-model="hashText" class="toolkit-textarea short mono" placeholder="粘贴要计算摘要的文本……" />

    <p class="toolkit-help">以下为输入的 SHA 系列摘要；如输入为空则不显示结果。</p>
    <div v-if="hashError" class="toolkit-error">{{ hashError }}</div>
    <div v-else-if="hashText.trim()" class="toolkit-result-panel hash-results">
      <div v-for="algo in hashAlgorithms" :key="algo" class="toolkit-result-line">
        <span>{{ algo }}</span>
        <code>{{ hashResults[algo] || '…' }}</code>
        <button v-if="hashResults[algo]" :aria-label="'复制 ' + algo + ' 摘要'" @click="copyText(hashResults[algo], algo + ' 摘要')"><Copy :size="13" /></button>
      </div>
    </div>

    <div class="toolkit-divider"></div>

    <div class="toolkit-card-heading compact"><div><h3>HMAC 校验码</h3><p>用密钥对文本计算 HMAC，常用于接口签名核对。</p></div><ShieldCheck :size="16" /></div>
    <div class="toolkit-grid-two">
      <div>
        <label class="toolkit-label" for="hmac-body">消息文本</label>
        <textarea id="hmac-body" v-model="hmacBody" class="toolkit-textarea short mono" placeholder="要签名的消息……" />
        <label class="toolkit-label" for="hmac-secret">密钥</label>
        <input id="hmac-secret" v-model="hmacSecret" type="password" class="toolkit-input mono" placeholder="共享密钥" autocomplete="off" />
        <label class="toolkit-label" for="hmac-algo">算法</label>
        <select id="hmac-algo" v-model="hmacAlgo" class="toolkit-select"><option v-for="algo in hmacAlgorithms" :key="algo" :value="algo">{{ algo }}</option></select>
      </div>
      <div class="toolkit-result-panel">
        <span class="toolkit-result-kicker">HMAC 结果</span>
        <div v-if="hmacError" class="toolkit-error">{{ hmacError }}</div>
        <div v-else-if="hmacResult" class="toolkit-output-block">
          <pre class="toolkit-hmac-value">{{ hmacResult }}</pre>
          <div class="toolkit-action-row"><button class="button button-quiet" @click="copyText(hmacResult, 'HMAC 结果')"><Copy :size="13" />复制</button></div>
        </div>
        <p v-else class="toolkit-help">同时输入消息文本与密钥后自动计算。</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.hash-results { margin-top: 10px; }
.toolkit-hmac-value { background: #fff; border: 1px solid var(--toolkit-border); border-radius: 8px; padding: 10px; font-family: "Cascadia Code", Consolas, monospace; font-size: 12px; word-break: break-all; white-space: pre-wrap; }
</style>