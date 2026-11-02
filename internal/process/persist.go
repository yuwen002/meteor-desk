package process

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// writeJSONAtomic 把 v 序列化后以“临时文件 + 重命名”的方式落到 path，
// 目录不存在时按 0700 创建，文件权限 0600。重命名在 Windows 上若目标被占用，
// 先删除目标再重试一次，行为与 persistLocked 保持一致。
// 该方法只做一次完整写入，不负责并发串行化，调用方需自行保证同一 path 不并发写。
func writeJSONAtomic(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		if removeErr := os.Remove(path); removeErr == nil {
			if retryErr := os.Rename(tmp, path); retryErr == nil {
				return nil
			}
		}
		_ = os.Remove(tmp)
		return fmt.Errorf("保存 %s 失败：%w", filepath.Base(path), err)
	}
	return nil
}
