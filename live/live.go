package live

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// 开播需要额外验证时的哨兵错误，配 StartError 用。
var (
	ErrVerifyRequired   = errors.New("本次开播需要扫码验证")
	ErrFaceAuthRequired = errors.New("本次开播需要人脸认证")
)

// 扫码轮询的 data.code。
const (
	QRSuccess     = 0
	QRExpired     = 86038
	QRWaitingScan = 86101
	QRWaitingOK   = 86090
)

// Stream 是一路推流凭据。
type Stream struct {
	Type     string `json:"type"` // rtmp-1 / rtmp-2 / srt-1
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Key      string `json:"key"`
	FullURL  string `json:"full_url"`
}

// Room 是 get_info 的摘要字段。
type Room struct {
	RoomID         int64  `json:"room_id"`
	UID            int64  `json:"uid"`
	Title          string `json:"title"`
	AreaID         ID     `json:"area_id"`
	AreaName       string `json:"area_name"`
	ParentAreaID   ID     `json:"parent_area_id"`
	ParentAreaName string `json:"parent_area_name"`
	LiveStatus     int    `json:"live_status"`
}

func (r *Room) Area() string {
	if r.ParentAreaName == "" {
		return r.AreaName
	}
	return r.ParentAreaName + "/" + r.AreaName
}

// SubArea 是子分区。
type SubArea struct {
	ID         ID     `json:"id"`
	ParentID   ID     `json:"parent_id"`
	Name       string `json:"name"`
	ParentName string `json:"parent_name"`
}

// ParentArea 是父分区，list 里挂着它的子分区。
type ParentArea struct {
	ID   ID        `json:"id"`
	Name string    `json:"name"`
	List []SubArea `json:"list"`
}

// QRStatus 是扫码轮询的返回。
type QRStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	URL     string `json:"url"`
}

// StartError 包装开播失败；VerifyQR 只在需要用户扫码验证时非空。
type StartError struct {
	Err      error
	VerifyQR string
}

func (e *StartError) Error() string { return e.Err.Error() }
func (e *StartError) Unwrap() error { return e.Err }

// VerifyQR 从错误里取出需要用户扫描的验证地址，没有就返回空串。
func VerifyQR(err error) string {
	var se *StartError
	if errors.As(err, &se) {
		return se.VerifyQR
	}
	return ""
}

// QRGenerate 申请一张登录二维码。
func (c *Client) QRGenerate() (loginURL, key string, err error) {
	data, err := c.get(passportBase, "/x/passport-login/web/qrcode/generate", nil, false)
	if err != nil {
		return "", "", err
	}
	var out struct {
		URL       string `json:"url"`
		QRCodeKey string `json:"qrcode_key"`
	}
	if err = json.Unmarshal(data, &out); err != nil {
		return "", "", err
	}
	return out.URL, out.QRCodeKey, nil
}

// QRPoll 查一次扫码状态。返回的 QRStatus.Code 见 QRSuccess / QRExpired 等。
func (c *Client) QRPoll(key string) (*QRStatus, error) {
	data, err := c.get(passportBase, "/x/passport-login/web/qrcode/poll",
		map[string]string{"qrcode_key": key}, false)
	if err != nil {
		return nil, err
	}
	status := new(QRStatus)
	if err = json.Unmarshal(data, status); err != nil {
		return nil, err
	}
	if status.Code == QRSuccess && !c.LoggedIn() {
		// 兜底：跳转地址的 query 里同样带着 Cookie。
		if u, perr := url.Parse(status.URL); perr == nil {
			for _, name := range cookieNames {
				if v := u.Query().Get(name); v != "" {
					c.cookies[name] = v
				}
			}
		}
	}
	return status, nil
}

// Nav 校验登录态并返回账号信息。
func (c *Client) Nav() (mid int64, uname string, err error) {
	data, err := c.get(mainBase, "/x/web-interface/nav", nil, false)
	if err != nil {
		return 0, "", err
	}
	var out struct {
		Mid     int64  `json:"mid"`
		Uname   string `json:"uname"`
		IsLogin bool   `json:"isLogin"`
	}
	if err = json.Unmarshal(data, &out); err != nil {
		return 0, "", err
	}
	if !out.IsLogin {
		return 0, "", errors.New("登录态已失效，请重新扫码登录")
	}
	return out.Mid, out.Uname, nil
}

// RoomIDByUID 按 UID 反查直播间号，用于自动填 RoomId。
func (c *Client) RoomIDByUID(uid int64) (int64, error) {
	data, err := c.get(liveBase, "/room/v2/Room/room_id_by_uid",
		map[string]string{"uid": strconv.FormatInt(uid, 10)}, false)
	if err != nil {
		return 0, err
	}
	var out struct {
		RoomID int64 `json:"room_id"`
	}
	if err = json.Unmarshal(data, &out); err != nil {
		return 0, err
	}
	return out.RoomID, nil
}

// Room 查直播间状态（只读）。
func (c *Client) Room(roomID int64) (*Room, error) {
	data, err := c.get(liveBase, "/room/v1/Room/get_info",
		map[string]string{"room_id": strconv.FormatInt(roomID, 10)}, false)
	if err != nil {
		return nil, err
	}
	room := new(Room)
	if err = json.Unmarshal(data, room); err != nil {
		return nil, err
	}
	return room, nil
}

// Areas 拉完整的两级分区表。接口无视 parent_area_id，一次就返回父分区套子分区。
func (c *Client) Areas() ([]ParentArea, error) {
	data, err := c.get(liveBase, "/room/v1/area/getList", nil, false)
	if err != nil {
		return nil, err
	}
	var areas []ParentArea
	if err = json.Unmarshal(data, &areas); err != nil {
		return nil, err
	}
	return areas, nil
}

func (c *Client) liveVersion() (version string, build int, err error) {
	data, err := c.get(liveBase, "/xlive/app-blink/v1/liveVersionInfo/getHomePageLiveVersion",
		map[string]string{"system_version": "2", "ts": ms()}, true)
	if err != nil {
		return "", 0, err
	}
	var out struct {
		CurrVersion string `json:"curr_version"`
		Build       int    `json:"build"`
	}
	if err = json.Unmarshal(data, &out); err != nil {
		return "", 0, err
	}
	return out.CurrVersion, out.Build, nil
}

// StartLive 开播并返回推流凭据。
// 需要验证时返回的 error 里带 ErrVerifyRequired / ErrFaceAuthRequired，
// 扫码地址用 VerifyQR(err) 取。开播会让直播间立刻对外可见。
func (c *Client) StartLive(roomID, areaV2 int64) ([]Stream, error) {
	version, build, err := c.liveVersion()
	if err != nil {
		return nil, fmt.Errorf("取开播版本号失败: %w", err)
	}

	data, err := c.post(liveBase, "/room/v1/Room/startLive", map[string]string{
		"room_id":       strconv.FormatInt(roomID, 10),
		"platform":      "pc_link",
		"backup_stream": "0",
		"csrf":          c.csrf(),
		"csrf_token":    c.csrf(),
		"area_v2":       strconv.FormatInt(areaV2, 10),
		"version":       version,
		"build":         strconv.Itoa(build),
		"ts":            ms(),
	}, true)
	if err != nil {
		var apiErr *ApiError
		if errors.As(err, &apiErr) {
			switch apiErr.Code {
			case 60024, 60043:
				wrapped := ErrVerifyRequired
				url := verifyURL(apiErr)
				if apiErr.Code == 60043 {
					wrapped = ErrFaceAuthRequired
					// 人脸认证服务端不返二维码地址，得自己拼 mid。
					if url == "" {
						if mid, _, err := c.Nav(); err == nil && mid > 0 {
							url = faceAuthURL(mid)
						}
					}
				}
				return nil, &StartError{Err: fmt.Errorf("%w：%s", wrapped, apiErr.Msg),
					VerifyQR: url}
			}
		}
		return nil, err
	}

	var out struct {
		Rtmp struct {
			Addr string `json:"addr"`
			Code string `json:"code"`
		} `json:"rtmp"`
		Protocols []struct {
			Protocol string `json:"protocol"`
			Addr     string `json:"addr"`
			Code     string `json:"code"`
		} `json:"protocols"`
	}
	if err = json.Unmarshal(data, &out); err != nil {
		return nil, err
	}

	streams := make([]Stream, 0, 4)
	add := func(protocol, addr, key string) {
		if addr == "" || key == "" {
			return
		}
		full := addr + "/" + key
		if strings.HasPrefix(key, "?") || strings.HasSuffix(addr, "/") {
			full = addr + key
		}
		n := 1
		for _, s := range streams {
			if s.Protocol == protocol {
				n++
			}
		}
		streams = append(streams, Stream{
			Type:     fmt.Sprintf("%s-%d", protocol, n),
			Protocol: protocol, Address: addr, Key: key, FullURL: full,
		})
	}
	add("rtmp", out.Rtmp.Addr, out.Rtmp.Code)
	for _, p := range out.Protocols {
		add(p.Protocol, p.Addr, p.Code)
	}
	if len(streams) == 0 {
		return nil, errors.New("开播成功但接口没返回推流地址")
	}
	return streams, nil
}

// faceAuthURL 拼人脸认证页地址；60043 的响应里没有二维码，只能拿 mid 自己拼。
func faceAuthURL(mid int64) string {
	return fmt.Sprintf("https://www.bilibili.com/blackboard/live/face-auth-middle.html?source_event=400&mid=%d", mid)
}

// verifyURL 从 60024 / 60043 的 data 里取验证地址。
func verifyURL(apiErr *ApiError) string {
	var out struct {
		QR string `json:"qr"`
	}
	if len(apiErr.Data) == 0 || json.Unmarshal(apiErr.Data, &out) != nil {
		return ""
	}
	return out.QR
}

// StopLive 下播。
func (c *Client) StopLive(roomID int64) error {
	_, err := c.post(liveBase, "/room/v1/Room/stopLive", map[string]string{
		"room_id":    strconv.FormatInt(roomID, 10),
		"platform":   "pc_link",
		"csrf":       c.csrf(),
		"csrf_token": c.csrf(),
	}, false)
	return err
}

// UpdateTitle 改直播间标题。
func (c *Client) UpdateTitle(roomID int64, title string) error {
	_, err := c.post(liveBase, "/room/v1/Room/update", map[string]string{
		"room_id":    strconv.FormatInt(roomID, 10),
		"title":      title,
		"platform":   "pc_link",
		"csrf":       c.csrf(),
		"csrf_token": c.csrf(),
	}, false)
	return err
}
