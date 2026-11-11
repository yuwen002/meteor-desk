# MeteorDesk 接口与代码 API

## 1. 接口形态

MeteorDesk 是 Wails v2 桌面应用，不提供供外部客户端调用的 REST/HTTP API。前端通过 Wails 生成的 Go 方法桥接调用后端 `App` 方法，典型调用路径为 `window.go.main.App`。

下表中的方法是前端绑定接口。Go 层签名以 `app.go` 为准；Go 的 `error` 在 JavaScript 桥接中表现为 Promise rejection。

## 2. 前端桥接方法

### 日志搜索

| 方法 | 参数 | 返回 | 说明 |
|---|---|---|---|
| `SelectLogFile()` | 无 | `string` | 打开系统文件选择器；取消选择时路径为空字符串。 |
| `SelectLogDirectory()` | 无 | `string` | 打开系统目录选择器，为多文件检索选择根目录；取消选择时路径为空字符串。 |
| `PollDroppedFiles()` | 无 | `string[]` | 取出并清空拖放文件队列。 |
| `SearchLogs(searchID, path, query, useRegex, maxResults)` | 任务 ID、路径、搜索内容、是否正则、结果上限 | `SearchResponse` | 旧版兼容入口，不附加上下文行。 |
| `SearchLogsAdvanced(searchID, path, query, useRegex, maxResults, options)` | 任务 ID、路径、查询、正则开关、结果上限、`SearchOptions` 结构体 | `SearchResponse` | 执行搜索。`options` 承载 `contextLines`、`encoding`（`auto`/`utf-8`/`gb18030`/`utf-16le`/`utf-16be`）、`multiLineJSON`、`eventContext`/`eventBoundary`/`eventBoundaryPattern`，以及时间窗（`timeWindowEnabled`/`timeWindowStartMs`/`timeWindowEndMs`/`timeWindowZone`：`local` 或 `utc`）和续扫起点（`startOffset`/`startLine`，-1 表示从头扫描）。`multiLineJSON=true` 时把跨行 JSON 文档作为整体匹配单元；`eventContext=true` 时按边界聚合事件。时间窗在后端扫描时过滤，行首无时间戳的命中会计入 `timeWindowSkipped`；因结果上限截断时返回续扫点 `resumeOffset`/`resumeLine`。任务需有唯一 ID，响应完成时返回权威完整结果。 |
| `SearchLogsMultiFile(searchID, rootDir, pattern, query, useRegex, maxResults, options)` | 任务 ID、根目录、通配串、查询、正则开关、结果上限、`SearchOptions` 结构体 | `MultiSearchResponse` | 目录/通配多文件检索（阶段 6 批次 E）。递归遍历 `rootDir` 收集文件名匹配 `pattern`（分号或逗号分隔的 glob，留空用默认扩展名 `.log/.txt/.out/.json/.ndjson`）的普通文件，最多 200 个；以有界并发（`min(NumCPU, 8, 文件数)`）复用单文件扫描内核，`maxResults` 作为所有文件合计的**全局命中上限**。结果按文件路径升序、文件内按行号顺序拼接，每条命中用 `filePath` 标注来源；按文件粒度上报进度、取消时保留已完成文件的结果、失败文件单独记录原因。与单文件共用同一任务登记、`log-search:update` 事件与进度/增量轮询机制。 |
| `GetLogSearchProgress(id)` | 任务 ID | `LogSearchProgress` | 读取字节数、扫描行数、匹配数、进度百分比及运行/取消状态。 |
| `GetLogSearchMatches(id, afterIndex)` | 任务 ID、已收到的匹配数量 | `LogMatch[]` | 只返回游标之后新发现的匹配，供前端增量展示；负游标按 0 处理，超出当前结果长度时返回空数组。取消任务会在保留期内保留已发现结果。 |
| `CancelLogSearch(id)` | 任务 ID | `boolean` | 请求取消仍在运行的搜索；返回 false 表示任务不存在或已结束。 |
| `ReadLogTail(path, offset, encoding)` | 路径、上次偏移、编码模式 | `TailResult` | 增量读取日志新增行；`offset` 为负时从文件末尾回退（默认 256 KiB）开始跟踪。只返回到最后一个换行符为止的完整行，支持 `auto`、`utf-8`、`gb18030`，UTF-16 直接返回错误。 |
| `ReadLogFileHead(path, encoding, maxBytes)` | 路径、编码模式、最大字节数 | `DecodedFileHead` | 读取日志文件开头并解码为 UTF-8，供「两份日志逐行对比」面板快速预览文件头部内容。`encoding` 同检索接口（`auto` / `utf-8` / `gb18030` / `utf-16le` / `utf-16be`）；单侧上限 4 MiB，超过即截断并把 `truncated` 标记为 `true`。自动识别复用 `file-indexes.json` 编码缓存。 |
| `ScanLogTimeline(files, options)` | `TimelineFileSpec[]`、`TimelineScanOptions` | `TimelineScanResponse` | 并发扫描多份日志，按每行可解析的日期时间戳合并成统一时间线视图（阶段 9）。每个文件返回一个 `entries` 数组（时间戳升序），全局 `entries` 按时间戳合并去重；单文件最多 5 万行、整体硬上限 20 万行，超出时 `truncated` 置 `true` 并在 `truncatedFiles` 列出文件名。`TimelineScanOptions` 含 `levelDetect: true` 时自动识别每行级别（`error`/`warn`/`info`/`debug`/`trace`），级别词匹配逻辑同聚合工具函数。进度通过 `log-timeline:update` 事件推送（事件含 `files`, `progress`, `entries` 增量），也可按 `scanID` 轮询 `GetLogSearchProgress` 复用同一任务登记通道。 |
| `GetLogCacheEntries(limit)` | 数量上限（默认 20，上限 50） | `CacheEntryMeta[]` | 返回最近访问的搜索缓存条目元数据，按访问时间降序（阶段 8）。每条含 `key`（路径+查询+正则+编码+上下文行数+时间窗的 SHA-256 前 16 位）、`filePath`、`query`、`useRegex`、`encoding`、`hitCount`、`bytesScanned`、`resultCount`、`createdAt`、`accessedAt`、`entrySizeBytes`。 |
| `GetLogCacheStatistics()` | 无 | `CacheStatistics` | 返回当前缓存统计快照：`entries` 条目数、`totalBytes` 总大小、`cacheDir` 落盘目录、`maxEntries`（默认 128）、`maxBytes`（默认 500 MiB）、`hitCount` 累计命中次数、`missCount` 累计未命中次数、`evictedCount` LRU 淘汰累计。 |
| `DeleteLogCacheEntry(key)` | 缓存键 | 无 | 按缓存键删除单条搜索结果并同步更新索引。键不存在时返回 `error`。 |
| `ClearLogCache()` | 无 | `CacheStatistics` | 清空全部搜索缓存条目并返回清空前的统计快照。 |

前端建议先生成唯一的 `searchID`，在调用 `SearchLogsAdvanced` 的同时监听 Wails `log-search:update` 事件以低延迟接收进度和新增匹配；轮询进度/增量接口仍作为兼容回退。事件包含 `searchId`、`progress` 和可选的 `matches`，前端必须按任务 ID 过滤并与轮询共用游标，防止重复追加。最终 Promise 返回后，以最终响应替换临时快照。取消是协作式操作：底层扫描会在检查点停止，并返回已累计结果。

多文件检索（`SearchLogsMultiFile`）与单文件共用上述任务登记、事件与轮询链路，前端只需按「多文件开关」分流到不同桥接方法即可复用同一套结果渲染与增量拼接；`MultiSearchResponse` 的字段命名与 `SearchResponse` 对齐，额外携带 `files` 文件级明细供「来源文件总览」与按文件分组过滤使用。

### 本地服务

| 方法 | 参数 | 返回 | 说明 |
|---|---|---|---|
| `GetProcesses()` | 无 | `ProcessView[]` | 返回服务定义和运行状态。 |
| `SaveProcess(spec)` | `ProcessSpec` | `ProcessView` | 新增或更新配置；不启动服务。 |
| `GetProcessCommandPreview(spec)` | `ProcessSpec` | `CommandPreview` | 把尚未保存或正在编辑的 `ProcessSpec` 渲染成用户可核对的命令行预览（阶段 9）。返回 `command`（完整 shell 命令字符串，含环境变量前缀和可执行文件+参数）、`workingDir`、`env`、`warnings`；敏感环境变量值会自动清空为 `***` 防止泄露到日志。可执行文件不是绝对路径或 `%PATH%` 找不到时在 `warnings` 中给出提示，不阻止预览。 |
| `DeleteProcess(id)` | 服务 ID | 无 | 仅允许删除已停止且没有被其他服务依赖的服务。 |
| `StartProcess(id)` | 服务 ID | 无 | 启动服务和所需上游依赖。 |
| `StopProcess(id)` | 服务 ID | 无 | 先停依赖它的下游服务，再停止目标。 |
| `RestartProcess(id)` | 服务 ID | 无 | 先停再启动。 |
| `StartProcesses(ids)` | 服务 ID 数组 | `ProcessBatchResult[]` | 按依赖拓扑顺序逐项启动。 |
| `StopProcesses(ids)` | 服务 ID 数组 | `ProcessBatchResult[]` | 按反向依赖顺序逐项停止。 |
| `GetProcessLogs(id, after)` | 服务 ID、上次输出序号 | `ProcessOutput[]` | 仅返回序号大于 `after` 的输出行。 |
| `GetResourceHistory(id)` | 服务 ID | `ResourceSample[]` | 返回最近 5 分钟资源采样点（最多 150 个，按时间升序）；停止后仍保留本次运行的最后一段。 |
| `GetPortDiagnostics(id)` | 服务 ID | `PortReport` | 扫描本机监听端口，返回配置端口的占用情况与本服务实际监听的端口；扫描失败时 `scanError` 说明原因。当占用端口的进程是本工作台托管的另一个服务时，该端口项带 `ownerSpecId`/`ownerName`，前端可据此提供停止入口；外部进程不填充这两个字段。 |
| `GetProcessTimeline(id)` | 服务 ID | `ProcessEvent[]` | 返回最近的生命周期事件（启动、就绪、自动重启、退出），按发生顺序排列；每个服务保留最近 200 条并持久化到 `process-events.json`，应用重启后仍可回看。`ready` 的 `durationMs` 为启动耗时，`exit` 的为本次运行时长。 |
| `GetProcessRunHistory(id)` | 服务 ID | `RunRecord[]` | 返回服务按时间升序的运行历史，每次运行一行（起止时间、退出原因与退出码、自动重启次数、峰值 CPU 与内存）；本机保留最近 100 次并持久化到 `run-history.json`。 |
| `ListProcessGroups()` | 无 | `Group[]` | 返回按名称排序的服务分组（场景）副本，分组只按 ID 引用已有服务。 |
| `SaveProcessGroup(group)` | `Group` | `Group` | 新建或更新分组；新建时分配 `id`，成员必须都是已配置的服务，返回规范化后的分组。 |
| `DeleteProcessGroup(id)` | 分组 ID | 无 | 只删除分组引用关系，不影响成员服务本身。 |
| `StartProcessGroup(id)` | 分组 ID | `ProcessBatchResult[]` | 按依赖拓扑顺序整组启动分组内服务及其尚未运行的上游依赖，逐项返回结果；已删除的成员被跳过。 |
| `StopProcessGroup(id)` | 分组 ID | `ProcessBatchResult[]` | 按反向依赖顺序整组停止分组内服务及依赖它们的成员。 |
| `FindServiceByLogPath(path)` | 落盘日志文件绝对路径 | `ProcessStatus` | 按落盘文件路径反查所属服务，供日志分析跳回进程详情；未匹配时返回字段为空的服务快照（`id` 为空字符串），前端据此判断是否命中。 |
| `ExportProcessConfigs(redactSensitive)` | 是否脱敏 | `string` | 通过系统对话框写入 JSON 文件并返回提示文本。 |
| `ImportProcessConfigs()` | 无 | `ProcessView[]` | 通过系统文件选择器导入 JSON 配置。 |

### 开发工具箱

| 方法 | 参数 | 返回 | 说明 |
|---|---|---|---|
| `SendHTTPRequest(request)` | `HttpRequest` | `HttpResponse` | 由用户显式触发的本地 HTTP 请求。支持 GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS，URL 必须为 http/https；请求与响应正文各限制 2 MiB，超时默认 30 秒、可配置上限 120 秒，默认最多跟随 10 次重定向。非 2xx 状态不作为错误，响应原样返回。 |

### 设置和本地数据

| 方法 | 参数 | 返回 | 说明 |
|---|---|---|---|
| `GetAppSettings()` | 无 | `AppSettings` | 获取本地偏好和历史。 |
| `SaveAppSettings(settings)` | `AppSettings` | 无 | 规范化、保存并应用设置。 |
| `UserDataDirectory()` | 无 | `string` | 返回应用配置目录路径。 |
| `GetRuntimeInfo()` | 无 | `RuntimeInfo` | 返回应用版本、Go 运行时版本、运行平台与配置目录，用于「关于」面板与诊断信息。 |
| `SaveExportFile(suggestedName, format, content)` | `string`、`string`、`string` | `string` | 打开系统保存对话框，把前端生成的导出文本写入用户选定路径。`format` 支持 `csv` 和 `json`；空内容、不支持的格式或超过 32 MiB 直接报错；默认文件名只保留基名并清理非法字符。返回保存路径，用户取消时返回空字符串。 |

## 3. 常用数据结构

### SearchResponse

```json
{
  "filePath": "/path/to/service.log",
  "encoding": "UTF-8",
  "fileSizeBytes": 1048576,
  "bytesScanned": 1048576,
  "scannedLines": 12000,
  "matches": [],
  "resultLimitReached": false,
  "canceled": false,
  "fromCache": false,
  "timeWindowSkipped": 0,
  "resumeOffset": -1,
  "resumeLine": -1
}
```

`fromCache` 为 `true` 表示本次结果命中了搜索缓存（见阶段 8「搜索结果缓存」），直接从落盘 JSON 加载，未重新扫描文件；`timeWindowSkipped` 仅在启用时间窗时填写，表示行首缺少可解析的完整日期时间戳而未计入结果的命中数；`resumeOffset`/`resumeLine` 在结果被硬上限截断时填写，前端用它们调用下一次 `SearchLogsAdvanced` 时分别作为 `startOffset`/`startLine` 传入续扫剩余内容，`-1` 表示当前不可续扫（已到文件末尾、被取消、或 UTF-16/GB18030 编码需要字符解码器时不支持）。

### LogSearchProgress

字段包括 `searchId`、`bytesScanned`、`fileSizeBytes`、`scannedLines`、`matchesFound`、`percent`、`running`、`canceled` 和可选的 `error`。多文件检索还携带可选的文件级字段：`filesTotal`、`filesDone`、`filesFailed` 与 `currentFile`（单文件检索时省略）。

### TailResult

字段包括 `lines`（新增行，按文件顺序）、`offset`（下一次读取的起点）、`fileSizeBytes`、`encoding`（实际编码）、`reset`（首次跟踪或文件被截断/替换，前端应清空已有行）和 `truncated`（达到单次上限、跳过超长行或回退点残行）。单次读取默认上限 256 KiB / 500 行，硬上限 2 MiB / 5,000 行；偏移只前进到最后一个完整换行之后，未写完的行留到下一次调用。

### DecodedFileHead

`ReadLogFileHead` 的返回值，供两份日志逐行对比面板预览文件头部：

```json
{
  "text": "2026-01-01 00:00:00 INFO Server starting...",
  "encoding": "UTF-8",
  "fileSizeBytes": 2048576,
  "headBytes": 65536,
  "truncated": false
}
```

`text` 是 UTF-8 解码后的文件头部文本；`headBytes` 是实际读取的原始字节数；`truncated` 为 `true` 时表示因超过 `maxBytes` 参数而截断。

### Timeline 系列结构

`ScanLogTimeline` 使用以下结构（阶段 9）：

```jsonc
// 输入：每个文件的扫描规格
{
  "path": "/path/to/app.log",
  "encoding": "auto"          // 可选，默认 auto
}
```

`TimelineScanOptions` 字段：`levelDetect`（是否自动识别每行级别，默认 `true`）、`timeZone`（时间戳解析时区，`local` 或 `utc`，默认 `local`）、`perFileMaxLines`（单文件上限，默认 50,000）、`globalMaxLines`（全局硬上限，默认 200,000）。

`TimelineScanResponse`：

```jsonc
{
  "fileEntries": [
    {
      "filePath": "/path/to/app.log",
      "entries": [
        {
          "timestamp": 1735689600000,   // 毫秒时间戳，无时间戳的行用 Long.MAX
          "originalTime": "2026-01-01 00:00:00",
          "lineNumber": 1,
          "level": "info",               // error / warn / info / debug / trace / unknown
          "content": "Server starting..."
        }
      ],
      "linesScanned": 12000,
      "truncated": false
    }
  ],
  "globalEntries": [ /* 按 timestamp 合并去重后的跨文件时间线 */ ],
  "truncated": false,
  "truncatedFiles": []
}
```

### 搜索缓存系列结构

阶段 8 引入的搜索结果落盘缓存，相关结构如下。

`CacheEntryMeta`（列表返回）：

```jsonc
{
  "key": "a1b2c3d4e5f6a7b8",             // 路径+查询+正则+编码+上下文+时间窗的 SHA-256 前 16 位
  "filePath": "/path/to/app.log",
  "query": "ERROR",
  "useRegex": false,
  "encoding": "UTF-8",
  "contextLines": 2,
  "hitCount": 15,                         // 累计命中次数
  "bytesScanned": 1048576,
  "resultCount": 42,
  "createdAt": "2026-01-01T10:00:00Z",
  "accessedAt": "2026-01-02T15:30:00Z",
  "entrySizeBytes": 128500
}
```

`CacheStatistics`（统计快照）：

```jsonc
{
  "entries": 42,
  "totalBytes": 536870912,
  "cacheDir": "C:/Users/.../search-cache",
  "maxEntries": 128,
  "maxBytes": 524288000,                 // 500 MiB
  "hitCount": 128,
  "missCount": 350,
  "evictedCount": 5
}
```

缓存键生成规则（Go 层 `buildCacheKey`）：`sha256(filePath + "|" + query + "|" + strconv.FormatBool(useRegex) + "|" + encoding + "|" + contextLines + "|" + timeWindowStartMs + "|" + timeWindowEndMs + "|" + timeZone)[:16]`。只有当文件的大小和修改时间与记录匹配时缓存才命中（`file-modernity` 校验），否则视为未命中，不触发回源。

### EventContext

命中详情中的可选字段，默认不返回。字段包括：`startLine`、`endLine`、`content`（完整事件或 JSON 文档，单条最多 512 KiB）、`contentClipped` 和 `kind`（`event` 表示按边界聚合的事件，`document` 表示多行 JSON 文档）。单次检索为所有上下文保留的内容总量上限为 32 MiB；预算耗尽时后续命中不返回内容，前端应回退到行预览。

### Match.filePath 与 MultiSearchResponse

每条命中（`Match`）都带可选的 `filePath`，仅多文件目录检索填写以标注来源文件，单文件检索省略；前端以「来源文件 + 行号」作为结果唯一标识，因此同名的行号在不同文件间不会串选。

`SearchLogsMultiFile` 返回 `MultiSearchResponse`，在 `SearchResponse` 的字段之外追加文件级明细：

```json
{
  "filePath": "/path/to/logs",
  "encoding": "多文件",
  "fileSizeBytes": 8388608,
  "bytesScanned": 8388608,
  "scannedLines": 96000,
  "matches": [{ "lineNumber": 2, "filePath": "/path/to/logs/app.log", "content": "..." }],
  "files": [{ "path": "/path/to/logs/app.log", "bytesScanned": 1048576, "fileSizeBytes": 1048576, "scannedLines": 12000, "matchesFound": 3, "done": true, "failed": false }],
  "filesTotal": 8, "filesSucceeded": 7, "filesFailed": 1, "filesSkipped": 0,
  "filesTruncated": false, "resultLimitReached": false, "canceled": false
}
```

`files[]` 按枚举（路径升序）顺序给出每个文件的处理状态：`done`/`failed` 标记成功或失败，失败时 `error` 记录原因；被跳过的文件（全局上限耗尽或取消后未领取）两者皆为 false。`filesSkipped` 为未扫描文件数，恒满足 `filesSucceeded + filesFailed + filesSkipped = filesTotal`；`filesTruncated` 表示目录内匹配文件超过 200 个、仅检索了排序后的前 200 个。

### ProcessSpec

配置字段包括 `id`、`name`、`executable`、`args`（字符串数组）、`workingDir`、`env`（字符串键值对象）、`autoStart`、`dependsOn`（服务 ID 数组）、可选的 `readyTimeoutSeconds`（0–600，0 表示不等待）、可选的 `ports`（监听端口，1–65535，最多 20 个，自动去重升序）、可选的 `logToDisk`（输出落盘开关），以及可选的 `restart` 和 `health`。程序通过可执行文件与参数数组启动，不默认拼接或解释 Shell 命令。`logToDisk` 为 `true` 时，本次运行的标准输出与标准错误按行追加写入配置目录 `logs/<服务名>.log`，单文件超过 64 MiB 后轮转并最多保留 3 份（详见「内部持久化文件与保留上限」）。

### RestartPolicy / HealthCheck

`restart` 字段为 `{"mode": "none"|"on-failure", "maxAttempts": 1..20}`；`mode` 为空时规范化为 `none`，`on-failure` 未填次数时默认 3。`health` 字段为 `null` 或 `{"type": "tcp"|"http", "target": "...", "intervalSeconds": 1..300, "timeoutSeconds": 1..30, "failureThreshold": 1..10}`，缺省值为间隔 10 秒、超时 2 秒（且不超过间隔）、阈值 3 次；`tcp` 目标须为 `host:port`，`http` 目标须带 `http://` 或 `https://` 协议与主机名。

`readyTimeoutSeconds` 仅在 `health` 非空且该服务在本次批量启动中作为上游依赖时生效：`StartBatch` 会等待其健康状态变为 `healthy` 再启动下游；超时、`unhealthy` 或进程提前退出会跳过下游，并在 `BatchResult` 与运行输出中给出原因。没有 `health` 时该字段被规范化为 0。

### ProcessStatus

运行快照字段包括 `state`、`pid`、`startedAt`、`finishedAt`、`durationMs`、`exitCode`、`exitReason` 和 `error`。退出原因枚举为 `normal`、`nonzero`、`signal`、`manual-stop`、`start-error`。启用健康检查时还包含 `health`（`checking` / `healthy` / `unhealthy`）、`healthMessage`（最近一次失败原因）和 `healthCheckedAt`（RFC3339Nano）；启用自动重启时包含 `restarts`（当前累计重启次数）和 `nextRestartAt`（等待重启的预计时间，无待执行重启时为空）。手动启动、手动停止或修改配置会把 `restarts` 归零。进程运行时还包含 `cpuPercent`（两次采样之间的单核百分比）、`memoryBytes`（常驻内存字节数）和 `metricsAt`（最近一次采样时间，RFC3339Nano）；进程退出后三个字段清空。开启落盘时还包含 `logFilePath`（本次运行输出文件的绝对路径）；服务停止后仍保留最后一次的落盘路径，供详情面板展示与日志分析跳转，未开启落盘时为空。

### ProcessOutput

输出行包括单调递增的服务内 `sequence`、本地时间、输出流类型（`stdout` / `stderr` / `system`）和文本内容。

### RunRecord

一次运行的汇总记录，在进程退出时写入，字段包括 `serviceId`、可选的 `serviceName`、`startedAt`、`finishedAt`、`durationMs`（本次运行时长）、`exitReason`（同 `ProcessStatus` 的退出原因枚举）、`exitCode`（为空表示未取到退出码，如启动前就失败）、`restarts`（本次运行周期内的自动重启次数），以及资源采样峰值 `peakCpuPercent` 与 `peakMemoryBytes`（未采样到时为零）。不含逐行输出与采样曲线。

### Group

服务分组（场景），只按 ID 引用已有服务，不复制服务配置。字段包括 `id`、`name`、可选的 `description` 和 `memberIds`（服务 ID 数组）。整组启停时按依赖顺序复用批量启停逻辑；已删除的成员在启停时被跳过，删除分组不影响服务本身。

### CommandPreview

`GetProcessCommandPreview` 的返回值，供前端在用户编辑 `ProcessSpec` 时实时展示将要执行的命令行（阶段 9）：

```json
{
  "command": "set MY_VAR=hello&& set DB_PASSWORD=***&& \"C:\\tools\\myapp.exe\" --config prod.yaml",
  "workingDir": "C:\\tools",
  "env": {
    "MY_VAR": "hello",
    "DB_PASSWORD": "***"
  },
  "warnings": [
    "可执行文件路径 C:\\tools\\myapp.exe 不是绝对路径，启动时会从 %PATH% 查找"
  ]
}
```

`command` 是按当前平台拼接的 shell 命令字符串（Windows 用 `set VAR=value&&` 前缀，POSIX 用 `VAR=value` 前缀）；`env` 中值为 `***` 的键表示被识别为敏感变量名（密码、token、key、secret 等子串大小写不敏感匹配），返回时已脱敏；`warnings` 收集非致命问题（可执行文件相对路径、workingDir 不存在等），不阻止预览。

### 内部持久化文件与保留上限

与 `processes.json` 同目录（应用配置目录）维护以下本机文件，写入都通过临时文件加重命名的原子方式（`writeJSONAtomic`）串行完成，避免并发追写出半行或乱序；删除服务时一并清理其时间线、运行历史与落盘文件：

- `process-events.json`：按服务保留最近 200 条生命周期事件，应用启动时载入，退出时同步落盘。
- `run-history.json`：按服务保留最近 100 次运行记录，进程退出时追加，启动时载入。
- `groups.json`：服务分组定义，与进程配置同目录。
- `logs/<服务名>.log`：开启 `logToDisk` 的服务输出，单文件上限 64 MiB，超出后按命名位移轮转（`name.log` → `name.log.1` → `name.log.2`），连同当前文件最多保留 3 份；服务名会被清理为安全文件名（去路径分隔符与非法字符，空名回退为 `service`）。
- `file-indexes.json`：日志编码检测缓存，按三元组键（路径+大小+修改时间）记录检测结果，最多 50 个文件、24 小时过期、LRU 淘汰；应用启动时载入（`WarmEncodingCache`）、退出时回写（`FlushEncodingIndex`），检索自动识别前命中缓存可跳过重复抽样。
- `search-cache/cache-index.json`：搜索结果缓存索引（阶段 8），每个条目含键、路径、查询、正则、编码、上下文行数、文件大小+修改时间、命中/访问时间与缓存文件路径；最多 128 条、总大小上限 500 MiB，LRU 淘汰。应用启动时载入（`WarmCacheIndex`）、退出时原子落盘。
- `search-cache/<key>.json`：单条缓存的完整 `SearchResponse` JSON（不含 `FromCache` 标志），由 `PutCachedSearch` 写入、`CheckCacheHit` 读回；键为 SHA-256 前 16 位，与索引一一对应。删除索引条目时同步删除对应文件；缓存面板清空/淘汰时一并清理。

### RuntimeInfo

字段包括 `appVersion`、`goVersion`、`platform`（`GOOS/GOARCH`）和 `dataDir`。版本号只在 `frontend/package.json` 维护：前端构建时直接读取该文件，发布构建通过 `-ldflags "-X main.appVersion=<version>"` 注入后端，本地开发未注入时为 `dev`。该结构不包含日志内容、环境变量或凭据。

### 导出格式

检索结果和进程输出由前端 `frontend/src/utils/exporters.ts` 生成文本，后端只负责写入文件。

- 检索结果 CSV：列为 `matchLine`、`type`、`lineNumber`、`endLine`、`query`、`content`；`type` 为 `match`、`context-before`、`context-after`、`event` 或 `document`，上下文行与事件内容逐条展开。文件带 UTF-8 BOM，使用 CRLF 换行并按 RFC 4180 转义。导出列表为前端当前过滤与排序后的结果，没有过滤时等同完整结果。
- 检索结果 JSON：包含 `exportedAt`、文件路径、检索词、是否正则、编码、上下文行数、`resultLimitReached`、`canceled`、`total` 和完整 `matches`。
- 进程日志 CSV：列为 `sequence`、`time`、`stream`、`text`；JSON 额外包含 `processId`、`processName` 和 `total`。导出范围仅限界面缓存的最近 1,500 条输出。

### 结果聚合统计

`frontend/src/utils/aggregate.ts` 提供纯前端聚合函数，不改动后端扫描逻辑，输入为当前过滤后的 `LogMatch[]`：

- `detectLevel(line)`：按行内第一个级别词返回 `error`、`warn`、`info`、`debug`、`trace` 或其他；`fatal` 与 `err` 归入 `error`，`warn`/`warning` 归入 `warn`，单词边界避免把 `information`、`debugging` 误判为级别。
- `normalizeSignature(line)`：去掉日期时间与纯时间戳、级别前缀、UUID 和数字（数字不带尾部边界，使 `3000ms` 这类耗时可归并），压缩空白并截断到 80 字符；结果为空时回退到原行文本。
- `summarizeMatches(matches, limit = 5)`：返回 `total`、`firstLine`、`lastLine`、按 `error → other` 排序的 `levels`，以及只统计 error/warn 行的高频签名 `signatures`（按次数降序、同频按文本排序）。
- `summaryText(summary)`：生成纯文本摘要（总计、行号范围、级别分布、签名排行），供「复制摘要」写入剪贴板。

错误时间分布由 `frontend/src/utils/timeDistribution.ts` 提供，同样只消费当前过滤后的结果：

- `extractLogTime(line)`：从行内解析 `YYYY-MM-DD HH:mm:ss`、ISO 时间戳（含毫秒）或 `HH:mm:ss`，按 UTC 返回毫秒时间戳；只有时间时锚定到 1970-01-01，无时间或时间值越界时返回 `undefined`。
- `buildTimeDistribution(matches, maxBuckets = 24)`：只统计 error/warn 行，按跨度自适应选择 1 秒至 1 天的步长，返回 `buckets`（`start`、`label`、`error`、`warn`）、`stepMs`、`stepLabel`、`maxCount`、`errorTotal`、`warnTotal` 与无时间戳的 `undated`；溢出上限的样本归入最后一桶。
- `filterMatchesByTime(matches, start, stepMs)`：保留时间戳落在 `[start, start + stepMs)` 内的行，用于点击时间格后的联动过滤；没有时间戳的行不会被选中，区间右边界开放，避免相邻两格重复计数。

聚合只依赖已返回的结果，不会重新读文件，也不影响导出内容。

### 服务依赖图

`frontend/src/utils/dependencyGraph.ts` 只做布局计算，不调用后端，也不改变启停顺序：

- `buildDependencyGraph(services)`：输入 `{ id, name, dependsOn }` 列表，返回 `nodes`（含 `layer`、`x`、`y`、`width`、`height`）、`edges`（含 `from`、`to`、SVG `path` 与 `cyclic`）、`layers`、画布 `width`/`height` 以及循环成员 `cyclic`。
- 分层取最长依赖链；未知依赖、重复依赖与自依赖会被忽略；循环边不参与分层，只标记 `cyclic`，布局仍保持有限。
- 节点渲染用的运行状态由 `ProcessManagement.vue` 从 `ProcessStatus` 中读取，不属于布局函数的输出。

### cURL 互转

`frontend/src/utils/curl.ts` 在表单与 cURL 命令之间转换，不发起请求，也不经过后端：

- `parseCurlCommand(input)`：返回 `{ ok: true, request, warnings }` 或 `{ ok: false, error }`。`request` 为 `{ method, url, headers, body }`；支持 `-X`/`--request`、`-H`/`--header`、`-d` 及 `--data-*`、`--url`、`-A`、`-b`、`-e`，兼容 `\` 与 `^` 续行、单双引号、`-XPOST` 与 `--data-raw=...` 紧凑写法。只带请求体未写 `-X` 时方法为 POST，否则为 GET；多段请求体以 `&` 连接。
- `buildCurlCommand(request)`：生成多行命令（`curl -X METHOD 'url' \` + 每行一个 `-H` + `--data-raw`），空名称或空值的请求头被跳过；取值统一用单引号包裹，内部单引号按 `'\''` 转义，可直接粘回 shell。
- 只影响传输方式的开关（`-k`、`-L`、`--compressed`、`-s`、`-i` 等）解析时直接忽略；未知选项、无效请求头（缺少冒号）与多余位置参数写入 `warnings`，不会中断导入。
- 未闭合引号、命令不以 `curl` 开头、缺少 URL 或选项缺值时返回 `error`，界面只展示提示且不修改当前表单。

### HttpRequest / HttpResponse

请求字段包括 `method`、`url`、`headers`（字符串键值对象）、`body`，以及可选的调优字段：`timeoutMs`（毫秒，0 或缺省时按 30 秒处理，超过 120 秒直接报错）、`followRedirects`（`false` 时不跟随重定向并直接返回 3xx；缺省保持默认跟随，最多 10 次）和 `insecureSkipVerify`（`true` 时跳过 TLS 证书校验，仅用于本机自签证书调试）。响应字段包括：`status`（状态码）、`statusText`（标准状态文本，如 `Created`）、`headers`（同名多值以 `, ` 合并）、`body`（最多保留 2 MiB，非法 UTF-8 字节替换为 `�`）、`bodyTruncated`、`sizeBytes`（实际接收字节数，可能大于保留的正文长度）、`durationMs`（毫秒）、`finalUrl`（跟随重定向后的最终地址）、`redirects`（实际跟随次数）和 `binary`（正文含 NUL 或非法 UTF-8）。

## 4. Go 包 API

### `internal/httpx`

- `Send(Request)`：执行一次受限的本机 HTTP 请求；校验方法、URL 协议、请求头数量、正文大小与超时上限，按需配置重定向跟随与 TLS 校验，返回状态、响应头、受限正文、最终地址、重定向次数、实际接收字节数、二进制标记与耗时。

### `internal/logx`

- `SearchFile(path, query, useRegex, maxResults)`：简单同步搜索入口。
- `SearchFileContext(ctx, path, query, useRegex, maxResults, onProgress)`：支持取消与进度/增量匹配回调。
- `SearchFileWithOptions(ctx, path, query, useRegex, maxResults, SearchOptions, onProgress)`：支持上下文行配置的核心流式搜索入口。
- `Progress.Matches`：每次进度通知中新增的匹配增量，不会重复包含此前已通知的结果。匹配对象的前后文可能尚未收集完整；结束时应使用函数最终返回的 `SearchResponse` 作为权威快照。
- `SearchOptions.MultiLineJSON`：启用多行 JSON 文档检索；对象/数组以括号深度识别，字符串中的括号与转义引号不会结束文档，单个文档限制 4 MiB。混合日志中的普通行仍按行匹配，只有文档命中附带 `EventContext`。
- `SearchOptions.EventContext`、`EventBoundary`、`EventBoundaryPattern`：启用事件上下文并按 `timestamp`（默认）、`blank` 或 `custom` 切分事件；`custom` 使用 Go 正则匹配去首尾空白后的整行，长度上限 512 字节，非法正则在打开文件前返回错误。空行模式下分隔空行不计入事件内容。
- `SearchOptions.Encoding`：接受 `auto`、`utf-8`、`gb18030`、`utf-16le`、`utf-16be`；最终识别结果通过 `SearchResponse.Encoding` 返回。自动模式先检查 BOM，再尝试 UTF-8/无 BOM UTF-16 特征，最后对非 UTF-8 文本按 GB18030 解码。
- 编码检测缓存（`internal/logx/index.go`）：`encodingIndex` 以三元组键（路径+大小+修改时间）缓存编码识别结果；`Lookup(path, size, mtime)` 命中则复用上次结果跳过 64 KiB 抽样，未命中走 `resolveTextEncoding` 后由 `Store` 写回。全局入口 `WarmEncodingCache()`（应用启动载入 `file-indexes.json`）与 `FlushEncodingIndex()`（退出原子落盘）。缓存最多 50 个文件、24 小时过期、LRU 淘汰；显式指定编码（非 `auto`）的检索不命中缓存。
- `TailFile(path, TailOptions)`：按偏移增量读取新增完整行，返回行、下一次偏移、文件大小、实际编码与重置/截断标记，供实时跟踪使用；编码识别与检索共用同一套抽样逻辑，UTF-16 明确拒绝。
- `EnumerateLogFiles(root, patterns, max)`：递归遍历目录，按 glob 模式（空则用默认日志扩展名）收集普通文件并按路径升序返回，最多 `max` 个（<=0 或超过 200 时按 `MaxMultiFiles`）；第二个返回值表示是否因超过文件数上限或访问条目上限（5 万）而截断；传入单个文件而非目录时报错。
- `SearchFilesConcurrently(ctx, files, query, useRegex, maxResults, SearchOptions, onProgress)`：以 `min(NumCPU, 8, 文件数)` 个 goroutine 从原子索引领取文件、逐个复用 `SearchFileWithOptions`（每文件回调传 nil），把结果合并成一份全局有序结果集。`maxResults` 是所有文件合计的命中上限，预算耗尽后停止领取新文件并把剩余文件计为跳过；`ctx` 取消后停止领取并保留已扫描文件（含中途取消部分）的结果；单文件失败只记入 `FileSearchState`，不整体报错。`onProgress` 在每个文件完成时以文件粒度回调一次，携带该文件新增匹配与跨文件聚合进度。
- 并行分块扫描（`internal/logx/parallel_search.go`）：`SearchFileWithOptions` 在满足 `isParallelizable` 条件时自动切换到分块并行模式（阶段 8）。判定条件：UTF-8 纯文本、非正则、无字符解码器、无多文件 JSON/事件/时间窗/续扫起点、文件大小 >= 64 MiB。文件被切成 64 MiB chunk，最多 `min(NumCPU, 8)` 个 worker 并发扫描；相邻 chunk 之间有 16 KiB 重叠窗口，避免行被截断；单行跨块时按行号顺序归并。每个 chunk 独立统计 `bytesScanned`、`scannedLines` 与 `matches`，最终按原始偏移排序合并回单份 `SearchResponse`。正则模式、需要 GB18030/UTF-16 解码、启用多文件 JSON 文档/事件上下文/时间窗过滤的检索强制走单线程流式扫描。
- 搜索结果缓存（`internal/logx/search_cache.go`，阶段 8）：
  - `WarmCacheIndex()`：应用启动时载入 `search-cache/cache-index.json`；缓存索引维护键到文件路径的映射与元数据。
  - `CheckCacheHit(ctx, filePath, query, useRegex, encoding, contextLines, timeWindowStartMs, timeWindowEndMs, timeZone)`：按 8 元组生成 SHA-256 键，命中后校验文件大小与修改时间是否与索引一致（`file-modernity`），一致则直接读回落盘 JSON 并返回带 `FromCache: true` 的 `SearchResponse`；不一致或键不存在返回 `hit=false`。
  - `PutCachedSearch(resp, timeWindowStartMs, timeWindowEndMs, timeZone)`：把成功检索结果（命中数 > 0）写入 `search-cache/<key>.json` 并更新索引，自动淘汰超过 128 条或总大小超过 500 MiB 的 LRU 冷条目。
  - `ListCacheEntries(limit)`：返回最近访问的 `CacheEntryMeta` 元数据列表，按访问时间降序；`limit` 默认 20，硬上限 50。
  - `DeleteCacheEntryByKey(key)` / `ClearAllCache()`：删除单条或全部缓存条目并同步更新索引。
  - `GetCacheStatistics()`：返回条目数、总大小、容量上限、命中/未命中/淘汰累计的 `CacheStatistics` 快照。
- 多文件时间线扫描（`internal/logx/timeline.go`，阶段 9）：
  - `ScanTimeline(ctx, files []TimelineFileSpec, opts TimelineScanOptions, onProgress)`：并发扫描多份日志，按每行可解析的日期时间戳合并成统一时间线。每个文件返回独立的 `entries`（时间戳升序），同时按时间戳合并去重生成全局 `entries`。默认每文件 5 万行、全局 20 万行硬上限，超出时 `truncated=true` 并在 `truncatedFiles` 列出文件。`levelDetect` 开启时自动识别每行级别；`timeZone` 控制时间戳解析时区。
- 日志文件头读取（`internal/logx/diff.go`，阶段 6 批次 D）：
  - `ReadDecodedFileHead(path, encoding, maxBytes)`：读取日志文件开头并解码为 UTF-8 返回 `DecodedFileHead`；编码识别复用 `resolveTextEncoding` 与 `file-indexes.json` 缓存；单侧 4 MiB 硬上限，超出截断并标记 `truncated`；编码不可解码时返回 error。

约束：普通文本按固定缓冲区流式扫描；结果上限为 5,000；结果预览 8 KiB；上下文预览 1 KiB；正则模式单行限制 4 MiB。

### `internal/process`

- `NewManager()` / `NewManagerAt(path)`：创建进程管理器。
- `Manager.List()`、`Save(Spec)`、`Delete(id)`：查看与管理配置。
- `Start(id)`、`Stop(id)`、`Restart(id)`、`StartBatch(ids)`、`StopBatch(ids)`、`StopAll()`：控制进程生命周期。
- `Manager.Logs(id, afterSequence)`：按游标读取输出。
- `Timeline(id)` / `RunHistory(id)`：返回按服务保留的最近生命周期事件与运行历史；`FlushEvents()` / `FlushRunHistory()` 同步排空待落盘变更，供退出收尾与测试使用（`StopAll()` 退出时会自动调用）。
- `ListGroups()` / `SaveGroup(Group)` / `DeleteGroup(id)` / `StartGroup(id)` / `StopGroup(id)`：服务分组（场景）的增删改查与整组启停，分组只存 ID 引用，启停复用 `StartBatch`/`StopBatch` 并过滤已删除成员。
- `FindServiceByLogPath(path)`：按落盘文件绝对路径反查所属服务，未命中返回 `ok=false`（`App.FindServiceByLogPath` 把它包装为单返回值快照，未命中时 `id` 为空）。
- `ExportConfig(redactSensitive)` / `ImportConfig(data)`：在 Go 层处理配置迁移；文件选择和写入由 Wails 的 `App` 方法处理。
- `RenderCommand(spec Spec)`：把尚未保存的 `Spec` 渲染成命令行预览（阶段 9），返回 `CommandPreview{ command, workingDir, env, warnings }`。按当前平台拼接：Windows 用 `set VAR=value&&` 前缀、POSIX 用 `VAR=value` 前缀；敏感变量名（`password`、`token`、`key`、`secret` 等子串，大小写不敏感）的值自动清空为 `***`。可执行文件不是绝对路径且不在 `%PATH%` 时在 `warnings` 列出提示，不阻止预览。`App.GetProcessCommandPreview` 直接调用此函数作为前端桥接入口。
- 自动重启：`Spec.Restart` 为 `on-failure` 且进程以非零码退出或收到信号时，按指数退避（1 秒起步、翻倍、上限 30 秒）调度重启，次数受 `MaxAttempts` 限制；定时器保存在管理器内部，手动停止、修改配置、删除服务、`Restart` 或再次启动都会取消待执行的重启。正常退出与启动失败（可执行文件创建失败）不触发重启。
- 健康检查：`health.go` 中的 `startHealthProbe` 在进程启动成功后起一个探测协程，按间隔执行 `probeHealth`（TCP `DialTimeout` 或 HTTP `Client.Get`，只读取 32 KiB 正文后丢弃）；探测结果写回快照，仅在健康状态变化时追加一条 `system` 输出。探测失败只标记状态，不触发重启；进程退出或实例被替换时通过 `probeStop` 结束协程。
- 依赖就绪：`readiness.go` 的 `waitReady` 在批量启动中等待上游健康。只有本次启动顺序里被下游依赖、且 `Health != nil`、`ReadyTimeoutSeconds > 0` 的服务才等待；每 50 ms 读取一次快照，`healthy` 后继续启动下游，`unhealthy`、实例被替换、进程停止或超时会阻断下游并把原因写入运行输出。
- 资源采样：`metrics.go` 在每次运行启动一个采样协程，每 2 秒调用平台实现 `sampleProcess`（Windows `GetProcessTimes` / `GetProcessMemoryInfo`，Linux `/proc/<pid>/stat`，macOS `ps -o time=,rss=`），按两次采样的差值计算单核 CPU 百分比并写入快照；停止或退出时通过 `stopSamplers` 关闭协程，采样失败只跳过本次写入。每次采样同时追加到实例内的历史窗口（`ResourceSample`，上限 150 个点）供 `ResourceHistory` 读取。
- 端口诊断：`ports.go` 在锁外调用系统命令并解析监听端口（Windows `netstat -ano -p TCP`，macOS `lsof -nP -iTCP -sTCP:LISTEN` 优先、`netstat` 回退，Linux `ss -ltnp` 优先、`lsof` 与 `netstat` 回退），单次命令 5 秒超时。`PortDiagnostics` 把配置端口标记为 `free`、`owned`、`occupied` 或 `unknown`，并单独返回本服务 PID 实际监听的端口；启动后若配置端口被其他进程占用，会在运行输出写入提示（含 PID 与进程名），不会阻止启动。当占用者是另一个本工作台托管的服务（非查询服务自身）时，`PortStatus` 会填充 `OwnerSpecID`/`OwnerName`，供前端提供停止入口；外部进程不填充。
- 输出落盘：`disklog.go` 的 `diskRotator` 把单个服务的输出按行追加到 `logs/<服务名>.log`，首次写入时才创建目录与文件（把权限/磁盘错误推迟到服务真正产生输出时）；内嵌换行用行分隔符替换以保证按行读取。单文件超过 64 MiB 后按命名位移轮转并最多保留 3 份，当前活动文件路径通过 `Snapshot.LogFilePath` 暴露。服务启动时创建写入器、退出时关闭，删除服务时清理当前及历史文件（共用同一文件名的其他配置不会被误删）。
- 时间线与运行历史持久化：`events.go`（`process-events.json`，每服务上限 200 条）与 `runhistory.go`（`run-history.json`，每服务上限 100 次）在 `NewManagerAt` 时载入历史、运行中累加，并通过 `writeJSONAtomic` 串行原子写回；应用退出前由 `FlushEvents`/`FlushRunHistory` 同步落盘。

### `internal/config`

- `DefaultSettings()`、`LoadSettings()`、`SaveSettings(settings)`：读写本地设置。
- `NormalizeSettings(settings)`：限制上下文行数和结果上限，并规范化历史记录。
- `AddRecentFile(settings, path)`、`AddSearchRecord(settings, record)`：更新最近文件与搜索历史。

## 5. 错误处理与安全说明

- 后端方法会返回明确的错误字符串；前端应捕获 Promise rejection 并提示用户。
- 搜索任务 ID 必须唯一；已完成任务在后续任务登记时按 10 分钟保留期清理；任务数量超过 32 时优先移除最早完成的任务，目标回落到 24 个。运行中的任务不会被自动驱逐。成功任务在最终响应形成后释放增量缓存；取消任务保留增量结果至过期，以支持晚到的轮询。
- 配置导出中的完整环境变量值可能是凭据。脱敏导出应作为默认选项。
- HTTP 调试器只发送用户显式填写的请求，不会自动访问网络；请求可指向本机或内网地址，发送前应确认目标地址与请求头中的凭据。
- 取消搜索不会撤销已经发现的结果。
- 本项目当前没有远程 API 身份认证层，因为其接口属于本机 Wails 桌面桥接而不是网络服务。