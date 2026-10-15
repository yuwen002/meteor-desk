export type ShortcutAction = 'openFile' | 'focusSearch' | 'runSearch' | 'logs' | 'processes' | 'toolkit'
export type ShortcutMap = Record<ShortcutAction, string>

export const DEFAULT_SHORTCUTS: ShortcutMap = {
  openFile: 'o',
  focusSearch: 'k',
  runSearch: 'Enter',
  logs: '1',
  processes: '2',
  toolkit: '3',
}

export const SHORTCUT_OPTIONS = [
  { value: 'o', label: 'Ctrl/⌘ + O' },
  { value: 'k', label: 'Ctrl/⌘ + K' },
  { value: 'Enter', label: 'Ctrl/⌘ + Enter' },
  { value: '1', label: 'Ctrl/⌘ + 1' },
  { value: '2', label: 'Ctrl/⌘ + 2' },
  { value: '3', label: 'Ctrl/⌘ + 3' },
] as const

const ACTIONS = Object.keys(DEFAULT_SHORTCUTS) as ShortcutAction[]
const ALLOWED_KEYS = new Set<string>(SHORTCUT_OPTIONS.map(option => option.value))

export function parseShortcutSettings(raw: string | null): ShortcutMap {
  if (!raw) return { ...DEFAULT_SHORTCUTS }
  try {
    const value: unknown = JSON.parse(raw)
    if (!value || typeof value !== 'object' || Array.isArray(value)) return { ...DEFAULT_SHORTCUTS }
    const candidate = value as Partial<ShortcutMap>
    if (!ACTIONS.every(action => typeof candidate[action] === 'string' && ALLOWED_KEYS.has(candidate[action]!))) {
      return { ...DEFAULT_SHORTCUTS }
    }
    const result = Object.fromEntries(ACTIONS.map(action => [action, candidate[action]])) as ShortcutMap
    return validateShortcutMap(result) ? { ...DEFAULT_SHORTCUTS } : result
  } catch {
    return { ...DEFAULT_SHORTCUTS }
  }
}

export function validateShortcutMap(shortcuts: ShortcutMap): string {
  const values = ACTIONS.map(action => shortcuts[action])
  return new Set(values).size === values.length
    ? ''
    : '快捷键不能重复，请为每个操作选择不同的组合键。'
}

export function resolveShortcutAction(shortcuts: ShortcutMap, key: string): ShortcutAction | undefined {
  const normalized = key === 'Enter' ? 'Enter' : key.toLowerCase()
  return ACTIONS.find(action => shortcuts[action] === normalized)
}
