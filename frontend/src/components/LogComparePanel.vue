<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowRight, FileDiff, Search, X } from 'lucide-vue-next'
import { getBridge } from '../utils/bridge'
import { diffText, type DiffLine } from '../utils/textDiff'
import { isOneSidedAlert, summarizeOneSidedAlerts } from '../utils/logDiff'
import { detectLevel, LEVEL_LABEL } from '../utils/aggregate'

// 两份本机日志逐行对比（阶段 6 批次 D）：读取各文件开头至多 4 MiB，复用 textDiff 纯函数做 LCS 差异，
// 再标出仅出现在一侧的 error/warn 行。组件自持状态，直接调用桌面桥接，不干扰日志检索页。
const MAX_DIFF_BYTES = 4 * 1024 * 1024 // 单侧读取上限，与后端 ReadDecodedFileHead 一致
const MAX_DIFF_LINES = 4000 // 两侧合计行数上限，交给 diffText 兜底（DP 规模约 2000×2000）
const RENDER_CAP = 1500 // 只渲染前若干行差异，避免超长列表卡顿

const leftPath = ref('')
const rightPath = ref('')
const encodingMode = ref('auto')
const busy = ref(false)
const errorMessage = ref('')
const rows = ref<DiffLine[]>([])
const compared = ref(false)
const alertOnly = ref(false)
const meta = ref<{ leftTruncated: boolean; rightTruncated: boolean; leftEncoding: string; rightEncoding: string } | null>(null)

const summary = computed(() => summarizeOneSidedAlerts(rows.value))
const scopedRows = computed(() => (alertOnly.value ? rows.value.filter(isOneSidedAlert) : rows.value))
const visibleRows = computed(() => scopedRows.value.slice(0, RENDER_CAP))
const hiddenByCap = computed(() => Math.max(0, scopedRows.value.length - RENDER_CAP))
// changedCount 只数非等值行；两份文件完全一致时 diffText 仍会返回全 equal 行，用它区分“无差异”。
const changedCount = computed(() => rows.value.reduce((count, line) => (line.kind === 'equal' ? count : count + 1), 0))
const hasChanges = computed(() => changedCount.value > 0)

const baseName = (path: string) => path.split(/[\\/]/).filter(Boolean).pop() || path

// rowClass 给单侧差异行叠加告警样式，方便一眼看到只在一侧出现的 error/warn。
function rowClass(line: DiffLine): string {
  if (line.kind === 'equal') return 'equal'
  const level = detectLevel(line.value)
  return line.kind + (level === 'error' || level === 'warn' ? ' alert' : '')
}

async function chooseSide(side: 'left' | 'right') {
  errorMessage.value = ''
  const a = getBridge()
  if (!a) {
    errorMessage.value = '请通过 Wails 桌面应用运行，才能使用系统文件选择器。'
    return
  }
  try {
    const picked = await a.SelectLogFile()
    if (!picked) return
    if (side === 'left') leftPath.value = picked
    else rightPath.value = picked
  } catch (e) {
    errorMessage.value = String(e)
  }
}

async function runCompare() {
  errorMessage.value = ''
  if (!leftPath.value || !rightPath.value) {
    errorMessage.value = '请先选择要对比的两个日志文件。'
    return
  }
  const a = getBridge()
  if (!a) {
    errorMessage.value = '未检测到桌面桥接服务，请通过 wails dev 启动。'
    return
  }
  busy.value = true
  try {
    const [left, right] = await Promise.all([
      a.ReadLogFileHead(leftPath.value, encodingMode.value, MAX_DIFF_BYTES),
      a.ReadLogFileHead(rightPath.value, encodingMode.value, MAX_DIFF_BYTES),
    ])
    meta.value = { leftTruncated: left.truncated, rightTruncated: right.truncated, leftEncoding: left.encoding, rightEncoding: right.encoding }
    try {
      rows.value = diffText(left.text, right.text, MAX_DIFF_LINES)
      compared.value = true
    } catch (e) {
      // 行数超过 diffText 上限会抛错：清空结果并给出可读提示，建议先缩小文件范围。
      rows.value = []
      compared.value = false
      errorMessage.value = String(e)
    }
  } catch (e) {
    rows.value = []
    compared.value = false
    errorMessage.value = '读取日志失败：' + String(e)
  } finally {
    busy.value = false
  }
}

function reset() {
  rows.value = []
  compared.value = false
  meta.value = null
  alertOnly.value = false
  errorMessage.value = ''
}
</script>

<template>
<section class="search-panel log-compare">
  <div class="panel-heading"><div class="panel-heading-left"><div class="panel-icon"><FileDiff :size="17"/></div><div><h2>两份日志对比</h2><p>选择两个本机文件做逐行差异，并标出仅出现在一侧的 error/warn 行；单侧最多读取 4 MiB。</p></div></div><div class="panel-meta"><span class="live-dot"></span>本地对比</div></div>
  <div class="compare-files">
    <div class="compare-file"><span class="compare-tag">基准</span><strong :title="leftPath">{{ leftPath ? baseName(leftPath) : '未选择文件' }}</strong><button class="button button-quiet" type="button" :disabled="busy" @click="chooseSide('left')"><Search :size="13"/>选择</button></div>
    <div class="compare-file"><span class="compare-tag">对比</span><strong :title="rightPath">{{ rightPath ? baseName(rightPath) : '未选择文件' }}</strong><button class="button button-quiet" type="button" :disabled="busy" @click="chooseSide('right')"><Search :size="13"/>选择</button></div>
    <label class="option-select">文本编码 <select v-model="encodingMode" :disabled="busy"><option value="auto">自动识别</option><option value="utf-8">UTF-8</option><option value="gb18030">GB18030 / GBK</option><option value="utf-16le">UTF-16 LE</option><option value="utf-16be">UTF-16 BE</option></select></label>
    <button class="button button-primary" type="button" :disabled="busy||!leftPath||!rightPath" @click="runCompare"><ArrowRight :size="15"/>{{ busy?'对比中':'开始对比' }}</button>
    <button v-if="compared" class="button button-quiet" type="button" :disabled="busy" @click="reset"><X :size="13"/>清除</button>
  </div>
  <div v-if="meta&&(meta.leftTruncated||meta.rightTruncated)" class="compare-note">文件超过 4 MiB，仅对比开头部分；基准编码 {{meta.leftEncoding}}、对比编码 {{meta.rightEncoding}}。</div>
  <div v-if="errorMessage" class="notice notice-error"><span>!</span>{{errorMessage}}</div>
  <div v-if="compared" class="compare-summary">
    <div class="compare-stats"><span class="compare-stat">差异行 {{changedCount}} / {{rows.length}}</span><span class="compare-stat">仅基准 <em>ERROR {{summary.leftError}}</em> / <em>WARN {{summary.leftWarn}}</em></span><span class="compare-stat">仅对比 <em>ERROR {{summary.rightError}}</em> / <em>WARN {{summary.rightWarn}}</em></span></div>
    <label class="toolbar-toggle"><input v-model="alertOnly" type="checkbox"/><span>只看单侧 error/warn</span></label>
  </div>
  <div v-if="compared&&summary.total" class="compare-alerts"><div v-for="(s,i) in summary.samples" :key="i" class="compare-alert" :class="[s.side, s.level]"><b>{{ s.side==='left'?'-':'+' }}</b><span class="alert-level">{{ LEVEL_LABEL[s.level] }}</span><code>{{ s.text }}</code></div></div>
  <div v-if="compared&&hasChanges&&visibleRows.length" class="compare-diff"><div v-for="(line,index) in visibleRows" :key="index" class="compare-row" :class="rowClass(line)"><b>{{ line.kind==='added'?'+':line.kind==='removed'?'-':' ' }}</b><code>{{ line.value }}</code></div><div v-if="hiddenByCap" class="compare-cap">差异较长，仅显示前 {{ RENDER_CAP }} 行。</div></div>
  <div v-else-if="compared&&!hasChanges" class="compare-empty">两份文件逐行一致，没有差异。</div>
  <div v-else-if="compared" class="compare-empty">只看单侧告警时没有可显示的 error/warn 差异行。</div>
</section>
</template>
