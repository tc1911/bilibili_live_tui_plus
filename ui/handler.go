package ui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"

	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/getter"
)

// lastMsg 只在弹幕那条 goroutine 里用：多行模式下同一个人连着说话就不再打一次名字。
var lastMsg = getter.DanmuMsg{}

func danmuHandler(app *tview.Application, messages *tview.TextView, busChan chan getter.DanmuMsg) {
	for msg := range busChan {
		if strings.Trim(msg.Content, " ") == "" {
			continue
		}

		timeStr := msg.Time.Format("15:04")
		if config.Config.ShowTime == 0 {
			timeStr = ""
		}

		str := ""
		if config.Config.SingleLine == 1 {
			str = fmt.Sprintf("[%s]%s [%s]%s[%s] %s", config.Config.TimeColor, timeStr,
				config.Config.NameColor, msg.Author, config.Config.ContentColor, msg.Content)
		} else {
			if lastMsg.Type != msg.Type || lastMsg.Author != msg.Author ||
				(timeStr != "" && lastMsg.Time.Format("15:04") != msg.Time.Format("15:04")) {
				str = fmt.Sprintf("[%s]%s [%s]%s[%s]", config.Config.TimeColor, timeStr,
					config.Config.NameColor, msg.Author, config.Config.ContentColor) + "\n"
			}
			str += " " + msg.Content
		}

		messages.SetText(messages.GetText(false) + str + "\n")
		lastMsg = msg
		app.Draw()
	}
}

// roomInfoHandler 一份房间信息要喂四处：文字信息、观众列表、推流状态、封面。
func roomInfoHandler(app *tview.Application, w *widgets, roomInfoChan chan getter.RoomInfo) {
	for ri := range roomInfoChan {
		w.info.SetText(fmt.Sprintf(
			"[%s]%s[-]\n房间: %d\n分区: %s/%s\n在线: %d    粉丝: %d",
			config.Config.InfoColor, ri.Title,
			ri.RoomId, ri.ParentAreaName, ri.AreaName, ri.Online, ri.Attention))

		w.infoBox.SetTitle(infoTitle(ri))
		w.viewers.SetTitle(fmt.Sprintf(" 观众列表 (%d) ", len(ri.OnlineRankUsers)))
		w.viewers.SetText(rankUsers(ri.OnlineRankUsers))

		w.stream.SetText(streamStatus(ri))

		app.Draw()
	}
}

// streamStatus 拼底部那格。主播最关心的是「现在到底有没有在推流」，
// 所以第一眼给出直播中 / 未开播，再补时长和在线。
func streamStatus(ri getter.RoomInfo) string {
	switch ri.LiveStatus {
	case 1:
		return fmt.Sprintf("[green]● 直播中[-]  已播 %s   在线 %d", ri.Time, ri.Online)
	case 2:
		return "[yellow]● 轮播中[-]"
	default:
		return "[gray]○ 未开播[-]"
	}
}

// infoTitle 把「最后一次成功拉到的时间」写在框边上：数据新不新一眼能看出来。
// 拉失败时明说摆的是旧数据，别让用户以为房间突然空了。
func infoTitle(ri getter.RoomInfo) string {
	if ri.UpdatedAt.IsZero() {
		return " 直播间信息 "
	}
	stamp := ri.UpdatedAt.Format("15:04")
	if ri.Failed {
		return " 直播间信息 · " + stamp + " 的数据（这次没刷上） "
	}
	return " 直播间信息 · " + stamp + " 更新 "
}

func rankUsers(users []getter.OnlineRankUser) string {
	if len(users) == 0 {
		return "[gray]还没有观众上榜[-]"
	}

	spec := []string{"👑 ", "🥈 ", "🥉 "}
	var b strings.Builder
	for i, u := range users {
		b.WriteString("[" + config.Config.RankColor + "]")
		if i < len(spec) {
			b.WriteString(spec[i])
		} else {
			b.WriteString("   ")
		}
		b.WriteString(u.Name)
		b.WriteString("\n")
	}
	return b.String()
}
