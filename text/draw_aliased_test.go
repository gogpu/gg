package text

import (
	"image"
	"image/color"
	"testing"
)

func TestDrawAliased_BinaryAlpha(t *testing.T) {
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

	DrawAliased(dst, "Hello", face, 10, 35, color.Black)

	hasNonZero := false
	for y := range dst.Bounds().Dy() {
		for x := range dst.Bounds().Dx() {
			_, _, _, a := dst.At(x, y).RGBA()
			a8 := a >> 8
			if a8 != 0 && a8 != 255 {
				t.Errorf("pixel(%d,%d) alpha = %d, want 0 or 255", x, y, a8)
				return
			}
			if a8 != 0 {
				hasNonZero = true
			}
		}
	}

	if !hasNonZero {
		t.Error("no pixels drawn — DrawAliased produced empty image")
	}
}

func TestDrawAliased_EmptyString(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 100, 50))
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(16.0)

	// Should not panic or modify image.
	DrawAliased(dst, "", face, 10, 30, color.Black)

	for y := range dst.Bounds().Dy() {
		for x := range dst.Bounds().Dx() {
			r, g, b, a := dst.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 || a != 0 {
				t.Fatal("DrawAliased with empty string modified the image")
			}
		}
	}
}

func TestDrawAliased_NilFace(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 100, 50))
	// Should not panic.
	DrawAliased(dst, "Hello", nil, 10, 30, color.Black)
}

func TestDrawAliased_MultipleSizes(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	sizes := []float64{10, 16, 24, 36, 48}
	for _, size := range sizes {
		t.Run("size_"+formatFloat(size), func(t *testing.T) {
			face := source.Face(size)
			dst := image.NewRGBA(image.Rect(0, 0, 300, 80))
			DrawAliased(dst, "Wg", face, 10, 60, color.Black)

			hasNonZero := false
			for _, pix := range dst.Pix {
				if pix != 0 {
					hasNonZero = true
					break
				}
			}
			if !hasNonZero {
				t.Errorf("size %.0f: no pixels drawn", size)
			}

			// Verify binary alpha.
			for i := 3; i < len(dst.Pix); i += 4 {
				a := dst.Pix[i]
				if a != 0 && a != 255 {
					t.Errorf("size %.0f: alpha = %d at byte %d, want 0 or 255", size, a, i)
					return
				}
			}
		})
	}
}

// formatFloat produces a short string for test names.
func formatFloat(f float64) string {
	if f == float64(int(f)) {
		return string(rune('0'+int(f)/10)) + string(rune('0'+int(f)%10))
	}
	return "x"
}

func TestDrawAliased_VsDraw_DifferentAlpha(t *testing.T) {
	fontPath := testFontPath(t)

	source, err := NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	defer func() {
		_ = source.Close()
	}()

	face := source.Face(24.0)

	dstAA := image.NewRGBA(image.Rect(0, 0, 200, 50))
	Draw(dstAA, "O", face, 10, 35, color.Black)

	dstAliased := image.NewRGBA(image.Rect(0, 0, 200, 50))
	DrawAliased(dstAliased, "O", face, 10, 35, color.Black)

	// AA version should have intermediate alpha values on edges.
	aaHasIntermediate := false
	for i := 3; i < len(dstAA.Pix); i += 4 {
		a := dstAA.Pix[i]
		if a > 0 && a < 255 {
			aaHasIntermediate = true
			break
		}
	}

	// Aliased version must have only binary alpha.
	for i := 3; i < len(dstAliased.Pix); i += 4 {
		a := dstAliased.Pix[i]
		if a != 0 && a != 255 {
			t.Errorf("aliased alpha = %d, want 0 or 255", a)
			return
		}
	}

	if aaHasIntermediate {
		t.Log("Confirmed: AA path produces intermediate alpha, aliased path does not")
	}
}
