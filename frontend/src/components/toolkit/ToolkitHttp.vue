<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Copy, Download, Send } from 'lucide-vue-next'
import { parseRequestHeaders } from '../../utils/httpRequest'
import { buildCurlCommand, parseCurlCommand } from '../../utils/curl'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

const { copyText } = useCopyFeedback()

const httpMethods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']
const httpMethod = ref('GET'), httpURL = ref(''), httpHeaders = ref('{\n  "Accept": "application/json"\n}'), httpBody = ref(''), httpResponse = ref<HttpResponse | null>(null), httpError = ref(''), httpBusy = ref(false)
const httpTimeoutMs = ref(30000), httpFollowRedirects = ref(true), httpInsecureSkipVerify = ref(false)

interface HTTPHistoryEntry { method: string; url: string; headers: string; body: string; at: string }
const httpHistoryKey = 'meteordesk.httpHistory.v1'
const httpHistory = ref<HTTPHistoryEntry[]>([])

function loadHTTPHistory() {
  try {
    const parsed: unknown = JSON.parse(window.localStorage.getItem(httpHistoryKey) ?? '[]')
    httpHistory.value = Array.isArray(parsed) ? parsed.filter((item): item is HTTPHistoryEntry => !!item && typeof item.url === 'string' && typeof item.method === 'string').slice(0, 10) : []
  } catch { httpHistory.value = [] }
}
function saveHTTPHistory() {
  try { window.localStorage.setItem(httpHistoryKey, JSON.stringify(httpHistory.value.slice(0, 10))) } catch { /* 历史仅在本机保留 */ }
}
function recordHTTPHistory(entry: HTTPHistoryEntry) {
  httpHistory.value = [entry, ...httpHistory.value.filter(item => !(item.method === entry.method && item.url === entry.url && item.headers === entry.headers && item.body === entry.body))].slice(0, 10)
  saveHTTPHistory()
}
function applyHTTPHistory(entry: HTTPHistoryEntry) {
  if (httpBusy.value) return
  httpMethod.value = entry.method
  httpURL.value = entry.url
  httpHeaders.value = entry.headers
  httpBody.value = entry.body
}
function clearHTTPHistory() {
  httpHistory.value = []
  saveHTTPHistory()
}
onMounted(loadHTTPHistory)

const httpResponseHeaders = computed(() => httpResponse.value ? Object.entries(httpResponse.value.headers).sort(([a], [b]) => a.localeCompare(b)) : [])

async function sendHTTPRequest() {
  const api = window.go?.main?.App
  httpError.value = ''; httpResponse.value = null
  if (!api) { httpError.value = '未检测到桌面桥接服务，请通过 wails dev 启动。'; return }
  if (!httpURL.value.trim()) { httpError.value = '请输入请求 URL 后再发送。'; return }
  let headers: Record<string, string>
  try { headers = parseRequestHeaders(httpHeaders.value) } catch (error) { httpError.value = error instanceof Error ? error.message : String(error); return }
  const request: HttpRequest = { method: httpMethod.value, url: httpURL.value.trim(), headers, body: httpBody.value, timeoutMs: httpTimeoutMs.value, followRedirects: httpFollowRedirects.value, insecureSkipVerify: httpInsecureSkipVerify.value }
  recordHTTPHistory({ method: request.method, url: request.url, headers: httpHeaders.value, body: httpBody.value, at: new Date().toISOString() })
  httpBusy.value = true
  try { httpResponse.value = await api.SendHTTPRequest(request) } catch (error) { httpError.value = error instanceof Error ? error.message : String(error) } finally { httpBusy.value = false }
}

const curlInput = ref('')
const curlMessage = ref('')
function importCurl() {
  curlMessage.value = ''
  const result = parseCurlCommand(curlInput.value)
  if (!result.ok) { curlMessage.value = result.error; return }
  httpMethod.value = result.request.method
  httpURL.value = result.request.url
  httpHeaders.value = Object.keys(result.request.headers).length ? JSON.stringify(result.request.headers, null, 2) : '{}'
  httpBody.value = result.request.body
  httpError.value = ''
  httpResponse.value = null
  curlMessage.value = '已导入 ' + result.request.method + ' 请求' + (result.warnings.length ? '；已忽略：' + result.warnings.join('、') : '')
}
async function copyCurl() {
  curlMessage.value = ''
  const url = httpURL.value.trim()
  if (!url) { curlMessage.value = '先填写请求 URL，再生成 cURL 命令。'; return }
  let headers: Record<string, string>
  try { headers = parseRequestHeaders(httpHeaders.value) } catch (error) { curlMessage.value = error instanceof Error ? error.message : String(error); return }
  const command = buildCurlCommand({ method: httpMethod.value, url, headers, body: httpBody.value })
  curlInput.value = command
  await copyText(command, 'cURL 命令')
}
function resetCurl() {
  curlInput.value = ''
  curlMessage.value = ''
}
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>HTTP 调试器</h2><p>请求由你显式发送、在桌面端本机执行；不会自动联网，也不会访问未填写的地址。</p></div><Send :size="18" /></div>
    <div class="http-curl">
      <div class="toolkit-output-heading"><label class="toolkit-label" for="http-curl">cURL 命令</label><span>粘贴导入，或把当前请求生成命令</span></div>
      <textarea id="http-curl" v-model="curlInput" class="toolkit-textarea short curl-textarea mono" spellcheck="false" placeholder="curl -X POST 'https://api.example.com/items' -H 'Content-Type: application/json' -d '{&quot;name&quot;:&quot;meteor&quot;}'" @keydown.ctrl.enter="importCurl"></textarea>
      <div class="toolkit-action-row"><button class="button button-secondary" @click="importCurl"><Download :size="14" />导入到表单</button><button class="button button-quiet" @click="copyCurl"><Copy :size="14" />生成并复制</button><button class="button button-quiet" @click="resetCurl">清空命令</button><span class="toolkit-help curl-hint">支持 -X/-H/-d、续行与引号；-A、-b、-e 会转成对应请求头。</span></div>
      <p v-if="curlMessage" class="toolkit-help curl-message">{{ curlMessage }}</p>
    </div>
    <div class="http-request-line"><select v-model="httpMethod" class="toolkit-select" aria-label="HTTP 方法"><option v-for="method in httpMethods" :key="method">{{ method }}</option><option v-if="!httpMethods.includes(httpMethod)" :value="httpMethod">{{ httpMethod }}</option></select><input v-model="httpURL" class="toolkit-input mono" aria-label="请求 URL" placeholder="https://api.example.com/v1/health" /><button class="button button-primary" :disabled="httpBusy" @click="sendHTTPRequest"><span v-if="httpBusy" class="spinner"></span><Send v-else :size="14" />{{ httpBusy ? '发送中' : '发送' }}</button></div>
    <div class="http-options">
      <label><input v-model="httpFollowRedirects" type="checkbox" />自动跟随重定向</label>
      <label><input v-model="httpInsecureSkipVerify" type="checkbox" />跳过 TLS 证书校验</label>
      <label>超时<select v-model.number="httpTimeoutMs" class="toolkit-select" aria-label="请求超时"><option :value="5000">5 秒</option><option :value="10000">10 秒</option><option :value="30000">30 秒</option><option :value="60000">60 秒</option><option :value="120000">120 秒</option></select></label>
    </div>
    <p class="toolkit-help">关闭跟随重定向时会直接返回 3xx 响应；跳过证书校验仅用于本机自签证书调试。</p>
    <div class="toolkit-json-grid http-editors"><div class="toolkit-editor-pane"><label class="toolkit-label" for="http-headers">请求 Header（JSON）</label><textarea id="http-headers" v-model="httpHeaders" class="toolkit-textarea short mono" spellcheck="false"></textarea></div><div class="toolkit-editor-pane"><label class="toolkit-label" for="http-body">请求 Body</label><textarea id="http-body" v-model="httpBody" class="toolkit-textarea short mono" spellcheck="false" placeholder="可选，请求正文最大 2 MiB"></textarea></div></div>
    <p v-if="httpError" class="toolkit-error">{{ httpError }}</p>
    <template v-else-if="httpResponse"><div class="http-response-summary"><strong :class="{ error: httpResponse.status >= 400 }">{{ httpResponse.status }} {{ httpResponse.statusText }}</strong><span>{{ httpResponse.durationMs }} ms</span><span>接收 {{ httpResponse.sizeBytes.toLocaleString() }} B</span><span v-if="httpResponse.redirects">重定向 {{ httpResponse.redirects }} 次</span><button class="button button-quiet" @click="copyText(httpResponse.body, '响应正文')"><Copy :size="13" />复制正文</button></div><p v-if="httpResponse.finalUrl && httpResponse.finalUrl !== httpURL.trim()" class="toolkit-help">最终地址：<code class="http-final-url">{{ httpResponse.finalUrl }}</code></p><p v-if="httpResponse.binary" class="toolkit-help">响应正文不是有效 UTF-8（可能是二进制内容），预览中的异常字节已显示为替换字符。</p><p v-if="httpResponse.bodyTruncated" class="toolkit-help">响应正文超过 2 MiB，界面仅保留前 2 MiB；上方字节数为实际接收量。</p><div class="toolkit-json-grid"><div class="toolkit-editor-pane"><div class="toolkit-output-heading"><label class="toolkit-label">响应 Header</label><span>{{ httpResponseHeaders.length }} 项</span></div><div class="toolkit-param-list"><div v-for="([key, value]) in httpResponseHeaders" :key="key"><code>{{ key }}</code><span>{{ value }}</span></div></div></div><div class="toolkit-editor-pane"><label class="toolkit-label">响应 Body</label><pre class="toolkit-json-output http-response-body">{{ httpResponse.body }}</pre></div></div></template>
    <div v-if="httpHistory.length" class="http-history">
      <div class="toolkit-output-heading"><strong>最近请求（仅保存在本机）</strong><button class="button button-quiet" @click="clearHTTPHistory">清空历史</button></div>
      <div class="http-history-list">
        <button v-for="entry in httpHistory" :key="entry.at + entry.url" type="button" class="http-history-item" :disabled="httpBusy" @click="applyHTTPHistory(entry)"><span>{{ entry.method }}</span><code>{{ entry.url }}</code><time>{{ new Date(entry.at).toLocaleString('zh-CN') }}</time></button>
      </div>
    </div>
  </div>
</template>
