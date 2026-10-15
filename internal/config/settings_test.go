package config

import (
 "path/filepath"
 "testing"
 "time"
)
func TestSettingsRoundTripAndLimits(t *testing.T){
 path:=filepath.Join(t.TempDir(),"nested","settings.json")
 settings:=DefaultSettings();settings.ContextLines=99;settings.ResultLimit=8000
 for i:=0;i<20;i++{AddRecentFile(&settings,filepath.Join("logs",string(rune('a'+i))+".log"))}
 AddSearchRecord(&settings,SearchRecord{Query:"timeout",Path:"/tmp/app.log",CreatedAt:time.Now()})
 if err:=saveSettingsTo(path,settings);err!=nil{t.Fatal(err)}
 loaded,err:=loadSettingsFrom(path);if err!=nil{t.Fatal(err)}
 if loaded.ContextLines!=10||loaded.ResultLimit!=5000{t.Fatalf("范围校验错误：%+v",loaded)}
 if len(loaded.RecentFiles)!=maxRecentFiles{t.Fatalf("最近文件数量=%d",len(loaded.RecentFiles))}
 if len(loaded.SearchHistory)!=1||loaded.SearchHistory[0].Query!="timeout"{t.Fatalf("历史未保存：%+v",loaded.SearchHistory)}
}
func TestAddSearchRecordMovesDuplicateToFront(t *testing.T){
 s:=DefaultSettings();AddSearchRecord(&s,SearchRecord{Query:"error",Path:"app.log"});AddSearchRecord(&s,SearchRecord{Query:"error",Path:"app.log"})
 if len(s.SearchHistory)!=1{t.Fatalf("重复历史未合并：%+v",s.SearchHistory)}
}
func TestLoadMissingSettingsUsesDefaults(t *testing.T){
 got,err:=loadSettingsFrom(filepath.Join(t.TempDir(),"missing.json"));if err!=nil{t.Fatal(err)}
 if got.ContextLines!=2||got.ResultLimit!=500{t.Fatalf("默认设置不正确：%+v",got)}
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
