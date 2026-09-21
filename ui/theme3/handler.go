package theme3

import (
	"fmt"
	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/getter"
	"strings"

	"github.com/rivo/tview"
)

func roomInfoHandler(app *tview.Application, roomInfoView *tview.TextView, roomInfoChan chan getter.RoomInfo) {
	for roomInfo := range roomInfoChan {
		roomInfoView.SetText(
			fmt.Sprintf("[%s] %d/%s %s/%s 👀: %d ❤️: %d 🕒: %s", config.Config.InfoColor, roomInfo.RoomId, roomInfo.Title, roomInfo.ParentAreaName, roomInfo.AreaName, roomInfo.Online, roomInfo.Attention, roomInfo.Time),
		)
		roomInfoView.ScrollToBeginning()
		app.Draw()
	}
}

var lastMsg = getter.DanmuMsg{}
var lastLine = ""

func danmuHandler(app *tview.Application, messages *tview.TextView, busChan chan getter.DanmuMsg) {
	for msg := range busChan {
		if strings.Trim(msg.Content, " ") == "" {
			continue
		}

		viewStr := messages.GetText(false)
		str := ""

		// 留意前面的空格显示
		timeStr := msg.Time.Format(" 15:04")
		if config.Config.ShowTime == 0 {
			timeStr = ""
		}

		if config.Config.SingleLine == 1 {
			str += fmt.Sprintf("[%s]%s [%s]%s[%s] %s", config.Config.TimeColor, timeStr, config.Config.NameColor, msg.Author, config.Config.ContentColor, msg.Content)
		} else {
			if lastMsg.Type != msg.Type || lastMsg.Author != msg.Author || (timeStr != "" && lastMsg.Time.Format("15:04") != msg.Time.Format("15:04")) {
				str += fmt.Sprintf("[%s]%s [%s]%s[%s]", config.Config.TimeColor, timeStr, config.Config.NameColor, msg.Author, config.Config.ContentColor) + "\n"
			}
			str += fmt.Sprintf(" %s", msg.Content) + "\n"
		}

		messages.SetText(viewStr + strings.TrimRight(str, "\n"))
		lastMsg = msg
		app.Draw()
	}
}
