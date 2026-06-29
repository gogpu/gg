package text

import (
	"math"
	"sort"
)

// Standard width computation and stem width quantization.
//
// References:
//   - FreeType aflatin.c:54  af_latin_metrics_init_widths
//   - FreeType aflatin.c:3967 af_latin_compute_stem_width
//   - FreeType afhints.c:121  af_sort_and_quantize_widths
//   - skrifa metrics/widths.rs compute_widths

// maxWidths is the maximum number of standard widths per axis.
// Matches FreeType AF_LATIN_MAX_WIDTHS.
const maxWidths = 16

// computeStandardWidths computes standard stem widths for one axis
// by analyzing a reference glyph. This loads the glyph at design size
// (1:1 scale), finds segments, links them into stems, and extracts
// the stem distances as standard widths.
//
// The script parameter determines which reference characters to use
// (e.g., 'o' for Latin, '\u05DD' for Hebrew).
//
// See FreeType aflatin.c:54 af_latin_metrics_init_widths.
func computeStandardWidths(font ParsedFont, dim hintDimension, script *scriptClass) unscaledAxisMetrics {
	upm := font.UnitsPerEm()
	result := unscaledAxisMetrics{}

	// Find a reference glyph using the script's standard characters.
	var gid uint16
	for _, ch := range script.stdChars {
		g := font.GlyphIndex(ch)
		if g != 0 {
			gid = g
			break
		}
	}

	if gid == 0 {
		// No standard character found — use fallback width.
		result.standardWidth = derivedConstant(upm)
		result.edgeDistThreshold = result.standardWidth / 5
		return result
	}

	// Extract outline at design size (1:1 font units → pixels).
	extractor := NewOutlineExtractor()
	outline, err := extractor.ExtractOutline(font, GlyphID(gid), float64(upm))
	if err != nil || outline == nil || len(outline.Segments) == 0 {
		result.standardWidth = derivedConstant(upm)
		result.edgeDistThreshold = result.standardWidth / 5
		return result
	}

	// Build point array at 1:1 scale.
	points := buildHintPoints(outline)
	if len(points.pts) == 0 {
		result.standardWidth = derivedConstant(upm)
		result.edgeDistThreshold = result.standardWidth / 5
		return result
	}

	// Compute segments and link them.
	segments := computeSegments(&points, dim)
	dummyAxis := scaledAxisMetrics{scale: 1.0}
	linkSegments(segments, &dummyAxis, scriptGroupDefault)

	// Extract stem widths from linked segment pairs.
	var widths []int32
	for i, seg := range segments {
		if seg.linkIdx < 0 {
			continue
		}
		link := &segments[seg.linkIdx]
		// Only consider mutual links (true stems) where link > seg.
		if link.linkIdx == int16(i) && seg.linkIdx > int16(i) {
			dist := seg.pos - link.pos
			if dist < 0 {
				dist = -dist
			}
			if len(widths) < maxWidths {
				widths = append(widths, int32(math.Round(float64(dist))))
			}
		}
	}

	// Sort and quantize (FreeType af_sort_and_quantize_widths).
	sortAndQuantizeWidths(&widths, int32(upm/100))

	if len(widths) == 0 {
		// Fallback.
		result.standardWidth = derivedConstant(upm)
	} else {
		result.widths = widths
		result.standardWidth = widths[0]
	}

	// Edge distance threshold: 20% of standard width.
	result.edgeDistThreshold = result.standardWidth / 5

	return result
}

// sortAndQuantizeWidths sorts widths and merges near-duplicates.
// This replaces multiple almost identical stem widths with a single one.
// The threshold parameter controls the quantization granularity.
//
// See FreeType afhints.c:121 af_sort_and_quantize_widths.
func sortAndQuantizeWidths(widths *[]int32, threshold int32) {
	if len(*widths) <= 1 {
		return
	}

	sort.Slice(*widths, func(i, j int) bool {
		return (*widths)[i] < (*widths)[j]
	})

	// Merge near-duplicates.
	result := []int32{(*widths)[0]}
	for i := 1; i < len(*widths); i++ {
		if (*widths)[i]-result[len(result)-1] > threshold {
			result = append(result, (*widths)[i])
		}
	}
	*widths = result
}

// computeStemWidth snaps a raw scaled stem width to a quantized value
// for consistent rendering. This is THE key algorithm for stem consistency.
// All values in 26.6 fixed-point (1 unit = 1/64 pixel).
//
// For smooth (anti-aliased) rendering:
//   - Snap to standard width if within threshold
//   - Piecewise quantization with "detents" at fractional values
//   - Minimum stem width of 0.75px (48/64) or 0.875px (56/64)
//
// Constants in 26.6:
//   - 1.0px  = 64    - 1.25px = 80    - 1.5px  = 96
//   - 0.5px  = 32    - 0.625px= 40    - 0.75px = 48
//   - 0.875px= 56    - 3.0px  = 192
//   - 10/64px= 10    - 54/64px= 54
//
// See FreeType aflatin.c:3967 af_latin_compute_stem_width.
// See skrifa hint/edges.rs stem_width.
//
//nolint:gocognit,nestif // FreeType aflatin.c port — algorithmic complexity is inherent
func computeStemWidth(axis *scaledAxisMetrics, width int32, edgeFlags, stemFlags uint32) int32 {
	if axis.isExtraLight {
		return width
	}

	dist := width
	sign := false
	if dist < 0 {
		dist = -dist
		sign = true
	}

	// Smooth hinting: lightly quantize the stem width.
	// Leave serif widths alone for vertical direction if < 3px (192 in 26.6).
	if (stemFlags&edgeFlagSerif) != 0 && dist < 192 {
		if sign {
			return -dist
		}
		return dist
	}

	if (edgeFlags & edgeFlagRound) != 0 {
		if dist < 80 { // < 1.25px
			dist = 64 // 1.0px
		}
	} else if dist < 56 { // < 0.875px
		dist = 56 // 0.875px
	}

	if len(axis.widths) > 0 {
		// Compare to standard width.
		delta := dist - axis.widths[0].scaled
		if delta < 0 {
			delta = -delta
		}

		if delta < 40 { // < 0.625px
			dist = axis.widths[0].scaled
			if dist < 48 { // < 0.75px
				dist = 48 // 0.75px
			}
			if sign {
				return -dist
			}
			return dist
		}

		if dist < 192 { // < 3.0px
			// Piecewise quantization for small stems.
			// Uses exact 26.6 fixed-point arithmetic matching FreeType.
			frac := dist - f26dot6Floor(dist)
			base := f26dot6Floor(dist)

			if frac < 10 { //nolint:gocritic // FreeType aflatin.c port — value range if-else chain // 10/64
				dist = base + frac
			} else if frac < 32 { // 32/64 = 0.5px
				dist = base + 10
			} else if frac < 54 { // 54/64 = 0.84375px
				dist = base + 54
			} else {
				dist = base + frac
			}
		} else {
			// Large stems: round to integer.
			dist = f26dot6Round(dist)
		}
	}

	if sign {
		return -dist
	}
	return dist
}

// computeStemWidthCJK computes the snapped width of a stem for CJK.
// CJK uses different quantization thresholds from Default and skips
// the extra-light early return and serif width preservation.
//
// See FreeType afcjk.c:1544 (CJK stem width computation).
// See skrifa hint/edges.rs:825-845 (CJK branch of stem_width).
//
//nolint:nestif // FreeType afcjk.c port — piecewise quantization is inherently nested
func computeStemWidthCJK(axis *scaledAxisMetrics, width int32) int32 {
	// CJK is never extra light — skip that check.

	dist := width
	sign := false
	if dist < 0 {
		dist = -dist
		sign = true
	}

	// CJK: no serif width preservation, no round/straight distinction.

	// Compare to standard width (if available).
	if len(axis.widths) > 0 {
		minWidth := axis.widths[0].scaled
		delta := dist - minWidth
		if delta < 0 {
			delta = -delta
		}
		if delta < 40 { // < 0.625px
			dist = minWidth
			if dist < 48 { // < 0.75px
				dist = 48
			}
			if sign {
				return -dist
			}
			return dist
		}
	}

	// CJK-specific quantization.
	if dist < 54 { // < 54/64 px
		dist += (54 - dist) / 2
	} else if dist < 3*64 { // < 3px
		frac := dist & 63
		base := dist & ^int32(63)
		if frac < 10 { //nolint:gocritic // skrifa port — value range if-else chain
			dist = base + frac
		} else if frac < 22 {
			dist = base + 10
		} else if frac < 42 {
			dist = base + frac
		} else if frac < 54 {
			dist = base + 54
		} else {
			dist = base + frac
		}
	}

	if sign {
		return -dist
	}
	return dist
}
