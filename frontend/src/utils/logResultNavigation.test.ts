import { describe, expect, it } from 'vitest'
import { resolveResultJumpIndex } from './logResultNavigation'

describe('resolveResultJumpIndex', () => {
  it('converts valid 1-based indexes and supports the final item', () => {
    expect(resolveResultJumpIndex('1', 500)).toBe(0)
    expect(resolveResultJumpIndex('500', 500)).toBe(499)
  })

  it.each(['', '0', '-1', '1.5', '501', 'abc'])('rejects invalid input %j', (value) => {
    expect(resolveResultJumpIndex(value, 500)).toBeNull()
  })

  it('rejects jumps when there are no results', () => {
    expect(resolveResultJumpIndex('1', 0)).toBeNull()
  })
})
