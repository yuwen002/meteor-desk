import { describe, expect, it } from 'vitest'
import {
  DEFAULT_SHORTCUTS,
  parseShortcutSettings,
  resolveShortcutAction,
  validateShortcutMap,
} from './shortcuts'

describe('shortcut settings', () => {
  it('uses the defaults when no saved settings exist', () => {
    expect(parseShortcutSettings(null)).toEqual(DEFAULT_SHORTCUTS)
  })

  it('restores defaults for malformed or unsupported saved settings', () => {
    expect(parseShortcutSettings('{bad json')).toEqual(DEFAULT_SHORTCUTS)
    expect(parseShortcutSettings(JSON.stringify({ ...DEFAULT_SHORTCUTS, openFile: 'z' }))).toEqual(DEFAULT_SHORTCUTS)
  })

  it('restores defaults when saved shortcuts conflict', () => {
    expect(parseShortcutSettings(JSON.stringify({ ...DEFAULT_SHORTCUTS, openFile: 'k' }))).toEqual(DEFAULT_SHORTCUTS)
  })

  it('rejects duplicate shortcuts before saving', () => {
    expect(validateShortcutMap({ ...DEFAULT_SHORTCUTS, openFile: 'k' })).toContain('不能重复')
    expect(validateShortcutMap(DEFAULT_SHORTCUTS)).toBe('')
  })

  it('resolves letters case-insensitively and preserves Enter', () => {
    expect(resolveShortcutAction(DEFAULT_SHORTCUTS, 'O')).toBe('openFile')
    expect(resolveShortcutAction(DEFAULT_SHORTCUTS, 'Enter')).toBe('runSearch')
    expect(resolveShortcutAction(DEFAULT_SHORTCUTS, 'x')).toBeUndefined()
  })
})
