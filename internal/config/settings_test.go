package config

import (
	"path/filepath"
	"runtime"
	"sync"
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
		t.Fatalf("范围校验错误：%+v", loaded)
	}
	if len(loaded.RecentFiles) != maxRecentFiles {
		t.Fatalf("最近文件数量=%d", len(loaded.RecentFiles))
	}
	if len(loaded.SearchHistory) != 1 || loaded.SearchHistory[0].Query != "timeout" {
		t.Fatalf("历史未保存：%+v", loaded.SearchHistory)
	}
}
func TestAddSearchRecordMovesDuplicateToFront(t *testing.T) {
	s := DefaultSettings()
	AddSearchRecord(&s, SearchRecord{Query: "error", Path: "app.log"})
	AddSearchRecord(&s, SearchRecord{Query: "error", Path: "app.log"})
	if len(s.SearchHistory) != 1 {
		t.Fatalf("重复历史未合并：%+v", s.SearchHistory)
	}
}
func TestLoadMissingSettingsUsesDefaults(t *testing.T) {
	got, err := loadSettingsFrom(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got.ContextLines != 2 || got.ResultLimit != 500 {
		t.Fatalf("默认设置不正确：%+v", got)
	}
}

func TestSearchRecordEncodingRoundTripAndDeduplication(t *testing.T) {
	settings := DefaultSettings()
	AddSearchRecord(&settings, SearchRecord{Query: "timeout", Path: "app.log", Encoding: "utf-8"})
	AddSearchRecord(&settings, SearchRecord{Query: "timeout", Path: "app.log", Encoding: "gb18030"})
	if len(settings.SearchHistory) != 2 {
		t.Fatalf("不同编码应保留为不同历史记录：%+v", settings.SearchHistory)
	}

	path := filepath.Join(t.TempDir(), "settings.json")
	if err := saveSettingsTo(path, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadSettingsFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.SearchHistory) != 2 {
		t.Fatalf("编码历史数量异常：%+v", loaded.SearchHistory)
	}
	if loaded.SearchHistory[0].Encoding != "gb18030" || loaded.SearchHistory[1].Encoding != "utf-8" {
		t.Fatalf("编码历史顺序或持久化错误：%+v", loaded.SearchHistory)
	}
}

func TestSaveSettingsUsesUniqueTemporaryFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	settings := DefaultSettings()
	settings.SearchHistory = []SearchRecord{{Query: "first", Path: "app.log"}}
	if err := saveSettingsTo(path, settings); err != nil {
		t.Fatal(err)
	}

	entries, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".settings-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("保存成功后不应残留临时文件：%v", entries)
	}
	loaded, err := loadSettingsFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.SearchHistory) != 1 || loaded.SearchHistory[0].Query != "first" {
		t.Fatalf("覆盖保存后内容异常：%+v", loaded.SearchHistory)
	}
}

func TestConcurrentSaveSettingsDoNotShareTemporaryFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows replaces existing files differently; App serializes production writes")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	const writers = 16
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			settings := DefaultSettings()
			settings.SearchHistory = []SearchRecord{{Query: "writer", Path: filepath.Join("logs", string(rune('a'+i))+".log")}}
			errs <- saveSettingsTo(path, settings)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("并发保存失败：%v", err)
		}
	}
	if _, err := loadSettingsFrom(path); err != nil {
		t.Fatalf("并发保存后文件不是有效 JSON：%v", err)
	}
	entries, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".settings-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("并发保存后残留临时文件：%v", entries)
	}
}

func TestWindowBoundsNormalize(t *testing.T) {
	tests := []struct {
		name      string
		input     WindowBounds
		expectedX int
		expectedY int
		expectOK  bool
	}{
		{"合法尺寸和最大化", WindowBounds{X: 100, Y: 80, Width: 1280, Height: 800, Maximised: true}, 100, 80, true},
		{"多显示器负坐标", WindowBounds{X: -1920, Y: 0, Width: 1440, Height: 900}, -1920, 0, true},
		{"宽高为零应清空", WindowBounds{Width: 0, Height: 0}, 0, 0, false},
		{"宽度过小应清空", WindowBounds{Width: 200, Height: 800}, 0, 0, false},
		{"高度过小应清空", WindowBounds{Width: 1280, Height: 100}, 0, 0, false},
		{"宽度过大应清空", WindowBounds{Width: 10000, Height: 800}, 0, 0, false},
		{"高度过大应清空", WindowBounds{Width: 1280, Height: 5000}, 0, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := DefaultSettings()
			s.Window = tc.input
			normalize(&s)
			if tc.expectOK {
				if s.Window.Width == 0 || s.Window.Height == 0 {
					t.Fatalf("期望保留窗口，但被清空：%+v", s.Window)
				}
				if s.Window.X != tc.expectedX || s.Window.Y != tc.expectedY {
					t.Fatalf("坐标异常：got (%d,%d) want (%d,%d)", s.Window.X, s.Window.Y, tc.expectedX, tc.expectedY)
				}
			} else {
				if s.Window.Width != 0 || s.Window.Height != 0 {
					t.Fatalf("期望清空窗口，但仍有值：%+v", s.Window)
				}
			}
		})
	}
}

func TestWindowBoundsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	settings := DefaultSettings()
	settings.Window = WindowBounds{X: 240, Y: 180, Width: 1440, Height: 920, Maximised: true}
	if err := saveSettingsTo(path, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadSettingsFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Window.X != 240 || loaded.Window.Y != 180 || loaded.Window.Width != 1440 || loaded.Window.Height != 920 || !loaded.Window.Maximised {
		t.Fatalf("窗口状态 round-trip 异常：%+v", loaded.Window)
	}
}