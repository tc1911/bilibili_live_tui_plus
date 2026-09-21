# Maintainer: tc1911 <171405782+tc1911@users.noreply.github.com>
# 源码包（不是 -bin）：从 GitHub tag 构建。
# 上游 yaocccc/bilibili_live_tui 的增强分支，加了扫码登录 / 分区选择 / 开播取推流码。
pkgname=bilibili-live-tui-plus
pkgver=1.0.0
pkgrel=1
pkgdesc='Bilibili 直播弹幕 TUI 客户端（扫码登录 / 分区选择 / 开播取推流码）'
arch=('x86_64')
url='https://github.com/tc1911/bilibili_live_tui_plus'
license=('GPL-2.0-only')
# 纯 Go，只有 glibc（按 Arch 的 Go 打包规范开了 CGO/PIE，net 走系统解析器）。
# ca-certificates 是运行时要的：Go 不内建根证书，访问 api.live.bilibili.com 必须读系统证书库。
depends=('glibc' 'ca-certificates')
makedepends=('go')
options=('!debug')
source=("$pkgname-$pkgver.tar.gz::https://github.com/tc1911/bilibili_live_tui_plus/archive/refs/tags/v$pkgver.tar.gz")
sha256sums=('fc88a97e9087e634a1c78d7567c39f41d9f59c2d598f7dab69c929a80c363c62')

# GitHub 自动生成的 tarball 解出来是 <仓库名>-<版本>：仓库名带下划线，版本号不带 v
_srcname="bilibili_live_tui_plus-$pkgver"

build() {
	cd "$_srcname"
	export CGO_CFLAGS="${CFLAGS}"
	export CGO_CPPFLAGS="${CPPFLAGS}"
	export CGO_CXXFLAGS="${CXXFLAGS}"
	export GOFLAGS="-buildmode=pie -trimpath -mod=readonly -modcacherw"
	go build -o bili .
}

check() {
	cd "$_srcname"
	go test ./...
}

package() {
	cd "$_srcname"
	install -Dm755 bili "$pkgdir/usr/bin/bili"
	install -Dm644 LICENSE "$pkgdir/usr/share/licenses/$pkgname/LICENSE"
}
