import { describe, expect, it } from 'vitest'
import { diffText } from './textDiff'

describe('diffText', () => {
  it('keeps common lines and marks additions and removals', () => {
    expect(diffText('one\ntwo\nthree', 'one\nnext\nthree')).toEqual([
      { kind: 'equal', value: 'one' }, { kind: 'removed', value: 'two' }, { kind: 'added', value: 'next' }, { kind: 'equal', value: 'three' },
    ])
  })
  it('bounds pathological input', () => { expect(() => diffText('a\nb\nc', 'a', 3)).toThrow('最多比较') })
})

  it('can ignore whitespace differences while preserving original output', () => {
    expect(diffText('  service = api  ', 'service=api', 2000, true)).toEqual([
      { kind: 'equal', value: '  service = api  ' },
    ])
  })
