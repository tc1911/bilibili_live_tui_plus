// 极简主题

package theme2

import (
	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/getter"
	"github.com/tc1911/bilibili_live_tui_plus/sender"
	"github.com/tc1911/bilibili_live_tui_plus/ui/control"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var submitHistory = []string{}
var submitHistoryIndex = 0
var bg = tcell.ColorDefault

func draw(app *tview.Application, roomId int64, busChan chan getter.DanmuMsg, roomInfoChan chan getter.RoomInfo) *tview.Grid {
	chatGrid := tview.NewGrid().SetRows(0, 1).SetBorders(false)
	messagesView := tview.NewTextView().SetDynamicColors(true)
	messagesView.SetBackgroundColor(bg)

	input := tview.NewInputField()
	input.SetFormAttributes(0, tcell.ColorDefault, bg, tcell.ColorDefault, bg)

	chatGrid.
		AddItem(messagesView, 0, 0, 1, 1, 0, 0, false).
		AddItem(input, 1, 0, 1, 1, 0, 0, true)

	go danmuHandler(app, messagesView, busChan)

	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			go sender.SendMsg(roomId, input.GetText(), busChan)

			submitHistory = append(submitHistory, input.GetText())
			if len(submitHistory) > 10 {
				submitHistory = submitHistory[1:]
			}
			submitHistoryIndex = len(submitHistory)

			input.SetText("")
		}
	})

	return chatGrid
}

func Run(busChan chan getter.DanmuMsg, roomInfoChan chan getter.RoomInfo, onLogin func()) {
	if config.Config.Background != "NONE" {
		bg = tcell.GetColor(config.Config.Background)
	}
	app := tview.NewApplication()
	root := control.Wrap(app, draw(app, config.Config.RoomId, busChan, roomInfoChan), onLogin)
	if err := app.SetRoot(root, true).EnableMouse(false).Run(); err != nil {
		panic(err)
	}
}
