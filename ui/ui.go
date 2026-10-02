package ui

import (
	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/getter"
	"github.com/tc1911/bilibili_live_tui_plus/ui/theme1"
	"github.com/tc1911/bilibili_live_tui_plus/ui/theme2"
	"github.com/tc1911/bilibili_live_tui_plus/ui/theme3"
	"github.com/tc1911/bilibili_live_tui_plus/ui/theme4"
	"github.com/tc1911/bilibili_live_tui_plus/ui/theme5"
)

func Run(busChan chan getter.DanmuMsg, roomInfoChan chan getter.RoomInfo, onLogin func()) {
	switch config.Config.Theme {
	case 1: // theme1
		theme1.Run(busChan, roomInfoChan, onLogin) // chat room
	case 2: // theme2
		theme2.Run(busChan, roomInfoChan, onLogin) // pure
	case 3: // theme3
		theme3.Run(busChan, roomInfoChan, onLogin) // simple
	case 4:
		theme4.Run(busChan, roomInfoChan, onLogin) // info
	case 5:
		theme5.Run(busChan, roomInfoChan, onLogin) // 新版：标题栏 + 封面预览 + 观众列表 + 推流状态
	default:
		theme1.Run(busChan, roomInfoChan, onLogin) // default theme1
	}
}
