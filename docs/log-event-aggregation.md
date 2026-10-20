# 日志事件聚合

MeteorDesk 将多行原始日志聚合为可独立查看的逻辑事件。当前实现位于 `internal/logx/events.go`，作为实时 Tail 和检索结果详情接入前的共享基础模块。

## 规则

- 以常见日期时间戳开头的日志行开启新事件。
- 含 TRACE、DEBUG、INFO、WARN、ERROR、FATAL 或 PANIC 级别的普通日志行也可开启新事件。
- Java 等多行异常堆栈、缩进文本和普通续行保留在当前事件中。
- 多行 JSON 对象和数组按括号深度聚合；字符串中的括号和转义引号不会影响边界判断。
- 每个事件可配置最大行数，超限后标记 `Truncated` 并继续保留后续行。
- 事件行号从 1 开始；未带时区的时间戳由调用方指定时区解析。

## 接口示例

```go
events := logx.AggregateEvents(strings.Split(logText, "\n"), 10000)
for _, event := range events {
    fmt.Printf("%d-%d [%s] %s\n", event.StartLine, event.EndLine, event.Level, event.Timestamp)
}
```

该模块目前是纯 Go、无网络依赖的逻辑层；后续应由实时 Tail 与检索结果详情共用，并在接入前继续补充日志格式兼容性测试。
