// Package ui 只有一套界面：主页面 + 配置页（ui/control）。
//
// 以前有 1~5 号主题，2026-10-03 拆掉了——只留重做的这套。
package ui

import (
	"fmt"
	"os"

	"github.com/rivo/tview"

	"github.com/tc1911/bilibili_live_tui_plus/getter"
	"github.com/tc1911/bilibili_live_tui_plus/ui/control"
)

// Run 起 TUI：主页面 + 配置页，跑在同一个 Application 上。
func Run(busChan chan getter.DanmuMsg, roomInfoChan chan getter.RoomInfo, onLogin func()) {
	app := tview.NewApplication()
	w := draw(busChan, roomInfoChan)

	go danmuHandler(app, w.messages, busChan)
	go roomInfoHandler(app, w, roomInfoChan)

	root := control.Wrap(app, w.root, onLogin)
	if err := app.SetRoot(root, true).EnableMouse(false).Run(); err != nil {
		// 不 panic：TUI 起不来时给一句能看懂的话，堆栈留给日志。
		fmt.Fprintln(os.Stderr, "TUI 启动失败: "+err.Error())
	}
}
