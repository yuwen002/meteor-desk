import { describe, expect, it } from 'vitest'
import {
  groupMemberNames,
  groupMissingCount,
  isGroupValid,
  normalizeGroupPayload,
  toggleMember,
} from './processGroups'

describe('process group helpers', () => {
  it('toggles membership without mutating the source array', () => {
    const source = ['a', 'b']
    const added = toggleMember(source, 'c')
    expect(added).toEqual(['a', 'b', 'c'])
    const removed = toggleMember(source, 'a')
    expect(removed).toEqual(['b'])
    expect(source).toEqual(['a', 'b'])
  })

  it('trims fields and dedupes members keeping selection order', () => {
    const normalized = normalizeGroupPayload({ id: ' g1 ', name: '  联调  ', description: '  x  ', memberIds: ['a', 'b', 'a', '', 'b'] })
    expect(normalized).toEqual({ id: 'g1', name: '联调', description: 'x', memberIds: ['a', 'b'] })
  })

  it('requires a name and at least one member to be valid', () => {
    expect(isGroupValid({ id: '', name: '', description: '', memberIds: ['a'] })).toBe(false)
    expect(isGroupValid({ id: '', name: '场景', description: '', memberIds: [] })).toBe(false)
    expect(isGroupValid({ id: '', name: '场景', description: '', memberIds: ['a'] })).toBe(true)
  })

  it('resolves member ids to names and flags removed services', () => {
    const members = [{ id: 'a', name: '数据库' }, { id: 'b', name: 'API' }]
    expect(groupMemberNames(['a', 'ghost', 'b'], members)).toEqual(['数据库', '已删除服务', 'API'])
    expect(groupMissingCount(['a', 'ghost', 'b'], members)).toBe(1)
  })
})
