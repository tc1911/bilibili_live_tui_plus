package main

import (
	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/getter"
	"github.com/tc1911/bilibili_live_tui_plus/sender"
	"github.com/tc1911/bilibili_live_tui_plus/ui"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

/** 用于修正环境变量 */
func fixCharset() {
	locale := os.Getenv("LANG")
	var asianCharset bool
	var wideCharset = []string{"zh_", "jp_", "ko_", "ja_", "th_", "hi_"}
	for k := range wideCharset {
		if strings.HasPrefix(locale, wideCharset[k]) {
			asianCharset = true
		}
	}
	if asianCharset {
		os.Setenv("LANG", "C.UTF-8")
		cmd := exec.Command(os.Args[0], os.Args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
		os.Exit(0)
	}
}

func main() {
	fixCharset()
	config.Init()
	busChan := make(chan getter.DanmuMsg, 100)
	roomInfoChan := make(chan getter.RoomInfo, 100)

	// 弹幕接口需要登录态，没 Cookie 就先只开 TUI，等 F2 扫码成功后再拉起。
	var once sync.Once
	start := func() {
		once.Do(func() {
			getter.Run(busChan, roomInfoChan)
			sender.Run()
		})
	}
	if config.Auth.SESSDATA != "" {
		start()
	} else {
		busChan <- getter.DanmuMsg{Author: "system", Content: "未登录：按 F2 用哔哩哔哩 App 扫码", Type: "NOTICE_MSG", Time: time.Now()}
	}

	ui.Run(busChan, roomInfoChan, start)
}
