package audit

import (
	"fmt"
	"testing"
	"time"
)

// G1-07.2: Audit Chain Append Benchmark
// 覆盖：带签名 vs 无签名、不同链长度（10/100/1000）。

// makeEntry 构造一个用于 benchmark 的 AuditEntry。
func makeEntry(i int) *AuditEntry {
	return &AuditEntry{
		Source:   "agent",
		Action:   "execute",
		Input:    map[string]any{"step": i, "query": "benchmark-input"},
		Output:   map[string]any{"result": "benchmark-output", "score": float64(i) * 0.1},
		Duration: time.Duration(i) * time.Millisecond,
		Metadata: map[string]string{
			"trace_id": fmt.Sprintf("trace-%d", i),
			"session":  "bench",
		},
	}
}

// BenchmarkAuditChainAppend 测试单次 Append 的开销（无签名 vs 带签名）。
func BenchmarkAuditChainAppend(b *testing.B) {
	cases := []struct {
		name      string
		sign      bool
		maxEntries int
	}{
		{"unsigned_unbounded", false, 0},
		{"signed_unbounded", true, 0},
		{"unsigned_cap_100", false, 100},
		{"signed_cap_1000", true, 1000},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			cfg := AuditConfig{
				Enabled:    true,
				MaxEntries: c.maxEntries,
			}
			if c.sign {
				cfg.SignatureKey = []byte("bench-hmac-key-32-bytes-long-xx")
			}
			chain, err := NewAuditChain(cfg)
			if err != nil {
				b.Fatalf("NewAuditChain: %v", err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := chain.Append(makeEntry(i))
				if err != nil {
					b.Fatalf("Append: %v", err)
				}
			}
		})
	}
}

// BenchmarkAuditChainAppendPrepopulated 测试在不同已有链长度下的 Append 开销
// （用于评估链长度对 Append 性能的影响，特别是哈希链的 PrevHash 取值）。
func BenchmarkAuditChainAppendPrepopulated(b *testing.B) {
	for _, size := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("pre_%d", size), func(b *testing.B) {
			chain, _ := NewAuditChain(AuditConfig{Enabled: true})
			// 预填充 size 条
			for i := 0; i < size; i++ {
				_, _ = chain.Append(makeEntry(i))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = chain.Append(makeEntry(size + i))
			}
		})
	}
}

// BenchmarkAuditChainQuery 测试在已填充链上的查询开销。
// query_test.go 提供了 Query 方法；这里只测常见 filter 场景。
func BenchmarkAuditChainQuery(b *testing.B) {
	for _, size := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("entries_%d", size), func(b *testing.B) {
			chain, _ := NewAuditChain(AuditConfig{Enabled: true})
			for i := 0; i < size; i++ {
				_, _ = chain.Append(makeEntry(i))
			}
			filter := QueryFilter{Source: "agent"}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = chain.Query(filter)
			}
		})
	}
}

// BenchmarkAuditChainSnapshot 测试 Query(QueryFilter{})（等价全量读取快照）开销。
func BenchmarkAuditChainSnapshot(b *testing.B) {
	for _, size := range []int{100, 1000} {
		b.Run(fmt.Sprintf("entries_%d", size), func(b *testing.B) {
			chain, _ := NewAuditChain(AuditConfig{Enabled: true})
			for i := 0; i < size; i++ {
				_, _ = chain.Append(makeEntry(i))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = chain.Query(QueryFilter{})
			}
		})
	}
}

// 简单 sanity check 确保以上 benchmark 数据可生成。
func TestBenchAuditSanity(t *testing.T) {
	chain, _ := NewAuditChain(AuditConfig{Enabled: true, SignatureKey: []byte("k")})
	for i := 0; i < 5; i++ {
		_, err := chain.Append(makeEntry(i))
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if chain.Len() != 5 {
		t.Errorf("Len = %d, want 5", chain.Len())
	}
}
