# 贡献指南

## 开发环境

- Go 1.23+
- Node.js 22 LTS 与 npm
- Wails v2；桌面构建还需要目标操作系统对应的 GTK/WebKit 或 WebView2 开发环境

## 本地验证

在仓库根目录执行：

```bash
go mod tidy
go vet ./...
go test ./...
```

构建前端：

```bash
cd frontend
npm install
npm run build
```

桌面端验证：

```bash
cd ..
wails dev
wails build
```

## 依赖锁文件

提交依赖变更时，必须使用真实工具链更新并提交 `go.sum` 与 `frontend/package-lock.json`，不要手工编造校验值，也不要只提交依赖清单而遗漏锁文件。

在仓库根目录执行并验证 Go 依赖：

```bash
go mod tidy
go mod verify
go vet ./...
go test ./...
go test -race ./...
```

再生成并验证 npm 锁文件：

```bash
cd frontend
npm install
npm test
npm run build
```

检查 `go.mod`、`frontend/package.json` 是否有非预期变化，并将对应锁文件与依赖清单一起提交。锁文件进入仓库后，CI 应使用 `go mod download`/`go mod verify` 和 `npm ci`，避免 CI 自行重写依赖解析结果。

## 变更要求

- 日志检索必须保持流式和内存有界；避免使用 `os.ReadFile` 读取整个日志。
- 新增编码处理时至少覆盖中文内容、行号、跨读取块匹配、取消和结果上下文。
- 进程管理变更需要考虑并发启停、输出管道关闭、父子进程及 Windows/macOS/Linux 的差异。
- 不应将凭据或完整生产日志提交到仓库；测试数据应使用可公开的合成内容。
- 注释重点解释边界条件、资源上限、并发时序和不直观的设计原因，不需要逐行复述代码。
- 文档接口签名、前端 TypeScript 声明和后端方法应同步更新。

## CI

GitHub Actions 会运行 Go 静态检查、完整单元测试、约 1 GiB 合成日志基准、前端类型检查/构建，并在 Ubuntu、Windows 和 macOS 编译桌面应用。大文件基准有固定的磁盘与时间开销；普通开发阶段可使用默认的 64 MiB 基准。

## 发布

发布由 tag 触发，不需要手动上传安装包：

1. 将 `frontend/package.json` 的版本号更新为待发布版本（`npm version <version> --no-git-tag-version`），同步 CHANGELOG 与文档。
2. 提交并推送到 `main`，确认基础检查工作流通过。
3. 创建并推送与版本一致的 tag，例如 `git tag v0.5.22 && git push origin v0.5.22`。
4. `发布` 工作流会在 Ubuntu、Windows、macOS 构建并打包安装包，校验 tag 与前端版本一致后创建 GitHub Release，附带 `SHA256SUMS.txt`。

若 tag 与 `frontend/package.json` 版本不一致，发布流程会直接失败。重新推送同一 tag 时，Release 资产会以 `--clobber` 覆盖更新。

## 提交建议

使用清楚的 Conventional Commits 风格，例如：

- `feat: support legacy log encodings`
- `fix: guard streamed search cancellation`
- `test: cover utf16 log line decoding`
- `docs: explain search and process architecture`
