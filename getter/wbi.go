package getter

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	myhttp "github.com/BYT0723/go-tools/http"

	"github.com/tidwall/gjson"
)

// mixinKeyEncTab 是 WBI 的固定置换表，来自 B 站 web 端 JS，不要改。
var mixinKeyEncTab = []int{
	46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49,
	33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13, 37, 48, 7, 16, 24, 55, 40,
	61, 26, 17, 0, 1, 60, 51, 30, 4, 22, 25, 54, 21, 56, 59, 6, 63, 57, 62, 11,
	36, 20, 34, 44, 52,
}

var (
	wbiMu     sync.Mutex
	wbiKey    string
	wbiExpire time.Time
)

// wbiMixinKey 用 nav 里的 img_key/sub_key 拼出 mixin key。
// B 站 2025-05-26 起强制 getDanmuInfo 带 WBI 签名，缺签名一律返 -352 风控、
// host_list 为空（老版本就是死在这上面）。key 每天轮换，所以只缓存半小时。
func wbiMixinKey(header http.Header) (string, error) {
	wbiMu.Lock()
	defer wbiMu.Unlock()

	if wbiKey != "" && time.Now().Before(wbiExpire) {
		return wbiKey, nil
	}

	_, body, err := myhttp.Get("https://api.bilibili.com/x/web-interface/nav", header, nil)
	if err != nil {
		return "", err
	}
	img := fileName(gjson.GetBytes(body, "data.wbi_img.img_url").String())
	sub := fileName(gjson.GetBytes(body, "data.wbi_img.sub_url").String())
	if len(img) < 32 || len(sub) < 32 {
		return "", fmt.Errorf("nav 未返回可用的 wbi_img，无法签名")
	}

	wbiKey = mixinKey(img, sub)
	wbiExpire = time.Now().Add(30 * time.Minute)
	return wbiKey, nil
}

// mixinKey 按置换表把 img_key / sub_key 拼成 mixin key。
func mixinKey(img, sub string) string {
	raw := img + sub
	var b strings.Builder
	for _, i := range mixinKeyEncTab {
		b.WriteByte(raw[i])
	}
	return b.String()[:32]
}

func fileName(u string) string {
	if i := strings.LastIndexByte(u, '/'); i >= 0 {
		u = u[i+1:]
	}
	return strings.TrimSuffix(u, ".png")
}

// wbiSign 补上 wts、算出 w_rid，返回可直接拼在 URL 后面的 query 串。
// wts 由调用方传入，方便测试固定时间戳下的签名结果。
func wbiSign(params map[string]string, mixinKey string, wts int64) string {
	params["wts"] = strconv.FormatInt(wts, 10)

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(k))
		b.WriteByte('=')
		// 值里的 !'()* 必须先剔掉，否则 w_rid 和服务端算出来的对不上
		b.WriteString(url.QueryEscape(strings.Map(func(r rune) rune {
			if strings.ContainsRune("!'()*", r) {
				return -1
			}
			return r
		}, params[k])))
	}

	sum := md5.Sum([]byte(b.String() + mixinKey))
	return b.String() + "&w_rid=" + hex.EncodeToString(sum[:])
}
