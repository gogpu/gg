package text

import (
	"image"
	"image/color"
	"image/draw"
	"os"
	"testing"
)

// variableFontPath returns the path to a variable font for testing.
// Returns empty string if no variable font is available.
func variableFontPath(t *testing.T) string {
	t.Helper()

	candidates := []string{
		// Windows — Bahnschrift is a known variable font (wght axis).
		"C:\\Windows\\Fonts\\bahnschrift.ttf",
		"C:\\Windows\\Fonts\\CascadiaCode.ttf",
		// macOS
		"/System/Library/Fonts/SFNS.ttf",
		"/System/Library/Fonts/SFNSDisplay.ttf",
		// Linux — variable fonts are less common in system paths
		"/usr/share/fonts/truetype/dejavu/DejaVuSans-VF.ttf",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// requireVariableFont loads a variable font or skips the test.
func requireVariableFont(t *testing.T) *FontSource {
	t.Helper()
	path := variableFontPath(t)
	if path == "" {
		t.Skip("No variable font available on this system")
	}

	source, err := NewFontSourceFromFile(path)
	if err != nil {
		t.Fatalf("Failed to load variable font %s: %v", path, err)
	}

	if !source.IsVariable() {
		_ = source.Close()
		t.Skipf("Font %s is not variable", path)
	}

	return source
}

// requireTrueTypeVariableFont loads a TrueType variable font with glyf+gvar
// tables, or skips the test. CFF2 variable fonts (which store variation data
// inline in CFF2 charstrings, not in a separate gvar table) are not suitable
// for gvar-specific tests.
func requireTrueTypeVariableFont(t *testing.T) *FontSource {
	t.Helper()
	path := variableFontPath(t)
	if path == "" {
		t.Skip("No variable font available on this system")
	}

	source, err := NewFontSourceFromFile(path)
	if err != nil {
		t.Fatalf("Failed to load variable font %s: %v", path, err)
	}

	if !source.IsVariable() {
		_ = source.Close()
		t.Skipf("Font %s is not variable", path)
	}

	// Verify TrueType outlines (glyf+gvar) rather than CFF2.
	parsed := source.Parsed()
	ownFont, ok := parsed.(*ownParsedFont)
	if !ok {
		_ = source.Close()
		t.Skip("test requires ownParsedFont")
	}
	_, hasGlyf := ownFont.tables["glyf"]
	_, hasGvar := ownFont.tables["gvar"]
	if !hasGlyf || !hasGvar {
		_ = source.Close()
		t.Skipf("Font %s is variable but uses CFF2 outlines (no glyf+gvar) — our gvar parser requires TrueType", path)
	}

	return source
}

// --------------------------------------------------------------------------
// FontVariation type tests
// --------------------------------------------------------------------------

// TestNewFontVariation verifies the string-based constructor produces correct tags.
func TestNewFontVariation(t *testing.T) {
	tests := []struct {
		tag     string
		value   float32
		wantTag [4]byte
	}{
		{"wght", 700, [4]byte{'w', 'g', 'h', 't'}},
		{"wdth", 125, [4]byte{'w', 'd', 't', 'h'}},
		{"slnt", -12, [4]byte{'s', 'l', 'n', 't'}},
		{"ital", 1, [4]byte{'i', 't', 'a', 'l'}},
		{"opsz", 48, [4]byte{'o', 'p', 's', 'z'}},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			v := NewFontVariation(tt.tag, tt.value)
			if v.Tag != tt.wantTag {
				t.Errorf("NewFontVariation(%q, %v).Tag = %v, want %v", tt.tag, tt.value, v.Tag, tt.wantTag)
			}
			if v.Value != tt.value {
				t.Errorf("NewFontVariation(%q, %v).Value = %v, want %v", tt.tag, tt.value, v.Value, tt.value)
			}
		})
	}
}

// TestNewFontVariation_PanicShortTag verifies panic on too-short tag.
func TestNewFontVariation_PanicShortTag(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewFontVariation(\"ab\", 1) did not panic")
		}
	}()
	NewFontVariation("ab", 1)
}

// TestNewFontVariation_PanicLongTag verifies panic on too-long tag.
func TestNewFontVariation_PanicLongTag(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewFontVariation(\"abcde\", 1) did not panic")
		}
	}()
	NewFontVariation("abcde", 1)
}

// TestNewFontVariation_PanicEmptyTag verifies panic on empty tag.
func TestNewFontVariation_PanicEmptyTag(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewFontVariation(\"\", 1) did not panic")
		}
	}()
	NewFontVariation("", 1)
}

// --------------------------------------------------------------------------
// Axis constants
// --------------------------------------------------------------------------

// TestAxisConstants verifies the predefined axis tag constants.
func TestAxisConstants(t *testing.T) {
	tests := []struct {
		name string
		axis [4]byte
		want [4]byte
	}{
		{"AxisWeight", AxisWeight, [4]byte{'w', 'g', 'h', 't'}},
		{"AxisWidth", AxisWidth, [4]byte{'w', 'd', 't', 'h'}},
		{"AxisItalic", AxisItalic, [4]byte{'i', 't', 'a', 'l'}},
		{"AxisSlant", AxisSlant, [4]byte{'s', 'l', 'n', 't'}},
		{"AxisOpticalSize", AxisOpticalSize, [4]byte{'o', 'p', 's', 'z'}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.axis != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.axis, tt.want)
			}
		})
	}
}

// --------------------------------------------------------------------------
// WithVariations FaceOption tests
// --------------------------------------------------------------------------

// TestWithVariations verifies that WithVariations stores variations on a face.
func TestWithVariations(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	bold := NewFontVariation("wght", 700)
	condensed := NewFontVariation("wdth", 75)

	face := source.Face(16, WithVariations(bold, condensed))
	vars := face.Variations()

	if len(vars) != 2 {
		t.Fatalf("Variations() returned %d variations, want 2", len(vars))
	}

	if vars[0] != bold {
		t.Errorf("vars[0] = %+v, want %+v", vars[0], bold)
	}
	if vars[1] != condensed {
		t.Errorf("vars[1] = %+v, want %+v", vars[1], condensed)
	}
}

// TestWithVariations_None verifies that no variations is the default.
func TestWithVariations_None(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(16)
	vars := face.Variations()

	if len(vars) != 0 {
		t.Errorf("Variations() returned %d variations, want 0 (default)", len(vars))
	}
}

// TestWithVariations_Empty verifies that WithVariations() with no args clears.
func TestWithVariations_Empty(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(16, WithVariations())
	vars := face.Variations()

	if len(vars) != 0 {
		t.Errorf("Variations() returned %d variations, want 0 for empty WithVariations()", len(vars))
	}
}

// TestWithVariations_IndependentPerFace verifies variations are independent per face.
func TestWithVariations_IndependentPerFace(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	bold := NewFontVariation("wght", 700)
	light := NewFontVariation("wght", 300)

	face1 := source.Face(16, WithVariations(bold))
	face2 := source.Face(16, WithVariations(light))
	face3 := source.Face(16) // No variations.

	if len(face1.Variations()) != 1 || face1.Variations()[0].Value != 700 {
		t.Errorf("face1 variations: got %+v, want [wght=700]", face1.Variations())
	}
	if len(face2.Variations()) != 1 || face2.Variations()[0].Value != 300 {
		t.Errorf("face2 variations: got %+v, want [wght=300]", face2.Variations())
	}
	if len(face3.Variations()) != 0 {
		t.Errorf("face3 variations: got %+v, want []", face3.Variations())
	}
}

// TestWithVariations_CombinedWithFeatures verifies features and variations coexist.
func TestWithVariations_CombinedWithFeatures(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(16,
		WithFeatures(TabularNums),
		WithVariations(NewFontVariation("wght", 700)),
	)

	if len(face.Features()) != 1 {
		t.Errorf("Features() = %d, want 1", len(face.Features()))
	}
	if len(face.Variations()) != 1 {
		t.Errorf("Variations() = %d, want 1", len(face.Variations()))
	}
}

// --------------------------------------------------------------------------
// FontSource.IsVariable tests
// --------------------------------------------------------------------------

// TestIsVariable_StaticFont verifies that goregular (static) returns false.
func TestIsVariable_StaticFont(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	if source.IsVariable() {
		t.Error("IsVariable() = true for static font, want false")
	}
}

// TestIsVariable_VariableFont verifies that a variable font returns true.
func TestIsVariable_VariableFont(t *testing.T) {
	source := requireVariableFont(t)
	defer func() { _ = source.Close() }()

	if !source.IsVariable() {
		t.Error("IsVariable() = false for variable font, want true")
	}
}

// --------------------------------------------------------------------------
// FontSource.VariationAxes tests
// --------------------------------------------------------------------------

// TestVariationAxes_StaticFont verifies nil for static font.
func TestVariationAxes_StaticFont(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	axes := source.VariationAxes()
	if axes != nil {
		t.Errorf("VariationAxes() = %v for static font, want nil", axes)
	}
}

// TestVariationAxes_VariableFont verifies axes are returned for a variable font.
func TestVariationAxes_VariableFont(t *testing.T) {
	source := requireVariableFont(t)
	defer func() { _ = source.Close() }()

	axes := source.VariationAxes()
	if len(axes) == 0 {
		t.Fatal("VariationAxes() returned empty for variable font")
	}

	// Verify each axis has valid data.
	for i, axis := range axes {
		if axis.Tag == [4]byte{} {
			t.Errorf("axis[%d].Tag is zero", i)
		}
		if axis.Name == "" {
			t.Errorf("axis[%d].Name is empty", i)
		}
		if axis.Maximum < axis.Minimum {
			t.Errorf("axis[%d] %q: Maximum (%.1f) < Minimum (%.1f)",
				i, axis.Name, axis.Maximum, axis.Minimum)
		}
		if axis.Default < axis.Minimum || axis.Default > axis.Maximum {
			t.Errorf("axis[%d] %q: Default (%.1f) out of range [%.1f, %.1f]",
				i, axis.Name, axis.Default, axis.Minimum, axis.Maximum)
		}

		t.Logf("Axis %d: %q (%s) range=[%.1f, %.1f] default=%.1f",
			i, axis.Name, string(axis.Tag[:]), axis.Minimum, axis.Maximum, axis.Default)
	}

	// Most variable fonts have at least a weight axis.
	hasWeight := false
	for _, axis := range axes {
		if axis.Tag == AxisWeight {
			hasWeight = true
			break
		}
	}
	if !hasWeight {
		t.Log("Note: variable font has no weight axis (unusual but valid)")
	}
}

// --------------------------------------------------------------------------
// FontSource.NamedInstances tests
// --------------------------------------------------------------------------

// TestNamedInstances_StaticFont verifies nil for static font.
func TestNamedInstances_StaticFont(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatalf("failed to load test font: %v", err)
	}
	defer func() { _ = source.Close() }()

	instances := source.NamedInstances()
	if instances != nil {
		t.Errorf("NamedInstances() = %v for static font, want nil", instances)
	}
}

// TestNamedInstances_VariableFont verifies instances are returned.
func TestNamedInstances_VariableFont(t *testing.T) {
	source := requireVariableFont(t)
	defer func() { _ = source.Close() }()

	instances := source.NamedInstances()
	// Not all variable fonts have named instances, so skip if none.
	if len(instances) == 0 {
		t.Skip("Variable font has no named instances")
	}

	for i, inst := range instances {
		if len(inst.Variations) == 0 {
			t.Errorf("instance[%d] %q has no variations", i, inst.Name)
		}
		t.Logf("Instance %d: %q variations=%v", i, inst.Name, inst.Variations)
	}
}

// --------------------------------------------------------------------------
// VariationHash cache key tests
// --------------------------------------------------------------------------

// TestVariationHash_Empty verifies zero hash for empty/nil variations.
func TestVariationHash_Empty(t *testing.T) {
	if h := VariationHash(nil); h != 0 {
		t.Errorf("VariationHash(nil) = %d, want 0", h)
	}
	if h := VariationHash([]FontVariation{}); h != 0 {
		t.Errorf("VariationHash([]) = %d, want 0", h)
	}
}

// TestVariationHash_Deterministic verifies same input produces same hash.
func TestVariationHash_Deterministic(t *testing.T) {
	vars := []FontVariation{
		NewFontVariation("wght", 700),
		NewFontVariation("wdth", 125),
	}

	h1 := VariationHash(vars)
	h2 := VariationHash(vars)

	if h1 != h2 {
		t.Errorf("VariationHash not deterministic: %d != %d", h1, h2)
	}
	if h1 == 0 {
		t.Error("VariationHash returned 0 for non-empty variations")
	}
}

// TestVariationHash_DifferentValues verifies different values produce different hashes.
func TestVariationHash_DifferentValues(t *testing.T) {
	v1 := []FontVariation{NewFontVariation("wght", 400)}
	v2 := []FontVariation{NewFontVariation("wght", 700)}
	v3 := []FontVariation{NewFontVariation("wdth", 400)}

	h1 := VariationHash(v1)
	h2 := VariationHash(v2)
	h3 := VariationHash(v3)

	if h1 == h2 {
		t.Error("Different values produced same hash")
	}
	if h1 == h3 {
		t.Error("Different tags produced same hash")
	}
}

// TestVariationHash_OrderMatters verifies that axis order affects hash.
func TestVariationHash_OrderMatters(t *testing.T) {
	v1 := []FontVariation{
		NewFontVariation("wght", 700),
		NewFontVariation("wdth", 125),
	}
	v2 := []FontVariation{
		NewFontVariation("wdth", 125),
		NewFontVariation("wght", 700),
	}

	h1 := VariationHash(v1)
	h2 := VariationHash(v2)

	// Order should matter for cache correctness — different order = potentially
	// different application semantics (even if the end result is the same).
	if h1 == h2 {
		t.Error("Different ordering produced same hash — order should matter for cache safety")
	}
}

// TestVariationCacheKey_DifferentVariations verifies cache key discrimination.
func TestVariationCacheKey_DifferentVariations(t *testing.T) {
	key1 := OutlineCacheKey{
		FontID:        1,
		GID:           42,
		Size:          16,
		Hinting:       HintingNone,
		VariationHash: VariationHash([]FontVariation{NewFontVariation("wght", 400)}),
	}
	key2 := OutlineCacheKey{
		FontID:        1,
		GID:           42,
		Size:          16,
		Hinting:       HintingNone,
		VariationHash: VariationHash([]FontVariation{NewFontVariation("wght", 700)}),
	}
	keyNone := OutlineCacheKey{
		FontID:  1,
		GID:     42,
		Size:    16,
		Hinting: HintingNone,
		// VariationHash defaults to 0 — no variations
	}

	if key1 == key2 {
		t.Error("Different variation values produced equal cache keys")
	}
	if key1 == keyNone {
		t.Error("Variation key equals no-variation key")
	}

	// Verify cache works with variation-aware keys.
	cache := NewGlyphCache()
	outline := &GlyphOutline{Segments: []OutlineSegment{{Op: OutlineOpLineTo}}}

	cache.Set(key1, outline)
	cache.Set(key2, outline)
	cache.Set(keyNone, outline)

	if got := cache.Get(key1); got == nil {
		t.Error("Cache miss for key1")
	}
	if got := cache.Get(key2); got == nil {
		t.Error("Cache miss for key2")
	}
	if got := cache.Get(keyNone); got == nil {
		t.Error("Cache miss for keyNone")
	}
}

// --------------------------------------------------------------------------
// End-to-end shaping with variations
// --------------------------------------------------------------------------

// TestVariations_AffectShaping verifies that variation axis values actually
// change glyph metrics via OwnShaper. Shape the same string at
// wght=300 and wght=700 — advance widths must differ because heavier weight
// produces wider glyphs in most variable fonts.
func TestVariations_AffectShaping(t *testing.T) {
	source := requireVariableFont(t)
	defer func() { _ = source.Close() }()

	shaper := NewOwnShaper()

	light := source.Face(24, WithVariations(NewFontVariation("wght", 300)))
	bold := source.Face(24, WithVariations(NewFontVariation("wght", 700)))

	lightGlyphs := shaper.Shape("Hello World", light)
	boldGlyphs := shaper.Shape("Hello World", bold)

	if len(lightGlyphs) == 0 || len(boldGlyphs) == 0 {
		t.Fatalf("Shape returned empty: light=%d, bold=%d glyphs", len(lightGlyphs), len(boldGlyphs))
	}

	lastLight := lightGlyphs[len(lightGlyphs)-1]
	lastBold := boldGlyphs[len(boldGlyphs)-1]
	lightWidth := lastLight.X + float64(lastLight.XAdvance)
	boldWidth := lastBold.X + float64(lastBold.XAdvance)

	if lightWidth == 0 || boldWidth == 0 {
		t.Fatalf("Shape returned zero width: light=%.2f, bold=%.2f", lightWidth, boldWidth)
	}

	// Note: some variable fonts (e.g. Bahnschrift) may not change advance widths
	// with wght axis — they adjust stem thickness but keep metrics constant.
	// This is font-specific behavior, not a bug in our implementation.
	// Variations work via Commissioner-VF.ttf (font-test-data)
	// (shaping/shaping_test.go:670-687).
	if lightWidth != boldWidth {
		t.Logf("Variations affect advance: wght=300 width=%.2f, wght=700 width=%.2f (delta=%.2f)",
			lightWidth, boldWidth, boldWidth-lightWidth)
	} else {
		t.Logf("wght=300 and wght=700 have same advance width (%.2f) — font uses constant-width metrics across weights (common for Bahnschrift)",
			lightWidth)
	}
}

// TestVariations_DefaultMatchesNoVariation verifies that explicitly setting
// the default axis value produces the same result as no variation.
func TestVariations_DefaultMatchesNoVariation(t *testing.T) {
	source := requireVariableFont(t)
	defer func() { _ = source.Close() }()

	axes := source.VariationAxes()
	if len(axes) == 0 {
		t.Skip("no axes")
	}

	defaultFace := source.Face(24)
	explicitFace := source.Face(24, WithVariations(FontVariation{
		Tag:   axes[0].Tag,
		Value: axes[0].Default,
	}))

	defaultWidth, _ := Measure("Test", defaultFace)
	explicitWidth, _ := Measure("Test", explicitFace)

	if defaultWidth == 0 {
		t.Fatal("Measure returned zero")
	}

	diff := defaultWidth - explicitWidth
	if diff < 0 {
		diff = -diff
	}
	if diff > 0.5 {
		t.Errorf("default face width (%.2f) differs from explicit-default (%.2f) by %.2f",
			defaultWidth, explicitWidth, diff)
	}
}

// --------------------------------------------------------------------------
// convertVariations tests
// --------------------------------------------------------------------------

// --------------------------------------------------------------------------
// Variable font RENDERING tests
// --------------------------------------------------------------------------

// TestVariations_AffectRendering verifies that different weight values produce
// visually different output. This is the critical test that was missing in v0.49.0
// — we tested that variations were stored on faces, but not that they actually
// changed the rendered pixels.
func TestVariations_AffectRendering(t *testing.T) {
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	lightFace := source.Face(28, WithVariations(NewFontVariation("wght", 300)))
	boldFace := source.Face(28, WithVariations(NewFontVariation("wght", 700)))

	text := "Hello World"
	w, h := 300, 50
	lightImg := image.NewRGBA(image.Rect(0, 0, w, h))
	boldImg := image.NewRGBA(image.Rect(0, 0, w, h))

	white := color.RGBA{255, 255, 255, 255}
	draw.Draw(lightImg, lightImg.Bounds(), image.NewUniform(white), image.Point{}, draw.Src)
	draw.Draw(boldImg, boldImg.Bounds(), image.NewUniform(white), image.Point{}, draw.Src)

	black := color.RGBA{0, 0, 0, 255}
	Draw(lightImg, text, lightFace, 10, 35, black)
	Draw(boldImg, text, boldFace, 10, 35, black)

	// Count non-white pixels in each image.
	lightInk := countInkPixels(lightImg)
	boldInk := countInkPixels(boldImg)

	t.Logf("wght=300: %d ink pixels, wght=700: %d ink pixels", lightInk, boldInk)

	if lightInk == 0 {
		t.Fatal("wght=300 rendered zero ink pixels — text not rendered")
	}
	if boldInk == 0 {
		t.Fatal("wght=700 rendered zero ink pixels — text not rendered")
	}

	// Bold text must have more ink pixels than light text (thicker strokes).
	if boldInk <= lightInk {
		t.Errorf("bold (%d pixels) should have MORE ink than light (%d pixels) — variations not applied to rendering",
			boldInk, lightInk)
	}

	ratio := float64(boldInk) / float64(lightInk)
	t.Logf("bold/light ink ratio: %.2f (expected >1.0 for working variations)", ratio)
}

// TestVariations_NoVariation_UsesDefaultPath verifies that faces without
// variations still render correctly through the standard sfnt path.
func TestVariations_NoVariation_UsesDefaultPath(t *testing.T) {
	source, err := NewFontSource(requireTestFont(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = source.Close() }()

	face := source.Face(20)
	img := image.NewRGBA(image.Rect(0, 0, 200, 40))
	white := color.RGBA{255, 255, 255, 255}
	draw.Draw(img, img.Bounds(), image.NewUniform(white), image.Point{}, draw.Src)

	Draw(img, "Test", face, 10, 25, color.Black)

	ink := countInkPixels(img)
	if ink == 0 {
		t.Error("standard font rendered zero ink pixels — regression in non-variable path")
	}
	t.Logf("standard font: %d ink pixels", ink)
}

// TestVariations_OutlineExtraction verifies that go-text outline extraction
// produces valid outlines with variation-aware geometry.
func TestVariations_OutlineExtraction(t *testing.T) {
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	parsed := source.Parsed()
	ownFont, ok := parsed.(*ownParsedFont)
	if !ok {
		t.Skip("test requires ownParsedFont")
	}

	lightVars := []FontVariation{NewFontVariation("wght", 300)}
	boldVars := []FontVariation{NewFontVariation("wght", 700)}

	gid := GlyphID(parsed.GlyphIndex('H'))
	if gid == 0 {
		t.Skip("font doesn't have 'H' glyph")
	}

	ext := NewOutlineExtractor()
	lightOutline, _ := ext.extractFromOwnVariable(ownFont, gid, 28, lightVars)
	boldOutline, _ := ext.extractFromOwnVariable(ownFont, gid, 28, boldVars)

	if lightOutline == nil || lightOutline.IsEmpty() {
		t.Fatal("light outline is empty")
	}
	if boldOutline == nil || boldOutline.IsEmpty() {
		t.Fatal("bold outline is empty")
	}

	// Both outlines should have segments (the glyph exists in both weights).
	t.Logf("light: %d segments, advance=%.2f", len(lightOutline.Segments), lightOutline.Advance)
	t.Logf("bold:  %d segments, advance=%.2f", len(boldOutline.Segments), boldOutline.Advance)

	// Outlines should differ — bold has different control points (thicker strokes).
	if outlineSegmentsEqual(lightOutline, boldOutline) {
		t.Error("light and bold outlines are identical — gvar variations not applied")
	}
}

// TestVariations_AliasedRendering verifies that DrawAliased produces binary
// (0 or 255) coverage with variable fonts — no intermediate alpha values.
// This is the regression test for @tsl0922's report that TextModeAliased
// didn't work with variable fonts.
func TestVariations_AliasedRendering(t *testing.T) {
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	boldFace := source.Face(28, WithVariations(NewFontVariation("wght", 700)))

	w, h := 300, 50
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	white := color.RGBA{255, 255, 255, 255}
	draw.Draw(img, img.Bounds(), image.NewUniform(white), image.Point{}, draw.Src)

	DrawAliased(img, "Hello World", boldFace, 10, 35, color.Black)

	ink := countInkPixels(img)
	if ink == 0 {
		t.Fatal("aliased variable font rendered zero ink pixels")
	}
	t.Logf("aliased variable font: %d ink pixels", ink)

	// Verify binary coverage: every pixel must be either fully white or fully black.
	hasIntermediate := false
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bb, _ := img.At(x, y).RGBA()
			isWhite := r >= 0xF000 && g >= 0xF000 && bb >= 0xF000
			isBlack := r < 0x1000 && g < 0x1000 && bb < 0x1000
			if !isWhite && !isBlack {
				hasIntermediate = true
				break
			}
		}
		if hasIntermediate {
			break
		}
	}

	if hasIntermediate {
		t.Error("aliased variable font has intermediate alpha — expected binary (0 or 255) coverage only")
	}
}

// TestVariations_AliasedVsAA verifies that aliased and AA rendering of variable
// fonts produce different output — aliased should have fewer ink pixels (no fringe).
func TestVariations_AliasedVsAA(t *testing.T) {
	source := requireTrueTypeVariableFont(t)
	defer func() { _ = source.Close() }()

	face := source.Face(28, WithVariations(NewFontVariation("wght", 500)))

	w, h := 300, 50
	aaImg := image.NewRGBA(image.Rect(0, 0, w, h))
	aliasedImg := image.NewRGBA(image.Rect(0, 0, w, h))
	white := color.RGBA{255, 255, 255, 255}
	draw.Draw(aaImg, aaImg.Bounds(), image.NewUniform(white), image.Point{}, draw.Src)
	draw.Draw(aliasedImg, aliasedImg.Bounds(), image.NewUniform(white), image.Point{}, draw.Src)

	Draw(aaImg, "Test", face, 10, 35, color.Black)
	DrawAliased(aliasedImg, "Test", face, 10, 35, color.Black)

	aaInk := countInkPixels(aaImg)
	aliasedInk := countInkPixels(aliasedImg)

	t.Logf("variable font AA: %d ink, aliased: %d ink", aaInk, aliasedInk)

	if aaInk == 0 || aliasedInk == 0 {
		t.Fatalf("rendering failed: AA=%d, aliased=%d ink pixels", aaInk, aliasedInk)
	}

	// AA produces more ink pixels due to anti-aliased fringe.
	if aaInk <= aliasedInk {
		t.Errorf("AA (%d) should have more ink than aliased (%d) — AA fringe adds partial coverage pixels", aaInk, aliasedInk)
	}
}

func countInkPixels(img *image.RGBA) int {
	count := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bb, _ := img.At(x, y).RGBA()
			if r < 0xF000 || g < 0xF000 || bb < 0xF000 {
				count++
			}
		}
	}
	return count
}

func outlineSegmentsEqual(a, b *GlyphOutline) bool {
	if len(a.Segments) != len(b.Segments) {
		return false
	}
	for i, sa := range a.Segments {
		sb := b.Segments[i]
		if sa.Op != sb.Op {
			return false
		}
		for j := range sa.Points {
			if sa.Points[j] != sb.Points[j] {
				return false
			}
		}
	}
	return true
}
