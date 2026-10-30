// 进程生命周期时间线的展示纯函数：事件命名、时长格式化与启动耗时提取。

export interface TimelineEventLike {
  kind: string
  at: string
  detail?: string
  durationMs?: number
}

const EVENT_LABELS: Record<string, string> = {
  start: '启动',
  ready: '就绪',
  restart: '自动重启',
  exit: '退出',
}

// eventLabel 把后端事件类型翻成界面文案；未知类型原样展示，避免丢信息。
export function eventLabel(kind: string): string {
  return EVENT_LABELS[kind] ?? kind
}

// formatEventDuration 按量级选择单位：毫秒、秒、分秒与小时分。
export function formatEventDuration(ms?: number): string {
  if (ms === undefined || ms === null || Number.isNaN(ms) || ms < 0) return '—'
  if (ms < 1000) return Math.round(ms) + ' 毫秒'
  if (ms < 60000) return (ms / 1000).toFixed(ms < 10000 ? 1 : 0) + ' 秒'
  const totalSeconds = Math.round(ms / 1000)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  if (minutes < 60) return minutes + ' 分 ' + String(seconds).padStart(2, '0') + ' 秒'
  return Math.floor(minutes / 60) + ' 小时 ' + String(minutes % 60).padStart(2, '0') + ' 分'
}

// latestStartupMs 返回最近一次就绪事件的耗时，即最近一次启动耗时；没有就绪事件返回 null。
export function latestStartupMs(events: TimelineEventLike[]): number | null {
  for (let index = events.length - 1; index >= 0; index--) {
    const event = events[index]
    if (event.kind === 'ready' && event.durationMs !== undefined) return event.durationMs
  }
  return null
}

// countEvents 统计某一类事件出现的次数，用于「已自动重启 N 次」之类的概览。
export function countEvents(events: TimelineEventLike[], kind: string): number {
  return events.filter(event => event.kind === kind).length
}
