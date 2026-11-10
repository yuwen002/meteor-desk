import { describe, it, expect } from 'vitest'
import { normalizeHex, hexToRgb, rgbToHex, rgbToHsl, hslToRgb, checkContrast, relativeLuminance } from './color'

describe('color', () => {
  it('HEX 缩写展开与校验', () => {
    expect(normalizeHex('#3b82f6')).toBe('#3b82f6')
    expect(normalizeHex('3B82F6')).toBe('#3b82f6')
    expect(normalizeHex('#abc')).toBe('#aabbcc')
    expect(normalizeHex('#aabbccff')).toBe('#aabbccff')
    expect(() => normalizeHex('#xyz')).toThrow(/格式/)
  })

  it('HEX ↔ RGB 往返', () => {
    const rgb = hexToRgb('#3b82f6')
    expect(rgb).toEqual({ r: 59, g: 130, b: 246 })
    expect(rgbToHex(59, 130, 246)).toBe('#3b82f6')
  })

  it('RGB ↔ HSL 往返', () => {
    const hsl = rgbToHsl(59, 130, 246)
    const back = hslToRgb(hsl.h, hsl.s, hsl.l)
    expect(Math.abs(back.r - 59)).toBeLessThanOrEqual(1)
    expect(Math.abs(back.g - 130)).toBeLessThanOrEqual(1)
    expect(Math.abs(back.b - 246)).toBeLessThanOrEqual(1)
  })

  it('灰色调饱和度为 0', () => {
    const hsl = rgbToHsl(128, 128, 128)
    expect(hsl.s).toBe(0)
    expect(hsl.l).toBe(50)
  })

  it('对比度计算', () => {
    const white = { r: 255, g: 255, b: 255 }
    const black = { r: 0, g: 0, b: 0 }
    const r = checkContrast(white, black)
    expect(r.ratio).toBeGreaterThan(20)
    expect(r.wcagAAA).toBe(true)
  })

  it('输入超出范围自动 clamp', () => {
    expect(rgbToHex(-10, 300, 999)).toBe('#00ffff')
  })
})