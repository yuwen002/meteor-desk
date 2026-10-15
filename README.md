# MeteorDesk

**本地开发与运维工作台**

MeteorDesk 是一款使用 Go、Wails v2 和 Vue 3 构建的本地桌面工具，面向开发人员和运维人员，集中处理大体积日志检索、本地服务编排和日常开发任务。

## 本轮迭代：v0.5.0

- 本地服务支持依赖配置、循环依赖检测、按拓扑顺序启动及反向顺序停止。
- 支持全部启动、全部停止，并展示每个服务的操作结果；删除仍被其他服务依赖的配置会被拒绝。
- CI 增加约 1 GiB 合成日志的完整扫描基准，以及服务 stdout/stderr 与依赖顺序测试。
- 新增本地开发工具箱：时间戳/时区转换、JSON 格式化与结构化对比、UTF-8 Base64、JWT 载荷查看、URL 和查询参数处理。
- GitHub Actions 在 Ubuntu、Windows、macOS 分别构建桌面应用，持续验证跨平台编译。

## 当前功能

### 日志分析

- 普通文本和 Go 正则表达式检索。
- 使用 256 KiB 缓冲区顺序扫描；普通关键字可以跨读取块匹配。
- 支持前后 0、2、5、10 行上下文。
- 结果虚拟滚动、行号、结果数量限制和每行 8 KiB 预览上限。
- 支持查看 JSON/NDJSON 单行内容、格式化 JSON，并按字段路径或字段值筛选。
- 支持查询进度与取消；正则模式单行上限 4 MiB。

### 本地进程管理

- 保存服务名称、可执行文件、参数数组、工作目录和环境变量。
- 启动、停止、重启服务并查看运行状态与 PID。
- 配置服务依赖，启动时自动按拓扑顺序拉起前置服务；停止依赖项时，先停止依赖它的下游服务。
- 支持全部启动、全部停止，并显示每个服务的执行结果；检测并拒绝依赖循环。
- 分别收集标准输出、标准错误和生命周期消息。
- 单行输出最多保存 64 KiB，每个服务在内存中最多保留 1,500 条输出。
- 配置仅保存到当前用户的本机配置目录；服务不会因为保存配置而自动执行。
- 关闭应用时会取消正在运行的日志检索，并尝试停止由 MeteorDesk 启动的服务。
- 进程通过可执行文件和参数直接启动，不会把输入默认拼接为 Shell 命令。

### 开发工具箱

- 时间戳秒/毫秒与 IANA 时区之间转换，支持指定时区的日期时间转 Unix 时间戳。
- JSON 格式化、压缩，以及基于字段路径的新增/删除/修改对比。
- Base64 编码/解码，按 UTF-8 处理中文文本。
- JWT Header/Payload 解码及 `exp` 过期时间提示；**不验证签名，也不保证载荷可信**。
- 解析完整 URL 和 query string，支持 URL 组件编码/解码。
- 转换在本机完成，不调用外部服务；不要将私钥或敏感 Token 粘贴到不可信环境。

### 桌面体验与本地配置

- 原生拖放文件打开。
- 最近文件、检索历史、上下文行数和结果上限持久化。
- 常用快捷键：Ctrl/⌘+O 打开文件，Ctrl/⌘+K 聚焦搜索，Ctrl/⌘+Enter 检索，Ctrl/⌘+1/2/3 切换模块。
- 暗色中文工作台界面。

## 技术栈

- Go 1.23 或更高版本
- [Wails v2](https://wails.io/)
- Vue 3、TypeScript、Vite
- Windows 使用 WebView2；macOS/Linux 使用系统 WebKit

## 环境准备

请先安装 Go、Node.js LTS、npm、Wails CLI，以及对应操作系统的桌面开发依赖。参见 [Wails v2 官方安装文档](https://wails.io/docs/gettingstarted/installation)。

## 开发启动

在仓库根目录执行：

```bash
cd frontend
npm install
npm run build
cd ..

go mod tidy
go test ./...
wails dev
```

## 构建桌面程序

```bash
cd frontend
npm install
npm run build
cd ..

wails build
```

## 大文件性能基准

默认本地基准会生成并扫描约 64 MiB 的合成日志；GitHub Actions 会执行约 1 GiB 的合成日志全量扫描：

```bash
go test ./internal/logx -run '^$' -bench BenchmarkSearchFileLargeLog -benchtime=1x -benchmem
```

内存和磁盘空间允许时，可提高到约 1 GiB：

```bash
# macOS / Linux
METEORDESK_BENCH_SIZE_MB=1024 go test ./internal/logx -run '^$' -bench BenchmarkSearchFileLargeLog -benchtime=1x -benchmem

# PowerShell
$env:METEORDESK_BENCH_SIZE_MB = "1024"
go test ./internal/logx -run '^$' -bench BenchmarkSearchFileLargeLog -benchtime=1x -benchmem
```

## 配置保存位置

应用偏好设置与进程配置保存在当前用户系统配置目录下的 `MeteorDesk` 子目录中，分别为 `settings.json` 和 `processes.json`。这些数据不会上传；环境变量可能包含敏感信息，请只在可信本机使用。

## 项目结构

```text
.
├── app.go
├── main.go
├── internal/
│   ├── config/       # 本地偏好与检索历史
│   ├── logx/         # 流式日志检索、测试与性能基准
│   └── process/      # 进程配置、生命周期和输出采集
├── frontend/src/     # Vue 中文桌面界面
└── docs/ROADMAP.md
```

## 已知限制

- 目前返回结果仍有上限，长时间扫描只会显示有限的匹配预览。
- 正则模式整行匹配，单行超过 4 MiB 会停止并提示改用普通文本检索。
- 进程优雅退出与进程树回收需要继续在 Windows、macOS、Linux 上验证；强制结束子进程不一定会自动结束它派生的所有后代进程。
- 性能基准使用合成日志。正式发布前应使用真实日志和目标硬件记录扫描吞吐、内存和磁盘 I/O。
