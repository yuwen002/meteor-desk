// 运行历史的展示纯函数：退出原因文案、时长/峰值格式化与概览统计。
// 与后端 process.RunRecord 的字段一一对应，方便脱离组件单测。

export interface RunRecordLike {
  startedAt?: string
  finishedAt?: string
  durationMs?: number
  exitReason?: string
  exitCode?: number
  restarts?: number
  peakCpuPercent?: number
  peakMemoryBytes?: number
}

const EXIT_LABELS: Record<string, string> = {
  normal: '正常退出',
  nonzero: '非零退出',
  signal: '信号终止',
  'manual-stop': '手动停止',
  'start-error': '启动失败',
}

// runExitLabel 把退出原因翻成界面文案；未知原因原样展示，缺失时返回空串。
export function runExitLabel(reason?: string): string {
  if (!reason) return ''
  return EXIT_LABELS[reason] ?? reason
}

// runExitSummary 组合退出原因与退出码，例如「非零退出（1）」；无退出码时只展示原因。
export function runExitSummary(reason?: string, exitCode?: number): string {
  const label = runExitLabel(reason)
  if (!label) return ''
  if (exitCode === undefined || exitCode === null) return label
  return label + '（' + exitCode + '）'
}

// formatRunDuration 按毫秒时长选择合适单位；无效或缺失返回破折号。
export function formatRunDuration(ms?: number): string {
  if (ms === undefined || ms === null || Number.isNaN(ms) || ms < 0) return '—'
  if (ms < 1000) return Math.round(ms) + ' 毫秒'
  if (ms < 60000) return (ms / 1000).toFixed(ms < 10000 ? 1 : 0) + ' 秒'
  const totalSeconds = Math.round(ms / 1000)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  if (minutes < 60) return minutes + ' 分 ' + String(seconds).padStart(2, '0') + ' 秒'
  return Math.floor(minutes / 60) + ' 小时 ' + String(minutes % 60).padStart(2, '0') + ' 分'
}

// formatRunBytes 把字节数格式化到合适单位；零或缺失返回破折号。
export function formatRunBytes(bytes?: number): string {
  if (!bytes || bytes <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index++
  }
  return (value >= 100 || index === 0 ? Math.round(value).toString() : value.toFixed(1)) + ' ' + units[index]
}

// formatRunPeakCPU 保留一位小数并加百分号；无数据返回破折号。
export function formatRunPeakCPU(percent?: number): string {
  if (!percent || percent <= 0) return '—'
  return percent.toFixed(1) + '%'
}

export interface RunHistorySummary {
  count: number
  nonzeroExits: number
  totalRestarts: number
}

// summarizeRunRecords 统计运行次数、异常退出（非零/信号/启动失败）次数与累计自动重启次数。
export function summarizeRunRecords(records: RunRecordLike[]): RunHistorySummary {
  let nonzeroExits = 0
  let totalRestarts = 0
  for (const record of records) {
    if (record.exitReason === 'nonzero' || record.exitReason === 'signal' || record.exitReason === 'start-error') nonzeroExits++
    totalRestarts += record.restarts ?? 0
  }
  return { count: records.length, nonzeroExits, totalRestarts }
}
