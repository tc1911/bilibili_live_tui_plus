// Package obs 通过 obs-websocket（OBS 28 起内置）跟 OBS 说话。
//
// 目前只干一件事：开播后把推流服务器和密钥填进 OBS 的「设置 → 推流」，
// 省得每次手动复制那一长串。协议是 obs-websocket 5.x 的 JSON 版，
// 握手：对方先 Hello，我们回 Identify（要鉴权的话带上 challenge 的应答），
// 对方 Identified 之后就能发 Request 了。
package obs

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
)

const (
	dialTimeout = 2 * time.Second
	readTimeout = 5 * time.Second
)

// 协议里的操作码（obs-websocket 5.x）。
const (
	opHello      = 0
	opIdentify   = 1
	opIdentified = 2
	opRequest    = 6
	opResponse   = 7
)

// Settings 是连 OBS 需要的东西。
type Settings struct {
	Host     string
	Port     int
	Password string
}

type frame struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
}

// SetStream 把服务器地址和密钥写进 OBS 的推流设置。
// 服务类型用 rtmp_custom：B 站给的地址就是 rtmp，密钥里自带 ?streamname=…，
// 整串原样塞进「串流密钥」框里就行，OBS 自己会拼。
func SetStream(set Settings, server, key string) error {
	host := set.Host
	if host == "" {
		host = "127.0.0.1"
	}
	u := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%d", host, set.Port), Path: "/"}

	dialer := websocket.Dialer{HandshakeTimeout: dialTimeout}
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("连不上 OBS 的 WebSocket（%s）: %w", u.Host, err)
	}
	defer conn.Close()

	f, err := readFrame(conn)
	if err != nil {
		return fmt.Errorf("读 OBS 的握手失败: %w", err)
	}
	if f.Op != opHello {
		return fmt.Errorf("OBS 第一句应该是 Hello，收到 op=%d", f.Op)
	}
	var hello struct {
		Authentication *struct {
			Challenge string `json:"challenge"`
			Salt      string `json:"salt"`
		} `json:"authentication"`
	}
	if err := json.Unmarshal(f.D, &hello); err != nil {
		return err
	}

	identify := map[string]any{"rpcVersion": 1}
	if hello.Authentication != nil {
		if set.Password == "" {
			return errors.New("OBS 要密码，但没拿到：要么在 OBS 里关掉鉴权，要么把密码填进 config.toml 的 OBSPassword")
		}
		identify["authentication"] = authString(set.Password, hello.Authentication.Salt, hello.Authentication.Challenge)
	}
	if err := writeFrame(conn, opIdentify, identify); err != nil {
		return err
	}

	f, err = readFrame(conn)
	if err != nil {
		return fmt.Errorf("OBS 没确认连接: %w", err)
	}
	if f.Op != opIdentified {
		return fmt.Errorf("OBS 拒绝了这次连接（op=%d），密码对不上？", f.Op)
	}

	req := map[string]any{
		"requestType": "SetStreamServiceSettings",
		"requestId":   "bili-fill",
		"requestData": map[string]any{
			"streamServiceType": "rtmp_custom",
			"streamServiceSettings": map[string]any{
				"server":   server,
				"key":      key,
				"use_auth": false,
			},
		},
	}
	if err := writeFrame(conn, opRequest, req); err != nil {
		return err
	}

	f, err = readFrame(conn)
	if err != nil {
		return fmt.Errorf("OBS 没回话: %w", err)
	}
	if f.Op != opResponse {
		return fmt.Errorf("OBS 回了条没见过的消息（op=%d）", f.Op)
	}
	var resp struct {
		RequestStatus struct {
			Result  bool   `json:"result"`
			Code    int    `json:"code"`
			Comment string `json:"comment"`
		} `json:"requestStatus"`
	}
	if err := json.Unmarshal(f.D, &resp); err != nil {
		return err
	}
	if !resp.RequestStatus.Result {
		return fmt.Errorf("OBS 拒绝了（%d）: %s", resp.RequestStatus.Code, resp.RequestStatus.Comment)
	}
	return nil
}

func writeFrame(conn *websocket.Conn, op int, d any) error {
	payload, err := json.Marshal(d)
	if err != nil {
		return err
	}
	conn.SetWriteDeadline(time.Now().Add(readTimeout))
	return conn.WriteJSON(frame{Op: op, D: payload})
}

func readFrame(conn *websocket.Conn) (*frame, error) {
	conn.SetReadDeadline(time.Now().Add(readTimeout))
	_, data, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	f := new(frame)
	if err := json.Unmarshal(data, f); err != nil {
		return nil, err
	}
	return f, nil
}

// authString 是 obs-websocket 的鉴权算法：
// 密码拼上 salt 哈希一次、base64，再拼上 challenge 哈希一次、base64。
func authString(password, salt, challenge string) string {
	secret := sha256.Sum256([]byte(password + salt))
	encoded := base64.StdEncoding.EncodeToString(secret[:])
	sum := sha256.Sum256([]byte(encoded + challenge))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// Resolve 补齐没填的项：端口和密码优先用配置里的，缺的就去读 OBS 自己的 websocket 配置
// （~/.config/obs-studio/plugin_config/obs-websocket/config.json）。
// 顺带看一眼服务器开没开 —— 没开的话直接说清楚怎么开，比连不上再猜强。
func Resolve(host string, port int, password string) (Settings, error) {
	set := Settings{Host: host, Port: port, Password: password}
	if set.Port != 0 && set.Password != "" {
		return set, nil
	}

	raw, err := os.ReadFile(configPath())
	if err != nil {
		return set, fmt.Errorf("没找到 OBS 的 websocket 配置，先去 OBS 里 工具 → WebSocket 服务器设置 打开它")
	}
	var cfg struct {
		Enabled  *bool  `json:"server_enabled"`
		Port     int    `json:"server_port"`
		Password string `json:"server_password"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return set, err
	}
	if cfg.Enabled != nil && !*cfg.Enabled {
		return set, errors.New("OBS 里的 WebSocket 服务器没开：工具 → WebSocket 服务器设置 → 勾上「启用 WebSocket 服务器」")
	}
	if set.Port == 0 {
		set.Port = cfg.Port
	}
	if set.Password == "" {
		set.Password = cfg.Password
	}
	if set.Port == 0 {
		set.Port = 4455
	}
	return set, nil
}

func configPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "obs-studio", "plugin_config", "obs-websocket", "config.json")
}
