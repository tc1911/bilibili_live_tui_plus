// Package control 在主题上叠一层直播间控制面板：
// F2 扫码登录、F3 选分区、F4 开播取推流码、F5 下播。
package control

import (
	"fmt"
	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/live"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	qrcode "github.com/skip2/go-qrcode"
)

// areaRef 挂在分区树的叶子上。
type areaRef struct {
	ID   int64
	Name string
}

// defaultRoomID 与 config.defaultCfgFile 里的初始值一致。
// 它就是「用户还没设过自己的房间号」这个状态。
const defaultRoomID int64 = 23333333

type panel struct {
	app     *tview.Application
	pages   *tview.Pages
	main    tview.Primitive
	client  *live.Client
	onLogin func()

	info    *tview.TextView
	side    *tview.TextView // 右栏：登录 / 验证二维码
	streams *tview.TextView // 推流码整页：密钥近百字符，只有占满宽度才选得全
	body    *tview.Flex
	hint    *tview.TextView
	tree    *tview.TreeView

	mu      sync.Mutex // 串行化接口调用
	qrGen   int        // 递增即作废旧的扫码轮询
	account string
}

// Wrap 把主题根组件包进 Pages 并挂上全局快捷键。
// 扫码登录成功后调用 onLogin，让 getter / sender 开始工作。
func Wrap(app *tview.Application, root tview.Primitive, onLogin func()) *tview.Pages {
	p := &panel{
		app:     app,
		main:    root,
		client:  live.NewClient(config.Config.Cookie),
		onLogin: onLogin,
		account: "未登录（F2 扫码登录）",
	}

	p.pages = tview.NewPages().
		AddPage("main", root, true, true).
		AddPage("control", p.build(), true, false).
		AddPage("streams", p.streams, true, false)
	app.SetInputCapture(p.onKey)

	p.setInfo()
	if p.client.LoggedIn() {
		go p.refreshAccount()
	} else {
		p.setHint("未登录：按 F2 用哔哩哔哩 App 扫码")
		p.pages.ShowPage("control")
	}
	return p.pages
}

func (p *panel) build() *tview.Flex {
	p.info = tview.NewTextView().SetDynamicColors(true)
	p.info.SetBorder(true).SetTitle(" 直播间控制 ")

	p.side = tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter).SetWrap(false)
	p.side.SetBorder(true).SetTitle(" 扫码 ")

	// 服务端返回的密钥有 90~100 字符，夹在 46 列的右栏里不管折不折行都复制不出来。
	p.streams = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	p.streams.SetBorder(true).SetTitle(" 推流码 ")

	p.tree = tview.NewTreeView()
	p.tree.SetBorder(true).SetTitle(" 分区 ")
	p.tree.SetSelectedFunc(p.pickArea)
	p.tree.SetInputCapture(treeKeyCapture(p.tree))

	p.hint = tview.NewTextView().SetDynamicColors(true).SetWrap(true)

	body := tview.NewFlex().
		AddItem(p.tree, 0, 1, true).
		AddItem(p.side, 46, 0, false)
	p.body = body

	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.info, 5, 0, false).
		AddItem(body, 0, 1, true).
		AddItem(p.hint, 2, 0, false)
}

// ---------------------------------------------------------------- 按键

func (p *panel) onKey(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyF2:
		p.show()
		go p.login()
	case tcell.KeyF3:
		p.show()
		go p.loadAreas()
	case tcell.KeyF4:
		p.show()
		p.confirmStart()
	case tcell.KeyF5:
		p.show()
		go p.stopLive()
	case tcell.KeyEscape:
		if p.pages.HasPage("streams") && p.app.GetFocus() == p.streams {
			p.pages.HidePage("streams")
			p.pages.ShowPage("control")
			p.app.SetFocus(p.tree)
			return nil
		}
		if p.pages.HasPage("control") && p.focused() {
			p.pages.HidePage("control")
			p.app.SetFocus(p.main)
			return nil
		}
	}
	return ev
}

func (p *panel) show() {
	p.pages.ShowPage("control")
	if p.pages.HasPage("control") {
		p.app.SetFocus(p.tree)
	}
}

func (p *panel) focused() bool { return p.app.GetFocus() != p.main }

// update 供后台 goroutine 改界面用；在事件循环里直接改会死锁。
func (p *panel) update(fn func()) { p.app.QueueUpdateDraw(fn) }

func (p *panel) setInfo() {
	area := config.Config.AreaName
	if area == "" {
		area = "未选择（F3）"
	}
	p.info.SetText(fmt.Sprintf(
		"[white]账号:[-] %s\n[white]直播间:[-] %d\n[white]分区:[-] %s\n[white]按键:[-] F2 登录  F3 分区  F4 开播  F5 下播  Esc 关闭",
		p.account, config.Config.RoomId, area))
}

func (p *panel) setHint(text string) { p.hint.SetText("[yellow]" + text + "[-]") }

// ---------------------------------------------------------------- 登录

func (p *panel) refreshAccount() {
	p.mu.Lock()
	mid, uname, err := p.client.Nav()
	p.mu.Unlock()

	if err != nil {
		p.update(func() {
			p.account = "登录态失效（F2 重新扫码）"
			p.setInfo()
			p.setHint(err.Error())
		})
		return
	}
	p.update(func() {
		p.account = fmt.Sprintf("%s (uid %d)", uname, mid)
		p.setInfo()
	})
}

func (p *panel) login() {
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
		p.update(func() { p.setHint("二维码获取失败: " + err.Error()) })
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
		if gen != p.qrGen {
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
			p.update(func() { p.setHint("二维码已过期，再按一次 F2 重新获取") })
			return
		case live.QRWaitingOK:
			p.update(func() { p.setHint("已扫码，请在手机上点确认") })
		}
	}
	p.update(func() { p.setHint("二维码已过期，再按一次 F2 重新获取") })
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
	MovedRoom := ownRoom > 0 && config.Config.RoomId == defaultRoomID
	if MovedRoom {
		config.Config.RoomId = ownRoom
	}

	if err := config.Save(); err != nil {
		p.update(func() { p.setHint("登录成功但写入配置失败: " + err.Error()) })
		return
	}
	if p.onLogin != nil {
		p.onLogin()
	}

	p.update(func() {
		p.account = fmt.Sprintf("%s (uid %d)", uname, mid)
		p.side.SetText("")
		p.setInfo()
		switch {
		case navErr != nil:
			// 凭据已落盘，nav 抖一下不值得让用户重扫。
			p.setHint("已登录，但获取账号信息失败: " + navErr.Error())
		case MovedRoom:
			p.setHint(fmt.Sprintf("登录成功，直播间已设为 %d，弹幕连接中", ownRoom))
		default:
			p.setHint(fmt.Sprintf("登录成功，弹幕连接中。你的直播间: %d", ownRoom))
		}
	})
}

// ---------------------------------------------------------------- 分区

func (p *panel) loadAreas() {
	p.mu.Lock()
	areas, err := p.client.Areas()
	p.mu.Unlock()
	if err != nil {
		p.update(func() { p.setHint("分区列表获取失败: " + err.Error()) })
		return
	}

	root, selected := areaTree(areas, config.Config.AreaV2)
	p.update(func() {
		p.tree.SetRoot(root)
		// 没存过分区时光标停在全部分区上：藏在收起节点里的光标是画不出来的。
		p.tree.SetCurrentNode(root)
		if selected.GetReference() != nil {
			p.tree.SetCurrentNode(selected)
		}
		p.setHint("上下键选分区，左右键展开/收起，回车确认；选中的分区会记进配置")
	})
}

// treeKeyCapture 补上 tview 没提供的展开/收起：tview 的左右键只是“在可见节点里上下移动”
// （KeyDown,KeyRight 都是 move +1），没有折叠这回事。
// 补成文件树那套：右键展开、左键收起；没什么可做的就交回 tview，让它自己跳。
func treeKeyCapture(tree *tview.TreeView) func(*tcell.EventKey) *tcell.EventKey {
	return func(ev *tcell.EventKey) *tcell.EventKey {
		node := tree.GetCurrentNode()
		if node == nil {
			return ev
		}
		// 叶子在 tview 里默认就是 expanded=true，不加这个判断它会把左键当成收起吃掉，
		// 人就再也回不到父分区了。
		hasChildren := len(node.GetChildren()) > 0
		switch ev.Key() {
		case tcell.KeyRight:
			if hasChildren && !node.IsExpanded() {
				node.SetExpanded(true)
				return nil
			}
		case tcell.KeyLeft:
			if hasChildren && node.IsExpanded() {
				node.SetExpanded(false)
				return nil
			}
		}
		return ev
	}
}

// areaTree 把两级分区铺成树。父分区必须是可选的：tview 的上下键只在可选节点上停留，
// 不可选就整段跳过去，回车又会被丢给 selectedFunc，于是父分区既进不去也选不了。
func areaTree(areas []live.ParentArea, saved int64) (root, selected *tview.TreeNode) {
	root = tview.NewTreeNode("全部分区").SetExpanded(true)
	selected = tview.NewTreeNode("")
	for _, parent := range areas {
		// tview 的 NewTreeNode 默认 expanded=true，所以这句不能省：
		// 不写就是十来个栏全开着；写成 SetExpanded(是不是第一个) 又永远是列表第一项（网游）开着。
		// 展开谁只看配置里记着的分区。
		pn := tview.NewTreeNode(parent.Name)
		pn.SetExpanded(false)
		for _, sub := range parent.List {
			ref := &areaRef{ID: int64(sub.ID), Name: parent.Name + "/" + sub.Name}
			leaf := tview.NewTreeNode("  " + sub.Name).SetReference(ref)
			if ref.ID == saved {
				pn.SetExpanded(true)
				selected = leaf
			}
			pn.AddChild(leaf)
		}
		root.AddChild(pn)
	}
	return root, selected
}

func (p *panel) pickArea(node *tview.TreeNode) {
	ref, ok := node.GetReference().(*areaRef)
	if !ok {
		if node.GetChildren() != nil { // 父分区：回车只负责展开/收起
			node.SetExpanded(!node.IsExpanded())
		}
		return
	}
	config.Config.AreaV2 = ref.ID
	config.Config.AreaName = ref.Name
	// 本函数由 TreeView 在事件循环里同步调用，只能直接改界面：
	// QueueUpdateDraw 要等事件循环回头执行它，而事件循环正卡在本函数里 —— 整个 TUI 冻住。
	p.setInfo()
	if err := config.Save(); err != nil {
		p.setHint("分区已选中，但写入配置失败: " + err.Error())
		return
	}
	p.setHint("开播分区已设为 " + ref.Name)
}

// ---------------------------------------------------------------- 开播 / 下播

func (p *panel) confirmStart() {
	modal := tview.NewModal().
		SetText("开播后直播间会立刻对外可见，粉丝会收到开播推送。\n确定开播？").
		AddButtons([]string{"开播", "取消"}).
		SetDoneFunc(func(_ int, label string) {
			p.pages.RemovePage("confirm")
			p.app.SetFocus(p.main)
			if label == "开播" {
				go p.startLive()
			}
		})
	p.pages.AddPage("confirm", modal, true, true)
	p.app.SetFocus(modal)
}

func (p *panel) startLive() {
	roomID := config.Config.RoomId
	areaV2 := config.Config.AreaV2

	p.mu.Lock()
	room, err := p.client.Room(roomID)
	if err != nil {
		p.mu.Unlock()
		p.update(func() { p.setHint("查询直播间失败: " + err.Error()) })
		return
	}
	if areaV2 == 0 {
		areaV2 = int64(room.AreaID)
	}
	if areaV2 == 0 {
		p.mu.Unlock()
		p.update(func() { p.setHint("直播间还没分区，先按 F3 选一个（或去网页端设一次）") })
		return
	}
	streams, err := p.client.StartLive(roomID, areaV2)
	p.mu.Unlock()

	if qr := live.VerifyQR(err); qr != "" {
		p.update(func() {
			p.showQR(qr, "开播需要验证，扫码后在手机上确认，再按 F4")
			p.setHint(err.Error())
		})
		return
	}
	if err != nil {
		p.update(func() { p.setHint("开播失败: " + err.Error()) })
		return
	}

	// 密钥近百字符，标签必须单独占一行：挤在一行里就整行放不下，
	// 在终端里选中复制出来是断的。
	text := "[yellow]推流地址与推流码[-]\n\n"
	for i, s := range streams {
		text += fmt.Sprintf("[white]%s[-]", s.Type)
		if i == 0 {
			text += "[yellow]（OBS 填这组）[-]"
		}
		text += "\n服务器\n" + s.Address + "\n密钥\n" + s.Key + "\n\n"
	}
	text += "[yellow]直播间已对外可见，下播按 F5；Esc 回控制面板[-]"
	p.update(func() {
		p.streams.SetText(text)
		p.pages.ShowPage("streams")
		p.app.SetFocus(p.streams)
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
		p.side.SetText("")
		p.pages.HidePage("streams")
		p.pages.ShowPage("control")
		p.app.SetFocus(p.tree)
		p.setHint("已下播")
	})
}

// ---------------------------------------------------------------- 二维码

// showQR 把二维码画成半格字符。
// 颜色用真彩色 #000000/#ffffff：用 black/white 名字会走终端调色板，
// 浅色主题下会和背景一样淡，扫不出来。
func (p *panel) showQR(content, title string) {
	code, err := qrcode.New(content, qrcode.Low)
	if err != nil {
		p.side.SetText("[white]" + title + "[-]\n\n" + content)
		return
	}
	bitmap := code.Bitmap() // 已含静默区

	var b strings.Builder
	b.WriteString("[yellow]" + title + "[-]\n\n")
	for y := 0; y < len(bitmap); y += 2 {
		b.WriteString("[#000000:#ffffff]")
		for x := 0; x < len(bitmap[y]); x++ {
			up := bitmap[y][x]
			down := y+1 < len(bitmap) && bitmap[y+1][x]
			switch {
			case up && down:
				b.WriteString("█")
			case up:
				b.WriteString("▀")
			case down:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteString("\n")
	}

	// 窄了会被 tview 截断 + 折行，二维码直接报废。留 2 列余量兜底。
	width := len(bitmap[0]) + 4
	if p.body != nil {
		p.body.ResizeItem(p.side, width, 0)
	}
	p.side.SetText(b.String())
}
