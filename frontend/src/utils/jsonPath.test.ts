import { describe, expect, it } from 'vitest'
import { appendJSONPath } from './jsonPath'

describe('appendJSONPath', () => {
  it('keeps identifier keys in readable dot notation', () => {
    expect(appendJSONPath('$', 'service')).toBe('service')
    expect(appendJSONPath('service', 'status')).toBe('service.status')
  })

  it('quotes keys containing dots, spaces, or brackets', () => {
    expect(appendJSONPath('$', 'service.name')).toBe('$["service.name"]')
    expect(appendJSONPath('payload', 'request id')).toBe('payload["request id"]')
    expect(appendJSONPath('payload', 'items[0]')).toBe('payload["items[0]"]')
  })

  it('preserves empty and Unicode keys without ambiguity', () => {
    expect(appendJSONPath('$', '')).toBe('$[""]')
    expect(appendJSONPath('payload', '用户')).toBe('payload["用户"]')
  })
})
