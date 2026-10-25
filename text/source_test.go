package text

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// testFontPath returns the path to a test font.
// For now, we'll skip tests if no font is available.
// Note: TTC (font collections) are not supported by golang.org/x/image.
func testFontPath(t *testing.T) string {
	t.Helper()

	// Only TTF files are supported (not TTC font collections)
	// macOS system fonts are mostly TTC, so we look for TTF alternatives
	candidates := []string{
		// Windows
		"C:\\Windows\\Fonts\\arial.ttf",
		"C:\\Windows\\Fonts\\calibri.ttf",
		// macOS - Supplemental fonts are TTF
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Courier New.ttf",
		"/System/Library/Fonts/Supplemental/Times New Roman.ttf",
		"/System/Library/Fonts/Supplemental/Verdana.ttf",
		"/System/Library/Fonts/Monaco.ttf",
		// Linux
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Check testdata directory
	testdataFont := filepath.Join("testdata", "test.ttf")
	if _, err := os.Stat(testdataFont); err == nil {
		return testdataFont
	}

	t.Skip("No TTF font available (TTC collections not supported)")
	return ""
}

func TestNewFontSource(t *testing.T) {
	fontPath := testFontPath(t)

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("failed to read font: %v", err)
	}

	source, err := NewFontSource(data)
	if err != nil {
		t.Fatalf("NewFontSource failed: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	if source == nil {
		t.Fatal("expected non-nil source")
	}

	if source.name == "" {
		t.Error("expected non-empty font name")
	}

	t.Logf("Font name: %s", source.name)
}

func TestNewFontSourceFromFile(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("NewFontSourceFromFile failed: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	if source == nil {
		t.Fatal("expected non-nil source")
	}

	if source.name == "" {
		t.Error("expected non-empty font name")
	}

	t.Logf("Font name: %s", source.name)
}

func TestFontSourceFace(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("NewFontSourceFromFile failed: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	// Create faces at different sizes
	sizes := []float64{12, 16, 24, 32, 48}
	for _, size := range sizes {
		face := source.Face(size)
		if face == nil {
			t.Errorf("Face(%v) returned nil", size)
		}
	}
}

func TestFontSourceCopyProtection(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("NewFontSourceFromFile failed: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	// Test copy protection
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when copying FontSource")
		} else {
			t.Logf("Copy protection panic (expected): %v", r)
		}
	}()

	// This should panic
	// We use a helper function to avoid govet copylocks warning
	testCopy(source)
}

// testCopy is a helper to test copy protection.
// Uses unsafe.Pointer to avoid go vet copylocks warning while still testing the mechanism.
func testCopy(source *FontSource) {
	// Create a copy by allocating new memory and copying bytes
	// This tests the copy protection mechanism without triggering copylocks
	var copySource FontSource
	copyBytes(source, &copySource)
	_ = copySource.Name() // Trigger copyCheck
}

// copyBytes copies the bytes from src to dst using unsafe.
// This is only used in tests to verify copy protection works.
//
//go:nocheckptr
func copyBytes(src, dst *FontSource) {
	// Use type assertion to copy fields manually (avoids unsafe)
	// The addr field will be wrong after copy, which is what we're testing
	dst.addr = src.addr // Will be wrong after copy!
	dst.data = src.data
	dst.parsed = src.parsed
	dst.name = src.name
	dst.config = src.config
	// Note: mu (sync.RWMutex) has a zero value that works
}

func TestFontSourceName(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("NewFontSourceFromFile failed: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	name := source.Name()
	if name == "" {
		t.Error("expected non-empty font name")
	}

	t.Logf("Font name: %s", name)
}

func TestFontSourceClose(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("NewFontSourceFromFile failed: %v", err)
	}

	err = source.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// After close, data should be nil
	if source.data != nil {
		t.Error("expected data to be nil after Close()")
	}
}

func TestNewFontSourceWithOptions(t *testing.T) {
	fontPath := testFontPath(t)

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("failed to read font: %v", err)
	}

	source, err := NewFontSource(data, WithCacheLimit(1024))
	if err != nil {
		t.Fatalf("NewFontSource failed: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	if source.config.cacheLimit != 1024 {
		t.Errorf("expected cache limit 1024, got %d", source.config.cacheLimit)
	}
}

func TestFaceWithOptions(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("NewFontSourceFromFile failed: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	// Test face with options
	face := source.Face(24,
		WithDirection(DirectionRTL),
		WithHinting(HintingNone),
		WithLanguage("ar"),
	)

	if face == nil {
		t.Error("expected non-nil face")
	}

	// Verify options were applied (internal check)
	sf := face.(*sourceFace)
	if sf.config.direction != DirectionRTL {
		t.Errorf("expected DirectionRTL, got %v", sf.config.direction)
	}
	if sf.config.hinting != HintingNone {
		t.Errorf("expected HintingNone, got %v", sf.config.hinting)
	}
	if sf.config.language != "ar" {
		t.Errorf("expected language 'ar', got %s", sf.config.language)
	}
}

func TestNewFontSourceEmptyData(t *testing.T) {
	_, err := NewFontSource(nil)
	if err == nil {
		t.Error("expected error for nil data")
	}

	_, err = NewFontSource([]byte{})
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestNewFontSourceInvalidData(t *testing.T) {
	invalidData := []byte("not a font file")
	_, err := NewFontSource(invalidData)
	if err == nil {
		t.Error("expected error for invalid font data")
	}
}

func TestNewFontSourceWithParser(t *testing.T) {
	fontPath := testFontPath(t)

	data, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("failed to read font: %v", err)
	}

	// Test with explicit ximage parser
	source, err := NewFontSource(data, WithParser("ximage"))
	if err != nil {
		t.Fatalf("NewFontSource with parser failed: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	if source.name == "" {
		t.Error("expected non-empty font name")
	}

	// Verify Parsed() returns a valid ParsedFont
	parsed := source.Parsed()
	if parsed == nil {
		t.Fatal("expected non-nil parsed font")
	}

	// Test ParsedFont interface methods
	if parsed.Name() == "" {
		t.Error("expected non-empty name from ParsedFont")
	}

	if parsed.NumGlyphs() <= 0 {
		t.Error("expected positive number of glyphs")
	}

	if parsed.UnitsPerEm() <= 0 {
		t.Error("expected positive units per em")
	}

	// Test glyph index for 'A'
	idx := parsed.GlyphIndex('A')
	if idx == 0 {
		t.Error("expected non-zero glyph index for 'A'")
	}

	// Test glyph advance
	advance := parsed.GlyphAdvance(idx, 24)
	if advance <= 0 {
		t.Error("expected positive advance width")
	}

	// Test font metrics
	metrics := parsed.Metrics(24)
	if metrics.Ascent <= 0 {
		t.Error("expected positive ascent")
	}

	t.Logf("Font: %s, Glyphs: %d, UnitsPerEm: %d", parsed.Name(), parsed.NumGlyphs(), parsed.UnitsPerEm())
	t.Logf("Glyph 'A' index: %d, advance at 24pt: %.2f", idx, advance)
	t.Logf("Metrics at 24pt: Ascent=%.2f, Descent=%.2f, Height=%.2f", metrics.Ascent, metrics.Descent, metrics.Height())
}

// NewFontSource documents that data may be reused after the call. The parsed
// font must therefore read the source's own copy, not the caller's slice.
func TestNewFontSourceDoesNotAliasCallerData(t *testing.T) {
	data := append([]byte(nil), requireTestFont(t)...)
	source, err := NewFontSource(data)
	if err != nil {
		t.Fatalf("NewFontSource failed: %v", err)
	}
	defer func() { _ = source.Close() }()

	gid := source.Parsed().GlyphIndex('A')
	if gid == 0 {
		t.Fatal("expected a glyph for 'A'")
	}
	before, err := NewOutlineExtractor().ExtractOutline(source.Parsed(), GlyphID(gid), 24)
	if err != nil || before == nil || len(before.Segments) == 0 {
		t.Fatalf("expected an outline for 'A' before reuse: %v", err)
	}

	clear(data) // the caller reuses its buffer, as the doc comment allows

	if got := source.Parsed().GlyphIndex('A'); got != gid {
		t.Errorf("GlyphIndex('A') after the caller reused its buffer = %d, want %d", got, gid)
	}
	after, err := NewOutlineExtractor().ExtractOutline(source.Parsed(), GlyphID(gid), 24)
	if err != nil || after == nil || len(after.Segments) != len(before.Segments) {
		t.Errorf("outline for 'A' changed after the caller reused its buffer: %v", err)
	}
}

// NewFontSourceFromFile owns the bytes it reads, so it hands them to the
// source instead of copying them: one copy of the file, not two. For a 183 MB
// emoji font the second copy was 183 MB of heap held for the font's lifetime.
func TestNewFontSourceFromFileKeepsOneCopy(t *testing.T) {
	data := requireTestFont(t)
	path := filepath.Join(t.TempDir(), "font.ttf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFontSourceFromFile(path); err != nil { // parser registry and other one-time setup
		t.Fatalf("NewFontSourceFromFile failed: %v", err)
	}

	const loads = 8
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range loads {
		if _, err := NewFontSourceFromFile(path); err != nil {
			t.Fatalf("NewFontSourceFromFile failed: %v", err)
		}
	}
	runtime.ReadMemStats(&after)

	perLoad := (after.TotalAlloc - before.TotalAlloc) / loads
	if limit := uint64(len(data)) * 3 / 2; perLoad > limit {
		t.Errorf("NewFontSourceFromFile allocated %d bytes for a %d-byte file, want under %d (one copy and the parse)",
			perLoad, len(data), limit)
	}
	t.Logf("%d bytes allocated per load of a %d-byte file", perLoad, len(data))
}

func TestNewFontSourceFromFileEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.ttf")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFontSourceFromFile(path); !errors.Is(err, ErrEmptyFontData) {
		t.Errorf("NewFontSourceFromFile(empty file) error = %v, want ErrEmptyFontData", err)
	}
}
