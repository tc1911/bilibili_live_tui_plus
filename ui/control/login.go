package control

import (
	"fmt"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/live"
)

// refreshAccount 开局校验一次登录态，失效就把二维码重新摆出来。
func (p *panel) refreshAccount() {
	p.mu.Lock()
	mid, uname, err := p.client.Nav()
	p.mu.Unlock()

	if err != nil {
		p.update(func() {
			p.account = "登录态失效"
			p.setInfo()
			p.setHint(err.Error())
			p.loggedIn.Store(false)
			p.fillTab() // 掉线了就把二维码摆上，等用户扫
		})
		return
	}

	p.update(func() {
		p.account = fmt.Sprintf("%s (uid %d)", uname, mid)
		p.setInfo()
	})
}

// login 取一张二维码并开始轮询。回车、以及进「账号」栏时都会走到这儿。
func (p *panel) login() {
	// 已经在等扫码了就别再生成一张：屏幕上两张二维码，用户扫了哪张都说不清。
	if p.loginPending.Swap(true) {
		return
	}
	defer p.loginPending.Store(false)

	p.mu.Lock()
	if p.client.LoggedIn() {
		if mid, uname, err := p.client.Nav(); err == nil {
			p.mu.Unlock()
			p.update(func() {
				p.account = fmt.Sprintf("%s (uid %d)", uname, mid)
				p.setInfo()
				p.setHint("已登录，无需重复扫码")
			})
			return
		}
	}
	loginURL, key, err := p.client.QRGenerate()
	p.qrGen++
	gen := p.qrGen
	p.mu.Unlock()

	if err != nil {
		p.setStatus("二维码获取失败: " + err.Error())
		return
	}

	p.update(func() {
		p.showQR(loginURL, "用哔哩哔哩 App 扫码登录")
		p.setHint("等待扫码…二维码 3 分钟内有效")
	})
	p.poll(gen, key)
}

func (p *panel) poll(gen int, key string) {
	for i := 0; i < 90; i++ {
		time.Sleep(2 * time.Second)

		p.mu.Lock()
		if gen != p.qrGen { // 又生成了新的，这一轮作废
			p.mu.Unlock()
			return
		}
		status, err := p.client.QRPoll(key)
		p.mu.Unlock()

		if err != nil {
			continue
		}
		switch status.Code {
		case live.QRSuccess:
			p.finishLogin()
			return
		case live.QRExpired:
			p.setStatus("二维码已过期，在「账号」栏按回车换一张")
			return
		case live.QRWaitingOK:
			p.setStatus("已扫码，请在手机上点确认")
		}
	}
	p.setStatus("二维码已过期，在「账号」栏按回车换一张")
}

func (p *panel) finishLogin() {
	p.mu.Lock()
	cookie := p.client.Cookie()
	mid, uname, navErr := p.client.Nav()
	ownRoom := int64(0)
	if navErr == nil {
		ownRoom, _ = p.client.RoomIDByUID(mid)
	}
	p.mu.Unlock()

	config.Config.Cookie = cookie
	config.RefreshAuth()

	// 默认配置里的房间号是别人的，登录后换成本人的，否则弹幕看的是别人房间。
	movedRoom := ownRoom > 0 && config.Config.RoomId == defaultRoomID
	if movedRoom {
		config.Config.RoomId = ownRoom
	}

	if err := config.Save(); err != nil {
		p.setStatus("登录成功但写入配置失败: " + err.Error())
		return
	}
	if p.onLogin != nil {
		p.onLogin()
	}

	p.update(func() {
		p.account = fmt.Sprintf("%s (uid %d)", uname, mid)
		p.loggedIn.Store(true)
		p.setInfo()
		p.side.SetText("") // 二维码没用了，收掉
		switch {
		case navErr != nil:
			// 凭据已落盘，nav 抖一下不值得让用户重扫。
			p.setHint("已登录，但获取账号信息失败: " + navErr.Error())
		case movedRoom:
			p.setHint(fmt.Sprintf("登录成功，直播间已设为 %d，弹幕连接中", ownRoom))
		default:
			p.setHint(fmt.Sprintf("登录成功，弹幕连接中。你的直播间: %d", ownRoom))
		}
		// 登录完顺手把分区列表拉出来，省得再切一次栏。
		p.fillTab()
	})
}

// showQR 把登录二维码画成半格字符，写在「账号」栏里。
// 颜色用真彩色 #000000/#ffffff：用 black/white 名字会走终端调色板，
// 浅色主题下会和背景一样淡，扫不出来。
func (p *panel) showQR(content, title string) {
	code, err := qrcode.New(content, qrcode.Low)
	if err != nil {
		p.side.SetText("\n[white]" + title + "[-]\n\n" + content)
		return
	}
	bitmap := code.Bitmap() // 已含静默区

	var b string
	b += "\n[yellow]" + title + "[-]\n\n"
	for y := 0; y < len(bitmap); y += 2 {
		b += "[#000000:#ffffff]"
		for x := 0; x < len(bitmap[y]); x++ {
			up := bitmap[y][x]
			down := y+1 < len(bitmap) && bitmap[y+1][x]
			switch {
			case up && down:
				b += "█"
			case up:
				b += "▀"
			case down:
				b += "▄"
			default:
				b += " "
			}
		}
		b += "[-]\n"
	}
	p.side.SetText(b)
}
