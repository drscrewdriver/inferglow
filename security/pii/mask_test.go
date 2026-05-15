package pii

import (
	"regexp"
	"strings"
	"testing"
)

func TestNewMasker_DefaultsApplied(t *testing.T) {
	m := NewMasker(MaskConfig{})
	cfg := m.Config()
	if cfg.MaskChar != "***" {
		t.Errorf("default MaskChar = %q, want %q", cfg.MaskChar, "***")
	}
	if cfg.KeepPrefix != 0 {
		t.Errorf("default KeepPrefix = %d, want 0", cfg.KeepPrefix)
	}
	if cfg.ApplyOn != MaskOnInput|MaskOnOutput {
		t.Errorf("default ApplyOn = %d, want %d", cfg.ApplyOn, MaskOnInput|MaskOnOutput)
	}
	if len(cfg.Patterns) == 0 {
		t.Error("default Patterns should be non-empty (DefaultPatterns)")
	}
}

func TestNewMasker_CustomMaskChar(t *testing.T) {
	m := NewMasker(MaskConfig{MaskChar: "[REDACTED]"})
	if got := m.Config().MaskChar; got != "[REDACTED]" {
		t.Errorf("MaskChar = %q, want [REDACTED]", got)
	}
}

func TestMask_Email(t *testing.T) {
	m := NewMasker(MaskConfig{ApplyOn: MaskOnInput | MaskOnOutput})
	in := "contact me at alice@example.com for details"
	out := m.Mask(in)
	if strings.Contains(out, "alice@example.com") {
		t.Errorf("email not masked: %q", out)
	}
	if !strings.Contains(out, "***") {
		t.Errorf("expected mask char in output, got %q", out)
	}
}

func TestMask_Phone(t *testing.T) {
	m := NewMasker(MaskConfig{})
	in := "my phone is 13812345678 call me"
	out := m.Mask(in)
	if strings.Contains(out, "13812345678") {
		t.Errorf("phone not masked: %q", out)
	}
}

func TestMask_IDCard(t *testing.T) {
	m := NewMasker(MaskConfig{})
	in := "id card 11010119900307888X registered"
	out := m.Mask(in)
	if strings.Contains(out, "11010119900307888X") {
		t.Errorf("id card not masked: %q", out)
	}
}

func TestMask_CreditCard(t *testing.T) {
	m := NewMasker(MaskConfig{})
	cases := []string{
		"card 1234567812345678 expired",
		"card 1234-5678-1234-5678 expired",
		"card 1234 5678 1234 5678 expired",
	}
	for _, in := range cases {
		out := m.Mask(in)
		if strings.Contains(out, "1234") {
			t.Errorf("credit card not masked for %q: got %q", in, out)
		}
	}
}

func TestMask_IPAddress(t *testing.T) {
	m := NewMasker(MaskConfig{})
	in := "server at 192.168.1.1 is down"
	out := m.Mask(in)
	if strings.Contains(out, "192.168.1.1") {
		t.Errorf("ip not masked: %q", out)
	}
}

func TestMask_BankAccount(t *testing.T) {
	m := NewMasker(MaskConfig{})
	in := "bank account 6222020200011111222"
	out := m.Mask(in)
	if strings.Contains(out, "6222020200011111222") {
		t.Errorf("bank account not masked: %q", out)
	}
}

func TestMask_KeepPrefixZero(t *testing.T) {
	m := NewMasker(MaskConfig{KeepPrefix: 0})
	out := m.Mask("email alice@example.com now")
	// With KeepPrefix=0 the entire match is replaced with the mask char.
	if !strings.Contains(out, "***") {
		t.Errorf("expected full mask, got %q", out)
	}
	if strings.Contains(out, "alice") {
		t.Errorf("expected no prefix kept, got %q", out)
	}
}

func TestMask_KeepPrefixTwo(t *testing.T) {
	m := NewMasker(MaskConfig{KeepPrefix: 2})
	out := m.Mask("email alice@example.com now")
	// Keep first 2 chars "al", rest masked.
	if !strings.Contains(out, "al***") {
		t.Errorf("expected 'al***' in output, got %q", out)
	}
}

func TestMask_KeepPrefixThree(t *testing.T) {
	m := NewMasker(MaskConfig{KeepPrefix: 3})
	out := m.Mask("email alice@example.com now")
	// Keep first 3 chars "ali", rest masked.
	if !strings.Contains(out, "ali***") {
		t.Errorf("expected 'ali***' in output, got %q", out)
	}
}

func TestMask_KeepPrefixExceedsMatchLength(t *testing.T) {
	m := NewMasker(MaskConfig{KeepPrefix: 100})
	in := "email alice@example.com now"
	out := m.Mask(in)
	// When KeepPrefix >= match length, the match is returned unchanged.
	if out != in {
		t.Errorf("expected unchanged output when KeepPrefix >= match len, got %q", out)
	}
}

func TestMaskInput_WhenMaskOnInputEnabled(t *testing.T) {
	m := NewMasker(MaskConfig{ApplyOn: MaskOnInput})
	in := "contact alice@example.com"
	out := m.MaskInput(in)
	if strings.Contains(out, "alice@example.com") {
		t.Errorf("MaskInput should mask when MaskOnInput set, got %q", out)
	}
}

func TestMaskInput_WhenMaskOnInputDisabled(t *testing.T) {
	m := NewMasker(MaskConfig{ApplyOn: MaskOnOutput})
	in := "contact alice@example.com"
	out := m.MaskInput(in)
	if out != in {
		t.Errorf("MaskInput should be a no-op when MaskOnInput not set, got %q want %q", out, in)
	}
}

func TestMaskOutput_WhenMaskOnOutputEnabled(t *testing.T) {
	m := NewMasker(MaskConfig{ApplyOn: MaskOnOutput})
	in := "contact alice@example.com"
	out := m.MaskOutput(in)
	if strings.Contains(out, "alice@example.com") {
		t.Errorf("MaskOutput should mask when MaskOnOutput set, got %q", out)
	}
}

func TestMaskOutput_WhenMaskOnOutputDisabled(t *testing.T) {
	m := NewMasker(MaskConfig{ApplyOn: MaskOnInput})
	in := "contact alice@example.com"
	out := m.MaskOutput(in)
	if out != in {
		t.Errorf("MaskOutput should be a no-op when MaskOnOutput not set, got %q want %q", out, in)
	}
}

func TestMask_BothInputAndOutput(t *testing.T) {
	m := NewMasker(MaskConfig{ApplyOn: MaskOnInput | MaskOnOutput})
	in := "contact alice@example.com"
	if m.MaskInput(in) == in {
		t.Error("MaskInput should have masked")
	}
	if m.MaskOutput(in) == in {
		t.Error("MaskOutput should have masked")
	}
}

func TestMask_MultiplePIITypesInOneText(t *testing.T) {
	m := NewMasker(MaskConfig{KeepPrefix: 0})
	in := "email alice@example.com, phone 13812345678, ip 10.0.0.1, card 1111222233334444"
	out := m.Mask(in)
	for _, secret := range []string{
		"alice@example.com",
		"13812345678",
		"10.0.0.1",
		"1111222233334444",
	} {
		if strings.Contains(out, secret) {
			t.Errorf("secret %q not masked in output: %q", secret, out)
		}
	}
}

func TestMask_NormalTextNotMasked(t *testing.T) {
	m := NewMasker(MaskConfig{})
	cases := []string{
		"hello world",
		"the quick brown fox jumps over the lazy dog",
		"order #12345 was shipped",
		"meeting at 3pm",
	}
	for _, in := range cases {
		out := m.Mask(in)
		if out != in {
			t.Errorf("normal text should not be masked: got %q want %q", out, in)
		}
	}
}

func TestMask_CustomPattern(t *testing.T) {
	custom := regexp.MustCompile(`SECRET-\d+`)
	m := NewMasker(MaskConfig{
		Patterns: map[PIIType]*regexp.Regexp{
			Custom: custom,
		},
		ApplyOn: MaskOnInput | MaskOnOutput,
	})
	in := "token is SECRET-12345 leaked"
	out := m.Mask(in)
	if strings.Contains(out, "SECRET-12345") {
		t.Errorf("custom pattern not masked: %q", out)
	}
	if !strings.Contains(out, "***") {
		t.Errorf("expected mask char in output, got %q", out)
	}
}

func TestMask_DoesNotMutateConfigPatterns(t *testing.T) {
	m := NewMasker(MaskConfig{})
	before := len(m.Config().Patterns)
	_ = m.Mask("alice@example.com 13812345678")
	after := len(m.Config().Patterns)
	if before != after {
		t.Errorf("Mask mutated patterns map: %d -> %d", before, after)
	}
}

func TestMask_Idempotent(t *testing.T) {
	m := NewMasker(MaskConfig{KeepPrefix: 2})
	in := "contact alice@example.com"
	once := m.Mask(in)
	twice := m.Mask(once)
	if once != twice {
		t.Errorf("Mask is not idempotent: first=%q second=%q", once, twice)
	}
}
