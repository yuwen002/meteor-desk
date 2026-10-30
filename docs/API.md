# MeteorDesk 接口与代码 API

## 1. 接口形态

MeteorDesk 是 Wails v2 桌面应用，不提供供外部客户端调用的 REST/HTTP API。前端通过 Wails 生成的 Go 方法桥接调用后端 `App` 方法，典型调用路径为 `window.go.main.App`。

下表中的方法是前端绑定接口。Go 层签名以 `app.go` 为准；Go 的 `error` 在 JavaScript 桥接中表现为 Promise rejection。

## 2. 前端桥接方法

### 日志搜索

| 方法 | 参数 | 返回 | 说明 |
|---|---|---|---|
| `SelectLogFile()` | 无 | `string` | 打开系统文件选择器；取消选择时路径为空字符串。 |
| `PollDroppedFiles()` | 无 | `string[]` | 取出并清空拖放文件队列。 |
| `SearchLogs(searchID, path, query, useRegex, maxResults)` | 任务 ID、路径、搜索内容、是否正则、结果上限 | `SearchResponse` | 旧版兼容入口，不附加上下文行。 |
| `SearchLogsAdvanced(searchID, path, query, useRegex, maxResults, contextLines, encoding, multiLineJSON, eventContext, eventBoundary, eventBoundaryPattern)` | 任务 ID、路径、查询、正则开关、结果上限、上下文行数、编码模式、多行 JSON 开关、事件上下文开关、事件边界模式、自定义边界正则 | `SearchResponse` | 执行搜索；编码支持 `auto`、`utf-8`、`gb18030`、`utf-16le`、`utf-16be`；`multiLineJSON=true` 时将跨行 JSON 对象/数组作为整体匹配单元，并为文档命中附带完整文档上下文；`eventContext=true` 时按 `eventBoundary`（`timestamp` 默认 / `blank` / `custom`）聚合事件，`custom` 时使用 `eventBoundaryPattern`。任务需有唯一 ID，响应完成时返回权威完整结果。 |
| `GetLogSearchProgress(id)` | 任务 ID | `LogSearchProgress` | 读取字节数、扫描行数、匹配数、进度百分比及运行/取消状态。 |
| `GetLogSearchMatches(id, afterIndex)` | 任务 ID、已收到的匹配数量 | `LogMatch[]` | 只返回游标之后新发现的匹配，供前端增量展示；负游标按 0 处理，超出当前结果长度时返回空数组。取消任务会在保留期内保留已发现结果。 |
| `CancelLogSearch(id)` | 任务 ID | `boolean` | 请求取消仍在运行的搜索；返回 false 表示任务不存在或已结束。 |
| `ReadLogTail(path, offset, encoding)` | 路径、上次偏移、编码模式 | `TailResult` | 增量读取日志新增行；`offset` 为负时从文件末尾回退（默认 256 KiB）开始跟踪。只返回到最后一个换行符为止的完整行，支持 `auto`、`utf-8`、`gb18030`，UTF-16 直接返回错误。 |

前端建议先生成唯一的 `searchID`，在调用 `SearchLogsAdvanced` 的同时监听 Wails `log-search:update` 事件以低延迟接收进度和新增匹配；轮询进度/增量接口仍作为兼容回退。事件包含 `searchId`、`progress` 和可选的 `matches`，前端必须按任务 ID 过滤并与轮询共用游标，防止重复追加。最终 Promise 返回后，以最终响应替换临时快照。取消是协作式操作：底层扫描会在检查点停止，并返回已累计结果。

### 本地服务

| 方法 | 参数 | 返回 | 说明 |
|---|---|---|---|
| `GetProcesses()` | 无 | `ProcessView[]` | 返回服务定义和运行状态。 |
| `SaveProcess(spec)` | `ProcessSpec` | `ProcessView` | 新增或更新配置；不启动服务。 |
| `DeleteProcess(id)` | 服务 ID | 无 | 仅允许删除已停止且没有被其他服务依赖的服务。 |
| `StartProcess(id)` | 服务 ID | 无 | 启动服务和所需上游依赖。 |
| `StopProcess(id)` | 服务 ID | 无 | 先停依赖它的下游服务，再停止目标。 |
| `RestartProcess(id)` | 服务 ID | 无 | 先停再启动。 |
| `StartProcesses(ids)` | 服务 ID 数组 | `ProcessBatchResult[]` | 按依赖拓扑顺序逐项启动。 |
| `StopProcesses(ids)` | 服务 ID 数组 | `ProcessBatchResult[]` | 按反向依赖顺序逐项停止。 |
| `GetProcessLogs(id, after)` | 服务 ID、上次输出序号 | `ProcessOutput[]` | 仅返回序号大于 `after` 的输出行。 |
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
  "canceled": false
}
```

### LogSearchProgress

字段包括 `searchId`、`bytesScanned`、`fileSizeBytes`、`scannedLines`、`matchesFound`、`percent`、`running`、`canceled` 和可选的 `error`。

### TailResult

字段包括 `lines`（新增行，按文件顺序）、`offset`（下一次读取的起点）、`fileSizeBytes`、`encoding`（实际编码）、`reset`（首次跟踪或文件被截断/替换，前端应清空已有行）和 `truncated`（达到单次上限、跳过超长行或回退点残行）。单次读取默认上限 256 KiB / 500 行，硬上限 2 MiB / 5,000 行；偏移只前进到最后一个完整换行之后，未写完的行留到下一次调用。

### EventContext

命中详情中的可选字段，默认不返回。字段包括：`startLine`、`endLine`、`content`（完整事件或 JSON 文档，单条最多 512 KiB）、`contentClipped` 和 `kind`（`event` 表示按边界聚合的事件，`document` 表示多行 JSON 文档）。单次检索为所有上下文保留的内容总量上限为 32 MiB；预算耗尽时后续命中不返回内容，前端应回退到行预览。

### ProcessSpec

配置字段包括 `id`、`name`、`executable`、`args`（字符串数组）、`workingDir`、`env`（字符串键值对象）、`autoStart` 和 `dependsOn`（服务 ID 数组）。程序通过可执行文件与参数数组启动，不默认拼接或解释 Shell 命令。

### ProcessStatus

运行快照字段包括 `state`、`pid`、`startedAt`、`finishedAt`、`durationMs`、`exitCode`、`exitReason` 和 `error`。退出原因枚举为 `normal`、`nonzero`、`signal`、`manual-stop`、`start-error`。

### ProcessOutput

输出行包括单调递增的服务内 `sequence`、本地时间、输出流类型（`stdout` / `stderr` / `system`）和文本内容。

### RuntimeInfo

字段包括 `appVersion`、`goVersion`、`platform`（`GOOS/GOARCH`）和 `dataDir`。版本号只在 `frontend/package.json` 维护：前端构建时直接读取该文件，发布构建通过 `-ldflags "-X main.appVersion=<version>"` 注入后端，本地开发未注入时为 `dev`。该结构不包含日志内容、环境变量或凭据。

### 导出格式

检索结果和进程输出由前端 `frontend/src/utils/exporters.ts` 生成文本，后端只负责写入文件。

- 检索结果 CSV：列为 `matchLine`、`type`、`lineNumber`、`endLine`、`query`、`content`；`type` 为 `match`、`context-before`、`context-after`、`event` 或 `document`，上下文行与事件内容逐条展开。文件带 UTF-8 BOM，使用 CRLF 换行并按 RFC 4180 转义。
- 检索结果 JSON：包含 `exportedAt`、文件路径、检索词、是否正则、编码、上下文行数、`resultLimitReached`、`canceled`、`total` 和完整 `matches`。
- 进程日志 CSV：列为 `sequence`、`time`、`stream`、`text`；JSON 额外包含 `processId`、`processName` 和 `total`。导出范围仅限界面缓存的最近 1,500 条输出。

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
- `TailFile(path, TailOptions)`：按偏移增量读取新增完整行，返回行、下一次偏移、文件大小、实际编码与重置/截断标记，供实时跟踪使用；编码识别与检索共用同一套抽样逻辑，UTF-16 明确拒绝。

约束：普通文本按固定缓冲区流式扫描；结果上限为 5,000；结果预览 8 KiB；上下文预览 1 KiB；正则模式单行限制 4 MiB。

### `internal/process`

- `NewManager()` / `NewManagerAt(path)`：创建进程管理器。
- `Manager.List()`、`Save(Spec)`、`Delete(id)`：查看与管理配置。
- `Start(id)`、`Stop(id)`、`Restart(id)`、`StartBatch(ids)`、`StopBatch(ids)`、`StopAll()`：控制进程生命周期。
- `Manager.Logs(id, afterSequence)`：按游标读取输出。
- `ExportConfig(redactSensitive)` / `ImportConfig(data)`：在 Go 层处理配置迁移；文件选择和写入由 Wails 的 `App` 方法处理。

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
