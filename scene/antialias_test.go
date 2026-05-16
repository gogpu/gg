package scene

import (
	"testing"

	"github.com/gogpu/gg"
)

func TestSceneSetAntiAlias(t *testing.T) {
	s := NewScene()

	// Default: AA enabled.
	if !s.AntiAlias() {
		t.Error("default AntiAlias() should be true")
	}

	// Disable.
	s.SetAntiAlias(false)
	if s.AntiAlias() {
		t.Error("AntiAlias() should be false after SetAntiAlias(false)")
	}

	// Re-enable.
	s.SetAntiAlias(true)
	if !s.AntiAlias() {
		t.Error("AntiAlias() should be true after SetAntiAlias(true)")
	}
}

func TestSceneAntiAliasResetRestoresDefault(t *testing.T) {
	s := NewScene()
	s.SetAntiAlias(false)
	s.Reset()

	if !s.AntiAlias() {
		t.Error("AntiAlias() should be true after Reset()")
	}
}

func TestSceneAntiAliasEncodedInStream(t *testing.T) {
	s := NewScene()
	rect := NewRectShape(0, 0, 100, 100)
	brush := SolidBrush(gg.Red)

	// Draw with AA enabled (default) — no TagSetAntiAlias should be emitted.
	s.Fill(FillNonZero, IdentityAffine(), brush, rect)

	enc := s.Encoding()
	if countTag(enc.Tags(), TagSetAntiAlias) != 0 {
		t.Error("should not emit TagSetAntiAlias when AA hasn't changed from default (true)")
	}
}

func TestSceneAntiAliasEmittedOnChange(t *testing.T) {
	s := NewScene()
	rect := NewRectShape(0, 0, 100, 100)
	brush := SolidBrush(gg.Red)

	// Change AA to false before fill.
	s.SetAntiAlias(false)
	s.Fill(FillNonZero, IdentityAffine(), brush, rect)

	enc := s.Encoding()
	tags := enc.Tags()

	count := countTag(tags, TagSetAntiAlias)
	if count != 1 {
		t.Errorf("expected 1 TagSetAntiAlias, got %d", count)
	}
}

func TestSceneAntiAliasDeltaEncoding(t *testing.T) {
	s := NewScene()
	rect := NewRectShape(0, 0, 100, 100)
	brush := SolidBrush(gg.Red)

	// Disable AA and draw twice — should only emit ONE TagSetAntiAlias.
	s.SetAntiAlias(false)
	s.Fill(FillNonZero, IdentityAffine(), brush, rect)
	s.Fill(FillNonZero, IdentityAffine(), brush, rect)

	enc := s.Encoding()
	tags := enc.Tags()

	count := countTag(tags, TagSetAntiAlias)
	if count != 1 {
		t.Errorf("delta encoding: expected 1 TagSetAntiAlias for two fills with same state, got %d", count)
	}
}

func TestSceneAntiAliasMultipleTransitions(t *testing.T) {
	s := NewScene()
	rect := NewRectShape(0, 0, 100, 100)
	brush := SolidBrush(gg.Red)

	// false → fill → true → fill → false → fill = 3 TagSetAntiAlias
	s.SetAntiAlias(false)
	s.Fill(FillNonZero, IdentityAffine(), brush, rect)
	s.SetAntiAlias(true)
	s.Fill(FillNonZero, IdentityAffine(), brush, rect)
	s.SetAntiAlias(false)
	s.Fill(FillNonZero, IdentityAffine(), brush, rect)

	enc := s.Encoding()
	tags := enc.Tags()

	count := countTag(tags, TagSetAntiAlias)
	if count != 3 {
		t.Errorf("expected 3 TagSetAntiAlias for 3 transitions, got %d", count)
	}
}

func TestSceneAntiAliasDecodeRoundTrip(t *testing.T) {
	s := NewScene()
	rect := NewRectShape(0, 0, 100, 100)
	brush := SolidBrush(gg.Red)

	s.SetAntiAlias(false)
	s.Fill(FillNonZero, IdentityAffine(), brush, rect)

	enc := s.Encoding()
	dec := NewDecoder(enc)

	foundAA := false
	aaValue := true
	for dec.Next() {
		if dec.Tag() == TagSetAntiAlias {
			foundAA = true
			aaValue = dec.AntiAlias()
		}
	}

	if !foundAA {
		t.Fatal("TagSetAntiAlias not found in encoded stream")
	}
	if aaValue {
		t.Error("decoded AntiAlias() should be false")
	}
}

func TestSceneAntiAliasDecoderDefaultTrue(t *testing.T) {
	// Test decoder returns true (default) when drawData is exhausted.
	enc := NewEncoding()
	// Manually emit a tag without data to test boundary.
	enc.tags = append(enc.tags, TagSetAntiAlias)
	// Don't add drawData — simulates corrupt/truncated stream.

	dec := NewDecoder(enc)
	dec.Next()
	if !dec.AntiAlias() {
		t.Error("AntiAlias() should default to true on exhausted data")
	}
}

func TestSceneBuilderSetAntiAlias(t *testing.T) {
	b := NewSceneBuilder()
	rect := NewRectShape(0, 0, 50, 50)
	brush := SolidBrush(gg.Blue)

	b.SetAntiAlias(false).Fill(rect, brush)

	if b.AntiAlias() {
		t.Error("builder AntiAlias() should be false")
	}

	scene := b.Build()
	enc := scene.Encoding()
	count := countTag(enc.Tags(), TagSetAntiAlias)
	if count != 1 {
		t.Errorf("builder: expected 1 TagSetAntiAlias, got %d", count)
	}
}

func TestSceneAntiAliasStroke(t *testing.T) {
	s := NewScene()
	line := NewLineShape(0, 0, 100, 100)
	brush := SolidBrush(gg.Black)
	style := DefaultStrokeStyle()

	s.SetAntiAlias(false)
	s.Stroke(style, IdentityAffine(), brush, line)

	enc := s.Encoding()
	count := countTag(enc.Tags(), TagSetAntiAlias)
	if count != 1 {
		t.Errorf("stroke: expected 1 TagSetAntiAlias, got %d", count)
	}
}

func TestSceneAntiAliasAppendWithTranslation(t *testing.T) {
	// Create child scene with AA disabled.
	child := NewScene()
	child.SetAntiAlias(false)
	child.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), NewRectShape(0, 0, 50, 50))

	// Append into parent with translation.
	parent := NewScene()
	parent.AppendWithTranslation(child, 10, 20)

	enc := parent.Encoding()
	tags := enc.Tags()

	// The appended encoding should contain the TagSetAntiAlias.
	count := countTag(tags, TagSetAntiAlias)
	if count != 1 {
		t.Errorf("AppendWithTranslation: expected 1 TagSetAntiAlias, got %d", count)
	}

	// Verify decode: find TagSetAntiAlias and check it decodes to false.
	dec := NewDecoder(enc)
	for dec.Next() {
		if dec.Tag() == TagSetAntiAlias {
			if dec.AntiAlias() {
				t.Error("AppendWithTranslation: decoded AntiAlias should be false")
			}
			return
		}
	}
	t.Fatal("AppendWithTranslation: TagSetAntiAlias not found in decoded stream")
}

// countTag counts occurrences of a specific tag in the tag stream.
//
//nolint:unparam // test utility: target is always TagSetAntiAlias in this file but the function is generic
func countTag(tags []Tag, target Tag) int {
	count := 0
	for _, t := range tags {
		if t == target {
			count++
		}
	}
	return count
}
