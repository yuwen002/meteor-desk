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
| `SearchLogsAdvanced(searchID, path, query, useRegex, maxResults, contextLines)` | 上述参数及上下文行数 | `SearchResponse` | 执行搜索；任务需有唯一 ID，响应完成时返回权威完整结果。 |
| `GetLogSearchProgress(id)` | 任务 ID | `LogSearchProgress` | 读取字节数、扫描行数、匹配数、进度百分比及运行/取消状态。 |
| `GetLogSearchMatches(id, afterIndex)` | 任务 ID、已收到的匹配数量 | `LogMatch[]` | 只返回游标之后新发现的匹配，供前端增量展示。 |
| `CancelLogSearch(id)` | 任务 ID | `boolean` | 请求取消仍在运行的搜索；返回 false 表示任务不存在或已结束。 |

前端建议先生成唯一的 `searchID`，在调用 `SearchLogsAdvanced` 的同时定时轮询进度与增量匹配，并在最终 Promise 返回后以最终响应替换临时快照。取消是协作式操作：底层扫描会在检查点停止，并返回已累计结果。

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

### 设置和本地数据

| 方法 | 参数 | 返回 | 说明 |
|---|---|---|---|
| `GetAppSettings()` | 无 | `AppSettings` | 获取本地偏好和历史。 |
| `SaveAppSettings(settings)` | `AppSettings` | 无 | 规范化、保存并应用设置。 |
| `UserDataDirectory()` | 无 | `string` | 返回应用配置目录路径。 |

## 3. 常用数据结构

### SearchResponse

```json
{
  "filePath": "/path/to/service.log",
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

### ProcessSpec

配置字段包括 `id`、`name`、`executable`、`args`（字符串数组）、`workingDir`、`env`（字符串键值对象）、`autoStart` 和 `dependsOn`（服务 ID 数组）。程序通过可执行文件与参数数组启动，不默认拼接或解释 Shell 命令。

### ProcessStatus

运行快照字段包括 `state`、`pid`、`startedAt`、`finishedAt`、`durationMs`、`exitCode`、`exitReason` 和 `error`。退出原因枚举为 `normal`、`nonzero`、`signal`、`manual-stop`、`start-error`。

### ProcessOutput

输出行包括单调递增的服务内 `sequence`、本地时间、输出流类型（`stdout` / `stderr` / `system`）和文本内容。

## 4. Go 包 API

### `internal/logx`

- `SearchFile(path, query, useRegex, maxResults)`：简单同步搜索入口。
- `SearchFileContext(ctx, path, query, useRegex, maxResults, onProgress)`：支持取消与进度/增量匹配回调。
- `SearchFileWithOptions(ctx, path, query, useRegex, maxResults, SearchOptions, onProgress)`：支持上下文行配置的核心流式搜索入口。
- `Progress.Matches`：每次进度通知中新增的匹配增量，不会重复包含此前已通知的结果。匹配对象的前后文可能尚未收集完整；结束时应使用函数最终返回的 `SearchResponse` 作为权威快照。

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
- 搜索任务 ID 必须唯一；旧任务状态会在后续任务登记时按策略清理。
- 配置导出中的完整环境变量值可能是凭据。脱敏导出应作为默认选项。
- 取消搜索不会撤销已经发现的结果。
- 本项目当前没有远程 API 身份认证层，因为其接口属于本机 Wails 桌面桥接而不是网络服务。
