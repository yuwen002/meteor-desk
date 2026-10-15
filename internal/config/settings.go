// Package config 管理 MeteorDesk 本地保存的偏好设置与检索历史。
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxRecentFiles = 12
const maxSearchHistory = 30

type SearchRecord struct {
	Query string `json:"query"`
	Path string `json:"path"`
	UseRegex bool `json:"useRegex"`
	ContextLines int `json:"contextLines"`
	CreatedAt time.Time `json:"createdAt"`
}

type Settings struct {
	RecentFiles []string `json:"recentFiles"`
	SearchHistory []SearchRecord `json:"searchHistory"`
	ContextLines int `json:"contextLines"`
	ResultLimit int `json:"resultLimit"`
}

func DefaultSettings() Settings {
	return Settings{
		RecentFiles: []string{},
		SearchHistory: []SearchRecord{},
		ContextLines: 2,
		ResultLimit: 500,
	}
}

// SettingsPath 返回当前用户配置目录下的设置文件路径。
func SettingsPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "MeteorDesk", "settings.json"), nil
}

func LoadSettings() (Settings, error) {
	path, err := SettingsPath()
	if err != nil {
		return DefaultSettings(), err
	}
	return loadSettingsFrom(path)
}

func loadSettingsFrom(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return DefaultSettings(), err
	}
	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return DefaultSettings(), err
	}
	normalize(&settings)
	return settings, nil
}

func SaveSettings(settings Settings) error {
	path, err := SettingsPath()
	if err != nil {
		return err
	}
	return saveSettingsTo(path, settings)
}

// saveSettingsTo 独立出来，方便在单元测试中使用临时目录。
func saveSettingsTo(path string, settings Settings) error {
	normalize(&settings)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		// Windows 无法直接覆盖已有目标，必要时移除旧文件后重试。
		if removeErr := os.Remove(path); removeErr == nil {
			if retryErr := os.Rename(tmp, path); retryErr == nil {
				return nil
			}
		}
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func normalize(settings *Settings) {
	if settings.ContextLines < 0 {
		settings.ContextLines = 0
	}
	if settings.ContextLines > 10 {
		settings.ContextLines = 10
	}
	if settings.ResultLimit < 1 {
		settings.ResultLimit = 500
	}
	if settings.ResultLimit > 5000 {
		settings.ResultLimit = 5000
	}
	settings.RecentFiles = dedupe(settings.RecentFiles, maxRecentFiles)
	if settings.SearchHistory == nil {
		settings.SearchHistory = []SearchRecord{}
	}
	if len(settings.SearchHistory) > maxSearchHistory {
		settings.SearchHistory = settings.SearchHistory[:maxSearchHistory]
	}
}

func AddRecentFile(settings *Settings, path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	next := []string{path}
	for _, item := range settings.RecentFiles {
		if !strings.EqualFold(item, path) {
			next = append(next, item)
		}
	}
	settings.RecentFiles = next
	normalize(settings)
}

func AddSearchRecord(settings *Settings, record SearchRecord) {
	if strings.TrimSpace(record.Query) == "" || strings.TrimSpace(record.Path) == "" {
		return
	}
	record.CreatedAt = time.Now()
	next := []SearchRecord{record}
	for _, item := range settings.SearchHistory {
		if item.Query != record.Query || item.Path != record.Path || item.UseRegex != record.UseRegex || item.ContextLines != record.ContextLines {
			next = append(next, item)
		}
	}
	settings.SearchHistory = next
	normalize(settings)
}

func dedupe(values []string, limit int) []string {
	result := make([]string, 0, min(len(values), limit))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}

 
// NormalizeSettings 对从界面或配置文件接收的设置进行边界校验。
func NormalizeSettings(settings *Settings) {
	if settings == nil { return }
	normalize(settings)
}
