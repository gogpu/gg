package text

import "sync"

// RuneToBoolMap is a memory-efficient map from rune to bool.
// Uses 2 bits per rune: (checked, hasGlyph).
// Optimized for sparse access patterns in Unicode space.
//
// Each block covers 256 runes (512 bits = 64 bytes).
// Blocks are allocated on-demand only when a rune in that range is accessed.
//
// RuneToBoolMap is safe for concurrent use.
// RuneToBoolMap must not be copied after creation (has mutex).
type RuneToBoolMap struct {
	mu     sync.RWMutex
	blocks map[uint32]*block // Keyed by rune >> 8 (256-rune blocks)
}

// block holds 256 runes (512 bits = 64 bytes).
// Each rune uses 2 bits: bit 0 = checked, bit 1 = hasGlyph.
type block struct {
	bits [8]uint64 // 512 bits for 256 runes × 2 bits
}

// NewRuneToBoolMap creates a new rune-to-bool map.
func NewRuneToBoolMap() *RuneToBoolMap {
	return &RuneToBoolMap{
		blocks: make(map[uint32]*block),
	}
}

// Get returns (hasGlyph, checked).
// If checked is false, the rune hasn't been queried yet.
func (m *RuneToBoolMap) Get(r rune) (hasGlyph, checked bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	blockIdx := uint32(r) >> 8
	runeIdx := uint32(r) & 0xFF

	b, ok := m.blocks[blockIdx]
	if !ok {
		return false, false
	}

	// Calculate bit position
	bitIdx := runeIdx * 2
	wordIdx := bitIdx / 64
	bitPos := bitIdx % 64

	word := b.bits[wordIdx]
	checkedBit := (word >> bitPos) & 1
	hasGlyphBit := (word >> (bitPos + 1)) & 1

	return hasGlyphBit != 0, checkedBit != 0
}

// Set stores the hasGlyph value for a rune.
// Marks the rune as checked.
func (m *RuneToBoolMap) Set(r rune, hasGlyph bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	blockIdx := uint32(r) >> 8
	runeIdx := uint32(r) & 0xFF

	// Get or create block
	b, ok := m.blocks[blockIdx]
	if !ok {
		b = &block{}
		m.blocks[blockIdx] = b
	}

	// Calculate bit position
	bitIdx := runeIdx * 2
	wordIdx := bitIdx / 64
	bitPos := bitIdx % 64

	// Set checked bit (bit 0)
	b.bits[wordIdx] |= 1 << bitPos

	// Set hasGlyph bit (bit 1)
	if hasGlyph {
		b.bits[wordIdx] |= 1 << (bitPos + 1)
	} else {
		b.bits[wordIdx] &^= 1 << (bitPos + 1)
	}
}

// Clear removes all entries from the map.
func (m *RuneToBoolMap) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.blocks = make(map[uint32]*block)
}
