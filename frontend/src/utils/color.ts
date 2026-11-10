export interface RGB { r: number; g: number; b: number }
export interface HSL { h: number; s: number; l: number }

export function normalizeHex(input: string): string {
  let hex = input.trim().replace(/^#/, '').replace(/[^0-9a-fA-F]/g, '').toLowerCase()
  if (hex.length === 3) hex = hex.split('').map(c => c + c).join('')
  if (hex.length === 6) return '#' + hex
  if (hex.length === 8) return '#' + hex
  throw new Error('HEX 格式应为 #RGB、#RRGGBB 或 #RRGGBBAA。')
}

export function hexToRgb(hex: string): RGB {
  const normalized = normalizeHex(hex)
  const clean = normalized.slice(1, 7)
  return {
    r: parseInt(clean.slice(0, 2), 16),
    g: parseInt(clean.slice(2, 4), 16),
    b: parseInt(clean.slice(4, 6), 16),
  }
}

export function rgbToHex(r: number, g: number, b: number, alpha?: number): string {
  r = clampRound(r, 0, 255)
  g = clampRound(g, 0, 255)
  b = clampRound(b, 0, 255)
  let hex = '#'
  hex += r.toString(16).padStart(2, '0')
  hex += g.toString(16).padStart(2, '0')
  hex += b.toString(16).padStart(2, '0')
  if (alpha !== undefined) {
    const a = clampRound(alpha * 255, 0, 255)
    hex += a.toString(16).padStart(2, '0')
  }
  return hex
}

export function rgbToHsl(r: number, g: number, b: number): HSL {
  const rn = r / 255, gn = g / 255, bn = b / 255
  const max = Math.max(rn, gn, bn), min = Math.min(rn, gn, bn)
  let h = 0, s = 0
  const l = (max + min) / 2
  if (max !== min) {
    const d = max - min
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
    switch (max) {
      case rn: h = (gn - bn) / d + (gn < bn ? 6 : 0); break
      case gn: h = (bn - rn) / d + 2; break
      case bn: h = (rn - gn) / d + 4; break
    }
    h /= 6
  }
  return { h: Math.round(h * 360), s: Math.round(s * 100), l: Math.round(l * 100) }
}

export function hslToRgb(h: number, s: number, l: number): RGB {
  h = ((h % 360) + 360) % 360
  const sn = clamp(s, 0, 100) / 100
  const ln = clamp(l, 0, 100) / 100
  if (sn === 0) {
    const v = clampRound(ln * 255, 0, 255)
    return { r: v, g: v, b: v }
  }
  const q = ln < 0.5 ? ln * (1 + sn) : ln + sn - ln * sn
  const p = 2 * ln - q
  const hk = h / 360
  return {
    r: clampRound(hueToRgb(p, q, hk + 1 / 3) * 255, 0, 255),
    g: clampRound(hueToRgb(p, q, hk) * 255, 0, 255),
    b: clampRound(hueToRgb(p, q, hk - 1 / 3) * 255, 0, 255),
  }
}

function hueToRgb(p: number, q: number, t: number): number {
  if (t < 0) t += 1
  if (t > 1) t -= 1
  if (t < 1 / 6) return p + (q - p) * 6 * t
  if (t < 1 / 2) return q
  if (t < 2 / 3) return p + (q - p) * (2 / 3 - t) * 6
  return p
}

function clamp(v: number, lo: number, hi: number): number { return Math.min(hi, Math.max(lo, v)) }
function clampRound(v: number, lo: number, hi: number): number { return Math.round(clamp(v, lo, hi)) }

export function relativeLuminance(r: number, g: number, b: number): number {
  const toLinear = (c: number) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
  }
  return 0.2126 * toLinear(r) + 0.7152 * toLinear(g) + 0.0722 * toLinear(b)
}

export function contrastRatio(l1: number, l2: number): number {
  const lighter = Math.max(l1, l2)
  const darker = Math.min(l1, l2)
  return (lighter + 0.05) / (darker + 0.05)
}

export interface ContrastCheck {
  ratio: number
  wcagAA: boolean
  wcagAAA: boolean
  suggestion: string
}

export function checkContrast(foreground: RGB, background: RGB): ContrastCheck {
  const ratio = contrastRatio(relativeLuminance(foreground.r, foreground.g, foreground.b), relativeLuminance(background.r, background.g, background.b))
  const rounded = Math.round(ratio * 100) / 100
  const wcagAA = rounded >= 4.5
  const wcagAAA = rounded >= 7
  let suggestion: string
  if (wcagAAA) suggestion = '对比度非常好（WCAG AAA 通过）。'
  else if (wcagAA) suggestion = '对比度良好（WCAG AA 通过），大字标题可读。'
  else if (rounded >= 3) suggestion = '对比度偏低，小字可能难以辨认。'
  else suggestion = '对比度不足，建议调整前景或背景。'
  return { ratio: rounded, wcagAA, wcagAAA, suggestion }
}

export const tailwindPalette: Record<string, Record<number, string>> = {
  slate: { 50: '#f8fafc', 100: '#f1f5f9', 200: '#e2e8f0', 300: '#cbd5e1', 400: '#94a3b8', 500: '#64748b', 600: '#475569', 700: '#334155', 800: '#1e293b', 900: '#0f172a' },
  red: { 50: '#fef2f2', 100: '#fee2e2', 200: '#fecaca', 300: '#fca5a5', 400: '#f87171', 500: '#ef4444', 600: '#dc2626', 700: '#b91c1c', 800: '#991b1b', 900: '#7f1d1d' },
  amber: { 50: '#fffbeb', 100: '#fef3c7', 200: '#fde68a', 300: '#fcd34d', 400: '#fbbf24', 500: '#f59e0b', 600: '#d97706', 700: '#b45309', 800: '#92400e', 900: '#78350f' },
  emerald: { 50: '#ecfdf5', 100: '#d1fae5', 200: '#a7f3d0', 300: '#6ee7b7', 400: '#34d399', 500: '#10b981', 600: '#059669', 700: '#047857', 800: '#065f46', 900: '#064e3b' },
  sky: { 50: '#f0f9ff', 100: '#e0f2fe', 200: '#bae6fd', 300: '#7dd3fc', 400: '#38bdf8', 500: '#0ea5e9', 600: '#0284c7', 700: '#0369a1', 800: '#075985', 900: '#0c4a6e' },
  violet: { 50: '#f5f3ff', 100: '#ede9fe', 200: '#ddd6fe', 300: '#c4b5fd', 400: '#a78bfa', 500: '#8b5cf6', 600: '#7c3aed', 700: '#6d28d9', 800: '#5b21b6', 900: '#4c1d95' },
}