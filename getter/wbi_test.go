package getter

import "testing"

// 这两个 key 是 nav 公开下发的 WBI 种子（每日轮换），只用来钉住算法，不是凭据。
const (
	goldenImg   = "7cd084941338484aae1ad9425b84077c"
	goldenSub   = "4932caff0ff746eab6f01bf08b70ac45"
	goldenMixin = "ea1db124af3c7062474693fa704f4ff8"
)

// 置换表或拼接口径错了，w_rid 就全错，getDanmuInfo 会永远返 -352。
func TestMixinKey(t *testing.T) {
	if got := mixinKey(goldenImg, goldenSub); got != goldenMixin {
		t.Fatalf("mixinKey = %q, want %q", got, goldenMixin)
	}
}

// 黄金值由官方 web 端同款算法（python urlencode + md5）算出。
func TestWbiSign(t *testing.T) {
	got := wbiSign(map[string]string{
		"id":           "123456",
		"type":         "0",
		"web_location": "444.8",
	}, goldenMixin, 1700000000)

	want := "id=123456&type=0&web_location=444.8&wts=1700000000&w_rid=54a1e0ac653a45674a24985c37fe7e32"
	if got != want {
		t.Fatalf("wbiSign =\n  %s\nwant\n  %s", got, want)
	}
}

// 值里的 !'()* 必须在签名前剔掉，否则服务端算出的 w_rid 不一致。
func TestWbiSignStripsBannedChars(t *testing.T) {
	a := wbiSign(map[string]string{"q": "a!b'c(d)e*f"}, goldenMixin, 1700000000)
	b := wbiSign(map[string]string{"q": "abcdef"}, goldenMixin, 1700000000)
	if a != b {
		t.Fatalf("值未按 WBI 规则过滤:\n  %s\n  %s", a, b)
	}
}
