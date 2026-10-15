# MeteorDesk 性能基线与真实日志测试

本项目的性能基准分成两类：

- **合成日志基准**：便于 CI 在固定条件下比较扫描性能；默认约 64 MiB，CI 可设置为 1 GiB。
- **本机真实日志基准**：直接读取指定的本地日志文件，不复制文件内容、不上传日志、不把日志内容输出到测试报告。请只使用有权限读取的文件，并确认输出环境可信。

## 1. 合成日志基准

在仓库根目录运行：

```bash
go test ./internal/logx -run '^$' -bench '^BenchmarkSearchFileLargeLog$' -benchtime=1x -benchmem
```

Linux/macOS 下运行约 1 GiB 版本：

```bash
METEORDESK_BENCH_SIZE_MB=1024 go test ./internal/logx -run '^$' -bench '^BenchmarkSearchFileLargeLog$' -benchtime=1x -benchmem
```

PowerShell：

```powershell
$env:METEORDESK_BENCH_SIZE_MB = "1024"
go test ./internal/logx -run '^$' -bench '^BenchmarkSearchFileLargeLog$' -benchtime=1x -benchmem
```

## 2. 本机真实日志基准

设置日志文件路径后运行。默认使用一个不应出现在普通日志中的检索词，以尽可能完整扫描文件：

Linux/macOS：

```bash
METEORDESK_BENCH_LOG_PATH="/path/to/your.log" go test ./internal/logx -run '^$' -bench '^BenchmarkSearchFileRealLog$' -benchtime=1x -benchmem
```

PowerShell：

```powershell
$env:METEORDESK_BENCH_LOG_PATH = "C:\logs\service.log"
go test ./internal/logx -run '^$' -bench '^BenchmarkSearchFileRealLog$' -benchtime=1x -benchmem
```

也可设置 `METEORDESK_BENCH_QUERY` 指定关键字。若关键字在文件中大量出现，检索可能达到 500 条结果上限并提前结束；测全文件吞吐时应选一个不匹配或极少匹配的词。当前真实日志基准使用普通文本模式；编码识别仍由搜索器自动执行。

基准输出中的 `MB/s` 是 Go benchmark 根据输入文件字节数和扫描耗时计算的吞吐率；`B/op` 和 `allocs/op` 用于观察内存分配。磁盘缓存、存储介质、文件编码和操作系统都会影响结果。

## 3. 建议记录项

每次对比时记录以下信息，不要将敏感日志上传到仓库：

| 项目 | 记录内容 |
|---|---|
| 版本 / Git commit | |
| 操作系统与版本 | |
| CPU / 内存 / 存储介质 | |
| 文件大小 | |
| 文件编码（已知时） | |
| 搜索模式 / 关键字是否命中 | |
| 扫描吞吐（MB/s） | |
| 分配量（B/op）与分配次数（allocs/op） | |
| 取消响应时间（手动测试） | |
| 备注（冷/热缓存、异常长行等） | |

比较版本时尽量使用同一台机器、同一个文件、相同搜索条件，并重复运行几次。合成日志与真实日志的结果应分别记录，不能用合成数据代替生产环境结论。

## 4. 边界场景检查清单

- UTF-8、GB18030、UTF-16LE/BE（含 BOM 与已知的无 BOM 文件）。
- 普通文本关键字跨越读取缓冲区边界。
- 超长行、长关键字和异常输入文件。
- 正则单行大小上限及无效正则表达式。
- 大量匹配达到结果上限时的进度和提示。
- 扫描中取消、快速切换页面、取消后继续读取已有结果。
- 同时运行多个搜索任务时的内存占用和取消行为。

不要将密码、访问令牌、用户信息等敏感内容写入基准报告；如果真实日志包含敏感信息，记录文件大小和环境信息即可。
