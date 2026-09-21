package live

import "testing"

// 人脸认证地址是写死的，B站改一次就静默失效，钉住它。
func TestFaceAuthURL(t *testing.T) {
	got := faceAuthURL(123456)
	want := "https://www.bilibili.com/blackboard/live/face-auth-middle.html?source_event=400&mid=123456"
	if got != want {
		t.Fatalf("faceAuthURL = %q, want %q", got, want)
	}
}
