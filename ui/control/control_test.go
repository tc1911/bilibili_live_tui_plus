package control

import (
	"os"
	"strings"
	"testing"

	"github.com/tc1911/bilibili_live_tui_plus/config"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/tc1911/bilibili_live_tui_plus/live"
)

func testAreas() []live.ParentArea {
	return []live.ParentArea{
		{Name: "网游", List: []live.SubArea{{ID: 1, Name: "英雄联盟"}, {ID: 2, Name: "DOTA2"}}},
		{Name: "手游", List: []live.SubArea{{ID: 3, Name: "王者荣耀"}}},
	}
}

// 用户报的“分区没法选择”：父分区不可选 + 默认全收起时，整棵树都点不动 ——
// 上下键会整段跳过不可选节点，回车又只认叶子。
// 真门槛在 tview 的按键移动里（move 只停在可选节点），所以这里直接按键，不查字段。
func TestAreaTreeNavigable(t *testing.T) {
	areas := testAreas()
	root, selected := areaTree(areas, 0)
	if selected.GetReference() != nil {
		t.Error("没配过分区时不该有默认选中项")
	}

	parents := root.GetChildren()
	if len(parents) != 2 {
		t.Fatalf("父分区数 = %d, want 2", len(parents))
	}
	tree := tview.NewTreeView().SetRoot(root)
	tree.SetCurrentNode(root)
	tree.InputHandler()(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), nil)
	if tree.GetCurrentNode() != parents[0] {
		t.Errorf("从根按下键停在 %v，want 第一个父分区（用户就是卡在这里）", tree.GetCurrentNode())
	}
	// 默认全收起，所以下一个可选节点是第二个父分区，不是藏在第一个里的子分区。
	tree.InputHandler()(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), nil)
	if got := tree.GetCurrentNode(); got != parents[1] {
		t.Errorf("收起状态下从父分区按下键停在 %v，want 第二个父分区", got)
	}
	// 展开了才能进子分区。
	parents[0].SetExpanded(true)
	tree.SetCurrentNode(parents[0])
	tree.InputHandler()(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone), nil)
	if got := tree.GetCurrentNode(); got != parents[0].GetChildren()[0] {
		t.Errorf("展开后从父分区按下键停在 %v，want 它的第一个子分区", got)
	}

	// 配过的分区要自动展开、并把选中项定到它。
	root, selected = areaTree(areas, 3)
	if ref, ok := selected.GetReference().(*areaRef); !ok || ref.ID != 3 || ref.Name != "手游/王者荣耀" {
		t.Errorf("选中项 = %#v, want 手游/王者荣耀(3)", selected.GetReference())
	}
	if !root.GetChildren()[1].IsExpanded() {
		t.Error("配置里的分区所在父分区没展开")
	}
	// 用户报的“默认会展开网游那一栏”：根节点的子节点数在第一个父分区时恒为 0，
	// 拿它当“是不是第一个”的判据就会永远展开列表第一项，跟配置无关。
	if root.GetChildren()[0].IsExpanded() {
		t.Error("配置里没有的父分区不该默认展开")
	}
}

// 分区接口返回空列表时不能把整个 TUI 拖崩（loadAreas 原来直接取 [0][0]）。
func TestAreaTreeEmpty(t *testing.T) {
	root, selected := areaTree(nil, 0)
	if len(root.GetChildren()) != 0 || selected.GetReference() != nil {
		t.Fatalf("空列表应建出空树，got %d 个子节点", len(root.GetChildren()))
	}
}

// tview 的左右键只是移动选中项，不会展开/收起；收起后还得把事件交回去，
// 否则人就回不到父分区了。
func TestTreeKeyCapture(t *testing.T) {
	root, _ := areaTree(testAreas(), 0)
	tree := tview.NewTreeView().SetRoot(root)
	parent := root.GetChildren()[1]
	tree.SetCurrentNode(parent)
	if parent.IsExpanded() {
		t.Fatal("第二个父分区不该默认展开")
	}

	right := tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone)
	if got := treeKeyCapture(tree)(right); got != nil {
		t.Error("收起状态下按右键应吃掉事件（展开它）")
	}
	if !parent.IsExpanded() {
		t.Error("按右键没展开")
	}
	if got := treeKeyCapture(tree)(right); got == nil {
		t.Error("已展开时按右键应交回 tview，由它挪进第一个子分区")
	}

	left := tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)
	if got := treeKeyCapture(tree)(left); got != nil {
		t.Error("展开状态下按左键应吃掉事件（收起它）")
	}
	if parent.IsExpanded() {
		t.Error("按左键没收起")
	}
	if got := treeKeyCapture(tree)(left); got == nil {
		t.Error("已收起时按左键应交回 tview，由它跳回父节点")
	}

	// 叶子节点上左右键都不该被吞。
	tree.SetCurrentNode(parent.GetChildren()[0])
	for _, key := range []*tcell.EventKey{right, left} {
		if got := treeKeyCapture(tree)(key); got == nil {
			t.Errorf("叶子节点上按键 %v 被吞了，人就走不动了", key.Key())
		}
	}
}

// 用户报的“中间的弹窗不会关闭，还会变得无法交互”：Esc 被全局 capture 抢走了。
// 它跑在焦点分发之前，而 focused() 当时写成「焦点不是 main 就算面板」——
// 确认弹窗的按钮恰好不是 main，于是 Esc 关掉的是面板，弹窗连焦点被丢在屏上。
func TestConfirmModalEsc(t *testing.T) {
	config.Config.Background = "NONE"
	app := tview.NewApplication()
	main := tview.NewBox()
	p := &panel{app: app, main: main, client: live.NewClient(""), onLogin: func() {}}
	p.pages = tview.NewPages().
		AddPage("main", main, true, true).
		AddPage("control", p.build(), true, true)
	app.SetInputCapture(p.onKey)
	esc := tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)

	p.onKey(tcell.NewEventKey(tcell.KeyF4, 0, tcell.ModNone))
	if !p.pages.HasPage("confirm") {
		t.Fatal("F4 该弹出确认窗")
	}

	// 第一下 Esc：只关弹窗，焦点还给面板。
	if got := p.onKey(esc); got != nil {
		t.Error("确认窗开着时 Esc 该被吞掉（关弹窗）")
	}
	if p.pages.HasPage("confirm") {
		t.Error("Esc 没关掉确认窗")
	}
	if app.GetFocus() != p.tree {
		t.Errorf("关掉确认窗后焦点 = %T，want 分区树（否则面板是死的）", app.GetFocus())
	}

	// 第二下 Esc：面板还在，关掉它。
	p.onKey(esc)
	if app.GetFocus() != p.main {
		t.Errorf("再按 Esc 该关掉面板，焦点 = %T", app.GetFocus())
	}
}

// 信息页盖在面板之上，Esc 必须先关它、把焦点还给分区树。
// 判断顺序写错就会先关掉整个面板，输入框跟着人一起消失。
func TestEditPageEsc(t *testing.T) {
	config.Config.Background = "NONE"
	app := tview.NewApplication()
	main := tview.NewBox()
	p := &panel{app: app, main: main, client: live.NewClient(""), onLogin: func() {}}
	p.pages = tview.NewPages().
		AddPage("main", main, true, true).
		AddPage("control", p.build(), true, true).
		AddPage("edit", p.buildEdit(), true, false)
	app.SetInputCapture(p.onKey)
	esc := tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)

	p.onKey(tcell.NewEventKey(tcell.KeyF6, 0, tcell.ModNone))
	if app.GetFocus() != p.editTitle {
		t.Fatalf("F6 后焦点 = %T，want 标题输入框", app.GetFocus())
	}

	// 第一下 Esc：关信息页，控制面板还在。
	if got := p.onKey(esc); got != nil {
		t.Error("信息页开着时 Esc 该被吞掉（关它）")
	}
	if app.GetFocus() != p.tree {
		t.Errorf("关掉信息页后焦点 = %T，want 分区树（否则整个面板被一起关了）", app.GetFocus())
	}

	// 第二下 Esc：面板还在，关掉它。
	p.onKey(esc)
	if app.GetFocus() != p.main {
		t.Errorf("再按 Esc 该关掉面板，焦点 = %T", app.GetFocus())
	}
}

// 信息栏是带边框的盒子，可用高度 = 高度 - 2。之前定成 5 行只装得下 3 行文字，
// 「按键: F2 登录 …」那行被静默吃掉，界面上完全看不出 F2~F6 是干什么的。
func TestInfoShowsKeyHints(t *testing.T) {
	config.Config.Background = "NONE"
	config.Config.RoomId = 23333333
	config.Config.AreaName = ""

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(120, 30)

	p := &panel{app: tview.NewApplication(), client: live.NewClient(""), onLogin: func() {}}
	root := p.build()
	p.setInfo()
	root.SetRect(0, 0, 120, 30)
	root.Draw(screen)
	screen.Show()

	found := false
	for _, row := range strings.Split(screenText(screen, 120, 30), "\n") {
		if strings.Contains(row, "Esc") && strings.Contains(row, "F5") {
			found = true
		}
	}
	if !found {
		t.Error("信息栏里看不到按键提示（F5 / Esc 一行没被画出来）")
	}
}

// screenText 把整屏拉成一段文本。宽字符在模拟屏幕上占两格、第二格是空的，
// 所以调用方只按 ASCII 关键字找。
func screenText(screen tcell.Screen, width, height int) string {
	var b strings.Builder
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			m, _, _, _ := screen.GetContent(x, y)
			if m == 0 {
				m = ' '
			}
			b.WriteRune(m)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// 标题和封面挤在同一页上，Tab 必须能把焦点挪到下一项 ——
// 挪不动的话封面那一栏就永远够不着，等于白加。
func TestEditPageTabSwitches(t *testing.T) {
	config.Config.Background = "NONE"
	app := tview.NewApplication()
	p := &panel{app: app, main: tview.NewBox(), client: live.NewClient(""), onLogin: func() {}}
	p.pages = tview.NewPages().
		AddPage("main", p.main, true, true).
		AddPage("control", p.build(), true, true).
		AddPage("edit", p.buildEdit(), true, true)
	app.SetFocus(p.editTitle)

	// InputField 只在 Enter/Tab/Backtab 上调 done，所以这里直接喂按键。
	tab := tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
	backtab := tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)
	noop := func(tview.Primitive) {}

	p.editTitle.InputHandler()(tab, noop)
	if app.GetFocus() != p.editCover {
		t.Fatalf("标题上按 Tab 后焦点 = %T，want 封面输入框", app.GetFocus())
	}
	p.editCover.InputHandler()(tab, noop)
	if app.GetFocus() != p.editTitle {
		t.Errorf("封面上按 Tab 后焦点 = %T，want 标题输入框", app.GetFocus())
	}
	p.editTitle.InputHandler()(backtab, noop)
	if app.GetFocus() != p.editCover {
		t.Errorf("标题上按 Shift+Tab 后焦点 = %T，want 封面输入框", app.GetFocus())
	}
}

// TUI 里让人手敲一长串绝对路径不现实，~ 得自己展开 —— os.Open 不认它。
func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("取不到家目录")
	}
	cases := []struct{ in, want string }{
		{"~/图.png", home + "/图.png"},
		{"~", home},
		{"/tmp/a.png", "/tmp/a.png"},
		{"a.png", "a.png"},
		{"~other/a.png", "~other/a.png"}, // 别人的家目录不归我们管
	}
	for _, c := range cases {
		if got := expandHome(c.in); got != c.want {
			t.Errorf("expandHome(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// 面板收起时（已登录就是这个状态）按 Esc 没有任何东西可关。
// 以前是彻底没反应，看着像按键坏了 —— 得浮一句告诉用户退出靠 Ctrl+C。
func TestEscAtBottomNotifies(t *testing.T) {
	config.Config.Background = "NONE"
	app := tview.NewApplication()
	main := tview.NewBox()
	p := &panel{app: app, main: main, client: live.NewClient(""), onLogin: func() {}}
	p.toast = tview.NewModal()
	p.pages = tview.NewPages().
		AddPage("main", main, true, true).
		AddPage("control", p.build(), true, false).
		AddPage("toast", p.toast, true, false)
	app.SetInputCapture(p.onKey)

	esc := tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)
	if got := p.onKey(esc); got != esc {
		t.Error("退到底的 Esc 该原样交给主题，别吞掉")
	}

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(100, 30)
	p.pages.SetRect(0, 0, 100, 30)
	p.pages.Draw(screen)
	screen.Show()

	if !strings.Contains(screenText(screen, 100, 30), "Ctrl+C") {
		t.Error("Esc 退到底时屏幕上没出现退出提示")
	}

	// 提示浮着的时候再按一下 Esc，该立刻收掉它，而不是干等那两秒。
	if got := p.onKey(esc); got != nil {
		t.Error("提示开着时 Esc 该被吞掉（收掉提示）")
	}
	if p.toastShown {
		t.Error("第二下 Esc 没把提示收掉")
	}
}

// 右栏（扫码）只在有内容时才该占地方：空着还写死 46 列，分区树就被白白挤窄。
// 宽字符在模拟屏幕上占两格、第二格是空的，所以边框标题取出来是「扫 码」。
func TestSideBoxFollowsContent(t *testing.T) {
	config.Config.Background = "NONE"
	config.Config.RoomId = 23333333
	config.Config.AreaName = ""

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(100, 30)

	p := &panel{app: tview.NewApplication(), client: live.NewClient(""), onLogin: func() {}}
	root := p.build()
	p.setInfo()
	root.SetRect(0, 0, 100, 30)

	draw := func() string {
		screen.Clear()
		root.Draw(screen)
		screen.Show()
		return screenText(screen, 100, 30)
	}

	if strings.Contains(draw(), "扫 码") {
		t.Error("扫码框空着也占了一栏")
	}

	p.showSide("[white]扫我[-]", 30)
	if !strings.Contains(draw(), "扫 码") {
		t.Error("扫码框有内容了却没露出来")
	}

	p.hideSide()
	if strings.Contains(draw(), "扫 码") {
		t.Error("扫码框收起来之后还占着地方")
	}
}
