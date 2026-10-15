# MeteorDesk

**本地开发与运维工作台**

MeteorDesk 是一款使用 Go 和 Wails 构建的轻量级桌面工具，面向开发人员和运维人员，集中处理大体积日志检索、本地服务编排和日常开发辅助任务。

## 项目目标

- **本地优先：** 日志文件、进程输出和配置默认只保存在本机。
- **适配大文件：** 使用固定大小缓冲区顺序扫描文件，避免把数 GB 日志一次性读入内存。
- **轻量桌面：** Go 后端搭配系统 WebView，减少额外运行时开销。
- **模块清晰：** 日志分析、进程管理和开发工具分别维护，便于持续迭代。

## 当前功能

### 已有基础

- Wails v2 桌面工程入口与 Vue 3 + TypeScript + Vite 前端。
- 暗色工作台界面和中文导航。
- 原生日志文件选择器。
- 普通文本和 Go 正则表达式检索。
- 固定大小缓冲区分块读取，普通文本关键字可跨缓冲区边界匹配。
- 限制返回结果数量和单行预览大小。
- 检索进度展示、任务取消和后台进度轮询。
- 固定行高的虚拟结果列表，只渲染可视区域附近的结果。
- 正则模式单行大小限制和超限提示。
- 流式检索单元测试。

### 本轮已实现

- 匹配行前后文窗口（可设置 0、2、5、10 行）。
- JSON 行格式化及字段路径/字段值筛选；NDJSON 可逐行查看。
- 本地进程配置持久化、启动/停止/重启、标准输出与错误输出采集。
- 最近文件、检索历史及偏好设置本地持久化。
- 原生拖放打开文件和常用键盘快捷键。
- 64 MiB 大文件检索基准测试，可通过环境变量扩大至约 1 GiB。

### 后续计划

- 时间戳转换、Base64、JWT 载荷查看等开发辅助工具。
- 多文件对比、可选索引和服务依赖编排。
- 更完整的跨平台进程树终止和退出信号验证。

## 技术栈

- Go 1.23 或更高版本
- [Wails v2](https://wails.io/)
- Vue 3、TypeScript、Vite
- 系统 WebView（Windows 使用 WebView2，macOS/Linux 使用 WebKit）

## 环境准备

请先安装 Go、Node.js LTS、npm、Wails CLI，以及对应操作系统所需的桌面开发依赖。安装步骤请参考 [Wails v2 官方安装文档](https://wails.io/docs/gettingstarted/installation)。

## 开发启动

在仓库根目录执行：

```bash
cd frontend
npm install
npm run build
cd ..

go mod tidy
go test ./internal/logx ./internal/process ./internal/config
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

首次构建前先生成前端产物，Go 会通过嵌入文件系统将 `frontend/dist` 打包到桌面程序中。

## 项目结构

```text
.
├── app.go                  # 桌面接口、检索任务进度与取消管理
├── main.go                 # Wails 桌面应用入口
├── internal/
│   ├── logx/               # 流式日志检索与测试
│   └── process/            # 本地进程管理的数据结构
├── frontend/
│   ├── src/                # Vue 界面
│   └── dist/               # 嵌入桌面程序的前端产物
└── docs/
    └── ROADMAP.md          # 项目路线图
```

## 大文件检索的边界

- 普通文本检索按固定大小的数据块读取，并保留有限的边界字节，因此可以匹配恰好横跨读取缓冲区的关键字。
- 检索结果最多返回 500 条（后端可配置，硬上限 5000 条），每条结果默认最多展示 8 KiB。
- 正则表达式需要整行匹配；为避免超长单行造成内存失控，当前限制正则模式的单行大小为 4 MiB。遇到超限会给出提示，建议改用普通文本检索。
- 检索可以取消，并定期更新扫描字节数、行数和命中数量。
- 当前实现仍需在真实桌面环境完成构建和大文件性能验证；不要把工程骨架视为已经通过所有平台测试的正式发行版。

## 工程原则

- 默认不启用遥测，不上传日志内容。
- 不为搜索而拼接整条超长日志。
- 控制搜索结果和界面预览大小。
- 长任务应可查询进度、可取消。
- 后续进程管理使用显式的可执行文件、参数和工作目录，避免默认解释任意 Shell 文本。


## 大文件性能基准

默认基准会生成并扫描约 64 MiB 的合成日志：

\`\`\`bash
go test ./internal/logx -run '^$' -bench BenchmarkSearchFileLargeLog -benchtime=1x -benchmem
\`\`\`

若磁盘和内存允许，可提高至约 1 GiB：

\`\`\`bash
# macOS / Linux
METEORDESK_BENCH_SIZE_MB=1024 go test ./internal/logx -run '^$' -bench BenchmarkSearchFileLargeLog -benchtime=1x -benchmem

# PowerShell
$env:METEORDESK_BENCH_SIZE_MB = "1024"
go test ./internal/logx -run '^$' -bench BenchmarkSearchFileLargeLog -benchtime=1x -benchmem
\`\`\`

进程配置和偏好设置默认保存在当前用户系统配置目录的 \`MeteorDesk\` 子目录中。
