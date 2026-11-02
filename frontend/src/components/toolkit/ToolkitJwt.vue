<script setup lang="ts">
import { computed, ref } from 'vue'
import { CheckCircle2, Copy, KeyRound, ShieldAlert } from 'lucide-vue-next'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

const { copyText } = useCopyFeedback()

const jwtInput = ref('')

function decodeBase64URL(segment: string): string {
  const base64 = segment.replace(/-/g, '+').replace(/_/g, '/')
  if (!base64 || !/^[A-Za-z0-9+/]*={0,2}$/.test(base64) || base64.length % 4 === 1) throw new Error('JWT 中的 Base64URL 片段格式不正确')
  const raw = base64.replace(/=+$/, '')
  const binary = atob(raw + '='.repeat((4 - raw.length % 4) % 4))
  const bytes = Uint8Array.from(binary, character => character.charCodeAt(0))
  return new TextDecoder('utf-8', { fatal: true }).decode(bytes)
}

const jwtResult = computed((): { error: string; header: string; payload: string; expires: string; state: 'valid' | 'expired' | 'unknown' } => {
  const token = jwtInput.value.trim()
  if (!token) return { error: '粘贴 JWT 后即可查看 Header 和 Payload。', header: '', payload: '', expires: '', state: 'unknown' }
  const parts = token.split('.')
  if (parts.length !== 3 || !parts[0] || !parts[1] || !parts[2]) return { error: 'JWT 应包含 Header、Payload、Signature 三段，以英文句点分隔。', header: '', payload: '', expires: '', state: 'unknown' }
  try {
    const headerValue: unknown = JSON.parse(decodeBase64URL(parts[0]))
    const payloadValue: unknown = JSON.parse(decodeBase64URL(parts[1]))
    const claims = payloadValue !== null && typeof payloadValue === 'object' ? payloadValue as Record<string, unknown> : {}
    const exp = claims.exp
    let expires = 'Payload 中没有 exp 字段。'
    let state: 'valid' | 'expired' | 'unknown' = 'unknown'
    if (typeof exp === 'number' && Number.isFinite(exp)) {
      const date = new Date(exp * 1000)
      expires = 'exp：' + date.toISOString() + (exp * 1000 <= Date.now() ? '（已过期）' : '（尚未过期）')
      state = exp * 1000 <= Date.now() ? 'expired' : 'valid'
    }
    return { error: '', header: JSON.stringify(headerValue, null, 2), payload: JSON.stringify(payloadValue, null, 2), expires, state }
  } catch (error) {
    return { error: 'JWT 解析失败：' + String(error), header: '', payload: '', expires: '', state: 'unknown' }
  }
})
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>JWT Payload 查看器</h2><p>只解码 JWT 的 Header 和 Payload，不进行签名验证。</p></div><KeyRound :size="18" /></div>
    <label class="toolkit-label" for="jwt-token">JWT Token</label><textarea id="jwt-token" v-model="jwtInput" class="toolkit-textarea jwt-input mono" spellcheck="false" placeholder="粘贴由 Header.Payload.Signature 组成的 JWT……"></textarea>
    <div class="toolkit-security-note"><ShieldAlert :size="16" /><span>安全提示：这里不会验证签名、issuer 或 audience。Payload 只是 Base64URL 编码，不应视为可信声明；不要把私钥或敏感 Token 粘贴到不可信环境。</span></div>
    <p v-if="jwtResult.error" class="toolkit-help">{{ jwtResult.error }}</p>
    <template v-else-if="jwtResult.payload">
      <div class="toolkit-jwt-status" :class="jwtResult.state"><CheckCircle2 v-if="jwtResult.state === 'valid'" :size="15" /><ShieldAlert v-else-if="jwtResult.state === 'expired'" :size="15" /><KeyRound v-else :size="15" />{{ jwtResult.expires }}</div>
      <div class="toolkit-json-grid"><div class="toolkit-editor-pane"><div class="toolkit-output-heading"><label class="toolkit-label">Header</label><button class="button button-quiet" @click="copyText(jwtResult.header, 'JWT Header')"><Copy :size="13" />复制</button></div><pre class="toolkit-json-output">{{ jwtResult.header }}</pre></div><div class="toolkit-editor-pane"><div class="toolkit-output-heading"><label class="toolkit-label">Payload</label><button class="button button-quiet" @click="copyText(jwtResult.payload, 'JWT Payload')"><Copy :size="13" />复制</button></div><pre class="toolkit-json-output">{{ jwtResult.payload }}</pre></div></div>
    </template>
  </div>
</template>
