import { describe, expect, it } from 'vitest'
import { isSensitiveField, redactSensitiveText } from './redact'

describe('redactSensitiveText', () => {
  it('redacts JSON fields while leaving ordinary fields intact', () => {
    const result = redactSensitiveText('{"password":"s3cret","name":"meteor","apiKey":"abc"}')
    expect(result.text).toBe('{"password":"[REDACTED]","name":"meteor","apiKey":"[REDACTED]"}')
    expect(result.replacements).toBe(2)
  })
  it('redacts header, assignment and URL query values', () => {
    const result = redactSensitiveText('Authorization: Bearer abc\ntrace=ok token=xyz\nhttps://x.test/?session_id=123&mode=dev')
    expect(result.text).toContain('Authorization: [REDACTED]')
    expect(result.text).toContain('token=[REDACTED]')
    expect(result.text).toContain('session_id=%5BREDACTED%5D')
    expect(result.text).toContain('mode=dev')
  })
  it('recognizes common sensitive field spellings', () => {
    expect(isSensitiveField('DB_PASSWORD')).toBe(true)
    expect(isSensitiveField('x-api-key')).toBe(true)
    expect(isSensitiveField('requestId')).toBe(false)
  })
})


describe('custom redaction rules', () => {
  it('applies user-defined regular expressions and replacement text', () => {
    const result = redactSensitiveText('phone=13812345678 email=dev@example.com', '[REDACTED]', [
      { pattern: '\\b1[3-9]\\d{9}\\b', replacement: '[PHONE]' },
      { pattern: '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}', replacement: '[EMAIL]' },
    ])
    expect(result.text).toBe('phone=[PHONE] email=[EMAIL]')
    expect(result.replacements).toBe(2)
    expect(result.invalidRules).toBe(0)
  })

  it('skips invalid patterns without throwing and reports them', () => {
    const result = redactSensitiveText('keep this text', '[REDACTED]', [
      { pattern: '[', replacement: 'x' },
      { pattern: '', replacement: 'x' },
    ])
    expect(result.text).toBe('keep this text')
    expect(result.invalidRules).toBe(1)
    expect(result.replacements).toBe(0)
  })
})
