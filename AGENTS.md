# AGENTS.md

给在本仓库里干活的 AI / 人看的项目说明。命令、结构、硬约束、容易踩的坑，都在这里。

## 这是什么

哔哩哔哩**直播弹幕 TUI 客户端**，Go 写的。是 [yaocccc/bilibili_live_tui](https://github.com/yaocccc/bilibili_live_tui) 的增强分支：
上游只能看弹幕，本分支把**开播那一整套**（扫码登录 / 选分区 / 取推流码 / 下播）搬进了同一个 TUI。

- 模块名：`github.com/tc1911/bilibili_live_tui_plus`
- 仓库目录名带加号：`bilibili_live_tui+`（模块名不带，别搞混）
- 二进制 / 包名：`bili` / `bilibili-live-tui-plus`
- `origin` 是本人的 fork，`upstream` 指 yaocccc 原仓库

## 常用命令

```bash
go build -o bili .          # 唯一的构建产出，别改名字，PKGBUILD 和 CI 都认它
go run .                    # 首次会生成 ~/.config/bili/config.toml 并提示按 F2
go run . -r 9527 -t 3       # 命令行参数优先级高于配置文件
go test ./...               # 全部测试，纯离线，不碰网络
go vet ./...
```

参数：`-c` 配置文件、`-r` 房间号、`-t` 主题、`-l` 单行、`-s` 显示时间。

## 代码结构

```text
main.go          入口：修正 locale、开两个 channel、把 ui 跑起来
config/          config.toml 读写 + 全局 config.Config / config.Auth
getter/          弹幕长连接（websocket + 二进制协议），断线自动重连
  wbi.go         WBI 签名（nav 取 img_key/sub_key -> mixin key -> w_rid）
  tools.go       zlib 解压、拆包、发包
live/            B 站 API 客户端：扫码登录、分区表、开播取推流码、下播
sender/          发弹幕（走 biligo）
ui/ui.go         按 config.Config.Theme 分发到 theme1~4
ui/themeN/       四套老主题，各自 ui.go（画）+ handler.go（收 channel）
ui/theme5/       新版主界面（-t 5）：标题栏 + 封面预览 + 观众列表 + 推流状态
ui/cover/        把封面画进终端：抓图 -> 区域平均缩图 -> 半格字符上色（跟二维码一个套路）
version/         版本号，打包时用 -ldflags -X 注入，本地 go build 是 dev
ui/control/      配置页（Shift+Tab 在两页间切）：顶部提示条 / 左栏功能 / 右栏内容，
                 功能是账号 / 分区 / 直播间信息 / 推流码，Tab 换栏，↑↓ 选，回车编辑，Esc 取消
                 面板每次露出来都走 autoFill：没登录直接摆二维码、登录了就拉分区树，
                 F2 / F3 只是手动重来一次（loggedIn / loginPending / areasPending 三个 atomic 管并发）
                 右栏「扫码」按内容显隐（showSide / hideSide）：只有登录二维码、开播验证地址时
                 才撑开，平时宽度全给分区树 —— 别再写死成 46 列
```

数据流是单向的，别绕开：

```text
getter.supervisor --> busChan (DanmuMsg) --> themeN.handler.danmuHandler --> tview
                  --> roomInfoChan (RoomInfo) --> themeN.handler.roomInfoHandler
发送：themeN 输入框 --> sender.SendMsg --> busChan（自己发的也回流显示）
```

## 硬约束（改代码前必读）

### 1. 任何一步失败都不许 panic

这程序是 TUI。`panic` 或者 `os.Exit` 会把整屏内容一起抹掉，用户看到的现象是「一打开就没了」，
连报错都看不到。历史上三次线上事故都是这么来的：弹幕连接失败 panic、自动生成 config.toml 后 panic、
sender 三次重试失败 `os.Exit(0)`。

现在的做法：错误写进 `busChan` 当系统消息显示，后台 30 秒退避重试。**改重试逻辑时保持这个形状。**

### 2. 弹幕接口必须带 WBI 签名 + 浏览器请求头

B 站 2025-05-26 起对 `getDanmuInfo` 强制 WBI 签名，缺签名一律返 `-352` 风控、`host_list` 为空。
光有签名还不够，`User-Agent` / `Origin` / `Referer` 这些头要一起带上，风控会看。

`getter/wbi.go` 里有两处**不能动**：`mixinKeyEncTab` 置换表、值里 `!'()*` 的过滤规则。
动任何一个 `w_rid` 就算错，`TestMixinKey` / `TestWbiSign` 会直接红。

### 3. 开播接口必须用 app 签名

`live/client.go` 里的 `appKey` / `appSec` 是**直播姬（bilibili link）的**，开播相关接口只能用这对，
和 web 端的 WBI 是两套东西。`encodeParams(..., sign=true)` 会自动追加 appkey 与 md5 签名。

### 4. 独立页面要自带状态行

信息页、推流码页这些是 `Pages` 里独立的一页，**盖在**控制面板上。面板底部那行 hint
此刻根本看不见 —— 往那儿写「正在上传」「改标题失败」，用户一个字都读不到。
页面自己的状态行（`panel.status`）才显示得出来。

### 5. 配置页的按键分工（别串）

- `Shift+Tab`：第一页（弹幕）↔ 第二页（配置）来回切。它是**翻页**键，不是换功能键
- `Tab`：配置页里换功能栏；弹幕页上的 Tab 要让给输入框，别抢
- `↑↓`：在当前栏里选；账号 / 推流码那两栏没有可选的东西，才顺手拿来换功能
- `回车`：账号栏重新扫码、直播间信息栏进编辑（再按一次提交）、分区树里确认
- `Esc`：取消编辑 → 收提示 → 收起配置页，退到弹幕页就停住

### 6. Esc 只负责「返回」，退出是 Ctrl+C

Esc 在弹窗 → 信息页 / 推流码页 → 面板之间逐层收，**退到底也只浮一句提示（再按一下收掉它），不退出**。
别顺手把退到底的 Esc 接成 `app.Stop()`：那会变成「一路按 Esc 就直接关掉程序」。
唯一退出键是 `Ctrl+C`（tview 自己处理的），README 里那句「`<esc>` 退出」是上游留下来的假话。

### 6. Esc 是全局 InputCapture，抢在焦点分发之前

`ui/control/control.go` 的 `onKey` 跑得比 tview 的按键分发早，所以判断顺序很讲究：
确认弹窗 > 推流码页 > 面板。`focused()` **不能**写成「焦点不在 main 就算面板」——
弹窗的按钮恰好也不在 main，那样 Esc 会关掉面板、把弹窗连焦点留在屏上变成死界面。

### 7. 分区树：父节点不可选，左右键要自己处理

tview 的上下键只停在可选节点，父分区不可选就会被整段跳过（用户卡过的坑）；
左右键在 tview 里只移动选中项，展开 / 收起得靠 `treeKeyCapture` 自己拦，
并且**在已经展开 / 收起之后要把事件交回去**，否则人回不到父分区。

### 8. 封面必须先落到 B 站图床

`UpdatePreLiveInfo` 的 `cover` 只认 `.hdslb.com` 下的图片地址，别处来的链接一律回 `100402`，
所以「改封面」是两步：`UploadImage` 传图床拿地址 → `UpdateCover` 更新。

- 传图走 `POST api.bilibili.com/x/upload/web/image`，multipart，字段是 `file` + `bucket` + `csrf`，
  返回 `data.image_url` 或 `data.location`（`location` 是 http 的，要升成 https）
- `bucket` 取 `openplatform`；换别的桶可能被拒
- `UpdatePreLiveInfo` 虽然挂在 `app-blink` 下，网页端只带 csrf 就能过，**不要**给它加 app 签名

### 9. 配置写入

- 路径固定在 `~/.config/bili/config.toml`（用 `os/user` 取 home，不是 `$HOME`）
- `config.Save()` 写回 `config.ConfigFile`，登录 / 选分区后调用
- `config.toml` 在 `.gitignore` 里，**永远不要**把它或里面的 Cookie 提交上去
- Cookie 只在本地文件里流转，不要打印到日志

### 10. locale 会重执行一次

`main.go` 的 `fixCharset`：`LANG` 是中日韩泰印地语系时把自己 `LANG=C.UTF-8` 重新 exec 一遍，
好让 tview 正确算宽字符宽度。改启动流程时别把这个丢了，也别在里面加副作用。

## 测试

`go test ./...` 必须全绿，且**不许引入真实网络请求**。

- `getter/wbi_test.go`：黄金值钉住 WBI 算法。里面的 img/sub key 是 nav 公开下发的种子（每日轮换），不是凭据
- `ui/control/control_test.go`：按真实按键事件测分区树导航与 Esc 层级，不查私有字段
- `live/live_test.go`：钉住人脸认证页地址
- `ui/control/control_test.go`：按键分层、分区树导航，外加 `TestInfoShowsKeyHints` ——
  信息栏是 height 减 2 的盒子，改高度或加行都会把提示挤掉，那个测试会红

改 B 站接口相关代码时顺手确认黄金值测试还是对的——它们存在的意义就是接口规则一变就报警。

## 发布

版本号要在**三处**保持一致：git tag、`PKGBUILD` 的 `pkgver`、README。

```bash
git tag v1.0.2 && git push origin v1.0.2   # 触发 .github/workflows/release.yml
```

CI 干的事：`go test ./...` → `CGO_ENABLED=0` 交叉编译 linux/amd64 → tar.gz + deb + rpm
（deb/rpm 走 nfpm，版本号从 tag 去掉前导 `v`）→ `gh release create`。

`PKGBUILD` 是**源码包**（不是 `-bin`），从 GitHub tag 构建；换 tag 之后必须同步更新 `sha256sums`。
注意解出来的目录名是 `bilibili_live_tui_plus-<版本>`（下划线、不带 v）。

## 许可证与来源

- 上游 `yaocccc/bilibili_live_tui` **没有 LICENSE 文件**，默认保留所有权利；上游部分版权归原作者及贡献者
- `live/` 的开播流程与 app 签名移植自 [Rsplwe/bili-live-hime](https://github.com/Rsplwe/bili-live-hime)，GPL-2.0
- 因此本仓库整体按 **GPL-2.0-only** 分发（见 `LICENSE`、`PKGBUILD`、`.github/nfpm.yaml`）

新增依赖前先看许可证能不能和 GPL-2.0 共存，并保持 `go.mod` 精简。

## 代码风格

- 注释写**为什么**，不写是什么；有线上现象就把现象写进去（比如「用户报的分区没法选择」）
- 已知的妥协用 `ponytail:` 起头标注，写清影响和更好的做法
- 提交信息用 conventional 前缀 + 中文：`fix: 网络一异常整个 TUI 就静默消失`
- 提交前跑 `gofmt -l .`

## 别做的事

- 不要把弹幕 / 开播接口换成没验证过的第三方封装
- 不要为了「更干净」把 `theme1~4` 合并，四套主题是上游的兼容面
- 不要在 `getter` 里塞请求节流以外的业务逻辑，它只负责把弹幕搬进 channel
- 不要在没有真机 / 真账号验证的情况下改开播流程，`F4` 会**立刻让直播间对外可见并推送给粉丝**
