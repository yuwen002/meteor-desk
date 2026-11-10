<script setup lang="ts">
import { ref } from 'vue'
import { Copy, FileJson, Files } from 'lucide-vue-next'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

const { copyText } = useCopyFeedback()

const leftText = ref('<?xml version="1.0" encoding="UTF-8"?>\n<service id="svc-001">\n  <name>meteor-desk</name>\n  <ports>\n    <http>8080</http>\n    <grpc>9090</grpc>\n  </ports>\n  <features>\n    <item>log-analysis</item>\n    <item>process-management</item>\n    <item>dev-toolkit</item>\n  </features>\n</service>')
const rightText = ref('')
const message = ref('')

interface JsonNode {
  '#text'?: string
  [key: string]: unknown
}

function xmlNodeToJson(node: Node): unknown {
  if (node.nodeType === Node.TEXT_NODE) {
    const t = node.textContent?.trim() ?? ''
    return t || undefined
  }
  if (node.nodeType === Node.CDATA_SECTION_NODE) return node.textContent
  if (node.nodeType !== Node.ELEMENT_NODE) return undefined

  const el = node as Element
  const obj: JsonNode = {}

  for (const attr of Array.from(el.attributes)) {
    obj['@' + attr.name] = attr.value
  }

  const childMap = new Map<string, unknown[]>()
  for (const child of Array.from(el.childNodes)) {
    if (child.nodeType === Node.TEXT_NODE && !(child.textContent?.trim())) continue
    if (child.nodeType === Node.COMMENT_NODE) continue
    const key = child.nodeType === Node.TEXT_NODE ? '#text' : (child as Element).tagName
    const val = xmlNodeToJson(child)
    if (val === undefined) continue
    if (!childMap.has(key)) childMap.set(key, [])
    childMap.get(key)!.push(val)
  }

  for (const [key, arr] of childMap) {
    if (arr.length === 1) obj[key] = arr[0]
    else obj[key] = arr
  }

  return obj
}

function xmlToJsonString(xml: string): string {
  const parser = new DOMParser()
  const doc = parser.parseFromString(xml, 'text/xml')
  const err = doc.querySelector('parsererror')
  if (err) throw new Error('XML 解析失败：' + (err.textContent || '格式错误'))
  const root = doc.documentElement
  const json: Record<string, unknown> = {}
  json[root.tagName] = xmlNodeToJson(root)
  return JSON.stringify(json, null, 2)
}

function jsonValueToXml(key: string, value: unknown, indent = 0): string {
  const pad = '  '.repeat(indent)
  if (value === null || value === undefined) return `${pad}<${key}/>\n`
  if (typeof value === 'boolean' || typeof value === 'number') return `${pad}<${key}>${value}</${key}>\n`
  if (typeof value === 'string') return `${pad}<${key}>${value}</${key}>\n`
  if (Array.isArray(value)) return value.map(v => jsonValueToXml(key, v, indent)).join('')

  const obj = value as Record<string, unknown>
  const attrs: string[] = []
  const children: string[] = []
  let textContent = ''

  for (const [k, v] of Object.entries(obj)) {
    if (k.startsWith('@')) { attrs.push(` ${k.slice(1)}="${String(v).replace(/"/g, '&quot;')}"`); continue }
    if (k === '#text') { textContent = String(v); continue }
    children.push(jsonValueToXml(k, v, indent + 1))
  }

  const openTag = `${pad}<${key}${attrs.join('')}>`
  if (children.length === 0 && textContent === '') return `${openTag}/>\n`
  if (children.length === 0) return `${openTag}${textContent}</${key}>\n`
  return openTag + '\n' + children.join('') + `${pad}</${key}>\n`
}

function jsonToXmlString(json: string): string {
  const parsed = JSON.parse(json)
  const keys = Object.keys(parsed)
  if (keys.length !== 1) throw new Error('JSON 必须只有一个根对象键，对应 XML 根元素。')
  const rootKey = keys[0]
  return '<?xml version="1.0" encoding="UTF-8"?>\n' + jsonValueToXml(rootKey, parsed[rootKey]).trimEnd()
}

function runXmlToJson() {
  message.value = ''
  if (!leftText.value.trim()) { message.value = '请输入 XML。'; return }
  try {
    rightText.value = xmlToJsonString(leftText.value)
    message.value = 'XML → JSON 转换成功。'
  } catch (e) { message.value = '转换失败：' + String(e) }
}

function runJsonToXml() {
  message.value = ''
  if (!rightText.value.trim()) { message.value = '请输入 JSON。'; return }
  try {
    leftText.value = jsonToXmlString(rightText.value)
    message.value = 'JSON → XML 转换成功。'
  } catch (e) { message.value = '转换失败：' + String(e) }
}

function swap() {
  const tmp = leftText.value; leftText.value = rightText.value; rightText.value = tmp
  message.value = '已交换两侧内容。'
}

function reset() { leftText.value = ''; rightText.value = ''; message.value = '' }
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>XML ↔ JSON 互转</h2><p>浏览器原生 DOMParser / XMLSerializer 零依赖实现；属性用 @key 表示，同名子元素自动收敛为数组。</p></div><Files :size="18" /></div>

    <div class="toolkit-action-row">
      <button class="button button-primary" @click="runXmlToJson"><FileJson :size="14" />XML → JSON</button>
      <button class="button button-secondary" @click="runJsonToXml">JSON → XML</button>
      <button class="button button-secondary" @click="swap">交换两侧</button>
      <button class="button button-quiet" @click="copyText(rightText, 'JSON')"><Copy :size="14" />复制 JSON</button>
      <button class="button button-quiet" @click="copyText(leftText, 'XML')"><Copy :size="14" />复制 XML</button>
      <button class="button button-quiet" @click="reset">清空</button>
    </div>

    <div class="toolkit-json-grid">
      <div class="toolkit-editor-pane">
        <label class="toolkit-label" for="xml-left">XML（左）</label>
        <textarea id="xml-left" v-model="leftText" class="toolkit-textarea tall mono" spellcheck="false" placeholder="<root><item>val</item></root>" />
      </div>
      <div class="toolkit-editor-pane">
        <label class="toolkit-label" for="xml-right">JSON（右）</label>
        <textarea id="xml-right" v-model="rightText" class="toolkit-textarea tall mono" spellcheck="false" placeholder='{"root":{"item":"val"}}' />
      </div>
    </div>

    <p v-if="message" class="toolkit-inline-message">{{ message }}</p>
    <p class="toolkit-help">同名兄弟元素在 JSON 中会变成数组；XML 属性映射为 @ 前缀键（如 @id）。</p>
  </div>
</template>