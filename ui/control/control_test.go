package control

import (
	"testing"

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
