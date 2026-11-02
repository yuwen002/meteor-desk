<script setup lang="ts">
import { computed, ref } from 'vue'
import { Copy, Link2, RefreshCw } from 'lucide-vue-next'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

const { copyText } = useCopyFeedback()

const urlInput = ref('https://example.com/path?name=Meteor%20Desk&mode=local')
const urlMessage = ref('')
const urlResult = computed((): { error: string; kind: 'url' | 'query'; rows: { key: string; value: string }[]; origin: string; pathname: string; hash: string } => {
  const input = urlInput.value.trim()
  if (!input) return { error: '输入完整 URL 或查询字符串即可解析。', kind: 'query', rows: [], origin: '', pathname: '', hash: '' }
  try {
    if (/^[a-zA-Z][a-zA-Z\d+.-]*:\/\//.test(input)) {
      const url = new URL(input)
      return { error: '', kind: 'url', rows: Array.from(url.searchParams.entries()).map(([key, value]) => ({ key, value })), origin: url.origin, pathname: url.pathname, hash: url.hash }
    }
    const params = new URLSearchParams(input.replace(/^[?#]/, ''))
    return { error: '', kind: 'query', rows: Array.from(params.entries()).map(([key, value]) => ({ key, value })), origin: '', pathname: '', hash: '' }
  } catch (error) {
    return { error: '无法解析输入：' + String(error), kind: 'query', rows: [], origin: '', pathname: '', hash: '' }
  }
})
const urlEncodeInput = ref('')
const urlDecodeInput = ref('')
const urlEncoded = computed(() => encodeURIComponent(urlEncodeInput.value))
const urlDecoded = computed(() => {
  try { return { value: decodeURIComponent(urlDecodeInput.value), error: '' } }
  catch (error) { return { value: '', error: '解码失败：' + String(error) } }
})

function resetURL() {
  urlMessage.value = ''
  urlInput.value = ''
  urlEncodeInput.value = ''
  urlDecodeInput.value = ''
}
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>URL 与查询参数</h2><p>解析完整 URL 或 query string，并对 URL 组件进行编码、解码。</p></div><Link2 :size="18" /></div>
    <label class="toolkit-label" for="url-input">完整 URL 或查询字符串</label><textarea id="url-input" v-model="urlInput" class="toolkit-textarea url-input mono" spellcheck="false" placeholder="https://example.com/path?a=1&b=hello%20world"></textarea>
    <div v-if="urlResult.error" class="toolkit-help">{{ urlResult.error }}</div>
    <template v-else>
      <div v-if="urlResult.kind === 'url'" class="toolkit-url-meta"><div><span>Origin</span><code>{{ urlResult.origin }}</code></div><div><span>Path</span><code>{{ urlResult.pathname }}</code></div><div><span>Hash</span><code>{{ urlResult.hash || '—' }}</code></div></div>
      <div class="toolkit-diff-heading"><strong>Query 参数</strong><span>{{ urlResult.rows.length }} 项</span></div>
      <div v-if="!urlResult.rows.length" class="toolkit-empty">没有查询参数。</div>
      <div v-else class="toolkit-param-list"><div v-for="(row, index) in urlResult.rows" :key="row.key + '-' + index"><code>{{ row.key }}</code><span>{{ row.value }}</span><button @click="copyText(row.key + '=' + row.value, '查询参数')"><Copy :size="13" /></button></div></div>
    </template>
    <div class="toolkit-divider"></div>
    <div class="toolkit-json-grid">
      <div class="toolkit-editor-pane"><label class="toolkit-label" for="url-encode">待编码文本</label><textarea id="url-encode" v-model="urlEncodeInput" class="toolkit-textarea short mono" placeholder="例如：中文 & a=b"></textarea><div class="toolkit-output-heading"><code class="toolkit-encoded-value">{{ urlEncoded }}</code><button class="button button-quiet" @click="copyText(urlEncoded, 'URL 编码结果')"><Copy :size="13" />复制</button></div></div>
      <div class="toolkit-editor-pane"><label class="toolkit-label" for="url-decode">待解码文本</label><textarea id="url-decode" v-model="urlDecodeInput" class="toolkit-textarea short mono" placeholder="%E4%B8%AD%E6%96%87"></textarea><div class="toolkit-output-heading"><code class="toolkit-encoded-value">{{ urlDecoded.error || urlDecoded.value }}</code><button class="button button-quiet" :disabled="!!urlDecoded.error" @click="copyText(urlDecoded.value, 'URL 解码结果')"><Copy :size="13" />复制</button></div></div>
    </div>
    <p v-if="urlDecoded.error" class="toolkit-error small">{{ urlDecoded.error }}</p>
    <div class="toolkit-action-row"><button class="button button-secondary" @click="resetURL"><RefreshCw :size="14" />清空全部</button><span v-if="urlMessage">{{ urlMessage }}</span></div>
  </div>
</template>
