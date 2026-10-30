export {}

declare global {
  interface LogLineContext { lineNumber:number; content:string; contentClipped:boolean }
  interface LogEventContext { startLine:number; endLine:number; content:string; contentClipped:boolean; kind?:'event'|'document' }
  interface LogMatch extends LogLineContext { contextBefore:LogLineContext[]; contextAfter:LogLineContext[]; event?:LogEventContext }
  interface LogSearchResponse { filePath:string; encoding:string; fileSizeBytes:number; bytesScanned:number; scannedLines:number; matches:LogMatch[]; resultLimitReached:boolean; canceled:boolean }
  interface LogSearchProgress { searchId:string; bytesScanned:number; fileSizeBytes:number; scannedLines:number; matchesFound:number; percent:number; running:boolean; canceled:boolean; error?:string }
  interface SearchMatchesEvent { searchId:string; progress:LogSearchProgress; matches?:LogMatch[] }
  interface SearchRecord { query:string; path:string; useRegex:boolean; contextLines:number; encoding?:string; createdAt:string }
  interface AppSettings { recentFiles:string[]; searchHistory:SearchRecord[]; contextLines:number; resultLimit:number }
  interface ProcessSpec { id:string; name:string; executable:string; args:string[]; workingDir:string; env:Record<string,string>; autoStart:boolean; dependsOn:string[] }
  interface ProcessBatchResult { id:string; name:string; action:'start'|'stop'; success:boolean; message:string }
  interface ProcessStatus { id:string; name:string; state:'stopped'|'starting'|'running'|'stopping'|'failed'; pid?:number; startedAt?:string; finishedAt?:string; durationMs?:number; exitCode?:number; exitReason?:'normal'|'nonzero'|'signal'|'manual-stop'|'start-error'; error?:string }
  interface ProcessView { spec:ProcessSpec; status:ProcessStatus }
  interface ProcessOutput { sequence:number; time:string; stream:'stdout'|'stderr'|'system'; text:string }
  interface JsonField { path:string; value:unknown }
  interface HttpRequest { method:string; url:string; headers:Record<string,string>; body:string; timeoutMs?:number; followRedirects?:boolean; insecureSkipVerify?:boolean }
  interface HttpResponse { status:number; statusText:string; headers:Record<string,string>; body:string; bodyTruncated:boolean; sizeBytes:number; durationMs:number; finalUrl?:string; redirects?:number; binary?:boolean }
  interface Window { runtime?:{EventsOn:(eventName:string,callback:(event:SearchMatchesEvent)=>void)=>()=>void}; go?:{main?:{App?:{
    SelectLogFile:()=>Promise<string>; PollDroppedFiles:()=>Promise<string[]>;
    SearchLogsAdvanced:(id:string,path:string,query:string,useRegex:boolean,maxResults:number,contextLines:number,encoding:string,multiLineJSON:boolean,eventContext:boolean,eventBoundary:string,eventBoundaryPattern:string)=>Promise<LogSearchResponse>;
    GetLogSearchProgress:(id:string)=>Promise<LogSearchProgress>; GetLogSearchMatches:(id:string,afterIndex:number)=>Promise<LogMatch[]>; CancelLogSearch:(id:string)=>Promise<boolean>;
    GetAppSettings:()=>Promise<AppSettings>; SaveAppSettings:(settings:AppSettings)=>Promise<void>;
    GetProcesses:()=>Promise<ProcessView[]>; SaveProcess:(spec:ProcessSpec)=>Promise<ProcessView>; DeleteProcess:(id:string)=>Promise<void>;
    StartProcess:(id:string)=>Promise<void>; StopProcess:(id:string)=>Promise<void>; RestartProcess:(id:string)=>Promise<void>;
    StartProcesses:(ids:string[])=>Promise<ProcessBatchResult[]>; StopProcesses:(ids:string[])=>Promise<ProcessBatchResult[]>;
    GetProcessLogs:(id:string,after:number)=>Promise<ProcessOutput[]>;
    ExportProcessConfigs:(redactSensitive:boolean)=>Promise<string>; ImportProcessConfigs:()=>Promise<ProcessView[]>;
    SendHTTPRequest:(request:HttpRequest)=>Promise<HttpResponse>;
  }}}}
}
