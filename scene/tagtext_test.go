package scene

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

// --- Tag tests ---

func TestTagText_Constants(t *testing.T) {
	if TagText != 0x60 {
		t.Errorf("TagText = 0x%02X, want 0x60", byte(TagText))
	}
}

func TestTagText_String(t *testing.T) {
	if TagText.String() != "Text" {
		t.Errorf("TagText.String() = %q, want %q", TagText.String(), "Text")
	}
}

func TestTagText_IsDrawCommand(t *testing.T) {
	if !TagText.IsDrawCommand() {
		t.Error("TagText.IsDrawCommand() = false, want true")
	}
}

func TestTagText_IsNotPathCommand(t *testing.T) {
	if TagText.IsPathCommand() {
		t.Error("TagText.IsPathCommand() = true, want false")
	}
}

func TestTagText_DataSize(t *testing.T) {
	if TagText.DataSize() != 0 {
		t.Errorf("TagText.DataSize() = %d, want 0 (text uses textData stream, not pathData)", TagText.DataSize())
	}
}

// --- Encoding round-trip tests ---

func TestEncodeText_RoundTrip(t *testing.T) {
	enc := NewEncoding()

	run := GlyphRunData{
		FontSourceID: 12345,
		FontSize:     14.0,
		GlyphCount:   3,
		Flags:        TextFlagHinting,
		OriginX:      10.5,
		OriginY:      20.5,
		BrushIndex:   0,
		TextLen:      3,
	}
	glyphs := []GlyphEntry{
		{GlyphID: 65, X: 0.0, Y: 0.0},
		{GlyphID: 66, X: 7.5, Y: 0.0},
		{GlyphID: 67, X: 15.0, Y: 0.0},
	}
	brush := SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1})
	enc.brushes = append(enc.brushes, brush)
	enc.EncodeText(run, glyphs, "ABC")

	if len(enc.Tags()) != 1 {
		t.Fatalf("Tags len = %d, want 1", len(enc.Tags()))
	}
	if enc.Tags()[0] != TagText {
		t.Errorf("Tag = 0x%02X, want TagText (0x60)", byte(enc.Tags()[0]))
	}
	if enc.ShapeCount() != 1 {
		t.Errorf("ShapeCount = %d, want 1", enc.ShapeCount())
	}

	expectedTextDataSize := glyphRunDataSize + 3*glyphEntrySize + 3
	if len(enc.TextData()) != expectedTextDataSize {
		t.Errorf("TextData len = %d, want %d", len(enc.TextData()), expectedTextDataSize)
	}

	// Decode
	dec := NewDecoder(enc)
	if !dec.Next() {
		t.Fatal("Decoder.Next() = false, want true")
	}
	if dec.Tag() != TagText {
		t.Fatalf("Decoder.Tag() = 0x%02X, want TagText", byte(dec.Tag()))
	}

	gotRun, gotGlyphs, gotStr, gotBrush := dec.Text()
	if gotRun.FontSourceID != 12345 {
		t.Errorf("FontSourceID = %d, want 12345", gotRun.FontSourceID)
	}
	if gotRun.FontSize != 14.0 {
		t.Errorf("FontSize = %f, want 14.0", gotRun.FontSize)
	}
	if gotRun.GlyphCount != 3 {
		t.Errorf("GlyphCount = %d, want 3", gotRun.GlyphCount)
	}
	if gotRun.Flags != TextFlagHinting {
		t.Errorf("Flags = %d, want TextFlagHinting", gotRun.Flags)
	}
	if gotRun.OriginX != 10.5 {
		t.Errorf("OriginX = %f, want 10.5", gotRun.OriginX)
	}
	if gotRun.OriginY != 20.5 {
		t.Errorf("OriginY = %f, want 20.5", gotRun.OriginY)
	}
	if gotStr != "ABC" {
		t.Errorf("str = %q, want %q", gotStr, "ABC")
	}
	if gotBrush.Kind != BrushSolid || gotBrush.Color.R != 1 {
		t.Errorf("brush = %+v, want solid red", gotBrush)
	}
	if len(gotGlyphs) != 3 {
		t.Fatalf("glyphs len = %d, want 3", len(gotGlyphs))
	}
	if gotGlyphs[0].GlyphID != 65 {
		t.Errorf("glyph[0].GlyphID = %d, want 65", gotGlyphs[0].GlyphID)
	}
	if gotGlyphs[1].X != 7.5 {
		t.Errorf("glyph[1].X = %f, want 7.5", gotGlyphs[1].X)
	}
	if gotGlyphs[2].GlyphID != 67 {
		t.Errorf("glyph[2].GlyphID = %d, want 67", gotGlyphs[2].GlyphID)
	}
}

func TestEncodeText_EmptyString(t *testing.T) {
	enc := NewEncoding()
	run := GlyphRunData{GlyphCount: 1, TextLen: 0}
	glyphs := []GlyphEntry{{GlyphID: 65, X: 0, Y: 0}}
	enc.EncodeText(run, glyphs, "")

	dec := NewDecoder(enc)
	dec.Next()
	gotRun, gotGlyphs, gotStr, _ := dec.Text()
	if gotRun.TextLen != 0 {
		t.Errorf("TextLen = %d, want 0", gotRun.TextLen)
	}
	if gotStr != "" {
		t.Errorf("str = %q, want empty", gotStr)
	}
	if len(gotGlyphs) != 1 {
		t.Errorf("glyphs len = %d, want 1", len(gotGlyphs))
	}
}

func TestEncodeText_MultipleRuns(t *testing.T) {
	enc := NewEncoding()
	brush := SolidBrush(gg.RGBA{R: 1, G: 1, B: 1, A: 1})
	enc.brushes = append(enc.brushes, brush)

	// First run: "Hi"
	run1 := GlyphRunData{FontSourceID: 100, FontSize: 12, GlyphCount: 2, TextLen: 2, BrushIndex: 0}
	enc.EncodeText(run1, []GlyphEntry{{GlyphID: 1, X: 0, Y: 0}, {GlyphID: 2, X: 5, Y: 0}}, "Hi")

	// Second run: "!"
	enc.brushes = append(enc.brushes, brush)
	run2 := GlyphRunData{FontSourceID: 200, FontSize: 18, GlyphCount: 1, TextLen: 1, BrushIndex: 1}
	enc.EncodeText(run2, []GlyphEntry{{GlyphID: 3, X: 0, Y: 0}}, "!")

	if enc.ShapeCount() != 2 {
		t.Errorf("ShapeCount = %d, want 2", enc.ShapeCount())
	}

	dec := NewDecoder(enc)

	// Decode first run
	dec.Next()
	r1, g1, s1, _ := dec.Text()
	if r1.FontSourceID != 100 || r1.FontSize != 12 || len(g1) != 2 || s1 != "Hi" {
		t.Errorf("run1: id=%d size=%f glyphs=%d str=%q", r1.FontSourceID, r1.FontSize, len(g1), s1)
	}

	// Decode second run
	dec.Next()
	r2, g2, s2, _ := dec.Text()
	if r2.FontSourceID != 200 || r2.FontSize != 18 || len(g2) != 1 || s2 != "!" {
		t.Errorf("run2: id=%d size=%f glyphs=%d str=%q", r2.FontSourceID, r2.FontSize, len(g2), s2)
	}
}

// --- Encoding operations with TagText ---

func TestEncoding_Reset_ClearsTextData(t *testing.T) {
	enc := NewEncoding()
	run := GlyphRunData{GlyphCount: 1, TextLen: 1}
	enc.EncodeText(run, []GlyphEntry{{GlyphID: 1}}, "A")

	if len(enc.textData) == 0 {
		t.Fatal("textData should not be empty before reset")
	}

	enc.Reset()
	if len(enc.textData) != 0 {
		t.Errorf("textData len after reset = %d, want 0", len(enc.textData))
	}
}

func TestEncoding_Clone_CopiesTextData(t *testing.T) {
	enc := NewEncoding()
	run := GlyphRunData{GlyphCount: 1, TextLen: 1, FontSize: 14}
	enc.EncodeText(run, []GlyphEntry{{GlyphID: 42, X: 1.5, Y: 2.5}}, "X")

	clone := enc.Clone()
	if len(clone.textData) != len(enc.textData) {
		t.Fatalf("clone textData len = %d, want %d", len(clone.textData), len(enc.textData))
	}

	// Verify independence — mutate original, clone should not change.
	origLen := len(enc.textData)
	enc.Reset()
	if len(clone.textData) != origLen {
		t.Errorf("clone textData changed after original reset: len=%d, want %d", len(clone.textData), origLen)
	}

	// Verify decode from clone.
	dec := NewDecoder(clone)
	dec.Next()
	r, g, s, _ := dec.Text()
	if r.FontSize != 14 || len(g) != 1 || g[0].GlyphID != 42 || s != "X" {
		t.Errorf("clone decode: size=%f glyphs=%d gid=%d str=%q", r.FontSize, len(g), g[0].GlyphID, s)
	}
}

func TestEncoding_Hash_IncludesTextData(t *testing.T) {
	enc1 := NewEncoding()
	enc2 := NewEncoding()

	run := GlyphRunData{GlyphCount: 1, TextLen: 1, FontSize: 12}
	enc1.EncodeText(run, []GlyphEntry{{GlyphID: 1}}, "A")
	enc2.EncodeText(run, []GlyphEntry{{GlyphID: 2}}, "B")

	if enc1.Hash() == enc2.Hash() {
		t.Error("different textData should produce different hashes")
	}
}

func TestEncoding_Size_IncludesTextData(t *testing.T) {
	enc := NewEncoding()
	sizeBefore := enc.Size()

	run := GlyphRunData{GlyphCount: 1, TextLen: 5}
	enc.EncodeText(run, []GlyphEntry{{GlyphID: 1}}, "Hello")

	sizeAfter := enc.Size()
	expectedDelta := glyphRunDataSize + glyphEntrySize + 5 + 1 // +1 for tag byte
	if sizeAfter-sizeBefore != expectedDelta {
		t.Errorf("size delta = %d, want %d", sizeAfter-sizeBefore, expectedDelta)
	}
}

// --- Scene.DrawText tests ---

func TestScene_DrawText_NilFace(t *testing.T) {
	s := NewScene()
	err := s.DrawText("hello", nil, 0, 0, SolidBrush(gg.Black))
	if err == nil {
		t.Error("DrawText with nil face should return error")
	}
}

func TestScene_DrawGlyphs_Empty(t *testing.T) {
	s := NewScene()
	err := s.DrawGlyphs("", nil, nil, 0, 0, SolidBrush(gg.Black))
	if err != nil {
		t.Errorf("DrawGlyphs with empty glyphs should not error, got %v", err)
	}
	if !s.IsEmpty() {
		t.Error("scene should be empty after DrawGlyphs with no glyphs")
	}
}

func TestScene_DrawGlyphs_NilFace(t *testing.T) {
	s := NewScene()
	glyphs := []text.ShapedGlyph{{GID: 1, X: 0, Y: 0}}
	err := s.DrawGlyphs("A", glyphs, nil, 0, 0, SolidBrush(gg.Black))
	if err == nil {
		t.Error("DrawGlyphs with nil face should return error")
	}
}

// --- Font registry tests ---

func TestScene_FontRegistry_Empty(t *testing.T) {
	s := NewScene()
	if len(s.FontRegistry()) != 0 {
		t.Errorf("new scene font registry len = %d, want 0", len(s.FontRegistry()))
	}
}

func TestScene_FontRegistry_Reset(t *testing.T) {
	s := NewScene()
	s.fontRegistry[12345] = nil // simulate registration
	s.Reset()
	if len(s.FontRegistry()) != 0 {
		t.Errorf("font registry len after reset = %d, want 0", len(s.FontRegistry()))
	}
}

func TestScene_FontRegistry_Append(t *testing.T) {
	s1 := NewScene()
	s2 := NewScene()

	s1.fontRegistry[111] = nil
	s2.fontRegistry[222] = nil
	// Scenes must have content for Append to merge (empty scenes are skipped).
	rect := NewRectShape(0, 0, 10, 10)
	s1.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Black), rect)
	s2.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Black), rect)

	s1.Append(s2)
	if len(s1.FontRegistry()) != 2 {
		t.Errorf("merged font registry len = %d, want 2", len(s1.FontRegistry()))
	}
}

func TestScene_FontRegistry_AppendWithTranslation(t *testing.T) {
	s1 := NewScene()
	s2 := NewScene()

	s1.fontRegistry[111] = nil
	s2.fontRegistry[222] = nil
	rect := NewRectShape(0, 0, 10, 10)
	s1.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Black), rect)
	s2.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Black), rect)

	s1.AppendWithTranslation(s2, 10, 20)
	if len(s1.FontRegistry()) != 2 {
		t.Errorf("merged font registry len = %d, want 2", len(s1.FontRegistry()))
	}
}

// --- GlyphRunData binary layout tests ---

func TestGlyphRunDataSize(t *testing.T) {
	if glyphRunDataSize != 30 {
		t.Errorf("glyphRunDataSize = %d, want 30", glyphRunDataSize)
	}
}

func TestGlyphEntrySize(t *testing.T) {
	if glyphEntrySize != 10 {
		t.Errorf("glyphEntrySize = %d, want 10", glyphEntrySize)
	}
}

func TestTextFlags(t *testing.T) {
	if TextFlagHinting != 1 {
		t.Errorf("TextFlagHinting = %d, want 1", TextFlagHinting)
	}
}

// --- Append tests with TagText ---

func TestEncoding_AppendWithImages_TagText_BrushOffset(t *testing.T) {
	enc1 := NewEncoding()
	enc1.brushes = append(enc1.brushes, SolidBrush(gg.RGBA{R: 1}), SolidBrush(gg.RGBA{G: 1}))

	enc2 := NewEncoding()
	enc2.brushes = append(enc2.brushes, SolidBrush(gg.RGBA{B: 1}))
	run := GlyphRunData{GlyphCount: 1, TextLen: 1, BrushIndex: 0, FontSize: 12}
	enc2.EncodeText(run, []GlyphEntry{{GlyphID: 42}}, "X")

	enc1.AppendWithImages(enc2, 0)

	dec := NewDecoder(enc1)
	dec.Next()
	gotRun, _, _, gotBrush := dec.Text()
	// enc1 had 2 brushes before append → brush 0 in enc2 → brush 2 in enc1
	if gotRun.BrushIndex != 2 {
		t.Errorf("BrushIndex = %d, want 2 (offset by 2)", gotRun.BrushIndex)
	}
	if gotBrush.Color.B != 1 {
		t.Errorf("Brush color B = %f, want 1 (blue)", gotBrush.Color.B)
	}
}

func TestEncoding_AppendWithTranslation_TagText_OriginOffset(t *testing.T) {
	enc1 := NewEncoding()
	enc2 := NewEncoding()

	enc2.brushes = append(enc2.brushes, SolidBrush(gg.White))
	run := GlyphRunData{
		GlyphCount: 1, TextLen: 2, FontSize: 14,
		OriginX: 10, OriginY: 20, BrushIndex: 0,
	}
	enc2.EncodeText(run, []GlyphEntry{{GlyphID: 1, X: 0, Y: 0}}, "AB")

	enc1.AppendWithTranslation(enc2, 100, 200, 0)

	dec := NewDecoder(enc1)
	dec.Next()
	gotRun, gotGlyphs, gotStr, _ := dec.Text()

	if gotRun.OriginX != 110 {
		t.Errorf("OriginX = %f, want 110 (10 + 100)", gotRun.OriginX)
	}
	if gotRun.OriginY != 220 {
		t.Errorf("OriginY = %f, want 220 (20 + 200)", gotRun.OriginY)
	}
	if gotStr != "AB" {
		t.Errorf("str = %q, want %q", gotStr, "AB")
	}
	if len(gotGlyphs) != 1 || gotGlyphs[0].GlyphID != 1 {
		t.Errorf("glyphs not preserved: %+v", gotGlyphs)
	}
}

func TestEncoding_AppendWithTranslation_TagText_Mixed(t *testing.T) {
	enc1 := NewEncoding()
	enc2 := NewEncoding()

	// enc2 has a fill + text
	enc2.encodeMoveTo(5, 5)
	enc2.encodeLineTo(10, 10)
	enc2.brushes = append(enc2.brushes, SolidBrush(gg.Red), SolidBrush(gg.Blue))
	run := GlyphRunData{GlyphCount: 1, TextLen: 1, FontSize: 12, OriginX: 0, OriginY: 0, BrushIndex: 1}
	enc2.EncodeText(run, []GlyphEntry{{GlyphID: 65}}, "A")

	enc1.AppendWithTranslation(enc2, 50, 50, 0)

	// Verify path data was offset.
	dec := NewDecoder(enc1)
	dec.Next() // MoveTo
	x, y := dec.MoveTo()
	if x != 55 || y != 55 {
		t.Errorf("MoveTo = (%f, %f), want (55, 55)", x, y)
	}

	dec.Next() // LineTo
	x, y = dec.LineTo()
	if x != 60 || y != 60 {
		t.Errorf("LineTo = (%f, %f), want (60, 60)", x, y)
	}

	// Find TagText
	dec.Next()                    // TagText
	gotRun, _, _, _ := dec.Text() //nolint:dogsled // only need run header for origin check
	if gotRun.OriginX != 50 {
		t.Errorf("text OriginX = %f, want 50", gotRun.OriginX)
	}
	if gotRun.OriginY != 50 {
		t.Errorf("text OriginY = %f, want 50", gotRun.OriginY)
	}
}

// --- Benchmarks ---

func BenchmarkEncodeText_10Glyphs(b *testing.B) {
	enc := NewEncoding()
	enc.brushes = append(enc.brushes, SolidBrush(gg.White))
	glyphs := make([]GlyphEntry, 10)
	for i := range glyphs {
		glyphs[i] = GlyphEntry{GlyphID: text.GlyphID(65 + i), X: float32(i) * 7.5, Y: 0}
	}
	run := GlyphRunData{
		FontSourceID: 12345, FontSize: 14, GlyphCount: 10,
		Flags: TextFlagHinting, OriginX: 0, OriginY: 0,
		BrushIndex: 0, TextLen: 10,
	}
	str := "Hello Wrld"

	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		enc.tags = enc.tags[:0]
		enc.textData = enc.textData[:0]
		enc.EncodeText(run, glyphs, str)
	}
}

func BenchmarkDecodeText_10Glyphs(b *testing.B) {
	enc := NewEncoding()
	enc.brushes = append(enc.brushes, SolidBrush(gg.White))
	glyphs := make([]GlyphEntry, 10)
	for i := range glyphs {
		glyphs[i] = GlyphEntry{GlyphID: text.GlyphID(65 + i), X: float32(i) * 7.5, Y: 0}
	}
	run := GlyphRunData{
		FontSourceID: 12345, FontSize: 14, GlyphCount: 10,
		Flags: TextFlagHinting, OriginX: 10, OriginY: 20,
		BrushIndex: 0, TextLen: 10,
	}
	enc.EncodeText(run, glyphs, "Hello Wrld")

	dec := NewDecoder(enc)
	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		dec.tagIdx = 0
		dec.textIdx = 0
		dec.Next()
		dec.Text()
	}
}
