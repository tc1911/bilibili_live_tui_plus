// Package theme5 是新版主界面，布局按效果图来：
//
//	┌──────────────────────────────────────────────┐
//	│ 艺术字                            版本: vX.XX │
//	├──────────────┬───────────────────────────────┤
//	│ 直播间信息    │                               │
//	│ + 封面预览    │            弹幕们              │
//	├──────────────┤                               │
//	│   观众列表    │                               │
//	├──────────────┼───────────────────────────────┤
//	│ obs 推流状态  │          弹幕输入框             │
//	└──────────────┴───────────────────────────────┘
//
// 左边一栏固定占三分之一（Grid 的列写成 -1 / -2），底下那条横线是通的：
// 左下推流状态和右下输入框同属最后一行。
package theme5

import (
	"fmt"
	"os"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/getter"
	"github.com/tc1911/bilibili_live_tui_plus/sender"
	"github.com/tc1911/bilibili_live_tui_plus/ui/control"
	"github.com/tc1911/bilibili_live_tui_plus/ui/cover"
	"github.com/tc1911/bilibili_live_tui_plus/version"
)

// banner 是软件名的 TUI 艺术字，半格字符拼的。改的话注意每行都是 11 格宽。
var banner = []string{
	"██▄ █ █  █",
	"█ █ █ █  █",
	"██▀ █ █  █",
	"█ █ █ █  █",
	"██▀ █ ███ █",
}

var (
	bg               = tcell.ColorDefault
	submitHistory    = []string{}
	submitHistoryIdx = 0
)

// widgets 把要在 handler 里更新的控件打包带走，省得 draw 返回一长串。
type widgets struct {
	root     *tview.Grid
	input    *tview.InputField
	messages *tview.TextView
	viewers  *tview.TextView
	info     *tview.TextView
	stream   *tview.TextView
	cover    *cover.View
}

// box 给控件套一个带边框的壳，颜色跟主题统一。
func box(title string, item tview.Primitive) *tview.Flex {
	f := tview.NewFlex().AddItem(item, 0, 1, true)
	f.SetBorder(true)
	if title != "" {
		f.SetTitle(" " + title + " ")
	}
	f.SetTitleAlign(tview.AlignLeft)
	f.SetBorderColor(tcell.GetColor(config.Config.FrameColor))
	f.SetTitleColor(tcell.GetColor(config.Config.FrameColor))
	f.SetBackgroundColor(bg)
	return f
}

func draw(busChan chan getter.DanmuMsg, roomInfoChan chan getter.RoomInfo) *widgets {
	w := &widgets{}

	// 顶部：左边艺术字，右边版本号
	bannerView := tview.NewTextView().SetWrap(false)
	bannerView.SetText(strings.Join(banner, "\n"))
	bannerView.SetTextColor(tcell.GetColor(config.Config.InfoColor))
	bannerView.SetBackgroundColor(bg)

	versionView := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignRight)
	versionView.SetText("[yellow]版本: " + version.Version + "[-]")
	versionView.SetBackgroundColor(bg)

	header := tview.NewFlex().
		AddItem(bannerView, 0, 1, false).
		AddItem(versionView, 18, 0, false)

	// 左上：直播间信息 + 封面预览
	w.info = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	w.info.SetBackgroundColor(bg)

	w.cover = cover.New()
	w.cover.SetHint("封面加载中…")
	w.cover.SetBackgroundColor(bg)

	infoPane := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(w.info, 5, 0, false).
		AddItem(w.cover, 0, 1, false)

	// 左下：观众列表
	w.viewers = tview.NewTextView().SetDynamicColors(true)
	w.viewers.SetBackgroundColor(bg)

	// 右：弹幕们
	w.messages = tview.NewTextView().SetDynamicColors(true)
	w.messages.SetBackgroundColor(bg)

	// 底部左：obs 推流状态
	w.stream = tview.NewTextView().SetDynamicColors(true)
	w.stream.SetBackgroundColor(bg)

	// 底部右：弹幕输入框
	w.input = tview.NewInputField()
	w.input.SetFormAttributes(0, tcell.ColorDefault, bg, tcell.ColorDefault, bg)
	w.input.SetBackgroundColor(bg)
	w.input.SetPlaceholder("在这里打字，回车发送")
	w.input.SetPlaceholderTextColor(tcell.ColorGray)
	w.input.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			return
		}
		text := w.input.GetText()
		if strings.TrimSpace(text) != "" {
			go sender.SendMsg(config.Config.RoomId, text, busChan)
			submitHistory = append(submitHistory, text)
			if len(submitHistory) > 10 {
				submitHistory = submitHistory[1:]
			}
			submitHistoryIdx = len(submitHistory)
		}
		w.input.SetText("")
	})
	// 上下键翻输入历史、Ctrl+U 清空。这些键面板不拦，转到这儿来了。
	w.input.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Key() {
		case tcell.KeyUp:
			if submitHistoryIdx > 0 {
				submitHistoryIdx--
				w.input.SetText(submitHistory[submitHistoryIdx])
			}
			return nil
		case tcell.KeyDown:
			if submitHistoryIdx < len(submitHistory)-1 {
				submitHistoryIdx++
				w.input.SetText(submitHistory[submitHistoryIdx])
			} else {
				submitHistoryIdx = len(submitHistory)
				w.input.SetText("")
			}
			return nil
		case tcell.KeyCtrlU:
			w.input.SetText("")
			return nil
		}
		return ev
	})

	// 左栏自己再竖着切一刀：上面房间信息（带封面），下面观众列表。
	leftColumn := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(box("直播间信息", infoPane), 0, 2, false).
		AddItem(box("观众列表", w.viewers), 0, 1, false)

	grid := tview.NewGrid().
		SetRows(7, 0, 3).
		SetColumns(-1, -2).
		AddItem(box("", header), 0, 0, 1, 2, 0, 0, false).
		AddItem(leftColumn, 1, 0, 1, 1, 0, 0, false).
		AddItem(box("弹幕们", w.messages), 1, 1, 1, 1, 0, 0, false).
		AddItem(box("obs 推流状态", w.stream), 2, 0, 1, 1, 0, 0, false).
		AddItem(box("弹幕输入框", w.input), 2, 1, 1, 1, 0, 0, true)
	grid.SetBackgroundColor(bg)

	w.root = grid
	return w
}

func Run(busChan chan getter.DanmuMsg, roomInfoChan chan getter.RoomInfo, onLogin func()) {
	if config.Config.Background != "NONE" {
		bg = tcell.GetColor(config.Config.Background)
	}

	app := tview.NewApplication()
	w := draw(busChan, roomInfoChan)

	go danmuHandler(app, w.messages, busChan)
	go roomInfoHandler(app, w, roomInfoChan)

	root := control.Wrap(app, w.root, onLogin)
	if err := app.SetRoot(root, true).EnableMouse(false).Run(); err != nil {
		// 不 panic：TUI 起不来时给一句能看懂的话，堆栈留给日志。
		fmt.Fprintln(os.Stderr, "TUI 启动失败: "+err.Error())
	}
}
