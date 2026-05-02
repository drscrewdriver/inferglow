package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ThreeZoneSession implements the Reasonix-style three-zone context architecture:
//   - Zone 1: ImmutablePrefix (system prompt + tool definitions, byte-stable)
//   - Zone 2: AppendOnlyHistory (conversation history, append-only)
//   - Zone 3: VolatileScratch (per-round reasoning, cleared each round)
//
// This structure maximizes prefix cache hits: Zone 1 never changes (100% hit),
// Zone 2 only grows (incremental hit), Zone 3 doesn't participate in caching.
type ThreeZoneSession struct {
	mu sync.RWMutex

	id string

	// Zone 1: Immutable Prefix
	// Set once via SetImmutablePrefix, never modified afterward.
	immutablePrefix    []ChatMessage // typically [system prompt]
	immutableToolsJSON []byte        // byte-stable serialized tools
	immutableHash      string        // SHA-256 of immutablePrefix+tools

	// Zone 2: Append-Only History
	// Only AddToHistory appends; resize strategies may shrink from the head
	// (snip), prune low-value (prune), or summarize (summary).
	appendOnlyHistory []ChatMessage
	maxHistoryBytes   int

	// Zone 3: Volatile Scratchpad
	// Cleared at the start of each round. Not part of prefix cache.
	volatileScratch []ChatMessage

	// Resize strategy chain: snip → prune → summary
	snipStrategy    ResizeHandler
	pruneStrategy   ResizeHandler
	summaryStrategy ResizeHandler

	// Memo for handlers
	memo map[string]any
}

// NewThreeZoneSession creates a new three-zone session.
func NewThreeZoneSession(id string, maxHistoryBytes int) *ThreeZoneSession {
	return &ThreeZoneSession{
		id:                id,
		appendOnlyHistory: make([]ChatMessage, 0),
		maxHistoryBytes:   maxHistoryBytes,
		memo:              make(map[string]any),
	}
}

// SetImmutablePrefix sets Zone 1 (system prompt + tools).
// Can only be called once per session; subsequent calls return error.
// The tools are serialized with byte-stable ordering for cache consistency.
func (s *ThreeZoneSession) SetImmutablePrefix(systemPrompt string, tools []any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.immutablePrefix != nil {
		return fmt.Errorf("immutable prefix already set")
	}
	s.immutablePrefix = []ChatMessage{{Role: "system", Content: systemPrompt, Timestamp: time.Now()}}
	// Stable serialization of tools
	if len(tools) > 0 {
		if b, err := stableMarshal(tools); err == nil {
			s.immutableToolsJSON = b
		}
	}
	s.immutableHash = s.computeImmutableHash()
	return nil
}

// AddToHistory appends a message to Zone 2 (append-only).
// Triggers resize if total bytes exceed maxHistoryBytes.
func (s *ThreeZoneSession) AddToHistory(msg ChatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendOnlyHistory = append(s.appendOnlyHistory, msg)
	// Check if resize needed
	if s.totalHistoryBytes() > s.maxHistoryBytes {
		s.runResizeChain()
	}
}

// SetVolatileScratch replaces Zone 3 contents.
func (s *ThreeZoneSession) SetVolatileScratch(msgs []ChatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.volatileScratch = make([]ChatMessage, len(msgs))
	copy(s.volatileScratch, msgs)
}

// ClearVolatileScratch empties Zone 3. Called at end of each round.
func (s *ThreeZoneSession) ClearVolatileScratch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.volatileScratch = nil
}

// BuildPrompt builds the full prompt for the LLM:
// Zone 1 (immutable) + Zone 2 (append-only history) + Zone 3 (volatile scratch).
// Zone 1 and Zone 2 are in cache-stable order; Zone 3 is appended last.
func (s *ThreeZoneSession) BuildPrompt() []ChatMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ChatMessage, 0, len(s.immutablePrefix)+len(s.appendOnlyHistory)+len(s.volatileScratch))
	result = append(result, s.immutablePrefix...)
	result = append(result, s.appendOnlyHistory...)
	result = append(result, s.volatileScratch...)
	return result
}

// ImmutableHash returns the SHA-256 hash of Zone 1, for cache invalidation detection.
// Two sessions with the same hash can share prefix cache.
func (s *ThreeZoneSession) ImmutableHash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.immutableHash
}

// SetResizeStrategies configures the snip/prune/summary chain.
func (s *ThreeZoneSession) SetResizeStrategies(snip, prune, summary ResizeHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snipStrategy = snip
	s.pruneStrategy = prune
	s.summaryStrategy = summary
}

// runResizeChain runs snip → prune → summary in order until under budget.
// snip is cache-friendly (only shortens tail), prune is partial, summary is full invalidation.
func (s *ThreeZoneSession) runResizeChain() {
	// L1: snip
	if s.snipStrategy != nil {
		resized, err := s.snipStrategy(s.appendOnlyHistory, s.appendOnlyHistory)
		if err == nil {
			s.appendOnlyHistory = resized
			if s.totalHistoryBytes() <= s.maxHistoryBytes {
				return
			}
		}
	}
	// L2: prune
	if s.pruneStrategy != nil {
		resized, err := s.pruneStrategy(s.appendOnlyHistory, s.appendOnlyHistory)
		if err == nil {
			s.appendOnlyHistory = resized
			if s.totalHistoryBytes() <= s.maxHistoryBytes {
				return
			}
		}
	}
	// L3: summary (cache-busting last resort)
	if s.summaryStrategy != nil {
		resized, err := s.summaryStrategy(s.appendOnlyHistory, s.appendOnlyHistory)
		if err == nil {
			s.appendOnlyHistory = resized
		}
	}
}

func (s *ThreeZoneSession) totalHistoryBytes() int {
	total := 0
	for _, m := range s.appendOnlyHistory {
		total += len(ContentToString(m.Content))
	}
	return total
}

func (s *ThreeZoneSession) computeImmutableHash() string {
	h := sha256.New()
	for _, m := range s.immutablePrefix {
		h.Write([]byte(ContentToString(m.Content)))
	}
	h.Write(s.immutableToolsJSON)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// SnipFromHead returns a ResizeHandler that removes the oldest N messages from
// the head of the window. This is cache-friendly: the prefix is preserved,
// only the tail shortens.
func SnipFromHead(n int) ResizeHandler {
	return func(fullContext, contextWindow []ChatMessage) ([]ChatMessage, error) {
		if n >= len(contextWindow) {
			return nil, fmt.Errorf("cannot snip %d from %d messages", n, len(contextWindow))
		}
		return contextWindow[n:], nil
	}
}

// PruneLowValue returns a ResizeHandler that removes messages where the
// content is shorter than minLen bytes (heuristic for low-information).
// This causes partial cache invalidation.
func PruneLowValue(minLen int) ResizeHandler {
	return func(fullContext, contextWindow []ChatMessage) ([]ChatMessage, error) {
		result := make([]ChatMessage, 0, len(contextWindow))
		for _, m := range contextWindow {
			if len(ContentToString(m.Content)) >= minLen {
				result = append(result, m)
			}
		}
		return result, nil
	}
}

// SummaryReplace returns a ResizeHandler that replaces the entire window with
// a single summary message. This causes full cache invalidation.
// The summaryMsg should be pre-computed by an LLM call (not done here).
func SummaryReplace(summaryMsg ChatMessage) ResizeHandler {
	return func(fullContext, contextWindow []ChatMessage) ([]ChatMessage, error) {
		return []ChatMessage{summaryMsg}, nil
	}
}

// stableMarshal serializes v to JSON with deterministic byte ordering:
// object keys sorted alphabetically (recursively), map keys sorted alphabetically.
// Kept local to the session package to avoid a cross-module dependency on
// github.com/inferglow/model.
func stableMarshal(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var decoded any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := encodeStableLocal(&buf, decoded); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// encodeStableLocal writes v to buf with stable ordering. Mirrors the
// implementation in model/cache_stable.go.
func encodeStableLocal(buf *bytes.Buffer, v any) error {
	switch val := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case json.Number:
		buf.WriteString(string(val))
	case string:
		b, err := json.Marshal(val)
		if err != nil {
			return err
		}
		buf.Write(b)
	case []any:
		buf.WriteByte('[')
		for i, item := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeStableLocal(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			buf.Write(kb)
			buf.WriteByte(':')
			if err := encodeStableLocal(buf, val[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}
