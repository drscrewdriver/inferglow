//go:build integration

package redis

import (
	"context"
	"os"
	"testing"

	contextmgr "github.com/inferglow/context"
)

// mockClient implements Client for testing.
type mockClient struct {
	data map[string]string
}

func newMockClient() *mockClient {
	return &mockClient{data: make(map[string]string)}
}

func (m *mockClient) Get(ctx context.Context, key string) (string, error) {
	v, ok := m.data[key]
	if !ok {
		return "", nil
	}
	return v, nil
}

func (m *mockClient) Set(ctx context.Context, key string, value interface{}, ttl int) error {
	m.data[key] = value.(string)
	return nil
}

func (m *mockClient) Del(ctx context.Context, keys ...string) error {
	for _, k := range keys {
		delete(m.data, k)
	}
	return nil
}

func (m *mockClient) Keys(ctx context.Context, pattern string) ([]string, error) {
	var out []string
	for k := range m.data {
		out = append(out, k)
	}
	return out, nil
}

func TestRedisCache_New(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("Skipping: REDIS_TEST_ADDR not set")
	}

	_ = addr // In a real integration test, connect to Redis here.
	// For now, use a mock client to verify the CacheStore works.
	client := newMockClient()
	cache := NewCacheStore(client, "test-session")
	if cache == nil {
		t.Fatal("NewCacheStore returned nil")
	}
}

func TestRedisCache_CacheStep(t *testing.T) {
	client := newMockClient()
	cache := NewCacheStore(client, "test-session")

	ctx := context.Background()
	step := contextmgr.StepRecord{
		StepID: 1, Type: "reasoning", Content: "cached content",
	}
	if err := cache.CacheStep(ctx, step); err != nil {
		t.Fatalf("CacheStep returned error: %v", err)
	}

	got, err := cache.GetCachedStep(ctx, 1)
	if err != nil {
		t.Fatalf("GetCachedStep returned error: %v", err)
	}
	if got == nil {
		t.Fatal("GetCachedStep returned nil")
	}
	if got.StepID != 1 || got.Content != "cached content" {
		t.Errorf("unexpected cached step: %+v", got)
	}
}

func TestRedisCache_Flush(t *testing.T) {
	client := newMockClient()
	cache := NewCacheStore(client, "test-session")

	ctx := context.Background()
	step := contextmgr.StepRecord{StepID: 1, Content: "flush me"}
	if err := cache.CacheStep(ctx, step); err != nil {
		t.Fatal(err)
	}

	if err := cache.Flush(ctx); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	_, err := cache.GetCachedStep(ctx, 1)
	if err == nil {
		t.Error("expected error after flush, got nil")
	}
}
