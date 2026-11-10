import { describe, it, expect } from 'vitest'
import { parseNumber, formatBase, allBases, extractBitRange, constantsForWidth, bitWidths } from './numberBase'

describe('numberBase', () => {
  it('十进制整数在所有基之间往返一致', () => {
    const rep = parseNumber('42', 10, 32, true)
    const bases = allBases(rep, true)
    expect(bases[2]).toBe('101010')
    expect(bases[8]).toBe('52')
    expect(bases[10]).toBe('42')
    expect(bases[16]).toBe('2A')
  })

  it('十六进制输入解析正确', () => {
    const rep = parseNumber('FF', 16, 8, false)
    expect(rep.value).toBe(255n)
    const bases = allBases(rep, false)
    expect(bases[10]).toBe('255')
    expect(bases[2]).toBe('11111111')
  })

  it('负数有符号表示正确', () => {
    const rep = parseNumber('-1', 10, 8, true)
    expect(rep.isNegative).toBe(true)
    const bases = allBases(rep, true)
    expect(bases[16]).toBe('FF')
    expect(bases[2]).toBe('11111111')
  })

  it('位段提取从高位到低位', () => {
    const rep = parseNumber('0xABCD', 16, 16, false)
    const topNibble = extractBitRange(rep, 15, 12)
    expect(topNibble.decimal).toBe('10')
    expect(topNibble.binary).toBe('1010')
    const lowByte = extractBitRange(rep, 7, 0)
    expect(lowByte.decimal).toBe('205')
  })

  it('越界输入抛明确错误', () => {
    expect(() => parseNumber('256', 10, 8, false)).toThrow(/超出上限/)
    expect(() => parseNumber('300', 10, 8, true)).toThrow(/超出上限/)
    expect(() => parseNumber('GG', 16, 8, false)).toThrow(/只能包含/)
    expect(() => extractBitRange(parseNumber('255', 10, 8, false), 9, 0)).toThrow(/位范围/)
  })

  it('常量表按位宽生成', () => {
    const c8 = constantsForWidth(8)
    expect(c8).toContainEqual(expect.objectContaining({ name: '全 1 掩码', decimal: '255' }))
    expect(c8).toContainEqual(expect.objectContaining({ name: '有符号最大值', decimal: '127' }))
    expect(constantsForWidth(64).length).toBeGreaterThanOrEqual(4)
    expect(bitWidths).toEqual([8, 16, 32, 64])
  })

  it('零与前导零处理', () => {
    const rep = parseNumber('0000', 10, 32, false)
    expect(rep.value).toBe(0n)
    const bases = allBases(rep, false)
    expect(bases[2]).toBe('0')
    expect(bases[16]).toBe('0')
  })
})