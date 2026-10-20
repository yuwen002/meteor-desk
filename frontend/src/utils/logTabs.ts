// Package-level frontend utility for keeping per-file log workspaces independent.
export type LogWorkspaceTab = {
  id: string
  title: string
  filePath: string
  query: string
  useRegex: boolean
  caseSensitive: boolean
  contextLines: number
  resultLimit: number
  encodingMode: string
  multiLineJSON: boolean
  timeRangeStart: string
  timeRangeEnd: string
  excludedTerms: string
  searchLevelFilter: 'ALL'|'INFO'|'WARN'|'ERROR'
}

export function createLogWorkspaceTab(id: string, filePath = ''): LogWorkspaceTab {
  const title = filePath.split(/[\\/]/).filter(Boolean).pop() || '新建日志'
  return {
    id, title, filePath, query: '', useRegex: false, caseSensitive: true,
    contextLines: 2, resultLimit: 500, encodingMode: 'auto', multiLineJSON: false,
    timeRangeStart: '', timeRangeEnd: '', excludedTerms: '', searchLevelFilter: 'ALL',
  }
}

export function normalizeLogWorkspaceTabs(tabs: LogWorkspaceTab[], activeId: string): { tabs: LogWorkspaceTab[]; activeId: string } {
  const unique: LogWorkspaceTab[] = []
  const seen = new Set<string>()
  for (const tab of tabs) {
    if (!tab.id || seen.has(tab.id)) continue
    seen.add(tab.id)
    unique.push({ ...tab })
  }
  if (!unique.length) {
    const first = createLogWorkspaceTab('log-1')
    return { tabs: [first], activeId: first.id }
  }
  return { tabs: unique, activeId: unique.some(tab => tab.id === activeId) ? activeId : unique[0].id }
}

export function closeLogWorkspaceTab(tabs: LogWorkspaceTab[], activeId: string, closingId: string): { tabs: LogWorkspaceTab[]; activeId: string } {
  if (tabs.length <= 1 || !tabs.some(tab => tab.id === closingId)) return { tabs: tabs.slice(), activeId }
  const index = tabs.findIndex(tab => tab.id === closingId)
  const remaining = tabs.filter(tab => tab.id !== closingId)
  const nextActive = activeId === closingId ? remaining[Math.min(index, remaining.length - 1)].id : activeId
  return normalizeLogWorkspaceTabs(remaining, nextActive)
}
