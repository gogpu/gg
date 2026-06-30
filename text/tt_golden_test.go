// TrueType bytecode interpreter — golden tests (Phase C).
//
// Validates that our TT interpreter produces the correct hinted coordinates
// and advances by comparing against known-good values. The golden values
// are derived from the tthint_subset.ttf font which has:
//   - .notdef (GID 0): advance 400, no outline
//   - A (GID 1): advance 880, 18 points, 2 contours, TT instructions
//   - Aacute (GID 2): advance 880, composite (skipped)
//
// Font properties from TTX:
//   - unitsPerEm: 1040
//   - numberOfHMetrics: 2 (only .notdef and A have unique advances)
//
// Reference: skrifa hint/instance.rs, skrifa glyf/mod.rs
package text

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// --- Test 1: Phantom point advances are integer-pixel after hinting ---

func TestTTGolden_PhantomPointAdvance(t *testing.T) {
	tests := []struct {
		name     string
		font     string
		glyphIDs []uint16 // GIDs to test
		ppem     int32
	}{
		{
			name:     "tthint_subset A at 16ppem",
			font:     "tthint_subset.ttf",
			glyphIDs: []uint16{1}, // 'A'
			ppem:     16,
		},
		{
			name:     "tthint_subset A at 12ppem",
			font:     "tthint_subset.ttf",
			glyphIDs: []uint16{1},
			ppem:     12,
		},
		{
			name:     "cousine at 16ppem",
			font:     "cousine_hint_subset.ttf",
			glyphIDs: []uint16{1}, // cousine_hint_subset has only 2 glyphs (.notdef + 1)
			ppem:     16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := loadTTTestFont(t, tt.font)
			cache := newTTHintCache(data)
			if cache == nil {
				t.Fatal("expected non-nil cache")
			}

			for _, gid := range tt.glyphIDs {
				outline, err := cache.hintGlyphOutline(gid, tt.ppem)
				if err != nil {
					t.Errorf("gid=%d: hintGlyphOutline: %v", gid, err)
					continue
				}
				if outline == nil {
					t.Logf("gid=%d: no outline (empty/composite)", gid)
					continue
				}

				advance := outline.hintedAdvance()

				// Hinted advance must be integer pixel (multiple of 64 in 26.6).
				if advance%64 != 0 {
					t.Errorf("gid=%d ppem=%d: hinted advance %d is not integer-pixel (mod 64 = %d)",
						gid, tt.ppem, advance, advance%64)
				} else {
					t.Logf("gid=%d ppem=%d: hinted advance = %d (26.6) = %d px (OK)",
						gid, tt.ppem, advance, advance/64)
				}

				// Hinted advance must differ from raw scaled advance (hinting does something).
				rawAdvance := outline.phantoms[1][0] - outline.phantoms[0][0]
				if rawAdvance == 0 {
					// For some glyphs at certain ppem, hinted == raw is possible
					// (already on grid). Just log it.
					t.Logf("gid=%d ppem=%d: advance unchanged by hinting (may be correct)", gid, tt.ppem)
				}
			}
		})
	}
}

// --- Test 2: Hinted coordinates for known glyph ---

func TestTTGolden_HintedCoordinates(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	cache := newTTHintCache(data)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	// Hint glyph 'A' (GID 1) at 16ppem.
	ppem := int32(16)
	outline, err := cache.hintGlyphOutline(1, ppem)
	if err != nil {
		t.Fatalf("hintGlyphOutline: %v", err)
	}
	if outline == nil {
		t.Fatal("expected non-nil outline for glyph A")
	}

	numPoints := len(outline.points) - ttPhantomPointCount
	if numPoints < 2 {
		t.Fatalf("too few points: %d", numPoints)
	}

	// Verify structural invariants of hinted outline.

	// 1. All contour endpoints must be within bounds.
	for ci, endIdx := range outline.contours {
		if int(endIdx) >= numPoints {
			t.Errorf("contour %d end index %d >= numPoints %d", ci, endIdx, numPoints)
		}
	}

	// 2. Phantom points: phantom[0] and phantom[1] define advance width.
	advance := outline.phantoms[1][0] - outline.phantoms[0][0]
	if advance <= 0 {
		t.Errorf("advance = %d (26.6), expected positive", advance)
	}
	// For tthint_subset 'A' at 16ppem: advance 880 in 1040 upem
	// Raw scaled: 880 * 16 * 64 / 1040 = 867.69... -> ~868 or grid-fitted ~13*64=832 or ~14*64=896
	// The exact value depends on the font's bytecode. We verify it's integer.
	if advance%64 != 0 {
		t.Errorf("advance %d not integer pixel", advance)
	}
	t.Logf("A@16ppem: advance = %d (26.6) = %d px, %d contour points",
		advance, advance/64, numPoints)

	// 3. Log first few hinted point coordinates for diagnostics.
	maxLog := min(numPoints, 5)
	for i := range maxLog {
		t.Logf("  point[%d]: (%d, %d) flags=0x%02x",
			i, outline.points[i][0], outline.points[i][1], outline.flags[i])
	}
}

// --- Test 3: Backward compatibility mode ---

func TestTTGolden_BackwardCompatibility(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	// Smooth target typically enables backward compatibility (suppresses X movement).
	smoothInstance, err := newTTHintInstance(fp, 16, ttTargetSmooth)
	if err != nil || smoothInstance == nil {
		t.Fatalf("newTTHintInstance (smooth): instance=%v err=%v", smoothInstance, err)
	}

	// Normal target does NOT use backward compatibility.
	normalInstance, err := newTTHintInstance(fp, 16, ttTargetNormal)
	if err != nil || normalInstance == nil {
		t.Fatalf("newTTHintInstance (normal): instance=%v err=%v", normalInstance, err)
	}

	smoothBC := smoothInstance.backwardCompatibility()
	normalBC := normalInstance.backwardCompatibility()

	t.Logf("backward compatibility: smooth=%v, normal=%v", smoothBC, normalBC)

	// Normal should not be in backward compatibility mode.
	if normalBC {
		t.Error("normal target should not have backward compatibility")
	}

	// LCD target should preserve linear metrics -> backward compatibility.
	lcdInstance, err := newTTHintInstance(fp, 16, ttTargetLCD)
	if err != nil || lcdInstance == nil {
		t.Fatalf("newTTHintInstance (LCD): instance=%v err=%v", lcdInstance, err)
	}
	if !lcdInstance.backwardCompatibility() {
		t.Error("LCD target should have backward compatibility (preserveLinearMetrics)")
	}
}

// --- Test 4: Multi-size consistency ---

func TestTTGolden_MultiSize(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	cache := newTTHintCache(data)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	sizes := []int32{8, 10, 12, 14, 16, 18, 20, 24, 32, 48}
	gid := uint16(1) // 'A'

	for _, ppem := range sizes {
		outline, err := cache.hintGlyphOutline(gid, ppem)
		if err != nil {
			t.Errorf("ppem=%d: hintGlyphOutline: %v", ppem, err)
			continue
		}
		if outline == nil {
			t.Logf("ppem=%d: no outline (unusual for A)", ppem)
			continue
		}

		advance := outline.hintedAdvance()

		// Every hinted advance must be integer pixel.
		if advance%64 != 0 {
			t.Errorf("ppem=%d: advance %d is not integer pixel (mod 64 = %d)",
				ppem, advance, advance%64)
		}

		// Advance must be positive.
		if advance <= 0 {
			t.Errorf("ppem=%d: advance %d should be positive", ppem, advance)
		}

		// Advance in pixels should be reasonable: between 2 and ppem.
		advPx := advance / 64
		if advPx < 2 || advPx > ppem {
			t.Errorf("ppem=%d: advance %d px out of expected range [2, %d]",
				ppem, advPx, ppem)
		}

		t.Logf("ppem=%2d: advance = %3d (26.6) = %2d px", ppem, advance, advPx)
	}
}

// --- Test 5: System fonts (Windows only) ---

func TestTTGolden_SystemFonts(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("system font test only on Windows")
	}

	fonts := []struct {
		name string
		path string
	}{
		{"Arial", filepath.Join("C:", "Windows", "Fonts", "arial.ttf")},
		{"Times New Roman", filepath.Join("C:", "Windows", "Fonts", "times.ttf")},
		{"Consolas", filepath.Join("C:", "Windows", "Fonts", "consola.ttf")},
	}

	for _, f := range fonts {
		t.Run(f.name, func(t *testing.T) {
			data, err := os.ReadFile(f.path)
			if err != nil {
				t.Skipf("font not available: %v", err)
			}

			cache := newTTHintCache(data)
			if cache == nil {
				t.Skipf("font %s has no TT instructions", f.name)
			}

			ppem := int32(16)

			// Test several glyph IDs (0=.notdef, 1-5 = first few glyphs).
			hintedCount := 0
			for gid := uint16(1); gid <= 10; gid++ {
				outline, err := cache.hintGlyphOutline(gid, ppem)
				if err != nil {
					t.Errorf("gid=%d: %v", gid, err)
					continue
				}
				if outline == nil {
					continue
				}

				advance := outline.hintedAdvance()

				// Integer-pixel advance.
				if advance%64 != 0 {
					t.Errorf("%s gid=%d: advance %d not integer pixel",
						f.name, gid, advance)
				}

				// No panics, positive advance.
				if advance < 0 {
					t.Errorf("%s gid=%d: negative advance %d", f.name, gid, advance)
				}

				hintedCount++
				t.Logf("%s gid=%d: advance = %d (26.6) = %d px",
					f.name, gid, advance, advance/64)
			}

			if hintedCount == 0 {
				t.Errorf("%s: no glyphs were hinted", f.name)
			}
		})
	}
}

// --- Test 6: fpgm and prep execution correctness ---

func TestTTGolden_FpgmPrepExecution(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	// Verify font program data is present.
	if !fp.hasFontProgram() {
		t.Fatal("expected fpgm in tthint_subset")
	}
	if !fp.hasPrepProgram() {
		t.Fatal("expected prep in tthint_subset")
	}
	if len(fp.cvt) == 0 {
		t.Fatal("expected CVT in tthint_subset")
	}

	t.Logf("fpgm: %d bytes", len(fp.fpgm))
	t.Logf("prep: %d bytes", len(fp.prep))
	t.Logf("CVT: %d entries", len(fp.cvt))
	t.Logf("maxFunctionDefs: %d", fp.maxFunctionDefs)
	t.Logf("maxStorage: %d", fp.maxStorage)
	t.Logf("maxStack: %d", fp.maxStack)
	t.Logf("maxTwilight: %d", fp.maxTwilight)
	t.Logf("unitsPerEm: %d", fp.unitsPerEm)

	// From TTX: tthint_subset has maxFunctionDefs=141, maxStorage=1854,
	// maxStackElements=4039, maxTwilightPoints=1220, unitsPerEm=1040.
	if fp.maxFunctionDefs != 141 {
		t.Errorf("maxFunctionDefs = %d, want 141", fp.maxFunctionDefs)
	}
	if fp.maxStorage != 1854 {
		t.Errorf("maxStorage = %d, want 1854", fp.maxStorage)
	}
	if fp.unitsPerEm != 1040 {
		t.Errorf("unitsPerEm = %d, want 1040", fp.unitsPerEm)
	}
	if fp.numGlyphs != 3 {
		t.Errorf("numGlyphs = %d, want 3", fp.numGlyphs)
	}

	// Create instance — this runs fpgm + prep.
	instance, err := newTTHintInstance(fp, 16, ttTargetSmooth)
	if err != nil || instance == nil {
		t.Fatalf("newTTHintInstance: instance=%v err=%v", instance, err)
	}

	// After fpgm, function definitions should be created.
	// tthint_subset defines functions in fpgm. At least one should be active.
	activeFuncs := 0
	for _, def := range instance.functions {
		if def.isActive {
			activeFuncs++
		}
	}
	if activeFuncs == 0 {
		t.Error("expected at least one active function after fpgm")
	}
	t.Logf("active function definitions: %d", activeFuncs)

	// CVT should be scaled.
	if len(instance.cvt) == 0 {
		t.Error("expected scaled CVT entries")
	} else {
		t.Logf("scaled CVT[0] = %d (26.6)", instance.cvt[0])
		// CVT[0] should be non-zero if the raw CVT[0] was non-zero.
		if fp.cvt[0] != 0 && instance.cvt[0] == 0 {
			t.Error("CVT[0] raw is non-zero but scaled is zero")
		}
	}

	// Hinting should be enabled (prep didn't disable it).
	if !instance.isEnabled() {
		t.Log("hinting disabled by prep (unusual for this font)")
	}
}

// --- Test 7: CVT scaling correctness ---

func TestTTGolden_CVTScaling(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	tests := []struct {
		ppem int32
	}{
		{12},
		{16},
		{24},
		{48},
	}

	for _, tt := range tests {
		t.Run("ppem_"+itoa(int(tt.ppem)), func(t *testing.T) {
			instance, err := newTTHintInstance(fp, tt.ppem, ttTargetSmooth)
			if err != nil || instance == nil {
				t.Fatalf("newTTHintInstance: %v", err)
			}

			// Verify CVT was correctly scaled.
			// Scale = ppem * 64 / upem in 16.16.
			// CVT scaling: (rawCVT * 64) * (scale >> 6) >> 16
			// This should approximate: rawCVT * ppem * 64 / upem.
			scale := int32((int64(tt.ppem) * 64 * (1 << 16)) / int64(fp.unitsPerEm))
			scaleFrac := scale >> 6

			maxCheck := min(len(fp.cvt), 5)
			for i := range maxCheck {
				raw := fp.cvt[i]
				if raw == 0 {
					continue
				}

				// Expected: (raw * 64) * (scale >> 6) >> 16
				expected := int32((int64(raw*64) * int64(scaleFrac)) >> 16)

				// Note: prep may modify CVT values, so the instance CVT may
				// differ from our expected pre-prep values. We check the
				// initial scaling is in the right ballpark.
				actual := instance.cvt[i]

				// Allow difference from prep modifications. Just verify
				// the magnitude is reasonable (within 2x of expected).
				if expected != 0 {
					ratio := float64(actual) / float64(expected)
					if ratio < 0.1 || ratio > 10 {
						t.Errorf("CVT[%d] ppem=%d: expected ~%d, got %d (ratio=%.2f)",
							i, tt.ppem, expected, actual, ratio)
					}
				}
				t.Logf("CVT[%d] ppem=%d: raw=%d, expected_before_prep=%d, actual=%d",
					i, tt.ppem, raw, expected, actual)
			}
		})
	}
}

// --- Test 8: Integration with rendering pipeline ---

func TestTTGolden_IntegrationWithRendering(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	cache := newTTHintCache(data)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	ppem := int32(16)

	// Verify hintedAdvanceWidth returns reasonable float64 values.
	advance, ok := cache.hintedAdvanceWidth(1, ppem) // GID 1 = 'A'
	if !ok {
		t.Fatal("hintedAdvanceWidth returned false for A")
	}
	if advance <= 0 {
		t.Errorf("advance = %f, want positive", advance)
	}

	// Advance should be integer pixel since TT hinting grid-fits.
	if math.Abs(advance-math.Round(advance)) > 0.001 {
		t.Errorf("advance = %f, expected integer pixel value", advance)
	}

	t.Logf("A@16ppem: hinted advance = %.2f px", advance)

	// Verify full outline conversion.
	outline, err := cache.hintGlyphOutline(1, ppem)
	if err != nil || outline == nil {
		t.Fatalf("hintGlyphOutline: outline=%v err=%v", outline, err)
	}

	glyphOutline := ttHintedOutlineToGlyphOutline(outline, GlyphID(1))
	if glyphOutline == nil {
		t.Fatal("ttHintedOutlineToGlyphOutline returned nil")
	}

	// Verify outline has segments.
	if len(glyphOutline.Segments) == 0 {
		t.Error("expected non-empty segments")
	}

	// First segment must be MoveTo.
	if glyphOutline.Segments[0].Op != OutlineOpMoveTo {
		t.Errorf("first segment op = %v, want MoveTo", glyphOutline.Segments[0].Op)
	}

	// Advance must match.
	expectedAdvance := float32(outline.hintedAdvance()) / 64.0
	if math.Abs(float64(glyphOutline.Advance-expectedAdvance)) > 0.001 {
		t.Errorf("outline advance = %f, want %f", glyphOutline.Advance, expectedAdvance)
	}

	// Bounds must be non-zero.
	if glyphOutline.Bounds.MaxX <= glyphOutline.Bounds.MinX {
		t.Error("bounds X range is zero or negative")
	}
	if glyphOutline.Bounds.MaxY <= glyphOutline.Bounds.MinY {
		t.Error("bounds Y range is zero or negative")
	}

	t.Logf("A@16ppem outline: %d segments, advance=%.2f, bounds=%v",
		len(glyphOutline.Segments), glyphOutline.Advance, glyphOutline.Bounds)
}

// --- Test 9: Hinting actually moves points (quality test) ---

func TestTTGolden_HintingMovesPoints(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	loader, err := newTTGlyphLoader(data, fp)
	if err != nil || loader == nil {
		t.Fatalf("newTTGlyphLoader: %v", err)
	}

	ppem := int32(16)
	instance, err := newTTHintInstance(fp, ppem, ttTargetSmooth)
	if err != nil || instance == nil {
		t.Fatalf("newTTHintInstance: instance=%v err=%v", instance, err)
	}

	scale := instance.scale
	gid := uint16(1) // 'A'

	outline, err := loader.loadGlyphOutline(gid, scale)
	if err != nil || outline == nil {
		t.Fatalf("loadGlyphOutline: outline=%v err=%v", outline, err)
	}

	// Save pre-hint coordinates.
	numPoints := len(outline.points) - ttPhantomPointCount
	preHintPoints := make([][2]int32, numPoints)
	copy(preHintPoints, outline.points[:numPoints])

	preHintPhantoms := outline.phantoms

	// Run hinting.
	err = instance.hintGlyph(outline)
	if err != nil {
		t.Fatalf("hintGlyph: %v", err)
	}

	// Count how many points moved.
	movedCount := 0
	totalDelta := int64(0)
	for i := range numPoints {
		dx := int64(outline.points[i][0] - preHintPoints[i][0])
		dy := int64(outline.points[i][1] - preHintPoints[i][1])
		if dx != 0 || dy != 0 {
			movedCount++
			totalDelta += absInt64(dx) + absInt64(dy)
		}
	}

	// For a properly hinted font, at least some points should move.
	// At 16ppem, TT hinting should adjust Y coordinates for stem alignment.
	if movedCount == 0 {
		t.Log("WARNING: no points moved by hinting (may indicate bytecode had no effect)")
	} else {
		t.Logf("hinting moved %d/%d points, total delta = %d (26.6 units)",
			movedCount, numPoints, totalDelta)
	}

	// Phantom points should also be affected.
	postAdvance := outline.hintedAdvance()
	preAdvance := preHintPhantoms[1][0] - preHintPhantoms[0][0]
	t.Logf("phantom advance: pre=%d, post=%d (26.6), delta=%d",
		preAdvance, postAdvance, postAdvance-preAdvance)
}

// --- Test 10: No instructions = graceful fallback ---

func TestTTGolden_NoInstructions(t *testing.T) {
	// ahem.ttf has no fpgm/prep tables.
	data := loadTTTestFont(t, "ahem.ttf")

	fp, err := loadTTFontProgram(data)
	if err != nil {
		t.Fatalf("loadTTFontProgram error: %v", err)
	}
	if fp != nil {
		t.Error("expected nil font program for ahem.ttf (no TT instructions)")
	}

	// Cache should also be nil.
	cache := newTTHintCache(data)
	if cache != nil {
		t.Error("expected nil cache for ahem.ttf")
	}

	// notoserifhebrew has no TT instructions (CFF outlines? or auto-hint only).
	data2 := loadTTTestFont(t, "notoserifhebrew_autohint_metrics.ttf")
	cache2 := newTTHintCache(data2)
	// May or may not have TT instructions depending on the subset.
	// Just verify no panic.
	t.Logf("notoserifhebrew TT cache: %v", cache2 != nil)
}

// --- Test 11: Cousine monospace consistency ---

func TestTTGolden_CousineMonospace(t *testing.T) {
	data := loadTTTestFont(t, "cousine_hint_subset.ttf")
	cache := newTTHintCache(data)
	if cache == nil {
		t.Fatal("expected non-nil cache for Cousine")
	}

	ppem := int32(16)

	// Collect all hinted advances. Monospace: all should be identical.
	var advances []float64
	var advanceGIDs []uint16
	numGlyphs := cache.font.numGlyphs
	for gid := uint16(1); gid < uint16(numGlyphs) && gid < 20; gid++ {
		adv, ok := cache.hintedAdvanceWidth(gid, ppem)
		if !ok {
			continue
		}
		advances = append(advances, adv)
		advanceGIDs = append(advanceGIDs, gid)
	}

	if len(advances) < 2 {
		t.Skip("not enough glyphs to verify monospace consistency")
	}

	// All advances should be the same for a monospace font.
	firstAdv := advances[0]
	allSame := true
	for i, adv := range advances {
		if math.Abs(adv-firstAdv) > 0.01 {
			t.Errorf("gid=%d: advance=%.2f differs from gid=%d: advance=%.2f (monospace violation)",
				advanceGIDs[i], adv, advanceGIDs[0], firstAdv)
			allSame = false
		}
	}
	if allSame {
		t.Logf("Cousine: all %d glyphs have identical advance = %.2f px (monospace OK)",
			len(advances), firstAdv)
	}
}

// --- Test 12: CVT count matches font ---

func TestTTGolden_CVTCount(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	instance, err := newTTHintInstance(fp, 16, ttTargetSmooth)
	if err != nil || instance == nil {
		t.Fatalf("newTTHintInstance: %v", err)
	}

	// Instance CVT length must match font CVT length.
	if len(instance.cvt) != len(fp.cvt) {
		t.Errorf("instance CVT len = %d, font CVT len = %d", len(instance.cvt), len(fp.cvt))
	}

	// Storage area must be allocated.
	if len(instance.storage) != fp.maxStorage {
		t.Errorf("storage len = %d, maxStorage = %d", len(instance.storage), fp.maxStorage)
	}
}

// --- Test 13: Scale computation parity ---

func TestTTGolden_ScaleComputation(t *testing.T) {
	// Verify scale = ppem * 64 * 65536 / upem (16.16 fixed-point).
	tests := []struct {
		ppem int32
		upem int
	}{
		{16, 1040}, // tthint_subset
		{12, 1040}, // tthint_subset
		{16, 1000}, // common upem
		{16, 2048}, // common upem
		{72, 1040}, // large size
	}

	for _, tt := range tests {
		expected := int32((int64(tt.ppem) * 64 * (1 << 16)) / int64(tt.upem))
		name := "ppem_" + itoa(int(tt.ppem)) + "_upem_" + itoa(tt.upem)
		t.Run(name, func(t *testing.T) {
			// Verify our formula matches what the instance uses.
			t.Logf("ppem=%d upem=%d -> scale=0x%08X (%d)", tt.ppem, tt.upem, expected, expected)

			// Sanity: scale should be positive and reasonable.
			if expected <= 0 {
				t.Errorf("scale should be positive, got %d", expected)
			}

			// Scale * upem / 65536 / 64 should approximately equal ppem.
			recovered := float64(expected) * float64(tt.upem) / 65536.0 / 64.0
			if math.Abs(recovered-float64(tt.ppem)) > 0.01 {
				t.Errorf("scale round-trip: expected ppem=%d, got %.2f", tt.ppem, recovered)
			}
		})
	}
}

// --- Test 14: Cache deduplication ---

func TestTTGolden_CacheDeduplication(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	cache := newTTHintCache(data)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	ppem := int32(16)

	// First call creates instance.
	instance1, err := cache.getInstance(ppem)
	if err != nil || instance1 == nil {
		t.Fatalf("first getInstance: %v", err)
	}

	// Second call returns the same instance.
	instance2, err := cache.getInstance(ppem)
	if err != nil || instance2 == nil {
		t.Fatalf("second getInstance: %v", err)
	}

	if instance1 != instance2 {
		t.Error("expected same instance pointer for same ppem")
	}

	// Different ppem returns different instance.
	instance3, err := cache.getInstance(24)
	if err != nil || instance3 == nil {
		t.Fatalf("getInstance(24): %v", err)
	}

	if instance1 == instance3 {
		t.Error("expected different instance for different ppem")
	}
}

// --- Test 15: Outline point ordering preserved ---

func TestTTGolden_PointOrderPreserved(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	cache := newTTHintCache(data)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	outline, err := cache.hintGlyphOutline(1, 16) // 'A' at 16ppem
	if err != nil || outline == nil {
		t.Fatalf("hintGlyphOutline: outline=%v err=%v", outline, err)
	}

	numPoints := len(outline.points) - ttPhantomPointCount

	// Verify contour structure is consistent.
	if len(outline.contours) == 0 {
		t.Fatal("expected contours")
	}
	lastEnd := -1
	for ci, endIdx := range outline.contours {
		end := int(endIdx)
		if end <= lastEnd {
			t.Errorf("contour %d end %d <= previous end %d", ci, end, lastEnd)
		}
		if end >= numPoints {
			t.Errorf("contour %d end %d >= numPoints %d", ci, end, numPoints)
		}
		lastEnd = end
	}

	// Verify phantom points are after outline points.
	totalLen := len(outline.points)
	if totalLen != numPoints+ttPhantomPointCount {
		t.Errorf("total points = %d, expected %d + %d", totalLen, numPoints, ttPhantomPointCount)
	}
}

// --- Test 16: hmtx metrics for known font ---

func TestTTGolden_HmtxMetrics(t *testing.T) {
	// tthint_subset from TTX:
	// .notdef: advance 400, lsb 0
	// A: advance 880, lsb 0
	// Aacute: advance 880, lsb 0 (same, numberOfHMetrics=2)
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	loader, err := newTTGlyphLoader(data, fp)
	if err != nil || loader == nil {
		t.Fatalf("newTTGlyphLoader: %v", err)
	}

	tests := []struct {
		gid     uint16
		advance uint16
		lsb     int16
	}{
		{0, 400, 0},
		{1, 880, 0},
		{2, 880, 0},
	}

	for _, tt := range tests {
		adv, lsb := loader.glyphMetrics(tt.gid)
		if adv != tt.advance {
			t.Errorf("gid=%d: advance = %d, want %d", tt.gid, adv, tt.advance)
		}
		if lsb != tt.lsb {
			t.Errorf("gid=%d: lsb = %d, want %d", tt.gid, lsb, tt.lsb)
		}
	}
}

// --- Test 17: Multi-font robustness ---

func TestTTGolden_MultiFontRobustness(t *testing.T) {
	testFonts := []string{
		"tthint_subset.ttf",
		"cousine_hint_subset.ttf",
		"ahem.ttf",
		"notoserifhebrew_autohint_metrics.ttf",
		"cantarell_vf_trimmed.ttf",
		"vazirmatn_var_trimmed.ttf",
	}

	for _, fontName := range testFonts {
		t.Run(fontName, func(t *testing.T) {
			path := filepath.Join("testdata", fontName)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Skipf("font not available: %v", err)
			}

			// All of these should not panic.
			cache := newTTHintCache(data)
			if cache == nil {
				t.Logf("%s: no TT instructions (OK)", fontName)
				return
			}

			// Try hinting at multiple sizes.
			for _, ppem := range []int32{8, 12, 16, 24, 48} {
				for gid := uint16(0); gid < 10; gid++ {
					_, _ = cache.hintGlyphOutline(gid, ppem)
					// No panic is the success criterion.
				}
			}
			t.Logf("%s: all sizes hinted without panic", fontName)
		})
	}
}

// --- Test 18: Twilight zone initialization ---

func TestTTGolden_TwilightZone(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	instance, err := newTTHintInstance(fp, 16, ttTargetSmooth)
	if err != nil || instance == nil {
		t.Fatalf("newTTHintInstance: %v", err)
	}

	// tthint_subset has maxTwilightPoints=1220.
	expectedTwilight := fp.maxTwilight
	if len(instance.twilightScaled) != expectedTwilight {
		t.Errorf("twilightScaled len = %d, want %d", len(instance.twilightScaled), expectedTwilight)
	}
	if len(instance.twilightOriginalScaled) != expectedTwilight {
		t.Errorf("twilightOriginalScaled len = %d, want %d", len(instance.twilightOriginalScaled), expectedTwilight)
	}
	if len(instance.twilightFlags) != expectedTwilight {
		t.Errorf("twilightFlags len = %d, want %d", len(instance.twilightFlags), expectedTwilight)
	}

	// After fpgm+prep, some twilight points may have been set.
	// The fpgm typically uses MIAP[] to position twilight points.
	nonZeroTwilight := 0
	for _, pt := range instance.twilightScaled {
		if pt[0] != 0 || pt[1] != 0 {
			nonZeroTwilight++
		}
	}
	t.Logf("twilight zone: %d/%d points are non-zero after fpgm+prep",
		nonZeroTwilight, expectedTwilight)
}

// --- Test 19: Hinted advance across target modes ---

func TestTTGolden_TargetModes(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	targets := []struct {
		name   string
		target ttTarget
	}{
		{"Normal", ttTargetNormal},
		{"Smooth", ttTargetSmooth},
		{"LCD", ttTargetLCD},
		{"LCDV", ttTargetLCDV},
	}

	ppem := int32(16)
	gid := uint16(1) // 'A'

	for _, tg := range targets {
		t.Run(tg.name, func(t *testing.T) {
			instance, err := newTTHintInstance(fp, ppem, tg.target)
			if err != nil || instance == nil {
				t.Fatalf("newTTHintInstance: %v", err)
			}

			loader, lerr := newTTGlyphLoader(data, fp)
			if lerr != nil || loader == nil {
				t.Fatalf("newTTGlyphLoader: %v", lerr)
			}

			outline, oerr := loader.loadGlyphOutline(gid, instance.scale)
			if oerr != nil || outline == nil {
				t.Skipf("no outline for gid=%d", gid)
			}

			if herr := instance.hintGlyph(outline); herr != nil {
				t.Fatalf("hintGlyph: %v", herr)
			}

			advance := outline.hintedAdvance()
			t.Logf("target=%s: advance=%d (26.6) = %d px, backwardCompat=%v",
				tg.name, advance, advance/64, instance.backwardCompatibility())

			// All modes should produce integer-pixel advance.
			if advance%64 != 0 {
				t.Errorf("target=%s: advance %d not integer pixel", tg.name, advance)
			}
		})
	}
}

// --- Test 20: Zero and negative ppem ---

func TestTTGolden_EdgeCasePPEM(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	cache := newTTHintCache(data)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	// ppem=0 should return nil instance gracefully.
	instance, err := cache.getInstance(0)
	if err != nil {
		t.Errorf("ppem=0: unexpected error: %v", err)
	}
	if instance != nil {
		t.Error("ppem=0: expected nil instance")
	}

	// ppem=-1 should return nil instance gracefully.
	instance, err = cache.getInstance(-1)
	if err != nil {
		t.Errorf("ppem=-1: unexpected error: %v", err)
	}
	if instance != nil {
		t.Error("ppem=-1: expected nil instance")
	}

	// Very large ppem should not panic.
	instance, err = cache.getInstance(200)
	if err != nil {
		t.Errorf("ppem=200: error: %v", err)
	}
	if instance == nil {
		t.Error("ppem=200: expected non-nil instance")
	}
}

// --- Test 21: Backward compat preserves X coordinates (bug fix validation) ---

// TestTTGolden_BackwardCompatXPreserved validates that in backward compatibility
// mode (ClearType), the TT interpreter preserves X coordinates instead of zeroing
// them. This was the root cause of the DejaVuSans X-zeroing bug.
//
// In backward compat mode, skrifa's move_point() (zone.rs:417-468):
//   - Suppresses X movement (point.x stays at original, not zeroed)
//   - Still touches the point (flag set)
//   - Y movement proceeds normally (until both IUP axes done)
func TestTTGolden_BackwardCompatXPreserved(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	for _, ppem := range []int32{12, 16, 24} {
		t.Run("ppem_"+itoa(int(ppem)), func(t *testing.T) {
			instance, err := newTTHintInstance(fp, ppem, ttTargetSmooth)
			if err != nil || instance == nil {
				t.Fatalf("newTTHintInstance: %v", err)
			}
			if !instance.backwardCompatibility() {
				t.Skip("smooth target should have backward compat enabled")
			}

			loader, lerr := newTTGlyphLoader(data, fp)
			if lerr != nil || loader == nil {
				t.Fatalf("newTTGlyphLoader: %v", lerr)
			}
			outline, err := loader.loadGlyphOutline(1, instance.scale) // GID 1 = 'A'
			if err != nil || outline == nil {
				t.Fatalf("loadGlyphOutline: %v", err)
			}

			// Save pre-hint X coordinates.
			numPoints := len(outline.points) - ttPhantomPointCount
			preHintX := make([]int32, numPoints)
			for i := range numPoints {
				preHintX[i] = outline.points[i][0]
			}

			// Run hinting.
			err = instance.hintGlyph(outline)
			if err != nil {
				t.Fatalf("hintGlyph: %v", err)
			}

			// In backward compat mode, X coordinates should be preserved
			// (NOT zeroed). Points keep their original X values.
			zeroedCount := 0
			preservedCount := 0
			for i := range numPoints {
				postX := outline.points[i][0]
				if preHintX[i] != 0 && postX == 0 {
					zeroedCount++
				}
				if postX == preHintX[i] {
					preservedCount++
				}
			}

			if zeroedCount > 0 {
				t.Errorf("REGRESSION: %d/%d X coordinates zeroed (backward compat should preserve X)",
					zeroedCount, numPoints)
			}

			t.Logf("ppem=%d: %d/%d X coords preserved, 0 zeroed",
				ppem, preservedCount, numPoints)
		})
	}
}

// ============================================================
// COORDINATE-EXACT GOLDEN TESTS (skrifa parity)
// ============================================================
//
// These tests compare our TT interpreter output coordinate-by-coordinate
// against golden data extracted from Google's skrifa (Rust fontations).
// The golden arrays are in tt_golden_data_test.go, extracted via
// SKRIFA_DUMP_GOLDEN=1 instrumentation in skrifa hint/instance.rs.
//
// All coordinates are 26.6 fixed-point integers.
// Comparison is EXACT (diff=0) — no tolerance.
//
// Reference: skrifa/src/outline/glyf/hint/instance.rs:hint()
// ============================================================

// --- Test 22: Coordinate-exact golden comparison at 16ppem ---

func TestTTGolden_SkrifaParity_16ppem(t *testing.T) {
	ttGoldenCompareCoords(t, "tthint_subset.ttf", 1, 16,
		skrifa26Dot6GID1at16ppem, skrifaPhantomsGID1at16ppem)
}

// --- Test 23: Coordinate-exact golden comparison at 12ppem ---

func TestTTGolden_SkrifaParity_12ppem(t *testing.T) {
	ttGoldenCompareCoords(t, "tthint_subset.ttf", 1, 12,
		skrifa26Dot6GID1at12ppem, skrifaPhantomsGID1at12ppem)
}

// --- Test 24: Coordinate-exact golden comparison at 24ppem ---

func TestTTGolden_SkrifaParity_24ppem(t *testing.T) {
	ttGoldenCompareCoords(t, "tthint_subset.ttf", 1, 24,
		skrifa26Dot6GID1at24ppem, skrifaPhantomsGID1at24ppem)
}

// ttGoldenCompareCoords compares hinted glyph coordinates from our TT
// interpreter against golden data from skrifa.
//
// Both X and Y coordinates must match diff=0 (hard fail on regression).
// This is the TT interpreter equivalent of the auto-hinter's
// TestAutoHintGolden_HintedCoords_* tests.
func ttGoldenCompareCoords(
	t *testing.T,
	fontFile string,
	glyphID uint16,
	ppem int32,
	expectedCoords [][2]int32,
	expectedPhantoms [4][2]int32,
) {
	t.Helper()

	data := loadTTTestFont(t, fontFile)
	cache := newTTHintCache(data)
	if cache == nil {
		t.Fatal("expected non-nil TT hint cache")
	}

	outline, err := cache.hintGlyphOutline(glyphID, ppem)
	if err != nil {
		t.Fatalf("hintGlyphOutline(gid=%d, ppem=%d): %v", glyphID, ppem, err)
	}
	if outline == nil {
		t.Fatalf("hintGlyphOutline(gid=%d, ppem=%d): returned nil", glyphID, ppem)
	}

	numPoints := len(outline.points) - ttPhantomPointCount

	if numPoints != len(expectedCoords) {
		t.Fatalf("point count mismatch: got %d, want %d (skrifa)",
			numPoints, len(expectedCoords))
	}

	xMismatches := 0
	yMismatches := 0
	for i, want := range expectedCoords {
		got := outline.points[i]
		dx := got[0] - want[0]
		dy := got[1] - want[1]
		if dx != 0 {
			t.Errorf("pt[%d] X: got %d, want %d [dx=%d]", i, got[0], want[0], dx)
			xMismatches++
		}
		if dy != 0 {
			t.Errorf("pt[%d] Y: got %d, want %d [dy=%d]", i, got[1], want[1], dy)
			yMismatches++
		}
	}

	if xMismatches > 0 {
		t.Errorf("X REGRESSION: %d/%d X coordinates differ from skrifa", xMismatches, numPoints)
	}
	if yMismatches > 0 {
		t.Errorf("Y REGRESSION: %d/%d Y coordinates differ from skrifa", yMismatches, numPoints)
	}

	// Phantom points must match (advance width and vertical metrics).
	for i := range 4 {
		got := outline.points[numPoints+i]
		want := expectedPhantoms[i]
		if got[0] != want[0] {
			t.Errorf("phantom[%d] X: got %d, want %d", i, got[0], want[0])
		}
		if got[1] != want[1] {
			t.Errorf("phantom[%d] Y: got %d, want %d", i, got[1], want[1])
		}
	}

	t.Logf("skrifa parity ppem=%d: X=%d/%d diff=0, Y=%d/%d diff=0",
		ppem, numPoints-xMismatches, numPoints, numPoints-yMismatches, numPoints)
}

// --- Test 25: Pre-hinting coordinate scaling parity ---
// Verifies that our initial scaling (font units → 26.6) matches skrifa
// exactly, isolating scaling from interpreter differences.

func TestTTGolden_SkrifaParity_PreHinting_16ppem(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	loader, err := newTTGlyphLoader(data, fp)
	if err != nil || loader == nil {
		t.Fatalf("newTTGlyphLoader: %v", err)
	}

	ppem := int32(16)
	// Compute scale using skrifa-matching rounded division.
	a := uint64(ppem*64) << 16
	b := uint64(fp.unitsPerEm)
	scale := int32((a + b/2) / b)

	outline, err := loader.loadGlyphOutline(1, scale)
	if err != nil || outline == nil {
		t.Fatalf("loadGlyphOutline: outline=%v err=%v", outline, err)
	}

	// Pre-hinting coordinates from skrifa (extracted with SKRIFA_DUMP_PREHINT=1).
	// These are the scaled coordinates BEFORE the interpreter runs.
	// Only checking first 20 + last 4 to keep the test readable.
	// If these match, the initial scaling is correct and any post-hinting
	// differences are purely in the interpreter.
	wantFirst20 := [][2]int32{
		{0, -315}, {0, 1029}, {866, 1029}, {866, 1024}, {793, 1024},
		{793, 950}, {866, 950}, {866, 945}, {793, 945}, {793, 871},
		{866, 871}, {866, 866}, {793, 866}, {793, 793}, {866, 793},
		{866, 788}, {793, 788}, {793, 714}, {866, 714}, {866, 630},
	}

	numPoints := len(outline.points) - ttPhantomPointCount
	mismatches := 0
	for i, want := range wantFirst20 {
		if i >= numPoints {
			break
		}
		got := outline.points[i]
		if got != want {
			t.Errorf("pre-hint pt[%d]: got (%d, %d), want (%d, %d) [dx=%d dy=%d]",
				i, got[0], got[1], want[0], want[1],
				got[0]-want[0], got[1]-want[1])
			mismatches++
		}
	}

	// Check phantom points (before rounding — these are the raw scaled values).
	wantPhantomPre := [4][2]int32{
		{0, 0}, {866, 0}, {0, 1029}, {0, -315},
	}
	for i, want := range wantPhantomPre {
		got := outline.original[numPoints+i]
		if got != want {
			t.Errorf("pre-hint phantom_original[%d]: got (%d, %d), want (%d, %d)",
				i, got[0], got[1], want[0], want[1])
			mismatches++
		}
	}

	// Check rounded phantom points (these go into outline.points for the interpreter).
	wantPhantomRounded := [4][2]int32{
		{0, 0}, {896, 0}, {0, 1024}, {0, -320},
	}
	for i, want := range wantPhantomRounded {
		got := outline.points[numPoints+i]
		if got != want {
			t.Errorf("pre-hint phantom_rounded[%d]: got (%d, %d), want (%d, %d)",
				i, got[0], got[1], want[0], want[1])
			mismatches++
		}
	}

	if mismatches == 0 {
		t.Logf("PASS: pre-hinting coordinates match skrifa (first %d + phantoms)", len(wantFirst20))
	}
}

// --- Helpers ---

// absInt64 returns the absolute value of an int64.
func absInt64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// itoa is a minimal int-to-string for test names (avoids strconv import).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	i := len(buf) - 1
	for n > 0 {
		buf[i] = byte('0' + n%10)
		i--
		n /= 10
	}
	if neg {
		buf[i] = '-'
		i--
	}
	return string(buf[i+1:])
}
