package text

import (
	"fmt"
	"hash/fnv"
)

// ComputeFontID generates a stable hash identifier for a font source.
// Uses the full font name (includes subfamily like "Regular"/"Bold") and
// number of glyphs as a lightweight fingerprint. The full name prevents
// glyph-cache collisions between faces in the same family (e.g., "Go Regular"
// vs "Go Bold") that share the same family name and glyph count.
func ComputeFontID(source *FontSource) uint64 {
	if source == nil {
		return 0
	}
	h := fnv.New64a()
	parsed := source.Parsed()
	fullName := parsed.FullName()
	if fullName == "" {
		fullName = source.Name()
	}
	_, _ = fmt.Fprintf(h, "%s:%d", fullName, parsed.NumGlyphs())
	return h.Sum64()
}
