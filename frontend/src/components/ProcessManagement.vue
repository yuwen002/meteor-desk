<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Activity, ChevronRight, Clock, ExternalLink, Eye, EyeOff, FileDown, FileUp, FolderKanban, HardDrive, History, LineChart, Network, Pause, Play, Plus, RefreshCw, Save, Search, ShieldCheck, Square, TerminalSquare, Trash2 } from 'lucide-vue-next'
import { getMaskedEnvironmentPreview, parsePortList, parseProcessForm } from '../utils/processConfig'
import { buildProcessLogExportCSV, buildProcessLogExportJSON, buildProcessLogExportTXT, processLogExportFileName, type ExportFormat } from '../utils/exporters'
import { sparklinePoints, summarizeHistory } from '../utils/resourceHistory'
import { buildDependencyGraph } from '../utils/dependencyGraph'
import { countEvents, eventLabel, formatEventDuration, latestStartupMs } from '../utils/processTimeline'
import { formatRunBytes, formatRunDuration as formatRunDurationLabel, formatRunPeakCPU, runExitSummary, summarizeRunRecords } from '../utils/runHistory'
import { groupMemberNames, groupMissingCount, isGroupValid, normalizeGroupPayload, toggleMember } from '../utils/processGroups'

const { t } = useI18n()

type ProcAction = 'start' | 'stop' | 'restart'

const props = defineProps<{ active: boolean; focusId?: string }>()
// 详情面板把落盘文件交给日志分析页打开；日志分析反查到服务后通过 focusId 请求选中它，处理完上报 focus-handled。
const emit = defineEmits<{ (e: 'open-log', path: string): void; (e: 'search-log', payload: { path: string; query: string; useRegex: boolean }): void; (e: 'focus-handled'): void }>()
const processes=ref<ProcessView[]>([]), selectedPID=ref(''), processLogs=ref<ProcessOutput[]>([]), outputCursor=ref(0), processError=ref(''), processBusy=ref(false)
const logGrep=ref(''), logGrepRegex=ref<RegExp|null>(null), autoScrollPaused=ref(false), logOutputEl=ref<HTMLElement|null>(null)
const batchResults=ref<ProcessBatchResult[]>([]), processMessage=ref(''), showProcEnvEditor=ref(false)
const editingID=ref(''), procName=ref(''), procExecutable=ref(''), procArgs=ref('[]'), procCwd=ref(''), procEnv=ref('{}'), procDependsOn=ref<string[]>([])
const procRestartMode=ref<'none'|'on-failure'>('none'), procRestartAttempts=ref(3)
const procReadyTimeout=ref(60)
const procPorts=ref(''), portReport=ref<ProcessPortReport|null>(null), loadingPorts=ref(false)
// procLogToDisk 控制是否把该服务的标准输出/错误落盘到 logs/<服务名>.log；status.logFilePath 是当前落盘路径。
const procLogToDisk=ref(false)
const runHistory=ref<ProcessRunRecord[]>([])
// 服务分组（场景）：groups 是列表，下面四个 ref 持有当前正在编辑的分组表单。
const groups=ref<ProcessGroup[]>([])
const groupEditingID=ref(''), groupName=ref(''), groupDescription=ref(''), groupMembers=ref<string[]>([])
const procHealthType=ref<'none'|'tcp'|'http'>('none'), procHealthTarget=ref(''), procHealthInterval=ref(10), procHealthTimeout=ref(2), procHealthThreshold=ref(3)
const selectedProcess=computed(()=>processes.value.find(p=>p.spec.id===selectedPID.value)||null)
const maskedProcEnvPreview=computed(()=>getMaskedEnvironmentPreview(procEnv.value))
const resourceHistory=ref<ProcessResourceSample[]>([])
const timeline=ref<ProcessEvent[]>([])
const latestStartup=computed(()=>latestStartupMs(timeline.value))
const restartEvents=computed(()=>countEvents(timeline.value,'restart'))
const runHistorySummary=computed(()=>summarizeRunRecords(runHistory.value))
const historySummary=computed(()=>summarizeHistory(resourceHistory.value))
const cpuPoints=computed(()=>sparklinePoints(historySummary.value.cpuValues,120,32))
const memoryPoints=computed(()=>sparklinePoints(historySummary.value.memoryValues,120,32))
const memoryDeltaLabel=computed(()=>{const delta=historySummary.value.memoryDelta;if(!delta)return '';return (delta>0?'+':'-')+formatBytes(Math.abs(delta))})
// 依赖图只读配置列表，随保存与删除自动重排；点节点即可切到对应服务的配置。
const dependencyGraph=computed(()=>buildDependencyGraph(processes.value.map(item=>({id:item.spec.id,name:item.spec.name,dependsOn:item.spec.dependsOn??[]}))))
function graphNodeLabel(name:string){return name.length>14?name.slice(0,13)+'…':name}
function graphNodeState(id:string){return processes.value.find(item=>item.spec.id===id)?.status.state??'stopped'}
function selectGraphNode(id:string){const view=processes.value.find(item=>item.spec.id===id);if(view)selectProcess(view)}

function api(){return window.go?.main?.App}
async function loadProcesses(){const a=api();if(!a)return;try{processes.value=await a.GetProcesses();if(!selectedPID.value&&processes.value.length)selectedPID.value=processes.value[0].spec.id;if(selectedPID.value&&!processes.value.some(x=>x.spec.id===selectedPID.value))selectedPID.value=processes.value[0]?.spec.id??''}catch(e){processError.value=String(e)}}
async function pollProcessLogs(){const a=api();if(!a||!selectedPID.value)return;try{const lines=await a.GetProcessLogs(selectedPID.value,outputCursor.value);if(lines.length){processLogs.value=[...processLogs.value,...lines].slice(-1500);outputCursor.value=lines[lines.length-1].sequence;if(!autoScrollPaused.value){await nextTick();if(logOutputEl.value)logOutputEl.value.scrollTop=logOutputEl.value.scrollHeight}}}catch(e){processError.value=String(e)}}
watch(logGrep,v=>{try{logGrepRegex.value=v?new RegExp(v.replace(/[.*+?^${}()|[\]\\]/g,'\\$&'),'i'):null}catch{logGrepRegex.value=null}})
function escapeHtml(s:string):string{let o='';for(const c of s){if(c==='&')o+='&amp;';else if(c==='<')o+='&lt;';else if(c==='>')o+='&gt;';else o+=c}return o}function highlightLogText(text:string):string{const r=logGrepRegex.value;if(!r)return text;const safe=escapeHtml(text);if(!r.test(text))return safe;return safe.replace(r,m=>`<mark>${m}</mark>`)}
const grepMatchCount=computed(()=>{const r=logGrepRegex.value;if(!r)return 0;return processLogs.value.reduce((n,l)=>n+(r.test(l.text)?1:0),0)})
// 资源曲线单独轮询：采样间隔 2 秒，不需要跟随运行输出的 1 秒节奏。
async function loadResourceHistory(){const a=api();if(!a||!selectedPID.value){resourceHistory.value=[];return}try{resourceHistory.value=await a.GetResourceHistory(selectedPID.value)}catch(e){processError.value=String(e)}}
// 生命周期事件变化不频繁，跟随资源曲线一起每 3 秒拉取即可。
async function loadTimeline(){const a=api();if(!a||!selectedPID.value){timeline.value=[];return}try{timeline.value=await a.GetProcessTimeline(selectedPID.value)}catch(e){processError.value=String(e)}}
// 端口诊断要调用系统命令，只在选中服务、启停操作后和手动刷新时扫描，不持续轮询。
async function loadPortDiagnostics(){const a=api();if(!a||!selectedPID.value){portReport.value=null;return}loadingPorts.value=true;try{portReport.value=await a.GetPortDiagnostics(selectedPID.value)}catch(e){processError.value=String(e)}finally{loadingPorts.value=false}}
// 运行历史低频变化（仅在服务退出时追加一条），跟随时间线每 3 秒拉取即可。
async function loadRunHistory(){const a=api();if(!a||!selectedPID.value){runHistory.value=[];return}try{runHistory.value=await a.GetProcessRunHistory(selectedPID.value)}catch(e){processError.value=String(e)}}
// 分组只在进入页面、增删改与启停后刷新，无需高频轮询。
async function loadGroups(){const a=api();if(!a)return;try{groups.value=await a.ListProcessGroups()}catch(e){processError.value=String(e)}}
// 把当前落盘文件交给日志分析页打开；未开启落盘时给提示。
function openLogInAnalysis(){const path=selectedProcess.value?.status.logFilePath;if(!path){processError.value=t('process.errNoLogDisk');return}emit('open-log',path)}
function openLogWithGrep(){const path=selectedProcess.value?.status.logFilePath;if(!path){processError.value=t('process.errNoLogDisk');return}if(!logGrep.value.trim()){processError.value=t('process.errNoGrep');return}emit('search-log',{path,query:logGrep.value.trim(),useRegex:false})}
// 端口被本工作台另一个服务占用时，可按占用者 ID 直接停止它（外部进程无 ownerSpecId，不提供入口）。
async function stopOccupyingService(id?:string){const a=api();if(!a||!id)return;processBusy.value=true;processError.value='';try{await a.StopProcess(id);await refreshProcesses();await loadPortDiagnostics()}catch(e){processError.value=String(e)}finally{processBusy.value=false}}
function groupMembersAsOptions(){return processes.value.map(item=>({id:item.spec.id,name:item.spec.name}))}
function groupNamesFor(memberIds:string[]){return groupMemberNames(memberIds,groupMembersAsOptions())}
function groupMissing(memberIds:string[]){return groupMissingCount(memberIds,groupMembersAsOptions())}
function newGroup(){groupEditingID.value='';groupName.value='';groupDescription.value='';groupMembers.value=[]}
function editGroup(group:ProcessGroup){groupEditingID.value=group.id;groupName.value=group.name;groupDescription.value=group.description??'';groupMembers.value=[...(group.memberIds??[])]}
function toggleGroupMember(id:string){groupMembers.value=toggleMember(groupMembers.value,id)}
async function saveGroup(){const a=api();if(!a)return;const payload=normalizeGroupPayload({id:groupEditingID.value,name:groupName.value,description:groupDescription.value,memberIds:groupMembers.value});if(!isGroupValid(payload)){processError.value=t('process.errGroupInvalid');return}processError.value='';processMessage.value='';processBusy.value=true;try{await a.SaveProcessGroup(payload);await loadGroups();newGroup();processMessage.value=t('process.groupSaved',{name:payload.name})}catch(e){processError.value=t('process.errSaveGroup')+String(e)}finally{processBusy.value=false}}
async function deleteGroup(id:string){const a=api();if(!a)return;if(!confirm(t('process.groupDeleteConfirm')))return;processBusy.value=true;try{await a.DeleteProcessGroup(id);await loadGroups();if(groupEditingID.value===id)newGroup()}catch(e){processError.value=String(e)}finally{processBusy.value=false}}
async function groupAction(id:string,action:'start'|'stop'){const a=api();if(!a)return;processBusy.value=true;processError.value='';batchResults.value=[];try{batchResults.value=action==='start'?await a.StartProcessGroup(id):await a.StopProcessGroup(id);const failed=batchResults.value.filter(item=>!item.success);if(failed.length)processError.value=action==='start'?t('process.batchResultStartFail',{ok:batchResults.value.length-failed.length,fail:failed.length}):t('process.batchResultStopFail',{ok:batchResults.value.length-failed.length,fail:failed.length});else processMessage.value=action==='start'?t('process.batchResultStartOk',{n:batchResults.value.length}):t('process.batchResultStopOk',{n:batchResults.value.length});await refreshProcesses();if(selectedPID.value){await loadResourceHistory();await loadTimeline();await loadRunHistory();await loadPortDiagnostics()}}catch(e){processError.value=String(e)}finally{processBusy.value=false}}
async function refreshProcesses(){await loadProcesses();if(selectedPID.value)await pollProcessLogs()}
function selectProcess(p:ProcessView){showProcEnvEditor.value=false;selectedPID.value=p.spec.id;editingID.value=p.spec.id;procName.value=p.spec.name;procExecutable.value=p.spec.executable;procArgs.value=JSON.stringify(p.spec.args??[],null,2);procCwd.value=p.spec.workingDir??'';procEnv.value=JSON.stringify(p.spec.env??{},null,2);procDependsOn.value=[...(p.spec.dependsOn??[])];procRestartMode.value=p.spec.restart?.mode==='on-failure'?'on-failure':'none';procRestartAttempts.value=p.spec.restart?.maxAttempts||3;const health=p.spec.health??null;procHealthType.value=health?(health.type==='http'?'http':'tcp'):'none';procHealthTarget.value=health?.target??'';procHealthInterval.value=health?.intervalSeconds??10;procHealthTimeout.value=health?.timeoutSeconds??2;procHealthThreshold.value=health?.failureThreshold??3;procReadyTimeout.value=p.spec.readyTimeoutSeconds??0;procPorts.value=(p.spec.ports??[]).join(', ');procLogToDisk.value=!!p.spec.logToDisk;portReport.value=null;processLogs.value=[];outputCursor.value=0;resourceHistory.value=[];timeline.value=[];runHistory.value=[];void pollProcessLogs();void loadResourceHistory();void loadTimeline();void loadPortDiagnostics();void loadRunHistory()}
function newProcess(){showProcEnvEditor.value=false;editingID.value='';selectedPID.value='';procName.value='';procExecutable.value='';procArgs.value='[]';procCwd.value='';procEnv.value='{}';procDependsOn.value=[];procRestartMode.value='none';procRestartAttempts.value=3;procHealthType.value='none';procHealthTarget.value='';procHealthInterval.value=10;procHealthTimeout.value=2;procHealthThreshold.value=3;procReadyTimeout.value=60;procPorts.value='';procLogToDisk.value=false;portReport.value=null;processLogs.value=[];outputCursor.value=0;resourceHistory.value=[];timeline.value=[];runHistory.value=[]}
async function saveProcess(){const a=api();if(!a)return;processError.value='';processMessage.value='';let args:string[],env:Record<string,string>,ports:number[];try{({args,env}=parseProcessForm(procArgs.value,procEnv.value));ports=parsePortList(procPorts.value)}catch(e){processError.value=t('process.errConfigFormat')+String(e);return}const health:ProcessHealthCheck|null=procHealthType.value==='none'?null:{type:procHealthType.value,target:procHealthTarget.value.trim(),intervalSeconds:procHealthInterval.value,timeoutSeconds:procHealthTimeout.value,failureThreshold:procHealthThreshold.value};processBusy.value=true;try{const saved=await a.SaveProcess({id:editingID.value,name:procName.value,executable:procExecutable.value,args,workingDir:procCwd.value,env,autoStart:false,dependsOn:procDependsOn.value,restart:{mode:procRestartMode.value,maxAttempts:procRestartAttempts.value},health,readyTimeoutSeconds:procHealthType.value==='none'?0:procReadyTimeout.value,ports,logToDisk:procLogToDisk.value});await loadProcesses();selectProcess(saved)}catch(e){processError.value=String(e)}finally{processBusy.value=false}}
async function processAction(action:ProcAction){const a=api();if(!a||!selectedProcess.value)return;processBusy.value=true;processError.value='';try{if(action==='start')await a.StartProcess(selectedPID.value);else if(action==='stop')await a.StopProcess(selectedPID.value);else await a.RestartProcess(selectedPID.value);await refreshProcesses();await loadResourceHistory();await loadTimeline();await loadRunHistory();await loadPortDiagnostics()}catch(e){processError.value=String(e)}finally{processBusy.value=false}}
async function batchProcessAction(action:'start'|'stop'){
 const a=api()
 if(!a||!processes.value.length)return
 processBusy.value=true;processError.value='';batchResults.value=[]
 try{
  const ids=processes.value.map(item=>item.spec.id)
  batchResults.value=action==='start'?await a.StartProcesses(ids):await a.StopProcesses(ids)
  const failed=batchResults.value.filter(item=>!item.success)
  const success=batchResults.value.length-failed.length
  if(failed.length)processError.value=action==='start'?t('process.batchResultStartFail',{ok:success,fail:failed.length}):t('process.batchResultStopFail',{ok:success,fail:failed.length})
  await refreshProcesses()
 }catch(e){processError.value=String(e)}
 finally{processBusy.value=false}
}
async function exportProcessConfigs(redactSensitive:boolean){
 const a=api();if(!a)return
 processError.value='';processMessage.value=''
 if(!redactSensitive&&!confirm(t('process.exportConfirmFull')))return
 processBusy.value=true
 try{const message=await a.ExportProcessConfigs(redactSensitive);if(message)processMessage.value=message}
 catch(e){processError.value=t('process.errExportConfig')+String(e)}
 finally{processBusy.value=false}
}
async function importProcessConfigs(){
 const a=api();if(!a)return
 if(!confirm(t('process.importConfirm')))return
 processBusy.value=true;processError.value='';processMessage.value=''
 try{const imported=await a.ImportProcessConfigs();if(imported.length){await loadProcesses();selectProcess(imported[0]);processMessage.value=t('process.importOk',{n:imported.length})}}
 catch(e){processError.value=t('process.errImportConfig')+String(e)}
 finally{processBusy.value=false}
}
async function deleteProcess(){const a=api();if(!a||!selectedProcess.value)return;if(!confirm(t('process.deleteConfirm',{name:selectedProcess.value.spec.name})))return;try{await a.DeleteProcess(selectedPID.value);newProcess();await loadProcesses();await loadGroups()}catch(e){processError.value=String(e)}}
// 导出当前已缓存的运行输出；不额外回溯磁盘或已淘汰的历史行。
async function exportProcessLogs(format:ExportFormat){
 const a=api(),view=selectedProcess.value
 if(!a||!view||!processLogs.value.length)return
 processError.value='';processMessage.value=''
 try{
  const content=format==='csv'?buildProcessLogExportCSV(processLogs.value):format==='txt'?buildProcessLogExportTXT(processLogs.value):buildProcessLogExportJSON(view.spec.id,view.spec.name,processLogs.value)
  const saved=await a.SaveExportFile(processLogExportFileName(view.spec.name,format),format,content)
  if(saved)processMessage.value=t('process.logExported',{n:processLogs.value.length,path:saved})
 }catch(e){processError.value=t('process.errExportLog')+String(e)}
}
function stateName(s:ProcessStatus['state']){return ({stopped:t('process.stateStopped'),starting:t('process.stateStarting'),running:t('process.stateRunning'),stopping:t('process.stateStopping'),failed:t('process.stateFailed')} as Record<ProcessStatus['state'],string>)[s]}
function healthName(health?:ProcessStatus['health']){return health?({checking:t('process.healthChecking'),healthy:t('process.healthHealthy'),unhealthy:t('process.healthUnhealthy')} as Record<NonNullable<ProcessStatus['health']>,string>)[health]:''}
function portStateName(item:ProcessPortStatus){if(item.state==='free')return t('process.portFree');if(item.state==='owned')return t('process.portOwned');if(item.state==='occupied'){if(item.ownerSpecId)return t('process.portOccupiedBySelf',{name:item.ownerName||item.ownerSpecId});if(item.process)return t('process.portOccupied',{pid:item.pid,proc:item.process});return t('process.portOccupiedUnknown',{pid:item.pid})}return t('process.portUnknown')}
function exitReasonName(reason?:ProcessStatus['exitReason']){
 if(!reason)return ''
 return ({normal:t('process.exitNormal'),nonzero:t('process.exitNonzero'),signal:t('process.exitSignal'),'manual-stop':t('process.exitManualStop'),'start-error':t('process.exitStartError')} as Record<NonNullable<ProcessStatus['exitReason']>,string>)[reason]
}
function processStatusSummary(status:ProcessStatus){
 const label=exitReasonName(status.exitReason)
 if(!label)return ''
 if(status.state==='failed')return label+t('process.statusColon')+(status.error||t('process.noErrorInfo'))
 return t('process.lastRun')+label
}
function formatLocalTime(value?:string){if(!value)return '—';const date=new Date(value);return Number.isNaN(date.getTime())?'—':date.toLocaleString()}
function formatRunDuration(status:ProcessStatus){
 const start=status.startedAt?Date.parse(status.startedAt):NaN
 const active=status.state==='running'||status.state==='starting'||status.state==='stopping'
 const elapsed=active&&Number.isFinite(start)?Math.max(0,Date.now()-start):(status.durationMs??0)
 const totalSeconds=Math.floor(elapsed/1000),hours=Math.floor(totalSeconds/3600),minutes=Math.floor((totalSeconds%3600)/60),seconds=totalSeconds%60
 if(hours>0)return hours+t('process.fmtHours')+String(minutes).padStart(2,'0')+t('process.fmtMins')+String(seconds).padStart(2,'0')+t('process.fmtSecs')
 if(minutes>0)return minutes+t('process.fmtMins')+String(seconds).padStart(2,'0')+t('process.fmtSecs')
 return totalSeconds+t('process.fmtSecs')
}
function formatEventTime(value:string){const date=new Date(value);return Number.isNaN(date.getTime())?'—':date.toLocaleTimeString()}
function formatBytes(bytes?:number){
 if(!bytes||bytes<=0)return '—'
 const units=['B','KB','MB','GB','TB']
 let value=bytes,index=0
 while(value>=1024&&index<units.length-1){value/=1024;index++}
 return (value>=100||index===0?Math.round(value).toString():value.toFixed(1))+' '+units[index]
}

let processTimer:number|undefined, historyTimer:number|undefined
// 日志分析反查到服务后通过 focusId 请求选中它；找到即切换并通知父组件清空，避免重复触发。
watch(()=>props.focusId,(id)=>{if(!id)return;const view=processes.value.find(item=>item.spec.id===id);if(view){selectProcess(view);emit('focus-handled')}})
onMounted(()=>{
 void loadProcesses()
 void loadGroups()
 processTimer=window.setInterval(()=>{if(props.active)void refreshProcesses()},1000)
 historyTimer=window.setInterval(()=>{if(props.active&&selectedPID.value){void loadResourceHistory();void loadTimeline();void loadRunHistory()}},3000)
})
onBeforeUnmount(()=>{if(processTimer!==undefined)clearInterval(processTimer);if(historyTimer!==undefined)clearInterval(historyTimer)})
</script>

<template>
   <section v-show="props.active" class="page-content process-page">
    <div class="page-heading"><div><div class="eyebrow"><span class="eyebrow-line"></span>{{ $t("process.pageEyebrow") }}</div><h1>{{ $t("process.pageTitle") }}<span class="title-period">。</span></h1><p class="page-subtitle">{{ $t("process.pageSubtitle") }}</p></div><div class="process-heading-actions"><button class="button button-secondary" :disabled="processBusy||!processes.length" @click="batchProcessAction('start')"><Play :size="14"/>{{ $t("process.batchStart") }}</button><button class="button button-secondary" :disabled="processBusy||!processes.length" @click="batchProcessAction('stop')"><Square :size="13"/>{{ $t("process.batchStop") }}</button><button class="button button-primary" @click="newProcess"><Plus :size="15"/>{{ $t("process.newService") }}</button></div></div>
    <div v-if="processError" class="notice notice-error process-notice"><span>!</span>{{processError}}</div>
    <div v-if="processMessage" class="notice notice-success process-notice"><span>✓</span>{{processMessage}}</div>
    <div class="process-config-toolbar">
     <div class="process-config-copy"><strong>{{ $t("process.migration") }}</strong><span>{{ $t("process.migrationHint") }}</span></div>
     <div class="process-config-actions">
      <button class="button button-secondary" :disabled="processBusy" @click="importProcessConfigs"><FileUp :size="14"/>{{ $t("process.importConfig") }}</button>
      <button class="button button-secondary" :disabled="processBusy" @click="exportProcessConfigs(true)"><FileDown :size="14"/>{{ $t("process.exportRedacted") }}</button>
      <button class="button button-quiet" :disabled="processBusy" @click="exportProcessConfigs(false)"><ShieldCheck :size="14"/>{{ $t("process.exportFull") }}</button>
     </div>
    </div>
    <div v-if="batchResults.length" class="batch-results">
     <div class="batch-results-header"><strong>{{ $t("process.recentBatch") }}</strong><span>{{ $t("process.batchSuccess") }} {{batchResults.filter(item=>item.success).length}} / {{batchResults.length}}</span></div>
     <div v-for="item in batchResults" :key="item.action+'-'+item.id" class="batch-result-row" :class="{failed:!item.success}"><span class="batch-result-mark">{{item.success?'✓':'!'}}</span><strong>{{item.name}}</strong><span>{{item.message}}</span></div>
    </div>
    <section v-if="processes.length" class="dependency-graph">
     <div class="graph-head"><span><Network :size="14"/>{{ $t("process.dependencyGraph") }}</span><small>{{ $t("process.servicesCount",{count:dependencyGraph.nodes.length,layers:dependencyGraph.layers}) }}<template v-if="dependencyGraph.cyclic.length">{{ $t("process.cyclicDetected") }}</template></small></div>
     <div class="graph-scroll">
      <svg :viewBox="'0 0 '+dependencyGraph.width+' '+dependencyGraph.height" :width="dependencyGraph.width" :height="dependencyGraph.height" role="img" :aria-label="$t('process.dependencyGraph')">
       <path v-for="edge in dependencyGraph.edges" :key="edge.from+'-'+edge.to" :d="edge.path" class="graph-edge" :class="{cyclic:edge.cyclic}"/>
       <g v-for="node in dependencyGraph.nodes" :key="node.id" class="graph-node" :class="[graphNodeState(node.id),{selected:selectedPID===node.id}]" role="button" tabindex="0" @click="selectGraphNode(node.id)" @keydown.enter="selectGraphNode(node.id)">
        <title>{{node.name}}<template v-if="node.dependsOn.length"> · {{ $t("process.graphDepCount",{count:node.dependsOn.length}) }}</template><template v-else> · {{ $t("process.graphNoDep") }}</template></title>
        <rect :x="node.x" :y="node.y" :width="node.width" :height="node.height" rx="6"/>
        <circle :cx="node.x+13" :cy="node.y+node.height/2" r="3.5" class="graph-dot"/>
        <text :x="node.x+24" :y="node.y+node.height/2+3.5">{{graphNodeLabel(node.name)}}</text>
       </g>
      </svg>
     </div>
     <p class="graph-hint">{{ $t("process.graphHint") }}</p>
    </section>
    <section v-if="processes.length" class="group-panel">
     <div class="group-head"><div><strong><FolderKanban :size="14"/>{{ $t("process.groupsTitle") }}</strong><p>{{ $t("process.groupsDesc") }}</p></div></div>
     <div class="group-list">
      <div v-for="group in groups" :key="group.id" class="group-item">
       <div class="group-item-main"><strong>{{group.name}}</strong><small v-if="group.description">{{group.description}}</small><small>{{groupNamesFor(group.memberIds).join('、')||$t('process.emptyGroup')}}</small><em v-if="groupMissing(group.memberIds)" class="group-warn">{{ $t("process.deletedWarn",{count:groupMissing(group.memberIds)}) }}</em></div>
       <div class="group-item-actions"><button class="button button-secondary" :disabled="processBusy" @click="groupAction(group.id,'start')"><Play :size="12"/>{{ $t("process.groupStart") }}</button><button class="button button-quiet" :disabled="processBusy" @click="groupAction(group.id,'stop')"><Square :size="12"/>{{ $t("process.groupStop") }}</button><button class="button button-quiet" :disabled="processBusy" @click="editGroup(group)">{{ $t("process.edit") }}</button><button class="button button-danger" :disabled="processBusy" @click="deleteGroup(group.id)"><Trash2 :size="12"/>{{ $t("process.delete") }}</button></div>
      </div>
      <p v-if="!groups.length" class="group-empty">{{ $t("process.noGroups") }}</p>
     </div>
     <div class="group-editor">
      <div class="group-editor-fields"><input v-model="groupName" class="text-field" :placeholder="$t('process.groupNamePlaceholder')" :disabled="processBusy" :aria-label="$t('process.groupName')"/><input v-model="groupDescription" class="text-field" :placeholder="$t('process.groupDescPlaceholder')" :disabled="processBusy" :aria-label="$t('process.groupDesc')"/></div>
      <div class="group-member-picker"><span class="group-picker-label">{{ $t("process.pickMembers",{count:groupMembers.length}) }}</span><label v-for="candidate in processes" :key="candidate.spec.id" class="group-member-item"><input type="checkbox" :checked="groupMembers.includes(candidate.spec.id)" :disabled="processBusy" @change="toggleGroupMember(candidate.spec.id)"/><span><strong>{{candidate.spec.name}}</strong><small>{{candidate.spec.id}}</small></span></label></div>
      <div class="group-editor-actions"><button class="button button-primary" :disabled="processBusy" @click="saveGroup"><Save :size="13"/>{{groupEditingID?$t("process.saveGroup"):$t("process.createGroup")}}</button><button v-if="groupEditingID" class="button button-secondary" :disabled="processBusy" @click="newGroup">{{ $t("process.cancelEdit") }}</button></div>
     </div>
    </section>
    <div class="process-layout">
     <section class="process-panel"><div class="process-panel-heading"><div><h2>{{ $t("process.serviceList") }}</h2><p>{{ $t("process.serviceListCount",{n:processes.length}) }}</p></div><button class="icon-button" @click="refreshProcesses"><RefreshCw :size="15"/></button></div><div v-if="processes.length" class="process-list"><button v-for="p in processes" :key="p.spec.id" class="process-item" :class="{selected:selectedPID===p.spec.id}" @click="selectProcess(p)"><span class="process-state-dot" :class="p.status.state"></span><span class="process-item-main"><strong>{{p.spec.name}}</strong><small>{{p.spec.executable}}</small><small v-if="p.status.exitReason" class="process-item-diagnostic" :class="{failed:p.status.state==='failed'}">{{processStatusSummary(p.status)}}</small><em>{{stateName(p.status.state)}}<template v-if="p.status.pid"> · PID {{p.status.pid}}</template><template v-if="p.status.health"> · {{healthName(p.status.health)}}</template><template v-if="p.status.memoryBytes"> · {{ $t("process.memoryLabel") }} {{formatBytes(p.status.memoryBytes)}}</template><template v-if="p.status.cpuPercent"> · CPU {{p.status.cpuPercent.toFixed(1)}}%</template><template v-if="p.status.restarts"> · {{ $t("process.restartedCount",{n:p.status.restarts}) }}</template></em></span><ChevronRight :size="14"/></button></div><div v-else class="process-empty"><TerminalSquare :size="25"/><strong>{{ $t("process.noServicesYet") }}</strong><span>{{ $t("process.noServicesHint") }}</span><button class="button button-secondary" @click="newProcess"><Plus :size="14"/>{{ $t("process.createFirstService") }}</button></div></section>
     <section class="process-panel"><div class="process-panel-heading"><div><h2>{{editingID?$t("process.configTitle"):$t("process.newConfigTitle")}}</h2><p>{{ $t("process.configSubtitle") }}</p></div><span class="safe-pill"><ShieldCheck :size="13"/>{{ $t("process.explicitArgs") }}</span></div>
      <form class="process-form" @submit.prevent="saveProcess"><label class="field-label">{{ $t("process.fieldName") }}</label><input v-model="procName" class="text-field" :placeholder="$t('process.fieldNamePlaceholder')" required/><label class="field-label">{{ $t("process.fieldExecutable") }}</label><input v-model="procExecutable" class="text-field mono-field" :placeholder="$t('process.fieldExecutablePlaceholder')" required/><label class="field-label">{{ $t("process.fieldArgs") }}</label><textarea v-model="procArgs" class="text-area mono-field" rows="3" spellcheck="false"></textarea><label class="field-label">{{ $t("process.fieldCwd") }}</label><input v-model="procCwd" class="text-field mono-field" :placeholder="$t('process.fieldCwdPlaceholder')"/><label class="field-label">{{ $t("process.fieldPorts") }}</label><input v-model="procPorts" class="text-field mono-field" :placeholder="$t('process.fieldPortsPlaceholder')" :aria-label="$t('process.fieldPorts')"/><p class="policy-hint">{{ $t("process.fieldPortsHint") }}</p><label class="toggle-option proc-log-toggle"><input v-model="procLogToDisk" type="checkbox" :disabled="processBusy"/><span class="custom-checkbox"><HardDrive v-if="procLogToDisk" :size="12"/></span><span>{{ $t("process.logToDisk") }}</span></label><p class="policy-hint">{{ $t("process.logToDiskHint") }}</p><div class="env-editor-heading"><label class="field-label">{{ $t("process.fieldEnv") }}</label><button type="button" class="button button-quiet env-toggle" @click="showProcEnvEditor=!showProcEnvEditor"><EyeOff v-if="showProcEnvEditor" :size="13"/><Eye v-else :size="13"/>{{showProcEnvEditor?$t("process.hideEnv"):$t("process.editEnv")}}</button></div><textarea v-if="showProcEnvEditor" v-model="procEnv" class="text-area mono-field" rows="4" spellcheck="false"></textarea><p v-if="showProcEnvEditor" class="env-editor-help">{{ $t("process.envEditHint") }}</p><pre v-else class="env-preview mono-field">{{maskedProcEnvPreview}}</pre>
       <label class="field-label">{{ $t("process.fieldDependsOn") }}</label>
       <div class="dependency-list">
        <label v-for="candidate in processes.filter(item=>item.spec.id!==editingID)" :key="candidate.spec.id" class="dependency-item">
         <input v-model="procDependsOn" type="checkbox" :value="candidate.spec.id" :disabled="processBusy"/>
         <span><strong>{{candidate.spec.name}}</strong><small>{{candidate.spec.id}}</small></span>
        </label>
        <span v-if="!processes.some(item=>item.spec.id!==editingID)" class="dependency-empty">{{ $t("process.dependsOnEmpty") }}</span>
       </div>
       <label class="field-label">{{ $t("process.fieldRestart") }}</label>
       <div class="process-policy-row">
        <label class="option-select">{{ $t("process.restartStrategy") }} <select v-model="procRestartMode" :disabled="processBusy" :aria-label="$t('process.fieldRestart')"><option value="none">{{ $t("process.restartNone") }}</option><option value="on-failure">{{ $t("process.restartOnFailure") }}</option></select></label>
        <label v-if="procRestartMode==='on-failure'" class="option-select">{{ $t("process.restartMax") }} <input v-model.number="procRestartAttempts" class="text-field mono-field policy-number" type="number" min="1" max="20" :disabled="processBusy" :aria-label="$t('process.restartMax')"/> {{ $t("process.restartTimes") }}</label>
       </div>
       <p class="policy-hint">{{ $t("process.restartHint") }}</p>
       <label class="field-label">{{ $t("process.fieldHealth") }}</label>
       <div class="process-policy-row">
        <label class="option-select">{{ $t("process.healthTypeLabel") }} <select v-model="procHealthType" :disabled="processBusy" :aria-label="$t('process.fieldHealth')"><option value="none">{{ $t("process.healthNone") }}</option><option value="tcp">{{ $t("process.healthTCP") }}</option><option value="http">{{ $t("process.healthHTTP") }}</option></select></label>
        <input v-if="procHealthType!=='none'" v-model="procHealthTarget" class="text-field mono-field policy-target" :placeholder="procHealthType==='tcp'?'127.0.0.1:8080':'http://127.0.0.1:8080/health'" :disabled="processBusy" :aria-label="$t('process.healthTarget')"/>
       </div>
       <div v-if="procHealthType!=='none'" class="process-policy-row">
        <label class="option-select">{{ $t("process.healthInterval") }} <input v-model.number="procHealthInterval" class="text-field mono-field policy-number" type="number" min="1" max="300" :disabled="processBusy" :aria-label="$t('process.healthInterval')"/> {{ $t("process.healthSecs") }}</label>
        <label class="option-select">{{ $t("process.healthTimeout") }} <input v-model.number="procHealthTimeout" class="text-field mono-field policy-number" type="number" min="1" max="30" :disabled="processBusy" :aria-label="$t('process.healthTimeout')"/> {{ $t("process.healthSecs") }}</label>
        <label class="option-select">{{ $t("process.healthThreshold") }} <input v-model.number="procHealthThreshold" class="text-field mono-field policy-number" type="number" min="1" max="10" :disabled="processBusy" :aria-label="$t('process.healthThreshold')"/> {{ $t("process.healthTimes") }}</label>
       </div>
       <p v-if="procHealthType!=='none'" class="policy-hint">{{ $t("process.healthHint") }}</p>
       <div v-if="procHealthType!=='none'" class="process-policy-row">
        <label class="option-select">{{ $t("process.readyTimeout") }} <input v-model.number="procReadyTimeout" class="text-field mono-field policy-number" type="number" min="0" max="600" :disabled="processBusy" :aria-label="$t('process.readyTimeout')"/> {{ $t("process.healthSecs") }}</label>
       </div>
       <p v-if="procHealthType!=='none'" class="policy-hint">{{ $t("process.readyTimeoutHint") }}</p>
       <div class="process-form-actions"><button type="submit" class="button button-primary" :disabled="processBusy"><Save :size="15"/>{{ $t("process.saveConfig") }}</button><button type="button" class="button button-secondary" @click="newProcess">{{ $t("process.clearForm") }}</button><button v-if="editingID" type="button" class="button button-danger" @click="deleteProcess"><Trash2 :size="14"/>{{ $t("process.deleteConfig") }}</button></div></form>
      <div v-if="selectedProcess" class="process-runtime"><div class="runtime-heading"><div><h3>{{ $t("process.runtimeControl") }}</h3><p>{{stateName(selectedProcess.status.state)}}<template v-if="selectedProcess.status.pid"> · PID {{selectedProcess.status.pid}}</template><template v-if="selectedProcess.status.exitCode!==undefined"> · {{ $t("process.exitCode") }} {{selectedProcess.status.exitCode}}</template><template v-if="selectedProcess.status.health"> · {{healthName(selectedProcess.status.health)}}</template></p><div v-if="selectedProcess.status.startedAt || selectedProcess.status.finishedAt" class="process-lifecycle-meta"><span v-if="selectedProcess.status.startedAt">{{ $t("process.metaStart") }}<code>{{formatLocalTime(selectedProcess.status.startedAt)}}</code></span><span v-if="selectedProcess.status.finishedAt">{{ $t("process.metaEnd") }}<code>{{formatLocalTime(selectedProcess.status.finishedAt)}}</code></span><span v-if="selectedProcess.status.startedAt">{{ $t("process.metaDuration") }}<strong>{{formatRunDuration(selectedProcess.status)}}</strong></span><span v-if="selectedProcess.status.memoryBytes">{{ $t("process.memoryLabel") }}<strong>{{formatBytes(selectedProcess.status.memoryBytes)}}</strong><template v-if="selectedProcess.status.cpuPercent"> · CPU{{ $t("process.statusColon") }}<strong>{{selectedProcess.status.cpuPercent.toFixed(1)}}%</strong></template></span><span v-if="selectedProcess.status.restarts">{{ $t("process.metaRestarts") }}<strong>{{selectedProcess.status.restarts}}</strong> {{ $t("process.restartTimes") }}</span><span v-if="selectedProcess.status.nextRestartAt">{{ $t("process.metaNextRestart") }}<code>{{formatLocalTime(selectedProcess.status.nextRestartAt)}}</code></span></div><div v-if="selectedProcess.status.healthMessage" class="process-lifecycle-meta"><span>{{ $t("process.metaHealthDiag") }}<code>{{selectedProcess.status.healthMessage}}</code></span></div></div><div class="runtime-actions"><button class="button button-primary" :disabled="processBusy||selectedProcess.status.state==='running'||selectedProcess.status.state==='starting'" @click="processAction('start')"><Play :size="13"/>{{ $t("process.actionStart") }}</button><button class="button button-secondary" :disabled="processBusy||(selectedProcess.status.state!=='running'&&!selectedProcess.status.nextRestartAt)" @click="processAction('stop')"><Square :size="13"/>{{selectedProcess.status.nextRestartAt?$t('process.actionCancelRestart'):$t('process.actionStop')}}</button><button class="button button-secondary" :disabled="processBusy||selectedProcess.status.state==='running'||selectedProcess.status.state==='starting'" @click="processAction('restart')"><RefreshCw :size="13"/>{{ $t("process.actionRestart") }}</button></div></div><p v-if="selectedProcess.status.error" class="process-error-text">{{selectedProcess.status.error}}</p><div v-if="selectedProcess.status.logFilePath" class="disk-log-panel"><div class="disk-log-head"><span><HardDrive :size="14"/>{{ $t("process.logToDisk") }}</span><button class="button button-quiet" type="button" :title="$t('process.openInAnalysisTitle')" @click="openLogInAnalysis"><ExternalLink :size="12"/>{{ $t("process.openInAnalysis") }}</button><button v-if="logGrep.trim()" class="button button-quiet" type="button" :title="$t('process.grepInAnalysisTitle',{q:logGrep})" @click="openLogWithGrep"><Search :size="12"/>{{ $t("process.grepInAnalysis",{q:logGrep}) }}</button></div><code class="disk-log-path">{{selectedProcess.status.logFilePath}}</code><p class="disk-log-hint">{{ $t("process.diskLogHint") }}</p></div><div v-if="historySummary.count" class="resource-history"><div class="resource-history-head"><span><LineChart :size="14"/>{{ $t("process.resources5min") }}</span><small>{{ $t("process.samplePoints",{count:historySummary.count}) }}<template v-if="selectedProcess.status.metricsAt"> · {{ $t("process.lastSample") }} {{formatLocalTime(selectedProcess.status.metricsAt)}}</template></small></div><div class="resource-charts"><figure class="resource-chart"><figcaption>{{ $t("process.peakCPU") }} <strong>{{historySummary.peakCPU.toFixed(1)}}%</strong></figcaption><svg viewBox="0 0 120 32" preserveAspectRatio="none" role="img" :aria-label="$t('process.cpuChart')"><polyline class="sparkline cpu" :points="cpuPoints"/></svg></figure><figure class="resource-chart"><figcaption>{{ $t("process.peakMemory") }} <strong>{{formatBytes(historySummary.peakMemory)}}</strong><em v-if="memoryDeltaLabel">{{memoryDeltaLabel}}</em></figcaption><svg viewBox="0 0 120 32" preserveAspectRatio="none" role="img" :aria-label="$t('process.memChart')"><polyline class="sparkline memory" :points="memoryPoints"/></svg></figure></div><p class="resource-history-hint">{{ $t("process.resourceHint") }}</p></div><div v-if="timeline.length" class="process-timeline"><div class="timeline-head"><span><Clock :size="14"/>{{ $t("process.timeline") }}</span><small>{{ $t("process.eventCount",{n:timeline.length}) }}<template v-if="latestStartup!==null"> · {{ $t("process.lastStartupTime",{t:formatEventDuration(latestStartup)}) }}</template><template v-if="restartEvents"> · {{ $t("process.restartEvents",{n:restartEvents}) }}</template></small></div><ol class="event-list"><li v-for="(event,index) in timeline" :key="index+event.kind+event.at" class="event-row" :class="event.kind"><span class="event-dot"></span><span class="event-name">{{eventLabel(event.kind)}}</span><code class="event-time">{{formatEventTime(event.at)}}</code><span class="event-detail">{{event.detail||''}}</span><em v-if="event.durationMs!==undefined">{{event.kind==='exit'?$t('process.eventRun'):$t('process.eventDuration')}}{{formatEventDuration(event.durationMs)}}</em></li></ol><p class="timeline-hint">{{ $t("process.timelineHint") }}</p></div><div v-if="runHistory.length" class="run-history"><div class="run-history-head"><span><History :size="14"/>{{ $t("process.runHistory") }}</span><small>{{ $t("process.runCountTotal",{n:runHistorySummary.count}) }}<template v-if="runHistorySummary.nonzeroExits"> · {{ $t("process.runAbnormalExit",{n:runHistorySummary.nonzeroExits}) }}</template><template v-if="runHistorySummary.totalRestarts"> · {{ $t("process.runTotalRestarts",{n:runHistorySummary.totalRestarts}) }}</template></small></div><ol class="run-list"><li v-for="(run,index) in runHistory.slice(-10).reverse()" :key="index+'-'+(run.finishedAt||run.startedAt)" class="run-row"><span class="run-index">{{ $t("process.runIndex",{n:runHistory.length-index}) }}</span><code class="run-time">{{formatLocalTime(run.finishedAt||run.startedAt)}}</code><span class="run-duration">{{formatRunDurationLabel(run.durationMs)}}</span><span class="run-exit" :class="{abnormal:run.exitReason==='nonzero'||run.exitReason==='signal'||run.exitReason==='start-error'}">{{runExitSummary(run.exitReason,run.exitCode)}}</span><em>{{ $t("process.peakCPU") }} {{formatRunPeakCPU(run.peakCpuPercent)}} · {{ $t("process.peakMemory") }} {{formatRunBytes(run.peakMemoryBytes)}}</em></li></ol><p class="run-history-hint">{{ $t("process.runHistoryHint") }}</p></div><div class="port-diagnostics"><div class="port-head"><span><Network :size="14"/>{{ $t("process.portDiagnostics") }}</span><small v-if="portReport?.scannedAt">{{ $t("process.scannedAt") }} {{formatLocalTime(portReport.scannedAt)}}</small><button class="button button-quiet" type="button" :disabled="loadingPorts" @click="loadPortDiagnostics"><RefreshCw :size="12"/>{{loadingPorts?$t('process.scanning'):$t('process.refresh')}}</button></div><p v-if="portReport?.scanError" class="port-hint">{{ $t("process.portScanError") }} {{portReport.scanError}}</p><template v-else><div v-if="portReport?.ports.length" class="port-group"><h4>{{ $t("process.configPorts") }}</h4><div class="port-rows"><span v-for="item in portReport.ports" :key="'d-'+item.port" class="port-row" :class="item.state"><code>{{item.port}}</code>{{portStateName(item)}}<button v-if="item.ownerSpecId" class="button button-quiet port-stop" type="button" :disabled="processBusy" :title="$t('process.stopOccupyingService')" @click="stopOccupyingService(item.ownerSpecId)"><Square :size="11"/>{{ $t("process.stopThisService") }}</button></span></div></div><div v-if="portReport?.listening.length" class="port-group"><h4>{{ $t("process.listeningPorts") }}</h4><div class="port-rows"><span v-for="item in portReport.listening" :key="'l-'+item.port" class="port-row owned"><code>{{item.port}}</code>{{ $t("process.listening") }}</span></div></div><p v-if="!portReport?.ports.length&&!portReport?.listening.length" class="port-hint">{{ $t("process.noPortDeclared") }}</p></template><p class="port-hint">{{ $t("process.portDiagnosticsHint") }}</p></div><div class="log-stream-heading"><span><Activity :size="14"/>{{ $t("process.logStream") }}</span><div class="log-stream-actions"><div class="log-grep-wrap"><Search :size="12"/><input class="log-grep-input" v-model="logGrep" type="search" :placeholder="$t('process.logGrepPlaceholder')" :title="$t('process.logGrepTitle',{n:processLogs.length})" /><em v-if="logGrep">{{ $t("process.logMatched",{n:grepMatchCount}) }}</em></div><button class="button button-quiet" type="button" :class="{paused:autoScrollPaused}" @click="autoScrollPaused=!autoScrollPaused" :title="autoScrollPaused?$t('process.autoScrollPaused'):$t('process.autoScrollOn')"><Pause v-if="!autoScrollPaused" :size="12"/><Play v-else :size="12"/>{{autoScrollPaused?$t('process.paused'):$t('process.autoScroll')}}</button><span>{{ $t("process.logCached",{n:processLogs.length}) }}</span><button class="button button-quiet" type="button" :disabled="!processLogs.length" :title="$t('process.exportCsvTitle')" @click="exportProcessLogs('csv')"><FileDown :size="13"/>CSV</button><button class="button button-quiet" type="button" :disabled="!processLogs.length" :title="$t('process.exportJsonTitle')" @click="exportProcessLogs('json')"><FileDown :size="13"/>JSON</button><button class="button button-quiet" type="button" :disabled="!processLogs.length" :title="$t('process.exportTxtTitle')" @click="exportProcessLogs('txt')"><FileDown :size="13"/>TXT</button></div></div><div class="process-log-output" ref="logOutputEl"><div v-for="line in processLogs" :key="line.sequence" class="process-log-line" :class="line.stream" :title="logGrepRegex && logGrepRegex.test(line.text) ? $t('process.grepMatched') : ''"><span>{{line.time}}</span><b>{{line.stream==='stdout'?'OUT':line.stream==='stderr'?'ERR':'SYS'}}</b><code v-html="highlightLogText(line.text)"></code></div><div v-if="!processLogs.length" class="process-log-empty">{{ $t("process.logEmptyHint") }}</div></div></div>
     </section>
    </div>
    <div class="bottom-note"><ShieldCheck :size="15"/><span>{{ $t("process.bottomNote") }}</span></div>
   </section>
</template>