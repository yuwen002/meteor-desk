package tray

import (
	"context"
	"os"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func Start(ctx context.Context, icon []byte) {
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
					runtime.WindowShow(ctx)
					runtime.WindowUnminimise(ctx)
				case <-mHide.ClickedCh:
					runtime.WindowHide(ctx)
				case <-mQuit.ClickedCh:
					systray.Quit()
					os.Exit(0)
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	systray.Run(onReady, func() {})
}

func Stop() {
	systray.Quit()
}