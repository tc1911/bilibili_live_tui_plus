# bilibili 直播间 TUI

[关联的bilibili介绍视频](https://www.bilibili.com/video/bv1gG411G7XG)

> 本文库是 [yaocccc/bilibili_live_tui](https://github.com/yaocccc/bilibili_live_tui) 的增强分支：
> 在原版「看弹幕」之外，把**开播**这一套也搬进了同一个 TUI。
> 上游的视频、主题截图、贡献者名单都原样保留在下面。

## 相比上游新增

| 按键 | 功能 |
| --- | --- |
| `F2` | 扫码登录：终端里直接画二维码，手机 B 站 App 扫一下，登录态写回配置 |
| `F3` | 选择开播分区：两级分区树，左右键展开/收起，回车确认，记忆到配置 |
| `F4` | 开播：自动带上直播间号与记忆的分区，成功后**独占一页**显示推流地址与推流码，OBS 直接填「服务器 + 密钥」 |
| `F5` | 下播 |
| `Esc` | 关闭面板；在推流码页按则退回控制面板 |

顺带修掉的上游问题：

1. 弹幕连接失败会 `panic` 崩掉整个 TUI（改为 30 秒退避重连，面板照常可用）
2. 首次自动生成 `config.toml` 后必定 panic
3. `getDanmuInfo` 缺 WBI 签名与浏览器请求头，被风控挡在 `-352`，弹幕永远连不上（B 站 2025-05-26 起强制签名）

> **注意**：`F4` 开播会让直播间立刻对外可见、并给粉丝发开播推送。人脸认证 / App 扫码验证
> （接口 `60043` / `60024`）会在面板里给出二维码，必须在手机上完成。

## 安装

```bash
go install github.com/tc1911/bilibili_live_tui_plus@latest
```

或者自己编译：

```bash
git clone https://github.com/tc1911/bilibili_live_tui_plus
cd bilibili_live_tui_plus
go build -o bili .
./bili
```

使用方法 直接下载 releases 中的 bin文件即可

---

风格1: chatroom

![t1](./theme1.png)

风格2: pure

![t2](./theme2.png)

风格3: simple

![t3](./theme3.png)

风格4: info (感谢@soft98-top添加的theme4)

![t4](./theme4.png)

项目文件:

```plaintext
  sender 发送弹幕的实现
  getter 获取弹幕的实现
  ui     TUI的实现
```

使用:

go run main.go

也可以从 参数定义 roomId, theme 优先级高于config(-r roomId, -t theme)

go run main.go -c config.toml -r 9527 -t 1

配置:

默认配置文件: ~/.config/bili/config.toml（首次运行自动生成）

与登录/开播相关的字段（`F2`、`F3` 会自动写回）：

```toml
Cookie   = "SESSDATA=...; bili_jct=...; DedeUserID=...; DedeUserID__ckMd5=...;" # 留空则用 F2 扫码登录
RoomId   = 123456  # 直播间号
AreaV2   = 0       # 开播分区 id，F3 选择后写入
AreaName = ""      # 分区名（仅显示用），F3 选择后写入
```

参数说明:  
  1. `-c string:configfile`
  2. `-r string:roomId`
  3. `-t int:theme`
  4. `-l int:singleline`
  5. `-s int:showtime`

快捷键:  
  1. \<esc> 退出
  2. <ctrl+c> 退出
  3. <ctrl+u> 清空输入内容
  4. <up> 上一个输入记录
  5. <down> 下一个输入记录

## 类似项目

[zaiic/bili-live-chat](https://github.com/zaiic/bili-live-chat): A bilibili streaming chat tool using TUI written in Rust. 

## 贡献者

- [yaocccc](https://github.com/yaocccc)  
- [soft98-top](https://github.com/soft98-top)
  - [PR#3 增加theme4，修复直播间rank显示](https://github.com/yaocccc/bilibili_live_tui/pull/3)  
- [zaiic](https://github.com/zaiic)
  - [PR#4 更新README，添加类似项目](https://github.com/yaocccc/bilibili_live_tui/pull/4)
- [Ruixi-rebirth](https://github.com/Ruixi-rebirth)
  - [PR#6 自动创建配置文件到 $HOME/.config/bili/config.toml](https://github.com/yaocccc/bilibili_live_tui/pull/6)

## Support: buy me a coffee)

<a href="https://www.buymeacoffee.com/yaocccc" target="_blank">
  <img src="https://github.com/yaocccc/yaocccc/raw/master/qr.png">
</a>

## 来源与许可证

- **上游**：[yaocccc/bilibili_live_tui](https://github.com/yaocccc/bilibili_live_tui)。上游仓库**没有 LICENSE 文件**，默认保留所有权利；
  本仓库是它的公开分支，上游部分的版权归原作者及下列贡献者所有。要复用上游代码请先联系上游作者。
- **开播部分**：APP 签名与开播流程（`live/client.go`、`live/live.go`）移植自
  [Rsplwe/bili-live-hime](https://github.com/Rsplwe/bili-live-hime) 的 `src/lib/app-sign.ts` 与 `src/api/live.ts`，
  该项目以 **GPL-2.0** 分发。因此这部分代码同样以 GPL-2.0 分发，见 [LICENSE](./LICENSE)。
