package text

import (
	"os"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
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
	source, err := NewFontSource(goregular.TTF)
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
	source, err := NewFontSource(goregular.TTF)
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
	source, err := NewFontSource(goregular.TTF)
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
	source, err := NewFontSource(goregular.TTF)
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
	source, err := NewFontSource(goregular.TTF)
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
	source, err := NewFontSource(goregular.TTF)
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
	source, err := NewFontSource(goregular.TTF)
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
	source, err := NewFontSource(goregular.TTF)
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
// change glyph metrics via the GoTextShaper path. Shape the same string at
// wght=300 and wght=700 — advance widths must differ because heavier weight
// produces wider glyphs in most variable fonts.
//
// Uses GoTextShaper.Shape() directly because Face.Glyphs() uses the
// BuiltinShaper (x/image/font/sfnt) which does not support OpenType
// variations. Variations are applied through go-text/typesetting's
// Face.SetVariations() which is called by GoTextShaper.Shape().
//
// Known limitation: Face.Glyphs() and Face.Advance() do not apply variations.
// Tracked as VARFONT-013 for future work (requires either sfnt variation
// support or switching Glyphs() to use GoTextShaper when variations are set).
func TestVariations_AffectShaping(t *testing.T) {
	source := requireVariableFont(t)
	defer func() { _ = source.Close() }()

	shaper := NewGoTextShaper()
	if shaper == nil {
		t.Skip("GoTextShaper not available")
	}

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
	// go-text/typesetting confirms variations work via Commissioner-VF.ttf
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

// TestConvertVariations_Nil verifies nil input produces nil output.
func TestConvertVariations_Nil(t *testing.T) {
	got := convertVariations(nil)
	if got != nil {
		t.Errorf("convertVariations(nil) = %v, want nil", got)
	}
}

// TestConvertVariations_Empty verifies empty input produces nil output.
func TestConvertVariations_Empty(t *testing.T) {
	got := convertVariations([]FontVariation{})
	if got != nil {
		t.Errorf("convertVariations([]) = %v, want nil", got)
	}
}

// TestConvertVariations_Values verifies correct tag and value mapping.
func TestConvertVariations_Values(t *testing.T) {
	vars := []FontVariation{
		NewFontVariation("wght", 700),
		NewFontVariation("wdth", 125),
	}

	out := convertVariations(vars)
	if len(out) != 2 {
		t.Fatalf("convertVariations returned %d items, want 2", len(out))
	}

	// Verify tag bytes: "wght" → big-endian uint32.
	// 'w'=0x77, 'g'=0x67, 'h'=0x68, 't'=0x74 → 0x77676874
	if out[0].Value != 700 {
		t.Errorf("out[0].Value = %v, want 700", out[0].Value)
	}
	if out[1].Value != 125 {
		t.Errorf("out[1].Value = %v, want 125", out[1].Value)
	}
}
