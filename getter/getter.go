package getter

import (
	"encoding/json"
	"fmt"
	"github.com/tc1911/bilibili_live_tui_plus/config"
	"net/http"
	"time"

	myhttp "github.com/BYT0723/go-tools/http"

	"github.com/asmcos/requests"
	"github.com/gorilla/websocket"
	bg "github.com/iyear/biligo"

	"github.com/tidwall/gjson"
)

type DanmuClient struct {
	roomID        uint32
	auth          bg.CookieAuth
	conn          *websocket.Conn
	unzlibChannel chan []byte
	isClosed      bool
}

type OnlineRankUser struct {
	Name  string
	Score int64
	Rank  int64
}

type RoomInfo struct {
	RoomId          int
	Uid             int
	Title           string
	ParentAreaName  string
	AreaName        string
	Online          int64
	Attention       int64
	Time            string
	OnlineRankUsers []OnlineRankUser
}

type DanmuMsg struct {
	Author  string
	Content string
	Type    string
	Time    time.Time
}

type receivedInfo struct {
	Cmd        string                 `json:"cmd"`
	Data       map[string]interface{} `json:"data"`
	Info       []interface{}          `json:"info"`
	Full       map[string]interface{} `json:"full"`
	Half       map[string]interface{} `json:"half"`
	Side       map[string]interface{} `json:"side"`
	RoomID     uint32                 `json:"roomid"`
	RealRoomID uint32                 `json:"real_roomid"`
	MsgCommon  string                 `json:"msg_common"`
	MsgSelf    string                 `json:"msg_self"`
	LinkUrl    string                 `json:"link_url"`
	MsgType    string                 `json:"msg_type"`
	ShieldUID  string                 `json:"shield_uid"`
	BusinessID string                 `json:"business_id"`
	Scatter    map[string]interface{} `json:"scatter"`
}

type handShakeInfo struct {
	UID      uint32 `json:"uid"`
	Roomid   uint32 `json:"roomid"`
	Protover uint8  `json:"protover"`
	Buvid    string `json:"buvid"`
	Platform string `json:"platform"`
	Type     uint8  `json:"type"`
	Key      string `json:"key"`
}

// 风控会看 UA/Referer：只有 WBI 签名、缺浏览器头，getDanmuInfo 依旧返 -352。
const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.110 Safari/537.36"

func (d *DanmuClient) connect() (err error) {
	var (
		uid    uint32
		body   []byte
		header = http.Header{
			"Cookie": []string{config.Config.Cookie},
		}
	)

	// 这些头必须赶在 getDanmuInfo 之前设好，它和下面 ws 握手用的是同一份 header。
	header.Set("User-Agent", browserUA)
	header.Set("Accept", "*/*")
	header.Set("Accept-Language", "zh-CN,zh;q=0.8,zh-TW;q=0.7,zh-HK;q=0.5,en-US;q=0.3,en;q=0.2")
	header.Set("Origin", "https://live.bilibili.com")
	header.Set("Referer", "https://live.bilibili.com/")
	header.Set("Pragma", "no-cache")
	header.Set("Cache-Control", "no-cache")

	_, body, err = myhttp.Get("https://api.bilibili.com/x/web-interface/nav", header, nil)
	if err != nil {
		return err
	}
	uid = uint32(gjson.GetBytes(body, "data.mid").Int())

	// getDanmuInfo 必须带 WBI 签名，否则一律返 -352 风控、host_list 为空。
	mixin, err := wbiMixinKey(header)
	if err != nil {
		return err
	}
	query := wbiSign(map[string]string{
		"id":           fmt.Sprint(d.roomID),
		"type":         "0",
		"web_location": "444.8",
	}, mixin, time.Now().Unix())
	_, body, err = myhttp.Get("https://api.live.bilibili.com/xlive/web-room/v1/index/getDanmuInfo?"+query, header, nil)
	if err != nil {
		return err
	}
	if code := gjson.GetBytes(body, "code").Int(); code != 0 {
		return fmt.Errorf("getDanmuInfo 返回 %d: %s", code, gjson.GetBytes(body, "message").String())
	}

	token := gjson.GetBytes(body, "data.token").String()
	hostList := []string{}
	gjson.GetBytes(body, "data.host_list").ForEach(func(key, value gjson.Result) bool {
		hostList = append(hostList, value.Get("host").String())
		return true
	})
	hsInfo := handShakeInfo{
		UID:      uid,
		Roomid:   d.roomID,
		Protover: 2,
		Platform: "web",
		Type:     2,
		Key:      token,
	}

	header.Set("Accept-Encoding", "gzip, deflate, br")
	for _, h := range hostList {
		d.conn, _, err = websocket.DefaultDialer.Dial(fmt.Sprintf("wss://%s:443/sub", h), header)
		if err != nil {
			continue
		}
		break
	}
	if err != nil {
		return
	}
	if d.conn == nil {
		// host_list 为空时循环一次都不跑，err 还停在上一个请求的 nil 上，
		// 继续走就会拿 nil 连接发握手包 → 整个进程 panic。
		return fmt.Errorf("没有可用的弹幕服务器（host_list 共 %d 个）", len(hostList))
	}
	body, err = json.Marshal(hsInfo)
	if err != nil {
		return
	}

	err = d.sendPackage(0, 16, 1, 7, 1, body)
	return
}

var historied = false

func (d *DanmuClient) getHistory(busChan chan DanmuMsg) {
	if historied {
		return
	}

	historyApi := fmt.Sprintf("https://api.live.bilibili.com/xlive/web-room/v1/dM/gethistory?roomid=%d", d.roomID)
	r, err := requests.Get(historyApi)
	if err != nil {
		return
	}

	histories := gjson.Get(r.Text(), "data.room").Array()
	for _, history := range histories {
		t, _ := time.Parse("2006-01-02 15:04:05", history.Get("timeline").String())
		danmu := DanmuMsg{
			Author:  history.Get("nickname").String(),
			Content: history.Get("text").String(),
			Type:    "DANMU_MSG",
			Time:    t,
		}
		busChan <- danmu
	}
	historied = true
}

func (d *DanmuClient) heartBeat(msgChan chan DanmuMsg) {
	for {
		if d.isClosed {
			return
		}
		obj := []byte("5b6f626a656374204f626a6563745d")
		if err := d.sendPackage(0, 16, 1, 2, 1, obj); err != nil {
			msgChan <- DanmuMsg{
				// Author
				// Content string
				// Type    string
			}
			continue
		}
		time.Sleep(30 * time.Second)
	}
}

// fuck 每次启动总容易失败panic
func (d *DanmuClient) receiveRawMsg(busChan chan DanmuMsg) {
	for {
		if d.isClosed {
			return
		}
		_, rawMsg, err := d.conn.ReadMessage()
		if err != nil {
			d.isClosed = true
		}
		if len(rawMsg) >= 8 && rawMsg[7] == 2 {
			msgs := splitMsg(zlibUnCompress(rawMsg[16:]))
			for _, msg := range msgs {
				uz := msg[16:]
				js := new(receivedInfo)
				json.Unmarshal(uz, js)
				m := DanmuMsg{}
				switch js.Cmd {
				case "COMBO_SEND":
					m.Author = js.Data["uname"].(string)
					m.Content = fmt.Sprintf("送给 %s %d 个 %s", js.Data["r_uname"].(string), int(js.Data["combo_num"].(float64)), js.Data["gift_name"].(string))
				case "DANMU_MSG":
					m.Author = js.Info[2].([]interface{})[1].(string)
					m.Content = js.Info[1].(string)
				case "GUARD_BUY":
					m.Author = js.Data["username"].(string)
					m.Content = fmt.Sprintf("购买了 %s", js.Data["giftName"].(string))
				case "INTERACT_WORD":
					m.Author = js.Data["uname"].(string)
					m.Content = "进入了房间"
				case "SEND_GIFT":
					m.Author = js.Data["uname"].(string)
					m.Content = fmt.Sprintf("投喂了 %d 个 %s", int(js.Data["num"].(float64)), js.Data["giftName"].(string))
				case "USER_TOAST_MSG":
					m.Author = "system"
					m.Content = js.Data["toast_msg"].(string)
				case "NOTICE_MSG":
					m.Author = "system"
					m.Content = js.MsgSelf
				default: // "LIVE" "ACTIVITY_BANNER_UPDATE_V2" "ONLINE_RANK_COUNT" "ONLINE_RANK_TOP3" "ONLINE_RANK_V2" "PANEL" "PREPARING" "WIDGET_BANNER" "LIVE_INTERACTIVE_GAME"
					continue
				}
				m.Type = js.Cmd
				m.Time = time.Now()
				busChan <- m
			}
		}
	}
}

func (d *DanmuClient) syncRoomInfo(roomInfoChan chan RoomInfo) {
	for {
		if d.isClosed {
			return
		}

		roomInfoApi := fmt.Sprintf("https://api.live.bilibili.com/room/v1/room/get_info?room_id=%d", d.roomID)
		roomInfo := new(RoomInfo)
		roomInfo.OnlineRankUsers = make([]OnlineRankUser, 0)
		r1, err1 := requests.Get(roomInfoApi)
		if err1 == nil {
			roomInfo.RoomId = int(d.roomID)
			roomInfo.Uid = int(gjson.Get(r1.Text(), "data.uid").Int())
			roomInfo.Title = gjson.Get(r1.Text(), "data.title").String()
			roomInfo.AreaName = gjson.Get(r1.Text(), "data.area_name").String()
			roomInfo.ParentAreaName = gjson.Get(r1.Text(), "data.parent_area_name").String()
			roomInfo.Online = gjson.Get(r1.Text(), "data.online").Int()
			roomInfo.Attention = gjson.Get(r1.Text(), "data.attention").Int()
			_time, _ := time.Parse("2006-01-02 15:04:05", gjson.Get(r1.Text(), "data.live_time").String())
			seconds := time.Now().Unix() - _time.Unix() + 8*60*60
			days := seconds / 86400
			hours := (seconds % 86400) / 3600
			minutes := (seconds % 3600) / 60
			if days > 0 {
				roomInfo.Time = fmt.Sprintf("%d天%d时%d分", days, hours, minutes)
			} else if hours > 0 {
				roomInfo.Time = fmt.Sprintf("%d时%d分", hours, minutes)
			} else {
				roomInfo.Time = fmt.Sprintf("%d分", minutes)
			}
		}

		onlineRankApi := fmt.Sprintf("https://api.live.bilibili.com/xlive/general-interface/v1/rank/getOnlineGoldRank?ruid=%d&roomId=%d&page=1&pageSize=50", roomInfo.Uid, d.roomID)
		r2, err2 := requests.Get(onlineRankApi)
		if err2 == nil {
			rawUsers := gjson.Get(r2.Text(), "data.OnlineRankItem").Array()
			for _, rawUser := range rawUsers {
				user := OnlineRankUser{
					Name:  rawUser.Get("name").String(),
					Score: rawUser.Get("score").Int(),
					Rank:  rawUser.Get("userRank").Int(),
				}
				roomInfo.OnlineRankUsers = append(roomInfo.OnlineRankUsers, user)
			}
		}

		roomInfoChan <- *roomInfo
		time.Sleep(30 * time.Second)
	}
}

// 总是会崩溃，不如直接重启
func supervisor(busChan chan DanmuMsg, roomInfoChan chan RoomInfo) {
	dc := DanmuClient{
		roomID:        uint32(config.Config.RoomId),
		auth:          config.Auth,
		unzlibChannel: make(chan []byte, 100),
	}

	defer func() {
		busChan <- DanmuMsg{
			Author:  "system",
			Content: "弹幕服务器已断开，正在重连 :)",
			Type:    "NOTICE_MSG",
			Time:    time.Now(),
		}
		dc.isClosed = true
		// connect() 里任何一步失败（改 nav / wbi / getDanmuInfo）都是提前 return，
		// 从来不会走到赋值 conn 那行，所以这里 conn 可能是 nil。
		// 以前结构体里塞了 new(websocket.Conn) 当占位，非 nil 但内部 net.Conn 是空的，
		// 这个判断拦不住，Close() 就 nil 崩了。
		if dc.conn != nil {
			dc.conn.Close()
		}
		time.Sleep(1 * time.Second)
		supervisor(busChan, roomInfoChan)
	}()

	err := dc.connect()
	if err != nil {
		// 这里原来直接 panic，会把整个 TUI 一起带走。
		// ponytail: 固定 30s 退避，够用了；要快就改成指数退避。
		busChan <- DanmuMsg{
			Author:  "system",
			Content: "弹幕连接失败（30 秒后重试）: " + err.Error(),
			Type:    "NOTICE_MSG",
			Time:    time.Now(),
		}
		time.Sleep(30 * time.Second)
		return
	}

	go dc.getHistory(busChan)
	go dc.receiveRawMsg(busChan)
	go dc.syncRoomInfo(roomInfoChan)
	go dc.heartBeat(busChan)

	for {
		time.Sleep(1 * time.Second)
		if dc.isClosed {
			return
		}
	}
}

func Run(busChan chan DanmuMsg, roomInfoChan chan RoomInfo) {
	go supervisor(busChan, roomInfoChan)
}
