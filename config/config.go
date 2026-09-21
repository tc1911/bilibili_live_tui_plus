package config

import (
	"flag"
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/BurntSushi/toml"
	bg "github.com/iyear/biligo"
)

type ConfigType struct {
	Cookie       string // 登录cookie
	RoomId       int64  // 直播间id
	AreaV2       int64  // 记忆的开播分区id (area_v2)
	AreaName     string // 记忆的开播分区名，仅用于显示
	Theme        int64  // 主题
	SingleLine   int64  // 是否开启单行
	ShowTime     int64  // 是否显示时间
	TimeColor    string // 时间颜色
	NameColor    string // 名字颜色
	ContentColor string // 内容颜色
	FrameColor   string // 边框颜色
	InfoColor    string // 房间信息颜色
	RankColor    string // 排行榜颜色
	Background   string // 背景颜色
}

var Auth bg.CookieAuth
var Config ConfigType

// ConfigFile 是本次实际读取的配置文件路径，Save 写回它。
var ConfigFile string

// Save 把当前配置写回 config.toml（登录、选分区后调用）。
func Save() error {
	f, err := os.Create(ConfigFile)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(Config)
}

// RefreshAuth 把 Config.Cookie 重新解析进 Auth，供登录后即时生效。
func RefreshAuth() {
	kvs := make(map[string]string)
	for _, attr := range strings.Split(Config.Cookie, ";") {
		kv := strings.SplitN(strings.TrimSpace(attr), "=", 2)
		if len(kv) == 2 {
			kvs[kv[0]] = strings.TrimSpace(kv[1])
		}
	}
	Auth.SESSDATA = kvs["SESSDATA"]
	Auth.DedeUserID = kvs["DedeUserID"]
	Auth.DedeUserIDCkMd5 = kvs["DedeUserID__ckMd5"]
	Auth.BiliJCT = kvs["bili_jct"]
}

func defaultCfgFile() (configFile string, err error) {
	currentUser, err := user.Current()
	if err != nil {
		return
	}
	homeDir := currentUser.HomeDir
	path := homeDir + "/.config/bili"
	if err = os.MkdirAll(path, 0755); err != nil {
		return
	}
	configFile = path + "/config.toml"
	ConfigFile = configFile
	_, err = os.Stat(configFile)
	if os.IsNotExist(err) {
		var f *os.File
		config := ConfigType{
			Cookie:       "从你BILIBILI的请求里抓一个Cookie",
			RoomId:       23333333,
			Theme:        1,
			SingleLine:   1,
			ShowTime:     1,
			TimeColor:    "#FFFFFF",
			NameColor:    "#FFFFFF",
			ContentColor: "#FFFFFF",
			FrameColor:   "#FFFFFF",
			InfoColor:    "#FFFFFF",
			RankColor:    "#FFFFFF",
			Background:   "NONE", // 默认无背景颜色 NONE表示无背景颜色
		}
		f, err = os.Create(configFile)
		if err != nil {
			return
		}
		defer f.Close()
		if err = toml.NewEncoder(f).Encode(config); err != nil {
			return
		}

		// 不 panic：默认配置已经写好，TUI 里按 F2 扫码会把 Cookie 写回来。
		fmt.Println("已生成默认配置：" + configFile + "，启动后按 F2 扫码登录")
	}

	return
}

func Init() {
	var err error
	configFile := ""
	roomId := int64(-1)
	theme := int64(-1)
	single_line := int64(-1)
	show_time := int64(-1)
	flag.StringVar(&configFile, "c", "", "usage for config")
	flag.Int64Var(&roomId, "r", -1, "usage for room id")
	flag.Int64Var(&theme, "t", -1, "usage for theme")
	flag.Int64Var(&single_line, "l", -1, "usage for single_line")
	flag.Int64Var(&show_time, "s", -1, "usage for show_time")
	flag.Parse()

	if configFile == "" {
		configFile, err = defaultCfgFile()
		if err != nil {
			panic(err)
		}
	}
	ConfigFile = configFile

	if _, err := toml.DecodeFile(configFile, &Config); err != nil {
		fmt.Printf("Error decoding config.toml: %s\n", err)
	}
	if Config.Cookie == "从你BILIBILI的请求里抓一个Cookie" {
		// 不再直接 panic：TUI 里的控制面板（F2）就是用来扫码登录的。
		fmt.Println("尚未登录，启动后按 F2 扫码登录。配置文件：" + configFile)
	}

	if roomId != -1 {
		Config.RoomId = roomId
	}
	if theme != -1 {
		Config.Theme = theme
	}
	if single_line != -1 {
		Config.SingleLine = single_line
	}
	if show_time != -1 {
		Config.ShowTime = show_time
	}
	if Config.TimeColor == "" {
		Config.TimeColor = "#bbbbbb"
	}
	if Config.NameColor == "" {
		Config.NameColor = "#bbbbbb"
	}
	if Config.ContentColor == "" {
		Config.ContentColor = "#bbbbbb"
	}
	if Config.TimeColor == "" {
		Config.TimeColor = "#bbbbbb"
	}
	if Config.NameColor == "" {
		Config.NameColor = "#bbbbbb"
	}
	if Config.ContentColor == "" {
		Config.ContentColor = "#bbbbbb"
	}
	if Config.InfoColor == "" {
		Config.InfoColor = "#bbbbbb"
	}
	if Config.RankColor == "" {
		Config.RankColor = "#bbbbbb"
	}
	if Config.FrameColor == "" {
		Config.FrameColor = "#bbbbbb"
	}
	if Config.Background == "" {
		Config.Background = "NONE"
	}

	RefreshAuth()
}
