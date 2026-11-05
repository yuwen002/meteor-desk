import { describe, expect, it } from 'vitest'
import { compilePattern, testRegex } from './regexTester'

describe('regex tester', () => {
  it('returns a readable error for invalid patterns', () => {
    const compiled = compilePattern('(', '')
    expect('error' in compiled).toBe(true)
  })

  it('returns an error for empty pattern', () => {
    const result = testRegex('', '', 'hello')
    expect(result.error).toContain('请输入正则表达式')
    expect(result.matchCount).toBe(0)
  })

  it('counts every global match with positions and groups', () => {
    const result = testRegex('aa', '', 'aa bb aa')
    expect(result.error).toBe('')
    expect(result.matchCount).toBe(2)
    expect(result.hits.map(h => h.text)).toEqual(['aa', 'aa'])
    expect(result.hits[0].start).toBe(0)
  })

  it('normalizes flags by forcing the global flag', () => {
    const compiled = compilePattern('ab', 'i')
    if ('error' in compiled) throw new Error('expected valid')
    expect(compiled.regex.global).toBe(true)
    expect(compiled.regex.ignoreCase).toBe(true)
  })

  it('handles zero-length matches without an infinite loop', () => {
    const result = testRegex('x*', '', 'abc')
    expect(result.error).toBe('')
    expect(result.matchCount).toBe(4)
  })
})