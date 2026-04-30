package sandbox

import (
	"testing"
)

// ---------- stub 层测试（跨平台） ----------

func TestSeatbeltHandleStubCreateReturnsError(t *testing.T) {
	provider := &SeatbeltProvider{}
	_, err := provider.CreateHandle(nil, nil)
	if err == nil {
		t.Error("expected error from SeatbeltProvider.CreateHandle on non-darwin")
	}
}

func TestSeatbeltHandleStubStatusIsCreated(t *testing.T) {
	// 在非 darwin 上无法创建 handle，所以只测试 stub provider
	provider := &SeatbeltProvider{}
	h, err := provider.CreateHandle(nil, nil)
	if err == nil {
		t.Fatal("expected error, got handle")
	}
	if h != nil {
		t.Error("expected nil handle on error")
	}
}
