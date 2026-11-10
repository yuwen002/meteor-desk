<script setup lang="ts">
import { ref } from 'vue'
import { Copy, FileJson, Files } from 'lucide-vue-next'
import yaml from 'js-yaml'
import { useCopyFeedback } from '../../composables/useCopyFeedback'

const { copyText } = useCopyFeedback()

const leftText = ref('name: meteor-desk\nversion: 2.0.0\nfeatures:\n  - log-analysis\n  - process-management\n  - dev-toolkit\nports:\n  http: 8080\n  grpc: 9090\nmetadata:\n  author: team\n  tags: [devtools, productivity]')
const rightText = ref('')
const message = ref('')

function yamlToJson() {
  message.value = ''
  if (!leftText.value.trim()) { message.value = '请输入 YAML。'; return }
  try {
    const parsed = yaml.load(leftText.value)
    rightText.value = JSON.stringify(parsed, null, 2)
    message.value = 'YAML → JSON 转换成功。'
  } catch (e) { message.value = 'YAML 解析失败：' + String(e) }
}

function jsonToYaml() {
  message.value = ''
  if (!rightText.value.trim()) { message.value = '请输入 JSON。'; return }
  try {
    const parsed = JSON.parse(rightText.value)
    leftText.value = yaml.dump(parsed, { indent: 2, lineWidth: 120, noRefs: true })
    message.value = 'JSON → YAML 转换成功。'
  } catch (e) { message.value = 'JSON 解析失败：' + String(e) }
}

function swap() {
  const tmp = leftText.value
  leftText.value = rightText.value
  rightText.value = tmp
  message.value = '已交换两侧内容。'
}

function reset() {
  leftText.value = ''
  rightText.value = ''
  message.value = ''
}
</script>

<template>
  <div class="toolkit-card">
    <div class="toolkit-card-heading"><div><h2>YAML ↔ JSON 互转</h2><p>配置文件场景高频；支持 YAML 数组、嵌套映射、锚点引用展开。</p></div><Files :size="18" /></div>

    <div class="toolkit-action-row">
      <button class="button button-primary" @click="yamlToJson"><FileJson :size="14" />YAML → JSON</button>
      <button class="button button-secondary" @click="jsonToYaml">JSON → YAML</button>
      <button class="button button-secondary" @click="swap">交换两侧</button>
      <button class="button button-quiet" @click="copyText(rightText, 'JSON')"><Copy :size="14" />复制 JSON</button>
      <button class="button button-quiet" @click="copyText(leftText, 'YAML')"><Copy :size="14" />复制 YAML</button>
      <button class="button button-quiet" @click="reset">清空</button>
    </div>

    <div class="toolkit-json-grid">
      <div class="toolkit-editor-pane">
        <label class="toolkit-label" for="yaml-left">YAML（左）</label>
        <textarea id="yaml-left" v-model="leftText" class="toolkit-textarea tall mono" spellcheck="false" placeholder="name: value&#10;items:&#10;  - one&#10;  - two" />
      </div>
      <div class="toolkit-editor-pane">
        <label class="toolkit-label" for="yaml-right">JSON（右）</label>
        <textarea id="yaml-right" v-model="rightText" class="toolkit-textarea tall mono" spellcheck="false" placeholder='{"name":"value","items":["one","two"]}' />
      </div>
    </div>

    <p v-if="message" class="toolkit-inline-message">{{ message }}</p>
    <p class="toolkit-help">转换依赖 js-yaml；不执行 YAML 里的 !!js/func 等危险标签（默认安全模式）。</p>
  </div>
</template>