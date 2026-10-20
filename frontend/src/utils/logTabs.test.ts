import { describe, expect, it } from 'vitest'
import { closeLogWorkspaceTab, createLogWorkspaceTab, normalizeLogWorkspaceTabs } from './logTabs'

describe('log workspace tabs', () => {
  it('creates a tab with stable defaults and a filename title', () => {
    const tab = createLogWorkspaceTab('tab-1', '/var/log/service.log')
    expect(tab.title).toBe('service.log')
    expect(tab.filePath).toBe('/var/log/service.log')
    expect(tab.caseSensitive).toBe(true)
    expect(tab.searchLevelFilter).toBe('ALL')
  })

  it('removes duplicate IDs and restores an invalid active ID', () => {
    const first = createLogWorkspaceTab('tab-1', 'one.log')
    const duplicate = createLogWorkspaceTab('tab-1', 'duplicate.log')
    const second = createLogWorkspaceTab('tab-2', 'two.log')
    const normalized = normalizeLogWorkspaceTabs([first, duplicate, second], 'missing')
    expect(normalized.tabs.map(tab => tab.filePath)).toEqual(['one.log', 'two.log'])
    expect(normalized.activeId).toBe('tab-1')
  })

  it('keeps at least one tab and selects a neighbor when closing the active tab', () => {
    const only = createLogWorkspaceTab('only')
    expect(closeLogWorkspaceTab([only], 'only', 'only')).toEqual({ tabs: [only], activeId: 'only' })
    const tabs = [createLogWorkspaceTab('a'), createLogWorkspaceTab('b'), createLogWorkspaceTab('c')]
    const closed = closeLogWorkspaceTab(tabs, 'b', 'b')
    expect(closed.tabs.map(tab => tab.id)).toEqual(['a', 'c'])
    expect(closed.activeId).toBe('c')
  })
})
