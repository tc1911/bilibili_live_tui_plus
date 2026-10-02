package getter

import (
	"testing"
	"time"
)

// Refresh 跑在界面的事件循环里，堵一下整个 TUI 就冻住。
// 连按也只能排队，绝不能阻塞 —— 这条挂了说明有人把它改成了不带缓存的 channel。
func TestRefreshNeverBlocks(t *testing.T) {
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			Refresh()
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Refresh 阻塞了")
	}
}
