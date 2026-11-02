// 时区换算纯函数：不依赖 Vue 与 DOM，可直接单元测试。
// 一律使用 Intl.DateTimeFormat 按 IANA 时区取名下时间，避免本机时区影响结果。

export interface DateParts {
  year: number
  month: number
  day: number
  hour: number
  minute: number
  second: number
}

/** 取某个瞬间在指定时区下的墙上时间分量。 */
export function partsInTimeZone(epochMs: number, zone: string): DateParts {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: zone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(new Date(epochMs))
  const value = (type: string) => Number(parts.find(part => part.type === type)?.value ?? 0)
  return { year: value('year'), month: value('month'), day: value('day'), hour: value('hour'), minute: value('minute'), second: value('second') }
}

/** 把瞬间格式化成 datetime-local 输入框需要的 YYYY-MM-DDTHH:mm（指定时区）。 */
export function getDateTimeInput(epochMs: number, zone: string): string {
  const p = partsInTimeZone(epochMs, zone)
  return String(p.year).padStart(4, '0') + '-' + String(p.month).padStart(2, '0') + '-' + String(p.day).padStart(2, '0') + 'T' + String(p.hour).padStart(2, '0') + ':' + String(p.minute).padStart(2, '0')
}

/**
 * 把「指定时区的墙上时间」换算为 Unix 毫秒。
 * 采用逐次修正：先把名义时间当成 UTC 得到候选瞬间，再读该瞬间在目标时区的实际
 * 墙上时间，用差值回推；最多迭代 5 次以覆盖夏令时切换带来的偏移变化。
 * 输入非法、或该墙上时间在目标时区不存在（夏令时跳时）时返回 null。
 */
export function zonedDateTimeToEpoch(input: string, zone: string): number | null {
  const match = input.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/)
  if (!match) return null
  const wanted: DateParts = {
    year: Number(match[1]), month: Number(match[2]), day: Number(match[3]),
    hour: Number(match[4]), minute: Number(match[5]), second: Number(match[6] ?? 0),
  }
  const wantedAsUTC = Date.UTC(wanted.year, wanted.month - 1, wanted.day, wanted.hour, wanted.minute, wanted.second)
  const validation = new Date(wantedAsUTC)
  if (validation.getUTCFullYear() !== wanted.year || validation.getUTCMonth() + 1 !== wanted.month || validation.getUTCDate() !== wanted.day || wanted.hour > 23 || wanted.minute > 59 || wanted.second > 59) return null

  let candidate = wantedAsUTC
  for (let i = 0; i < 5; i++) {
    const actual = partsInTimeZone(candidate, zone)
    const actualAsUTC = Date.UTC(actual.year, actual.month - 1, actual.day, actual.hour, actual.minute, actual.second)
    const adjustment = wantedAsUTC - actualAsUTC
    candidate += adjustment
    if (adjustment === 0) break
  }
  const final = partsInTimeZone(candidate, zone)
  if (final.year !== wanted.year || final.month !== wanted.month || final.day !== wanted.day || final.hour !== wanted.hour || final.minute !== wanted.minute || final.second !== wanted.second) return null
  return candidate
}
