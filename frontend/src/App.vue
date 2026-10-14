<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import {
  Activity, ArrowRight, Braces, CheckCircle2, ChevronDown, ChevronRight,
  CircleHelp, Command, FileSearch, FolderOpen, Gauge, HardDrive, Layers3,
  Regex, Search, ShieldCheck, TerminalSquare, Wrench, X,
} from 'lucide-vue-next'

type PageKey = 'logs' | 'processes' | 'toolkit'
const activePage = ref<PageKey>('logs')
const filePath = ref('')
const query = ref('')
const useRegex = ref(false)
const busy = ref(false)
const cancelRequested = ref(false)
const errorMessage = ref('')
const response = ref<LogSearchResponse | null>(null)
const searchProgress = ref<LogSearchProgress | null>(null)
const currentSearchID = ref('')
let progressTimer: number | undefined

const fileName = computed(() => filePath.value.split(/[\\/]/).filter(Boolean).pop() || '')
const fileSize = computed(() => formatBytes(response.value?.fileSizeBytes ?? 0))
const progressBytes = computed(() => formatBytes(searchProgress.value?.bytesScanned ?? 0))
const totalBytes = computed(() => formatBytes(searchProgress.value?.fileSizeBytes ?? 0))

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit++ }
  return `${value.toFixed(value >= 100 ? 0 : 1)} ${units[unit]}`
}

function stopProgressPolling() {
  if (progressTimer !== undefined) {
    window.clearInterval(progressTimer)
    progressTimer = undefined
  }
}

async function chooseFile() {
  errorMessage.value = ''
  const bridge = window.go?.main?.App
  if (!bridge) { errorMessage.value = '请在 Wails 桌面应用中使用原生文件选择器。'; return }
  try {
    const selected = await bridge.SelectLogFile()
    if (selected) {
      filePath.value = selected
      response.value = null
      searchProgress.value = null
    }
  } catch (error) {
    errorMessage.value = String(error)
  }
}

async function pollProgress(searchID: string) {
  const bridge = window.go?.main?.App
  if (!bridge) return
  try {
    const progress = await bridge.GetLogSearchProgress(searchID)
    if (currentSearchID.value === searchID) searchProgress.value = progress
  } catch {
    // 任务可能刚创建；下一轮轮询会再次尝试。
  }
}

async function cancelSearch() {
  const bridge = window.go?.main?.App
  if (!bridge || !currentSearchID.value) return
  cancelRequested.value = true
  try {
    await bridge.CancelLogSearch(currentSearchID.value)
  } catch (error) {
    errorMessage.value = `取消检索失败：${String(error)}`
  }
}

async function runSearch() {
  errorMessage.value = ''
  if (!filePath.value) { errorMessage.value = '请先选择一个日志文件。'; return }
  if (!query.value) { errorMessage.value = '请输入关键字或正则表达式。'; return }
  const bridge = window.go?.main?.App
  if (!bridge) { errorMessage.value = '未检测到 Go 桌面桥接服务，请通过 wails dev 启动应用。'; return }

  stopProgressPolling()
  const searchID = `${Date.now()}-${Math.random().toString(16).slice(2)}`
  currentSearchID.value = searchID
  cancelRequested.value = false
  response.value = null
  searchProgress.value = {
    searchId: searchID, bytesScanned: 0, fileSizeBytes: 0, scannedLines: 0,
    matchesFound: 0, percent: 0, running: true, canceled: false,
  }
  busy.value = true
  progressTimer = window.setInterval(() => { void pollProgress(searchID) }, 250)

  try {
    response.value = await bridge.SearchLogs(searchID, filePath.value, query.value, useRegex.value, 500)
    await pollProgress(searchID)
    if (response.value.canceled) {
      errorMessage.value = '检索已取消，以下保留已找到的部分结果。'
    }
  } catch (error) {
    errorMessage.value = String(error)
  } finally {
    stopProgressPolling()
    busy.value = false
    cancelRequested.value = false
  }
}

function setPage(page: PageKey) {
  activePage.value = page
  if (page !== 'logs') errorMessage.value = ''
}

onBeforeUnmount(() => stopProgressPolling())
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand">
        <div class="brand-mark"><Activity :size="20" :stroke-width="2.4" /></div>
        <div class="brand-copy"><span class="brand-name">Meteor<span>Desk</span></span><span class="brand-caption">本地开发工作台</span></div>
      </div>
      <div class="workspace-label">工作空间</div>
      <nav class="nav-list" aria-label="主导航">
        <button class="nav-item" :class="{ active: activePage === 'logs' }" @click="setPage('logs')"><FileSearch :size="17" /><span>日志分析</span><span class="nav-indicator"></span></button>
        <button class="nav-item" :class="{ active: activePage === 'processes' }" @click="setPage('processes')"><TerminalSquare :size="17" /><span>进程管理</span><span class="soon-dot"></span></button>
        <button class="nav-item" :class="{ active: activePage === 'toolkit' }" @click="setPage('toolkit')"><Wrench :size="17" /><span>开发工具</span><span class="soon-dot"></span></button>
      </nav>
      <div class="sidebar-divider"></div>
      <div class="workspace-label">运行方式</div>
      <div class="local-card"><div class="local-icon"><ShieldCheck :size="16" /></div><div><div class="local-title">本地优先</div><div class="local-description">文件仅在本机处理</div></div><span class="status-dot"></span></div>
      <div class="sidebar-bottom">
        <div class="version-row"><span>早期版本</span><span class="version-tag">v0.1.0</span></div>
        <div class="profile-row"><div class="avatar">MD</div><div class="profile-copy"><strong>MeteorDesk</strong><span>开发与运维工作台</span></div><CircleHelp :size="17" class="help-icon" /></div>
      </div>
    </aside>

    <main class="main-content">
      <header class="topbar">
        <div class="breadcrumbs"><span>工作空间</span><ChevronRight :size="14" /><strong>{{ activePage === 'logs' ? '日志分析' : activePage === 'processes' ? '进程管理' : '开发工具' }}</strong></div>
        <div class="topbar-right"><div class="runtime-status"><span class="status-dot"></span> 本机运行</div><div class="topbar-divider"></div><div class="shortcut-hint"><Command :size="13" /><span>K</span></div></div>
      </header>

      <div class="page-wrap">
        <section v-if="activePage === 'logs'" class="page-content">
          <div class="page-heading">
            <div><div class="eyebrow"><span class="eyebrow-line"></span> 可观测性 / 文件分析</div><h1>日志分析<span class="title-period">。</span></h1><p class="page-subtitle">逐块读取大体积日志，避免一次性将整个文件载入内存。</p></div>
            <button class="button button-secondary" @click="chooseFile"><FolderOpen :size="16" /> 选择日志</button>
          </div>

          <div class="metric-grid">
            <div class="metric-card"><div class="metric-top"><span class="metric-icon mint"><Gauge :size="16" /></span><span class="metric-kicker">检索方式</span></div><div class="metric-value">流式扫描</div><div class="metric-foot"><span class="tiny-check"><CheckCircle2 :size="12" /></span> 固定大小缓冲区</div></div>
            <div class="metric-card"><div class="metric-top"><span class="metric-icon blue"><HardDrive :size="16" /></span><span class="metric-kicker">数据处理</span></div><div class="metric-value">仅限本地</div><div class="metric-foot"><span class="tiny-check"><CheckCircle2 :size="12" /></span> 不上传日志内容</div></div>
            <div class="metric-card"><div class="metric-top"><span class="metric-icon purple"><Layers3 :size="16" /></span><span class="metric-kicker">结果上限</span></div><div class="metric-value">500 条</div><div class="metric-foot">限制结果规模，降低内存占用</div></div>
          </div>

          <section class="search-panel">
            <div class="panel-heading"><div class="panel-heading-left"><div class="panel-icon"><Search :size="17" /></div><div><h2>查找日志</h2><p>输入关键字或正则表达式，定位匹配行。</p></div></div><div class="panel-meta"><span class="live-dot"></span> 本地检索</div></div>
            <div class="file-drop" :class="{ 'file-selected': !!filePath }"><div class="file-icon"><FileSearch :size="20" /></div><div class="file-info"><strong>{{ fileName || '请选择要分析的日志文件' }}</strong><span>{{ filePath || '支持 .log、.txt、.out、.json 和 .ndjson 文件' }}</span></div><div v-if="filePath && response" class="file-size">{{ fileSize }}</div><button class="button button-quiet" @click="chooseFile">{{ filePath ? '更换文件' : '浏览文件' }} <ArrowRight :size="14" /></button></div>

            <form class="query-form" @submit.prevent="runSearch">
              <label class="field-label" for="search-query">搜索内容</label>
              <div class="search-input-row"><div class="query-input-wrap"><Search :size="17" class="query-input-icon" /><input id="search-query" v-model="query" autocomplete="off" spellcheck="false" placeholder="输入关键字、错误码、请求 ID……" /><span class="input-shortcut">回车 ↵</span></div><button type="submit" class="button button-primary" :disabled="busy"><Search v-if="!busy" :size="16" /><span v-else class="spinner"></span>{{ busy ? '正在检索' : '开始检索' }}</button><button v-if="busy" type="button" class="button button-secondary button-cancel" :disabled="cancelRequested" @click="cancelSearch"><X :size="15" />{{ cancelRequested ? '正在取消' : '取消检索' }}</button></div>
              <div class="query-options"><label class="toggle-option"><input v-model="useRegex" type="checkbox" /><span class="custom-checkbox"><Regex v-if="useRegex" :size="12" /></span><span>使用正则表达式</span></label><span class="option-separator"></span><span class="option-note">区分大小写 · 最多返回 500 条</span><span class="option-spacer"></span><span class="query-tip"><kbd>.*</kbd> 支持 Go 正则语法</span></div>
              <div v-if="busy && searchProgress" class="progress-wrap">
                <div class="progress-meta"><span>{{ cancelRequested ? '正在停止检索……' : '正在扫描文件' }} · {{ searchProgress.scannedLines.toLocaleString() }} 行</span><span>{{ searchProgress.percent }}%　{{ progressBytes }} / {{ totalBytes }}</span></div>
                <div class="progress-track"><div class="progress-bar" :style="{ width: `${searchProgress.percent}%` }"></div></div>
              </div>
            </form>

            <div v-if="errorMessage" class="notice notice-error"><span>!</span>{{ errorMessage }}</div>
            <div class="results-header"><div><h3>检索结果 <span v-if="response" class="result-count">{{ response.matches.length }}</span></h3><p>{{ response ? `已扫描 ${response.scannedLines.toLocaleString()} 行 · ${fileSize}` : '检索结果会显示在这里。' }}</p></div><div v-if="response" class="results-actions"><span v-if="response.resultLimitReached" class="limit-pill">已达到结果上限</span><span v-if="response.canceled" class="result-state">已取消</span><span v-else class="result-state"><span class="status-dot"></span> 已完成</span></div></div>
            <div v-if="response && response.matches.length" class="result-list"><div v-for="(match, index) in response.matches" :key="match.lineNumber" class="result-row"><span class="line-number">{{ String(match.lineNumber).padStart(5, '0') }}</span><span class="result-marker" :class="{ first: index === 0 }"></span><code>{{ match.content }}<span v-if="match.contentClipped" class="clipped-note"> [内容过长，已截断预览]</span></code><span class="row-chevron"><ChevronDown :size="14" /></span></div></div>
            <div v-else-if="response" class="empty-results"><div class="empty-icon"><Search :size="20" /></div><strong>没有找到匹配内容</strong><span>尝试更换关键字，或启用正则表达式。</span></div>
            <div v-else class="empty-results initial-empty"><div class="empty-illustration"><div class="empty-orbit orbit-one"></div><div class="empty-orbit orbit-two"></div><div class="empty-center"><FileSearch :size="24" /></div><span class="orbit-dot dot-a"></span><span class="orbit-dot dot-b"></span></div><strong>准备就绪</strong><span>选择本地日志文件并输入搜索内容，即可开始分析。</span></div>
            <div class="panel-footer"><span><span class="footer-led"></span> 日志内容仅在本机处理</span><span>流式检索 <span class="footer-divider">/</span> 单行预览最多 8 KiB</span></div>
          </section>
          <div class="bottom-note"><Braces :size="15" /><span>JSON 格式化和结构化日志分析将在后续版本加入。</span><button @click="setPage('toolkit')">查看开发工具 <ArrowRight :size="13" /></button></div>
        </section>

        <section v-else class="coming-soon-page">
          <div class="coming-icon"><TerminalSquare v-if="activePage === 'processes'" :size="26" /><Wrench v-else :size="26" /></div>
          <div class="eyebrow"><span class="eyebrow-line"></span> 后续规划</div><h1>{{ activePage === 'processes' ? '进程管理' : '开发工具' }}<span class="title-period">。</span></h1>
          <p>{{ activePage === 'processes' ? '管理本地开发服务，查看标准输出，并支持批量启动和停止服务。' : '提供时间戳、JSON、Base64 和 JWT 载荷查看等常用开发辅助能力。' }}</p>
          <div class="coming-card"><span class="coming-card-icon"><Layers3 :size="17" /></span><div><strong>{{ activePage === 'processes' ? '本地进程编排' : '开发者常用工具集' }}</strong><span>{{ activePage === 'processes' ? '后端已建立进程配置的数据结构，完整生命周期管理仍待实现。' : '所有工具默认在本地运行，不向外部服务发送数据。' }}</span></div><span class="roadmap-tag">计划中</span></div>
          <button class="button button-secondary" @click="setPage('logs')"><ArrowRight :size="15" class="back-arrow" /> 返回日志分析</button>
        </section>
      </div>
      <footer class="app-footer"><span>METEORDESK <span class="footer-divider">/</span> 本地开发与运维工作台</span><span>专注效率 <span class="footer-divider">·</span> 数据留在本机</span></footer>
    </main>
  </div>
</template>
