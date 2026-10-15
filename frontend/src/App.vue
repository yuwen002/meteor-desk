<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { Activity, ArrowRight, Braces, CheckCircle2, ChevronRight, CircleHelp, Command, Eye, EyeOff, FileDown, FileSearch, FileUp, FolderOpen, Gauge, HardDrive, History, Layers3, Play, Plus, RefreshCw, Regex, Save, Search, ShieldCheck, Square, TerminalSquare, Trash2, Wrench, X } from 'lucide-vue-next'
import DeveloperToolkit from './components/DeveloperToolkit.vue'

type PageKey='logs'|'processes'|'toolkit'
type ProcAction='start'|'stop'|'restart'
const activePage=ref<PageKey>('logs'), filePath=ref(''), query=ref(''), useRegex=ref(false), contextLines=ref(2), resultLimit=ref(500)
const busy=ref(false), cancelRequested=ref(false), errorMessage=ref(''), response=ref<LogSearchResponse|null>(null), searchProgress=ref<LogSearchProgress|null>(null), searchID=ref('')
const inputRef=ref<HTMLInputElement|null>(null), selectedMatch=ref<LogMatch|null>(null), jsonFilter=ref(''), scrollTop=ref(0), resultListElement=ref<HTMLElement|null>(null)
const appSettings=ref<AppSettings>({recentFiles:[],searchHistory:[],contextLines:2,resultLimit:500})
const processes=ref<ProcessView[]>([]), selectedPID=ref(''), processLogs=ref<ProcessOutput[]>([]), outputCursor=ref(0), processError=ref(''), processBusy=ref(false)
const batchResults=ref<ProcessBatchResult[]>([]), processMessage=ref(''), showProcEnvEditor=ref(false)
const editingID=ref(''), procName=ref(''), procExecutable=ref(''), procArgs=ref('[]'), procCwd=ref(''), procEnv=ref('{}'), procDependsOn=ref<string[]>([])
const rowHeight=42
const virtualStart=computed(()=>Math.max(0,Math.floor(scrollTop.value/rowHeight)-3))
const visibleMatches=computed(()=>(response.value?.matches??[]).slice(virtualStart.value,virtualStart.value+24))
const virtualTop=computed(()=>virtualStart.value*rowHeight)
const virtualBottom=computed(()=>Math.max(0,((response.value?.matches.length??0)-virtualStart.value-visibleMatches.value.length)*rowHeight))
const selectedIndex=computed(()=>response.value?.matches.findIndex(m=>m.lineNumber===selectedMatch.value?.lineNumber)??-1)
const fileName=computed(()=>filePath.value.split(/[\\/]/).filter(Boolean).pop()||'')
const selectedProcess=computed(()=>processes.value.find(p=>p.spec.id===selectedPID.value)||null)
const isSensitiveEnvKey=(key:string)=>{const normalized=key.toLowerCase().replaceAll('_','').replaceAll('-','').replaceAll('.','');return ['password','passwd','pwd','secret','token','apikey','accesskey','privatekey','credential','authorization','cookie','signingkey','clientsecret','dsn'].some(part=>normalized.includes(part))}
const maskedProcEnvPreview=computed(()=>{try{const value:unknown=JSON.parse(procEnv.value);if(value===null||Array.isArray(value)||typeof value!=='object'||!Object.values(value).every(item=>typeof item==='string'))return '环境变量 JSON 无效，请进入编辑模式修正。';const masked=Object.fromEntries(Object.entries(value as Record<string,string>).map(([key,item])=>[key,isSensitiveEnvKey(key)&&item!==''?'••••••••':item]));return JSON.stringify(masked,null,2)}catch{return '环境变量 JSON 无效，请进入编辑模式修正。'}})
const fmtBytes=(bytes:number)=>{if(bytes<1024)return bytes+' B';const u=['KB','MB','GB','TB'];let v=bytes/1024,i=0;while(v>=1024&&i<u.length-1){v/=1024;i++}return v.toFixed(v>=100?0:1)+' '+u[i]}
const fileSize=computed(()=>fmtBytes(response.value?.fileSizeBytes??0))
const parsedJSON=computed(()=>{if(!selectedMatch.value||selectedMatch.value.contentClipped)return null;try{const v=JSON.parse(selectedMatch.value.content);return v!==null&&typeof v==='object'?v:null}catch{return null}})
const prettyJSON=computed(()=>parsedJSON.value===null?'':JSON.stringify(parsedJSON.value,null,2))
const jsonFields=computed(()=>{const out:JsonField[]=[];const add=(v:unknown,path:string,depth:number)=>{if(out.length>=300||depth>8)return;if(Array.isArray(v)){if(!v.length)out.push({path,value:[]});v.forEach((x,i)=>add(x,path+'['+i+']',depth+1))}else if(v&&typeof v==='object'){const e=Object.entries(v as Record<string,unknown>);if(!e.length)out.push({path,value:{}});for(const [k,x] of e)add(x,path==='$'?k:path+'.'+k,depth+1)}else out.push({path,value:v})};if(parsedJSON.value!==null)add(parsedJSON.value,'$',0);const q=jsonFilter.value.trim().toLowerCase();return q?out.filter(x=>x.path.toLowerCase().includes(q)||JSON.stringify(x.value).toLowerCase().includes(q)):out})
let progressTimer:number|undefined, processTimer:number|undefined, dropTimer:number|undefined

function api(){return window.go?.main?.App}
function clearTimer(which:'progress'|'process'|'drop'){if(which==='progress'&&progressTimer!==undefined){clearInterval(progressTimer);progressTimer=undefined}if(which==='process'&&processTimer!==undefined){clearInterval(processTimer);processTimer=undefined}if(which==='drop'&&dropTimer!==undefined){clearInterval(dropTimer);dropTimer=undefined}}
async function persistSettings(){const a=api();if(!a)return;const settings:AppSettings={recentFiles:appSettings.value.recentFiles.slice(0,12),searchHistory:appSettings.value.searchHistory.slice(0,30),contextLines:contextLines.value,resultLimit:resultLimit.value};appSettings.value=settings;try{await a.SaveAppSettings(settings)}catch(e){errorMessage.value='保存本地设置失败：'+String(e)}}
function addRecent(path:string){if(!path)return;appSettings.value.recentFiles=[path,...appSettings.value.recentFiles.filter(x=>x.toLowerCase()!==path.toLowerCase())].slice(0,12);void persistSettings()}
async function loadSettings(){const a=api();if(!a)return;try{appSettings.value=await a.GetAppSettings();contextLines.value=appSettings.value.contextLines??2;resultLimit.value=appSettings.value.resultLimit??500;if(appSettings.value.recentFiles.length)filePath.value=appSettings.value.recentFiles[0]}catch(e){errorMessage.value='读取设置失败：'+String(e)}}
async function chooseFile(){errorMessage.value='';const a=api();if(!a){errorMessage.value='请通过 Wails 桌面应用运行，才能使用系统文件选择器。';return}try{const p=await a.SelectLogFile();if(p){filePath.value=p;response.value=null;selectedMatch.value=null;scrollTop.value=0;addRecent(p)}}catch(e){errorMessage.value=String(e)}}
async function pollDrop(){const a=api();if(!a)return;try{const paths=await a.PollDroppedFiles();if(paths.length){filePath.value=paths[0];response.value=null;selectedMatch.value=null;scrollTop.value=0;addRecent(paths[0]);if(paths.length>1)errorMessage.value='已打开第一个文件；本次共拖入 '+paths.length+' 个文件。'}}catch{/* 等待桌面运行时就绪 */}}
async function pollProgress(id:string){const a=api();if(!a)return;try{const p=await a.GetLogSearchProgress(id);if(searchID.value===id)searchProgress.value=p}catch{/* 刚创建任务时可能尚未登记，下一轮重试 */}}
async function cancelSearch(){const a=api();if(!a||!searchID.value)return;cancelRequested.value=true;try{await a.CancelLogSearch(searchID.value)}catch(e){errorMessage.value='取消检索失败：'+String(e)}}
async function runSearch(){errorMessage.value='';if(!filePath.value){errorMessage.value='请先选择日志文件。';return}if(!query.value){errorMessage.value='请输入关键字或正则表达式。';return}const a=api();if(!a){errorMessage.value='未检测到桌面桥接服务，请通过 wails dev 启动。';return}
 clearTimer('progress');const id=Date.now()+'-'+Math.random().toString(16).slice(2);searchID.value=id;cancelRequested.value=false;busy.value=true;response.value=null;selectedMatch.value=null;scrollTop.value=0
 searchProgress.value={searchId:id,bytesScanned:0,fileSizeBytes:0,scannedLines:0,matchesFound:0,percent:0,running:true,canceled:false}
 progressTimer=window.setInterval(()=>void pollProgress(id),250)
 try{response.value=await a.SearchLogsAdvanced(id,filePath.value,query.value,useRegex.value,resultLimit.value,contextLines.value);await pollProgress(id);if(response.value.canceled){errorMessage.value='检索已取消，以下保留已找到的部分结果。'}else{const record:SearchRecord={query:query.value,path:filePath.value,useRegex:useRegex.value,contextLines:contextLines.value,createdAt:new Date().toISOString()};appSettings.value.searchHistory=[record,...appSettings.value.searchHistory.filter(x=>!(x.query===record.query&&x.path===record.path&&x.useRegex===record.useRegex&&x.contextLines===record.contextLines))].slice(0,30);addRecent(filePath.value);await persistSettings()}}
 catch(e){errorMessage.value=String(e)}finally{clearTimer('progress');busy.value=false;cancelRequested.value=false}}
function applyHistory(r:SearchRecord){filePath.value=r.path;query.value=r.query;useRegex.value=r.useRegex;contextLines.value=r.contextLines;activePage.value='logs';selectedMatch.value=null}
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
async function loadProcesses(){const a=api();if(!a)return;try{processes.value=await a.GetProcesses();if(!selectedPID.value&&processes.value.length)selectedPID.value=processes.value[0].spec.id;if(selectedPID.value&&!processes.value.some(x=>x.spec.id===selectedPID.value))selectedPID.value=processes.value[0]?.spec.id??''}catch(e){processError.value=String(e)}}
async function pollProcessLogs(){const a=api();if(!a||!selectedPID.value)return;try{const lines=await a.GetProcessLogs(selectedPID.value,outputCursor.value);if(lines.length){processLogs.value=[...processLogs.value,...lines].slice(-1500);outputCursor.value=lines[lines.length-1].sequence}}catch(e){processError.value=String(e)}}
async function refreshProcesses(){await loadProcesses();if(selectedPID.value)await pollProcessLogs()}
function selectProcess(p:ProcessView){showProcEnvEditor.value=false;selectedPID.value=p.spec.id;editingID.value=p.spec.id;procName.value=p.spec.name;procExecutable.value=p.spec.executable;procArgs.value=JSON.stringify(p.spec.args??[],null,2);procCwd.value=p.spec.workingDir??'';procEnv.value=JSON.stringify(p.spec.env??{},null,2);procDependsOn.value=[...(p.spec.dependsOn??[])];processLogs.value=[];outputCursor.value=0;void pollProcessLogs()}
function newProcess(){showProcEnvEditor.value=false;editingID.value='';selectedPID.value='';procName.value='';procExecutable.value='';procArgs.value='[]';procCwd.value='';procEnv.value='{}';procDependsOn.value=[];processLogs.value=[];outputCursor.value=0}
async function saveProcess(){const a=api();if(!a)return;processError.value='';let args:string[],env:Record<string,string>;try{const p:unknown=JSON.parse(procArgs.value),e:unknown=JSON.parse(procEnv.value);if(!Array.isArray(p)||!p.every(x=>typeof x==='string'))throw new Error('参数必须是 JSON 字符串数组，例如 ["run","dev"]');if(e===null||Array.isArray(e)||typeof e!=='object'||!Object.values(e).every(x=>typeof x==='string'))throw new Error('环境变量必须是 JSON 字符串对象');args=p;env=e as Record<string,string>}catch(e){processError.value='配置格式错误：'+String(e);return}processBusy.value=true;try{const saved=await a.SaveProcess({id:editingID.value,name:procName.value,executable:procExecutable.value,args,workingDir:procCwd.value,env,autoStart:false,dependsOn:procDependsOn.value});await loadProcesses();selectProcess(saved)}catch(e){processError.value=String(e)}finally{processBusy.value=false}}
async function processAction(action:ProcAction){const a=api();if(!a||!selectedProcess.value)return;processBusy.value=true;processError.value='';try{if(action==='start')await a.StartProcess(selectedPID.value);else if(action==='stop')await a.StopProcess(selectedPID.value);else await a.RestartProcess(selectedPID.value);await refreshProcesses()}catch(e){processError.value=String(e)}finally{processBusy.value=false}}
async function batchProcessAction(action:'start'|'stop'){
 const a=api()
 if(!a||!processes.value.length)return
 processBusy.value=true;processError.value='';batchResults.value=[]
 try{
  const ids=processes.value.map(item=>item.spec.id)
  batchResults.value=action==='start'?await a.StartProcesses(ids):await a.StopProcesses(ids)
  const failed=batchResults.value.filter(item=>!item.success)
  const success=batchResults.value.length-failed.length
  if(failed.length)processError.value='批量'+(action==='start'?'启动':'停止')+'完成：成功 '+success+' 个，失败 '+failed.length+' 个。请查看批量操作结果。'
  await refreshProcesses()
 }catch(e){processError.value=String(e)}
 finally{processBusy.value=false}
}
async function exportProcessConfigs(redactSensitive:boolean){
 const a=api();if(!a)return
 processError.value='';processMessage.value=''
 if(!redactSensitive&&!confirm('完整导出会包含真实环境变量值（可能有密码、Token 或密钥）。请仅保存到可信位置。确定继续？'))return
 processBusy.value=true
 try{const message=await a.ExportProcessConfigs(redactSensitive);if(message)processMessage.value=message}
 catch(e){processError.value='导出服务配置失败：'+String(e)}
 finally{processBusy.value=false}
}
async function importProcessConfigs(){
 const a=api();if(!a)return
 if(!confirm('导入文件中的服务会作为新配置添加，不会覆盖当前配置，也不会自动启动。请只导入可信配置。继续？'))return
 processBusy.value=true;processError.value='';processMessage.value=''
 try{const imported=await a.ImportProcessConfigs();if(imported.length){await loadProcesses();selectProcess(imported[0]);processMessage.value='已导入 '+imported.length+' 个服务配置。启动前请检查可执行文件、工作目录及已脱敏的环境变量。'}}
 catch(e){processError.value='导入服务配置失败：'+String(e)}
 finally{processBusy.value=false}
}
async function deleteProcess(){const a=api();if(!a||!selectedProcess.value)return;if(!confirm('确定删除服务配置“'+selectedProcess.value.spec.name+'”？运行中的服务必须先停止。'))return;try{await a.DeleteProcess(selectedPID.value);newProcess();await loadProcesses()}catch(e){processError.value=String(e)}}
function stateName(s:ProcessStatus['state']){return ({stopped:'已停止',starting:'启动中',running:'运行中',stopping:'停止中',failed:'异常退出'} as Record<ProcessStatus['state'],string>)[s]}
function setPage(p:PageKey){activePage.value=p}
function shortcuts(e:KeyboardEvent){if(!(e.ctrlKey||e.metaKey)||e.altKey)return;const k=e.key.toLowerCase();if(k==='o'){e.preventDefault();void chooseFile()}else if(k==='k'){e.preventDefault();setPage('logs');void nextTick(()=>inputRef.value?.focus())}else if(e.key==='Enter'&&activePage.value==='logs'){e.preventDefault();if(!busy.value)void runSearch()}else if(k==='1'){e.preventDefault();setPage('logs')}else if(k==='2'){e.preventDefault();setPage('processes')}else if(k==='3'){e.preventDefault();setPage('toolkit')}}
onMounted(()=>{window.addEventListener('keydown',shortcuts);void loadSettings();void loadProcesses();dropTimer=window.setInterval(()=>void pollDrop(),300);processTimer=window.setInterval(()=>{if(activePage.value==='processes')void refreshProcesses()},1000)})
onBeforeUnmount(()=>{window.removeEventListener('keydown',shortcuts);clearTimer('progress');clearTimer('process');clearTimer('drop')})
</script>

<template>
<div class="app-shell" style="--wails-drop-target: drop">
 <aside class="sidebar">
  <div class="brand"><div class="brand-mark"><Activity :size="20"/></div><div class="brand-copy"><span class="brand-name">Meteor<span>Desk</span></span><span class="brand-caption">本地开发工作台</span></div></div>
  <div class="workspace-label">工作空间</div><nav class="nav-list" aria-label="主导航">
   <button class="nav-item" :class="{active:activePage==='logs'}" @click="setPage('logs')"><FileSearch :size="17"/><span>日志分析</span><kbd>1</kbd></button>
   <button class="nav-item" :class="{active:activePage==='processes'}" @click="setPage('processes')"><TerminalSquare :size="17"/><span>进程管理</span><kbd>2</kbd></button>
   <button class="nav-item" :class="{active:activePage==='toolkit'}" @click="setPage('toolkit')"><Wrench :size="17"/><span>开发工具</span><kbd>3</kbd></button>
  </nav>
  <div class="sidebar-divider"></div><div class="workspace-label">运行方式</div><div class="local-card"><div class="local-icon"><ShieldCheck :size="16"/></div><div><div class="local-title">本地优先</div><div class="local-description">文件仅在本机处理</div></div><span class="status-dot"></span></div>
  <div class="sidebar-bottom"><div class="version-row"><span>早期版本</span><span class="version-tag">v0.5.1</span></div><div class="profile-row"><div class="avatar">MD</div><div class="profile-copy"><strong>MeteorDesk</strong><span>开发与运维工作台</span></div><CircleHelp :size="17" class="help-icon"/></div></div>
 </aside>
 <main class="main-content">
  <header class="topbar"><div class="breadcrumbs"><span>工作空间</span><ChevronRight :size="14"/><strong>{{activePage==='logs'?'日志分析':activePage==='processes'?'进程管理':'开发工具'}}</strong></div><div class="topbar-right"><div class="runtime-status"><span class="status-dot"></span>本机运行</div><div class="topbar-divider"></div><div class="shortcut-hint"><Command :size="13"/><span>K</span></div></div></header>
  <div class="page-wrap">
   <section v-if="activePage==='logs'" class="page-content">
    <div class="page-heading"><div><div class="eyebrow"><span class="eyebrow-line"></span>可观测性 / 文件分析</div><h1>日志分析<span class="title-period">。</span></h1><p class="page-subtitle">逐块读取大体积日志，支持关键字、正则、上下文和 JSON 字段过滤。</p></div><button class="button button-secondary" @click="chooseFile"><FolderOpen :size="16"/>选择日志</button></div>
    <div class="metric-grid">
     <div class="metric-card"><div class="metric-top"><span class="metric-icon mint"><Gauge :size="16"/></span><span class="metric-kicker">检索方式</span></div><div class="metric-value">流式扫描</div><div class="metric-foot"><CheckCircle2 :size="12"/>固定大小缓冲区</div></div>
     <div class="metric-card"><div class="metric-top"><span class="metric-icon blue"><HardDrive :size="16"/></span><span class="metric-kicker">数据处理</span></div><div class="metric-value">仅限本地</div><div class="metric-foot"><CheckCircle2 :size="12"/>不上传日志内容</div></div>
     <div class="metric-card"><div class="metric-top"><span class="metric-icon purple"><Layers3 :size="16"/></span><span class="metric-kicker">结果上限</span></div><div class="metric-value">{{resultLimit.toLocaleString()}} 条</div><div class="metric-foot">可调整结果数量</div></div>
    </div>
    <section class="search-panel">
     <div class="panel-heading"><div class="panel-heading-left"><div class="panel-icon"><Search :size="17"/></div><div><h2>查找日志</h2><p>输入关键字或正则表达式，定位匹配行。</p></div></div><div class="panel-meta"><span class="live-dot"></span>本地检索</div></div>
     <div class="file-drop" :class="{'file-selected':!!filePath}"><div class="file-icon"><FileSearch :size="20"/></div><div class="file-info"><strong>{{fileName||'请选择要分析的日志文件'}}</strong><span>{{filePath||'支持 .log、.txt、.out、.json、.ndjson；也可将文件拖入窗口'}}</span></div><button class="button button-quiet" @click="chooseFile">{{filePath?'更换文件':'浏览文件'}} <ArrowRight :size="14"/></button></div>
     <details v-if="appSettings.recentFiles.length" class="history-panel"><summary><History :size="14"/>最近打开的文件 <span class="history-count">{{appSettings.recentFiles.length}}</span></summary><div class="history-items"><button v-for="p in appSettings.recentFiles" :key="p" class="history-item" @click="filePath=p"><FileSearch :size="13"/><span>{{p}}</span><ArrowRight :size="12"/></button></div></details>
     <form class="query-form" @submit.prevent="runSearch"><label class="field-label" for="search-query">搜索内容</label><div class="search-input-row"><div class="query-input-wrap"><Search :size="17" class="query-input-icon"/><input id="search-query" ref="inputRef" v-model="query" autocomplete="off" spellcheck="false" placeholder="输入关键字、错误码、请求 ID……"/><span class="input-shortcut">回车 ↵</span></div><button type="submit" class="button button-primary" :disabled="busy"><Search v-if="!busy" :size="16"/><span v-else class="spinner"></span>{{busy?'正在检索':'开始检索'}}</button><button v-if="busy" type="button" class="button button-secondary button-cancel" :disabled="cancelRequested" @click="cancelSearch"><X :size="15"/>{{cancelRequested?'正在取消':'取消检索'}}</button></div>
      <div class="query-options"><label class="toggle-option"><input v-model="useRegex" type="checkbox"/><span class="custom-checkbox"><Regex v-if="useRegex" :size="12"/></span><span>使用正则表达式</span></label><label class="option-select">上下文行数 <select v-model.number="contextLines"><option :value="0">不显示</option><option :value="2">前后 2 行</option><option :value="5">前后 5 行</option><option :value="10">前后 10 行</option></select></label><label class="option-select">结果上限 <select v-model.number="resultLimit"><option :value="500">500 条</option><option :value="1000">1,000 条</option><option :value="5000">5,000 条</option></select></label><span class="option-spacer"></span><span class="query-tip"><kbd>.*</kbd> Go 正则语法</span></div>
      <div v-if="busy&&searchProgress" class="progress-wrap"><div class="progress-meta"><span>{{cancelRequested?'正在停止检索':'正在扫描文件'}} · {{searchProgress.scannedLines.toLocaleString()}} 行</span><span>{{searchProgress.percent}}%　{{fmtBytes(searchProgress.bytesScanned)}} / {{fmtBytes(searchProgress.fileSizeBytes)}}</span></div><div class="progress-track"><div class="progress-bar" :style="{width:searchProgress.percent+'%'}"></div></div></div>
     </form>
     <details v-if="appSettings.searchHistory.length" class="history-panel query-history"><summary><History :size="14"/>最近检索 <span class="history-count">{{appSettings.searchHistory.length}}</span></summary><div class="history-items"><button v-for="(r,i) in appSettings.searchHistory" :key="r.createdAt+r.query+i" class="history-item" @click="applyHistory(r)"><Search :size="13"/><span><strong>{{r.query}}</strong><small>{{r.path}} · {{r.useRegex?'正则':'文本'}}</small></span><ArrowRight :size="12"/></button></div></details>
     <div v-if="errorMessage" class="notice notice-error"><span>!</span>{{errorMessage}}</div>
     <div class="results-header"><div><h3>检索结果 <span v-if="response" class="result-count">{{response.matches.length}}</span></h3><p>{{response?'已扫描 '+response.scannedLines.toLocaleString()+' 行 · '+fileSize:'检索结果会显示在这里。'}}</p></div><div v-if="response" class="results-actions"><span v-if="response.resultLimitReached" class="limit-pill">已达到结果上限</span><span v-if="response.canceled" class="result-state">已取消</span><span v-else class="result-state"><span class="status-dot"></span>已完成</span></div></div>
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
    <div class="bottom-note"><Braces :size="15"/><span>快捷键：Ctrl/⌘+O 打开文件，Ctrl/⌘+K 聚焦搜索，Ctrl/⌘+Enter 检索。</span><button @click="setPage('processes')">管理本地服务 <ArrowRight :size="13"/></button></div>
   </section>

   <section v-else-if="activePage==='processes'" class="page-content process-page">
    <div class="page-heading"><div><div class="eyebrow"><span class="eyebrow-line"></span>本地运行时 / 服务编排</div><h1>进程管理<span class="title-period">。</span></h1><p class="page-subtitle">按依赖顺序启动本地服务；停止依赖时，会先停止依赖它的服务。</p></div><div class="process-heading-actions"><button class="button button-secondary" :disabled="processBusy||!processes.length" @click="batchProcessAction('start')"><Play :size="14"/>全部启动</button><button class="button button-secondary" :disabled="processBusy||!processes.length" @click="batchProcessAction('stop')"><Square :size="13"/>全部停止</button><button class="button button-primary" @click="newProcess"><Plus :size="15"/>新建服务</button></div></div>
    <div v-if="processError" class="notice notice-error process-notice"><span>!</span>{{processError}}</div>
    <div v-if="processMessage" class="notice notice-success process-notice"><span>✓</span>{{processMessage}}</div>
    <div class="process-config-toolbar">
     <div class="process-config-copy"><strong>服务配置迁移</strong><span>导入会创建新配置，不覆盖现有服务，也不会自动启动。</span></div>
     <div class="process-config-actions">
      <button class="button button-secondary" :disabled="processBusy" @click="importProcessConfigs"><FileUp :size="14"/>导入配置</button>
      <button class="button button-secondary" :disabled="processBusy" @click="exportProcessConfigs(true)"><FileDown :size="14"/>导出脱敏配置</button>
      <button class="button button-quiet" :disabled="processBusy" @click="exportProcessConfigs(false)"><ShieldCheck :size="14"/>导出完整配置</button>
     </div>
    </div>
    <div v-if="batchResults.length" class="batch-results">
     <div class="batch-results-header"><strong>最近一次批量操作</strong><span>成功 {{batchResults.filter(item=>item.success).length}} / {{batchResults.length}}</span></div>
     <div v-for="item in batchResults" :key="item.action+'-'+item.id" class="batch-result-row" :class="{failed:!item.success}"><span class="batch-result-mark">{{item.success?'✓':'!'}}</span><strong>{{item.name}}</strong><span>{{item.message}}</span></div>
    </div>
    <div class="process-layout">
     <section class="process-panel"><div class="process-panel-heading"><div><h2>服务列表</h2><p>{{processes.length}} 个已保存的服务配置</p></div><button class="icon-button" @click="refreshProcesses"><RefreshCw :size="15"/></button></div><div v-if="processes.length" class="process-list"><button v-for="p in processes" :key="p.spec.id" class="process-item" :class="{selected:selectedPID===p.spec.id}" @click="selectProcess(p)"><span class="process-state-dot" :class="p.status.state"></span><span class="process-item-main"><strong>{{p.spec.name}}</strong><small>{{p.spec.executable}}</small><em>{{stateName(p.status.state)}}<template v-if="p.status.pid"> · PID {{p.status.pid}}</template></em></span><ChevronRight :size="14"/></button></div><div v-else class="process-empty"><TerminalSquare :size="25"/><strong>还没有服务配置</strong><span>创建一条命令配置，即可开始管理本地服务。</span><button class="button button-secondary" @click="newProcess"><Plus :size="14"/>创建第一个服务</button></div></section>
     <section class="process-panel"><div class="process-panel-heading"><div><h2>{{editingID?'服务配置':'新增服务配置'}}</h2><p>参数使用 JSON 数组，不会拼接执行为 Shell 命令。</p></div><span class="safe-pill"><ShieldCheck :size="13"/>显式参数</span></div>
      <form class="process-form" @submit.prevent="saveProcess"><label class="field-label">服务名称</label><input v-model="procName" class="text-field" placeholder="例如：后端 API" required/><label class="field-label">可执行文件</label><input v-model="procExecutable" class="text-field mono-field" placeholder="例如：go、npm 或程序绝对路径" required/><label class="field-label">命令参数（JSON 字符串数组）</label><textarea v-model="procArgs" class="text-area mono-field" rows="3" spellcheck="false"></textarea><label class="field-label">工作目录</label><input v-model="procCwd" class="text-field mono-field" placeholder="可选，项目目录"/><div class="env-editor-heading"><label class="field-label">环境变量（JSON 字符串对象）</label><button type="button" class="button button-quiet env-toggle" @click="showProcEnvEditor=!showProcEnvEditor"><EyeOff v-if="showProcEnvEditor" :size="13"/><Eye v-else :size="13"/>{{showProcEnvEditor?'隐藏编辑内容':'编辑环境变量'}}</button></div><textarea v-if="showProcEnvEditor" v-model="procEnv" class="text-area mono-field" rows="4" spellcheck="false"></textarea><p v-if="showProcEnvEditor" class="env-editor-help">编辑模式会显示真实环境变量值。保存后退出编辑模式；导出时建议使用脱敏选项。</p><pre v-else class="env-preview mono-field">{{maskedProcEnvPreview}}</pre>
       <label class="field-label">启动前置依赖</label>
       <div class="dependency-list">
        <label v-for="candidate in processes.filter(item=>item.spec.id!==editingID)" :key="candidate.spec.id" class="dependency-item">
         <input v-model="procDependsOn" type="checkbox" :value="candidate.spec.id" :disabled="processBusy"/>
         <span><strong>{{candidate.spec.name}}</strong><small>{{candidate.spec.id}}</small></span>
        </label>
        <span v-if="!processes.some(item=>item.spec.id!==editingID)" class="dependency-empty">先保存其他服务，再选择它们作为启动依赖。</span>
       </div>
       <div class="process-form-actions"><button type="submit" class="button button-primary" :disabled="processBusy"><Save :size="15"/>保存配置</button><button type="button" class="button button-secondary" @click="newProcess">清空表单</button><button v-if="editingID" type="button" class="button button-danger" @click="deleteProcess"><Trash2 :size="14"/>删除配置</button></div></form>
      <div v-if="selectedProcess" class="process-runtime"><div class="runtime-heading"><div><h3>进程控制</h3><p>{{stateName(selectedProcess.status.state)}}<template v-if="selectedProcess.status.pid"> · PID {{selectedProcess.status.pid}}</template></p></div><div class="runtime-actions"><button class="button button-primary" :disabled="processBusy||selectedProcess.status.state==='running'||selectedProcess.status.state==='starting'" @click="processAction('start')"><Play :size="13"/>启动</button><button class="button button-secondary" :disabled="processBusy||selectedProcess.status.state!=='running'" @click="processAction('stop')"><Square :size="13"/>停止</button><button class="button button-secondary" :disabled="processBusy||selectedProcess.status.state==='running'||selectedProcess.status.state==='starting'" @click="processAction('restart')"><RefreshCw :size="13"/>重启</button></div></div><p v-if="selectedProcess.status.error" class="process-error-text">{{selectedProcess.status.error}}</p><div class="log-stream-heading"><span><Activity :size="14"/>运行输出</span><span>{{processLogs.length}} 条缓存记录</span></div><div class="process-log-output"><div v-for="line in processLogs" :key="line.sequence" class="process-log-line" :class="line.stream"><span>{{line.time}}</span><b>{{line.stream==='stdout'?'OUT':line.stream==='stderr'?'ERR':'SYS'}}</b><code>{{line.text}}</code></div><div v-if="!processLogs.length" class="process-log-empty">启动服务后，标准输出和错误输出会显示在这里。</div></div></div>
     </section>
    </div>
    <div class="bottom-note"><ShieldCheck :size="15"/><span>配置保存在本机用户目录；全部启动会按依赖排序，停止某项服务会先停止依赖它的下游服务。</span></div>
   </section>

   <DeveloperToolkit v-else />
  </div>
  <footer class="app-footer"><span>METEORDESK <span class="footer-divider">/</span>本地开发与运维工作台</span><span>专注效率 <span class="footer-divider">·</span>数据留在本机</span></footer>
 </main>
</div>
</template>
