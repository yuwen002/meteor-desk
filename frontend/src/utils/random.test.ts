import { describe, expect, it, vi } from 'vitest'
import { charPool, generateRandomString, generateUUID, type RandomOptions, type RandomSource } from './random'

function bytesSource(hexSeed: string): RandomSource {
  const values = hexSeed.match(/.{1,2}/g)?.map(pair => parseInt(pair, 16)) ?? []
  return (array: Uint8Array) => {
    for (let i = 0; i < array.length; i++) array[i] = values[i % values.length]
    return array
  }
}

describe('random generator', () => {
  it('charPool only includes requested character classes', () => {
    const pool = charPool({ length: 8, lowercase: true, uppercase: true, digits: false, symbols: false })
    expect(pool).toMatch(/^[a-z]+[A-Z]+$/)
    expect(pool).not.toContain('0')
  })

  it('returns empty string when no character class is selected', () => {
    const options: RandomOptions = { length: 8, lowercase: false, uppercase: false, digits: false, symbols: false }
    const source = bytesSource('00')
    expect(generateRandomString(options, source)).toBe('')
  })

  it('generates a string matching the requested length and pool', () => {
    const options: RandomOptions = { length: 12, lowercase: true, uppercase: true, digits: true, symbols: false }
    const out = generateRandomString(options, bytesSource('a1b2'))
    expect(out).toHaveLength(12)
    expect(out).toMatch(new RegExp(`^[${charPool(options)}]{12}$`))
  })

  it('produces UUID v4 with fixed version and variant bits', () => {
    const uuid = generateUUID(bytesSource('0123456789abcdef'))
    expect(uuid).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
  })

  it('does not fall back to Math.random', () => {
    const spy = vi.spyOn(Math, 'random')
    generateRandomString({ length: 16, lowercase: true, uppercase: true, digits: true, symbols: false }, bytesSource('a5'))
    expect(spy).not.toHaveBeenCalled()
    spy.mockRestore()
  })
})