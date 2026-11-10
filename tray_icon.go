package main

import (
	"embed"
	goruntime "runtime"
)

//go:embed build/appicon.ico build/appicon.png
var trayIconFS embed.FS

func LoadTrayIcon() []byte {
	name := "build/appicon.png"
	if goruntime.GOOS == "windows" {
		name = "build/appicon.ico"
	}
	data, _ := trayIconFS.ReadFile(name)
	return data
}