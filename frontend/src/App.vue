<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  Activity, ArrowRight, Braces, CheckCircle2, ChevronDown, ChevronRight,
  CircleHelp, Command, FileSearch, FolderOpen, Gauge, HardDrive, Layers3,
  Regex, Search, ShieldCheck, TerminalSquare, Wrench,
} from 'lucide-vue-next'

type PageKey = 'logs' | 'processes' | 'toolkit'
const activePage = ref<PageKey>('logs')
const filePath = ref('')
const query = ref('')
const useRegex = ref(false)
const busy = ref(false)
const errorMessage = ref('')
const response = ref<LogSearchResponse | null>(null)
const fileName = computed(() => filePath.value.split(/[\\/]/).filter(Boolean).pop() || '')
const fileSize = computed(() => {
  const bytes = response.value?.fileSizeBytes ?? 0
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024, unit = 0
  while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit++ }
  return `${value.toFixed(value >= 100 ? 0 : 1)} ${units[unit]}`
})

async function chooseFile() {
  errorMessage.value = ''
  const bridge = window.go?.main?.App
  if (!bridge) { errorMessage.value = 'Open MeteorDesk as a Wails desktop app to use the native file picker.'; return }
  try {
    const selected = await bridge.SelectLogFile()
    if (selected) { filePath.value = selected; response.value = null }
  } catch (error) { errorMessage.value = String(error) }
}

async function runSearch() {
  errorMessage.value = ''
  if (!filePath.value) { errorMessage.value = 'Choose a log file before searching.'; return }
  if (!query.value) { errorMessage.value = 'Enter a search term or regular expression.'; return }
  const bridge = window.go?.main?.App
  if (!bridge) { errorMessage.value = 'The Go desktop bridge is unavailable. Launch with “wails dev” to search local files.'; return }
  busy.value = true
  try { response.value = await bridge.SearchLogs(filePath.value, query.value, useRegex.value, 500) }
  catch (error) { errorMessage.value = String(error) }
  finally { busy.value = false }
}
function setPage(page: PageKey) {
  activePage.value = page
  if (page !== 'logs') errorMessage.value = ''
}
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand">
        <div class="brand-mark"><Activity :size="20" :stroke-width="2.4" /></div>
        <div class="brand-copy"><span class="brand-name">Meteor<span>Desk</span></span><span class="brand-caption">LOCAL WORKBENCH</span></div>
      </div>
      <div class="workspace-label">WORKSPACE</div>
      <nav class="nav-list" aria-label="Main navigation">
        <button class="nav-item" :class="{ active: activePage === 'logs' }" @click="setPage('logs')"><FileSearch :size="17" /><span>Log Explorer</span><span class="nav-indicator"></span></button>
        <button class="nav-item" :class="{ active: activePage === 'processes' }" @click="setPage('processes')"><TerminalSquare :size="17" /><span>Process Manager</span><span class="soon-dot"></span></button>
        <button class="nav-item" :class="{ active: activePage === 'toolkit' }" @click="setPage('toolkit')"><Wrench :size="17" /><span>Dev Toolkit</span><span class="soon-dot"></span></button>
      </nav>
      <div class="sidebar-divider"></div>
      <div class="workspace-label">SYSTEM</div>
      <div class="local-card"><div class="local-icon"><ShieldCheck :size="16" /></div><div><div class="local-title">Local by design</div><div class="local-description">Your files stay on this machine.</div></div><span class="status-dot"></span></div>
      <div class="sidebar-bottom">
        <div class="version-row"><span>Early access</span><span class="version-tag">v0.1.0</span></div>
        <div class="profile-row"><div class="avatar">MD</div><div class="profile-copy"><strong>MeteorDesk</strong><span>Developer workspace</span></div><CircleHelp :size="17" class="help-icon" /></div>
      </div>
    </aside>

    <main class="main-content">
      <header class="topbar">
        <div class="breadcrumbs"><span>Workspace</span><ChevronRight :size="14" /><strong>{{ activePage === 'logs' ? 'Log Explorer' : activePage === 'processes' ? 'Process Manager' : 'Dev Toolkit' }}</strong></div>
        <div class="topbar-right"><div class="runtime-status"><span class="status-dot"></span> Running locally</div><div class="topbar-divider"></div><div class="shortcut-hint"><Command :size="13" /><span>K</span></div></div>
      </header>

      <div class="page-wrap">
        <section v-if="activePage === 'logs'" class="page-content">
          <div class="page-heading">
            <div><div class="eyebrow"><span class="eyebrow-line"></span> OBSERVABILITY / FILE ANALYSIS</div><h1>Log Explorer<span class="title-period">.</span></h1><p class="page-subtitle">Search through massive log files without loading them all into memory.</p></div>
            <button class="button button-secondary" @click="chooseFile"><FolderOpen :size="16" /> Open log file</button>
          </div>

          <div class="metric-grid">
            <div class="metric-card"><div class="metric-top"><span class="metric-icon mint"><Gauge :size="16" /></span><span class="metric-kicker">SEARCH ENGINE</span></div><div class="metric-value">Stream-first</div><div class="metric-foot"><span class="tiny-check"><CheckCircle2 :size="12" /></span> Sequential file reads</div></div>
            <div class="metric-card"><div class="metric-top"><span class="metric-icon blue"><HardDrive :size="16" /></span><span class="metric-kicker">DATA HANDLING</span></div><div class="metric-value">Local only</div><div class="metric-foot"><span class="tiny-check"><CheckCircle2 :size="12" /></span> No cloud uploads</div></div>
            <div class="metric-card"><div class="metric-top"><span class="metric-icon purple"><Layers3 :size="16" /></span><span class="metric-kicker">RESULT WINDOW</span></div><div class="metric-value">500 lines</div><div class="metric-foot">Bounded results per search</div></div>
          </div>

          <section class="search-panel">
            <div class="panel-heading"><div class="panel-heading-left"><div class="panel-icon"><Search :size="17" /></div><div><h2>Find in files</h2><p>Configure a query and inspect matching lines.</p></div></div><div class="panel-meta"><span class="live-dot"></span> LIVE SEARCH</div></div>
            <div class="file-drop" :class="{ 'file-selected': !!filePath }"><div class="file-icon"><FileSearch :size="20" /></div><div class="file-info"><strong>{{ fileName || 'Select a log file to get started' }}</strong><span>{{ filePath || 'Supports .log, .txt, .out, .json and .ndjson' }}</span></div><div v-if="filePath && response" class="file-size">{{ fileSize }}</div><button class="button button-quiet" @click="chooseFile">{{ filePath ? 'Change file' : 'Browse files' }} <ArrowRight :size="14" /></button></div>

            <form class="query-form" @submit.prevent="runSearch">
              <label class="field-label" for="search-query">SEARCH QUERY</label>
              <div class="search-input-row"><div class="query-input-wrap"><Search :size="17" class="query-input-icon" /><input id="search-query" v-model="query" autocomplete="off" spellcheck="false" placeholder="Search keywords, error codes, request IDs..." /><span class="input-shortcut">Enter ↵</span></div><button type="submit" class="button button-primary" :disabled="busy"><Search v-if="!busy" :size="16" /><span v-else class="spinner"></span>{{ busy ? 'Searching' : 'Search logs' }}</button></div>
              <div class="query-options"><label class="toggle-option"><input v-model="useRegex" type="checkbox" /><span class="custom-checkbox"><Regex v-if="useRegex" :size="12" /></span><span>Regular expression</span></label><span class="option-separator"></span><span class="option-note">Case-sensitive · First 500 matches</span><span class="option-spacer"></span><span class="query-tip"><kbd>.*</kbd> Regex supported</span></div>
            </form>

            <div v-if="errorMessage" class="notice notice-error"><span>!</span>{{ errorMessage }}</div>
            <div class="results-header"><div><h3>Search results <span v-if="response" class="result-count">{{ response.matches.length }}</span></h3><p>{{ response ? `Scanned ${response.scannedLines.toLocaleString()} lines · ${fileSize}` : 'Matching lines will appear here after a search.' }}</p></div><div v-if="response" class="results-actions"><span v-if="response.resultLimitReached" class="limit-pill">Result cap reached</span><span class="result-state"><span class="status-dot"></span> Complete</span></div></div>
            <div v-if="response && response.matches.length" class="result-list"><div v-for="(match, index) in response.matches" :key="match.lineNumber" class="result-row"><span class="line-number">{{ String(match.lineNumber).padStart(5, '0') }}</span><span class="result-marker" :class="{ first: index === 0 }"></span><code>{{ match.content }}<span v-if="match.contentClipped" class="clipped-note"> [preview clipped]</span></code><span class="row-chevron"><ChevronDown :size="14" /></span></div></div>
            <div v-else-if="response" class="empty-results"><div class="empty-icon"><Search :size="20" /></div><strong>No matches found</strong><span>Try another search term or enable regular expressions.</span></div>
            <div v-else class="empty-results initial-empty"><div class="empty-illustration"><div class="empty-orbit orbit-one"></div><div class="empty-orbit orbit-two"></div><div class="empty-center"><FileSearch :size="24" /></div><span class="orbit-dot dot-a"></span><span class="orbit-dot dot-b"></span></div><strong>Ready when you are</strong><span>Choose a local file, enter a query, and let MeteorDesk do the digging.</span></div>
            <div class="panel-footer"><span><span class="footer-led"></span> All processing happens on your device</span><span>Streaming search <span class="footer-divider">/</span> Preview capped at 8 KB per line</span></div>
          </section>
          <div class="bottom-note"><Braces :size="15" /><span>JSON formatting and structured log inspection are planned next.</span><button @click="setPage('toolkit')">Explore toolkit <ArrowRight :size="13" /></button></div>
        </section>

        <section v-else class="coming-soon-page">
          <div class="coming-icon"><TerminalSquare v-if="activePage === 'processes'" :size="26" /><Wrench v-else :size="26" /></div>
          <div class="eyebrow"><span class="eyebrow-line"></span> ON THE ROADMAP</div><h1>{{ activePage === 'processes' ? 'Process Manager' : 'Dev Toolkit' }}<span class="title-period">.</span></h1>
          <p>{{ activePage === 'processes' ? 'Manage local development services, stream their output, and start or stop a whole workspace.' : 'Small, privacy-friendly utilities for everyday development: JSON, timestamps, Base64 and JWT inspection.' }}</p>
          <div class="coming-card"><span class="coming-card-icon"><Layers3 :size="17" /></span><div><strong>{{ activePage === 'processes' ? 'Process orchestration foundation' : 'Developer utility collection' }}</strong><span>{{ activePage === 'processes' ? 'Command specs and lifecycle types are in the Go backend.' : 'The tools will run locally without sending data to external services.' }}</span></div><span class="roadmap-tag">PLANNED</span></div>
          <button class="button button-secondary" @click="setPage('logs')"><ArrowRight :size="15" class="back-arrow" /> Back to Log Explorer</button>
        </section>
      </div>
      <footer class="app-footer"><span>METEORDESK <span class="footer-divider">/</span> LOCAL DEVELOPER WORKBENCH</span><span>BUILT FOR FOCUS <span class="footer-divider">·</span> BUILT TO STAY LOCAL</span></footer>
    </main>
  </div>
</template>
