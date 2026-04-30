package sandbox

import (
	"runtime"
	"sort"
	"testing"
)

func TestOSConstants(t *testing.T) {
	cases := []struct {
		name string
		got  OS
		want string
	}{
		{"OSDarwin", OSDarwin, "darwin"},
		{"OSLinux", OSLinux, "linux"},
		{"OSWindows", OSWindows, "windows"},
		{"OSUnknown", OSUnknown, "unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if string(c.got) != c.want {
				t.Errorf("%s = %q, want %q", c.name, string(c.got), c.want)
			}
		})
	}
}

func TestProviderKindConstants(t *testing.T) {
	cases := []struct {
		name string
		got  ProviderKind
		want string
	}{
		{"ProviderTrustedLocal", ProviderTrustedLocal, "trusted_local"},
		{"ProviderDocker", ProviderDocker, "docker"},
		{"ProviderGVisor", ProviderGVisor, "gvisor"},
		{"ProviderSeatbelt", ProviderSeatbelt, "seatbelt"},
		{"ProviderBubblewrap", ProviderBubblewrap, "bubblewrap"},
		{"ProviderLandlock", ProviderLandlock, "landlock"},
		{"ProviderWindowsRuntime", ProviderWindowsRuntime, "windows_runtime"},
		{"ProviderE2B", ProviderE2B, "e2b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if string(c.got) != c.want {
				t.Errorf("%s = %q, want %q", c.name, string(c.got), c.want)
			}
		})
	}
}

func TestDetectOS(t *testing.T) {
	got := DetectOS()
	var want OS
	switch runtime.GOOS {
	case "darwin":
		want = OSDarwin
	case "linux":
		want = OSLinux
	case "windows":
		want = OSWindows
	default:
		want = OSUnknown
	}
	if got != want {
		t.Errorf("DetectOS() = %q, want %q (runtime.GOOS=%q)", got, want, runtime.GOOS)
	}
}

func TestDefaultProviderMatrixNotNil(t *testing.T) {
	if DefaultProviderMatrix == nil {
		t.Fatal("DefaultProviderMatrix is nil")
	}
}

func TestDefaultProviderMatrixCoverage(t *testing.T) {
	crossOS := []OS{OSDarwin, OSLinux, OSWindows}
	// 这 4 个 provider 在所有 3 个 OS 上都应可用
	for _, kind := range []ProviderKind{ProviderTrustedLocal, ProviderDocker, ProviderGVisor, ProviderE2B} {
		for _, os := range crossOS {
			if !DefaultProviderMatrix.IsSupported(kind, os) {
				t.Errorf("IsSupported(%s, %s) = false, want true", kind, os)
			}
		}
	}
	// ProviderSeatbelt 仅 darwin true
	if !DefaultProviderMatrix.IsSupported(ProviderSeatbelt, OSDarwin) {
		t.Errorf("IsSupported(ProviderSeatbelt, OSDarwin) = false, want true")
	}
	for _, os := range []OS{OSLinux, OSWindows} {
		if DefaultProviderMatrix.IsSupported(ProviderSeatbelt, os) {
			t.Errorf("IsSupported(ProviderSeatbelt, %s) = true, want false", os)
		}
	}
	// ProviderBubblewrap 仅 linux true
	if !DefaultProviderMatrix.IsSupported(ProviderBubblewrap, OSLinux) {
		t.Errorf("IsSupported(ProviderBubblewrap, OSLinux) = false, want true")
	}
	for _, os := range []OS{OSDarwin, OSWindows} {
		if DefaultProviderMatrix.IsSupported(ProviderBubblewrap, os) {
			t.Errorf("IsSupported(ProviderBubblewrap, %s) = true, want false", os)
		}
	}
	// ProviderLandlock 仅 linux true
	if !DefaultProviderMatrix.IsSupported(ProviderLandlock, OSLinux) {
		t.Errorf("IsSupported(ProviderLandlock, OSLinux) = false, want true")
	}
	for _, os := range []OS{OSDarwin, OSWindows} {
		if DefaultProviderMatrix.IsSupported(ProviderLandlock, os) {
			t.Errorf("IsSupported(ProviderLandlock, %s) = true, want false", os)
		}
	}
	// ProviderWindowsRuntime 仅 windows true
	if !DefaultProviderMatrix.IsSupported(ProviderWindowsRuntime, OSWindows) {
		t.Errorf("IsSupported(ProviderWindowsRuntime, OSWindows) = false, want true")
	}
	for _, os := range []OS{OSDarwin, OSLinux} {
		if DefaultProviderMatrix.IsSupported(ProviderWindowsRuntime, os) {
			t.Errorf("IsSupported(ProviderWindowsRuntime, %s) = true, want false", os)
		}
	}
}

func TestIsProviderSupportedOnOS(t *testing.T) {
	if !IsProviderSupportedOnOS(ProviderSeatbelt, OSDarwin) {
		t.Errorf("IsProviderSupportedOnOS(ProviderSeatbelt, OSDarwin) = false, want true")
	}
	if IsProviderSupportedOnOS(ProviderSeatbelt, OSLinux) {
		t.Errorf("IsProviderSupportedOnOS(ProviderSeatbelt, OSLinux) = true, want false")
	}
}

func TestAvailableProvidersOnOS_Darwin(t *testing.T) {
	got := AvailableProvidersOnOS(OSDarwin)
	want := []ProviderKind{ProviderDocker, ProviderE2B, ProviderGVisor, ProviderSeatbelt, ProviderTrustedLocal}
	if len(got) != len(want) {
		t.Fatalf("AvailableProvidersOnOS(OSDarwin) returned %d providers, want %d: %+v", len(got), len(want), got)
	}
	// 验证已按字典序排序
	sorted := make([]ProviderKind, len(got))
	copy(sorted, got)
	sort.Slice(sorted, func(i, j int) bool { return string(sorted[i]) < string(sorted[j]) })
	for i := range got {
		if got[i] != sorted[i] {
			t.Errorf("AvailableProvidersOnOS(OSDarwin) not sorted at index %d: got %q, want %q", i, got[i], sorted[i])
		}
		if got[i] != want[i] {
			t.Errorf("AvailableProvidersOnOS(OSDarwin)[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAvailableProvidersOnOS_Linux(t *testing.T) {
	got := AvailableProvidersOnOS(OSLinux)
	want := []ProviderKind{ProviderBubblewrap, ProviderDocker, ProviderE2B, ProviderGVisor, ProviderLandlock, ProviderTrustedLocal}
	if len(got) != len(want) {
		t.Fatalf("AvailableProvidersOnOS(OSLinux) returned %d providers, want %d: %+v", len(got), len(want), got)
	}
	sorted := make([]ProviderKind, len(got))
	copy(sorted, got)
	sort.Slice(sorted, func(i, j int) bool { return string(sorted[i]) < string(sorted[j]) })
	for i := range got {
		if got[i] != sorted[i] {
			t.Errorf("AvailableProvidersOnOS(OSLinux) not sorted at index %d: got %q, want %q", i, got[i], sorted[i])
		}
		if got[i] != want[i] {
			t.Errorf("AvailableProvidersOnOS(OSLinux)[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAvailableProvidersOnOS_Windows(t *testing.T) {
	got := AvailableProvidersOnOS(OSWindows)
	want := []ProviderKind{ProviderDocker, ProviderE2B, ProviderGVisor, ProviderTrustedLocal, ProviderWindowsRuntime}
	if len(got) != len(want) {
		t.Fatalf("AvailableProvidersOnOS(OSWindows) returned %d providers, want %d: %+v", len(got), len(want), got)
	}
	sorted := make([]ProviderKind, len(got))
	copy(sorted, got)
	sort.Slice(sorted, func(i, j int) bool { return string(sorted[i]) < string(sorted[j]) })
	for i := range got {
		if got[i] != sorted[i] {
			t.Errorf("AvailableProvidersOnOS(OSWindows) not sorted at index %d: got %q, want %q", i, got[i], sorted[i])
		}
		if got[i] != want[i] {
			t.Errorf("AvailableProvidersOnOS(OSWindows)[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAvailableProvidersOnOS_Unknown(t *testing.T) {
	got := AvailableProvidersOnOS(OSUnknown)
	if len(got) != 0 {
		t.Errorf("AvailableProvidersOnOS(OSUnknown) returned %d providers, want empty: %+v", len(got), got)
	}
}

func TestProviderMatrixIsSupportedOnNilReceiver(t *testing.T) {
	var m *ProviderMatrix
	if m.IsSupported(ProviderDocker, OSLinux) {
		t.Errorf("nil ProviderMatrix.IsSupported should return false")
	}
}

func TestProviderMatrixAvailableProvidersOnNilReceiver(t *testing.T) {
	var m *ProviderMatrix
	got := m.AvailableProviders(OSLinux)
	if len(got) != 0 {
		t.Errorf("nil ProviderMatrix.AvailableProviders should return empty, got %+v", got)
	}
}
