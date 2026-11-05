package main

import (
	"embed"
	"os"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/appicon.png
var trayIconFS embed.FS

func loadTrayIcon() []byte {
	data, err := trayIconFS.ReadFile("build/appicon.png")
	if err != nil {
		return nil
	}
	return data
}

func startTray(app *App) {
	icon := loadTrayIcon()

	onReady := func() {
		if len(icon) > 0 {
			systray.SetIcon(icon)
		}
		systray.SetTitle("MeteorDesk")
		systray.SetTooltip("MeteorDesk")

		mShow := systray.AddMenuItem("显示主窗口", "")
		mHide := systray.AddMenuItem("隐藏主窗口", "")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("退出 MeteorDesk", "")

		go func() {
			for {
				select {
				case <-mShow.ClickedCh:
					runtime.WindowShow(app.ctx)
					runtime.WindowUnminimise(app.ctx)
				case <-mHide.ClickedCh:
					runtime.WindowHide(app.ctx)
				case <-mQuit.ClickedCh:
					systray.Quit()
					os.Exit(0)
				case <-app.ctx.Done():
					return
				}
			}
		}()
	}

	onExit := func() {}

	systray.Run(onReady, onExit)
}

func stopTray() {
	systray.Quit()
}
