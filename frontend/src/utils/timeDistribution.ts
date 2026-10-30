import { detectLevel } from './aggregate'

// 时间分布只关注错误与警告，用于快速定位错误集中的时间段。
export interface TimeBucket { start:number; label:string; error:number; warn:number }
export interface TimeDistribution {
  buckets:TimeBucket[]
  stepMs:number
  stepLabel:string
  maxCount:number
  errorTotal:number
  warnTotal:number
  undated:number
}

export const MAX_TIME_BUCKETS = 24

// 分桶步长取整秒到整天的常用值，保证标签可读且桶数不超过上限。
const STEPS = [1000, 5000, 10000, 30000, 60000, 300000, 600000, 1800000, 3600000, 10800000, 21600000, 43200000, 86400000]

// 支持 2026-10-30 10:00:00、2026-10-30T10:00:00.123Z 以及只有时间的 10:00:00。
const FULL_TIMESTAMP = /(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2}):(\d{2})(?:[.,](\d{1,3}))?/
const CLOCK_ONLY = /(?:^|[^\d])(\d{2}):(\d{2}):(\d{2})(?:[.,](\d{1,3}))?/

function fractionMs(fraction?:string):number {
  return fraction?Number(fraction.padEnd(3, '0')):0
}

function inRange(hour:string, minute:string, second:string):boolean {
  return Number(hour) <= 23 && Number(minute) <= 59 && Number(second) <= 59
}

// 时间戳统一按 UTC 计算，避免本机时区或夏令时影响分桶边界与标签。
export function extractLogTime(line:string):number|undefined {
  const full = FULL_TIMESTAMP.exec(line)
  if (full && inRange(full[4], full[5], full[6])) {
    return Date.UTC(Number(full[1]), Number(full[2]) - 1, Number(full[3]), Number(full[4]), Number(full[5]), Number(full[6]), fractionMs(full[7]))
  }
  const clock = CLOCK_ONLY.exec(line)
  if (!clock || !inRange(clock[1], clock[2], clock[3])) return undefined
  // 只有时间时锚定到 1970-01-01，保证同一天内的先后顺序稳定。
  return Date.UTC(1970, 0, 1, Number(clock[1]), Number(clock[2]), Number(clock[3]), fractionMs(clock[4]))
}

function pad(value:number):string {
  return String(value).padStart(2, '0')
}

// 秒级分桶保留秒，避免同一分钟内的错误被标成同一个时间点。
function formatBucketLabel(time:number, step:number):string {
  const date = new Date(time)
  const clock = pad(date.getUTCHours()) + ':' + pad(date.getUTCMinutes()) + (step < 60000 ? ':' + pad(date.getUTCSeconds()) : '')
  if (time < 86400000) return clock
  const day = pad(date.getUTCMonth() + 1) + '-' + pad(date.getUTCDate())
  return step >= 86400000 ? day : day + ' ' + clock
}

function formatStep(step:number):string {
  if (step < 60000) return step / 1000 + ' 秒'
  if (step < 3600000) return step / 60000 + ' 分钟'
  return step / 3600000 + ' 小时'
}

// buildTimeDistribution 按时间把 error/warn 行分桶；没有时间戳的行只计数，不参与分桶。
export function buildTimeDistribution(matches:Array<{content:string}>, maxBuckets = MAX_TIME_BUCKETS):TimeDistribution {
  const entries:Array<{time:number; level:'error'|'warn'}> = []
  let undated = 0
  for (const match of matches) {
    const level = detectLevel(match.content)
    if (level !== 'error' && level !== 'warn') continue
    const time = extractLogTime(match.content)
    if (time === undefined) { undated++; continue }
    entries.push({ time, level })
  }
  if (!entries.length) {
    return { buckets: [], stepMs: 0, stepLabel: '', maxCount: 0, errorTotal: 0, warnTotal: 0, undated }
  }

  let minTime = entries[0].time
  let maxTime = entries[0].time
  for (const entry of entries) {
    if (entry.time < minTime) minTime = entry.time
    if (entry.time > maxTime) maxTime = entry.time
  }
  const span = maxTime - minTime
  const step = STEPS.find(value => Math.ceil((span + 1) / value) <= maxBuckets) ?? STEPS[STEPS.length - 1]
  const firstStart = Math.floor(minTime / step) * step
  const count = Math.min(maxBuckets, Math.floor((maxTime - firstStart) / step) + 1)
  const buckets:TimeBucket[] = Array.from({ length: count }, (_, index) => {
    const start = firstStart + index * step
    return { start, label: formatBucketLabel(start, step), error: 0, warn: 0 }
  })

  let errorTotal = 0
  let warnTotal = 0
  let maxCount = 0
  for (const entry of entries) {
    const index = Math.min(count - 1, Math.floor((entry.time - firstStart) / step))
    if (index < 0) continue
    const bucket = buckets[index]
    if (entry.level === 'error') { bucket.error++; errorTotal++ } else { bucket.warn++; warnTotal++ }
    maxCount = Math.max(maxCount, bucket.error + bucket.warn)
  }
  return { buckets, stepMs: step, stepLabel: formatStep(step), maxCount, errorTotal, warnTotal, undated }
}
