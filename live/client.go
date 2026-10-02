// Package live 封装哔哩哔哩直播控制接口：扫码登录、分区列表、开播并取回推流码。
//
// 签名与开播流程对齐 bili-live-hime 的 src/lib/app-sign.ts 与 src/api/live.ts。
package live

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

// cookieHeader 把手上所有 cookie 拼成一个请求头。
// 给的是全集而不是 Cookie() 那几个字段：风控认 buvid 之类的旁路 cookie，
// 只带 SESSDATA/bili_jct 容易被挡。
func (c *Client) cookieHeader() string {
	parts := make([]string, 0, len(c.cookies))
	for k, v := range c.cookies {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
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
	if cookie := c.cookieHeader(); cookie != "" {
		req.Header.Set("cookie", cookie)
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
	return unwrap(path, resp.StatusCode, raw)
}

// unwrap 解开所有接口共用的 {code,message,data} 外壳，code 非 0 一律转成 *ApiError。
// 传图走 multipart，用不上 do，但外壳是同一套，所以拎出来两边共用。
func unwrap(path string, status int, raw []byte) (json.RawMessage, error) {
	var payload struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Msg     string          `json:"msg"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("接口 %s 返回非 JSON (HTTP %d): %s", path, status, truncate(string(raw), 200))
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

// UploadImage 把本地图片传到 B 站图床，返回 .hdslb.com 下的地址。
// 改封面躲不开这一步：UpdatePreLiveInfo 的 cover 只认 .hdslb.com 的链接，
// 别的地址一律回 100402（图片地址不合法）。表单字段对齐网页端上传组件。
func (c *Client) UploadImage(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	// bucket 是分桶参数，缺了会报「图片位置不对」这类错；openplatform 是公开图床桶，
	// 返回的同样是 i0.hdslb.com 的地址，能被封面接口接受。
	if err := w.WriteField("bucket", "openplatform"); err != nil {
		return "", err
	}
	if err := w.WriteField("csrf", c.csrf()); err != nil {
		return "", err
	}
	part, err := w.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, f); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, mainBase+"/x/upload/web/image", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", w.FormDataContentType())
	req.Header.Set("user-agent", userAgent)
	req.Header.Set("accept", "*/*")
	req.Header.Set("origin", mainBase)
	if cookie := c.cookieHeader(); cookie != "" {
		req.Header.Set("cookie", cookie)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("上传图片失败: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	data, err := unwrap("/x/upload/web/image", resp.StatusCode, raw)
	if err != nil {
		return "", err
	}

	var out struct {
		ImageURL string `json:"image_url"`
		Location string `json:"location"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	url := out.ImageURL
	if url == "" {
		// location 是 http 的，统一升成 https，免得被下游当不安全链接拒掉。
		url = strings.Replace(out.Location, "http://", "https://", 1)
	}
	if url == "" {
		return "", errors.New("图床没返回图片地址")
	}
	return url, nil
}

// UpdateCover 更新直播间封面。cover 必须是 .hdslb.com 下的地址，本地图先走 UploadImage。
// 接口挂在 app-blink 下，但网页端只带 csrf 就能过，不用 app 签名。
func (c *Client) UpdateCover(cover string) error {
	_, err := c.post(liveBase, "/xlive/app-blink/v1/preLive/UpdatePreLiveInfo", map[string]string{
		"platform":   "web",
		"mobi_app":   "web",
		"build":      "1",
		"csrf":       c.csrf(),
		"csrf_token": c.csrf(),
		"cover":      cover,
	}, false)
	return err
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
