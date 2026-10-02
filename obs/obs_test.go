package obs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// 黄金值：用 obs-websocket 文档那套算法（python 独立实现一遍）算出来的，
// 换算法或写错一步这个测试就红。
const (
	goldenSalt      = "salt123"
	goldenChallenge = "chal456"
	goldenPassword  = "secret"
	goldenAuth      = "yo3DuCXyQQheiGKNZpyXB//3OodP2GoXULXeX19lE4M="
)

func TestAuthString(t *testing.T) {
	if got := authString(goldenPassword, goldenSalt, goldenChallenge); got != goldenAuth {
		t.Fatalf("authString = %q, want %q", got, goldenAuth)
	}
}

// 跟一个假 OBS 走一遍完整握手：Hello -> Identify -> Identified -> Request -> Response，
// 顺带验请求体的形状 —— 字段写错 OBS 那头会直接不认。
func TestSetStreamRoundTrip(t *testing.T) {
	reqs := make(chan map[string]any, 1)
	upgrader := websocket.Upgrader{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_ = conn.WriteJSON(map[string]any{"op": 0, "d": map[string]any{
			"obsWebSocketVersion": "5.5.2",
			"rpcVersion":          1,
			"authentication":      map[string]any{"challenge": goldenChallenge, "salt": goldenSalt},
		}})

		var ident struct {
			Op int `json:"op"`
			D  struct {
				RPCVersion     int    `json:"rpcVersion"`
				Authentication string `json:"authentication"`
			} `json:"d"`
		}
		if err := conn.ReadJSON(&ident); err != nil {
			return
		}
		if ident.Op != opIdentify {
			t.Errorf("第二个包该是 Identify，收到 op=%d", ident.Op)
		}
		if ident.D.Authentication != goldenAuth {
			t.Errorf("鉴权串 = %q，want %q", ident.D.Authentication, goldenAuth)
		}
		if err := conn.WriteJSON(map[string]any{"op": 2, "d": map[string]any{"negotiatedRpcVersion": 1}}); err != nil {
			return
		}

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			return
		}
		reqs <- req
		_ = conn.WriteJSON(map[string]any{"op": 7, "d": map[string]any{
			"requestType":   "SetStreamServiceSettings",
			"requestId":     "bili-fill",
			"requestStatus": map[string]any{"result": true, "code": 100},
		}})
	}))
	defer srv.Close()

	host, port := hostPort(t, srv.URL)
	server := "rtmp://live-push.bilivideo.com/live-bvc/"
	key := "?streamname=live_1_2&key=abc"
	if err := SetStream(Settings{Host: host, Port: port, Password: goldenPassword}, server, key); err != nil {
		t.Fatalf("SetStream: %v", err)
	}

	select {
	case req := <-reqs:
		d, _ := req["d"].(map[string]any)
		if d["requestType"] != "SetStreamServiceSettings" {
			t.Fatalf("requestType = %v", d["requestType"])
		}
		rd, _ := d["requestData"].(map[string]any)
		if rd["streamServiceType"] != "rtmp_custom" {
			t.Errorf("streamServiceType = %v，want rtmp_custom", rd["streamServiceType"])
		}
		ss, _ := rd["streamServiceSettings"].(map[string]any)
		if ss["server"] != server || ss["key"] != key {
			t.Errorf("填进去的是 server=%v key=%v", ss["server"], ss["key"])
		}
	default:
		t.Fatal("假 OBS 没收到请求")
	}
}

// 配置里没写端口和密码时，去读 OBS 自己那份 config.json；服务器没开要说清楚。
func TestResolveReadsOBSConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "obs-studio", "plugin_config", "obs-websocket", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	write := func(body string) {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(`{"server_enabled":true,"server_port":4455,"server_password":"pw"}`)
	set, err := Resolve("", 0, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if set.Port != 4455 || set.Password != "pw" {
		t.Errorf("读出来的是 port=%d password=%q", set.Port, set.Password)
	}

	// 配置里写了就以配置为准
	set, err = Resolve("10.0.0.1", 4456, "mine")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if set.Host != "10.0.0.1" || set.Port != 4456 || set.Password != "mine" {
		t.Errorf("配置没优先: %+v", set)
	}

	write(`{"server_enabled":false,"server_port":4455,"server_password":"pw"}`)
	_, err = Resolve("", 0, "")
	if err == nil || !strings.Contains(err.Error(), "没开") {
		t.Errorf("服务器没开时该给一句人话，拿到 %v", err)
	}
}

func hostPort(t *testing.T, raw string) (string, int) {
	t.Helper()
	i := strings.LastIndex(raw, ":")
	if i < 0 {
		t.Fatalf("奇怪的地址 %q", raw)
	}
	var port int
	if err := json.Unmarshal([]byte(strings.TrimPrefix(raw[i+1:], "")), &port); err != nil {
		t.Fatalf("解析端口失败: %v", err)
	}
	return strings.TrimPrefix(raw[:i], "http://"), port
}
