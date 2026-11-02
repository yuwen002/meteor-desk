<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Activity, ArrowDownUp, ArrowRight, Braces, CheckCircle2, ChevronRight, CircleHelp, Command, Copy, FileDown, FileSearch, FolderOpen, Gauge, HardDrive, History, Layers3, PieChart, Regex, Search, ShieldCheck, Star, Sun, Moon, TerminalSquare, Wrench, X } from 'lucide-vue-next'
import DeveloperToolkit from './components/DeveloperToolkit.vue'
import ProcessManagement from './components/ProcessManagement.vue'
import AboutDialog from './components/AboutDialog.vue'
import ShortcutsDialog from './components/ShortcutsDialog.vue'
import { createSerialTaskQueue } from './utils/serialTaskQueue'
import { appVersion } from './appMeta'
import { buildSearchExportCSV, buildSearchExportJSON, searchExportFileName, type ExportFormat } from './utils/exporters'
import { highlightSegments } from './utils/highlight'
import { LEVEL_LABEL, summaryText } from './utils/aggregate'
import { DEFAULT_SHORTCUTS, parseShortcutSettings, resolveShortcutAction, type ShortcutAction, type ShortcutMap } from './utils/shortcuts'
import { getBridge } from './utils/bridge'
import { TIME_WINDOW_PRESETS } from './utils/timeWindow'
import { searchMetaFrom, useLogSearch } from './composables/useLogSearch'
import { useResultView } from './composables/useResultView'
import { useJsonDetail } from './composables/useJsonDetail'
import { useBookmarks } from './composables/useBookmarks'
import { useLogTail } from './composables/useLogTail'

type PageKey='logs'|'processes'|'toolkit'
const activePage=ref<PageKey>('logs'), filePath=ref(''), query=ref(''), useRegex=ref(false), contextLines=ref(2), resultLimit=ref(500), encodingMode=ref('auto'), multiLineJSON=ref(false), eventContext=ref(true), eventBoundary=ref('timestamp'), eventBoundaryPattern=ref('')
// 时间窗（阶段 6 批次 A）：启用开关、相对快捷项、绝对起止与解释参照。
const timeWindowEnabled=ref(false), timeWindowPreset=ref(''), timeWindowStart=ref(''), timeWindowEnd=ref(''), timeWindowZone=ref<'local'|'utc'>('local')
const errorMessage=ref(''), response=ref<LogSearchResponse|null>(null), inputRef=ref<HTMLInputElement|null>(null)
const appSettings=ref<AppSettings>({recentFiles:[],searchHistory:[],contextLines:2,resultLimit:500})
const shortcutsConfig=ref<ShortcutMap>({...DEFAULT_SHORTCUTS})
const shortcutsDialogOpen=ref(false)
const aboutDialogOpen=ref(false)
const exportMessage=ref('')
const summaryOpen=ref(false), summaryMessage=ref('')
const themeMode=ref<'dark'|'light'>('dark')
// 跨模块协同：进程详情把落盘文件交给日志分析（openProcessLog）；日志分析按文件路径反查所属服务并跳回进程管理。
// processOwner 保存当前打开文件反查到的服务快照（非本工作台落盘文件时为空），processFocusId 请求进程管理页选中某个服务。
const processOwner=ref<ProcessStatus|null>(null), processFocusId=ref('')

// 检索状态机、结果视图、详情、收藏与实时跟踪分别交给 composable 持有，可脱离模板单测；
// App.vue 只留页面级职责：文件与设置、主题、快捷键、导出与高亮。
const { bookmarkKeys, fileBookmarks, isBookmarked, toggleMatchBookmark } = useBookmarks({ filePath, query, errorMessage })
const {
 resultFilter, bookmarkOnly, resultSort, timeFilter, selectedMatch, jsonFilter, resultListElement,
 filteredMatches, resultSummary, timeDistribution,
 facetPath, facetFilter, trendPath, fieldInfos, trendCandidates, activeFacet, activeTrend,
 selectFacetValue, facetValueActive, clearFacet, trendBarHeight, trendTitle, trendLabelVisible,
 virtualStart, visibleMatches, virtualTop, virtualBottom, selectedIndex,
 selectTimelineBucket, timelineActive, timelineHeight, timelineTitle, timelineLabelVisible,
 updateScroll, showMatch, moveMatch, resetViewState,
} = useResultView({ matches: () => response.value?.matches ?? [], filePath, bookmarkKeys })
const { busy, cancelRequested, searchProgress, lastSearchMeta, runSearch, resumeSearch, canResume, cancelSearch, attachEventUpdates, dispose: disposeSearch } = useLogSearch({
 input: { filePath, query, useRegex, contextLines, resultLimit, encodingMode, multiLineJSON, eventContext, eventBoundary, eventBoundaryPattern, timeWindowEnabled, timeWindowPreset, timeWindowStart, timeWindowEnd, timeWindowZone },
 response, errorMessage, resetViewState, onSearchStart, recordSearch, onSearchFinish,
})
const { detailContent, detailContentClipped, detailEventLabel, detailHint, parsedJSON, prettyJSON, jsonFields } = useJsonDetail({ selectedMatch, jsonFilter })
const { tailEnabled, tailLines, tailEncoding, tailError, tailTruncated, tailFollow, tailListElement, clearTail, startTail, stopTail, toggleTail, dispose: disposeTail } = useLogTail({ filePath, encodingMode, errorMessage })

const fileName=computed(()=>filePath.value.split(/[\\/]/).filter(Boolean).pop()||'')
// 命中高亮使用实际执行检索时的关键词与模式，避免修改输入框后标错位置。
function displaySegments(text:string){const meta=lastSearchMeta.value;return highlightSegments(text,meta?.query??query.value,meta?.useRegex??useRegex.value)}
function toggleResultSort(){resultSort.value=resultSort.value==='asc'?'desc':'asc'}
function clearResultFilter(){resultFilter.value=''}
function clearTimeFilter(){timeFilter.value=null}
// 「清除过滤」一次撤掉关键字、收藏、时间段与字段取值四个条件，让列表回到本次检索的原始结果。
function clearResultFilters(){resultFilter.value='';bookmarkOnly.value=false;timeFilter.value=null;clearFacet()}
function closeDetail(){selectedMatch.value=null}
async function copySummary(){
 const text=summaryText(resultSummary.value)
 try{await navigator.clipboard.writeText(text);summaryMessage.value='摘要已复制到剪贴板。'}
 catch{summaryMessage.value='复制失败，请检查剪贴板权限。'}
}
async function exportResults(format:ExportFormat){
 const a=getBridge(),current=response.value
 if(!a||!current?.matches.length)return
 // 导出所见即所得：应用结果过滤与排序后的列表，但不影响未过滤时的全量导出。
 const matches=filteredMatches.value
 if(!matches.length){errorMessage.value='当前过滤条件下没有可导出的结果。';return}
 exportMessage.value=''
 const meta=lastSearchMeta.value??searchMetaFrom({path:filePath.value,query:query.value,useRegex:useRegex.value,encoding:encodingMode.value,contextLines:contextLines.value,multiLineJSON:multiLineJSON.value,eventContext:eventContext.value},current)
 try{
  const content=format==='csv'?buildSearchExportCSV(meta,matches):buildSearchExportJSON(meta,matches)
  const saved=await a.SaveExportFile(searchExportFileName(meta,format),format,content)
  if(saved)exportMessage.value='已导出 '+matches.length+' 条检索结果到 '+saved
 }catch(e){errorMessage.value='导出检索结果失败：'+String(e)}
}

const fmtBytes=(bytes:number)=>{if(bytes<1024)return bytes+' B';const u=['KB','MB','GB','TB'];let v=bytes/1024,i=0;while(v>=1024&&i<u.length-1){v/=1024;i++}return v.toFixed(v>=100?0:1)+' '+u[i]}
const fileSize=computed(()=>fmtBytes(response.value?.fileSizeBytes??0))

let dropTimer:number|undefined
let pendingDroppedPaths:string[]=[]
const enqueueSettingsSave=createSerialTaskQueue()

function onSearchStart(){exportMessage.value='';summaryMessage.value=''}
// 历史去重与落盘属于页面职责：检索状态机只交出这次成功执行的参数。
async function recordSearch(record:SearchRecord){appSettings.value.searchHistory=[record,...appSettings.value.searchHistory.filter(x=>!(x.query===record.query&&x.path===record.path&&x.useRegex===record.useRegex&&x.contextLines===record.contextLines&&x.encoding===record.encoding))].slice(0,30);addRecent(record.path);await persistSettings()}
function onSearchFinish(){if(!pendingDroppedPaths.length)return;const paths=pendingDroppedPaths;pendingDroppedPaths=[];openDroppedFiles(paths)}

async function persistSettings(){const a=getBridge();if(!a)return;const settings:AppSettings={recentFiles:appSettings.value.recentFiles.slice(0,12),searchHistory:appSettings.value.searchHistory.slice(0,30),contextLines:contextLines.value,resultLimit:resultLimit.value};appSettings.value=settings;try{await enqueueSettingsSave(()=>a.SaveAppSettings(settings))}catch(e){errorMessage.value='保存本地设置失败：'+String(e)}}
function addRecent(path:string){if(!path)return;appSettings.value.recentFiles=[path,...appSettings.value.recentFiles.filter(x=>x.toLowerCase()!==path.toLowerCase())].slice(0,12);void persistSettings()}
async function loadSettings(){const a=getBridge();if(!a)return;try{appSettings.value=await a.GetAppSettings();contextLines.value=appSettings.value.contextLines??2;resultLimit.value=appSettings.value.resultLimit??500;if(appSettings.value.recentFiles.length)filePath.value=appSettings.value.recentFiles[0]}catch(e){errorMessage.value='读取设置失败：'+String(e)}}
function openDroppedFiles(paths:string[]){if(!paths.length)return;if(busy.value){pendingDroppedPaths=paths.slice();return}stopTail();clearTail();filePath.value=paths[0];response.value=null;resetViewState();addRecent(paths[0]);if(paths.length>1)errorMessage.value='已打开第一个文件；本次共拖入 '+paths.length+' 个文件。'}
async function chooseFile(){if(busy.value)return;errorMessage.value='';const a=getBridge();if(!a){errorMessage.value='请通过 Wails 桌面应用运行，才能使用系统文件选择器。';return}try{const p=await a.SelectLogFile();if(p&&!busy.value)openDroppedFiles([p])}catch(e){errorMessage.value=String(e)}}
async function pollDrop(){const a=getBridge();if(!a)return;try{const paths=await a.PollDroppedFiles();if(paths.length)openDroppedFiles(paths)}catch{/* 等待桌面运行时就绪 */}}
function applyHistory(r:SearchRecord){if(busy.value)return;stopTail();clearTail();filePath.value=r.path;query.value=r.query;useRegex.value=r.useRegex;contextLines.value=r.contextLines;encodingMode.value=r.encoding??'auto';multiLineJSON.value=false;activePage.value='logs';response.value=null;resetViewState()}
// 把进程落盘文件交给日志分析页：切页、载入文件并直接开始实时跟踪，复用 ReadLogTail 偏移增量读取。
function openProcessLog(path:string){if(!path)return;stopTail();clearTail();filePath.value=path;response.value=null;resetViewState();activePage.value='logs';addRecent(path);void nextTick(()=>startTail())}
// 按当前文件路径反查它是否属于某个已配置服务；命中时提供跳回进程管理的入口。
async function lookupProcessOwner(){const a=getBridge();if(!a||!filePath.value){processOwner.value=null;return}try{const snap=await a.FindServiceByLogPath(filePath.value);processOwner.value=snap&&snap.id?snap:null}catch{processOwner.value=null}}
function jumpToProcess(id?:string){if(!id)return;setPage('processes');processFocusId.value=id}
// 切换打开文件时都重新反查一次归属服务。
watch(filePath,()=>void lookupProcessOwner())

function loadTheme(){try{themeMode.value=window.localStorage.getItem('meteordesk.theme')==='light'?'light':'dark'}catch{themeMode.value='dark'}}
function toggleTheme(){themeMode.value=themeMode.value==='dark'?'light':'dark';try{window.localStorage.setItem('meteordesk.theme',themeMode.value)}catch{/* theme remains active for this session */}}
function loadShortcuts(){try{shortcutsConfig.value=parseShortcutSettings(window.localStorage.getItem('meteordesk.shortcuts.v1'))}catch{shortcutsConfig.value={...DEFAULT_SHORTCUTS}}}
function shortcutLabel(action:ShortcutAction){const key=shortcutsConfig.value[action];return 'Ctrl/⌘+'+(key==='Enter'?'Enter':key.toUpperCase())}
function setPage(p:PageKey){activePage.value=p}
function shortcuts(e:KeyboardEvent){if(!(e.ctrlKey||e.metaKey)||e.altKey)return;const k=e.key==='Enter'?'Enter':e.key.toLowerCase();const action=resolveShortcutAction(shortcutsConfig.value,k);if(!action)return;if(action==='runSearch'&&activePage.value!=='logs')return;e.preventDefault();switch(action){case'openFile':void chooseFile();break;case'focusSearch':setPage('logs');void nextTick(()=>inputRef.value?.focus());break;case'runSearch':if(!busy.value)void runSearch();break;case'logs':setPage('logs');break;case'processes':setPage('processes');break;case'toolkit':setPage('toolkit');break}}
onMounted(()=>{loadShortcuts();loadTheme();window.addEventListener('keydown',shortcuts);attachEventUpdates();void loadSettings();dropTimer=window.setInterval(()=>void pollDrop(),300)})
onBeforeUnmount(()=>{window.removeEventListener('keydown',shortcuts);disposeSearch();disposeTail();if(dropTimer!==undefined)clearInterval(dropTimer)})
</script>

<template>
<div class="app-shell" :class="{'theme-light':themeMode==='light'}" style="--wails-drop-target: drop">
 <aside class="sidebar">
  <div class="brand"><div class="brand-mark"><Activity :size="20"/></div><div class="brand-copy"><span class="brand-name">Meteor<span>Desk</span></span><span class="brand-caption">本地开发工作台</span></div></div>
  <div class="workspace-label">工作空间</div><nav class="nav-list" aria-label="主导航">
   <button class="nav-item" :class="{active:activePage==='logs'}" @click="setPage('logs')"><FileSearch :size="17"/><span>日志分析</span><kbd>1</kbd></button>
   <button class="nav-item" :class="{active:activePage==='processes'}" @click="setPage('processes')"><TerminalSquare :size="17"/><span>进程管理</span><kbd>2</kbd></button>
   <button class="nav-item" :class="{active:activePage==='toolkit'}" @click="setPage('toolkit')"><Wrench :size="17"/><span>开发工具</span><kbd>3</kbd></button>
  </nav>
  <div class="sidebar-divider"></div><div class="workspace-label">运行方式</div><div class="local-card"><div class="local-icon"><ShieldCheck :size="16"/></div><div><div class="local-title">本地优先</div><div class="local-description">文件仅在本机处理</div></div><span class="status-dot"></span></div>
  <div class="sidebar-bottom"><div class="version-row"><span>当前版本</span><button class="version-tag" type="button" title="查看版本信息" @click="aboutDialogOpen=true">v{{appVersion}}</button></div><div class="profile-row"><div class="avatar">MD</div><div class="profile-copy"><strong>MeteorDesk</strong><span>开发与运维工作台</span></div><button class="help-icon-button" type="button" aria-label="关于 MeteorDesk" title="关于 MeteorDesk" @click="aboutDialogOpen=true"><CircleHelp :size="17" class="help-icon"/></button></div></div>
 </aside>
 <main class="main-content">
  <header class="topbar"><div class="breadcrumbs"><span>工作空间</span><ChevronRight :size="14"/><strong>{{activePage==='logs'?'日志分析':activePage==='processes'?'进程管理':'开发工具'}}</strong></div><div class="topbar-right"><div class="runtime-status"><span class="status-dot"></span>本机运行</div><div class="topbar-divider"></div><button class="theme-toggle" type="button" :aria-label="themeMode==='dark'?'切换浅色主题':'切换深色主题'" :title="themeMode==='dark'?'切换浅色主题':'切换深色主题'" @click="toggleTheme"><Sun v-if="themeMode==='dark'" :size="15"/><Moon v-else :size="15"/></button><button class="shortcut-hint" type="button" title="配置快捷键" @click="shortcutsDialogOpen=true"><Command :size="13"/><span>快捷键</span></button></div></header>
  <div class="page-wrap">
   <section v-if="activePage==='logs'" class="page-content">
    <div class="page-heading"><div><div class="eyebrow"><span class="eyebrow-line"></span>可观测性 / 文件分析</div><h1>日志分析<span class="title-period">。</span></h1><p class="page-subtitle">逐块读取大体积日志，支持关键字、正则、上下文、跨行 JSON 文档检索、字段过滤和常见文本编码。</p></div><button class="button button-secondary" :disabled="busy" @click="chooseFile"><FolderOpen :size="16"/>选择日志</button></div>
    <div class="metric-grid">
     <div class="metric-card"><div class="metric-top"><span class="metric-icon mint"><Gauge :size="16"/></span><span class="metric-kicker">检索方式</span></div><div class="metric-value">流式扫描</div><div class="metric-foot"><CheckCircle2 :size="12"/>固定大小缓冲区</div></div>
     <div class="metric-card"><div class="metric-top"><span class="metric-icon blue"><HardDrive :size="16"/></span><span class="metric-kicker">数据处理</span></div><div class="metric-value">仅限本地</div><div class="metric-foot"><CheckCircle2 :size="12"/>不上传日志内容</div></div>
     <div class="metric-card"><div class="metric-top"><span class="metric-icon purple"><Layers3 :size="16"/></span><span class="metric-kicker">结果上限</span></div><div class="metric-value">{{resultLimit.toLocaleString()}} 条</div><div class="metric-foot">可调整结果数量</div></div>
    </div>
    <section class="search-panel">
     <div class="panel-heading"><div class="panel-heading-left"><div class="panel-icon"><Search :size="17"/></div><div><h2>查找日志</h2><p>输入关键字或正则表达式；事件上下文可按时间戳、空行或自定义正则聚合，详情可查看整段内容。</p></div></div><div class="panel-meta"><span class="live-dot"></span>本地检索</div></div>
     <div class="file-drop" :class="{'file-selected':!!filePath}"><div class="file-icon"><FileSearch :size="20"/></div><div class="file-info"><strong>{{fileName||'请选择要分析的日志文件'}}</strong><span>{{filePath||'支持 .log、.txt、.out、.json、.ndjson；也可将文件拖入窗口'}}</span></div><button class="button button-quiet" :disabled="busy" @click="chooseFile">{{filePath?'更换文件':'浏览文件'}} <ArrowRight :size="14"/></button></div>
     <details v-if="appSettings.recentFiles.length" class="history-panel"><summary><History :size="14"/>最近打开的文件 <span class="history-count">{{appSettings.recentFiles.length}}</span></summary><div class="history-items"><button v-for="p in appSettings.recentFiles" :key="p" class="history-item" :disabled="busy" @click="openDroppedFiles([p])"><FileSearch :size="13"/><span>{{p}}</span><ArrowRight :size="12"/></button></div></details>
     <form class="query-form" @submit.prevent="runSearch"><label class="field-label" for="search-query">搜索内容</label><div class="search-input-row"><div class="query-input-wrap"><Search :size="17" class="query-input-icon"/><input id="search-query" ref="inputRef" v-model="query" autocomplete="off" spellcheck="false" placeholder="输入关键字、错误码、请求 ID……"/><span class="input-shortcut">回车 ↵</span></div><button type="submit" class="button button-primary" :disabled="busy"><Search v-if="!busy" :size="16"/><span v-else class="spinner"></span>{{busy?'正在检索':'开始检索'}}</button><button v-if="busy" type="button" class="button button-secondary button-cancel" :disabled="cancelRequested" @click="cancelSearch"><X :size="15"/>{{cancelRequested?'正在取消':'取消检索'}}</button></div>
      <div class="query-options"><label class="toggle-option"><input v-model="useRegex" type="checkbox"/><span class="custom-checkbox"><Regex v-if="useRegex" :size="12"/></span><span>使用正则表达式</span></label><label class="toggle-option"><input v-model="eventContext" type="checkbox" :disabled="busy||multiLineJSON"/><span class="custom-checkbox"><CheckCircle2 v-if="eventContext" :size="12"/></span><span>聚合事件上下文</span></label><label class="option-select">事件边界 <select v-model="eventBoundary" :disabled="busy||!eventContext||multiLineJSON"><option value="timestamp">时间戳</option><option value="blank">空行</option><option value="custom">自定义正则</option></select></label><label v-if="eventContext&&!multiLineJSON&&eventBoundary==='custom'" class="option-input">边界正则 <input v-model="eventBoundaryPattern" :disabled="busy" placeholder="^--- " spellcheck="false" autocomplete="off"/></label><label class="toggle-option"><input v-model="multiLineJSON" type="checkbox" :disabled="busy"/><span class="custom-checkbox"><Braces v-if="multiLineJSON" :size="12"/></span><span>多行 JSON 文档</span></label><label class="option-select">上下文行数 <select v-model.number="contextLines" :disabled="multiLineJSON||eventContext"><option :value="0">不显示</option><option :value="2">前后 2 行</option><option :value="5">前后 5 行</option><option :value="10">前后 10 行</option></select></label><label class="option-select">结果上限 <select v-model.number="resultLimit"><option :value="500">500 条</option><option :value="1000">1,000 条</option><option :value="5000">5,000 条</option></select></label><label class="option-select">文本编码 <select v-model="encodingMode" :disabled="busy"><option value="auto">自动识别</option><option value="utf-8">UTF-8</option><option value="gb18030">GB18030 / GBK</option><option value="utf-16le">UTF-16 LE</option><option value="utf-16be">UTF-16 BE</option></select></label><span class="option-spacer"></span><span class="query-tip"><kbd>.*</kbd> Go 正则语法</span></div>
      <div class="query-options time-window-row"><label class="toggle-option"><input v-model="timeWindowEnabled" type="checkbox" :disabled="busy"/><span class="custom-checkbox"><CheckCircle2 v-if="timeWindowEnabled" :size="12"/></span><span>时间窗过滤</span></label><label class="option-select">相对时间 <select v-model="timeWindowPreset" :disabled="busy||!timeWindowEnabled"><option v-for="p in TIME_WINDOW_PRESETS" :key="p.value" :value="p.value">{{p.label}}</option></select></label><label class="option-input">开始 <input v-model="timeWindowStart" type="datetime-local" step="1" :disabled="busy||!timeWindowEnabled||!!timeWindowPreset"/></label><label class="option-input">结束 <input v-model="timeWindowEnd" type="datetime-local" step="1" :disabled="busy||!timeWindowEnabled||!!timeWindowPreset"/></label><label class="option-select">时区参照 <select v-model="timeWindowZone" :disabled="busy||!timeWindowEnabled"><option value="local">本机时区</option><option value="utc">UTC</option></select></label><span v-if="timeWindowEnabled&&!timeWindowPreset&&!timeWindowStart&&!timeWindowEnd" class="query-tip">未填起止或相对时间，本次将忽略时间窗</span><span v-else-if="timeWindowEnabled" class="query-tip">只保留行首时间戳落在窗口内的命中</span></div>
      <div v-if="busy&&searchProgress" class="progress-wrap"><div class="progress-meta"><span>{{cancelRequested?'正在停止检索':'正在扫描文件'}} · {{searchProgress.scannedLines.toLocaleString()}} 行</span><span>{{searchProgress.percent}}% · {{fmtBytes(searchProgress.bytesScanned)}} / {{fmtBytes(searchProgress.fileSizeBytes)}}</span></div><div class="progress-track"><div class="progress-bar" :style="{width:searchProgress.percent+'%'}"></div></div></div>
     </form>
     <details v-if="appSettings.searchHistory.length" class="history-panel query-history"><summary><History :size="14"/>最近检索 <span class="history-count">{{appSettings.searchHistory.length}}</span></summary><div class="history-items"><button v-for="(r,i) in appSettings.searchHistory" :key="r.createdAt+r.query+i" class="history-item" :disabled="busy" @click="applyHistory(r)"><Search :size="13"/><span><strong>{{r.query}}</strong><small>{{r.path}} · {{r.useRegex?'正则':'文本'}} · {{!r.encoding||r.encoding==='auto'?'自动识别':r.encoding}}</small></span><ArrowRight :size="12"/></button></div></details>
     <div v-if="errorMessage" class="notice notice-error"><span>!</span>{{errorMessage}}</div>
     <div v-if="filePath" class="tail-panel">
      <div v-if="processOwner" class="process-owner-hint"><Layers3 :size="14"/><span>该文件是服务“{{processOwner.name}}”的落盘输出</span><button class="button button-quiet" type="button" title="跳到进程管理并选中该服务" @click="jumpToProcess(processOwner.id)">在进程管理中查看 <ArrowRight :size="12"/></button></div>
      <div class="tail-heading"><div class="tail-title"><Activity :size="15"/><div><h3>实时跟踪</h3><p>{{tailEnabled?'每 1 秒读取新增行 · '+(tailEncoding||'自动识别')+' 编码':'只读取开启后的新增内容，不扫描整份文件。'}}</p></div></div><div class="tail-actions"><label v-if="tailEnabled" class="tail-follow"><input v-model="tailFollow" type="checkbox"/><span>跟随滚动</span></label><button class="button" :class="tailEnabled?'button-secondary':'button-primary'" type="button" @click="toggleTail"><Activity :size="13"/>{{tailEnabled?'停止跟踪':'开始跟踪'}}</button><button v-if="tailLines.length" class="button button-quiet" type="button" @click="clearTail">清空</button></div></div>
      <div v-if="tailError" class="notice notice-error tail-notice"><span>!</span>{{tailError}}</div>
      <div v-if="tailTruncated" class="tail-note">单次增量达到上限，部分行已跳过或截断；可清空后重新开始。</div>
      <div v-if="tailEnabled||tailLines.length" ref="tailListElement" class="tail-output"><div v-for="(line,i) in tailLines" :key="i" class="tail-line"><code>{{line}}</code></div><div v-if="!tailLines.length" class="tail-empty">{{tailEnabled?'等待新的日志行……':'上次跟踪的内容已清空。'}}</div></div>
     </div>
     <div class="results-header"><div><h3>检索结果 <span v-if="response" class="result-count">{{filteredMatches.length}}</span></h3><p>{{response?'已扫描 '+response.scannedLines.toLocaleString()+' 行 · '+fileSize+' · 编码 '+response.encoding:'检索结果会显示在这里。'}}</p></div><div v-if="response" class="results-actions"><button v-if="response.matches.length" class="button button-quiet" type="button" :disabled="busy" title="导出当前检索结果（CSV，含上下文）" @click="exportResults('csv')"><FileDown :size="13"/>CSV</button><button v-if="response.matches.length" class="button button-quiet" type="button" :disabled="busy" title="导出当前检索结果（JSON，含上下文）" @click="exportResults('json')"><FileDown :size="13"/>JSON</button><span v-if="response.timeWindowSkipped" class="limit-pill" title="启用时间窗后，行首缺少可解析日期时间戳的命中未计入结果">时间窗跳过 {{response.timeWindowSkipped}} 条无时间戳</span><button v-if="response.resultLimitReached&&canResume" class="button button-quiet resume-button" type="button" :disabled="busy" title="从上次截断的位置继续扫描剩余内容，保留已有结果" @click="resumeSearch"><ArrowRight :size="13"/>从这里继续</button><span v-else-if="response.resultLimitReached" class="limit-pill" title="该编码或检索模式暂不支持续扫；可提高结果上限或缩小时间窗后重新检索">已达到结果上限</span><span v-if="response.canceled" class="result-state">已取消</span><span v-else class="result-state"><span class="status-dot"></span>已完成</span></div></div>
     <div v-if="response&&response.matches.length" class="results-toolbar"><div class="result-filter"><Search :size="13"/><input v-model="resultFilter" placeholder="在当前结果中过滤：内容或行号" aria-label="过滤检索结果" spellcheck="false" autocomplete="off"/><button v-if="resultFilter" type="button" class="filter-clear" aria-label="清除过滤" @click="clearResultFilter"><X :size="12"/></button></div><label class="toolbar-toggle"><input v-model="bookmarkOnly" type="checkbox"/><span class="custom-checkbox"><Star v-if="bookmarkOnly" :size="11"/></span><span>仅看收藏</span><em>{{fileBookmarks}}</em></label><button class="button button-quiet sort-button" type="button" :title="resultSort==='asc'?'当前按行号升序，点击改为降序':'当前按行号降序，点击改为升序'" @click="toggleResultSort"><ArrowDownUp :size="13"/>{{resultSort==='asc'?'行号升序':'行号降序'}}</button><button class="button button-quiet summary-toggle" :class="{active:summaryOpen}" type="button" :title="summaryOpen?'收起聚合统计':'按日志级别与错误签名聚合当前结果'" @click="summaryOpen=!summaryOpen"><PieChart :size="13"/>统计</button><button v-if="timeFilter" class="button button-quiet time-filter-chip" type="button" title="清除时间段过滤" @click="clearTimeFilter">时间段 {{timeFilter.label}} <X :size="11"/></button><button v-if="facetFilter" class="button button-quiet facet-filter-chip" type="button" title="清除字段取值过滤" @click="clearFacet">{{facetFilter.path}}={{facetFilter.value}} <X :size="11"/></button><span class="filter-summary">{{filteredMatches.length}} / {{response.matches.length}} 条</span></div>
     <div v-if="summaryOpen&&filteredMatches.length" class="summary-panel">
      <div class="summary-head"><div><strong>结果摘要</strong><p>共 {{resultSummary.total}} 条，行号 {{resultSummary.firstLine}}–{{resultSummary.lastLine}}；仅统计当前过滤后的结果。</p></div><button class="button button-quiet" type="button" title="复制可粘贴到工单或聊天窗口的纯文本摘要" @click="copySummary"><Copy :size="13"/>复制摘要</button></div>
      <div class="summary-levels"><span v-for="item in resultSummary.levels" :key="item.level" class="level-chip" :class="'level-'+item.level">{{LEVEL_LABEL[item.level]}}<em>{{item.count}}</em></span><span v-if="!resultSummary.levels.length" class="summary-muted">未识别到日志级别。</span></div>
      <div v-if="resultSummary.signatures.length" class="summary-signatures"><div class="summary-subtitle">高频错误 / 警告 · Top {{resultSummary.signatures.length}}</div><ol><li v-for="item in resultSummary.signatures" :key="item.text"><code>{{item.text}}</code><em>{{item.count}} 次</em></li></ol></div>
      <p v-else class="summary-muted">当前结果中没有 error/warn 行，暂无错误签名。</p>
      <div v-if="timeDistribution.errorTotal||timeDistribution.warnTotal" class="summary-timeline">
       <div class="summary-subtitle">错误时间分布 · 每格 {{timeDistribution.stepLabel}} · ERROR {{timeDistribution.errorTotal}} / WARN {{timeDistribution.warnTotal}}</div>
       <div class="timeline-bars">
        <button v-for="(bucket,index) in timeDistribution.buckets" :key="bucket.start" type="button" class="timeline-bucket" :class="{active:timelineActive(bucket)}" :title="timelineTitle(bucket)" @click="selectTimelineBucket(bucket)">
         <div class="timeline-bar" :style="{height:timelineHeight(bucket.error+bucket.warn)+'px'}"><span v-if="bucket.warn" class="timeline-warn" :style="{flexGrow:bucket.warn}"></span><span v-if="bucket.error" class="timeline-error" :style="{flexGrow:bucket.error}"></span></div>
         <span class="timeline-label">{{timelineLabelVisible(index,timeDistribution.buckets.length)?bucket.label:''}}</span>
        </button>
       </div>
      </div>
      <p v-else class="summary-muted">当前结果中没有带时间戳的 error/warn 行，暂无时间分布。</p>
      <p v-if="timeDistribution.undated" class="summary-muted">{{timeDistribution.undated}} 条错误/警告行缺少可识别的时间戳，未计入时间分布。</p>
      <p v-if="timeDistribution.buckets.length" class="summary-muted timeline-tip">点击时间格可按该时间段过滤结果，再次点击取消。</p>
      <div v-if="fieldInfos.length" class="summary-facets">
       <div class="summary-subtitle">结构化字段分布 · 选择 JSON 字段后点击取值即可过滤</div>
       <label class="option-select facet-picker">分布字段 <select v-model="facetPath"><option value="">（未选择）</option><option v-for="field in fieldInfos" :key="field.path" :value="field.path">{{field.path}}（{{field.count}} 行）</option></select></label>
       <div v-if="activeFacet" class="facet-values">
        <button v-for="item in activeFacet.values" :key="item.value" type="button" class="facet-chip" :class="{active:facetValueActive(item.value)}" @click="selectFacetValue(item.value)">{{item.value}}<em>{{item.count}}</em></button>
        <span v-if="activeFacet.otherCount" class="facet-other">其他 {{activeFacet.otherDistinct}} 项 · {{activeFacet.otherCount}} 条</span>
        <p class="summary-muted">含该字段 {{activeFacet.presentCount}} 条，缺失 {{activeFacet.missingCount}} 条；共 {{activeFacet.values.length}} 个主要取值。</p>
       </div>
       <p v-else-if="facetPath" class="summary-muted">该字段暂无可统计的取值。</p>
      </div>
      <div v-if="trendCandidates.length" class="summary-trend">
       <div class="summary-subtitle">字段数值趋势 · 按自适应时间桶求和</div>
       <label class="option-select facet-picker">趋势字段 <select v-model="trendPath"><option value="">（未选择）</option><option v-for="field in trendCandidates" :key="field.path" :value="field.path">{{field.path}}（数值 {{field.numeric}} 行）</option></select></label>
       <div v-if="activeTrend&&activeTrend.buckets.length" class="trend-bars">
        <div v-for="(bucket,index) in activeTrend.buckets" :key="bucket.start" class="trend-bucket" :title="trendTitle(bucket)">
         <div class="trend-bar" :style="{height:trendBarHeight(bucket.sum)+'px'}"></div>
         <span class="timeline-label">{{trendLabelVisible(index,activeTrend.buckets.length)?bucket.label:''}}</span>
        </div>
       </div>
       <p v-else-if="trendPath" class="summary-muted">该字段没有同时带数值与行首时间戳的行，无法绘制趋势。</p>
       <p v-if="activeTrend&&activeTrend.undated" class="summary-muted">{{activeTrend.undated}} 行有数值但缺可识别时间戳，未计入趋势。</p>
      </div>
      <p v-if="summaryMessage" class="summary-note" role="status">{{summaryMessage}}</p>
     </div>
     <div v-if="exportMessage" class="notice notice-success"><span>✓</span>{{exportMessage}}</div>
     <div v-if="response&&filteredMatches.length" ref="resultListElement" class="result-list" @scroll="updateScroll"><div class="result-spacer" :style="{height:virtualTop+'px'}"></div><div v-for="(m,i) in visibleMatches" :key="m.lineNumber" class="result-row" :class="{selected:selectedMatch?.lineNumber===m.lineNumber}" role="button" tabindex="0" :title="m.content" @click="showMatch(m)" @keydown.enter="showMatch(m)"><span class="line-number">{{String(m.lineNumber).padStart(5,'0')}}</span><span class="result-marker" :class="{first:virtualStart+i===0}"></span><code><template v-for="(seg,si) in displaySegments(m.content)" :key="si"><mark v-if="seg.hit">{{seg.text}}</mark><template v-else>{{seg.text}}</template></template><span v-if="m.contentClipped" class="clipped-note"> [预览已截断]</span></code><button class="bookmark-button" :class="{active:isBookmarked(m)}" type="button" :aria-label="(isBookmarked(m)?'取消收藏第 ':'收藏第 ')+m.lineNumber+' 行'" :title="isBookmarked(m)?'取消收藏':'收藏该行'" @click.stop="toggleMatchBookmark(m)"><Star :size="12"/></button><ArrowRight :size="13" class="row-chevron"/></div><div class="result-spacer" :style="{height:virtualBottom+'px'}"></div></div>
     <div v-else-if="response&&response.matches.length" class="empty-results filtered-empty"><div class="empty-icon"><Search :size="20"/></div><strong>没有符合过滤条件的结果</strong><span>调整过滤关键字，或关闭「仅看收藏」。</span><button class="button button-quiet" type="button" @click="clearResultFilters">清除过滤</button></div>
     <div v-else-if="response" class="empty-results"><div class="empty-icon"><Search :size="20"/></div><strong>没有找到匹配内容</strong><span>尝试更换关键字，或启用正则表达式。</span></div>
     <div v-else class="empty-results initial-empty"><div class="empty-illustration"><div class="empty-orbit orbit-one"></div><div class="empty-orbit orbit-two"></div><div class="empty-center"><FileSearch :size="24"/></div><span class="orbit-dot dot-a"></span><span class="orbit-dot dot-b"></span></div><strong>准备就绪</strong><span>选择本地日志文件并输入搜索内容，即可开始分析。</span></div>
     <section v-if="selectedMatch" class="detail-panel">
      <div class="detail-heading"><div><h3>{{selectedMatch.event?detailEventLabel:'第'}} {{selectedMatch.event?selectedMatch.event.startLine.toLocaleString():selectedMatch.lineNumber.toLocaleString()}}<template v-if="selectedMatch.event"> - {{selectedMatch.event.endLine.toLocaleString()}} 行</template>详情</h3><p>{{detailHint}}</p></div><div class="detail-actions"><button class="button button-quiet" type="button" @click="toggleMatchBookmark(selectedMatch)"><Star :size="13"/>{{isBookmarked(selectedMatch)?'取消收藏':'收藏'}}</button><button class="button button-quiet" :disabled="selectedIndex<=0" @click="moveMatch(-1)">上一条</button><button class="button button-quiet" :disabled="selectedIndex<0||selectedIndex>=((filteredMatches.length||1)-1)" @click="moveMatch(1)">下一条</button><button class="icon-button" title="关闭详情" @click="closeDetail"><X :size="16"/></button></div></div>
      <div v-if="selectedMatch.contextBefore.length||selectedMatch.contextAfter.length" class="context-view"><div v-for="c in selectedMatch.contextBefore" :key="'b'+c.lineNumber" class="context-row"><span>{{c.lineNumber}}</span><code><template v-for="(seg,si) in displaySegments(c.content)" :key="si"><mark v-if="seg.hit">{{seg.text}}</mark><template v-else>{{seg.text}}</template></template></code></div><div class="context-row context-current"><span>{{selectedMatch.lineNumber}}</span><code><template v-for="(seg,si) in displaySegments(selectedMatch.content)" :key="si"><mark v-if="seg.hit">{{seg.text}}</mark><template v-else>{{seg.text}}</template></template></code></div><div v-for="c in selectedMatch.contextAfter" :key="'a'+c.lineNumber" class="context-row"><span>{{c.lineNumber}}</span><code><template v-for="(seg,si) in displaySegments(c.content)" :key="si"><mark v-if="seg.hit">{{seg.text}}</mark><template v-else>{{seg.text}}</template></template></code></div></div>
      <div v-if="parsedJSON" class="json-view"><div class="json-toolbar"><div><Braces :size="15"/><strong>JSON 结构</strong></div><input v-model="jsonFilter" placeholder="按字段路径或字段值筛选"/></div><div class="json-columns"><div class="json-raw"><div class="json-subtitle">格式化内容</div><pre>{{prettyJSON}}</pre></div><div class="json-fields"><div class="json-subtitle">字段列表 · {{jsonFields.length}} 项</div><div class="json-field-list"><div v-for="field in jsonFields" :key="field.path" class="json-field"><code>{{field.path}}</code><span>{{JSON.stringify(field.value)}}</span></div><div v-if="!jsonFields.length" class="json-empty">没有匹配字段</div></div></div></div></div>
      <div v-else class="raw-view"><div class="json-subtitle">{{selectedMatch.event?(selectedMatch.event.kind==='document'?'完整 JSON 文档':'完整事件内容'):'原始日志内容'}}</div><pre><template v-for="(seg,si) in displaySegments(detailContent)" :key="si"><mark v-if="seg.hit">{{seg.text}}</mark><template v-else>{{seg.text}}</template></template></pre><p>{{detailContentClipped?'上下文超过 512 KiB 安全上限，仅展示部分内容。':selectedMatch.event?'完整上下文已保留在本机内存中；可切换上一条/下一条继续查看。':'当前行不是可解析的 JSON 对象或数组；NDJSON 可逐行查看。'}}</p></div>
     </section>
     <div class="panel-footer"><span><span class="footer-led"></span>日志内容仅在本机处理</span><span>流式检索 <span class="footer-divider">/</span>单行预览最多 8 KiB</span></div>
    </section>
    <div class="bottom-note"><Braces :size="15"/><span>快捷键：{{shortcutLabel('openFile')}} 打开文件，{{shortcutLabel('focusSearch')}} 聚焦搜索，{{shortcutLabel('runSearch')}} 检索。</span><button @click="setPage('processes')">管理本地服务 <ArrowRight :size="13"/></button></div>
   </section>

   <ProcessManagement :active="activePage==='processes'" :focus-id="processFocusId" @open-log="openProcessLog" @focus-handled="processFocusId=''" />

   <DeveloperToolkit v-if="activePage==='toolkit'" />
  </div>
  <footer class="app-footer"><span>METEORDESK <span class="footer-divider">/</span>本地开发与运维工作台</span><span>专注效率 <span class="footer-divider">·</span>数据留在本机</span></footer>
 </main>
 <ShortcutsDialog v-if="shortcutsDialogOpen" :config="shortcutsConfig" @close="shortcutsDialogOpen=false" @change="shortcutsConfig=$event"/>
 <AboutDialog v-if="aboutDialogOpen" @close="aboutDialogOpen=false"/>
</div>
</template>
