package control

import (
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/ui/cover"
)

// buildInfoPane 拼「直播间信息」那一栏：两行字段，上下选、回车编辑。
// 字段本身就是普通的 InputField，平时不给焦点 —— 只有进编辑时才交给它，
// 这样键入的字不会被别的东西吃掉。
func (p *panel) buildInfoPane() *tview.Flex {
	bg := bgColor()

	p.editTitle = tview.NewInputField()
	p.editCover = tview.NewInputField()
	for _, f := range []*tview.InputField{p.editTitle, p.editCover} {
		f.SetBackgroundColor(bg)
		f.SetFieldBackgroundColor(bg)
		f.SetFormAttributes(0, tcell.ColorDefault, bg, tcell.ColorDefault, bg)
	}
	// 标题上限 40 字符，超了服务端只会回一句看不懂的报错，这里先拦住。
	p.editTitle.SetAcceptanceFunc(func(text string, _ rune) bool {
		return utf8.RuneCountInString(text) < maxTitleRunes
	})
	// 事件循环里取值，再交给 goroutine：输入框只由事件循环改，跨 goroutine 读是脏的。
	p.editTitle.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			p.finishEdit(strings.TrimSpace(p.editTitle.GetText()), func(v string) { go p.submitTitle(v) })
		}
	})
	p.editCover.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			p.finishEdit(strings.TrimSpace(p.editCover.GetText()), func(v string) { go p.submitCover(v) })
		}
	})
	p.markFields()

	// 封面图跟着终端大小现采样，所以直接给它剩下的高度，能画多大画多大。
	p.coverView = cover.New()
	p.coverView.SetHint("封面加载中…")
	p.coverView.SetBackgroundColor(bg)
	coverBox := tview.NewFlex().AddItem(p.coverView, 0, 1, false)
	coverBox.SetBorder(true)
	coverBox.SetTitle(" 当前封面 ")
	coverBox.SetTitleAlign(tview.AlignLeft)
	coverBox.SetBorderColor(frameColor())
	coverBox.SetTitleColor(frameColor())
	coverBox.SetBackgroundColor(bg)

	tip := tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	tip.SetText("[gray]标题上限 40 字；封面填本地图片路径或 .hdslb.com 链接，留空表示不改[-]")
	tip.SetBackgroundColor(bg)

	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.editTitle, 1, 0, false).
		AddItem(p.editCover, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(tip, 1, 0, false).
		AddItem(coverBox, 0, 1, false)
}

// markFields 用 ▸ 标出选中的那一行 —— 平时没有光标，得有个东西告诉用户选的是谁。
func (p *panel) markFields() {
	label := func(name string, idx int) string {
		if p.fieldIdx == idx {
			return "▸ " + name + " "
		}
		return "  " + name + " "
	}
	p.editTitle.SetLabel(label("标题", 0))
	p.editCover.SetLabel(label("封面", 1))
}

// moveField 上下换一行。只有两项，到头就绕回去。
func (p *panel) moveField(key tcell.Key) {
	if key == tcell.KeyUp {
		p.fieldIdx = (p.fieldIdx - 1 + 2) % 2
	} else {
		p.fieldIdx = (p.fieldIdx + 1) % 2
	}
	p.markFields()
}

func (p *panel) fieldOf(idx int) *tview.InputField {
	if idx == 0 {
		return p.editTitle
	}
	return p.editCover
}

// startEdit 回车进编辑：把焦点交给那一行，并记下原值好让 Esc 能还原。
func (p *panel) startEdit() {
	f := p.fieldOf(p.fieldIdx)
	p.editBackup = f.GetText()
	p.editing = true
	p.app.SetFocus(f)
	p.setHint("编辑中：回车提交，Esc 取消")
}

// cancelEdit 取消编辑，值还原成进编辑之前的样子。
func (p *panel) cancelEdit() {
	p.fieldOf(p.fieldIdx).SetText(p.editBackup)
	p.editing = false
	p.app.SetFocus(p.contentBox)
	p.setHint("已取消，没有改动")
}

// finishEdit 回车提交当前这一行。焦点先还给内容区，免得提交失败后键还落在输入框上。
func (p *panel) finishEdit(value string, submit func(string)) {
	p.editing = false
	p.app.SetFocus(p.contentBox)
	submit(value)
}

func (p *panel) loadTitle(roomID int64) {
	p.mu.Lock()
	room, err := p.client.Room(roomID)
	p.mu.Unlock()

	p.update(func() {
		if err != nil {
			// 读不到不等于改不了，输入框照用。
			p.setHint("读取当前标题失败，可以直接输入新标题: " + err.Error())
			return
		}
		// 慢网下用户可能已经敲上了，别把人家打的字冲掉。
		if p.editTitle.GetText() == "" {
			p.editTitle.SetText(room.Title)
		}
		// 封面只在地址变了的时候真去抓，Load 自己记得上一张是什么。
		p.coverView.Load(room.Cover, func() { p.app.Draw() })
		p.setHint("↑↓ 选一项，回车编辑，再回车提交；Esc 取消")
	})
}

func (p *panel) submitTitle(title string) {
	if title == "" {
		p.setStatus("标题不能为空")
		return
	}
	p.setStatus("正在提交标题…")

	p.mu.Lock()
	err := p.client.UpdateTitle(config.Config.RoomId, title)
	p.mu.Unlock()

	if err != nil {
		p.setStatus("改标题失败: " + err.Error())
		return
	}
	p.setStatus("标题已提交，生效要等几秒")
}

func (p *panel) submitCover(src string) {
	if src == "" {
		p.setStatus("封面留空，没有改动")
		return
	}

	p.setStatus("正在处理 " + filepath.Base(src) + " …")
	cover, err := p.uploadIfLocal(src)
	if err != nil {
		p.setStatus("上传失败: " + err.Error())
		return
	}

	p.mu.Lock()
	err = p.client.UpdateCover(cover)
	p.mu.Unlock()

	if err != nil {
		p.setStatus("改封面失败: " + err.Error())
		return
	}
	p.setStatus("封面已提交，生效要等几秒")
	// 把新的那张拉回来：Load 看到地址变了才会重新抓。
	go p.loadTitle(config.Config.RoomId)
}

// uploadIfLocal 填的已经是链接就直接用，否则当本地路径传图床 —— 封面只认
// .hdslb.com 下的图，别处来的地址服务端一律 100402 拒绝，所以这一步躲不掉。
func (p *panel) uploadIfLocal(src string) (string, error) {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return src, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	return p.client.UploadImage(expandHome(src))
}

// expandHome 把开头的 ~ 换成家目录。让用户在 TUI 里手敲一长串绝对路径不现实，
// 但 os.Open 不认 ~，这活儿得自己干。
func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}
