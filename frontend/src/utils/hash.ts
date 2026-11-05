// 哈希 / HMAC 计算工具：基于 Web Crypto API（crypto.subtle），全部在本机完成。
// 纯函数在 Node/jsdom 测试环境没有 crypto.subtle，因此把底层引擎注入为参数，
// 便于单元测试替换为可控实现，也方便将来复用同一套逻辑给文件摘要等入口。

export type HashAlgorithm = 'MD5' | 'SHA-1' | 'SHA-256' | 'SHA-384' | 'SHA-512'

export interface HashEngine {
  digest(algorithm: string, data: BufferSource): Promise<ArrayBuffer>
  importKey(format: 'raw', keyData: BufferSource, algorithm: string | { name: string; hash?: { name: string } }, extractable: boolean, keyUsages: string[]): Promise<unknown>
  sign(algorithm: { name: string; hash: { name: string } }, key: unknown, data: BufferSource): Promise<ArrayBuffer>
  getRandomValues<T extends ArrayBufferView | null>(array: T): T
}

// MD5 不在 Web Crypto 标准里，这里显式标注：只列出浏览器原生支持的算法。
export const hashAlgorithms: HashAlgorithm[] = ['SHA-1', 'SHA-256', 'SHA-384', 'SHA-512']

function defaultEngine(): HashEngine | null {
  const subtle = (globalThis.crypto as globalThis.Crypto | undefined)?.subtle as unknown as
    | Pick<HashEngine, 'digest' | 'importKey' | 'sign'>
    | undefined
  if (!subtle) return null
  return subtle as HashEngine
}

export function bytesToHex(bytes: Uint8Array): string {
  // 每字节两个十六进制字符；分块拼接避免超大输入时拼接 O(n²) 的性能问题。
  let hex = ''
  for (let offset = 0; offset < bytes.length; offset += 0x4000) {
    const chunk = bytes.subarray(offset, Math.min(offset + 0x4000, bytes.length))
    hex += Array.from(chunk, byte => byte.toString(16).padStart(2, '0')).join('')
  }
  return hex
}

export function hexToBytes(hexString: string): Uint8Array {
  const clean = hexString.replace(/\s+/g, '')
  if (!/^[0-9a-fA-F]*$/.test(clean)) throw new Error('十六进制字符串只能包含 0-9 与 a-f。')
  if (clean.length % 2 !== 0) throw new Error('十六进制字符串长度必须为偶数。')
  const bytes = new Uint8Array(clean.length / 2)
  for (let i = 0; i < bytes.length; i++) bytes[i] = parseInt(clean.slice(i * 2, i * 2 + 2), 16)
  return bytes
}

function encodeText(text: string): Uint8Array<ArrayBuffer> {
  return new TextEncoder().encode(text) as Uint8Array<ArrayBuffer>
}

export async function digestText(text: string, algorithm: HashAlgorithm, engine: HashEngine | null = defaultEngine()): Promise<string> {
  if (!engine) throw new Error('当前环境不支持 Web Crypto，无法计算哈希。')
  const buffer = await engine.digest(algorithm, encodeText(text).buffer)
  return bytesToHex(new Uint8Array(buffer as ArrayBuffer))
}

export async function hmacText(text: string, secretText: string, algorithm: 'SHA-256' | 'SHA-384' | 'SHA-512', engine: HashEngine | null = defaultEngine()): Promise<string> {
  if (!engine) throw new Error('当前环境不支持 Web Crypto，无法计算 HMAC。')
  const key = await engine.importKey('raw', encodeText(secretText), { name: 'HMAC', hash: { name: algorithm } }, false, ['sign'])
  const buffer = await engine.sign({ name: 'HMAC', hash: { name: algorithm } }, key, encodeText(text).buffer)
  return bytesToHex(new Uint8Array(buffer as ArrayBuffer))
}