// 聊天室主题

package theme1

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

func setBoxAttr(box *tview.Box, title string) {
	box.SetBorder(true)
	box.SetTitleAlign(tview.AlignLeft)
	box.SetTitle(title)
	box.SetBackgroundColor(bg)
	box.SetBorderColor(tcell.GetColor(config.Config.FrameColor))
	box.SetTitleColor(tcell.GetColor(config.Config.FrameColor))
}

func drawSlidebar() (*tview.Grid, *tview.TextView, *tview.TextView) {
	slidebarGrid := tview.NewGrid().SetRows(0, 0).SetBorders(false)
	roomInfoView := tview.NewTextView().SetDynamicColors(true)
	roomInfoView.SetBackgroundColor(bg)
	setBoxAttr(roomInfoView.Box, "RoomInfo")

	rankUsersView := tview.NewTextView().SetDynamicColors(true)
	rankUsersView.SetBackgroundColor(bg)
	setBoxAttr(rankUsersView.Box, "RankUsers")

	slidebarGrid.
		AddItem(roomInfoView, 0, 0, 1, 1, 0, 0, false).
		AddItem(rankUsersView, 1, 0, 1, 1, 0, 0, false)

	return slidebarGrid, roomInfoView, rankUsersView
}

func drawChat() (*tview.Grid, *tview.InputField, *tview.TextView) {
	chatGrid := tview.NewGrid().SetRows(0, 3).SetBorders(false)
	messagesView := tview.NewTextView().SetDynamicColors(true)
	messagesView.SetBackgroundColor(bg)
	setBoxAttr(messagesView.Box, "Messages")

	input := tview.NewInputField()
	input.SetFormAttributes(0, tcell.ColorDefault, bg, tcell.ColorDefault, bg)
	setBoxAttr(input.Box, "Send")

	chatGrid.
		AddItem(messagesView, 0, 0, 1, 1, 0, 0, false).
		AddItem(input, 1, 0, 1, 1, 0, 0, true)

	return chatGrid, input, messagesView
}

func draw(app *tview.Application, roomId int64, busChan chan getter.DanmuMsg, roomInfoChan chan getter.RoomInfo) *tview.Grid {
	slidebarGrid, roomInfoView, rankUsersView := drawSlidebar()
	chatGrid, input, messagesView := drawChat()
	rootGrid := tview.NewGrid().SetColumns(20, 0).SetBorders(false)
	rootGrid.
		AddItem(slidebarGrid, 0, 0, 1, 1, 0, 0, false).
		AddItem(chatGrid, 0, 1, 1, 1, 0, 0, true)

	go roomInfoHandler(app, roomInfoView, rankUsersView, roomInfoChan)
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

	return rootGrid
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
