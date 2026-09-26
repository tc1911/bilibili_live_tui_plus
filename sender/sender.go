package sender

import (
	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/getter"
	"time"

	bg "github.com/iyear/biligo"
)

var bc *bg.BiliClient
var err error

func SendMsg(roomId int64, msg string, busChan chan getter.DanmuMsg) {
	if bc == nil { // 未登录或发送端没起来
		busChan <- getter.DanmuMsg{Author: "system", Content: "发不出弹幕：发送端未就绪（网络或登录问题）"}
		return
	}
	msgRune := []rune(msg)
	for i := 0; i < len(msgRune); i += 20 {
		err = nil
		if i+20 < len(msgRune) {
			err = bc.LiveSendDanmaku(roomId, 16777215, 25, 1, string(msgRune[i:i+20]), 0)
			time.Sleep(time.Second * 1)
		} else {
			err = bc.LiveSendDanmaku(roomId, 16777215, 25, 1, string(msgRune[i:]), 0)
		}
		if err != nil {
			busChan <- getter.DanmuMsg{Author: "system", Content: "发送弹幕失败", Type: ""}
		}
	}
}

func Run() {
	for retry := 0; retry < 3; retry++ {
		bc, err = bg.NewBiliClient(&bg.BiliSetting{
			Auth:      &config.Auth,
			DebugMode: false,
		})
		if err == nil {
			return
		}
		time.Sleep(time.Second * 1)
	}
	// 三次都起不来（网络不通 / 登录失效）时不能像以前那样 os.Exit(0)：
	// 那会把整个 TUI 一起静静地带走，屏幕上什么都不剩，看起来就是「打不开」。
	// 现在留 bc == nil，发弹幕时 SendMsg 会提示，30 秒后再自己试一次。
	// ponytail: 固定 30s 退避，跟 getter 一致；要更快就改指数退避。
	time.AfterFunc(time.Second*30, Run)
}
