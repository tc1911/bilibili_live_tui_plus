package control

import (
	"os"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/live"
)

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

func testAreas() []live.ParentArea {
	return []live.ParentArea{
		{Name: "网游", List: []live.SubArea{{ID: 1, Name: "英雄联盟"}, {ID: 2, Name: "DOTA2"}}},
		{Name: "手游", List: []live.SubArea{{ID: 3, Name: "王者荣耀"}}},
	}
}

// newTestPanel 拼一个跟 Wrap 差不多的面板：测试里不跑 app.Run，只喂按键。
func newTestPanel(t *testing.T) (*panel, *tview.Application, *tview.Box) {
	t.Helper()
	config.Config.Background = "NONE"
	config.Config.FrameColor = "#bbbbbb"
	config.Config.InfoColor = "#ffffff"
	config.Config.RankColor = "#bbbbbb"

	app := tview.NewApplication()
	main := tview.NewBox()
	p := &panel{app: app, main: main, client: live.NewClient(""), onLogin: func() {}}
	p.toast = tview.NewModal()
	p.pages = tview.NewPages().
		AddPage("main", main, true, true).
		AddPage("control", p.build(), true, false).
		AddPage("toast", p.toast, true, false)
	app.SetInputCapture(p.onKey)
	p.setInfo()
	p.panelOpen = true
	p.syncTab()
	return p, app, main
}

func key(k tcell.Key) *tcell.EventKey { return tcell.NewEventKey(k, 0, tcell.ModNone) }

// Tab 换功能栏，Shift+Tab 是第一页和第二页来回切 —— 两个键管的事不一样，别串了。
func TestTabAndBacktab(t *testing.T) {
	p, app, main := newTestPanel(t)
	tab, backtab := key(tcell.KeyTab), key(tcell.KeyBacktab)

	// 弹幕页上 Tab 是输入框的键，不能抢；Shift+Tab 才是翻开第二页。
	p.panelOpen = false
	p.app.SetFocus(main)
	if got := p.onKey(tab); got != tab {
		t.Error("弹幕页上的 Tab 该原样交给主题")
	}
	if p.panelOpen {
		t.Fatal("弹幕页上按 Tab 不该翻开配置页")
	}
	p.onKey(backtab)
	if !p.panelOpen || p.tab != tabAccount {
		t.Fatalf("Shift+Tab 该翻开配置页并停在账号栏，open=%v tab=%d", p.panelOpen, p.tab)
	}

	// 配置页里 Tab 一栏一栏往下走，走完绕回账号。
	for i := 1; i < tabCount; i++ {
		p.onKey(tab)
		if p.tab != i {
			t.Fatalf("第 %d 次 Tab 停在 %d 栏", i, p.tab)
		}
	}
	p.onKey(tab)
	if p.tab != tabAccount {
		t.Errorf("绕一圈没回到账号栏，停在 %d", p.tab)
	}

	// 再按 Shift+Tab 收回弹幕页，焦点得还给主题。
	p.onKey(backtab)
	if p.panelOpen {
		t.Error("Shift+Tab 没收起配置页")
	}
	if app.GetFocus() != main {
		t.Errorf("收起后焦点 = %T，want 弹幕页", app.GetFocus())
	}
}

// 换栏时左栏高亮、右栏页面、顶部提示要一起动，不然三者会对不上。
func TestSyncTabKeepsThreePartsInSync(t *testing.T) {
	p, _, _ := newTestPanel(t)

	wantKeys := []string{"重新扫码", "确认该分区", "再回车 提交", "F5 下播"}
	for tab := 0; tab < tabCount; tab++ {
		p.openTab(tab)
		if !strings.Contains(p.keybar.GetText(true), wantKeys[tab]) {
			t.Errorf("第 %d 栏的提示里没有 %q", tab, wantKeys[tab])
		}
		if !strings.Contains(p.keybar.GetText(true), "Shift+Tab") {
			t.Errorf("第 %d 栏的提示里缺「Shift+Tab 回弹幕页」", tab)
		}
		if !strings.Contains(p.sidebar.GetText(true), "▸ "+tabNames[tab]) {
			t.Errorf("左栏没高亮 %q", tabNames[tab])
		}
		if !strings.Contains(p.contentBox.GetTitle(), tabNames[tab]) {
			t.Errorf("右栏标题 = %q，want %q", p.contentBox.GetTitle(), tabNames[tab])
		}
	}
}

// 「上下选、回车编辑、Esc 取消」是直播间信息那栏的核心，三样都得对。
func TestInfoFieldEditing(t *testing.T) {
	p, app, _ := newTestPanel(t)
	p.openTab(tabInfo)

	enter, esc := key(tcell.KeyEnter), key(tcell.KeyEscape)
	if p.fieldIdx != 0 {
		t.Fatalf("默认该停在「标题」那行，实际 %d", p.fieldIdx)
	}
	p.onKey(key(tcell.KeyDown))
	if p.fieldIdx != 1 {
		t.Errorf("按下键后选中的是第 %d 行，want 封面", p.fieldIdx)
	}
	p.onKey(key(tcell.KeyUp))
	if p.fieldIdx != 0 {
		t.Errorf("按上键后选中的是第 %d 行，want 标题", p.fieldIdx)
	}

	// 回车进编辑：焦点要真的落到输入框上，不然打字打不进去。
	p.editTitle.SetText("原来的标题")
	p.onKey(enter)
	if !p.editing || app.GetFocus() != p.editTitle {
		t.Fatalf("回车后 editing=%v focus=%T，want 编辑中的标题输入框", p.editing, app.GetFocus())
	}

	// Esc 取消：改了一半的字要还原，焦点还给内容区。
	p.editTitle.SetText("改了一半")
	p.onKey(esc)
	if p.editing {
		t.Error("Esc 没退出编辑")
	}
	if got := p.editTitle.GetText(); got != "原来的标题" {
		t.Errorf("Esc 之后输入框是 %q，want 还原成「原来的标题」", got)
	}
	// tview 的 SetFocus 会往下派给子控件，所以只断言「不在输入框上」——
	// 光标还留在输入框，用户后面按的键就全打进去了。
	if app.GetFocus() == p.editTitle {
		t.Error("取消编辑后焦点还留在输入框上")
	}
}

// Esc 一路退：提示 > 编辑 > 配置页，退到弹幕页就停住（退出是 Ctrl+C）。
func TestEscLayers(t *testing.T) {
	p, app, main := newTestPanel(t)
	p.openTab(tabAccount)
	esc := key(tcell.KeyEscape)

	p.showToast("按 Ctrl+C 退出")
	if got := p.onKey(esc); got != nil {
		t.Error("提示开着时 Esc 该被吞掉（收提示）")
	}
	if p.toastShown {
		t.Error("Esc 没收掉提示")
	}

	p.onKey(esc)
	if p.panelOpen {
		t.Error("Esc 没收起配置页")
	}
	if app.GetFocus() != main {
		t.Errorf("焦点 = %T，want 弹幕页", app.GetFocus())
	}
}

// 确认弹窗开着时 Esc 归它；关掉之后焦点要回到当前那一栏，
// 而不是丢给弹幕页 —— 那样配置页还在屏上、键却全落到背面。
func TestConfirmModalEsc(t *testing.T) {
	p, app, _ := newTestPanel(t)
	p.openTab(tabArea)

	p.onKey(key(tcell.KeyF4))
	if !p.pages.HasPage("confirm") {
		t.Fatal("F4 该弹出确认窗")
	}
	if got := p.onKey(key(tcell.KeyEscape)); got != nil {
		t.Error("确认窗开着时 Esc 该被吞掉（关弹窗）")
	}
	if p.pages.HasPage("confirm") {
		t.Error("Esc 没关掉确认窗")
	}
	if app.GetFocus() != p.tree {
		t.Errorf("关掉确认窗后焦点 = %T，want 分区树", app.GetFocus())
	}
}

// 骨架画出来要是那个样子：顶部提示条、左栏四个功能名、右栏标题跟着当前栏走。
func TestPanelLayout(t *testing.T) {
	config.Config.Background = "NONE"
	config.Config.FrameColor = "#bbbbbb"
	p := &panel{app: tview.NewApplication(), client: live.NewClient(""), onLogin: func() {}}
	root := p.build()
	p.setInfo()
	p.panelOpen = true
	p.tab = tabStream
	p.syncTab()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(100, 30)
	root.SetRect(0, 0, 100, 30)
	root.Draw(screen)
	screen.Show()

	// 宽字符占两格、第二格是空的，把空格去掉再找。
	flat := strings.ReplaceAll(screenText(screen, 100, 30), " ", "")
	for _, want := range []string{"按键提示", "功能", "▸推流码", "账号", "分区", "直播间信息", "F5下播"} {
		if !strings.Contains(flat, want) {
			t.Errorf("屏幕上找不到 %q", want)
		}
	}
}
