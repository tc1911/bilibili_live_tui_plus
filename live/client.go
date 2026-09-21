// Package live 封装哔哩哔哩直播控制接口：扫码登录、分区列表、开播并取回推流码。
//
// 签名与开播流程对齐 bili-live-hime 的 src/lib/app-sign.ts 与 src/api/live.ts。
package live

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	liveBase     = "https://api.live.bilibili.com"
	mainBase     = "https://api.bilibili.com"
	passportBase = "https://passport.bilibili.com"

	// 直播姬 (bilibili link) 的 appkey / appsec，开播相关接口必须用这对签名。
	appKey = "aae92bc66f3edfab"
	appSec = "af125a0d5279fd576c1b4418a3e8276d"

	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
	httpTimeout = 15 * time.Second
)

// cookieNames 是持久化到 config.toml 的字段顺序。
var cookieNames = []string{"SESSDATA", "bili_jct", "DedeUserID", "DedeUserID__ckMd5", "sid"}

// ApiError 是接口返回的非 0 code。
type ApiError struct {
	Code int
	Msg  string
	Data json.RawMessage // 60024 / 60043 的验证信息挂在这里
}

func (e *ApiError) Error() string { return fmt.Sprintf("%s (%d)", e.Msg, e.Code) }

// Client 持有登录凭据，并发调用前请套锁。
type Client struct {
	cookies map[string]string
	http    *http.Client
}

// NewClient 从 config.toml 里的 Cookie 串建客户端。
func NewClient(cookie string) *Client {
	return &Client{cookies: parseCookie(cookie), http: &http.Client{Timeout: httpTimeout}}
}

func parseCookie(s string) map[string]string {
	out := make(map[string]string)
	for _, attr := range strings.Split(s, ";") {
		kv := strings.SplitN(strings.TrimSpace(attr), "=", 2)
		if len(kv) == 2 && kv[0] != "" {
			out[kv[0]] = kv[1]
		}
	}
	return out
}

// LoggedIn 只看两个必需字段，是否真的有效交给 Nav 判断。
func (c *Client) LoggedIn() bool {
	return c.cookies["SESSDATA"] != "" && c.cookies["bili_jct"] != ""
}

// Cookie 拼回可写进 config.toml 的 Cookie 串。
func (c *Client) Cookie() string {
	parts := make([]string, 0, len(cookieNames))
	for _, name := range cookieNames {
		if v := c.cookies[name]; v != "" {
			parts = append(parts, name+"="+v)
		}
	}
	return strings.Join(parts, "; ")
}

func (c *Client) csrf() string { return c.cookies["bili_jct"] }

// encodeParams 在需要签名时追加 appkey 并附上 md5(表单串 + appSec)。
//
// 用 url.Values.Encode 而不是手写表单序列化：它已经按 key 排序、空格转 +，
// 与 JS URLSearchParams 一致；只是 * 和 ~ 的转义与 WHATWG 规范有出入，
// 而本文件所有取值都是字母数字，碰不到这两个字符。
// url.Values 每次 Encode 都会重新排序，所以签名和发送用同一串。
func encodeParams(params url.Values, sign bool) string {
	if len(params) == 0 {
		return ""
	}
	if !sign {
		return params.Encode()
	}
	params.Set("appkey", appKey)
	sum := md5.Sum([]byte(params.Encode() + appSec))
	return params.Encode() + "&sign=" + hex.EncodeToString(sum[:])
}

// do 发一次请求并解开 {code,message,data} 外壳。sign 为真时走 app 签名规则。
func (c *Client) do(method, base, path string, params map[string]string, sign bool) (json.RawMessage, error) {
	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	rawURL := base + path
	body := ""
	if method == http.MethodGet {
		if q := encodeParams(values, sign); q != "" {
			rawURL += "?" + q
		}
	} else {
		body = encodeParams(values, sign)
	}

	req, err := http.NewRequest(method, rawURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("user-agent", userAgent)
	req.Header.Set("accept", "*/*")
	req.Header.Set("origin", base)
	if body != "" {
		req.Header.Set("content-type", "application/x-www-form-urlencoded; charset=UTF-8")
	}
	if len(c.cookies) > 0 {
		parts := make([]string, 0, len(c.cookies))
		for k, v := range c.cookies {
			parts = append(parts, k+"="+v)
		}
		req.Header.Set("cookie", strings.Join(parts, "; "))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s 失败: %w", path, err)
	}
	defer resp.Body.Close()

	// 扫码登录的凭据就是靠这里带回来的。
	for _, ck := range resp.Cookies() {
		if ck.Value != "" && !strings.EqualFold(ck.Value, "deleted") {
			c.cookies[ck.Name] = ck.Value
		}
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Msg     string          `json:"msg"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("接口 %s 返回非 JSON (HTTP %d): %s", path, resp.StatusCode, truncate(string(raw), 200))
	}
	if payload.Code != 0 {
		msg := payload.Message
		if msg == "" {
			msg = payload.Msg
		}
		return nil, &ApiError{Code: payload.Code, Msg: msg, Data: payload.Data}
	}
	return payload.Data, nil
}

func (c *Client) get(base, path string, params map[string]string, sign bool) (json.RawMessage, error) {
	return c.do(http.MethodGet, base, path, params, sign)
}

func (c *Client) post(base, path string, form map[string]string, sign bool) (json.RawMessage, error) {
	return c.do(http.MethodPost, base, path, form, sign)
}

func ms() string { return strconv.FormatInt(time.Now().UnixMilli(), 10) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ID 兼容接口把 id 返回成字符串或数字两种写法。
type ID int64

func (i *ID) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*i = 0
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	*i = ID(v)
	return err
}

func (i ID) String() string { return strconv.FormatInt(int64(i), 10) }
