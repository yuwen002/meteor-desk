// 随机字符串与 UUID 生成器：使用 Web Crypto 的加密安全随机源（crypto.getRandomValues），
// 不用 Math.random，避免在 token、密钥等安全场景产生可预测取值。
// 底层随机源通过 `getRandomValues` 注入为参数，方便在无 crypto 的测试环境替换。

export type RandomSource = (array: Uint8Array) => Uint8Array

function defaultSource(): RandomSource | null {
  const g = (globalThis.crypto as globalThis.Crypto | undefined)?.getRandomValues
  if (!g) return null
  return (array: Uint8Array) => g.call(globalThis.crypto, array) as unknown as Uint8Array
}

const LETTERS_LOWER = 'abcdefghijklmnopqrstuvwxyz'
const LETTERS_UPPER = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'
const DIGITS = '0123456789'
const SYMBOLS = '!@#$%^&*()-_=+[]{};:,.<>?/~`'

export interface RandomOptions {
  length: number
  lowercase: boolean
  uppercase: boolean
  digits: boolean
  symbols: boolean
}

export function charPool(options: RandomOptions): string {
  let pool = ''
  if (options.lowercase) pool += LETTERS_LOWER
  if (options.uppercase) pool += LETTERS_UPPER
  if (options.digits) pool += DIGITS
  if (options.symbols) pool += SYMBOLS
  return pool
}

function randomBytes(length: number, source: RandomSource): Uint8Array {
  const bytes = new Uint8Array(length)
  source(bytes)
  return bytes
}

// 用拒绝采样避免模偏差（modulo bias）：每个字符在池长不大的情况下用“范围尽可能接近
// 256 的倍数”的采样，保证每个候选字符出现概率均等。
export function generateRandomString(options: RandomOptions, source: RandomSource = defaultSource()!): string {
  if (!source) throw new Error('当前环境不支持 Web Crypto，无法生成安全随机数。')
  const pool = charPool(options)
  if (!pool) return ''
  const poolSize = pool.length
  const maxValid = 256 - (256 % poolSize)
  const bytes = randomBytes(options.length, source)
  const out = new Array<string>(options.length)
  for (let i = 0; i < options.length; i++) {
    let value = bytes[i]
    while (value >= maxValid) {
      value = randomBytes(1, source)[0]
    }
    out[i] = pool[value % poolSize]
  }
  return out.join('')
}

// 标准 UUID v4：第 13 个半字节固定为版本 4，第 17 个半字节设为 RFC 4122 的 variant 位。
export function generateUUID(source: RandomSource = defaultSource()!): string {
  const bytes = randomBytes(16, source)
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex = Array.from(bytes, b => b.toString(16).padStart(2, '0'))
  return `${hex[0]}${hex[1]}${hex[2]}${hex[3]}-${hex[4]}${hex[5]}-${hex[6]}${hex[7]}-${hex[8]}${hex[9]}-${hex[10]}${hex[11]}${hex[12]}${hex[13]}${hex[14]}${hex[15]}`
}