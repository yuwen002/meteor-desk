<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowLeftRight, Braces, CheckCircle2, Clock3, Copy, FileJson, KeyRound, Link2, RefreshCw, ShieldAlert } from 'lucide-vue-next'

type ToolKey = 'timestamp' | 'json' | 'base64' | 'jwt' | 'url'
interface JsonDiffRow { path: string; kind: '新增' | '删除' | '修改'; before: unknown; after: unknown }
interface DateParts { year: number; month: number; day: number; hour: number; minute: number; second: number }

const activeTool = ref<ToolKey>('timestamp')
const copyStatus = ref('')

const toolTabs: { key: ToolKey; label: string; description: string }[] = [
  { key: 'timestamp', label: '时间戳 / 时区', description: '秒、毫秒与指定时区互转' },
  { key: 'json', label: 'JSON 工具', description: '格式化、压缩和结构化对比' },
  { key: 'base64', label: 'Base64', description: '支持中文等 UTF-8 文本' },
  { key: 'jwt', label: 'JWT 载荷', description: '查看 Header、Payload 与过期时间' },
  { key: 'url', label: 'URL / 参数', description: '解析 URL、编码和查询参数' },
]

const timestampInput = ref(String(Math.floor(Date.now() / 1000)))
const timestampUnit = ref<'seconds' | 'milliseconds'>('seconds')
const timeZone = ref('Asia/Shanghai')
const localDateTime = ref(getDateTimeInput(Date.now(), 'Asia/Shanghai'))

function partsInTimeZone(epochMs: number, zone: string): DateParts {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: zone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(new Date(epochMs))
  const value = (type: string) => Number(parts.find(part => part.type === type)?.value ?? 0)
  return { year: value('year'), month: value('month'), day: value('day'), hour: value('hour'), minute: value('minute'), second: value('second') }
}

function getDateTimeInput(epochMs: number, zone: string): string {
  const p = partsInTimeZone(epochMs, zone)
  return String(p.year).padStart(4, '0') + '-' + String(p.month).padStart(2, '0') + '-' + String(p.day).padStart(2, '0') + 'T' + String(p.hour).padStart(2, '0') + ':' + String(p.minute).padStart(2, '0')
}

function zonedDateTimeToEpoch(input: string, zone: string): number | null {
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/)
  if (!match) return null
  const wanted: DateParts = {
    year: Number(match[1]), month: Number(match[2]), day: Number(match[3]),
    hour: Number(match[4]), minute: Number(match[5]), second: Number(match[6] ?? 0),
  }
  const wantedAsUTC = Date.UTC(wanted.year, wanted.month - 1, wanted.day, wanted.hour, wanted.minute, wanted.second)
  const validation = new Date(wantedAsUTC)
  if (validation.getUTCFullYear() !== wanted.year || validation.getUTCMonth() + 1 !== wanted.month || validation.getUTCDate() !== wanted.day || wanted.hour > 23 || wanted.minute > 59 || wanted.second > 59) return null

  let candidate = wantedAsUTC
  for (let i = 0; i < 5; i++) {
    const actual = partsInTimeZone(candidate, zone)
    const actualAsUTC = Date.UTC(actual.year, actual.month - 1, actual.day, actual.hour, actual.minute, actual.second)
    const adjustment = wantedAsUTC - actualAsUTC
    candidate += adjustment
    if (adjustment === 0) break
  }
  const final = partsInTimeZone(candidate, zone)
  if (final.year !== wanted.year || final.month !== wanted.month || final.day !== wanted.day || final.hour !== wanted.hour || final.minute !== wanted.minute || final.second !== wanted.second) return null
  return candidate
}

const timestampInfo = computed(() => {
  const raw = timestampInput.value.trim()
  if (!raw) return { error: '请输入时间戳。', formatted: '', iso: '', seconds: '', milliseconds: '' }
  const n = Number(raw)
  if (!Number.isFinite(n)) return { error: '时间戳必须是有效数字。', formatted: '', iso: '', seconds: '', milliseconds: '' }
  const epochMs = timestampUnit.value === 'seconds' ? n * 1000 : n
  const date = new Date(epochMs)
  if (!Number.isFinite(date.getTime())) return { error: '该时间戳超出 JavaScript Date 支持的范围。', formatted: '', iso: '', seconds: '', milliseconds: '' }
  try {
    return {
      error: '',
      formatted: new Intl.DateTimeFormat('zh-CN', { timeZone: timeZone.value, dateStyle: 'full', timeStyle: 'long' }).format(date),
      iso: date.toISOString(),
      seconds: String(Math.floor(epochMs / 1000)),
      milliseconds: String(Math.trunc(epochMs)),
    }
  } catch {
    return { error: '无法识别该时区，请使用有效的 IANA 时区名称，例如 Asia/Shanghai。', formatted: '', iso: '', seconds: '', milliseconds: '' }
  }
})

const localDateEpoch = computed(() => {
  try {
    return zonedDateTimeToEpoch(localDateTime.value, timeZone.value)
  } catch {
    return null
  }
})
const localDateError = computed(() => {
  if (!localDateTime.value) return '请选择日期和时间。'
  if (localDateEpoch.value === null) return '日期无效，或该时区的本地时间不存在（例如夏令时跳时）。'
  return ''
})

const jsonInput = ref('')
const jsonCompareInput = ref('')
const jsonOutput = ref('')
const jsonMessage = ref('')
function parseJSON(value: string): { valid: true; value: unknown } | { valid: false; error: string } {
  try { return { valid: true, value: JSON.parse(value) as unknown } }
  catch (error) { return { valid: false, error: error instanceof Error ? error.message : String(error) } }
}
function prettyJSON(source: string, compact = false): string {
  const parsed = parseJSON(source)
  if (!parsed.valid) throw new Error(parsed.error)
  return JSON.stringify(parsed.value, null, compact ? undefined : 2)
}
function formatJSON(target: 'left' | 'right' | 'compact') {
  jsonMessage.value = ''
  const source = target === 'right' ? jsonCompareInput.value : jsonInput.value
  try {
    const formatted = prettyJSON(source, target === 'compact')
    if (target === 'right') jsonCompareInput.value = formatted
    else jsonInput.value = formatted
    jsonOutput.value = formatted
    jsonMessage.value = target === 'compact' ? 'JSON 已压缩。' : 'JSON 格式有效，已完成格式化。'
  } catch (error) {
    jsonMessage.value = 'JSON 解析失败：' + String(error)
  }
}
function flattenJSON(value: unknown, path = '$', target = new Map<string, unknown>()): Map<string, unknown> {
  if (Array.isArray(value)) {
    if (!value.length) target.set(path, [])
    value.forEach((item, index) => flattenJSON(item, path + '[' + index + ']', target))
  } else if (value !== null && typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>)
    if (!entries.length) target.set(path, {})
    entries.forEach(([key, child]) => flattenJSON(child, path === '$' ? key : path + '.' + key, target))
  } else {
    target.set(path, value)
  }
  return target
}
const jsonComparison = computed((): { error: string; rows: JsonDiffRow[] } => {
  if (!jsonInput.value.trim() || !jsonCompareInput.value.trim()) return { error: '在左右两侧输入 JSON 后，即可按字段路径比较差异。', rows: [] }
  const before = parseJSON(jsonInput.value)
  if (!before.valid) return { error: '左侧 JSON 无效：' + before.error, rows: [] }
  const after = parseJSON(jsonCompareInput.value)
  if (!after.valid) return { error: '右侧 JSON 无效：' + after.error, rows: [] }
  try {
    const a = flattenJSON(before.value)
    const b = flattenJSON(after.value)
    if (a.size + b.size > 20000) return { error: '字段数量过多，当前版本最多对比约 20,000 个叶子字段。', rows: [] }
    const paths = Array.from(new Set([...a.keys(), ...b.keys()])).sort()
    const rows: JsonDiffRow[] = []
    for (const path of paths) {
      const hasBefore = a.has(path)
      const hasAfter = b.has(path)
      if (!hasBefore) rows.push({ path, kind: '新增', before: undefined, after: b.get(path) })
      else if (!hasAfter) rows.push({ path, kind: '删除', before: a.get(path), after: undefined })
      else if (JSON.stringify(a.get(path)) !== JSON.stringify(b.get(path))) rows.push({ path, kind: '修改', before: a.get(path), after: b.get(path) })
    }
    return { error: '', rows }
  } catch (error) {
    return { error: '对比失败：' + String(error), rows: [] }
  }
})
function showJSONValue(value: unknown): string {
  if (value === undefined) return '—'
  const result = JSON.stringify(value)
  return result === undefined ? String(value) : result
}

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

async function copyText(value: string, label = '内容') {
  copyStatus.value = ''
  if (!value) {
    copyStatus.value = '没有可复制的' + label + '。'
    return
  }
  try {
    await navigator.clipboard.writeText(value)
    copyStatus.value = label + '已复制。'
  } catch {
    copyStatus.value = '复制失败，请检查桌面应用的剪贴板权限。'
  }
}
function resetJSON() {
  jsonInput.value = ''
  jsonCompareInput.value = ''
  jsonOutput.value = ''
  jsonMessage.value = ''
}
</script>

<template>
  <section class="page-content toolkit-page">
    <div class="page-heading">
      <div>
        <div class="eyebrow"><span class="eyebrow-line"></span>开发者效率 / 本地工具</div>
        <h1>开发工具<span class="title-period">。</span></h1>
        <p class="page-subtitle">常用数据转换与排查工具，全部在本机运行，不会上传输入内容。</p>
      </div>
    </div>

    <div class="toolkit-tabs" role="tablist" aria-label="开发工具">
      <button v-for="tool in toolTabs" :key="tool.key" role="tab" :aria-selected="activeTool === tool.key" class="toolkit-tab" :class="{ active: activeTool === tool.key }" @click="activeTool = tool.key">
        <Clock3 v-if="tool.key === 'timestamp'" :size="16" />
        <Braces v-else-if="tool.key === 'json'" :size="16" />
        <FileJson v-else-if="tool.key === 'base64'" :size="16" />
        <KeyRound v-else-if="tool.key === 'jwt'" :size="16" />
        <Link2 v-else :size="16" />
        <span>{{ tool.label }}</span>
      </button>
    </div>

    <div v-if="copyStatus" class="toolkit-feedback" role="status"><CheckCircle2 :size="14" />{{ copyStatus }}<button type="button" @click="copyStatus = ''">关闭</button></div>

    <div v-if="activeTool === 'timestamp'" class="toolkit-card">
      <div class="toolkit-card-heading"><div><h2>时间戳与时区转换</h2><p>支持 Unix 秒、毫秒与指定时区的本地日期时间互转。</p></div><Clock3 :size="18" /></div>
      <div class="toolkit-grid-two">
        <div>
          <label class="toolkit-label" for="timestamp-value">Unix 时间戳</label>
          <div class="toolkit-input-row"><input id="timestamp-value" v-model="timestampInput" class="toolkit-input mono" inputmode="decimal" placeholder="例如 1735689600" /><select v-model="timestampUnit" class="toolkit-select" aria-label="时间戳单位"><option value="seconds">秒（s）</option><option value="milliseconds">毫秒（ms）</option></select></div>
          <p class="toolkit-help">秒和毫秒可切换，结果会同步更新。</p>
          <label class="toolkit-label" for="timezone-name">IANA 时区名称</label>
          <input id="timezone-name" v-model="timeZone" class="toolkit-input mono" placeholder="Asia/Shanghai" list="common-timezones" />
          <datalist id="common-timezones"><option value="Asia/Shanghai" /><option value="Asia/Singapore" /><option value="UTC" /><option value="America/Los_Angeles" /><option value="Europe/London" /><option value="Asia/Tokyo" /></datalist>
          <div class="toolkit-action-row"><button class="button button-secondary" @click="timestampInput = String(Math.floor(Date.now() / (timestampUnit === 'seconds' ? 1000 : 1)))"><RefreshCw :size="14" />当前时间</button><button class="button button-quiet" @click="copyText(timestampInfo.seconds, '秒级时间戳')"><Copy :size="14" />复制秒级时间戳</button></div>
        </div>
        <div class="toolkit-result-panel">
          <div v-if="timestampInfo.error" class="toolkit-error">{{ timestampInfo.error }}</div>
          <template v-else>
            <span class="toolkit-result-kicker">指定时区时间</span><strong class="toolkit-time-result">{{ timestampInfo.formatted }}</strong>
            <div class="toolkit-result-line"><span>Unix 秒</span><code>{{ timestampInfo.seconds }}</code><button aria-label="复制秒级时间戳" @click="copyText(timestampInfo.seconds, '秒级时间戳')"><Copy :size="13" /></button></div>
            <div class="toolkit-result-line"><span>Unix 毫秒</span><code>{{ timestampInfo.milliseconds }}</code><button aria-label="复制毫秒时间戳" @click="copyText(timestampInfo.milliseconds, '毫秒时间戳')"><Copy :size="13" /></button></div>
            <div class="toolkit-result-line"><span>UTC ISO 8601</span><code>{{ timestampInfo.iso }}</code><button aria-label="复制 ISO 时间" @click="copyText(timestampInfo.iso, 'ISO 时间')"><Copy :size="13" /></button></div>
          </template>
        </div>
      </div>
      <div class="toolkit-divider"></div>
      <div class="toolkit-card-heading compact"><div><h3>本地日期时间转时间戳</h3><p>输入框里的日期时间将按上方时区解释。</p></div></div>
      <div class="toolkit-grid-two bottom-converter">
        <div><label class="toolkit-label" for="local-datetime">日期和时间（{{ timeZone || '请填写时区' }}）</label><input id="local-datetime" v-model="localDateTime" type="datetime-local" class="toolkit-input" step="1" /><p v-if="localDateError" class="toolkit-error small">{{ localDateError }}</p><p v-else class="toolkit-help">切换时区不会改写输入的墙上时间；重新选择即可换算。</p></div>
        <div class="toolkit-result-panel compact-result"><template v-if="!localDateError"><div class="toolkit-result-line"><span>Unix 秒</span><code>{{ Math.floor((localDateEpoch ?? 0) / 1000) }}</code><button @click="copyText(String(Math.floor((localDateEpoch ?? 0) / 1000)), '秒级时间戳')"><Copy :size="13" /></button></div><div class="toolkit-result-line"><span>Unix 毫秒</span><code>{{ localDateEpoch }}</code><button @click="copyText(String(localDateEpoch), '毫秒时间戳')"><Copy :size="13" /></button></div><div class="toolkit-result-line"><span>对应 UTC</span><code>{{ new Date(localDateEpoch ?? 0).toISOString() }}</code><button @click="copyText(new Date(localDateEpoch ?? 0).toISOString(), 'UTC 时间')"><Copy :size="13" /></button></div></template></div>
      </div>
    </div>

    <div v-else-if="activeTool === 'json'" class="toolkit-card">
      <div class="toolkit-card-heading"><div><h2>JSON 格式化与结构化对比</h2><p>格式化、压缩 JSON，并按字段路径识别新增、删除和修改项。</p></div><Braces :size="18" /></div>
      <div class="toolkit-action-row json-global-actions"><button class="button button-primary" @click="formatJSON('left')"><Braces :size="14" />格式化左侧</button><button class="button button-secondary" @click="formatJSON('right')"><Braces :size="14" />格式化右侧</button><button class="button button-secondary" @click="formatJSON('compact')">压缩左侧</button><button class="button button-quiet" @click="copyText(jsonOutput || jsonInput, 'JSON')"><Copy :size="14" />复制最近输出</button><button class="button button-quiet" @click="resetJSON">清空</button></div>
      <div class="toolkit-json-grid">
        <div class="toolkit-editor-pane"><label class="toolkit-label" for="json-left">原始 JSON（左侧）</label><textarea id="json-left" v-model="jsonInput" class="toolkit-textarea mono" spellcheck="false" placeholder='{"name":"MeteorDesk","version":1}'></textarea></div>
        <div class="toolkit-editor-pane"><label class="toolkit-label" for="json-right">对比 JSON（右侧）</label><textarea id="json-right" v-model="jsonCompareInput" class="toolkit-textarea mono" spellcheck="false" placeholder='{"name":"MeteorDesk","version":2}'></textarea></div>
      </div>
      <p v-if="jsonMessage" class="toolkit-inline-message">{{ jsonMessage }}</p>
      <div class="toolkit-diff-heading"><strong>结构化差异</strong><span v-if="!jsonComparison.error">发现 {{ jsonComparison.rows.length }} 项差异</span></div>
      <p v-if="jsonComparison.error" class="toolkit-help">{{ jsonComparison.error }}</p>
      <div v-else-if="!jsonComparison.rows.length" class="toolkit-empty"><CheckCircle2 :size="17" />左右 JSON 内容一致。</div>
      <div v-else class="toolkit-diff-list"><div v-for="row in jsonComparison.rows" :key="row.path" class="toolkit-diff-row" :class="row.kind"><div class="toolkit-diff-path"><span>{{ row.kind }}</span><code>{{ row.path }}</code></div><div class="toolkit-diff-values"><code>{{ showJSONValue(row.before) }}</code><ArrowLeftRight :size="13" /><code>{{ showJSONValue(row.after) }}</code></div></div></div>
      <p class="toolkit-help">比较基于解析后的字段路径；数组按下标比较。大于约 20,000 个叶子字段时会停止差异展开。</p>
    </div>

    <div v-else-if="activeTool === 'base64'" class="toolkit-card">
      <div class="toolkit-card-heading"><div><h2>Base64 编码与解码</h2><p>按 UTF-8 处理文本，支持中文和常见多字节字符。</p></div><FileJson :size="18" /></div>
      <div class="toolkit-json-grid">
        <div class="toolkit-editor-pane"><label class="toolkit-label" for="base64-source">原始文本</label><textarea id="base64-source" v-model="base64Source" class="toolkit-textarea tall" placeholder="输入要编码的文本……"></textarea><div class="toolkit-action-row"><button class="button button-primary" @click="encodeBase64">编码为 Base64 <ArrowLeftRight :size="14" /></button><button class="button button-quiet" @click="base64Source = ''; base64Encoded = ''; base64Decoded = ''; base64Message = ''">清空</button></div></div>
        <div class="toolkit-editor-pane"><label class="toolkit-label" for="base64-result">Base64 字符串</label><textarea id="base64-result" v-model="base64Encoded" class="toolkit-textarea tall mono" placeholder="编码结果或粘贴待解码字符串……"></textarea><div class="toolkit-action-row"><button class="button button-primary" @click="decodeBase64">解码为文本 <ArrowLeftRight :size="14" /></button><button class="button button-quiet" @click="copyText(base64Encoded, 'Base64 字符串')"><Copy :size="14" />复制</button></div></div>
      </div>
      <div v-if="base64Message" class="toolkit-inline-message">{{ base64Message }}</div>
      <div v-if="base64Decoded !== ''" class="toolkit-output-block"><div class="toolkit-output-heading"><strong>解码结果</strong><button class="button button-quiet" @click="copyText(base64Decoded, '解码文本')"><Copy :size="13" />复制</button></div><pre>{{ base64Decoded }}</pre></div>
    </div>

    <div v-else-if="activeTool === 'jwt'" class="toolkit-card">
      <div class="toolkit-card-heading"><div><h2>JWT Payload 查看器</h2><p>只解码 JWT 的 Header 和 Payload，不进行签名验证。</p></div><KeyRound :size="18" /></div>
      <label class="toolkit-label" for="jwt-token">JWT Token</label><textarea id="jwt-token" v-model="jwtInput" class="toolkit-textarea jwt-input mono" spellcheck="false" placeholder="粘贴由 Header.Payload.Signature 组成的 JWT……"></textarea>
      <div class="toolkit-security-note"><ShieldAlert :size="16" /><span>安全提示：这里不会验证签名、issuer 或 audience。Payload 只是 Base64URL 编码，不应视为可信声明；不要把私钥或敏感 Token 粘贴到不可信环境。</span></div>
      <p v-if="jwtResult.error" class="toolkit-help">{{ jwtResult.error }}</p>
      <template v-else-if="jwtResult.payload">
        <div class="toolkit-jwt-status" :class="jwtResult.state"><CheckCircle2 v-if="jwtResult.state === 'valid'" :size="15" /><ShieldAlert v-else-if="jwtResult.state === 'expired'" :size="15" /><KeyRound v-else :size="15" />{{ jwtResult.expires }}</div>
        <div class="toolkit-json-grid"><div class="toolkit-editor-pane"><div class="toolkit-output-heading"><label class="toolkit-label">Header</label><button class="button button-quiet" @click="copyText(jwtResult.header, 'JWT Header')"><Copy :size="13" />复制</button></div><pre class="toolkit-json-output">{{ jwtResult.header }}</pre></div><div class="toolkit-editor-pane"><div class="toolkit-output-heading"><label class="toolkit-label">Payload</label><button class="button button-quiet" @click="copyText(jwtResult.payload, 'JWT Payload')"><Copy :size="13" />复制</button></div><pre class="toolkit-json-output">{{ jwtResult.payload }}</pre></div></div>
      </template>
    </div>

    <div v-else class="toolkit-card">
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
      <div class="toolkit-action-row"><button class="button button-secondary" @click="urlMessage = ''; urlInput = ''; urlEncodeInput = ''; urlDecodeInput = ''"><RefreshCw :size="14" />清空全部</button><span v-if="urlMessage">{{ urlMessage }}</span></div>
    </div>

    <div class="toolkit-footer-note"><ShieldAlert :size="15" /><span>所有转换均在桌面端本地完成。JWT 查看器仅负责解码，不验证令牌签名，也不代表其内容可信。</span></div>
  </section>
</template>

<style scoped>
.toolkit-page{max-width:1320px}
.toolkit-tabs{display:flex;gap:6px;flex-wrap:wrap;margin:0 0 15px;padding:6px;border:1px solid #29313f;border-radius:8px;background:#111721}
.toolkit-tab{display:flex;align-items:center;gap:8px;border:1px solid transparent;border-radius:5px;padding:10px 12px;background:transparent;color:#8291a4;font-size:10px;white-space:nowrap}
.toolkit-tab:hover{background:#19212d;color:#d4e1ed}
.toolkit-tab.active{border-color:rgba(134,247,197,.2);background:rgba(134,247,197,.08);color:#a0f3cd}
.toolkit-card{min-width:0;border:1px solid #29313f;border-radius:9px;background:linear-gradient(150deg,rgba(20,26,37,.98),rgba(16,21,31,.98));padding:19px}
.toolkit-card-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:15px;margin:0 0 20px;color:#8feac2}
.toolkit-card-heading h2{margin:0 0 6px;color:#e0e8f1;font-size:13px;font-weight:650}
.toolkit-card-heading h3{margin:0 0 5px;color:#dce5ee;font-size:11px;font-weight:650}
.toolkit-card-heading p{margin:0;color:#7e8da1;font-size:10px;line-height:1.6}
.toolkit-card-heading.compact{margin-bottom:12px}
.toolkit-grid-two,.toolkit-json-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:18px}
.toolkit-label{display:block;margin:0 0 7px;color:#9aa9ba;font-size:10px;font-weight:600}
.toolkit-input-row{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:7px}
.toolkit-input,.toolkit-select,.toolkit-textarea{width:100%;min-width:0;border:1px solid #303b4a;border-radius:5px;background:#0d131c;color:#dbe4ed;padding:10px 11px;font-size:11px;outline:none}
.toolkit-input:focus,.toolkit-select:focus,.toolkit-textarea:focus{border-color:#5eaa8a;box-shadow:0 0 0 2px rgba(110,210,165,.06)}
.toolkit-select{width:auto;max-width:130px;color:#b4c1d0}
.toolkit-help{margin:8px 0 14px;color:#718197;font-size:9px;line-height:1.7}
.toolkit-help:last-child{margin-bottom:0}
.toolkit-help.small,.toolkit-error.small{margin-top:7px}
.toolkit-error{color:#ffaaa1;font-size:10px;line-height:1.7;overflow-wrap:anywhere}
.toolkit-action-row{display:flex;align-items:center;gap:7px;flex-wrap:wrap;margin-top:13px}
.toolkit-action-row .button{height:32px;padding:0 10px;font-size:10px}
.toolkit-result-panel{min-width:0;border:1px solid #2b3943;border-radius:7px;background:rgba(9,15,21,.7);padding:15px}
.toolkit-result-kicker{display:block;margin-bottom:8px;color:#6f8d83;font:9px ui-monospace,monospace;letter-spacing:.6px;text-transform:uppercase}
.toolkit-time-result{display:block;margin-bottom:15px;color:#c8f5df;font-size:14px;line-height:1.6;font-weight:600;overflow-wrap:anywhere}
.toolkit-result-line{display:grid;grid-template-columns:78px minmax(0,1fr) 25px;align-items:start;gap:8px;padding:9px 0;border-top:1px solid rgba(46,60,70,.58)}
.toolkit-result-line>span{color:#738398;font-size:9px;padding-top:2px}
.toolkit-result-line code{color:#b8d8c9;font:10px/1.6 ui-monospace,monospace;overflow-wrap:anywhere}
.toolkit-result-line button{display:grid;place-items:center;padding:4px;border:0;background:transparent;color:#7d9a8e;border-radius:3px}
.toolkit-result-line button:hover{background:#1a2827;color:#c9f4dd}
.toolkit-divider{height:1px;margin:22px 0;background:#29313d}
.compact-result{padding:9px 13px}
.compact-result .toolkit-result-line:first-child{border-top:0}
.bottom-converter{align-items:start}
.toolkit-feedback{display:flex;align-items:center;gap:8px;margin:0 0 12px;border:1px solid rgba(134,247,197,.18);background:rgba(134,247,197,.05);border-radius:5px;padding:9px 11px;color:#a3edce;font-size:10px}
.toolkit-feedback button{margin-left:auto;border:0;background:transparent;color:#859c91;font-size:9px}
.json-global-actions{margin:-4px 0 14px}
.toolkit-editor-pane{min-width:0}
.toolkit-textarea{min-height:220px;resize:vertical;line-height:1.65;font-size:11px}
.toolkit-textarea.tall{min-height:310px}
.toolkit-textarea.short{min-height:84px}
.toolkit-textarea.jwt-input{min-height:105px;margin-bottom:12px}
.toolkit-textarea.url-input{min-height:90px}
.mono,.toolkit-json-output,.toolkit-diff-values,.toolkit-param-list code,.toolkit-encoded-value{font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
.toolkit-inline-message{margin:12px 0;color:#91dcb8;font-size:10px;overflow-wrap:anywhere}
.toolkit-diff-heading{display:flex;justify-content:space-between;gap:12px;margin:20px 0 9px;color:#d4dfea;font-size:10px}
.toolkit-diff-heading span{color:#77889a;font-weight:400}
.toolkit-diff-list{display:flex;flex-direction:column;border:1px solid #2a3441;border-radius:6px;overflow:hidden}
.toolkit-diff-row{padding:10px 11px;border-bottom:1px solid #26303c;background:rgba(10,15,22,.43)}
.toolkit-diff-row:last-child{border-bottom:0}
.toolkit-diff-row.新增 .toolkit-diff-path>span{color:#9ae8c0;background:rgba(70,162,119,.12)}
.toolkit-diff-row.删除 .toolkit-diff-path>span{color:#ffb2a9;background:rgba(170,73,73,.15)}
.toolkit-diff-row.修改 .toolkit-diff-path>span{color:#e9ce8f;background:rgba(176,140,62,.12)}
.toolkit-diff-path{display:flex;align-items:center;gap:9px;margin-bottom:7px;min-width:0}
.toolkit-diff-path>span{padding:3px 5px;border-radius:3px;font-size:9px;flex-shrink:0}
.toolkit-diff-path code{color:#a1cdb9;font:10px ui-monospace,monospace;overflow-wrap:anywhere}
.toolkit-diff-values{display:grid;grid-template-columns:minmax(0,1fr) 18px minmax(0,1fr);align-items:start;gap:8px;color:#8291a4;font-size:10px}
.toolkit-diff-values code{overflow-wrap:anywhere;white-space:pre-wrap;color:#b3c1d0}
.toolkit-diff-values svg{margin-top:2px;color:#718196}
.toolkit-empty{display:flex;align-items:center;gap:7px;padding:15px 12px;color:#90e7bc;font-size:10px}
.toolkit-output-block{margin-top:13px;border:1px solid #293340;border-radius:6px;overflow:hidden}
.toolkit-output-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;min-width:0}
.toolkit-output-heading strong{color:#cbd8e5;font-size:10px}
.toolkit-output-heading .toolkit-label{margin:0}
.toolkit-output-heading .button{height:27px;padding:0 8px;font-size:9px}
.toolkit-output-block>pre,.toolkit-json-output{display:block;max-height:300px;overflow:auto;margin:0;padding:12px;background:#0c1118;color:#b9d9cb;font:10px/1.7 ui-monospace,monospace;white-space:pre-wrap;overflow-wrap:anywhere}
.toolkit-security-note{display:flex;align-items:flex-start;gap:9px;padding:11px;border:1px solid rgba(221,177,99,.2);border-radius:5px;background:rgba(110,78,27,.09);color:#cdb783;font-size:9px;line-height:1.7;margin:12px 0}
.toolkit-security-note svg{flex-shrink:0;margin-top:1px}
.toolkit-jwt-status{display:flex;align-items:center;gap:7px;margin:13px 0;padding:10px 12px;border:1px solid #313a45;border-radius:5px;background:#111721;color:#b0bfce;font-size:10px}
.toolkit-jwt-status.valid{color:#99e4b9;border-color:rgba(134,247,197,.2)}
.toolkit-jwt-status.expired{color:#ffaca3;border-color:rgba(248,113,113,.22)}
.toolkit-url-meta{display:grid;grid-template-columns:1fr 1fr 1fr;gap:7px;margin:13px 0}
.toolkit-url-meta>div{min-width:0;display:flex;flex-direction:column;gap:6px;padding:10px;border:1px solid #293442;border-radius:5px;background:#111721}
.toolkit-url-meta span{color:#75869b;font-size:9px}
.toolkit-url-meta code{color:#bbd6c9;font:10px/1.5 ui-monospace,monospace;overflow-wrap:anywhere}
.toolkit-param-list{border:1px solid #293442;border-radius:5px;overflow:hidden}
.toolkit-param-list>div{display:grid;grid-template-columns:minmax(90px,.7fr) minmax(0,1.4fr) 25px;align-items:start;gap:10px;padding:9px 11px;border-bottom:1px solid #26303c;background:rgba(10,15,22,.35)}
.toolkit-param-list>div:last-child{border-bottom:0}
.toolkit-param-list code{font-size:10px;color:#93dcb8;overflow-wrap:anywhere}
.toolkit-param-list span{font:10px/1.6 ui-monospace,monospace;color:#b9c8d8;overflow-wrap:anywhere}
.toolkit-param-list button{border:0;background:transparent;color:#82998f}
.toolkit-encoded-value{flex:1;min-width:0;overflow-wrap:anywhere;white-space:pre-wrap;color:#b9d9cb;font-size:10px;line-height:1.6}
.toolkit-footer-note{display:flex;align-items:flex-start;gap:8px;margin-top:15px;padding:0 3px;color:#748499;font-size:9px;line-height:1.7}
.toolkit-footer-note svg{flex-shrink:0;color:#88c6a9}
@media(max-width:980px){.toolkit-grid-two,.toolkit-json-grid{grid-template-columns:1fr}.toolkit-textarea{min-height:180px}.toolkit-url-meta{grid-template-columns:1fr}.toolkit-card{padding:14px}}
@media(max-width:680px){.toolkit-tabs{display:grid;grid-template-columns:1fr 1fr}.toolkit-tab{justify-content:flex-start}.toolkit-diff-values{grid-template-columns:minmax(0,1fr)}.toolkit-diff-values svg{display:none}.toolkit-result-line{grid-template-columns:70px minmax(0,1fr) 25px}}
</style>
