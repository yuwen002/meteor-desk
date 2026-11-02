import { describe, expect, it } from 'vitest'
import { parseRequestHeaders } from './httpRequest'

describe('parseRequestHeaders', () => {
  it('parses string header pairs and ignores surrounding whitespace', () => {
    expect(parseRequestHeaders(' {\n  "Accept": "application/json",\n  "X-Trace": "trace-1"\n} ')).toEqual({
      Accept: 'application/json',
      'X-Trace': 'trace-1',
    })
  })

  it('treats blank input as no custom headers', () => {
    expect(parseRequestHeaders('   ')).toEqual({})
  })

  it('rejects malformed JSON with a readable message and keeps the parser error as cause', () => {
    expect(() => parseRequestHeaders('{')).toThrow('请求头 JSON 无效')
    let cause: unknown
    try {
      parseRequestHeaders('{')
    } catch (error) {
      cause = (error as { cause?: unknown }).cause
    }
    // cause 里是 JSON.parse 抛出的原始异常，便于在控制台定位具体出错位置。
    expect(cause).toBeInstanceOf(Error)
  })

  it('rejects arrays, primitives and non-string values', () => {
    expect(() => parseRequestHeaders('["Accept"]')).toThrow('请求头必须是字符串键值对组成的 JSON 对象')
    expect(() => parseRequestHeaders('"Accept"')).toThrow('请求头必须是字符串键值对组成的 JSON 对象')
    expect(() => parseRequestHeaders('{"X-Retry": 3}')).toThrow('请求头必须是字符串键值对组成的 JSON 对象')
    expect(() => parseRequestHeaders('null')).toThrow('请求头必须是字符串键值对组成的 JSON 对象')
  })
})
