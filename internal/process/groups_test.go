package process

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitRunning 轮询直到服务进入运行态，超时则失败。
func waitRunning(t *testing.T, manager *Manager, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, view := range manager.List() {
			if view.Spec.ID == id && view.Status.State == StateRunning {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("等待服务 %s 运行超时", id)
}

// TestGroupSaveListAndReload 验证分组可保存、列举，并在重载后仍能载入。
func TestGroupSaveListAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "processes.json")
	manager, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := manager.Save(Spec{Name: "后端", Executable: os.Args[0], Args: []string{"-test.run=TestProcessHelper"}})
	if err != nil {
		t.Fatal(err)
	}
	frontend, err := manager.Save(Spec{Name: "前端", Executable: os.Args[0], Args: []string{"-test.run=TestProcessHelper"}})
	if err != nil {
		t.Fatal(err)
	}

	saved, err := manager.SaveGroup(Group{Name: "本地联调", Description: "前后端一起起", MemberIDs: []string{backend.Spec.ID, frontend.Spec.ID, backend.Spec.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" {
		t.Fatal("新建分组应分配 ID")
	}
	// 成员应按 ID 去重。
	if len(saved.MemberIDs) != 2 {
		t.Fatalf("成员应去重为 2 个：%+v", saved.MemberIDs)
	}
	groups := manager.ListGroups()
	if len(groups) != 1 || groups[0].Name != "本地联调" {
		t.Fatalf("分组列表不正确：%+v", groups)
	}

	reloaded, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	reloadedGroups := reloaded.ListGroups()
	if len(reloadedGroups) != 1 || len(reloadedGroups[0].MemberIDs) != 2 {
		t.Fatalf("重载后分组未正确载入：%+v", reloadedGroups)
	}
}

// TestGroupValidationRejectsBadInput 验证空名称、悬空成员、同名分组都被拒绝。
func TestGroupValidationRejectsBadInput(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	existing, err := manager.Save(Spec{Name: "已存在服务", Executable: os.Args[0]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SaveGroup(Group{Name: "   ", MemberIDs: []string{existing.Spec.ID}}); err == nil {
		t.Fatal("空名称分组应被拒绝")
	}
	if _, err := manager.SaveGroup(Group{Name: "含悬空成员", MemberIDs: []string{"missing-id"}}); err == nil {
		t.Fatal("引用不存在服务的分组应被拒绝")
	}
	if _, err := manager.SaveGroup(Group{Name: "工作区", MemberIDs: []string{existing.Spec.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SaveGroup(Group{Name: "工作区", MemberIDs: []string{existing.Spec.ID}}); err == nil {
		t.Fatal("同名分组应被拒绝")
	}
}

// TestStartAndStopGroupHonorsDependencyOrder 验证整组按依赖顺序启动、逆序停止。
func TestStartAndStopGroupHonorsDependencyOrder(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	// 依赖与被依赖服务都用持续运行的 helper，确保启动后处于 running。
	dep, err := manager.Save(runLoopSpec("依赖服务"))
	if err != nil {
		t.Fatal(err)
	}
	dependent, err := manager.Save(Spec{
		Name:       "下游服务",
		Executable: os.Args[0],
		Args:       []string{"-test.run=TestProcessHelper"},
		Env:        map[string]string{"METEORDESK_PROCESS_HELPER": "1"},
		DependsOn:  []string{dep.Spec.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	group, err := manager.SaveGroup(Group{Name: "联调场景", MemberIDs: []string{dep.Spec.ID, dependent.Spec.ID}})
	if err != nil {
		t.Fatal(err)
	}

	results, err := manager.StartGroup(group.ID)
	if err != nil {
		t.Fatal(err)
	}
	// StartBatch 递归包含上游依赖，顺序应为依赖服务在前、下游服务在后。
	if len(results) != 2 {
		t.Fatalf("整组启动应包含上游依赖共 2 项：%+v", results)
	}
	if results[0].ID != dep.Spec.ID || results[1].ID != dependent.Spec.ID {
		t.Fatalf("启动顺序未按依赖优先：%+v", results)
	}
	for _, result := range results {
		if !result.Success {
			t.Fatalf("启动 %s 失败：%s", result.ID, result.Message)
		}
	}
	waitRunning(t, manager, dep.Spec.ID)
	waitRunning(t, manager, dependent.Spec.ID)

	if _, err := manager.StopGroup(group.ID); err != nil {
		t.Fatal(err)
	}
	for _, view := range manager.List() {
		if view.Status.State == StateRunning || view.Status.State == StateStarting {
			t.Fatalf("停止分组后仍有服务在运行：%+v", view.Status)
		}
	}
}

// TestDeleteServicePurgesGroupMembership 验证删除服务后其从分组中移除并落盘。
func TestDeleteServicePurgesGroupMembership(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "processes.json")
	manager, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.Save(Spec{Name: "成员一", Executable: os.Args[0]})
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Save(Spec{Name: "成员二", Executable: os.Args[0]})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.SaveGroup(Group{Name: "两成员", MemberIDs: []string{first.Spec.ID, second.Spec.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Delete(first.Spec.ID); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewManagerAt(path)
	if err != nil {
		t.Fatal(err)
	}
	groups := reloaded.ListGroups()
	if len(groups) != 1 {
		t.Fatalf("删除服务不应删除分组本身：%+v", groups)
	}
	if len(groups[0].MemberIDs) != 1 || groups[0].MemberIDs[0] != second.Spec.ID {
		t.Fatalf("被删除服务应从分组中移除：%+v", groups[0].MemberIDs)
	}
}

// TestStartGroupSkipsDeletedMembers 验证分组内成员被删除后，整组启动只作用于现存服务且不报错。
func TestStartGroupSkipsDeletedMembers(t *testing.T) {
	manager, err := NewManagerAt(filepath.Join(t.TempDir(), "processes.json"))
	if err != nil {
		t.Fatal(err)
	}
	keep, err := manager.Save(runLoopSpec("保留成员"))
	if err != nil {
		t.Fatal(err)
	}
	drop, err := manager.Save(Spec{Name: "将被删除", Executable: os.Args[0]})
	if err != nil {
		t.Fatal(err)
	}
	group, err := manager.SaveGroup(Group{Name: "含悬空成员", MemberIDs: []string{keep.Spec.ID, drop.Spec.ID}})
	if err != nil {
		t.Fatal(err)
	}
	// 删除其中一个成员服务，验证分组仅保留现存服务。
	if err := manager.Delete(drop.Spec.ID); err != nil {
		t.Fatal(err)
	}
	// 再向内存中的分组注入一个悬空 ID，模拟历史数据里残留的失效引用。
	manager.mu.Lock()
	g := manager.groups[group.ID]
	g.MemberIDs = append(g.MemberIDs, "ghost-id")
	manager.groups[group.ID] = g
	manager.mu.Unlock()

	results, err := manager.StartGroup(group.ID)
	if err != nil {
		t.Fatalf("存在悬空成员时整组启动不应报错：%v", err)
	}
	if len(results) != 1 || results[0].ID != keep.Spec.ID {
		t.Fatalf("应只启动现存成员：%+v", results)
	}
	_, _ = manager.StopGroup(group.ID)
}
