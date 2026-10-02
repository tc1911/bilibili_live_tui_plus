// Package version 只放一个版本号。
//
// 打包时用 ldflags 塞进来：
//
//	go build -ldflags "-X github.com/tc1911/bilibili_live_tui_plus/version.Version=v1.0.2"
//
// 本地直接 go build 不塞，就是 "dev"。
package version

// Version 是显示在界面右上角的版本号。
var Version = "dev"
