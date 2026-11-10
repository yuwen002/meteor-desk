export type NumericBase = 2 | 8 | 10 | 16
export type BitWidth = 8 | 16 | 32 | 64

export interface NumberRepresentation {
  value: bigint
  isNegative: boolean
  width: BitWidth
}

const BASE_LABELS: Record<NumericBase, string> = { 2: '二进制', 8: '八进制', 10: '十进制', 16: '十六进制' }
const BASE_CHARS: Record<NumericBase, RegExp> = {
  2: /^[01]+$/,
  8: /^[0-7]+$/,
  10: /^[0-9]+$/,
  16: /^[0-9a-fA-F]+$/,
}

export const numberBaseLabels = BASE_LABELS
export const bitWidths: BitWidth[] = [8, 16, 32, 64]

export function parseNumber(input: string, base: NumericBase, width: BitWidth, signed: boolean): NumberRepresentation {
  let trimmed = input.trim()
  if (!trimmed) throw new Error('请输入要转换的数字。')

  let negative = false
  if (trimmed.startsWith('-')) {
    if (base !== 10 || !signed) throw new Error('只有有符号十进制整数支持负数。')
    negative = true
    trimmed = trimmed.slice(1)
  }

  let body = trimmed
  if (base === 16 && /^0x/i.test(body)) body = body.slice(2)
  else if (base === 2 && /^0b/i.test(body)) body = body.slice(2)
  else if (base === 8 && /^0o/i.test(body)) body = body.slice(2)

  body = body.replace(/^0+/, '') || '0'

  if (!BASE_CHARS[base].test(body)) {
    throw new Error(`${BASE_LABELS[base]} 字符串只能包含合法字符：${base === 2 ? '0/1' : base === 8 ? '0–7' : base === 10 ? '0–9' : '0–9/a–f/A–F'}。`)
  }

  const maxUnsigned = (1n << BigInt(width)) - 1n
  let value = 0n
  const baseN = BigInt(base)
  for (const ch of body) {
    const digit = BigInt(parseInt(ch, 16))
    value = value * baseN + digit
  }
  if (negative) value = -value

  if (!signed) {
    if (value < 0n) throw new Error('无符号整数不能为负数。')
    if (value > maxUnsigned) throw new Error(`无符号 ${width} 位整数范围 0 – ${maxUnsigned.toString()}，超出上限。`)
  } else {
    const minSigned = -(1n << BigInt(width - 1))
    const maxSigned = (1n << BigInt(width - 1)) - 1n
    if (value < minSigned || value > maxSigned) throw new Error(`有符号 ${width} 位整数范围 ${minSigned.toString()} – ${maxSigned.toString()}，超出上限。`)
  }

  return { value, isNegative: negative, width }
}

export function formatBase(rep: NumberRepresentation, base: NumericBase, signed: boolean): string {
  let v = rep.value
  if (signed && v < 0n) v = (1n << BigInt(rep.width)) + v
  if (v < 0n) v = -v
  const str = v.toString(Number(base))
  if (base === 16) return str.toUpperCase()
  return str
}

export function allBases(rep: NumberRepresentation, signed: boolean): Record<NumericBase, string> {
  return {
    2: formatBase(rep, 2, signed),
    8: formatBase(rep, 8, signed),
    10: signed && rep.isNegative ? rep.value.toString() : rep.value.toString(),
    16: formatBase(rep, 16, signed),
  }
}

export function extractBitRange(rep: NumberRepresentation, high: number, low: number): { value: bigint; binary: string; decimal: string } {
  if (high < low) throw new Error('高位不能小于低位。')
  if (low < 0 || high >= rep.width) throw new Error(`位范围必须在 0 – ${rep.width - 1} 之间。`)
  let v = rep.value
  if (v < 0n) v = (1n << BigInt(rep.width)) + v
  const mask = (1n << BigInt(high - low + 1)) - 1n
  const shifted = v >> BigInt(low)
  const extracted = shifted & mask
  const binary = extracted.toString(2).padStart(high - low + 1, '0')
  return { value: extracted, binary, decimal: extracted.toString() }
}

export interface ConstantsEntry {
  name: string
  decimal: string
  hex: string
  note?: string
}

export function constantsForWidth(width: BitWidth): ConstantsEntry[] {
  const maxUnsigned = (1n << BigInt(width)) - 1n
  const maxSigned = (1n << BigInt(width - 1)) - 1n
  const minSigned = -(1n << BigInt(width - 1))
  const allOnes = maxUnsigned
  const entries: ConstantsEntry[] = [
    { name: '零', decimal: '0', hex: '0' },
    { name: '全 1 掩码', decimal: allOnes.toString(), hex: '0x' + allOnes.toString(16).toUpperCase(), note: `无符号 ${width} 位上限` },
    { name: '有符号最大值', decimal: maxSigned.toString(), hex: '0x' + maxSigned.toString(16).toUpperCase() },
    { name: '有符号最小值', decimal: minSigned.toString(), hex: '-0x' + (-minSigned).toString(16).toUpperCase() },
  ]
  if (width >= 16) {
    entries.push({ name: '0xFF / 0xFFFF', decimal: width === 16 ? '65535' : '255', hex: width === 16 ? '0xFFFF' : '0xFF' })
  }
  return entries
}