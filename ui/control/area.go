package control

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/live"
)

// areaRef 挂在分区树的叶子上。
type areaRef struct {
	ID   int64
	Name string
}

// defaultRoomID 与 config.defaultCfgFile 里的初始值一致。
// 它就是「用户还没设过自己的房间号」这个状态。
const defaultRoomID int64 = 23333333

func (p *panel) loadAreas() {
	// 同一时刻只拉一次：切栏和用户按 F3 可能一起来。
	if p.areasPending.Swap(true) {
		return
	}
	defer p.areasPending.Store(false)

	p.mu.Lock()
	areas, err := p.client.Areas()
	p.mu.Unlock()
	if err != nil {
		p.setStatus("分区列表获取失败: " + err.Error())
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
		p.setHint("↑↓ 选分区，←→ 展开/收起，回车确认；选中的分区会记进配置")
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
	p.setHint("开播分区已设为 " + ref.Name)
	if err := config.Save(); err != nil {
		p.setHint("分区已选中，但写入配置失败: " + err.Error())
	}
}
