import { describe, expect, it } from 'vitest'
import { bytesToHex, digestText, hexToBytes, hmacText, type HashEngine } from './hash'

// 底层 Web Crypto 引擎在测试环境不可靠，用可控假引擎验证纯逻辑：输入与算法转发正确、
// 十六进制编解码往返一致、异常输入被拒绝。
function stubHexEngine(hexResult: string): HashEngine {
  const bytes = hexToBytes(hexResult)
  const buf = bytes.buffer as ArrayBuffer
  return {
    digest: async (_algo: string, data: BufferSource) => {
      void new Uint8Array(data as ArrayBuffer)
      return buf
    },
    importKey: async () => ({}),
    sign: async () => buf,
    getRandomValues: (array) => array,
  }
}

describe('hash utils', () => {
  it('bytesToHex formats a byte array as lowercase hex', () => {
    expect(bytesToHex(new Uint8Array([0xde, 0xad, 0xbe, 0xef]))).toBe('deadbeef')
    expect(bytesToHex(new Uint8Array([0x00, 0xff, 0x10]))).toBe('00ff10')
  })

  it('hexToBytes accepts hex and round-trips with bytesToHex', () => {
    expect(bytesToHex(hexToBytes('aabbcc'))).toBe('aabbcc')
    expect(bytesToHex(hexToBytes('00 FF 10'))).toBe('00ff10')
  })

  it('hexToBytes rejects invalid characters and odd length', () => {
    expect(() => hexToBytes('xyz')).toThrow(/只能包含 0-9 与 a-f/i)
    expect(() => hexToBytes('abc')).toThrow(/长度必须为偶数/i)
  })

  it('digestText forwards bytes and returns hex from the engine', async () => {
    const engine = stubHexEngine('0f0f')
    expect(await digestText('hello', 'SHA-256', engine)).toBe('0f0f')
  })

  it('hmacText forwards bytes and returns hex from the engine', async () => {
    const engine = stubHexEngine('1234abcd')
    expect(await hmacText('body', 'secret', 'SHA-256', engine)).toBe('1234abcd')
  })

  it('digestText throws when no crypto engine is available', async () => {
    await expect(digestText('x', 'SHA-256', null)).rejects.toThrow(/不支持 Web Crypto/i)
  })
})