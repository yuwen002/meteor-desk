<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Activity, ChevronRight, Eye, EyeOff, FileDown, FileUp, LineChart, Network, Play, Plus, RefreshCw, Save, ShieldCheck, Square, TerminalSquare, Trash2 } from 'lucide-vue-next'
import { getMaskedEnvironmentPreview, parsePortList, parseProcessForm } from '../utils/processConfig'
import { buildProcessLogExportCSV, buildProcessLogExportJSON, processLogExportFileName, type ExportFormat } from '../utils/exporters'
import { sparklinePoints, summarizeHistory } from '../utils/resourceHistory'

type ProcAction = 'start' | 'stop' | 'restart'

const props = defineProps<{ active: boolean }>()
const processes=ref<ProcessView[]>([]), selectedPID=ref(''), processLogs=ref<ProcessOutput[]>([]), outputCursor=ref(0), processError=ref(''), processBusy=ref(false)
const batchResults=ref<ProcessBatchResult[]>([]), processMessage=ref(''), showProcEnvEditor=ref(false)
const editingID=ref(''), procName=ref(''), procExecutable=ref(''), procArgs=ref('[]'), procCwd=ref(''), procEnv=ref('{}'), procDependsOn=ref<string[]>([])
const procRestartMode=ref<'none'|'on-failure'>('none'), procRestartAttempts=ref(3)
const procReadyTimeout=ref(60)
const procPorts=ref(''), portReport=ref<ProcessPortReport|null>(null), loadingPorts=ref(false)
const procHealthType=ref<'none'|'tcp'|'http'>('none'), procHealthTarget=ref(''), procHealthInterval=ref(10), procHealthTimeout=ref(2), procHealthThreshold=ref(3)
const selectedProcess=computed(()=>processes.value.find(p=>p.spec.id===selectedPID.value)||null)
const maskedProcEnvPreview=computed(()=>getMaskedEnvironmentPreview(procEnv.value))
const resourceHistory=ref<ProcessResourceSample[]>([])
const historySummary=computed(()=>summarizeHistory(resourceHistory.value))
const cpuPoints=computed(()=>sparklinePoints(historySummary.value.cpuValues,120,32))
const memoryPoints=computed(()=>sparklinePoints(historySummary.value.memoryValues,120,32))
const memoryDeltaLabel=computed(()=>{const delta=historySummary.value.memoryDelta;if(!delta)return '';return (delta>0?'+':'-')+formatBytes(Math.abs(delta))})

function api(){return window.go?.main?.App}
async function loadProcesses(){const a=api();if(!a)return;try{processes.value=await a.GetProcesses();if(!selectedPID.value&&processes.value.length)selectedPID.value=processes.value[0].spec.id;if(selectedPID.value&&!processes.value.some(x=>x.spec.id===selectedPID.value))selectedPID.value=processes.value[0]?.spec.id??''}catch(e){processError.value=String(e)}}
async function pollProcessLogs(){const a=api();if(!a||!selectedPID.value)return;try{const lines=await a.GetProcessLogs(selectedPID.value,outputCursor.value);if(lines.length){processLogs.value=[...processLogs.value,...lines].slice(-1500);outputCursor.value=lines[lines.length-1].sequence}}catch(e){processError.value=String(e)}}
// 资源曲线单独轮询：采样间隔 2 秒，不需要跟随运行输出的 1 秒节奏。
async function loadResourceHistory(){const a=api();if(!a||!selectedPID.value){resourceHistory.value=[];return}try{resourceHistory.value=await a.GetResourceHistory(selectedPID.value)}catch(e){processError.value=String(e)}}
// 端口诊断要调用系统命令，只在选中服务、启停操作后和手动刷新时扫描，不持续轮询。
async function loadPortDiagnostics(){const a=api();if(!a||!selectedPID.value){portReport.value=null;return}loadingPorts.value=true;try{portReport.value=await a.GetPortDiagnostics(selectedPID.value)}catch(e){processError.value=String(e)}finally{loadingPorts.value=false}}
async function refreshProcesses(){await loadProcesses();if(selectedPID.value)await pollProcessLogs()}
function selectProcess(p:ProcessView){showProcEnvEditor.value=false;selectedPID.value=p.spec.id;editingID.value=p.spec.id;procName.value=p.spec.name;procExecutable.value=p.spec.executable;procArgs.value=JSON.stringify(p.spec.args??[],null,2);procCwd.value=p.spec.workingDir??'';procEnv.value=JSON.stringify(p.spec.env??{},null,2);procDependsOn.value=[...(p.spec.dependsOn??[])];procRestartMode.value=p.spec.restart?.mode==='on-failure'?'on-failure':'none';procRestartAttempts.value=p.spec.restart?.maxAttempts||3;const health=p.spec.health??null;procHealthType.value=health?(health.type==='http'?'http':'tcp'):'none';procHealthTarget.value=health?.target??'';procHealthInterval.value=health?.intervalSeconds??10;procHealthTimeout.value=health?.timeoutSeconds??2;procHealthThreshold.value=health?.failureThreshold??3;procReadyTimeout.value=p.spec.readyTimeoutSeconds??0;procPorts.value=(p.spec.ports??[]).join(', ');portReport.value=null;processLogs.value=[];outputCursor.value=0;resourceHistory.value=[];void pollProcessLogs();void loadResourceHistory();void loadPortDiagnostics()}
function newProcess(){showProcEnvEditor.value=false;editingID.value='';selectedPID.value='';procName.value='';procExecutable.value='';procArgs.value='[]';procCwd.value='';procEnv.value='{}';procDependsOn.value=[];procRestartMode.value='none';procRestartAttempts.value=3;procHealthType.value='none';procHealthTarget.value='';procHealthInterval.value=10;procHealthTimeout.value=2;procHealthThreshold.value=3;procReadyTimeout.value=60;procPorts.value='';portReport.value=null;processLogs.value=[];outputCursor.value=0;resourceHistory.value=[]}
async function saveProcess(){const a=api();if(!a)return;processError.value='';processMessage.value='';let args:string[],env:Record<string,string>,ports:number[];try{({args,env}=parseProcessForm(procArgs.value,procEnv.value));ports=parsePortList(procPorts.value)}catch(e){processError.value='配置格式错误：'+String(e);return}const health:ProcessHealthCheck|null=procHealthType.value==='none'?null:{type:procHealthType.value,target:procHealthTarget.value.trim(),intervalSeconds:procHealthInterval.value,timeoutSeconds:procHealthTimeout.value,failureThreshold:procHealthThreshold.value};processBusy.value=true;try{const saved=await a.SaveProcess({id:editingID.value,name:procName.value,executable:procExecutable.value,args,workingDir:procCwd.value,env,autoStart:false,dependsOn:procDependsOn.value,restart:{mode:procRestartMode.value,maxAttempts:procRestartAttempts.value},health,readyTimeoutSeconds:procHealthType.value==='none'?0:procReadyTimeout.value,ports});await loadProcesses();selectProcess(saved)}catch(e){processError.value=String(e)}finally{processBusy.value=false}}
async function processAction(action:ProcAction){const a=api();if(!a||!selectedProcess.value)return;processBusy.value=true;processError.value='';try{if(action==='start')await a.StartProcess(selectedPID.value);else if(action==='stop')await a.StopProcess(selectedPID.value);else await a.RestartProcess(selectedPID.value);await refreshProcesses();await loadResourceHistory();await loadPortDiagnostics()}catch(e){processError.value=String(e)}finally{processBusy.value=false}}
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
// 导出当前已缓存的运行输出；不额外回溯磁盘或已淘汰的历史行。
async function exportProcessLogs(format:ExportFormat){
 const a=api(),view=selectedProcess.value
 if(!a||!view||!processLogs.value.length)return
 processError.value='';processMessage.value=''
 try{
  const content=format==='csv'?buildProcessLogExportCSV(processLogs.value):buildProcessLogExportJSON(view.spec.id,view.spec.name,processLogs.value)
  const saved=await a.SaveExportFile(processLogExportFileName(view.spec.name,format),format,content)
  if(saved)processMessage.value='已导出 '+processLogs.value.length+' 条运行日志到 '+saved
 }catch(e){processError.value='导出运行日志失败：'+String(e)}
}
function stateName(s:ProcessStatus['state']){return ({stopped:'已停止',starting:'启动中',running:'运行中',stopping:'停止中',failed:'异常退出'} as Record<ProcessStatus['state'],string>)[s]}
function healthName(health?:ProcessStatus['health']){return health?({checking:'健康探测中',healthy:'健康正常',unhealthy:'健康异常'} as Record<NonNullable<ProcessStatus['health']>,string>)[health]:''}
function portStateName(item:ProcessPortStatus){if(item.state==='free')return '空闲';if(item.state==='owned')return '本服务监听';if(item.state==='occupied')return '被 PID '+item.pid+(item.process?'（'+item.process+'）':'')+'占用';return '无法判断'}
function exitReasonName(reason?:ProcessStatus['exitReason']){
 if(!reason)return ''
 return ({normal:'正常退出',nonzero:'非零退出',signal:'信号终止','manual-stop':'手动停止','start-error':'启动失败'} as Record<NonNullable<ProcessStatus['exitReason']>,string>)[reason]
}
function processStatusSummary(status:ProcessStatus){
 const label=exitReasonName(status.exitReason)
 if(!label)return ''
 if(status.state==='failed')return label+'：'+(status.error||'未提供错误信息')
 return '最近一次：'+label
}
function formatLocalTime(value?:string){if(!value)return '—';const date=new Date(value);return Number.isNaN(date.getTime())?'—':date.toLocaleString()}
function formatRunDuration(status:ProcessStatus){
 const start=status.startedAt?Date.parse(status.startedAt):NaN
 const active=status.state==='running'||status.state==='starting'||status.state==='stopping'
 const elapsed=active&&Number.isFinite(start)?Math.max(0,Date.now()-start):(status.durationMs??0)
 const totalSeconds=Math.floor(elapsed/1000),hours=Math.floor(totalSeconds/3600),minutes=Math.floor((totalSeconds%3600)/60),seconds=totalSeconds%60
 if(hours>0)return hours+'小时 '+String(minutes).padStart(2,'0')+'分 '+String(seconds).padStart(2,'0')+'秒'
 if(minutes>0)return minutes+'分 '+String(seconds).padStart(2,'0')+'秒'
 return totalSeconds+'秒'
}
function formatBytes(bytes?:number){
 if(!bytes||bytes<=0)return '—'
 const units=['B','KB','MB','GB','TB']
 let value=bytes,index=0
 while(value>=1024&&index<units.length-1){value/=1024;index++}
 return (value>=100||index===0?Math.round(value).toString():value.toFixed(1))+' '+units[index]
}

let processTimer:number|undefined, historyTimer:number|undefined
onMounted(()=>{
 void loadProcesses()
 processTimer=window.setInterval(()=>{if(props.active)void refreshProcesses()},1000)
 historyTimer=window.setInterval(()=>{if(props.active&&selectedPID.value)void loadResourceHistory()},3000)
})
onBeforeUnmount(()=>{if(processTimer!==undefined)clearInterval(processTimer);if(historyTimer!==undefined)clearInterval(historyTimer)})
</script>

<template>
   <section v-show="props.active" class="page-content process-page">
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
     <section class="process-panel"><div class="process-panel-heading"><div><h2>服务列表</h2><p>{{processes.length}} 个已保存的服务配置</p></div><button class="icon-button" @click="refreshProcesses"><RefreshCw :size="15"/></button></div><div v-if="processes.length" class="process-list"><button v-for="p in processes" :key="p.spec.id" class="process-item" :class="{selected:selectedPID===p.spec.id}" @click="selectProcess(p)"><span class="process-state-dot" :class="p.status.state"></span><span class="process-item-main"><strong>{{p.spec.name}}</strong><small>{{p.spec.executable}}</small><small v-if="p.status.exitReason" class="process-item-diagnostic" :class="{failed:p.status.state==='failed'}">{{processStatusSummary(p.status)}}</small><em>{{stateName(p.status.state)}}<template v-if="p.status.pid"> · PID {{p.status.pid}}</template><template v-if="p.status.health"> · {{healthName(p.status.health)}}</template><template v-if="p.status.memoryBytes"> · 内存 {{formatBytes(p.status.memoryBytes)}}</template><template v-if="p.status.cpuPercent"> · CPU {{p.status.cpuPercent.toFixed(1)}}%</template><template v-if="p.status.restarts"> · 已自动重启 {{p.status.restarts}} 次</template></em></span><ChevronRight :size="14"/></button></div><div v-else class="process-empty"><TerminalSquare :size="25"/><strong>还没有服务配置</strong><span>创建一条命令配置，即可开始管理本地服务。</span><button class="button button-secondary" @click="newProcess"><Plus :size="14"/>创建第一个服务</button></div></section>
     <section class="process-panel"><div class="process-panel-heading"><div><h2>{{editingID?'服务配置':'新增服务配置'}}</h2><p>参数使用 JSON 数组，不会拼接执行为 Shell 命令。</p></div><span class="safe-pill"><ShieldCheck :size="13"/>显式参数</span></div>
      <form class="process-form" @submit.prevent="saveProcess"><label class="field-label">服务名称</label><input v-model="procName" class="text-field" placeholder="例如：后端 API" required/><label class="field-label">可执行文件</label><input v-model="procExecutable" class="text-field mono-field" placeholder="例如：go、npm 或程序绝对路径" required/><label class="field-label">命令参数（JSON 字符串数组）</label><textarea v-model="procArgs" class="text-area mono-field" rows="3" spellcheck="false"></textarea><label class="field-label">工作目录</label><input v-model="procCwd" class="text-field mono-field" placeholder="可选，项目目录"/><label class="field-label">监听端口（可选，逗号分隔）</label><input v-model="procPorts" class="text-field mono-field" placeholder="例如：8080, 5432" aria-label="监听端口"/><p class="policy-hint">仅用于启动时的占用提示与详情面板诊断，不会修改启动参数；最多声明 20 个。</p><div class="env-editor-heading"><label class="field-label">环境变量（JSON 字符串对象）</label><button type="button" class="button button-quiet env-toggle" @click="showProcEnvEditor=!showProcEnvEditor"><EyeOff v-if="showProcEnvEditor" :size="13"/><Eye v-else :size="13"/>{{showProcEnvEditor?'隐藏编辑内容':'编辑环境变量'}}</button></div><textarea v-if="showProcEnvEditor" v-model="procEnv" class="text-area mono-field" rows="4" spellcheck="false"></textarea><p v-if="showProcEnvEditor" class="env-editor-help">编辑模式会显示真实环境变量值。保存后退出编辑模式；导出时建议使用脱敏选项。</p><pre v-else class="env-preview mono-field">{{maskedProcEnvPreview}}</pre>
       <label class="field-label">启动前置依赖</label>
       <div class="dependency-list">
        <label v-for="candidate in processes.filter(item=>item.spec.id!==editingID)" :key="candidate.spec.id" class="dependency-item">
         <input v-model="procDependsOn" type="checkbox" :value="candidate.spec.id" :disabled="processBusy"/>
         <span><strong>{{candidate.spec.name}}</strong><small>{{candidate.spec.id}}</small></span>
        </label>
        <span v-if="!processes.some(item=>item.spec.id!==editingID)" class="dependency-empty">先保存其他服务，再选择它们作为启动依赖。</span>
       </div>
       <label class="field-label">自动重启</label>
       <div class="process-policy-row">
        <label class="option-select">策略 <select v-model="procRestartMode" :disabled="processBusy" aria-label="自动重启"><option value="none">关闭</option><option value="on-failure">异常退出时重启</option></select></label>
        <label v-if="procRestartMode==='on-failure'" class="option-select">最多 <input v-model.number="procRestartAttempts" class="text-field mono-field policy-number" type="number" min="1" max="20" :disabled="processBusy" aria-label="自动重启次数"/> 次</label>
       </div>
       <p class="policy-hint">异常退出按 1 秒起步指数退避重启，最长 30 秒；正常退出、手动停止和配置修改都会取消待执行的重启。</p>
       <label class="field-label">健康检查</label>
       <div class="process-policy-row">
        <label class="option-select">类型 <select v-model="procHealthType" :disabled="processBusy" aria-label="健康检查类型"><option value="none">关闭</option><option value="tcp">TCP 端口</option><option value="http">HTTP 地址</option></select></label>
        <input v-if="procHealthType!=='none'" v-model="procHealthTarget" class="text-field mono-field policy-target" :placeholder="procHealthType==='tcp'?'127.0.0.1:8080':'http://127.0.0.1:8080/health'" :disabled="processBusy" aria-label="健康检查目标"/>
       </div>
       <div v-if="procHealthType!=='none'" class="process-policy-row">
        <label class="option-select">间隔 <input v-model.number="procHealthInterval" class="text-field mono-field policy-number" type="number" min="1" max="300" :disabled="processBusy" aria-label="健康检查间隔"/> 秒</label>
        <label class="option-select">超时 <input v-model.number="procHealthTimeout" class="text-field mono-field policy-number" type="number" min="1" max="30" :disabled="processBusy" aria-label="健康检查超时"/> 秒</label>
        <label class="option-select">失败阈值 <input v-model.number="procHealthThreshold" class="text-field mono-field policy-number" type="number" min="1" max="10" :disabled="processBusy" aria-label="健康检查失败阈值"/> 次</label>
       </div>
       <p v-if="procHealthType!=='none'" class="policy-hint">探测失败只标记状态并写入运行输出，不会自动重启服务；从启动后第一次探测结果开始展示。</p>
       <div v-if="procHealthType!=='none'" class="process-policy-row">
        <label class="option-select">上游就绪等待 <input v-model.number="procReadyTimeout" class="text-field mono-field policy-number" type="number" min="0" max="600" :disabled="processBusy" aria-label="依赖就绪等待"/> 秒</label>
       </div>
       <p v-if="procHealthType!=='none'" class="policy-hint">作为其他服务的依赖启动时，最多等待这么久通过健康检查再启动下游；0 表示不等待。建议等待时间大于探测间隔。</p>
       <div class="process-form-actions"><button type="submit" class="button button-primary" :disabled="processBusy"><Save :size="15"/>保存配置</button><button type="button" class="button button-secondary" @click="newProcess">清空表单</button><button v-if="editingID" type="button" class="button button-danger" @click="deleteProcess"><Trash2 :size="14"/>删除配置</button></div></form>
      <div v-if="selectedProcess" class="process-runtime"><div class="runtime-heading"><div><h3>进程控制</h3><p>{{stateName(selectedProcess.status.state)}}<template v-if="selectedProcess.status.pid"> · PID {{selectedProcess.status.pid}}</template><template v-if="selectedProcess.status.exitCode!==undefined"> · 退出码 {{selectedProcess.status.exitCode}}</template><template v-if="selectedProcess.status.health"> · {{healthName(selectedProcess.status.health)}}</template></p><div v-if="selectedProcess.status.startedAt || selectedProcess.status.finishedAt" class="process-lifecycle-meta"><span v-if="selectedProcess.status.startedAt">开始：<code>{{formatLocalTime(selectedProcess.status.startedAt)}}</code></span><span v-if="selectedProcess.status.finishedAt">结束：<code>{{formatLocalTime(selectedProcess.status.finishedAt)}}</code></span><span v-if="selectedProcess.status.startedAt">运行时长：<strong>{{formatRunDuration(selectedProcess.status)}}</strong></span><span v-if="selectedProcess.status.memoryBytes">内存：<strong>{{formatBytes(selectedProcess.status.memoryBytes)}}</strong><template v-if="selectedProcess.status.cpuPercent"> · CPU：<strong>{{selectedProcess.status.cpuPercent.toFixed(1)}}%</strong></template></span><span v-if="selectedProcess.status.restarts">自动重启：<strong>{{selectedProcess.status.restarts}}</strong> 次</span><span v-if="selectedProcess.status.nextRestartAt">下次重启：<code>{{formatLocalTime(selectedProcess.status.nextRestartAt)}}</code></span></div><div v-if="selectedProcess.status.healthMessage" class="process-lifecycle-meta"><span>健康诊断：<code>{{selectedProcess.status.healthMessage}}</code></span></div></div><div class="runtime-actions"><button class="button button-primary" :disabled="processBusy||selectedProcess.status.state==='running'||selectedProcess.status.state==='starting'" @click="processAction('start')"><Play :size="13"/>启动</button><button class="button button-secondary" :disabled="processBusy||(selectedProcess.status.state!=='running'&&!selectedProcess.status.nextRestartAt)" @click="processAction('stop')"><Square :size="13"/>{{selectedProcess.status.nextRestartAt?'取消重启':'停止'}}</button><button class="button button-secondary" :disabled="processBusy||selectedProcess.status.state==='running'||selectedProcess.status.state==='starting'" @click="processAction('restart')"><RefreshCw :size="13"/>重启</button></div></div><p v-if="selectedProcess.status.error" class="process-error-text">{{selectedProcess.status.error}}</p><div v-if="historySummary.count" class="resource-history"><div class="resource-history-head"><span><LineChart :size="14"/>最近 5 分钟资源</span><small>{{historySummary.count}} 个采样点<template v-if="selectedProcess.status.metricsAt"> · 最后采样 {{formatLocalTime(selectedProcess.status.metricsAt)}}</template></small></div><div class="resource-charts"><figure class="resource-chart"><figcaption>CPU 峰值 <strong>{{historySummary.peakCPU.toFixed(1)}}%</strong></figcaption><svg viewBox="0 0 120 32" preserveAspectRatio="none" role="img" aria-label="CPU 资源曲线"><polyline class="sparkline cpu" :points="cpuPoints"/></svg></figure><figure class="resource-chart"><figcaption>内存峰值 <strong>{{formatBytes(historySummary.peakMemory)}}</strong><em v-if="memoryDeltaLabel">{{memoryDeltaLabel}}</em></figcaption><svg viewBox="0 0 120 32" preserveAspectRatio="none" role="img" aria-label="内存资源曲线"><polyline class="sparkline memory" :points="memoryPoints"/></svg></figure></div><p class="resource-history-hint">采样间隔 2 秒，最多保留 150 个点；进程退出后仍可查看本次运行的最后一段曲线。</p></div><div class="port-diagnostics"><div class="port-head"><span><Network :size="14"/>端口诊断</span><small v-if="portReport?.scannedAt">扫描于 {{formatLocalTime(portReport.scannedAt)}}</small><button class="button button-quiet" type="button" :disabled="loadingPorts" @click="loadPortDiagnostics"><RefreshCw :size="12"/>{{loadingPorts?'扫描中':'刷新'}}</button></div><p v-if="portReport?.scanError" class="port-hint">无法读取系统端口信息：{{portReport.scanError}}</p><template v-else><div v-if="portReport?.ports.length" class="port-group"><h4>配置端口</h4><div class="port-rows"><span v-for="item in portReport.ports" :key="'d-'+item.port" class="port-row" :class="item.state"><code>{{item.port}}</code>{{portStateName(item)}}</span></div></div><div v-if="portReport?.listening.length" class="port-group"><h4>本服务监听</h4><div class="port-rows"><span v-for="item in portReport.listening" :key="'l-'+item.port" class="port-row owned"><code>{{item.port}}</code>监听中</span></div></div><p v-if="!portReport?.ports.length&&!portReport?.listening.length" class="port-hint">未声明端口，本服务当前也没有监听端口。可在配置中填写「监听端口」后查看占用情况。</p></template><p class="port-hint">声明端口只用于提示，不会改变启动命令；占用信息来自系统 netstat/lsof/ss，点击刷新可重新扫描。</p></div><div class="log-stream-heading"><span><Activity :size="14"/>运行输出</span><div class="log-stream-actions"><span>{{processLogs.length}} 条缓存记录</span><button class="button button-quiet" type="button" :disabled="!processLogs.length" title="导出当前运行日志（CSV）" @click="exportProcessLogs('csv')"><FileDown :size="13"/>CSV</button><button class="button button-quiet" type="button" :disabled="!processLogs.length" title="导出当前运行日志（JSON）" @click="exportProcessLogs('json')"><FileDown :size="13"/>JSON</button></div></div><div class="process-log-output"><div v-for="line in processLogs" :key="line.sequence" class="process-log-line" :class="line.stream"><span>{{line.time}}</span><b>{{line.stream==='stdout'?'OUT':line.stream==='stderr'?'ERR':'SYS'}}</b><code>{{line.text}}</code></div><div v-if="!processLogs.length" class="process-log-empty">启动服务后，标准输出和错误输出会显示在这里。</div></div></div>
     </section>
    </div>
    <div class="bottom-note"><ShieldCheck :size="15"/><span>配置保存在本机用户目录；全部启动会按依赖排序，停止某项服务会先停止依赖它的下游服务。</span></div>
   </section>
</template>
