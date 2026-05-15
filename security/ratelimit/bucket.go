package ratelimit

import (
	"context"
	"sync"
	"time"
)

// TokenBucket 是经典的令牌桶限流器。桶以 refillRate（每分钟）的速率
// 补充令牌，容量上限为 capacity。每次 Take 操作尝试取出若干令牌，
// 成功则放行，失败则拒绝或阻塞。
//
// TokenBucket 线程安全，可在并发请求间复用。
type TokenBucket struct {
	capacity   int
	tokens     int
	refillRate int // 每分钟补充的令牌数
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket 创建一个满载的令牌桶。capacity 为桶容量（即最大
// 突发请求数），refillRate 为每分钟补充的令牌数。refillRate 为 0
// 表示不补充（一次性配额）。
func NewTokenBucket(capacity, refillRate int) *TokenBucket {
	if capacity < 0 {
		capacity = 0
	}
	if refillRate < 0 {
		refillRate = 0
	}
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Take 尝试取出 count 个令牌。成功返回 true 并扣减相应令牌；
// 不足则返回 false 且不扣减。count <= 0 时直接返回 true。
// Take 在执行前会先调用 Refill 按时间差补充令牌。
func (b *TokenBucket) Take(count int) bool {
	if count <= 0 {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refillLocked()
	if b.tokens >= count {
		b.tokens -= count
		return true
	}
	return false
}

// TakeBlock 阻塞直到取出 count 个令牌或 ctx 被取消。成功返回 nil，
// ctx 取消返回 ctx.Err()。count <= 0 时直接返回 nil。
// 用于 LimitSoft 模式下的排队等待。
func (b *TokenBucket) TakeBlock(ctx context.Context, count int) error {
	if count <= 0 {
		return nil
	}
	for {
		b.mu.Lock()
		b.refillLocked()
		if b.tokens >= count {
			b.tokens -= count
			b.mu.Unlock()
			return nil
		}
		needed := count - b.tokens
		var wait time.Duration
		if b.refillRate > 0 {
			wait = time.Duration(needed) * time.Minute / time.Duration(b.refillRate)
		} else {
			wait = time.Hour
		}
		b.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// Refill 按时间差补充令牌，不超过 capacity。线程安全。
func (b *TokenBucket) Refill() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refillLocked()
}

// refillLocked 是 Refill 的内部实现，调用方必须持有 b.mu。
func (b *TokenBucket) refillLocked() {
	if b.refillRate <= 0 {
		return
	}
	now := time.Now()
	elapsed := now.Sub(b.lastRefill)
	if elapsed <= 0 {
		return
	}
	refill := int(elapsed.Seconds() * float64(b.refillRate) / 60.0)
	if refill > 0 {
		b.tokens += refill
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.lastRefill = now
	}
}

// Tokens 返回当前可用令牌数（含补充）。线程安全。
func (b *TokenBucket) Tokens() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refillLocked()
	return b.tokens
}

// Capacity 返回桶容量。
func (b *TokenBucket) Capacity() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.capacity
}

// RefillRate 返回每分钟补充速率。
func (b *TokenBucket) RefillRate() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.refillRate
}
