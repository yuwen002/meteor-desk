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
