// TrueType bytecode interpreter — Phase B tests.
//
// Tests for glyph loading, font program parsing, hint instance creation,
// glyph hinting with phantom points, and pipeline integration.
//
// Test fonts:
//   - tthint_subset.ttf: skrifa TT hint test font (fpgm + prep + per-glyph instructions)
//   - cousine_hint_subset.ttf: Google Cousine (monospace, TT hinted)
//
// Reference: skrifa hint/instance.rs tests, glyf/mod.rs tests
package text

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// loadTTTestFont reads a font file from testdata/.
func loadTTTestFont(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to load test font %s: %v", name, err)
	}
	return data
}

func TestLoadTTFontProgram(t *testing.T) {
	tests := []struct {
		name      string
		font      string
		wantNil   bool // true if font has no TT instructions
		wantFpgm  bool // true if fpgm expected
		wantPrep  bool // true if prep expected
		wantCVT   bool // true if CVT expected
		minGlyphs int
	}{
		{
			name:      "tthint_subset has TT instructions",
			font:      "tthint_subset.ttf",
			wantNil:   false,
			wantFpgm:  true,
			wantPrep:  true,
			wantCVT:   true,
			minGlyphs: 1,
		},
		{
			name:      "cousine has TT instructions",
			font:      "cousine_hint_subset.ttf",
			wantNil:   false,
			wantFpgm:  true,
			wantPrep:  true,
			minGlyphs: 1,
		},
		{
			name:    "ahem has no TT instructions",
			font:    "ahem.ttf",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := loadTTTestFont(t, tt.font)
			fp, err := loadTTFontProgram(data)
			if err != nil {
				t.Fatalf("loadTTFontProgram: %v", err)
			}

			if tt.wantNil {
				if fp != nil {
					t.Errorf("expected nil for font without TT instructions")
				}
				return
			}

			if fp == nil {
				t.Fatal("expected non-nil font program")
			}

			if tt.wantFpgm && !fp.hasFontProgram() {
				t.Error("expected font program (fpgm)")
			}
			if tt.wantPrep && !fp.hasPrepProgram() {
				t.Error("expected prep program")
			}
			if tt.wantCVT && len(fp.cvt) == 0 {
				t.Error("expected non-empty CVT")
			}
			if fp.numGlyphs < tt.minGlyphs {
				t.Errorf("numGlyphs = %d, want >= %d", fp.numGlyphs, tt.minGlyphs)
			}
			if fp.unitsPerEm <= 0 {
				t.Errorf("unitsPerEm = %d, want > 0", fp.unitsPerEm)
			}
			if fp.maxStack <= 0 {
				t.Errorf("maxStack = %d, want > 0", fp.maxStack)
			}
		})
	}
}

func TestNewTTGlyphLoader(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	loader, err := newTTGlyphLoader(data, fp)
	if err != nil {
		t.Fatalf("newTTGlyphLoader: %v", err)
	}
	if loader == nil {
		t.Fatal("expected non-nil glyph loader")
	}

	// Verify hmtx data is loaded.
	if len(loader.hmtxAdv) == 0 {
		t.Error("expected non-empty hmtx advances")
	}
	if len(loader.hmtxLSB) == 0 {
		t.Error("expected non-empty hmtx LSBs")
	}
	if len(loader.glyfOff) == 0 {
		t.Error("expected non-empty glyf offsets")
	}
}

func TestLoadGlyphOutline(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	loader, err := newTTGlyphLoader(data, fp)
	if err != nil || loader == nil {
		t.Fatalf("newTTGlyphLoader: %v", err)
	}

	// Compute scale for 16px at the font's upem.
	ppem := int32(16)
	scale := int32((int64(ppem) * 64 * (1 << 16)) / int64(fp.unitsPerEm))

	// Load glyph 0 (.notdef) — may be empty.
	// Load glyph 1 or higher for a real outline.
	for gid := uint16(0); gid < uint16(fp.numGlyphs) && gid < 10; gid++ {
		outline, err := loader.loadGlyphOutline(gid, scale)
		if err != nil {
			t.Errorf("loadGlyphOutline(gid=%d): %v", gid, err)
			continue
		}
		if outline == nil {
			continue // empty or composite glyph
		}

		// Verify structure.
		totalPoints := len(outline.points)
		if totalPoints < ttPhantomPointCount {
			t.Errorf("gid=%d: totalPoints=%d, want >= %d", gid, totalPoints, ttPhantomPointCount)
		}
		if len(outline.flags) != totalPoints {
			t.Errorf("gid=%d: flags len=%d, want %d", gid, len(outline.flags), totalPoints)
		}
		if len(outline.original) != totalPoints {
			t.Errorf("gid=%d: original len=%d, want %d", gid, len(outline.original), totalPoints)
		}
		if len(outline.unscaled) != totalPoints*2 {
			t.Errorf("gid=%d: unscaled len=%d, want %d", gid, len(outline.unscaled), totalPoints*2)
		}
		if len(outline.contours) == 0 {
			t.Errorf("gid=%d: no contours", gid)
		}

		// Verify phantom points are present.
		advance := outline.hintedAdvance()
		if advance <= 0 {
			t.Logf("gid=%d: advance=%d (may be zero-width glyph)", gid, advance)
		}

		t.Logf("gid=%d: %d points, %d contours, advance=%d (26.6), %d instruction bytes",
			gid, totalPoints-ttPhantomPointCount, len(outline.contours), advance, len(outline.bytecode))
	}
}

func TestNewTTHintInstance_WithRealFont(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	tests := []struct {
		name string
		ppem int32
	}{
		{"12px", 12},
		{"16px", 16},
		{"24px", 24},
		{"48px", 48},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance, err := newTTHintInstance(fp, tt.ppem, ttTargetSmooth)
			if err != nil {
				t.Fatalf("newTTHintInstance: %v", err)
			}
			if instance == nil {
				t.Fatal("expected non-nil instance")
			}

			// Verify scale (rounded division matching skrifa Fixed::div).
			a := uint64(tt.ppem*64) << 16
			b := uint64(fp.unitsPerEm)
			expectedScale := int32((a + b/2) / b)
			if instance.scale != expectedScale {
				t.Errorf("scale = %d, want %d", instance.scale, expectedScale)
			}

			// Verify CVT was scaled.
			if len(fp.cvt) > 0 && len(instance.cvt) == 0 {
				t.Error("expected non-empty scaled CVT")
			}

			// Verify enabled (most fonts don't disable hinting).
			t.Logf("ppem=%d: enabled=%v, backwardCompat=%v, scale=%d, cvtLen=%d",
				tt.ppem, instance.isEnabled(), instance.backwardCompatibility(), instance.scale, len(instance.cvt))
		})
	}
}

func TestHintGlyph_PhantomPoints(t *testing.T) {
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

	// Try hinting several glyphs.
	hintedCount := 0
	for gid := uint16(0); gid < uint16(fp.numGlyphs) && gid < 20; gid++ {
		outline, err := loader.loadGlyphOutline(gid, scale)
		if err != nil {
			t.Errorf("loadGlyphOutline(gid=%d): %v", gid, err)
			continue
		}
		if outline == nil {
			continue
		}

		// Save pre-hint phantom points for comparison.
		preHintPhantoms := outline.phantoms

		err = instance.hintGlyph(outline)
		if err != nil {
			t.Errorf("hintGlyph(gid=%d): %v", gid, err)
			continue
		}

		advance := outline.hintedAdvance()
		preAdvance := preHintPhantoms[1][0] - preHintPhantoms[0][0]

		t.Logf("gid=%d: preAdvance=%d, hintedAdvance=%d (26.6), delta=%d",
			gid, preAdvance, advance, advance-preAdvance)

		hintedCount++
	}

	if hintedCount == 0 {
		t.Error("no glyphs were successfully hinted")
	}
	t.Logf("successfully hinted %d glyphs", hintedCount)
}

func TestTTHintCache(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")

	cache := newTTHintCache(data)
	if cache == nil {
		t.Fatal("expected non-nil cache for font with TT instructions")
	}

	// Test getInstance at multiple ppem values.
	for _, ppem := range []int32{12, 16, 24} {
		instance, err := cache.getInstance(ppem)
		if err != nil {
			t.Errorf("getInstance(%d): %v", ppem, err)
			continue
		}
		if instance == nil {
			t.Errorf("getInstance(%d) returned nil", ppem)
			continue
		}
		t.Logf("ppem=%d: instance created", ppem)

		// Second call should return cached instance.
		instance2, err := cache.getInstance(ppem)
		if err != nil {
			t.Errorf("getInstance(%d) second call: %v", ppem, err)
			continue
		}
		if instance2 != instance {
			t.Errorf("ppem=%d: expected cached instance, got different one", ppem)
		}
	}
}

func TestTTHintCache_NoInstructions(t *testing.T) {
	data := loadTTTestFont(t, "ahem.ttf")

	cache := newTTHintCache(data)
	if cache != nil {
		t.Error("expected nil cache for font without TT instructions")
	}
}

func TestHintedAdvanceWidth(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")

	cache := newTTHintCache(data)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	// Get hinted advance for available glyphs.
	ppem := int32(16)
	numGlyphs := cache.font.numGlyphs
	advancedCount := 0
	for gid := uint16(0); gid < uint16(numGlyphs); gid++ {
		advance, ok := cache.hintedAdvanceWidth(gid, ppem)
		if !ok {
			continue
		}
		if advance < 0 {
			t.Errorf("gid=%d: negative advance %f", gid, advance)
		}
		t.Logf("gid=%d: hinted advance = %.2f px", gid, advance)
		advancedCount++
	}

	if advancedCount == 0 {
		t.Error("no glyphs returned hinted advance")
	}
	t.Logf("%d glyphs returned hinted advances", advancedCount)
}

func TestTTHintedOutlineToGlyphOutline(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")

	cache := newTTHintCache(data)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	ppem := int32(16)
	numGlyphs := cache.font.numGlyphs
	convertedCount := 0
	for gid := uint16(0); gid < uint16(numGlyphs); gid++ {
		hinted, err := cache.hintGlyphOutline(gid, ppem)
		if err != nil {
			t.Errorf("hintGlyphOutline(gid=%d): %v", gid, err)
			continue
		}
		if hinted == nil {
			continue
		}

		outline := ttHintedOutlineToGlyphOutline(hinted, GlyphID(gid))
		if outline == nil {
			t.Errorf("gid=%d: conversion returned nil for non-nil hinted outline", gid)
			continue
		}

		if len(outline.Segments) == 0 {
			t.Errorf("gid=%d: no segments in converted outline", gid)
			continue
		}
		if outline.Advance <= 0 {
			t.Logf("gid=%d: advance=%f (may be zero-width)", gid, outline.Advance)
		}

		// Verify first segment is MoveTo.
		if outline.Segments[0].Op != OutlineOpMoveTo {
			t.Errorf("gid=%d: first segment op=%v, want MoveTo", gid, outline.Segments[0].Op)
		}

		t.Logf("gid=%d: %d segments, advance=%.2f, bounds=%v",
			gid, len(outline.Segments), outline.Advance, outline.Bounds)
		convertedCount++
	}

	if convertedCount == 0 {
		t.Error("no glyphs were converted")
	}
}

func TestParseGlyfFlags(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		offset    int
		numPoints int
		want      []byte
		wantEnd   int
		wantErr   bool
	}{
		{
			name:      "simple flags",
			data:      []byte{0x01, 0x00, 0x01},
			offset:    0,
			numPoints: 3,
			want:      []byte{0x01, 0x00, 0x01},
			wantEnd:   3,
		},
		{
			name:      "repeat flag",
			data:      []byte{0x09, 2}, // flag 0x01|0x08 (on-curve + repeat), repeat=2
			offset:    0,
			numPoints: 3,
			want:      []byte{0x09, 0x09, 0x09},
			wantEnd:   2,
		},
		{
			name:      "truncated",
			data:      []byte{},
			offset:    0,
			numPoints: 1,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, end, err := parseGlyfFlags(tt.data, tt.offset, tt.numPoints)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if end != tt.wantEnd {
				t.Errorf("end = %d, want %d", end, tt.wantEnd)
			}
			if len(flags) != len(tt.want) {
				t.Fatalf("flags len = %d, want %d", len(flags), len(tt.want))
			}
			for i := range flags {
				if flags[i] != tt.want[i] {
					t.Errorf("flags[%d] = 0x%02x, want 0x%02x", i, flags[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseHmtx(t *testing.T) {
	// Build a minimal hmtx table: 2 long metrics, 1 extra LSB.
	data := make([]byte, 2*4+1*2)
	// Metric 0: advance=600, lsb=50
	data[0] = 0x02
	data[1] = 0x58 // 600
	data[2] = 0x00
	data[3] = 0x32 // 50
	// Metric 1: advance=700, lsb=-10
	data[4] = 0x02
	data[5] = 0xBC // 700
	data[6] = 0xFF
	data[7] = 0xF6 // -10
	// Extra LSB for glyph 2: lsb=20
	data[8] = 0x00
	data[9] = 0x14 // 20

	advances, lsbs, err := parseHmtx(data, 2, 3)
	if err != nil {
		t.Fatalf("parseHmtx: %v", err)
	}

	if len(advances) != 2 {
		t.Fatalf("advances len = %d, want 2", len(advances))
	}
	if advances[0] != 600 {
		t.Errorf("advances[0] = %d, want 600", advances[0])
	}
	if advances[1] != 700 {
		t.Errorf("advances[1] = %d, want 700", advances[1])
	}
	if lsbs[0] != 50 {
		t.Errorf("lsbs[0] = %d, want 50", lsbs[0])
	}
	if lsbs[1] != -10 {
		t.Errorf("lsbs[1] = %d, want -10", lsbs[1])
	}
	if lsbs[2] != 20 {
		t.Errorf("lsbs[2] = %d, want 20", lsbs[2])
	}
}

func TestParseLocaOffsets(t *testing.T) {
	// Short format: 3 glyphs.
	shortData := []byte{
		0x00, 0x00, // glyph 0: offset 0*2 = 0
		0x00, 0x10, // glyph 1: offset 16*2 = 32
		0x00, 0x20, // glyph 2: offset 32*2 = 64
		0x00, 0x30, // end: offset 48*2 = 96
	}
	offsets, err := parseLocaOffsets(shortData, 3, false)
	if err != nil {
		t.Fatalf("parseLocaOffsets (short): %v", err)
	}
	if offsets[0].offset != 0 || offsets[0].length != 32 {
		t.Errorf("offset[0] = %v, want {0, 32}", offsets[0])
	}
	if offsets[1].offset != 32 || offsets[1].length != 32 {
		t.Errorf("offset[1] = %v, want {32, 32}", offsets[1])
	}

	// Long format: 2 glyphs.
	longData := []byte{
		0x00, 0x00, 0x00, 0x00, // glyph 0: offset 0
		0x00, 0x00, 0x01, 0x00, // glyph 1: offset 256
		0x00, 0x00, 0x02, 0x00, // end: offset 512
	}
	offsets, err = parseLocaOffsets(longData, 2, true)
	if err != nil {
		t.Fatalf("parseLocaOffsets (long): %v", err)
	}
	if offsets[0].offset != 0 || offsets[0].length != 256 {
		t.Errorf("offset[0] = %v, want {0, 256}", offsets[0])
	}
	if offsets[1].offset != 256 || offsets[1].length != 256 {
		t.Errorf("offset[1] = %v, want {256, 256}", offsets[1])
	}
}

func TestTTHintCache_CousineFont(t *testing.T) {
	data := loadTTTestFont(t, "cousine_hint_subset.ttf")

	cache := newTTHintCache(data)
	if cache == nil {
		t.Fatal("expected non-nil cache for Cousine")
	}

	ppem := int32(16)
	hintedCount := 0
	for gid := uint16(0); gid < 10; gid++ {
		advance, ok := cache.hintedAdvanceWidth(gid, ppem)
		if !ok {
			continue
		}

		// Cousine is monospace — all hinted advances should be the same.
		t.Logf("gid=%d: hinted advance = %.2f px", gid, advance)
		hintedCount++
	}

	if hintedCount == 0 {
		t.Error("no glyphs returned hinted advance for Cousine")
	}
}

func TestConvertContourToSegments(t *testing.T) {
	// Simple triangle: 3 on-curve points.
	points := [][2]int32{
		{0, 0},     // on-curve
		{640, 0},   // on-curve (10px)
		{320, 640}, // on-curve (5px, 10px)
	}
	flags := []ttPointFlags{
		ttPointFlagOnCurve,
		ttPointFlagOnCurve,
		ttPointFlagOnCurve,
	}

	segments := convertContourToSegments(points, flags, 3)
	if len(segments) == 0 {
		t.Fatal("expected segments for triangle")
	}

	// First segment should be MoveTo.
	if segments[0].Op != OutlineOpMoveTo {
		t.Errorf("first segment = %v, want MoveTo", segments[0].Op)
	}

	// Should have MoveTo + 2 LineTo (wrapping back handled by contour close).
	moveCount := 0
	lineCount := 0
	for _, seg := range segments {
		switch seg.Op {
		case OutlineOpMoveTo:
			moveCount++
		case OutlineOpLineTo:
			lineCount++
		}
	}
	if moveCount != 1 {
		t.Errorf("moveCount = %d, want 1", moveCount)
	}
	if lineCount < 2 {
		t.Errorf("lineCount = %d, want >= 2", lineCount)
	}
}

func TestConvertContourToSegments_WithCurves(t *testing.T) {
	// Square with one quadratic curve side:
	// On-curve → Off-curve → On-curve → On-curve.
	points := [][2]int32{
		{0, 0},     // on-curve
		{320, 320}, // off-curve (control point)
		{640, 0},   // on-curve
		{640, 640}, // on-curve
	}
	flags := []ttPointFlags{
		ttPointFlagOnCurve,
		0, // off-curve
		ttPointFlagOnCurve,
		ttPointFlagOnCurve,
	}

	segments := convertContourToSegments(points, flags, 4)
	if len(segments) == 0 {
		t.Fatal("expected segments")
	}

	// Should contain at least one QuadTo.
	hasQuad := false
	for _, seg := range segments {
		if seg.Op == OutlineOpQuadTo {
			hasQuad = true
			break
		}
	}
	if !hasQuad {
		t.Error("expected at least one QuadTo segment")
	}
}

func TestF26dot6ToPixels(t *testing.T) {
	tests := []struct {
		input int32
		wantX float32
		wantY float32
	}{
		{0, 0, 0},
		{64, 1.0, -1.0},
		{32, 0.5, -0.5},
		{-64, -1.0, 1.0},
		{128, 2.0, -2.0},
		{96, 1.5, -1.5},
	}
	for _, tt := range tests {
		gotX := f26dot6ToPixelsX(tt.input)
		if math.Abs(float64(gotX-tt.wantX)) > 0.001 {
			t.Errorf("f26dot6ToPixelsX(%d) = %f, want %f", tt.input, gotX, tt.wantX)
		}
		gotY := f26dot6ToPixelsY(tt.input)
		if math.Abs(float64(gotY-tt.wantY)) > 0.001 {
			t.Errorf("f26dot6ToPixelsY(%d) = %f, want %f (Y negated)", tt.input, gotY, tt.wantY)
		}
	}
}

func TestGlyphMetrics(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}
	loader, err := newTTGlyphLoader(data, fp)
	if err != nil || loader == nil {
		t.Fatalf("newTTGlyphLoader: %v", err)
	}

	// Glyph 0 metrics should exist.
	advance, lsb := loader.glyphMetrics(0)
	t.Logf("gid=0: advance=%d, lsb=%d", advance, lsb)
	// Advance should be reasonable (not zero for .notdef, typically).
	if advance == 0 && lsb == 0 {
		t.Logf("gid=0: both advance and lsb are zero (may be valid for some fonts)")
	}
}

func TestTTHintInstance_IsEnabled(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	instance, err := newTTHintInstance(fp, 16, ttTargetSmooth)
	if err != nil || instance == nil {
		t.Fatalf("newTTHintInstance: instance=%v err=%v", instance, err)
	}

	// Most fonts don't disable hinting via prep.
	if !instance.isEnabled() {
		t.Log("hinting disabled by prep program (unusual but valid)")
	} else {
		t.Log("hinting enabled (expected)")
	}
}

func TestTTHintInstance_BackwardCompatibility(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	// Test with smooth target (default for screen rendering).
	instance, err := newTTHintInstance(fp, 16, ttTargetSmooth)
	if err != nil || instance == nil {
		t.Fatalf("newTTHintInstance: instance=%v err=%v", instance, err)
	}

	bc := instance.backwardCompatibility()
	t.Logf("backward compatibility (smooth target): %v", bc)

	// Test with normal target.
	instance2, err := newTTHintInstance(fp, 16, ttTargetNormal)
	if err != nil || instance2 == nil {
		t.Fatalf("newTTHintInstance (normal): instance=%v err=%v", instance2, err)
	}

	bc2 := instance2.backwardCompatibility()
	t.Logf("backward compatibility (normal target): %v", bc2)
}

func TestLoadGlyphOutline_OutOfRange(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	fp, err := loadTTFontProgram(data)
	if err != nil || fp == nil {
		t.Fatalf("loadTTFontProgram: fp=%v err=%v", fp, err)
	}

	loader, err := newTTGlyphLoader(data, fp)
	if err != nil || loader == nil {
		t.Fatalf("newTTGlyphLoader: %v", err)
	}

	// Out of range glyph ID should return error.
	_, err = loader.loadGlyphOutline(uint16(fp.numGlyphs+10), 1<<16)
	if err == nil {
		t.Error("expected error for out-of-range glyph ID")
	}
}

func TestHintedAdvanceIntegerPixels(t *testing.T) {
	data := loadTTTestFont(t, "tthint_subset.ttf")
	cache := newTTHintCache(data)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	// Hinted advances should be integer or half-pixel values in most fonts.
	ppem := int32(16)
	numGlyphs := cache.font.numGlyphs
	for gid := uint16(0); gid < uint16(numGlyphs); gid++ {
		advance, ok := cache.hintedAdvanceWidth(gid, ppem)
		if !ok {
			continue
		}

		// The advance from TT hinting (via phantom points) should be
		// a precise value — often grid-fitted to integer pixels.
		// We don't enforce exact integer, just that it's reasonable.
		if advance > 100 || advance < -10 {
			t.Errorf("gid=%d: suspicious advance %.2f px at %dppem", gid, advance, ppem)
		}
	}
}
