package sender

import (
	"sync"
	"time"

	bg "github.com/iyear/biligo"

	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/getter"
)

// bc 由后台重试的 Run 写、TUI 的 SendMsg 读，不在同一个 goroutine 里，必须加锁。
var (
	mu sync.RWMutex
	bc *bg.BiliClient
)

// client 取当前可用的发送端，没就绪返回 nil。
func client() *bg.BiliClient {
	mu.RLock()
	defer mu.RUnlock()
	return bc
}

func SendMsg(roomId int64, msg string, busChan chan getter.DanmuMsg) {
	c := client()
	if c == nil { // 未登录或发送端没起来
		busChan <- getter.DanmuMsg{Author: "system", Content: "发不出弹幕：发送端未就绪（网络或登录问题）", Type: "NOTICE_MSG", Time: time.Now()}
		return
	}

	// 单条弹幕上限 20 字，超出的切段补发，段间留一秒。
	msgRune := []rune(msg)
	for i := 0; i < len(msgRune); i += 20 {
		end := i + 20
		if end > len(msgRune) {
			end = len(msgRune)
		}
		// 错误原来存在包级变量 err 里，两条弹幕同时发会互相覆盖。
		if err := c.LiveSendDanmaku(roomId, 16777215, 25, 1, string(msgRune[i:end]), 0); err != nil {
			busChan <- getter.DanmuMsg{Author: "system", Content: "发送弹幕失败: " + err.Error(), Type: "NOTICE_MSG", Time: time.Now()}
		}
		if end < len(msgRune) {
			time.Sleep(time.Second)
		}
	}
}

func Run() {
	for retry := 0; retry < 3; retry++ {
		c, err := bg.NewBiliClient(&bg.BiliSetting{
			Auth:      &config.Auth,
			DebugMode: false,
		})
		if err == nil {
			mu.Lock()
			bc = c
			mu.Unlock()
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
