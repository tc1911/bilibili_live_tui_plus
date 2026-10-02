// Package control 在主题上叠一层配置页，布局跟效果图一致：
//
//	┌────────────────────────────────────────────────────┐
//	│ 按键提示 / 最近一条消息                              │
//	├──────────────┬─────────────────────────────────────┤
//	│ ▸ 账号       │                                     │
//	│   分区       │             实际内容                 │
//	│   直播间信息 │                                     │
//	│   推流码     │                                     │
//	└──────────────┴─────────────────────────────────────┘
//
// Tab / Shift+Tab 换栏，↑↓ 在栏里选，回车编辑、再回车提交、Esc 取消或返回。
// Esc 退到弹幕页就停住，退出只有 Ctrl+C；F4 开播、F5 下播在哪一栏都能按。
package control

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/getter"
	"github.com/tc1911/bilibili_live_tui_plus/live"
	"github.com/tc1911/bilibili_live_tui_plus/ui/cover"
)

// 功能栏。顺序就是左栏从上到下的顺序，tabNames / tabHints 都按它排。
const (
	tabAccount = iota
	tabArea
	tabInfo
	tabStream
	tabCount
)

var tabNames = []string{"账号", "分区", "直播间信息", "推流码"}

// tabHints 是每栏的按键提示，写在顶部那条里。
var tabHints = []string{
	"回车 重新扫码    Ctrl+R 刷新房间信息    Tab 换功能    Shift+Tab 回弹幕页    Esc 返回",
	"↑↓ 选分区    ←→ 展开/收起    回车 确认该分区    Tab 换功能    Shift+Tab 回弹幕页",
	"↑↓ 选一项    回车 编辑    再回车 提交    Esc 取消    Tab 换功能    Shift+Tab 回弹幕页",
	"F4 开播    F5 下播    Tab 换功能    Shift+Tab 回弹幕页    Esc 返回",
}

const (
	// maxTitleRunes 是服务端给的标题上限（bilibili-API-collect 的 live/manage.md）。
	maxTitleRunes = 40
	sidebarWidth  = 16
)

type panel struct {
	app     *tview.Application
	pages   *tview.Pages
	main    tview.Primitive
	client  *live.Client
	onLogin func()

	// 页面状态
	tab        int    // 当前功能栏
	panelOpen  bool   // 面板是不是正露着
	editBackup string // 进编辑前的原值，Esc 用它还原
	fieldIdx   int    // 「直播间信息」里选中的那一行
	editing    bool   // 正在编辑输入框（这时键要留给它）

	// 控件
	keybar      *tview.TextView // 顶部第一行：按键提示
	hint        *tview.TextView // 顶部第二行：最近一条消息
	sidebar     *tview.TextView // 左栏功能列表
	content     *tview.Pages    // 右栏内容，一栏一页
	contentBox  *tview.Flex
	accountView *tview.TextView // 账号栏第一行：登录状态
	side        *tview.TextView // 账号栏：二维码
	tree        *tview.TreeView // 分区栏
	streams     *tview.TextView // 推流码栏
	editTitle   *tview.InputField
	editCover   *tview.InputField
	coverView   *cover.View // 「直播间信息」栏下半格：当前封面画出来

	toast      *tview.Modal // 浮在最上层的临时提示
	toastGen   int          // 递增作废旧的隐藏定时器
	toastShown bool

	mu      sync.Mutex // 串行化接口调用
	qrGen   int        // 递增即作废旧的扫码轮询
	account string

	// 下面三个给「需要什么显示什么」用：它们由后台 goroutine 改，
	// 而 autoFill / fillTab 在事件循环里跑，普通 bool 会撞车，所以用 atomic。
	loggedIn     atomic.Bool
	loginPending atomic.Bool // 正在等扫码，别再生一张二维码
	areasPending atomic.Bool // 正在拉分区列表
}

func frameColor() tcell.Color { return tcell.GetColor(config.Config.FrameColor) }

// bgColor 跟主题用同一个背景色。
// 不刷的控件会落到 tview 的默认背景色（一个固定颜色），在终端上就是一块
// 和主题不搭的色块；NONE 表示「用终端自己的背景」（tcell.ColorDefault）。
func bgColor() tcell.Color {
	if config.Config.Background != "NONE" {
		return tcell.GetColor(config.Config.Background)
	}
	return tcell.ColorDefault
}

// Wrap 把主题根组件包进 Pages 并挂上全局快捷键。
// 扫码登录成功后调用 onLogin，让 getter / sender 开始工作。
func Wrap(app *tview.Application, root tview.Primitive, onLogin func()) *tview.Pages {
	p := &panel{
		app:     app,
		main:    root,
		client:  live.NewClient(config.Config.Cookie),
		onLogin: onLogin,
		account: "未登录（回车扫码）",
	}

	p.toast = tview.NewModal()
	p.pages = tview.NewPages().
		AddPage("main", root, true, true).
		AddPage("control", p.build(), true, false).
		AddPage("toast", p.toast, true, false)
	app.SetInputCapture(p.onKey)

	p.syncTab()
	if p.client.LoggedIn() {
		p.loggedIn.Store(true)
		go p.refreshAccount()
	} else {
		// 没登录就把二维码直接摆出来，别让人先去找 F2。
		p.pages.ShowPage("control")
		p.panelOpen = true
		p.setHint("未登录：回车重新生成二维码，用哔哩哔哩 App 扫一下")
		p.fillTab()
	}
	return p.pages
}

// build 按效果图拼出配置页：顶部提示条，左边功能列，右边内容区。
func (p *panel) build() *tview.Flex {
	bg := bgColor()

	p.keybar = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	p.keybar.SetBackgroundColor(bg)
	p.hint = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	p.hint.SetBackgroundColor(bg)
	bar := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.keybar, 1, 0, false).
		AddItem(p.hint, 1, 0, false)
	bar.SetBorder(true)
	bar.SetTitle(" 按键提示 ")
	bar.SetTitleAlign(tview.AlignLeft)
	bar.SetBorderColor(frameColor())
	bar.SetTitleColor(frameColor())
	bar.SetBackgroundColor(bg)

	p.sidebar = tview.NewTextView().SetDynamicColors(true)
	p.sidebar.SetBackgroundColor(bg)

	// 账号栏：上面一行账号状态，下面是二维码（横着居中）
	p.accountView = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	p.accountView.SetBackgroundColor(bg)
	p.side = tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter).SetWrap(false)
	p.side.SetBackgroundColor(bg)
	accountPane := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.accountView, 2, 0, false).
		AddItem(p.side, 0, 1, false)

	// 分区栏：还是那棵树
	p.tree = tview.NewTreeView()
	p.tree.SetSelectedFunc(p.pickArea)
	p.tree.SetInputCapture(treeKeyCapture(p.tree))
	p.tree.SetBackgroundColor(bg)

	// 推流码栏
	p.streams = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	p.streams.SetBackgroundColor(bg)

	p.content = tview.NewPages().
		AddPage("account", accountPane, true, true).
		AddPage("area", p.tree, true, false).
		AddPage("info", p.buildInfoPane(), true, false).
		AddPage("stream", p.streams, true, false)

	p.contentBox = tview.NewFlex().AddItem(p.content, 0, 1, true)
	p.contentBox.SetBorder(true)
	p.contentBox.SetTitleAlign(tview.AlignLeft)
	p.contentBox.SetBorderColor(frameColor())
	p.contentBox.SetTitleColor(frameColor())
	p.contentBox.SetBackgroundColor(bg)

	sideBox := tview.NewFlex().AddItem(p.sidebar, 0, 1, false)
	sideBox.SetBorder(true)
	sideBox.SetTitle(" 功能 ")
	sideBox.SetTitleAlign(tview.AlignLeft)
	sideBox.SetBorderColor(frameColor())
	sideBox.SetTitleColor(frameColor())
	sideBox.SetBackgroundColor(bg)

	body := tview.NewFlex().
		AddItem(sideBox, sidebarWidth, 0, false).
		AddItem(p.contentBox, 0, 1, true)

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(bar, 4, 0, false).
		AddItem(body, 0, 1, true)
	root.SetBackgroundColor(bg)
	return root
}

// syncTab 把左栏高亮、右栏页面、提示语都对齐到 p.tab。
func (p *panel) syncTab() {
	var b strings.Builder
	for i, name := range tabNames {
		if i == p.tab {
			b.WriteString("[yellow]▸ " + name + "[-]\n")
		} else {
			b.WriteString("[gray]  " + name + "[-]\n")
		}
	}
	p.sidebar.SetText(b.String())

	switch p.tab {
	case tabAccount:
		p.content.SwitchToPage("account")
	case tabArea:
		p.content.SwitchToPage("area")
	case tabInfo:
		p.content.SwitchToPage("info")
	case tabStream:
		p.content.SwitchToPage("stream")
	}
	p.contentBox.SetTitle(" " + tabNames[p.tab] + " ")
	p.keybar.SetText(tabHints[p.tab])

	// 焦点跟着栏走：分区栏要交还给树，别的栏把焦点收在内容区上（编辑时再交给输入框）。
	if p.tab == tabArea {
		p.app.SetFocus(p.tree)
	} else {
		p.app.SetFocus(p.contentBox)
	}
}

// openTab 露出配置页并切到指定栏。
func (p *panel) openTab(tab int) {
	p.pages.ShowPage("control")
	p.panelOpen = true
	p.tab = tab
	p.syncTab()
	p.fillTab()
}

// cycleTab 用 Tab / Shift+Tab 换栏，到头就绕回去。
func (p *panel) cycleTab(step int) {
	p.tab = (p.tab + step + tabCount) % tabCount
	p.editing = false
	p.syncTab()
	p.fillTab()
}

// fillTab 让当前栏「需要什么显示什么」：该拉的自己拉，不用先按一堆键。
func (p *panel) fillTab() {
	switch p.tab {
	case tabAccount:
		if !p.loggedIn.Load() {
			go p.login()
		}
	case tabArea:
		if p.tree.GetRoot() == nil {
			go p.loadAreas()
		}
	case tabInfo:
		if p.editTitle.GetText() == "" {
			go p.loadTitle(config.Config.RoomId)
		}
	case tabStream:
		if p.streams.GetText(false) == "" {
			go p.loadStreamStatus()
		}
	}
}

func (p *panel) setHint(text string) {
	// 命令框只有一行，换行会把下面的内容顶掉。
	p.hint.SetText(strings.ReplaceAll(text, "\n", " "))
}

func (p *panel) closePanel() {
	p.editing = false
	p.panelOpen = false
	p.pages.HidePage("control")
	p.app.SetFocus(p.main)
}

// onKey 是全局输入捕获，跑在焦点分发之前，所以顺序很讲究：
// 确认弹窗 > 提示 > 编辑中 > 换栏/返回。
func (p *panel) onKey(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyF4:
		p.openPanel()
		p.confirmStart()
		return nil
	case tcell.KeyF5:
		p.openPanel()
		go p.stopLive()
		return nil
	case tcell.KeyCtrlR:
		// 房间信息本来每 30 秒自己拉一次，想立刻看新的就按这个。
		// 它不阻塞 —— Refresh 里是「塞得进就塞，塞不进算了」。
		getter.Refresh()
		p.setHint("已请求刷新房间信息")
		return nil
	case tcell.KeyF2:
		p.openTab(tabAccount)
		return nil
	case tcell.KeyF3:
		p.openTab(tabArea)
		return nil
	case tcell.KeyF6:
		p.openTab(tabInfo)
		return nil
	}

	if !p.panelOpen {
		// 弹幕页上按 Shift+Tab 就是「翻开第二页」。
		if ev.Key() == tcell.KeyBacktab {
			p.openPanel()
			return nil
		}
		return ev
	}

	switch ev.Key() {
	case tcell.KeyBacktab:
		// Shift+Tab 在第一页（弹幕）和第二页（配置）之间来回切。
		p.closePanel()
		return nil
	case tcell.KeyTab:
		p.cycleTab(1)
		return nil
	case tcell.KeyEscape:
		// 确认弹窗开着时 Esc 归它。必须先判：全局 capture 跑在焦点分发之前，
		// 底下的分支会把 Esc 当成「关配置页」，弹窗就被连人带焦点丢在屏上。
		if p.pages.HasPage("confirm") {
			p.closeConfirm()
			return nil
		}
		if p.toastShown {
			p.hideToast()
			return nil
		}
		if p.editing {
			p.cancelEdit()
			return nil
		}
		p.closePanel()
		return nil
	case tcell.KeyEnter:
		if p.tab == tabInfo && !p.editing {
			p.startEdit()
			return nil
		}
		if p.tab == tabAccount && !p.editing {
			go p.login()
			return nil
		}
	case tcell.KeyUp, tcell.KeyDown:
		switch {
		case p.tab == tabInfo && !p.editing:
			p.moveField(ev.Key())
			return nil
		case p.tab == tabAccount || p.tab == tabStream:
			// 这两栏本来没有可上下选的东西，就顺手拿来换功能。
			if ev.Key() == tcell.KeyUp {
				p.cycleTab(-1)
			} else {
				p.cycleTab(1)
			}
			return nil
		}
	}
	return ev
}

// openPanel 露出配置页（不换栏），已经开着就什么都不做。
func (p *panel) openPanel() {
	if p.panelOpen {
		return
	}
	p.pages.ShowPage("control")
	p.panelOpen = true
	p.syncTab()
}

// ---------------------------------------------------------------- 提示

// showToast 在最上层浮一句提示，两秒后自己收；想立刻收掉就再按一下 Esc。
// 不能用顶部那行 hint：这会儿配置页已经收起，用户看不到它。
func (p *panel) showToast(text string) {
	p.toastGen++
	gen := p.toastGen
	p.toast.SetText(text)
	p.pages.ShowPage("toast")
	p.toastShown = true
	// 连按 Esc 要重新计时，否则上一条的定时器会把新的一条提前收走。
	time.AfterFunc(2*time.Second, func() {
		p.update(func() {
			if gen == p.toastGen {
				p.hideToast()
			}
		})
	})
}

func (p *panel) hideToast() {
	// 顺手把代数加一：还在倒计的定时器看到对不上，就不会再来动这条。
	p.toastGen++
	p.toastShown = false
	p.pages.HidePage("toast")
}

// update 供后台 goroutine 改界面用；在事件循环里直接改会死锁。
func (p *panel) update(fn func()) { p.app.QueueUpdateDraw(fn) }

// setStatus 写顶部第二条。跟 setHint 分开只是为了读起来清楚。
func (p *panel) setStatus(text string) { p.update(func() { p.setHint(text) }) }

// setInfo 把账号那行刷出来。
func (p *panel) setInfo() {
	p.accountView.SetText("[white]账号:[-] " + p.account)
}
