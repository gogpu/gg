package text

import (
	"image"
	"image/color"
	"os"
	"sync"
	"testing"
)

func TestDraw(t *testing.T) {
	fontPath := testFontPath(t)

	// Load font
	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	// Create face
	face := source.Face(12.0)

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, 200, 50))

	// Draw text
	Draw(dst, "Hello, World!", face, 10, 30, color.Black)

	// Verify that some pixels were modified (basic smoke test)
	modified := false
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			r, g, b, a := dst.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 || a != 0 {
				modified = true
				break
			}
		}
		if modified {
			break
		}
	}

	if !modified {
		t.Error("Expected Draw to modify the destination image")
	}
}

func TestDrawEmpty(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)
	dst := image.NewRGBA(image.Rect(0, 0, 100, 50))

	// Draw empty string (should not panic)
	Draw(dst, "", face, 10, 30, color.Black)
}

func TestDrawCachesCPUGlyphMasksPerFontSource(t *testing.T) {
	source := loadTestFont(t)
	defer func() { _ = source.Close() }()

	face := source.Face(16)
	dst := image.NewRGBA(image.Rect(0, 0, 200, 50))
	Draw(dst, "Repeated text", face, 10, 30, color.Black)

	cache := source.cpuGlyphMaskCache()
	if cache.Len() == 0 {
		t.Fatal("CPU glyph mask cache is empty after drawing text")
	}
	cache.ResetStats()

	Draw(dst, "Repeated text", face, 10, 30, color.Black)
	stats := cache.Stats()
	if stats.Hits == 0 {
		t.Fatal("second draw did not reuse cached CPU glyph masks")
	}
	if stats.Misses != 0 {
		t.Fatalf("second draw cache misses = %d, want 0", stats.Misses)
	}
}

func TestDrawCPUGlyphMaskCacheSeparatesRasterModes(t *testing.T) {
	source := loadTestFont(t)
	defer func() { _ = source.Close() }()

	face := source.Face(16)
	dst := image.NewRGBA(image.Rect(0, 0, 100, 50))
	Draw(dst, "A", face, 10, 30, color.Black)
	entriesAfterAA := source.cpuGlyphMaskCache().Len()

	DrawAliased(dst, "A", face, 10, 30, color.Black)
	if got := source.cpuGlyphMaskCache().Len(); got <= entriesAfterAA {
		t.Fatalf("cache entries after aliased draw = %d, want more than AA entries %d", got, entriesAfterAA)
	}
}

func TestDrawCPUGlyphMaskCacheConcurrent(t *testing.T) {
	source := loadTestFont(t)
	defer func() { _ = source.Close() }()

	face := source.Face(16)
	const goroutines = 8
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			dst := image.NewRGBA(image.Rect(0, 0, 200, 50))
			for range 10 {
				Draw(dst, "Concurrent text", face, 10, 30, color.Black)
			}
		}()
	}
	wg.Wait()
}

func TestDrawNilFace(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 100, 50))

	// Draw with nil face (should not panic)
	Draw(dst, "Hello", nil, 10, 30, color.Black)
}

func TestMeasure(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)

	tests := []struct {
		name string
		text string
	}{
		{"Simple", "Hello"},
		{"With spaces", "Hello World"},
		{"Long text", "The quick brown fox jumps over the lazy dog"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := Measure(tt.text, face)

			if w <= 0 {
				t.Errorf("Expected positive width, got %f", w)
			}

			if h <= 0 {
				t.Errorf("Expected positive height, got %f", h)
			}

			// Width should increase with text length
			if len(tt.text) > 5 {
				shortW, _ := Measure(tt.text[:5], face)
				if w <= shortW {
					t.Errorf("Expected width to increase with text length: %f vs %f", w, shortW)
				}
			}
		})
	}
}

func TestMeasureEmpty(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)

	w, h := Measure("", face)

	if w != 0 {
		t.Errorf("Expected width 0 for empty string, got %f", w)
	}

	// Height might be non-zero (line height), which is acceptable
	if h < 0 {
		t.Errorf("Expected non-negative height, got %f", h)
	}
}

func TestMeasureNilFace(t *testing.T) {
	w, h := Measure("Hello", nil)

	if w != 0 || h != 0 {
		t.Errorf("Expected (0, 0) for nil face, got (%f, %f)", w, h)
	}
}

func TestDrawColor(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(24.0)
	dst := image.NewRGBA(image.Rect(0, 0, 200, 50))

	// Draw with red color
	Draw(dst, "Test", face, 10, 30, color.RGBA{R: 255, A: 255})

	// Find a colored pixel
	foundRed := false
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			r, g, b, a := dst.At(x, y).RGBA()
			// Check if we have a red-ish pixel (r > 0, g ≈ 0, b ≈ 0)
			if r > 0 && g < 100 && b < 100 && a > 0 {
				foundRed = true
				break
			}
		}
		if foundRed {
			break
		}
	}

	if !foundRed {
		t.Error("Expected to find red pixels in the drawn text")
	}
}

func TestMeasureConsistency(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)

	// Measure the same text multiple times
	text := "Consistency Test"
	w1, h1 := Measure(text, face)
	w2, h2 := Measure(text, face)

	if w1 != w2 {
		t.Errorf("Width not consistent: %f vs %f", w1, w2)
	}

	if h1 != h2 {
		t.Errorf("Height not consistent: %f vs %f", h1, h2)
	}
}

func TestDrawMultiFace(t *testing.T) {
	fontPath := testFontPath(t)

	// Load font
	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	// Create faces for MultiFace
	face1 := source.Face(12.0)
	face2 := source.Face(14.0)

	// Create MultiFace
	multiFace, err := NewMultiFace(face1, face2)
	if err != nil {
		t.Fatalf("Failed to create MultiFace: %v", err)
	}

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, 200, 50))

	// Draw text using MultiFace - this was the bug in Issue #34
	Draw(dst, "Hello, World!", multiFace, 10, 30, color.Black)

	// Verify that some pixels were modified
	modified := false
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			r, g, b, a := dst.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 || a != 0 {
				modified = true
				break
			}
		}
		if modified {
			break
		}
	}

	if !modified {
		t.Error("Expected Draw with MultiFace to modify the destination image (Issue #34 regression)")
	}
}

func TestDrawFilteredFace(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)

	// Create FilteredFace with ASCII range
	filteredFace := NewFilteredFace(face, UnicodeRange{Start: 0x0020, End: 0x007F})

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, 200, 50))

	// Draw text using FilteredFace
	Draw(dst, "Hello", filteredFace, 10, 30, color.Black)

	// Verify that some pixels were modified
	modified := false
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			r, g, b, a := dst.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 || a != 0 {
				modified = true
				break
			}
		}
		if modified {
			break
		}
	}

	if !modified {
		t.Error("Expected Draw with FilteredFace to modify the destination image")
	}
}

func TestDrawMultiFaceWithFilteredFaces(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	// Create base faces
	face1 := source.Face(12.0)
	face2 := source.Face(12.0)

	// Create filtered faces for different ranges
	latinFace := NewFilteredFace(face1, UnicodeRange{Start: 0x0000, End: 0x024F}) // Latin
	extendedFace := NewFilteredFace(face2, UnicodeRange{Start: 0x0250, End: 0xFFFF})

	// Create MultiFace from filtered faces
	multiFace, err := NewMultiFace(latinFace, extendedFace)
	if err != nil {
		t.Fatalf("Failed to create MultiFace: %v", err)
	}

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, 300, 50))

	// Draw text - this tests the full composite face rendering chain
	Draw(dst, "Hello World 123", multiFace, 10, 30, color.Black)

	// Verify that some pixels were modified
	modified := false
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			r, g, b, a := dst.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 || a != 0 {
				modified = true
				break
			}
		}
		if modified {
			break
		}
	}

	if !modified {
		t.Error("Expected Draw with MultiFace containing FilteredFaces to modify the destination image")
	}
}

func TestMeasureMultiFace(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face1 := source.Face(12.0)
	face2 := source.Face(14.0)

	multiFace, err := NewMultiFace(face1, face2)
	if err != nil {
		t.Fatalf("Failed to create MultiFace: %v", err)
	}

	w, h := Measure("Hello", multiFace)

	if w <= 0 {
		t.Errorf("Expected positive width for MultiFace, got %f", w)
	}

	if h <= 0 {
		t.Errorf("Expected positive height for MultiFace, got %f", h)
	}
}

func TestMeasureFilteredFace(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)
	filteredFace := NewFilteredFace(face, UnicodeRange{Start: 0x0020, End: 0x007F})

	w, h := Measure("Hello", filteredFace)

	if w <= 0 {
		t.Errorf("Expected positive width for FilteredFace, got %f", w)
	}

	if h <= 0 {
		t.Errorf("Expected positive height for FilteredFace, got %f", h)
	}
}

// findNotoSansSCVar returns the path to NotoSansSC variable font for testing.
func findNotoSansSCVar() string {
	paths := []string{
		"../tmp/tsl0922_fonts/NotoSansSC-VariableFont_wght.ttf",
		"tmp/tsl0922_fonts/NotoSansSC-VariableFont_wght.ttf",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// TestVarOrRawAdvance verifies the helper uses HVAR when available,
// raw hmtx otherwise.
func TestVarOrRawAdvance(t *testing.T) {
	fontPath := findNotoSansSCVar()
	if fontPath == "" {
		fontPath = findVariableFontForTest(t)
	}
	if fontPath == "" {
		t.Skip("No variable font with HVAR available")
	}

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()

	parsed := source.Parsed()
	varProv, ok := parsed.(VariableAdvanceProvider)
	if !ok {
		t.Skip("font does not implement VariableAdvanceProvider")
	}

	axes := source.VariationAxes()
	var wghtMax float32
	for _, ax := range axes {
		if ax.Tag == [4]byte{'w', 'g', 'h', 't'} {
			wghtMax = ax.Maximum
			break
		}
	}
	if wghtMax == 0 {
		t.Skip("font has no wght axis")
	}

	variations := []FontVariation{NewFontVariation("wght", wghtMax)}
	gid := parsed.GlyphIndex('H')
	if gid == 0 {
		gid = parsed.GlyphIndex('A')
	}
	ppem := 14.0

	// With HVAR provider: should return HVAR advance.
	hvarAdv := varOrRawAdvance(varProv, parsed, gid, ppem, variations)
	expectedHVAR := varProv.GlyphAdvanceVar(gid, ppem, variations)
	if hvarAdv != expectedHVAR {
		t.Errorf("with varProv: got %.4f, want %.4f", hvarAdv, expectedHVAR)
	}

	// Without HVAR provider: should return raw hmtx advance.
	rawAdv := varOrRawAdvance(nil, parsed, gid, ppem, variations)
	expectedRaw := parsed.GlyphAdvance(gid, ppem)
	if rawAdv != expectedRaw {
		t.Errorf("without varProv: got %.4f, want %.4f", rawAdv, expectedRaw)
	}

	t.Logf("gid=%d: HVAR=%.4f, raw=%.4f", gid, hvarAdv, rawAdv)
}

// TestDrawGlyphsVariable_UsesHVARAdvance_405 verifies that drawGlyphsVariable
// uses HVAR advances for positioning, matching Face.Advance(). This is the fix
// for #405: TT-hinted phantom advances (integer-rounded) diverged from HVAR
// advances (fractional) causing measurement/rendering mismatch.
//
// FreeType ttgload.c:964-977: when HVAR present, discard TT-hinted phantom
// advances and use HVAR-scaled advance instead.
func TestDrawGlyphsVariable_UsesHVARAdvance_405(t *testing.T) {
	fontPath := findNotoSansSCVar()
	if fontPath == "" {
		fontPath = findVariableFontForTest(t)
	}
	if fontPath == "" {
		t.Skip("No variable font available")
	}

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()

	axes := source.VariationAxes()
	var wghtMax float32
	for _, ax := range axes {
		if ax.Tag == [4]byte{'w', 'g', 'h', 't'} {
			wghtMax = ax.Maximum
			break
		}
	}
	if wghtMax == 0 {
		t.Skip("font has no wght axis")
	}

	variations := []FontVariation{NewFontVariation("wght", wghtMax)}

	tests := []struct {
		name string
		text string
		size float64
	}{
		{"Hello_14px", "Hello", 14},
		{"Hello_22px", "Hello", 22},
		{"Abcdef_14px", "abcdef", 14},
		{"Test_36px", "Test", 36},
	}

	// Add CJK tests only when using NotoSansSC.
	if findNotoSansSCVar() != "" {
		tests = append(tests,
			struct {
				name, text string
				size       float64
			}{"Chinese_14px", "你好世界", 14},
			struct {
				name, text string
				size       float64
			}{"Chinese_22px", "你好世界", 22},
		)
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			face := source.Face(tc.size, WithVariations(variations...))

			faceAdv := face.Advance(tc.text)
			if faceAdv <= 0 {
				t.Fatalf("Face.Advance(%q) = %.4f — should be positive", tc.text, faceAdv)
			}

			// Render to an image and measure actual ink extent.
			// If rendering uses a different advance than Face.Advance, the last
			// glyph will be positioned at the wrong X.
			dst := image.NewRGBA(image.Rect(0, 0, 500, 100))
			startX := 50.0
			Draw(dst, tc.text, face, startX, 60, color.Black)

			// Find rightmost ink pixel.
			rightmostInk := -1
			for x := 499; x >= 0; x-- {
				for y := 0; y < 100; y++ {
					_, _, _, a := dst.At(x, y).RGBA()
					if a > 0 {
						rightmostInk = x
						goto found
					}
				}
			}
		found:

			if rightmostInk < 0 {
				t.Skipf("font lacks glyphs for %q — no ink rendered", tc.text)
			}

			// The rightmost ink should be near startX + faceAdv.
			// Allow tolerance for glyph overshoot (typically ≤3px at these sizes).
			renderedWidth := float64(rightmostInk) - startX
			diff := renderedWidth - faceAdv
			if diff < 0 {
				diff = -diff
			}

			t.Logf("%s: Face.Advance=%.2f, rendered ink width=%.1f, diff=%.1f",
				tc.name, faceAdv, renderedWidth, diff)

			// Before fix: diff was 1-4px due to integer phantom advance.
			// After fix: diff should be <3px (glyph overshoot only).
			if diff > 5 {
				t.Errorf("measurement/rendering mismatch: Face.Advance=%.2f, rendered=%.1f (diff=%.1f > 5px)",
					faceAdv, renderedWidth, diff)
			}
		})
	}
}

// TestFaceGlyphAdvance_UsesAdvanceResolver verifies that faceGlyphAdvance
// (used by drawMultiFace) returns the same advance as Face.Advance for
// both static and variable fonts.
func TestFaceGlyphAdvance_UsesAdvanceResolver(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(14.0)
	sf := face.(*sourceFace)

	for _, r := range "Hello" {
		runeStr := string(r)
		fga := faceGlyphAdvance(sf, runeStr)
		faceAdv := sf.Advance(runeStr)

		if fga != faceAdv {
			t.Errorf("faceGlyphAdvance(%q)=%.4f != Face.Advance(%q)=%.4f",
				runeStr, fga, runeStr, faceAdv)
		}
	}
}

// TestFaceGlyphAdvance_VariableFont_UsesHVAR verifies that faceGlyphAdvance
// returns HVAR advance for variable fonts, not TT-hinted phantom advance.
func TestFaceGlyphAdvance_VariableFont_UsesHVAR(t *testing.T) {
	fontPath := findNotoSansSCVar()
	if fontPath == "" {
		fontPath = findVariableFontForTest(t)
	}
	if fontPath == "" {
		t.Skip("No variable font available")
	}

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()

	axes := source.VariationAxes()
	var wghtMax float32
	for _, ax := range axes {
		if ax.Tag == [4]byte{'w', 'g', 'h', 't'} {
			wghtMax = ax.Maximum
			break
		}
	}
	if wghtMax == 0 {
		t.Skip("font has no wght axis")
	}

	variations := []FontVariation{NewFontVariation("wght", wghtMax)}
	face := source.Face(14.0, WithVariations(variations...))
	sf := face.(*sourceFace)

	parsed := source.Parsed()
	varProv, ok := parsed.(VariableAdvanceProvider)
	if !ok {
		t.Skip("font does not implement VariableAdvanceProvider")
	}

	for _, r := range "Helo" {
		gid := parsed.GlyphIndex(r)
		if gid == 0 {
			continue
		}
		runeStr := string(r)
		fga := faceGlyphAdvance(sf, runeStr)
		faceAdv := sf.Advance(runeStr)

		if fga != faceAdv {
			t.Errorf("faceGlyphAdvance(%q)=%.4f != Face.Advance(%q)=%.4f — advance source mismatch",
				runeStr, fga, runeStr, faceAdv)
		}

		hvarAdv := varProv.GlyphAdvanceVar(gid, 14.0, variations)
		if fga != hvarAdv {
			t.Errorf("faceGlyphAdvance(%q)=%.4f != HVAR=%.4f — not using HVAR for variable font",
				runeStr, fga, hvarAdv)
		}
	}
}

// TestDrawVariable_MeasureRenderConsistency_405 is the definitive regression
// test for #405. It verifies the invariant that the total advance used during
// rendering matches Face.Advance() — so MeasureString-based positioning works.
func TestDrawVariable_MeasureRenderConsistency_405(t *testing.T) {
	fontPath := findNotoSansSCVar()
	if fontPath == "" {
		fontPath = findVariableFontForTest(t)
	}
	if fontPath == "" {
		t.Skip("No variable font available")
	}

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()

	parsed := source.Parsed()
	varProv, ok := parsed.(VariableAdvanceProvider)
	if !ok {
		t.Skip("font does not implement VariableAdvanceProvider")
	}

	axes := source.VariationAxes()
	var wghtMax float32
	for _, ax := range axes {
		if ax.Tag == [4]byte{'w', 'g', 'h', 't'} {
			wghtMax = ax.Maximum
			break
		}
	}
	if wghtMax == 0 {
		t.Skip("font has no wght axis")
	}

	variations := []FontVariation{NewFontVariation("wght", wghtMax)}

	testCases := []struct {
		size float64
		text string
	}{
		{14, "Hello"},
		{14, "rl"},
		{22, "Hello"},
		{36, "Test"},
	}
	if findNotoSansSCVar() != "" {
		testCases = append(testCases, struct {
			size float64
			text string
		}{22, "你好世界"})
	}

	for _, tc := range testCases {
		face := source.Face(tc.size, WithVariations(variations...))
		faceAdv := face.Advance(tc.text)

		// Compute the advance that drawGlyphsVariable would use per-glyph.
		renderAdv := 0.0
		for _, r := range tc.text {
			if r < 0x20 && r != '\t' {
				continue
			}
			gid := parsed.GlyphIndex(r)
			renderAdv += varOrRawAdvance(varProv, parsed, gid, tc.size, variations)
		}

		if faceAdv != renderAdv {
			t.Errorf("size=%.0f %q: Face.Advance=%.4f != render advance=%.4f (diff=%.4f)",
				tc.size, tc.text, faceAdv, renderAdv, faceAdv-renderAdv)
		}
	}
}

func BenchmarkDraw(b *testing.B) {
	// Try to get a font, skip if not available
	candidates := []string{
		"C:\\Windows\\Fonts\\arial.ttf",
		"/System/Library/Fonts/Helvetica.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}

	var fontPath string
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			fontPath = path
			break
		}
	}

	if fontPath == "" {
		b.Skip("No font available for benchmarking")
	}

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		b.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)
	dst := image.NewRGBA(image.Rect(0, 0, 400, 100))
	text := "The quick brown fox jumps over the lazy dog"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Draw(dst, text, face, 10, 50, color.Black)
	}
}

func BenchmarkDrawCPUGlyphMaskCacheHit(b *testing.B) {
	source, err := NewFontSource(requireTestFont(b))
	if err != nil {
		b.Fatalf("NewFontSource: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(16)
	dst := image.NewRGBA(image.Rect(0, 0, 400, 100))
	const value = "The quick brown fox jumps over the lazy dog"
	Draw(dst, value, face, 10, 50, color.Black)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		Draw(dst, value, face, 10, 50, color.Black)
	}
}

func BenchmarkMeasure(b *testing.B) {
	candidates := []string{
		"C:\\Windows\\Fonts\\arial.ttf",
		"/System/Library/Fonts/Helvetica.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}

	var fontPath string
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			fontPath = path
			break
		}
	}

	if fontPath == "" {
		b.Skip("No font available for benchmarking")
	}

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		b.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(12.0)
	text := "The quick brown fox jumps over the lazy dog"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Measure(text, face)
	}
}
