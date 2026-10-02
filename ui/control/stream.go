package control

import (
	"fmt"

	"github.com/rivo/tview"

	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/live"
)

// loadStreamStatus 进「推流码」栏时先看一眼开播状态，没开播就别摆个空框。
func (p *panel) loadStreamStatus() {
	roomID := config.Config.RoomId

	p.mu.Lock()
	room, err := p.client.Room(roomID)
	p.mu.Unlock()

	p.update(func() {
		switch {
		case err != nil:
			p.setHint("读取开播状态失败: " + err.Error())
		case room.LiveStatus == 1:
			p.setHint("正在直播中；推流地址与密钥就在本栏，下播按 F5")
		default:
			p.streams.SetText("[gray]还没开播。[-]按 [yellow]F4[-] 开播，这里会显示推流地址与推流码。\n")
			p.setHint("未开播")
		}
	})
}

func (p *panel) confirmStart() {
	modal := tview.NewModal().
		SetText("开播后直播间会立刻对外可见，粉丝会收到开播推送。\n确定开播？").
		AddButtons([]string{"开播", "取消"}).
		SetDoneFunc(func(_ int, label string) {
			p.closeConfirm()
			if label == "开播" {
				go p.startLive()
			}
		})
	p.pages.AddPage("confirm", modal, true, true)
	p.app.SetFocus(modal)
}

// closeConfirm 收起确认弹窗，把焦点还给当前那一栏。
// 焦点不能丢给 main：配置页还留在屏上，丢过去之后箭头和 Esc 就全落到弹幕页，
// 弹窗看着还开着、实际是死的。
func (p *panel) closeConfirm() {
	p.pages.RemovePage("confirm")
	p.syncTab()
}

func (p *panel) startLive() {
	roomID := config.Config.RoomId
	areaV2 := config.Config.AreaV2

	p.mu.Lock()
	room, err := p.client.Room(roomID)
	if err != nil {
		p.mu.Unlock()
		p.setStatus("查询直播间失败: " + err.Error())
		return
	}
	if areaV2 == 0 {
		areaV2 = int64(room.AreaID)
	}
	if areaV2 == 0 {
		p.mu.Unlock()
		p.setStatus("直播间还没分区，先在「分区」栏选一个（或去网页端设一次）")
		return
	}
	streams, err := p.client.StartLive(roomID, areaV2)
	p.mu.Unlock()

	if qr := live.VerifyQR(err); qr != "" {
		p.update(func() {
			p.tab = tabAccount
			p.syncTab()
			p.showQR(qr, "开播需要验证，扫码后在手机上确认，再按 F4")
			p.setHint(err.Error())
		})
		return
	}
	if err != nil {
		p.setStatus("开播失败: " + err.Error())
		return
	}

	// 密钥近百字符，得单独占一行：挤在一行里就整行放不下，
	// 在终端里选中复制出来是断的。
	text := "[yellow]推流地址与推流码[-]\n\n"
	for i, s := range streams {
		text += fmt.Sprintf("[white]%s[-]", s.Type)
		if i == 0 {
			text += "[yellow]（OBS 填这组）[-]"
		}
		text += "\n服务器\n" + s.Address + "\n密钥\n" + s.Key + "\n\n"
	}
	text += "[yellow]直播间已对外可见；下播按 F5，回弹幕页按 Shift+Tab[-]"

	p.update(func() {
		p.streams.SetText(text)
		p.side.SetText("")
		p.tab = tabStream
		p.syncTab()
		p.setHint(fmt.Sprintf("已开播：%s（%s）", room.Title, room.Area()))
	})
}

func (p *panel) stopLive() {
	p.mu.Lock()
	err := p.client.StopLive(config.Config.RoomId)
	p.mu.Unlock()

	p.update(func() {
		if err != nil {
			p.setHint("下播失败: " + err.Error())
			return
		}
		p.streams.SetText("")
		p.setHint("已下播")
	})
}
