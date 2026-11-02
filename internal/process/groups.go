package process

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// groupsFileName 是与 processes.json 同目录的服务分组持久化文件名。
const groupsFileName = "groups.json"

// normalizeGroup 就地整理分组定义：去空白、按 ID 去重成员、校验名称非空。
// 只校验名称；成员是否存在由调用方在需要时判断，便于服务被删除后分组仍能安全启停其余成员。
func normalizeGroup(group *Group) error {
	group.ID = strings.TrimSpace(group.ID)
	group.Name = strings.TrimSpace(group.Name)
	group.Description = strings.TrimSpace(group.Description)
	if group.Name == "" {
		return errors.New("分组名称不能为空")
	}
	group.MemberIDs = uniqueIDs(group.MemberIDs)
	return nil
}

// cloneGroup 复制成员切片，避免调用方持有内部可变切片。
func cloneGroup(group Group) Group {
	group.MemberIDs = append([]string(nil), group.MemberIDs...)
	return group
}

// loadGroupsFromDisk 在构造期从 groupsPath 载入分组；文件缺失视为空而非错误。
// 此时尚未有其他并发访问，不需要持锁。
func (m *Manager) loadGroupsFromDisk() {
	data, err := os.ReadFile(m.groupsPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "读取服务分组失败，将忽略：%v\n", err)
		}
		return
	}
	var stored []Group
	if err := json.Unmarshal(data, &stored); err != nil {
		fmt.Fprintf(os.Stderr, "解析服务分组失败，将忽略：%v\n", err)
		return
	}
	for _, group := range stored {
		if group.ID == "" {
			continue
		}
		m.groups[group.ID] = cloneGroup(group)
	}
}

// persistGroupsLocked 把当前全部分组按 ID 排序后写入 groupsPath；调用方需持有 m.mu。
func (m *Manager) persistGroupsLocked() error {
	groups := make([]Group, 0, len(m.groups))
	for _, group := range m.groups {
		groups = append(groups, cloneGroup(group))
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	return writeJSONAtomic(m.groupsPath, groups)
}

// ListGroups 返回按名称排序的服务分组副本；仅返回引用关系，成员服务需另行查询。
func (m *Manager) ListGroups() []Group {
	m.mu.RLock()
	defer m.mu.RUnlock()
	groups := make([]Group, 0, len(m.groups))
	for _, group := range m.groups {
		groups = append(groups, cloneGroup(group))
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Name != groups[j].Name {
			return groups[i].Name < groups[j].Name
		}
		return groups[i].ID < groups[j].ID
	})
	return groups
}

// SaveGroup 校验并持久化一个服务分组。新建时分配 ID；更新时保留原 ID。
// 成员 ID 必须都是已配置的服务，避免留下悬空引用。
func (m *Manager) SaveGroup(group Group) (Group, error) {
	if err := normalizeGroup(&group); err != nil {
		return Group{}, err
	}
	if group.ID == "" {
		group.ID = newID()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, memberID := range group.MemberIDs {
		if _, exists := m.specs[memberID]; !exists {
			return Group{}, fmt.Errorf("分组包含不存在的服务：%s", memberID)
		}
	}
	// 同名分组（不同 ID）拒绝，避免场景列表出现难以区分的重复名称。
	for id, existing := range m.groups {
		if id != group.ID && existing.Name == group.Name {
			return Group{}, fmt.Errorf("已存在同名分组：%s", group.Name)
		}
	}
	previous, hadPrevious := m.groups[group.ID]
	m.groups[group.ID] = cloneGroup(group)
	if err := m.persistGroupsLocked(); err != nil {
		if hadPrevious {
			m.groups[group.ID] = previous
		} else {
			delete(m.groups, group.ID)
		}
		return Group{}, err
	}
	return cloneGroup(group), nil
}

// DeleteGroup 删除一个服务分组；只移除引用关系，不影响成员服务本身。
func (m *Manager) DeleteGroup(id string) error {
	id = strings.TrimSpace(id)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.groups[id]; !exists {
		return errors.New("未找到该分组")
	}
	delete(m.groups, id)
	return m.persistGroupsLocked()
}

// group 返回指定分组的成员 ID 副本，并过滤掉已被删除的服务；分组不存在时返回错误。
func (m *Manager) groupMemberIDs(id string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	group, exists := m.groups[id]
	if !exists {
		return nil, errors.New("未找到该分组")
	}
	members := make([]string, 0, len(group.MemberIDs))
	for _, memberID := range group.MemberIDs {
		if _, ok := m.specs[memberID]; ok {
			members = append(members, memberID)
		}
	}
	return members, nil
}

// StartGroup 按依赖顺序启动分组内的全部服务，并复用批量启动的上游依赖补全能力。
// 分组为空（或成员均已被删除）时返回空结果而非错误。
func (m *Manager) StartGroup(id string) ([]BatchResult, error) {
	members, err := m.groupMemberIDs(id)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []BatchResult{}, nil
	}
	return m.StartBatch(members)
}

// StopGroup 按逆依赖顺序停止分组内的全部服务及其依赖方，复用批量停止逻辑。
func (m *Manager) StopGroup(id string) ([]BatchResult, error) {
	members, err := m.groupMemberIDs(id)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []BatchResult{}, nil
	}
	return m.StopBatch(members)
}

// purgeGroupMembershipLocked 在删除服务时把它从所有分组成员中移除，并在有变更时落盘；调用方需持有 m.mu。
// 返回的 error 仅反映落盘结果，调用方通常在其后统一处理。
func (m *Manager) purgeGroupMembershipLocked(removedID string) error {
	changed := false
	for id, group := range m.groups {
		filtered := make([]string, 0, len(group.MemberIDs))
		for _, memberID := range group.MemberIDs {
			if memberID != removedID {
				filtered = append(filtered, memberID)
			}
		}
		if len(filtered) != len(group.MemberIDs) {
			group.MemberIDs = filtered
			m.groups[id] = group
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return m.persistGroupsLocked()
}
