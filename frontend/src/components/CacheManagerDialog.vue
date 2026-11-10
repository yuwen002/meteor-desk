<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { X, Trash2, RefreshCw, HardDrive, FileSearch, Clock, AlertCircle, Download, Search } from 'lucide-vue-next'

const { t } = useI18n()
const emit = defineEmits<{ close: [] }>()

const api = () => window.go?.main?.App

const stats = ref<CacheStatistics | null>(null)
const entries = ref<CacheEntryMeta[]>([])
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const toast = ref('')

const ENTRY_LIMIT = 50

function fmtBytes(bytes: number) {
  if (!bytes || bytes < 1024) return (bytes ?? 0) + ' B'
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = bytes / 1024, i = 0
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return v.toFixed(v >= 100 ? 0 : 1) + ' ' + units[i]
}

function fmtTime(iso: string) {
  if (!iso) return '—'
  try {
    const d = new Date(iso)
    if (isNaN(d.getTime())) return iso
    return d.toLocaleString()
  } catch { return iso }
}

function shortPath(p: string) {
  if (!p) return ''
  const sep = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
  return sep >= 0 ? p.slice(sep + 1) : p
}

async function loadAll() {
  loading.value = true
  error.value = ''
  try {
    const a = api()
    if (!a) { error.value = t('cache.errBridge'); loading.value = false; return }
    const [s, e] = await Promise.all([
      a.GetLogCacheStatistics(),
      a.GetLogCacheEntries(ENTRY_LIMIT),
    ])
    stats.value = s
    entries.value = e
  } catch (e) {
    error.value = t('cache.errLoad') + '：' + String(e)
  } finally {
    loading.value = false
  }
}

async function onClear() {
  busy.value = true
  error.value = ''
  try {
    const a = api()
    if (!a) throw new Error(t('cache.errBridge'))
    await a.ClearLogCache()
    toast.value = t('cache.cleared')
    setTimeout(() => { toast.value = '' }, 2500)
    await loadAll()
  } catch (e) {
    error.value = t('cache.errClear') + '：' + String(e)
  } finally {
    busy.value = false
  }
}

async function onDelete(key: string) {
  busy.value = true
  error.value = ''
  try {
    const a = api()
    if (!a) throw new Error(t('cache.errBridge'))
    await a.DeleteLogCacheEntry(key)
    toast.value = t('cache.deleted')
    setTimeout(() => { toast.value = '' }, 2500)
    await loadAll()
  } catch (e) {
    error.value = t('cache.errDelete') + '：' + String(e)
  } finally {
    busy.value = false
  }
}

onMounted(loadAll)
</script>

<template>
  <div class="settings-overlay" @click.self="emit('close')">
    <section class="settings-dialog cache-dialog" role="dialog" aria-modal="true" aria-labelledby="cache-title">
      <div class="settings-dialog-heading">
        <div>
          <span class="eyebrow">SESSION</span>
          <h2 id="cache-title">{{ $t('cache.title') }}</h2>
          <p>{{ $t('cache.subtitle') }}</p>
        </div>
        <button class="icon-button" type="button" aria-label="关闭" @click="emit('close')"><X :size="17"/></button>
      </div>

      <div class="cache-stats" v-if="stats">
        <div class="cache-stat-item"><HardDrive :size="16"/><span class="stat-num">{{stats.entries}}</span><span class="stat-label">{{ $t('cache.entries') }}</span></div>
        <div class="cache-stat-item"><Download :size="16"/><span class="stat-num">{{fmtBytes(stats.sizeBytes)}}</span><span class="stat-label">{{ $t('cache.sizeBytes') }}</span></div>
        <div class="cache-stat-item"><FileSearch :size="16"/><span class="stat-num">{{stats.maxEntries}}</span><span class="stat-label">{{ $t('cache.maxEntries') }}</span></div>
        <div class="cache-stat-item"><Clock :size="16"/><span class="stat-num">{{fmtBytes(stats.maxSizeBytes)}}</span><span class="stat-label">{{ $t('cache.maxSizeBytes') }}</span></div>
      </div>
      <p v-if="stats" class="cache-dir-note">{{ $t('cache.cacheDir') }}<span class="mono">{{stats.cacheDir}}</span></p>

      <div class="cache-list-header">
        <span class="cache-list-title">{{ $t('cache.recentSessions') }}（{{entries.length}}）</span>
        <button class="button button-quiet" type="button" :disabled="busy||loading" @click="loadAll"><RefreshCw :size="13" :class="{'spinner-icon': loading}"/>{{ $t('cache.refresh') }}</button>
      </div>

      <div class="cache-list" v-if="entries.length">
        <div v-for="e in entries" :key="e.query+'|'+e.path+'|'+e.fileSize+'|'+e.fileMtime" class="cache-entry">
          <div class="ce-main">
            <div class="ce-top">
              <span class="ce-filename" :title="e.path">{{shortPath(e.path)}}</span>
              <span class="ce-meta-tag" :class="{'regex-tag':e.useRegex}">{{e.useRegex ? $t('cache.modeRegex') : $t('cache.modePlain')}}</span>
              <span v-if="e.encoding" class="ce-meta-tag">{{e.encoding}}</span>
              <span v-if="e.resultLimitReached" class="ce-meta-tag limit-tag">{{ $t('cache.partial') }}</span>
            </div>
            <div class="ce-query" :title="e.query"><Search :size="13"/>{{e.query}}</div>
            <div class="ce-foot">
              <span>{{e.matches}} {{ $t('cache.matchesUnit') }}</span>
              <span>·</span>
              <span>{{fmtBytes(e.fileSize)}}</span>
              <span>·</span>
              <span>{{ $t('cache.created') }} {{fmtTime(e.createdAt)}}</span>
              <span>·</span>
              <span>{{ $t('cache.accessed') }} {{fmtTime(e.accessedAt)}}</span>
            </div>
          </div>
          <button class="ce-delete" type="button" :disabled="busy" title="删除这条缓存" @click="onDelete(e.query+'|'+e.path+'|'+e.fileSize+'|'+e.fileMtime)">
            <Trash2 :size="14"/>
          </button>
        </div>
      </div>
      <p v-else-if="!loading" class="cache-empty">{{ $t('cache.empty') }}</p>

      <div v-if="loading" class="cache-loading"><RefreshCw :size="14" class="spinner-icon"/>{{ $t('cache.loading') }}</div>
      <p v-if="error" class="settings-error" role="alert"><AlertCircle :size="13"/>{{error}}</p>
      <p v-if="toast" class="settings-success" role="status">{{toast}}</p>

      <div class="settings-dialog-actions">
        <button class="button button-secondary" type="button" :disabled="busy||loading||!stats||stats.entries===0" @click="onClear"><Trash2 :size="14"/>{{ $t('cache.clearAll') }}</button>
        <span class="option-spacer"></span>
        <button class="button button-primary" type="button" @click="emit('close')">{{ $t('cache.close') }}</button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.cache-dialog { max-width: 720px; max-height: 86vh; display: flex; flex-direction: column; }
.cache-dialog-heading { flex-shrink: 0; }
.cache-stats { display: grid; grid-template-columns: repeat(4, 1fr); gap: 10px; margin: 14px 0 6px; }
.cache-stat-item { display: flex; flex-direction: column; align-items: center; gap: 2px; padding: 10px 6px; background: var(--surface-elev); border-radius: 8px; color: var(--text-secondary); font-size: 11px; }
.cache-stat-item svg { color: var(--accent); margin-bottom: 4px; }
.cache-stat-item .stat-num { font-size: 15px; font-weight: 600; color: var(--text-primary); line-height: 1.2; }
.cache-stat-item .stat-label { font-size: 11px; opacity: 0.7; }
.cache-dir-note { font-size: 11px; color: var(--text-tertiary); margin: 0 0 6px; }
.cache-dir-note .mono { font-family: var(--font-mono); word-break: break-all; }

.cache-list-header { display: flex; justify-content: space-between; align-items: center; margin: 16px 0 8px; flex-shrink: 0; }
.cache-list-title { font-size: 13px; font-weight: 600; color: var(--text-primary); }

.cache-list { overflow-y: auto; flex: 1; border: 1px solid var(--border); border-radius: 8px; min-height: 120px; max-height: 340px; }
.cache-entry { display: flex; align-items: center; gap: 10px; padding: 10px 12px; border-bottom: 1px solid var(--border-subtle); transition: background 0.15s; }
.cache-entry:last-child { border-bottom: none; }
.cache-entry:hover { background: var(--surface-hover); }
.ce-main { flex: 1; min-width: 0; }
.ce-top { display: flex; align-items: center; gap: 6px; margin-bottom: 3px; flex-wrap: wrap; }
.ce-filename { font-family: var(--font-mono); font-size: 12px; font-weight: 600; color: var(--text-primary); }
.ce-meta-tag { font-size: 10px; padding: 1px 6px; border-radius: 10px; background: var(--surface-subtle); color: var(--text-tertiary); }
.regex-tag { background: rgba(255,165,0,0.15); color: #c87a00; }
.limit-tag { background: rgba(231,76,60,0.12); color: #c0392b; }
.ce-query { display: flex; align-items: center; gap: 5px; font-size: 12px; color: var(--text-secondary); margin-bottom: 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ce-query svg { flex-shrink: 0; color: var(--text-tertiary); }
.ce-foot { font-size: 11px; color: var(--text-tertiary); display: flex; gap: 5px; flex-wrap: wrap; }

.ce-delete { background: transparent; border: none; color: var(--text-tertiary); cursor: pointer; padding: 6px; border-radius: 6px; transition: all 0.15s; flex-shrink: 0; }
.ce-delete:hover:not(:disabled) { background: rgba(231,76,60,0.1); color: #e74c3c; }
.ce-delete:disabled { opacity: 0.4; cursor: not-allowed; }

.cache-empty { font-size: 12px; color: var(--text-tertiary); text-align: center; padding: 28px; margin: 0; border: 1px dashed var(--border); border-radius: 8px; }
.cache-loading { font-size: 12px; color: var(--text-tertiary); display: flex; align-items: center; gap: 6px; padding: 12px 0; }
.settings-success { font-size: 12px; color: #27ae60; padding: 6px 0; margin: 0; }
.spinner-icon { animation: spin 1s linear infinite; }
@keyframes spin { from { transform: rotate(0); } to { transform: rotate(360deg); } }
</style>