import { detectLevel } from './aggregate'
import type { DiffLine } from './textDiff'

// 两份日志逐行对比的告警聚合：标出「仅出现在一侧」的 error/warn 行。
// removed = 只在左侧（基准文件），added = 只在右侧（对比文件）；等值行不参与。

export interface DiffAlertSample {
  side: 'left' | 'right'
  level: 'error' | 'warn'
  text: string
}

export interface DiffAlertSummary {
  leftError: number
  leftWarn: number
  rightError: number
  rightWarn: number
  total: number
  samples: DiffAlertSample[]
}

// isOneSidedAlert 判断某差异行是否为只在一侧出现的 error/warn 行。
export function isOneSidedAlert(line: DiffLine): boolean {
  if (line.kind === 'equal') return false
  const level = detectLevel(line.value)
  return level === 'error' || level === 'warn'
}

export function summarizeOneSidedAlerts(diff: DiffLine[], sampleLimit = 20): DiffAlertSummary {
  let leftError = 0
  let leftWarn = 0
  let rightError = 0
  let rightWarn = 0
  const samples: DiffAlertSample[] = []
  for (const line of diff) {
    if (line.kind === 'equal') continue
    const level = detectLevel(line.value)
    if (level !== 'error' && level !== 'warn') continue
    const side: DiffAlertSample['side'] = line.kind === 'removed' ? 'left' : 'right'
    if (side === 'left') {
      if (level === 'error') leftError++
      else leftWarn++
    } else {
      if (level === 'error') rightError++
      else rightWarn++
    }
    if (samples.length < sampleLimit) samples.push({ side, level, text: line.value.slice(0, 240) })
  }
  return { leftError, leftWarn, rightError, rightWarn, total: leftError + leftWarn + rightError + rightWarn, samples }
}
