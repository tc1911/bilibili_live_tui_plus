// Package cover 把直播间封面画进终端。
//
// 终端没有像素，只能拿一个字符格当两个像素使：上半个用前景色、下半个用背景色，
// 字符用 "▀"。所以抓完图先缩到一个小尺寸存着，真正画的时候再按控件当前大小现采样 ——
// 终端一拉伸，画面自己就跟着变，不用重新抓图。
package cover

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// 抓下来先缩到这个上限（像素）。控件最大也就百来格宽，再大是白算。
const (
	maxSampleW = 96
	maxSampleH = 140
	fetchLimit = 8 << 20 // 封面再大也不该超过 8M
	userAgent  = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36"
)

// View 按自己的实际尺寸画封面。
type View struct {
	*tview.Box

	mu     sync.Mutex
	small  *image.RGBA // 缩好的小图，画的时候直接从它采样
	hint   string      // 还没图时显示的话
	loaded string      // 已经抓过的地址，同一张图不重复抓
}

func New() *View {
	return &View{Box: tview.NewBox(), hint: "封面加载中…"}
}

// SetHint 换掉「还没图」时显示的文字。
func (v *View) SetHint(text string) {
	v.mu.Lock()
	v.hint = text
	v.mu.Unlock()
}

// Load 异步抓一张封面。同一个地址只会抓一次，抓失败就把原因写在框里。
// done 在状态变好后回调，调用方拿它去 app.Draw()。
func (v *View) Load(rawURL string, done func()) {
	rawURL = normalize(rawURL)
	v.mu.Lock()
	if rawURL == "" || rawURL == v.loaded {
		v.mu.Unlock()
		return
	}
	v.loaded = rawURL
	v.mu.Unlock()

	go func() {
		img, err := fetch(rawURL)

		v.mu.Lock()
		if err != nil {
			v.small = nil
			v.hint = "封面加载失败: " + err.Error()
		} else {
			v.small = downscale(img, maxSampleW, maxSampleH)
			v.hint = ""
		}
		v.mu.Unlock()

		if done != nil {
			done()
		}
	}()
}

// normalize 补上 // 开头的协议：房间接口有时给的是协议相对地址。
func normalize(rawURL string) string {
	if strings.HasPrefix(rawURL, "//") {
		return "https:" + rawURL
	}
	return rawURL
}

func fetch(rawURL string) (image.Image, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("user-agent", userAgent)
	req.Header.Set("referer", "https://live.bilibili.com/")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	img, _, err := image.Decode(io.LimitReader(resp.Body, fetchLimit))
	if err != nil {
		return nil, fmt.Errorf("解不开这张图: %w", err)
	}
	return img, nil
}

// downscale 按区域平均缩到不超过 maxW×maxH，保持比例。
// 封面是几 MB 的照片，直接拿原图逐像素采样太慢，先缩一遍。
func downscale(src image.Image, maxW, maxH int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return nil
	}

	w, h := maxW, maxH
	if sw*maxH > sh*maxW {
		h = sh * maxW / sw // 原图更宽，以宽为准
	} else {
		w = sw * maxH / sh
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if w > sw {
		w = sw
	}
	if h > sh {
		h = sh
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1 := b.Min.Y+y*sh/h, b.Min.Y+(y+1)*sh/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0, x1 := b.Min.X+x*sw/w, b.Min.X+(x+1)*sw/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var sr, sg, sb, sa, n uint64
			for sy := y0; sy < y1 && sy < b.Max.Y; sy++ {
				for sx := x0; sx < x1 && sx < b.Max.X; sx++ {
					r, g, bl, a := src.At(sx, sy).RGBA()
					sr, sg, sb, sa, n = sr+uint64(r), sg+uint64(g), sb+uint64(bl), sa+uint64(a), n+1
				}
			}
			if n == 0 {
				continue
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(sr / n >> 8), G: uint8(sg / n >> 8),
				B: uint8(sb / n >> 8), A: uint8(sa / n >> 8),
			})
		}
	}
	return dst
}

func (v *View) Draw(screen tcell.Screen) {
	v.Box.DrawForSubclass(screen, v)
	x, y, w, h := v.GetInnerRect()
	if w <= 0 || h <= 0 {
		return
	}

	v.mu.Lock()
	img, hint := v.small, v.hint
	v.mu.Unlock()

	if img == nil {
		if hint != "" {
			tview.Print(screen, hint, x, y, w, tview.AlignLeft, tcell.ColorYellow)
		}
		return
	}

	// 一格 = 1×2 像素，把图按比例摆进 w×(2h) 的框里居中，别拉变形。
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	dw, dh := w, 2*h
	if iw*2*h > ih*w {
		dh = ih * w / iw
	} else {
		dw = iw * 2 * h / ih
	}
	if dw > w {
		dw = w
	}
	if dh > 2*h {
		dh = 2 * h
	}
	ox := x + (w-dw)/2
	oy := y + (h-dh/2)/2

	for cy := 0; cy < dh/2; cy++ {
		for cx := 0; cx < dw; cx++ {
			top := sample(img, cx, cy*2, dw, dh)
			bottom := sample(img, cx, cy*2+1, dw, dh)
			style := tcell.StyleDefault.Foreground(top).Background(bottom)
			screen.SetContent(ox+cx, oy+cy, '▀', nil, style)
		}
	}
}

// sample 取缩放后第 (cx, cy) 个像素的色，cx/cy 是在 dw×dh 的网格里。
func sample(img *image.RGBA, cx, cy, dw, dh int) tcell.Color {
	b := img.Bounds()
	sx := b.Min.X + cx*b.Dx()/dw
	sy := b.Min.Y + cy*b.Dy()/dh
	r, g, bl, _ := img.At(sx, sy).RGBA()
	return tcell.NewRGBColor(int32(r>>8), int32(g>>8), int32(bl>>8))
}
