package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// 将构建好的前端文件嵌入桌面程序；开发环境会由 Wails 提供前端服务。
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title: "MeteorDesk",
		Width: 1440, Height: 920,
		MinWidth: 1080, MinHeight: 680,
		BackgroundColour: &options.RGBA{R: 12, G: 15, B: 22, A: 1},
		AssetServer: &assetserver.Options{Assets: assets},
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true, DisableWebViewDrop: true},
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true, DisableWebViewDrop: true},
		OnStartup: app.startup,
		Bind: []interface{}{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
