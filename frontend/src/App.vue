<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { Activity, ArrowRight, Braces, CheckCircle2, ChevronRight, CircleHelp, Command, FileSearch, FolderOpen, Gauge, HardDrive, History, Layers3, Regex, Search, ShieldCheck, Sun, Moon, TerminalSquare, Wrench, X } from 'lucide-vue-next'
import DeveloperToolkit from './components/DeveloperToolkit.vue'
import ProcessManagement from './components/ProcessManagement.vue'
import { createSerialTaskQueue } from './utils/serialTaskQueue'
import { appendJSONPath } from './utils/jsonPath'
import { DEFAULT_SHORTCUTS, SHORTCUT_OPTIONS, parseShortcutSettings, resolveShortcutAction, validateShortcutMap, type ShortcutAction, type ShortcutMap } from './utils/shortcuts'

type PageKey='logs'|'processes'|'toolkit'
const activePage=ref<PageKey>('logs'), filePath=ref(''), query=ref(''), useRegex=ref(false), contextLines=ref(2), resultLimit=ref(500), encodingMode=ref('auto'), multiLineJSON=ref(false)
const busy=ref(false), cancelRequested=ref(false), errorMessage=ref(''), response=ref<LogSearchResponse|null>(null), searchProgress=ref<LogSearchProgress|null>(null), searchID=ref('')
const inputRef=ref<HTMLInputElement|null>(null), selectedMatch=ref<LogMatch|null>(null), jsonFilter=ref(''), scrollTop=ref(0), resultListElement=ref<HTMLElement|null>(null)
const appSettings=ref<AppSettings>({recentFiles:[],searchHistory:[],contextLines:2,resultLimit:500})
const shortcutsConfig=ref<ShortcutMap>({...DEFAULT_SHORTCUTS})
const shortcutsDialogOpen=ref(false)
const shortcutsError=ref('')
const themeMode=ref<'dark'|'light'>('dark')
function loadTheme(){try{themeMode.value=window.localStorage.getItem('meteordesk.theme')==='light'?'light':'dark'}catch{themeMode.value='dark'}}
function toggleTheme(){themeMode.value=themeMode.value==='dark'?'light':'dark';try{window.localStorage.setItem('meteordesk.theme',themeMode.value)}catch{/* theme remains active for this session */}}
const shortcutActions:{key:ShortcutAction;label:string;description:string}[]=[
 {key:'openFile',label:'打开日志文件',description:'打开系统文件选择器'},
 {key:'focusSearch',label:'聚焦搜索框',description:'切换到日志分析并聚焦搜索输入'},
 {key:'runSearch',label:'开始检索',description:'在日志分析页面启动检索'},
 {key:'logs',label:'日志分析页面',description:'切换到日志分析'},
 {key:'processes',label:'进程管理页面',description:'切换到本地服务管理'},
 {key:'toolkit',label:'开发工具页面',description:'切换到开发工具箱'},
]
function loadShortcuts(){try{shortcutsConfig.value=parseShortcutSettings(window.localStorage.getItem('meteordesk.shortcuts.v1'))}catch{shortcutsConfig.value={...DEFAULT_SHORTCUTS}}}
function saveShortcuts(){const validationError=validateShortcutMap(shortcutsConfig.value);if(validationError){shortcutsError.value=validationError;return}try{window.localStorage.setItem('meteordesk.shortcuts.v1',JSON.stringify(shortcutsConfig.value));shortcutsError.value='';shortcutsDialogOpen.value=false}catch{shortcutsError.value='无法保存快捷键设置，请检查本地存储权限。'}}
function resetShortcuts(){shortcutsConfig.value={...DEFAULT_SHORTCUTS};shortcutsError.value=''}
function shortcutLabel(action:ShortcutAction){const key=shortcutsConfig.value[action];return 'Ctrl/⌘+'+(key==='Enter'?'Enter':key.toUpperCase())}




const rowHeight=42
const virtualStart=computed(()=>Math.max(0,Math.floor(scrollTop.value/rowHeight)-3))
const visibleMatches=computed(()=>(response.value?.matches??[]).slice(virtualStart.value,virtualStart.value+24))
const virtualTop=computed(()=>virtualStart.value*rowHeight)
const virtualBottom=computed(()=>Math.max(0,((response.value?.matches.length??0)-virtualStart.value-visibleMatches.value.length)*rowHeight))
const selectedIndex=computed(()=>response.value?.matches.findIndex(m=>m.lineNumber===selectedMatch.value?.lineNumber)??-1)
const fileName=computed(()=>filePath.value.split(/[\\/]/).filter(Boolean).pop()||'')

const fmtBytes=(bytes:number)=>{if(bytes<1024)return bytes+' B';const u=['KB','MB','GB','TB'];let v=bytes/1024,i=0;while(v>=1024&&i<u.length-1){v/=1024;i++}return v.toFixed(v>=100?0:1)+' '+u[i]}
const fileSize=computed(()=>fmtBytes(response.value?.fileSizeBytes??0))
const parsedJSON=computed(()=>{if(!selectedMatch.value||selectedMatch.value.contentClipped)return null;try{const v=JSON.parse(selectedMatch.value.content);return v!==null&&typeof v==='object'?v:null}catch{return null}})
const prettyJSON=computed(()=>parsedJSON.value===null?'':JSON.stringify(parsedJSON.value,null,2))
const jsonFields=computed(()=>{const out:JsonField[]=[];const add=(v:unknown,path:string,depth:number)=>{if(out.length>=300||depth>8)return;if(Array.isArray(v)){if(!v.length)out.push({path,value:[]});v.forEach((x,i)=>add(x,path+'['+i+']',depth+1))}else if(v&&typeof v==='object'){const e=Object.entries(v as Record<string,unknown>);if(!e.length)out.push({path,value:{}});for(const [k,x] of e)add(x,appendJSONPath(path,k),depth+1)}else out.push({path,value:v})};if(parsedJSON.value!==null)add(parsedJSON.value,'$',0);const q=jsonFilter.value.trim().toLowerCase();return q?out.filter(x=>x.path.toLowerCase().includes(q)||JSON.stringify(x.value).toLowerCase().includes(q)):out})
let progressTimer:number|undefined, dropTimer:number|undefined
let partialMatchCursor=0
let progressPollInFlight=false
let completedSearchID=''
let pendingDroppedPaths:string[]=[]
let stopSearchUpdates:(()=>void)|undefined
const enqueueSettingsSave=createSerialTaskQueue()

function api(){return window.go?.main?.App}
function clearTimer(which:'progress'|'drop'){if(which==='progress'&&progressTimer!==undefined){clearInterval(progressTimer);progressTimer=undefined}if(which==='drop'&&dropTimer!==undefined){clearInterval(dropTimer);dropTimer=undefined}}
async function persistSettings(){const a=api();if(!a)return;const settings:AppSettings={recentFiles:appSettings.value.recentFiles.slice(0,12),searchHistory:appSettings.value.searchHistory.slice(0,30),contextLines:contextLines.value,resultLimit:resultLimit.value};appSettings.value=settings;try{await enqueueSettingsSave(()=>a.SaveAppSettings(settings))}catch(e){errorMessage.value='保存本地设置失败：'+String(e)}}
function addRecent(path:string){if(!path)return;appSettings.value.recentFiles=[path,...appSettings.value.recentFiles.filter(x=>x.toLowerCase()!==path.toLowerCase())].slice(0,12);void persistSettings()}
async function loadSettings(){const a=api();if(!a)return;try{appSettings.value=await a.GetAppSettings();contextLines.value=appSettings.value.contextLines??2;resultLimit.value=appSettings.value.resultLimit??500;if(appSettings.value.recentFiles.length)filePath.value=appSettings.value.recentFiles[0]}catch(e){errorMessage.value='读取设置失败：'+String(e)}}
function openDroppedFiles(paths:string[]){if(!paths.length)return;if(busy.value){pendingDroppedPaths=paths.slice();return}filePath.value=paths[0];response.value=null;selectedMatch.value=null;scrollTop.value=0;addRecent(paths[0]);if(paths.length>1)errorMessage.value='已打开第一个文件；本次共拖入 '+paths.length+' 个文件。'}
async function chooseFile(){if(busy.value)return;errorMessage.value='';const a=api();if(!a){errorMessage.value='请通过 Wails 桌面应用运行，才能使用系统文件选择器。';return}try{const p=await a.SelectLogFile();if(p&&!busy.value)openDroppedFiles([p])}catch(e){errorMessage.value=String(e)}}
async function pollDrop(){const a=api();if(!a)return;try{const paths=await a.PollDroppedFiles();if(paths.length)openDroppedFiles(paths)}catch{/* 等待桌面运行时就绪 */}}
// 每轮只追加尚未展示的匹配项，并同步进度元数据；不重复拉取完整结果集。
async function pollProgress(id:string){
 const a=api()
 if(!a||progressPollInFlight||completedSearchID===id)return
 progressPollInFlight=true
 try{
  const p=await a.GetLogSearchProgress(id)
  if(searchID.value!==id||completedSearchID===id)return
  searchProgress.value=p
  if(response.value){
   response.value={...response.value,fileSizeBytes:p.fileSizeBytes||response.value.fileSizeBytes,bytesScanned:p.bytesScanned,scannedLines:p.scannedLines,canceled:p.canceled}
  }
  const cursor=partialMatchCursor
  const updates=await a.GetLogSearchMatches(id,cursor)
  if(searchID.value!==id||completedSearchID===id||cursor!==partialMatchCursor||!updates.length)return
  partialMatchCursor+=updates.length
  const current=response.value
  if(current){
   response.value={...current,fileSizeBytes:p.fileSizeBytes||current.fileSizeBytes,bytesScanned:p.bytesScanned,scannedLines:p.scannedLines,matches:[...current.matches,...updates]}
  }
 }catch{/* 刚创建任务时可能尚未登记，下一轮重试 */}
 finally{progressPollInFlight=false}
}
// Wails 事件用于低延迟追加结果；保留轮询作为兼容回退，并共享游标防止重复追加。
function onSearchUpdate(event:SearchMatchesEvent){
 if(event.searchId!==searchID.value||completedSearchID===event.searchId)return
 searchProgress.value=event.progress
 const current=response.value
 if(!current)return
 const updates=event.matches??[]
 if(!updates.length){
  response.value={...current,fileSizeBytes:event.progress.fileSizeBytes||current.fileSizeBytes,bytesScanned:event.progress.bytesScanned,scannedLines:event.progress.scannedLines,canceled:event.progress.canceled}
  return
 }
 partialMatchCursor+=updates.length
 response.value={...current,fileSizeBytes:event.progress.fileSizeBytes||current.fileSizeBytes,bytesScanned:event.progress.bytesScanned,scannedLines:event.progress.scannedLines,canceled:event.progress.canceled,matches:[...current.matches,...updates]}
}
async function cancelSearch(){const a=api();if(!a||!searchID.value)return;cancelRequested.value=true;try{await a.CancelLogSearch(searchID.value)}catch(e){errorMessage.value='取消检索失败：'+String(e)}}
async function runSearch(){errorMessage.value='';if(busy.value)return;if(!filePath.value){errorMessage.value='请先选择日志文件。';return}if(!query.value){errorMessage.value='请输入关键字或正则表达式。';return}const a=api();if(!a){errorMessage.value='未检测到桌面桥接服务，请通过 wails dev 启动。';return}
 const input={path:filePath.value,query:query.value,useRegex:useRegex.value,resultLimit:resultLimit.value,contextLines:contextLines.value,encoding:encodingMode.value,multiLineJSON:multiLineJSON.value}
 clearTimer('progress');const id=Date.now()+'-'+Math.random().toString(16).slice(2);searchID.value=id;completedSearchID='';cancelRequested.value=false;busy.value=true;partialMatchCursor=0;response.value={filePath:filePath.value,encoding:'',fileSizeBytes:0,bytesScanned:0,scannedLines:0,matches:[],resultLimitReached:false,canceled:false};selectedMatch.value=null;scrollTop.value=0
 searchProgress.value={searchId:id,bytesScanned:0,fileSizeBytes:0,scannedLines:0,matchesFound:0,percent:0,running:true,canceled:false}
 progressTimer=window.setInterval(()=>void pollProgress(id),250)
 try{
  const completed=await a.SearchLogsAdvanced(id,input.path,input.query,input.useRegex,input.resultLimit,input.contextLines,input.encoding,input.multiLineJSON)
  response.value=completed
  partialMatchCursor=completed.matches.length
  completedSearchID=id
  let finalPercent=100
  if(completed.canceled||completed.resultLimitReached){
   finalPercent=completed.fileSizeBytes>0?Math.min(99,Math.floor(completed.bytesScanned*100/completed.fileSizeBytes)):0
  }
  searchProgress.value={searchId:id,bytesScanned:completed.bytesScanned,fileSizeBytes:completed.fileSizeBytes,scannedLines:completed.scannedLines,matchesFound:completed.matches.length,percent:finalPercent,running:false,canceled:completed.canceled}
  await pollProgress(id)
  if(response.value.canceled){errorMessage.value='检索已取消，以下保留已找到的部分结果。'}else{const record:SearchRecord={query:input.query,path:input.path,useRegex:input.useRegex,contextLines:input.contextLines,encoding:input.encoding,createdAt:new Date().toISOString()};appSettings.value.searchHistory=[record,...appSettings.value.searchHistory.filter(x=>!(x.query===record.query&&x.path===record.path&&x.useRegex===record.useRegex&&x.contextLines===record.contextLines&&x.encoding===record.encoding))].slice(0,30);addRecent(input.path);await persistSettings()}}
 catch(e){errorMessage.value=String(e)}finally{clearTimer('progress');busy.value=false;cancelRequested.value=false;if(pendingDroppedPaths.length){const paths=pendingDroppedPaths;pendingDroppedPaths=[];openDroppedFiles(paths)}}}
function applyHistory(r:SearchRecord){if(busy.value)return;filePath.value=r.path;query.value=r.query;useRegex.value=r.useRegex;contextLines.value=r.contextLines;encodingMode.value=r.encoding??'auto';multiLineJSON.value=false;activePage.value='logs';response.value=null;selectedMatch.value=null;scrollTop.value=0}
function updateScroll(e:Event){if(e.target instanceof HTMLElement)scrollTop.value=e.target.scrollTop}
async function moveMatch(delta:number){
 const all=response.value?.matches??[]
 if(!all.length)return
 const from=selectedIndex.value
 const target=Math.max(0,Math.min(all.length-1,from<0?(delta>0?0:all.length-1):from+delta))
 selectedMatch.value=all[target]
 jsonFilter.value=''
 await nextTick()
 if(resultListElement.value)resultListElement.value.scrollTop=Math.max(0,target*rowHeight-3*rowHeight)
}
function showMatch(m:LogMatch){selectedMatch.value=m;jsonFilter.value=''}
function setPage(p:PageKey){activePage.value=p}
function shortcuts(e:KeyboardEvent){if(!(e.ctrlKey||e.metaKey)||e.altKey)return;const k=e.key==='Enter'?'Enter':e.key.toLowerCase();const action=resolveShortcutAction(shortcutsConfig.value,k);if(!action)return;if(action==='runSearch'&&activePage.value!=='logs')return;e.preventDefault();switch(action){case'openFile':void chooseFile();break;case'focusSearch':setPage('logs');void nextTick(()=>inputRef.value?.focus());break;case'runSearch':if(!busy.value)void runSearch();break;case'logs':setPage('logs');break;case'processes':setPage('processes');break;case'toolkit':setPage('toolkit');break}}
onMounted(()=>{loadShortcuts();loadTheme();window.addEventListener('keydown',shortcuts);stopSearchUpdates=window.runtime?.EventsOn('log-search:update',onSearchUpdate);void loadSettings();dropTimer=window.setInterval(()=>void pollDrop(),300)})
onBeforeUnmount(()=>{window.removeEventListener('keydown',shortcuts);stopSearchUpdates?.();stopSearchUpdates=undefined;clearTimer('progress');clearTimer('drop')})
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
  <div class="sidebar-bottom"><div class="version-row"><span>早期版本</span><span class="version-tag">v0.5.19</span></div><div class="profile-row"><div class="avatar">MD</div><div class="profile-copy"><strong>MeteorDesk</strong><span>开发与运维工作台</span></div><CircleHelp :size="17" class="help-icon"/></div></div>
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
     <div class="panel-heading"><div class="panel-heading-left"><div class="panel-icon"><Search :size="17"/></div><div><h2>查找日志</h2><p>输入关键字或正则表达式，定位匹配行；多行 JSON 模式会将完整对象或数组作为一个匹配单元。</p></div></div><div class="panel-meta"><span class="live-dot"></span>本地检索</div></div>
     <div class="file-drop" :class="{'file-selected':!!filePath}"><div class="file-icon"><FileSearch :size="20"/></div><div class="file-info"><strong>{{fileName||'请选择要分析的日志文件'}}</strong><span>{{filePath||'支持 .log、.txt、.out、.json、.ndjson；也可将文件拖入窗口'}}</span></div><button class="button button-quiet" :disabled="busy" @click="chooseFile">{{filePath?'更换文件':'浏览文件'}} <ArrowRight :size="14"/></button></div>
     <details v-if="appSettings.recentFiles.length" class="history-panel"><summary><History :size="14"/>最近打开的文件 <span class="history-count">{{appSettings.recentFiles.length}}</span></summary><div class="history-items"><button v-for="p in appSettings.recentFiles" :key="p" class="history-item" :disabled="busy" @click="openDroppedFiles([p])"><FileSearch :size="13"/><span>{{p}}</span><ArrowRight :size="12"/></button></div></details>
     <form class="query-form" @submit.prevent="runSearch"><label class="field-label" for="search-query">搜索内容</label><div class="search-input-row"><div class="query-input-wrap"><Search :size="17" class="query-input-icon"/><input id="search-query" ref="inputRef" v-model="query" autocomplete="off" spellcheck="false" placeholder="输入关键字、错误码、请求 ID……"/><span class="input-shortcut">回车 ↵</span></div><button type="submit" class="button button-primary" :disabled="busy"><Search v-if="!busy" :size="16"/><span v-else class="spinner"></span>{{busy?'正在检索':'开始检索'}}</button><button v-if="busy" type="button" class="button button-secondary button-cancel" :disabled="cancelRequested" @click="cancelSearch"><X :size="15"/>{{cancelRequested?'正在取消':'取消检索'}}</button></div>
      <div class="query-options"><label class="toggle-option"><input v-model="useRegex" type="checkbox"/><span class="custom-checkbox"><Regex v-if="useRegex" :size="12"/></span><span>使用正则表达式</span></label><label class="toggle-option"><input v-model="multiLineJSON" type="checkbox" :disabled="busy"/><span class="custom-checkbox"><Braces v-if="multiLineJSON" :size="12"/></span><span>多行 JSON 文档</span></label><label class="option-select">上下文行数 <select v-model.number="contextLines"><option :value="0">不显示</option><option :value="2">前后 2 行</option><option :value="5">前后 5 行</option><option :value="10">前后 10 行</option></select></label><label class="option-select">结果上限 <select v-model.number="resultLimit"><option :value="500">500 条</option><option :value="1000">1,000 条</option><option :value="5000">5,000 条</option></select></label><label class="option-select">文本编码 <select v-model="encodingMode" :disabled="busy"><option value="auto">自动识别</option><option value="utf-8">UTF-8</option><option value="gb18030">GB18030 / GBK</option><option value="utf-16le">UTF-16 LE</option><option value="utf-16be">UTF-16 BE</option></select></label><span class="option-spacer"></span><span class="query-tip"><kbd>.*</kbd> Go 正则语法</span></div>
      <div v-if="busy&&searchProgress" class="progress-wrap"><div class="progress-meta"><span>{{cancelRequested?'正在停止检索':'正在扫描文件'}} · {{searchProgress.scannedLines.toLocaleString()}} 行</span><span>{{searchProgress.percent}}%　{{fmtBytes(searchProgress.bytesScanned)}} / {{fmtBytes(searchProgress.fileSizeBytes)}}</span></div><div class="progress-track"><div class="progress-bar" :style="{width:searchProgress.percent+'%'}"></div></div></div>
     </form>
     <details v-if="appSettings.searchHistory.length" class="history-panel query-history"><summary><History :size="14"/>最近检索 <span class="history-count">{{appSettings.searchHistory.length}}</span></summary><div class="history-items"><button v-for="(r,i) in appSettings.searchHistory" :key="r.createdAt+r.query+i" class="history-item" :disabled="busy" @click="applyHistory(r)"><Search :size="13"/><span><strong>{{r.query}}</strong><small>{{r.path}} · {{r.useRegex?'正则':'文本'}} · {{!r.encoding||r.encoding==='auto'?'自动识别':r.encoding}}</small></span><ArrowRight :size="12"/></button></div></details>
     <div v-if="errorMessage" class="notice notice-error"><span>!</span>{{errorMessage}}</div>
     <div class="results-header"><div><h3>检索结果 <span v-if="response" class="result-count">{{response.matches.length}}</span></h3><p>{{response?'已扫描 '+response.scannedLines.toLocaleString()+' 行 · '+fileSize+' · 编码 '+response.encoding:'检索结果会显示在这里。'}}</p></div><div v-if="response" class="results-actions"><span v-if="response.resultLimitReached" class="limit-pill">已达到结果上限</span><span v-if="response.canceled" class="result-state">已取消</span><span v-else class="result-state"><span class="status-dot"></span>已完成</span></div></div>
     <div v-if="response&&response.matches.length" ref="resultListElement" class="result-list" @scroll="updateScroll"><div class="result-spacer" :style="{height:virtualTop+'px'}"></div><div v-for="(m,i) in visibleMatches" :key="m.lineNumber" class="result-row" :class="{selected:selectedMatch?.lineNumber===m.lineNumber}" role="button" tabindex="0" :title="m.content" @click="selectedMatch=m;jsonFilter=''" @keydown.enter="selectedMatch=m"><span class="line-number">{{String(m.lineNumber).padStart(5,'0')}}</span><span class="result-marker" :class="{first:virtualStart+i===0}"></span><code>{{m.content}}<span v-if="m.contentClipped" class="clipped-note"> [预览已截断]</span></code><ArrowRight :size="13" class="row-chevron"/></div><div class="result-spacer" :style="{height:virtualBottom+'px'}"></div></div>
     <div v-else-if="response" class="empty-results"><div class="empty-icon"><Search :size="20"/></div><strong>没有找到匹配内容</strong><span>尝试更换关键字，或启用正则表达式。</span></div>
     <div v-else class="empty-results initial-empty"><div class="empty-illustration"><div class="empty-orbit orbit-one"></div><div class="empty-orbit orbit-two"></div><div class="empty-center"><FileSearch :size="24"/></div><span class="orbit-dot dot-a"></span><span class="orbit-dot dot-b"></span></div><strong>准备就绪</strong><span>选择本地日志文件并输入搜索内容，即可开始分析。</span></div>
     <section v-if="selectedMatch" class="detail-panel">
      <div class="detail-heading"><div><h3>第 {{selectedMatch.lineNumber.toLocaleString()}} 行详情</h3><p>查看前后文；JSON 行可格式化并按字段筛选。</p></div><div class="detail-actions"><button class="button button-quiet" :disabled="selectedIndex<=0" @click="moveMatch(-1)">上一条</button><button class="button button-quiet" :disabled="selectedIndex<0||selectedIndex>=((response?.matches.length??1)-1)" @click="moveMatch(1)">下一条</button><button class="icon-button" title="关闭详情" @click="selectedMatch=null"><X :size="16"/></button></div></div>
      <div v-if="selectedMatch.contextBefore.length||selectedMatch.contextAfter.length" class="context-view"><div v-for="c in selectedMatch.contextBefore" :key="'b'+c.lineNumber" class="context-row"><span>{{c.lineNumber}}</span><code>{{c.content}}</code></div><div class="context-row context-current"><span>{{selectedMatch.lineNumber}}</span><code>{{selectedMatch.content}}</code></div><div v-for="c in selectedMatch.contextAfter" :key="'a'+c.lineNumber" class="context-row"><span>{{c.lineNumber}}</span><code>{{c.content}}</code></div></div>
      <div v-if="parsedJSON" class="json-view"><div class="json-toolbar"><div><Braces :size="15"/><strong>JSON 结构</strong></div><input v-model="jsonFilter" placeholder="按字段路径或字段值筛选"/></div><div class="json-columns"><div class="json-raw"><div class="json-subtitle">格式化内容</div><pre>{{prettyJSON}}</pre></div><div class="json-fields"><div class="json-subtitle">字段列表 · {{jsonFields.length}} 项</div><div class="json-field-list"><div v-for="field in jsonFields" :key="field.path" class="json-field"><code>{{field.path}}</code><span>{{JSON.stringify(field.value)}}</span></div><div v-if="!jsonFields.length" class="json-empty">没有匹配字段</div></div></div></div></div>
      <div v-else class="raw-view"><div class="json-subtitle">原始日志内容</div><pre>{{selectedMatch.content}}</pre><p>{{selectedMatch.contentClipped?'该行超过 8 KiB，仅展示预览片段。':'当前行不是可解析的 JSON 对象或数组；NDJSON 可逐行查看。'}}</p></div>
     </section>
     <div class="panel-footer"><span><span class="footer-led"></span>日志内容仅在本机处理</span><span>流式检索 <span class="footer-divider">/</span>单行预览最多 8 KiB</span></div>
    </section>
    <div class="bottom-note"><Braces :size="15"/><span>快捷键：{{shortcutLabel('openFile')}} 打开文件，{{shortcutLabel('focusSearch')}} 聚焦搜索，{{shortcutLabel('runSearch')}} 检索。</span><button @click="setPage('processes')">管理本地服务 <ArrowRight :size="13"/></button></div>
   </section>

   <ProcessManagement :active="activePage==='processes'" />

   <DeveloperToolkit v-if="activePage==='toolkit'" />
  </div>
  <footer class="app-footer"><span>METEORDESK <span class="footer-divider">/</span>本地开发与运维工作台</span><span>专注效率 <span class="footer-divider">·</span>数据留在本机</span></footer>
 </main>
 <div v-if="shortcutsDialogOpen" class="settings-overlay" @click.self="shortcutsDialogOpen=false">
  <section class="settings-dialog" role="dialog" aria-modal="true" aria-labelledby="shortcuts-title">
   <div class="settings-dialog-heading"><div><span class="eyebrow">PREFERENCES</span><h2 id="shortcuts-title">快捷键设置</h2><p>快捷键使用 Ctrl（Windows/Linux）或 ⌘（macOS）。修改后立即生效。</p></div><button class="icon-button" type="button" aria-label="关闭" @click="shortcutsDialogOpen=false"><X :size="17"/></button></div>
   <div class="shortcut-settings-list"><label v-for="action in shortcutActions" :key="action.key" class="shortcut-setting-row"><span><strong>{{action.label}}</strong><small>{{action.description}}</small></span><select v-model="shortcutsConfig[action.key]" :aria-label="action.label+'快捷键'"><option v-for="option in SHORTCUT_OPTIONS" :key="option.value" :value="option.value">{{option.label}}</option></select></label></div>
   <p v-if="shortcutsError" class="settings-error" role="alert">{{shortcutsError}}</p>
   <div class="settings-dialog-actions"><button class="button button-secondary" type="button" @click="resetShortcuts">恢复默认</button><span class="option-spacer"></span><button class="button button-secondary" type="button" @click="shortcutsDialogOpen=false">取消</button><button class="button button-primary" type="button" @click="saveShortcuts">保存设置</button></div>
  </section>
 </div>
</div>
</template>
