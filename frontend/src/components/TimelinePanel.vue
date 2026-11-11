<script setup lang="ts">
import { computed, ref } from 'vue'
import { Activity, Filter, Layers3, Scan, X } from 'lucide-vue-next'
import { getBridge } from '../utils/bridge'
import { useTimeline } from '../composables/useTimeline'

const props = defineProps<{
  fileSpecs?: TimelineFileSpec[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'scan', files: TimelineFileSpec[]): void
}>()

const pickerOpen = ref(false)
const pickerPaths = ref<string[]>([])

const {
  busy, errorMessage, response, selectedEntry,
  levelFilter, sourceFilter,
  sourceColors, availableSources, filteredEntries, levelCounts,
  runScan, selectEntry, filterByLevel, filterBySource,
  levelBadgeClass, formatLevel, sortKeyForLevel,
} = useTimeline()

const hasFiles = computed(() => (props.fileSpecs?.length ?? 0) > 0)

async function addFile() {
  const a = getBridge()
  if (!a) return
  try {
    const p = await a.SelectLogFile()
    if (p) pickerPaths.value.push(p)
  } catch {}
}

function scanWithPicker() {
  if (!pickerPaths.value.length) return
  const specs: TimelineFileSpec[] = pickerPaths.value.map(p => ({
    path: p,
    sourceLabel: p.split(/[\\/]/).pop() ?? p,
  }))
  void runScan(specs, { maxEntries: 5000 })
  pickerOpen.value = false
}

async function scanFromContext() {
  const base = props.fileSpecs
  if (!base?.length) return
  void runScan(base, { maxEntries: 5000 })
}

function removePickerFile(idx: number) {
  pickerPaths.value.splice(idx, 1)
}

function triggerScan() {
  if (hasFiles.value) {
    void scanFromContext()
  } else {
    pickerOpen.value = true
  }
}

function entrySourceColor(entry: TimelineEntry): string {
  return sourceColors.value[entry.sourceLabel] ?? '#64748b'
}

function entryDateLabel(entry: TimelineEntry): string {
  if (entry.dateOnly) return entry.dateOnly
  if (!entry.timestampMs) return ''
  const d = new Date(entry.timestampMs)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

function entryTimeLabel(entry: TimelineEntry): string {
  if (entry.timeOnly) return entry.timeOnly
  if (!entry.timestampMs) return ''
  const d = new Date(entry.timestampMs)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`
}

const lastDateShown = ref<string>('')

function shouldShowDateDivider(entry: TimelineEntry, idx: number): boolean {
  if (idx === 0) return true
  const prev = filteredEntries.value[idx - 1]
  return entry.dateOnly !== prev.dateOnly || entry.dateOnly !== ''
}

function onEntryClick(entry: TimelineEntry) {
  selectEntry(entry)
}

function closeDetail() {
  selectEntry(null)
}
</script>

<template>
  <div class="timeline-panel">
    <header class="tl-header">
      <div class="tl-title">
        <Activity class="tl-icon" />
        <span>统一时间线</span>
        <span v-if="response" class="tl-meta">
          · {{ response.totalEntries }} 条
          <span v-if="response.limitReached" class="tl-limit">（已达上限）</span>
          <span v-if="response.canceled" class="tl-limit">（已取消）</span>
        </span>
      </div>
      <div class="tl-actions">
        <button v-if="!response" class="tl-btn primary" :disabled="busy" @click="triggerScan">
          <Scan v-if="!busy" class="tl-btn-icon" />
          {{ busy ? '扫描中…' : (hasFiles ? '使用当前文件扫描' : '选择文件') }}
        </button>
        <button v-else class="tl-btn" @click="triggerScan" :disabled="busy">
          <Scan v-if="!busy" class="tl-btn-icon" />
          重新扫描
        </button>
        <button class="tl-btn ghost" @click="emit('close')" title="关闭">
          <X class="tl-btn-icon" />
        </button>
      </div>
    </header>

    <div v-if="pickerOpen" class="tl-picker">
      <div class="tl-picker-head">
        <span>选择要合并的日志文件</span>
        <button class="tl-btn ghost" @click="pickerOpen=false">取消</button>
      </div>
      <div class="tl-picker-list">
        <div v-for="(p, i) in pickerPaths" :key="i" class="tl-picker-item">
          <Layers3 class="tl-picker-icon" />
          <span class="tl-picker-path">{{ p }}</span>
          <button class="tl-btn ghost tl-picker-remove" @click="removePickerFile(i)"><X class="tl-btn-icon" /></button>
        </div>
        <div v-if="!pickerPaths.length" class="tl-picker-empty">还没选择文件</div>
      </div>
      <div class="tl-picker-foot">
        <button class="tl-btn" @click="addFile">+ 添加文件</button>
        <button class="tl-btn primary" :disabled="!pickerPaths.length" @click="scanWithPicker">开始扫描</button>
      </div>
    </div>

    <div v-if="errorMessage" class="tl-error">{{ errorMessage }}</div>

    <div v-if="busy && !response" class="tl-busy">扫描中，正在并发处理多份日志文件…</div>

    <template v-if="response && filteredEntries.length">
      <div class="tl-filters">
        <div class="tl-filter-group">
          <Filter class="tl-filter-icon" />
          <button
            v-for="level in ['all', 'error', 'warn', 'info', 'debug', 'trace', 'other']"
            :key="level"
            class="tl-level-chip"
            :class="[`tlvl-${level}`, { active: levelFilter === level }]"
            @click="filterByLevel(level as any)"
          >
            {{ level === 'all' ? '全部' : formatLevel(level) }}
            <span v-if="level !== 'all'" class="tl-level-count">{{ levelCounts[level] ?? 0 }}</span>
          </button>
        </div>
        <div v-if="availableSources.length > 1" class="tl-filter-group">
          <span class="tl-source-label">来源：</span>
          <button
            class="tl-source-chip"
            :class="{ active: sourceFilter === 'all' }"
            @click="filterBySource('all')"
          >全部</button>
          <button
            v-for="src in availableSources"
            :key="src"
            class="tl-source-chip"
            :class="{ active: sourceFilter === src }"
            @click="filterBySource(src)"
          >
            <span class="tl-source-dot" :style="{ background: sourceColors[src] ?? '#64748b' }" />
            {{ src }}
          </button>
        </div>
      </div>

      <div class="tl-list">
        <template v-for="(entry, idx) in filteredEntries" :key="idx">
          <div v-if="shouldShowDateDivider(entry, idx)" class="tl-date-divider">
            {{ entryDateLabel(entry) }}
          </div>
          <div
            class="tl-row"
            :class="{ selected: selectedEntry?.lineNumber === entry.lineNumber && selectedEntry?.filePath === entry.filePath }"
            @click="onEntryClick(entry)"
          >
            <span class="tl-time">{{ entryTimeLabel(entry) || '——' }}</span>
            <span class="tl-source-bar" :style="{ background: entrySourceColor(entry) }" :title="entry.sourceLabel" />
            <span class="tl-level" :class="levelBadgeClass(entry.level)">
              {{ formatLevel(entry.level) }}
            </span>
            <span class="tl-content" :class="{ clipped: entry.contentClipped }">{{ entry.content }}</span>
          </div>
        </template>
        <div v-if="!filteredEntries.length" class="tl-empty">当前过滤条件下没有条目</div>
      </div>

      <div class="tl-footer">
        <span>共 {{ filteredEntries.length }} 条 / 总 {{ response.totalEntries }} 条</span>
      </div>
    </template>

    <div v-if="selectedEntry" class="tl-detail" @click.self="closeDetail">
      <div class="tl-detail-card">
        <div class="tl-detail-head">
          <div class="tl-detail-title">
            <span class="tl-level" :class="levelBadgeClass(selectedEntry.level)">
              {{ formatLevel(selectedEntry.level) }}
            </span>
            <span class="tl-detail-time">
              {{ entryDateLabel(selectedEntry) }} {{ entryTimeLabel(selectedEntry) }}
            </span>
            <span class="tl-detail-source" :style="{ borderColor: entrySourceColor(selectedEntry) }">
              {{ selectedEntry.sourceLabel }}
            </span>
          </div>
          <button class="tl-btn ghost" @click="closeDetail"><X class="tl-btn-icon" /></button>
        </div>
        <pre class="tl-detail-body">{{ selectedEntry.content }}</pre>
        <div class="tl-detail-meta">
          <span>文件：{{ selectedEntry.filePath }}</span>
          <span>行号：{{ selectedEntry.lineNumber }}</span>
        </div>
      </div>
    </div>

    <div v-if="!response && !busy && !errorMessage" class="tl-placeholder">
      <Activity class="tl-placeholder-icon" />
      <p>选择多份日志文件，快速合并成统一时间线视图</p>
      <p class="tl-placeholder-hint">跨服务调试时，按时间排序 + 来源着色能帮你更快理解事件先后关系</p>
      <button class="tl-btn primary" @click="triggerScan">开始</button>
    </div>
  </div>
</template>

<style scoped>
.timeline-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: #0f1320;
  color: #e2e8f0;
  font-family: -apple-system, 'Segoe UI', sans-serif;
}

.tl-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid #1e293b;
  background: linear-gradient(180deg, #141a2b 0%, #0f1320 100%);
}
.tl-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.02em;
}
.tl-icon { width: 16px; height: 16px; color: #6366f1; }
.tl-meta { color: #94a3b8; font-size: 12px; font-weight: 400; }
.tl-limit { color: #fbbf24; }

.tl-actions { display: flex; gap: 8px; }
.tl-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border: 1px solid #334155;
  border-radius: 6px;
  background: #1e293b;
  color: #e2e8f0;
  font-size: 12px;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
}
.tl-btn:hover:not(:disabled) { background: #334155; border-color: #475569; }
.tl-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.tl-btn.primary { background: #4f46e5; border-color: #6366f1; color: #fff; }
.tl-btn.primary:hover:not(:disabled) { background: #4338ca; }
.tl-btn.ghost { background: transparent; border-color: transparent; color: #94a3b8; }
.tl-btn.ghost:hover { background: #1e293b; border-color: #334155; color: #e2e8f0; }
.tl-btn-icon { width: 14px; height: 14px; }

.tl-picker {
  margin: 12px 16px;
  border: 1px solid #334155;
  border-radius: 8px;
  background: #141a2b;
  overflow: hidden;
}
.tl-picker-head {
  display: flex; justify-content: space-between; align-items: center;
  padding: 10px 14px;
  border-bottom: 1px solid #1e293b;
  font-size: 13px; font-weight: 500;
}
.tl-picker-list { padding: 8px; max-height: 200px; overflow-y: auto; }
.tl-picker-item {
  display: flex; align-items: center; gap: 8px;
  padding: 6px 8px; border-radius: 4px;
}
.tl-picker-item:hover { background: #1e293b; }
.tl-picker-icon { width: 14px; height: 14px; color: #64748b; flex-shrink: 0; }
.tl-picker-path { flex: 1; font-size: 12px; color: #cbd5e1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tl-picker-remove { padding: 4px; }
.tl-picker-empty { padding: 16px; text-align: center; color: #64748b; font-size: 12px; }
.tl-picker-foot { display: flex; justify-content: flex-end; gap: 8px; padding: 10px 14px; border-top: 1px solid #1e293b; }

.tl-error {
  margin: 12px 16px;
  padding: 10px 14px;
  border-radius: 6px;
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.3);
  color: #fca5a5;
  font-size: 12px;
}
.tl-busy {
  margin: 12px 16px;
  padding: 10px 14px;
  border-radius: 6px;
  background: rgba(99, 102, 241, 0.1);
  border: 1px solid rgba(99, 102, 241, 0.3);
  color: #a5b4fc;
  font-size: 12px;
}

.tl-filters {
  padding: 10px 16px;
  border-bottom: 1px solid #1e293b;
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
}
.tl-filter-group { display: flex; align-items: center; gap: 4px; flex-wrap: wrap; }
.tl-filter-icon { width: 14px; height: 14px; color: #64748b; }

.tl-level-chip {
  display: inline-flex; align-items: center; gap: 4px;
  padding: 3px 10px; border-radius: 12px;
  border: 1px solid #334155;
  background: transparent; color: #94a3b8;
  font-size: 11px; cursor: pointer;
  transition: all 0.15s;
}
.tl-level-chip:hover { border-color: #475569; color: #e2e8f0; }
.tl-level-chip.active { border-color: currentColor; }
.tlvl-all.active { background: #334155; color: #e2e8f0; }
.tlvl-error { color: #fca5a5; } .tlvl-error.active { background: rgba(239,68,68,0.15); }
.tlvl-warn { color: #fbbf24; } .tlvl-warn.active { background: rgba(251,191,36,0.15); }
.tlvl-info { color: #60a5fa; } .tlvl-info.active { background: rgba(96,165,250,0.15); }
.tlvl-debug { color: #a78bfa; } .tlvl-debug.active { background: rgba(167,139,250,0.15); }
.tlvl-trace { color: #34d399; } .tlvl-trace.active { background: rgba(52,211,153,0.15); }
.tlvl-other { color: #94a3b8; } .tlvl-other.active { background: rgba(148,163,184,0.15); }
.tl-level-count { opacity: 0.7; font-size: 10px; }

.tl-source-label { color: #64748b; font-size: 11px; }
.tl-source-chip {
  display: inline-flex; align-items: center; gap: 5px;
  padding: 3px 9px; border-radius: 12px;
  border: 1px solid #334155;
  background: transparent; color: #94a3b8;
  font-size: 11px; cursor: pointer;
}
.tl-source-chip:hover { border-color: #475569; }
.tl-source-chip.active { background: #1e293b; border-color: #6366f1; color: #e2e8f0; }
.tl-source-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }

.tl-list {
  flex: 1;
  overflow-y: auto;
  font-family: 'JetBrains Mono', 'Fira Code', Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
}
.tl-list::-webkit-scrollbar { width: 8px; }
.tl-list::-webkit-scrollbar-thumb { background: #334155; border-radius: 4px; }

.tl-date-divider {
  position: sticky;
  top: 0;
  z-index: 1;
  padding: 6px 16px;
  background: #0f1320;
  color: #94a3b8;
  font-size: 11px;
  font-weight: 600;
  border-bottom: 1px solid #1e293b;
  letter-spacing: 0.04em;
}

.tl-row {
  display: grid;
  grid-template-columns: 70px 3px 44px 1fr;
  gap: 0;
  align-items: start;
  padding: 4px 12px;
  cursor: pointer;
  border-bottom: 1px solid transparent;
}
.tl-row:hover { background: #141a2b; }
.tl-row.selected { background: #1e293b; border-bottom-color: #334155; border-top: 1px solid #334155; }
.tl-row.selected .tl-source-bar { height: 100%; }

.tl-time {
  padding: 0 8px 0 4px;
  color: #64748b;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  user-select: none;
}
.tl-source-bar {
  width: 3px;
  height: 18px;
  border-radius: 2px;
  flex-shrink: 0;
}
.tl-level {
  padding: 0 6px;
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  white-space: nowrap;
  align-self: center;
  font-family: -apple-system, 'Segoe UI', sans-serif;
}
.tlvl-error { color: #fca5a5; }
.tlvl-warn { color: #fbbf24; }
.tlvl-info { color: #60a5fa; }
.tlvl-debug { color: #a78bfa; }
.tlvl-trace { color: #34d399; }
.tlvl-other { color: #94a3b8; }

.tl-content {
  padding: 0 8px;
  color: #e2e8f0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tl-content.clipped { color: #94a3b8; }
.tl-row:hover .tl-content {
  text-overflow: clip;
}

.tl-empty { padding: 24px; text-align: center; color: #64748b; font-size: 12px; }

.tl-footer {
  padding: 8px 16px;
  border-top: 1px solid #1e293b;
  color: #64748b;
  font-size: 11px;
  display: flex; justify-content: flex-end;
}

.tl-detail {
  position: absolute;
  inset: 0;
  background: rgba(0,0,0,0.5);
  display: flex; align-items: center; justify-content: center;
  z-index: 100;
}
.tl-detail-card {
  width: min(720px, 90%);
  max-height: 80vh;
  background: #141a2b;
  border: 1px solid #334155;
  border-radius: 10px;
  display: flex; flex-direction: column;
  overflow: hidden;
}
.tl-detail-head {
  display: flex; align-items: center; gap: 8px;
  padding: 10px 14px;
  border-bottom: 1px solid #1e293b;
}
.tl-detail-title { flex: 1; display: flex; align-items: center; gap: 12px; }
.tl-detail-time { font-size: 12px; color: #94a3b8; font-variant-numeric: tabular-nums; }
.tl-detail-source {
  font-size: 11px; color: #cbd5e1;
  border: 1px solid #334155; border-radius: 4px;
  padding: 2px 8px;
}
.tl-detail-body {
  flex: 1;
  padding: 14px;
  margin: 0;
  overflow: auto;
  font-family: 'JetBrains Mono', 'Fira Code', Consolas, monospace;
  font-size: 12px;
  line-height: 1.6;
  color: #e2e8f0;
  white-space: pre-wrap;
  word-break: break-all;
}
.tl-detail-meta {
  padding: 8px 14px;
  border-top: 1px solid #1e293b;
  display: flex; justify-content: space-between;
  font-size: 11px; color: #64748b;
}

.tl-placeholder {
  flex: 1;
  display: flex; flex-direction: column; align-items: center; justify-content: center;
  text-align: center;
  color: #94a3b8;
  padding: 40px;
  gap: 8px;
}
.tl-placeholder-icon { width: 40px; height: 40px; color: #6366f1; margin-bottom: 8px; }
.tl-placeholder p { margin: 0; font-size: 14px; }
.tl-placeholder-hint { font-size: 12px; color: #64748b; max-width: 320px; }
.tl-placeholder .tl-btn { margin-top: 12px; }
</style>