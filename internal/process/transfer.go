package process

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// 导入配置的体积与服务数量上限，防止一次导入撑爆内存与配置文件。
const maxConfigImportBytes = 1 << 20
const maxServiceConfigs = 500

// ExportConfig serializes service definitions. Sensitive environment values
// are blanked when redaction is requested; the keys are retained for editing.
func (m *Manager) ExportConfig(redactSensitive bool) ([]byte, error) {
	m.mu.RLock()
	specs := make([]Spec, 0, len(m.specs))
	for _, spec := range m.specs {
		specs = append(specs, cloneSpec(spec))
	}
	m.mu.RUnlock()
	sort.Slice(specs, func(i, j int) bool { return specs[i].ID < specs[j].ID })
	if redactSensitive {
		for i := range specs {
			for key := range specs[i].Env {
				if isSensitiveEnvKey(key) {
					specs[i].Env[key] = ""
				}
			}
		}
	}
	return json.MarshalIndent(specs, "", "  ")
}

// ImportConfig merges definitions as new, stopped services. IDs are regenerated
// and dependency references within the import file are rewritten.
func (m *Manager) ImportConfig(data []byte) ([]View, error) {
	if len(data) == 0 {
		return nil, errors.New("导入文件为空")
	}
	if len(data) > maxConfigImportBytes {
		return nil, fmt.Errorf("导入文件过大，最大支持 %d KiB", maxConfigImportBytes/1024)
	}
	var incoming []Spec
	if err := json.Unmarshal(data, &incoming); err != nil {
		return nil, fmt.Errorf("解析服务配置失败：%w", err)
	}
	if len(incoming) > maxServiceConfigs {
		return nil, fmt.Errorf("单次最多导入 %d 个服务配置", maxServiceConfigs)
	}
	if len(incoming) == 0 {
		return []View{}, nil
	}
	sourceIDs := make(map[string]struct{}, len(incoming))
	for i := range incoming {
		incoming[i].ID = strings.TrimSpace(incoming[i].ID)
		if incoming[i].ID == "" {
			return nil, fmt.Errorf("第 %d 个服务缺少配置 ID", i+1)
		}
		if _, exists := sourceIDs[incoming[i].ID]; exists {
			return nil, fmt.Errorf("导入文件中存在重复服务 ID：%s", incoming[i].ID)
		}
		sourceIDs[incoming[i].ID] = struct{}{}
		if err := normalizeSpec(&incoming[i]); err != nil {
			return nil, fmt.Errorf("服务“%s”配置无效：%w", incoming[i].Name, err)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.specs)+len(incoming) > maxServiceConfigs {
		return nil, fmt.Errorf("导入后服务配置总数不能超过 %d 个", maxServiceConfigs)
	}
	idMap := make(map[string]string, len(incoming))
	for _, spec := range incoming {
		idMap[spec.ID] = newID()
	}
	for i := range incoming {
		sourceID := incoming[i].ID
		incoming[i].ID = idMap[sourceID]
		for j, dependencyID := range incoming[i].DependsOn {
			if mappedID, exists := idMap[dependencyID]; exists {
				incoming[i].DependsOn[j] = mappedID
				continue
			}
			if _, exists := m.specs[dependencyID]; !exists {
				return nil, fmt.Errorf("服务“%s”依赖的服务 ID 不存在：%s", incoming[i].Name, dependencyID)
			}
		}
	}
	importedIDs := make([]string, 0, len(incoming))
	batchIDs := make(map[string]struct{}, len(incoming))
	for _, spec := range incoming {
		batchIDs[spec.ID] = struct{}{}
		importedIDs = append(importedIDs, spec.ID)
	}
	for _, spec := range incoming {
		for _, dependencyID := range spec.DependsOn {
			if dependencyID == spec.ID {
				return nil, fmt.Errorf("服务“%s”不能依赖自身", spec.Name)
			}
			if _, exists := m.specs[dependencyID]; !exists {
				if _, inBatch := batchIDs[dependencyID]; !inBatch {
					return nil, fmt.Errorf("服务“%s”依赖的服务不存在：%s", spec.Name, dependencyID)
				}
			}
		}
	}
	for _, spec := range incoming {
		m.specs[spec.ID] = cloneSpec(spec)
	}
	rollback := func() {
		for _, id := range importedIDs {
			delete(m.specs, id)
		}
	}
	if _, err := m.dependencyOrderLocked(importedIDs, true); err != nil {
		rollback()
		return nil, err
	}
	if err := m.persistLocked(); err != nil {
		rollback()
		return nil, err
	}
	views := make([]View, 0, len(incoming))
	for _, spec := range incoming {
		views = append(views, View{Spec: cloneSpec(spec), Status: Snapshot{ID: spec.ID, Name: spec.Name, State: StateStopped}})
	}
	return views, nil
}
