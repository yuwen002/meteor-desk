export type ByteUnit = 'B' | 'KB' | 'MB' | 'GB' | 'TB' | 'PB'
export type UnitSystem = 'iec' | 'si'

const IEC_FACTOR = 1024
const SI_FACTOR = 1000

export const iecUnits: ByteUnit[] = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']

export function bytesToUnit(bytes: number, unit: ByteUnit, system: UnitSystem = 'iec'): number {
  if (!Number.isFinite(bytes) || bytes < 0) throw new Error('字节数必须是 >= 0 的有限数字。')
  const factor = system === 'iec' ? IEC_FACTOR : SI_FACTOR
  const idx = iecUnits.indexOf(unit)
  return bytes / Math.pow(factor, idx)
}

export function unitToBytes(value: number, unit: ByteUnit, system: UnitSystem = 'iec'): number {
  if (!Number.isFinite(value) || value < 0) throw new Error('数值必须是 >= 0 的有限数字。')
  const factor = system === 'iec' ? IEC_FACTOR : SI_FACTOR
  const idx = iecUnits.indexOf(unit)
  return Math.round(value * Math.pow(factor, idx))
}

export function humanizeBytes(bytes: number, system: UnitSystem = 'iec', digits: number = 2): string {
  if (!Number.isFinite(bytes) || bytes < 0) throw new Error('字节数必须是 >= 0 的有限数字。')
  const factor = system === 'iec' ? IEC_FACTOR : SI_FACTOR
  if (bytes === 0) return '0 B'
  let idx = 0
  let val = bytes
  while (val >= factor && idx < iecUnits.length - 1) { val /= factor; idx++ }
  const suffix = system === 'iec' && idx > 0 ? 'iB' : (idx === 0 ? 'B' : iecUnits[idx])
  const numStr = val >= 100 ? val.toFixed(0) : val >= 10 ? val.toFixed(1) : val.toFixed(digits)
  return numStr + ' ' + suffix
}

export function parseHumanized(input: string, system: UnitSystem = 'iec'): number {
  const trimmed = input.trim()
  const m = trimmed.match(/^([\d.]+)\s*([KMGTPE]?)\s*(iB|B)?$/i)
  if (!m) throw new Error('格式示例："2.5 GiB"、"500 MB"、"1024"。')
  const num = parseFloat(m[1])
  if (!Number.isFinite(num) || num < 0) throw new Error('数值必须是 >= 0 的有限数字。')
  const letter = (m[2] || '').toUpperCase()
  const unit: ByteUnit = letter ? ((letter + 'B') as ByteUnit) : 'B'
  const hasI = !!m[3] && m[3].toLowerCase() === 'ib'
  const effectiveSystem: UnitSystem = hasI ? 'iec' : system
  return unitToBytes(num, unit, effectiveSystem)
}

export interface BandwidthEstimate { transferSeconds: number; formatted: string }

export function estimateTransfer(bytes: number, bytesPerSecond: number): BandwidthEstimate {
  if (bytes < 0 || bytesPerSecond <= 0) throw new Error('字节数与传输速率必须为正数。')
  const seconds = bytes / bytesPerSecond
  let formatted: string
  if (seconds < 1) formatted = Math.round(seconds * 1000) + ' 毫秒'
  else if (seconds < 60) formatted = seconds.toFixed(1) + ' 秒'
  else if (seconds < 3600) formatted = Math.floor(seconds / 60) + ' 分 ' + Math.round(seconds % 60) + ' 秒'
  else formatted = Math.floor(seconds / 3600) + ' 时 ' + Math.round((seconds % 3600) / 60) + ' 分'
  return { transferSeconds: seconds, formatted }
}

export const bandwidthPresets = [
  { label: '1 Mbps 宽带', bytesPerSecond: 125000 },
  { label: '10 Mbps 宽带', bytesPerSecond: 1250000 },
  { label: '100 Mbps 局域网', bytesPerSecond: 12500000 },
  { label: '1 Gbps 局域网', bytesPerSecond: 125000000 },
  { label: '本地磁盘顺序读', bytesPerSecond: 500000000 },
  { label: 'NVMe SSD 顺序读', bytesPerSecond: 3500000000 },
]
