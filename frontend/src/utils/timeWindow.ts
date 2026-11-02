// 时间窗解析纯函数：把界面上的时间窗输入（相对快捷项或绝对起止）换算成
// 后端需要的 Unix 毫秒区间。不依赖 Vue 与 DOM，可直接单元测试。
import { zonedDateTimeToEpoch } from './timezone'

export type TimeWindowZone = 'local' | 'utc'

/** 界面上与时间窗相关的输入。 */
export interface TimeWindowInput {
  /** 是否启用时间窗。 */
  enabled: boolean
  /** 相对快捷项：''、5m、30m、1h、6h、24h；非空时优先于绝对起止。 */
  preset: string
  /** datetime-local 字符串（YYYY-MM-DDTHH:mm[:ss]），可空。 */
  start: string
  /** datetime-local 字符串，可空；空表示上界开放。 */
  end: string
  /** naive 时间戳与窗口边界的解释参照，与后端 TimeWindowZone 一致。 */
  zone: TimeWindowZone
}

/** 解析后的时间窗，直接对应后端 SearchOptions 的时间窗字段。 */
export interface ResolvedTimeWindow {
  enabled: boolean
  startMs: number
  endMs: number
  zone: TimeWindowZone
  /** 非空表示输入有误，调用方应提示并阻止检索。 */
  error?: string
}

const PRESET_MS: Record<string, number> = {
  '5m': 5 * 60_000,
  '30m': 30 * 60_000,
  '1h': 60 * 60_000,
  '6h': 6 * 60 * 60_000,
  '24h': 24 * 60 * 60_000,
}

/** 相对快捷项的可选项，供界面下拉与解析共用。 */
export const TIME_WINDOW_PRESETS: { value: string; label: string }[] = [
  { value: '', label: '不使用相对时间窗' },
  { value: '5m', label: '最近 5 分钟' },
  { value: '30m', label: '最近 30 分钟' },
  { value: '1h', label: '最近 1 小时' },
  { value: '6h', label: '最近 6 小时' },
  { value: '24h', label: '最近 24 小时' },
]

const DISABLED: ResolvedTimeWindow = { enabled: false, startMs: 0, endMs: 0, zone: 'local' }

/**
 * 解析时间窗输入。nowMs 与 localZone 由调用方注入（Date.now 与本机 IANA 时区），
 * 便于测试且不依赖运行环境时区。
 */
export function resolveTimeWindow(input: TimeWindowInput, nowMs: number, localZone: string): ResolvedTimeWindow {
  if (!input.enabled) {
    return { ...DISABLED, zone: input.zone }
  }
  // 相对快捷项以提交时刻为基准锚定（文件是静态快照，长扫描内不需要跟随当前时间移动）。
  if (input.preset) {
    const span = PRESET_MS[input.preset]
    if (!span) return { ...DISABLED, zone: input.zone, error: '未知的相对时间窗选项。' }
    return { enabled: true, startMs: nowMs - span, endMs: nowMs, zone: input.zone }
  }

  const zoneIana = input.zone === 'utc' ? 'UTC' : localZone
  const hasStart = Boolean(input.start)
  const hasEnd = Boolean(input.end)
  if (!hasStart && !hasEnd) {
    // 勾选了时间窗却没给任何边界：按未启用处理，避免误用 Unix 0（1970）作为下界。
    return { ...DISABLED, zone: input.zone }
  }

  let startMs = 0
  let endMs = 0
  if (hasStart) {
    const parsed = zonedDateTimeToEpoch(input.start, zoneIana)
    if (parsed === null) return { ...DISABLED, zone: input.zone, error: '开始时间格式无效或在该时区不存在。' }
    startMs = parsed
  }
  if (hasEnd) {
    const parsed = zonedDateTimeToEpoch(input.end, zoneIana)
    if (parsed === null) return { ...DISABLED, zone: input.zone, error: '结束时间格式无效或在该时区不存在。' }
    endMs = parsed
  }
  if (hasStart && hasEnd && startMs >= endMs) {
    return { ...DISABLED, zone: input.zone, error: '开始时间必须早于结束时间。' }
  }
  return { enabled: true, startMs, endMs, zone: input.zone }
}
