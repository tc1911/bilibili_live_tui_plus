# bilibili 直播间 TUI

> **已封存**：这个仓库不再维护，代码保持原样，只当参考实现留着。
> 后续版本是用 Rust + ratatui 从零重写的 `bililive`（`~/项目/bililive`），
> 接口行为（WBI 签名、弹幕协议、开播流程）都以本仓库的实测结论为准。
> 下面的安装、使用、来源说明一律保留原样，不再更新。

[关联的bilibili介绍视频](https://www.bilibili.com/video/bv1gG411G7XG)

> 本文库是 [yaocccc/bilibili_live_tui](https://github.com/yaocccc/bilibili_live_tui) 的增强分支：
> 在原版「看弹幕」之外，把**开播**这一套也搬进了同一个 TUI。
> 配置页（`Shift+Tab` 翻开）是后来重做的：左边一列功能，右边是那一栏的实际内容。
> 上游的视频、主题截图、贡献者名单都原样保留在下面。

## 相比上游新增

| 按键 | 功能 |
| --- | --- |
| `Shift+Tab` | 在**弹幕页**和**配置页**之间来回切（第二次按就回去） |
| `Tab` | 配置页里换功能：账号 / 分区 / 直播间信息 / 推流码（「直播间信息」栏下半格会把当前封面画出来） |
| `↑` `↓` | 在当前功能里选：分区树上下走，直播间信息里换标题 / 封面那一行 |
| `回车` | 账号栏重新扫码；直播间信息栏进编辑，**再按一次提交**；分区树里确认分区 |
| `Esc` | 取消编辑 / 收起配置页；退到弹幕页就停住，退出用 `Ctrl+C` |
| `F4` | 开播：带上直播间号与记忆的分区，成功后切到「推流码」栏显示地址与密钥，**并自动填进 OBS**（见下） |
| `F5` | 下播 |
| `Ctrl+R` | 立刻重拉一次房间信息（平时每 30 秒自己刷）；框边上写着最后一次刷上的时间 |
| `F2` `F3` `F6` | 直接翻开配置页并跳到账号 / 分区 / 直播间信息那一栏 |

顺带修掉的上游问题：

1. 弹幕连接失败会 `panic` 崩掉整个 TUI（改为 30 秒退避重连，面板照常可用）
2. 首次自动生成 `config.toml` 后必定 panic
3. `getDanmuInfo` 缺 WBI 签名与浏览器请求头，被风控挡在 `-352`，弹幕永远连不上（B 站 2025-05-26 起强制签名）
4. 控制面板的信息栏高度少给了一行，`F2 登录 … Esc 关闭` 那行提示一直没被画出来
5. 上面的「`<esc>` 退出」从上游起就没实现过（四个主题都没有 Escape 处理器），退出实际只有 `Ctrl+C`

> **注意**：`F4` 开播会让直播间立刻对外可见、并给粉丝发开播推送。人脸认证 / App 扫码验证
> （接口 `60043` / `60024`）会在面板里给出二维码，必须在手机上完成。

## OBS 联动

开播（`F4`）拿到推流凭据后，会通过 [obs-websocket](https://github.com/obsproject/obs-websocket)（OBS 28 起内置）
把**服务器**和**密钥**直接写进 OBS 的「设置 → 推流」，不用再手抄那串近百字符的密钥。

OBS 那边只需要做一次：`工具` → `WebSocket 服务器设置` → 勾上 **启用 WebSocket 服务器**，
端口默认 `4455`，密码留不留都行。

```toml
OBSFill     = true   # 开播后自动填入，不想联动就设 false
OBSHost     = ""     # 留空等于 127.0.0.1
OBSPort     = 0      # 0 = 去读 OBS 自己的 websocket 配置
OBSPassword = ""     # 留空 = 读 OBS 配置里的那个密码
```

端口和密码留空时会自动读 `~/.config/obs-studio/plugin_config/obs-websocket/config.json`，
所以一般把 `OBSFill` 打开就够了。填完**不会自动推流**，OBS 里那下「开始推流」还得你自己按。

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

界面（只有这一套）：

```text
┌──────────────────────────────────────────────────────────────┐
│ BILI（艺术字）                                      版本: vX  │
├──────────────┬───────────────────────────────────────────────┤
│ 直播间信息    │                                               │
├──────────────┤                 弹幕们                        │
│              │                                               │
│  观众列表     │                                               │
├──────────────┼───────────────────────────────────────────────┤
│ obs 推流状态  │              弹幕输入框                        │
└──────────────┴───────────────────────────────────────────────┘
```

项目文件:

```plaintext
  sender 发送弹幕的实现
  getter 获取弹幕的实现
  ui     TUI的实现
```

使用:

go run main.go

也可以从参数定义 roomId，优先级高于配置文件（-r roomId）

go run main.go -c config.toml -r 9527

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
  4. `-l int:singleline`
  5. `-s int:showtime`

快捷键:  
  1. \<esc> 返回上一层；退到底浮一句提示，再按一下收掉；退出用 Ctrl+C
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
