package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/tc1911/bilibili_live_tui_plus/config"
	"github.com/tc1911/bilibili_live_tui_plus/getter"
)

// 效果图定下来的骨架：顶部通栏、左边信息与观众、右边弹幕、底部推流状态与输入框。
// 框名写错或漏一个，用户一眼就能看出来，所以钉住。
func TestLayoutBoxes(t *testing.T) {
	config.Config.Background = "NONE"
	config.Config.FrameColor = "#bbbbbb"
	config.Config.InfoColor = "#ffffff"
	config.Config.RankColor = "#bbbbbb"

	w := draw(make(chan getter.DanmuMsg, 1), make(chan getter.RoomInfo, 1))

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(100, 30)
	w.root.SetRect(0, 0, 100, 30)
	w.root.Draw(screen)
	screen.Show()

	// 宽字符占两格、第二格为空，所以把空格去掉再找。
	flat := strings.ReplaceAll(screenText(screen, 100, 30), " ", "")
	for _, want := range []string{"直播间信息", "弹幕们", "观众列表", "obs推流状态", "弹幕输入框", "版本:", "██▄███"} {
		if !strings.Contains(flat, want) {
			t.Errorf("屏幕上找不到 %q", want)
		}
	}
}

// 左栏是三分之一、右栏三分之二，底下的竖线要和上面那条对齐。
func TestColumnSplit(t *testing.T) {
	config.Config.Background = "NONE"
	config.Config.FrameColor = "#bbbbbb"

	w := draw(make(chan getter.DanmuMsg, 1), make(chan getter.RoomInfo, 1))
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(96, 30) // 96 的 1/3 刚好 32，好数
	w.root.SetRect(0, 0, 96, 30)
	w.root.Draw(screen)
	screen.Show()

	// 弹幕那一行（第 8 行）上，第 32 列应该是分隔两个框的竖线。
	line := strings.Split(screenText(screen, 96, 30), "\n")[8]
	if got := []rune(line)[32]; got != '│' && got != '║' {
		t.Errorf("第 8 行第 32 列 = %q，want 竖线（左栏应占三分之一）", got)
	}
}

// screenText 把整屏拉成一段文本。宽字符的第二格是空的，调用方找 ASCII 关键字即可。
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
