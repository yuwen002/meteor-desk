// 服务分组（场景）的表单纯函数：成员增删、载荷整理与成员名称回填。
// 分组只按 ID 引用服务，这里负责把界面选择转换成后端 Group 结构并做基础清理。

export interface GroupMemberLike {
  id: string
  name: string
}

export interface GroupPayloadLike {
  id: string
  name: string
  description: string
  memberIds: string[]
}

// toggleMember 在成员列表中增删指定 ID，返回新数组，不修改入参。
export function toggleMember(memberIds: string[], id: string): string[] {
  if (memberIds.includes(id)) return memberIds.filter(item => item !== id)
  return [...memberIds, id]
}

// normalizeGroupPayload 去掉名称与描述的空白、按 ID 去重成员并保持选择顺序。
export function normalizeGroupPayload(payload: GroupPayloadLike): GroupPayloadLike {
  const seen = new Set<string>()
  const memberIds: string[] = []
  for (const id of payload.memberIds) {
    if (id && !seen.has(id)) {
      seen.add(id)
      memberIds.push(id)
    }
  }
  return {
    id: payload.id.trim(),
    name: payload.name.trim(),
    description: payload.description.trim(),
    memberIds,
  }
}

// isGroupValid 判断分组是否可提交：名称非空且至少选择一个成员。
export function isGroupValid(payload: GroupPayloadLike): boolean {
  const normalized = normalizeGroupPayload(payload)
  return normalized.name !== '' && normalized.memberIds.length > 0
}

// groupMemberNames 把成员 ID 解析成服务名称；找不到服务的（已删除成员）标记为「已删除服务」，
// 与后端“整组启停自动跳过失效成员”的行为保持一致的提示。
export function groupMemberNames(memberIds: string[], members: GroupMemberLike[]): string[] {
  const nameById = new Map(members.map(member => [member.id, member.name]))
  return memberIds.map(id => nameById.get(id) ?? '已删除服务')
}

// groupMissingCount 统计分组里已失效（不在服务列表中）的成员数量，用于展示提醒。
export function groupMissingCount(memberIds: string[], members: GroupMemberLike[]): number {
  const ids = new Set(members.map(member => member.id))
  return memberIds.filter(id => !ids.has(id)).length
}
