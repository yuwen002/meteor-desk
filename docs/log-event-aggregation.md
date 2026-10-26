# 日志事件聚合

MeteorDesk 的事件聚合逻辑位于 `internal/logx/events.go`，用于把多行原始日志整理成逻辑事件。现有检索结果上下文（`EventContext`）仍负责提供命中事件的完整内容；`AggregateEvents` 是用于实时 Tail 等场景的行级聚合基础模块，两者用途不同，不互相替代。

## 规则

- 以常见日期时间戳开头的日志行开启新事件。
- 识别 TRACE、DEBUG、INFO、WARN/WARNING、ERROR、FATAL 和 PANIC 等级别。
- 普通续行、Java 等多行异常堆栈保留在当前事件中。
- 多行 JSON 对象和数组按括号深度聚合；字符串内的括号、时间戳和日志级别不会错误触发边界。
- 通过 `maxEventLines` 限制单事件行数；达到上限时标记当前事件 `Truncated`，后续行仍会保留为后续事件。
- 事件的 `StartLine` / `EndLine` 从 1 开始。
- `ParseEventTimestamp` 支持常见 RFC3339、日期时间与仅时分秒格式；不带时区的值按传入的时区解释。

## 示例

```go
lines := strings.Split(logText, "\n")
events := logx.AggregateEvents(lines, 10000)
for _, event := range events {
    fmt.Printf("%d-%d [%s] %s\n", event.StartLine, event.EndLine, event.Level, event.Timestamp)
}
```

聚合是纯本地逻辑，不联网、不上传日志。当前行级聚合模块已补充边界、长堆栈、时间戳、多行 JSON 和超限截断测试；将它接入实时 Tail 仍是后续任务。
