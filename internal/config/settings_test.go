package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSettingsRoundTripAndLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	settings := DefaultSettings()
	settings.ContextLines = 99
	settings.ResultLimit = 8000
	for i := 0; i < 20; i++ {
		AddRecentFile(&settings, filepath.Join("logs", string(rune('a'+i))+".log"))
	}
	AddSearchRecord(&settings, SearchRecord{Query: "timeout", Path: "/tmp/app.log", CreatedAt: time.Now()})
	if err := saveSettingsTo(path, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadSettingsFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ContextLines != 10 || loaded.ResultLimit != 5000 {
		t.Fatalf("设置范围未被限制：%+v", loaded)
	}
	if len(loaded.RecentFiles) != maxRecentFiles {
		t.Fatalf("最近文件数量 = %d，期望 %d", len(loaded.RecentFiles), maxRecentFiles)
	}
	if len(loaded.SearchHistory) != 1 || loaded.SearchHistory[0].Query != "timeout" {
		t.Fatalf("检索历史未持久化：%+v", loaded.SearchHistory)
	}
}

func TestAddSearchRecordMovesDuplicateToFront(t *testing.T) {
	settings := DefaultSettings()
	AddSearchRecord(&settings, SearchRecord{Query: "error", Path: "app.log"})
	time.Sleep(time.Millisecond)
	AddSearchRecord(&settings, SearchRecord{Query: "error", Path: "app.log"})
	if len(settings.SearchHistory) != 1 {
		t.Fatalf("重复条件应合并，得到 %d 条", len(settings.SearchHistory))
	}
}

func TestLoadMissingSettingsUsesDefaults(t *testing.T) {
	got, err := loadSettingsFrom(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got.ContextLines != 2 || got.ResultLimit != 500 {
		t.Fatalf("默认设置不符合预期：%+v", got)
	}
}
