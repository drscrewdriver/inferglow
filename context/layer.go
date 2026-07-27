package contextmgr

import "strings"

// LayerID identifies one of the 9 assembly layers (A-1).
type LayerID int

const (
	LayerSystemSafety   LayerID = 1 // permanent: safety policy preamble
	LayerIdentity       LayerID = 2 // permanent: agent identity
	LayerProtocol       LayerID = 3 // permanent: protocol instructions
	LayerTaskBackground LayerID = 4 // task-level: head buffer / task description
	LayerProhibitions   LayerID = 5 // semi-stable: constitutional / prohibitions
	LayerLTMFacts       LayerID = 6 // volatile: hot facts + long-term memory
	LayerCompressedHist LayerID = 7 // volatile: compressed history + tail
	LayerHotIndex       LayerID = 8 // volatile: backtrack / hot index
	LayerHintBlock      LayerID = 9 // volatile: dynamic hint
)

// LayerContent is a single rendered layer with fingerprint and version (A-1).
type LayerContent struct {
	ID      LayerID
	Content string
	Sha256  string
	Version int64
	Stable  bool // true for L1-L5 (setup phase)
}

// LayerCacheStat holds per-layer cache observability (A-11).
type LayerCacheStat struct {
	Hits   int64
	Misses int64
}

// classifyBlock maps a RenderedBlock to its layer based on pseudo StepID.
// headStepIDs contains StepIDs of head buffer blocks (for L4 detection).
func classifyBlock(b RenderedBlock, headStepIDs map[int]bool) LayerID {
	if headStepIDs[b.StepID] {
		return LayerTaskBackground
	}
	switch b.StepID {
	case -3:
		return LayerProhibitions
	case -2, -4:
		return LayerLTMFacts
	case -5:
		return LayerHotIndex
	case -1:
		return LayerHintBlock
	default:
		if b.StepID > 0 {
			return LayerCompressedHist
		}
		return LayerCompressedHist // fallback
	}
}

// groupIntoLayers folds []RenderedBlock into up to 9 LayerContent entries.
// Empty layers are omitted from the result.
func groupIntoLayers(blocks []RenderedBlock, headStepIDs map[int]bool) []LayerContent {
	// Accumulate content per layer.
	var contents [10]strings.Builder
	var present [10]bool

	for _, b := range blocks {
		lid := classifyBlock(b, headStepIDs)
		if lid < 1 || lid > 9 {
			continue
		}
		contents[lid].WriteString(b.Content)
		contents[lid].WriteString("\n")
		present[lid] = true
	}

	var layers []LayerContent
	for i := 1; i <= 9; i++ {
		if !present[i] {
			continue
		}
		content := strings.TrimRight(contents[i].String(), "\n")
		layers = append(layers, LayerContent{
			ID:      LayerID(i),
			Content: content,
			Sha256:  computeHash(content),
			Stable:  i <= 5,
		})
	}
	return layers
}
