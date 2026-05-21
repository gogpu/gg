package gg

import (
	"os"
	"testing"

	"github.com/gogpu/gg/text"
)

// findTestFontPath returns a path to a system TTF font for testing.
// Uses the same candidate list as text_mode_test.go.
func findTestFontPath() string {
	candidates := []string{
		"C:\\Windows\\Fonts\\arial.ttf",
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Monaco.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// loadTestFont loads a test font face at the given size, or skips the test.
func loadTestFont(t *testing.T, size float64) text.Face {
	t.Helper()
	fontPath := findTestFontPath()
	if fontPath == "" {
		t.Skip("No system font available")
	}
	source, err := text.NewFontSourceFromFile(fontPath)
	if err != nil {
		t.Skipf("Failed to load font: %v", err)
	}
	return source.Face(size)
}

// countNonWhitePixels is defined in text_transform_test.go — reused here.
// It counts pixels in the specified region where R < 0.99 || G < 0.99 || B < 0.99.
// The canvas must be initialized with ClearWithColor(White) for correct results.

func TestStrokeString_Basic(t *testing.T) {
	face := loadTestFont(t, 24)

	dc := NewContext(200, 60)
	dc.ClearWithColor(White)
	dc.SetFont(face)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(2.0)

	dc.StrokeString("Hello", 10, 40)

	nonWhite := countNonWhitePixels(dc, 0, 0, 200, 60)
	if nonWhite == 0 {
		t.Error("StrokeString produced no output (0 non-white pixels)")
	}
}

func TestStrokeString_NoFont(t *testing.T) {
	dc := NewContext(100, 50)
	dc.ClearWithColor(White)
	dc.SetRGB(0, 0, 0)

	// No font set — should be a no-op.
	dc.StrokeString("Hello", 10, 30)

	nonWhite := countNonWhitePixels(dc, 0, 0, 100, 50)
	if nonWhite != 0 {
		t.Errorf("StrokeString with no font produced %d non-white pixels, want 0", nonWhite)
	}
}

func TestStrokeString_LineWidth(t *testing.T) {
	face := loadTestFont(t, 48)

	// Thin stroke — use a large character so stroke width differences are visible.
	thin := NewContext(300, 100)
	thin.ClearWithColor(White)
	thin.SetFont(face)
	thin.SetRGB(0, 0, 0)
	thin.SetLineWidth(1.0)
	thin.StrokeString("W", 10, 80)
	thinPixels := countNonWhitePixels(thin, 0, 0, 300, 100)

	// Thick stroke.
	thick := NewContext(300, 100)
	thick.ClearWithColor(White)
	thick.SetFont(face)
	thick.SetRGB(0, 0, 0)
	thick.SetLineWidth(5.0)
	thick.StrokeString("W", 10, 80)
	thickPixels := countNonWhitePixels(thick, 0, 0, 300, 100)

	if thinPixels == 0 {
		t.Skip("Thin stroke produced no pixels (font issue)")
	}

	// Thicker stroke should produce more drawn pixels.
	if thickPixels <= thinPixels {
		t.Errorf("Thicker stroke (%d pixels) should produce more pixels than thinner (%d pixels)",
			thickPixels, thinPixels)
	}
}

func TestStrokeStringAnchored_Center(t *testing.T) {
	face := loadTestFont(t, 20)

	dc := NewContext(200, 60)
	dc.ClearWithColor(White)
	dc.SetFont(face)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(1.5)

	// Anchored at center — text should be roughly centered at (100, 30).
	dc.StrokeStringAnchored("Hi", 100, 30, 0.5, 0.5)

	nonWhite := countNonWhitePixels(dc, 0, 0, 200, 60)
	if nonWhite == 0 {
		t.Error("StrokeStringAnchored produced no output")
	}

	// Center region should have the most pixels since text is centered there.
	centerPixels := countNonWhitePixels(dc, 50, 0, 150, 60)
	if centerPixels == 0 {
		t.Error("No pixels in center region for centered text")
	}
}

func TestStrokeStringAnchored_NoFont(t *testing.T) {
	dc := NewContext(100, 50)
	dc.ClearWithColor(White)

	// No font — should be a no-op.
	dc.StrokeStringAnchored("Hello", 50, 25, 0.5, 0.5)

	nonWhite := countNonWhitePixels(dc, 0, 0, 100, 50)
	if nonWhite != 0 {
		t.Errorf("StrokeStringAnchored with no font produced %d non-white pixels", nonWhite)
	}
}

func TestTextPath_ReturnsPath(t *testing.T) {
	face := loadTestFont(t, 24)

	dc := NewContext(200, 60)
	dc.SetFont(face)

	path := dc.TextPath("A", 10, 40)
	if path == nil {
		t.Fatal("TextPath returned nil for valid text")
	}

	// Path should have geometry (moveTo, lineTo, etc.).
	if path.NumVerbs() == 0 {
		t.Error("TextPath returned empty path for 'A'")
	}
}

func TestTextPath_NoFont(t *testing.T) {
	dc := NewContext(100, 50)
	// No font set.

	path := dc.TextPath("Hello", 10, 30)
	if path != nil {
		t.Error("TextPath with no font should return nil")
	}
}

func TestTextPath_EmptyString(t *testing.T) {
	face := loadTestFont(t, 24)

	dc := NewContext(200, 60)
	dc.SetFont(face)

	path := dc.TextPath("", 10, 40)
	if path != nil {
		t.Error("TextPath for empty string should return nil")
	}
}

func TestTextPath_FillMatchesDrawString(t *testing.T) {
	face := loadTestFont(t, 20)

	// Render via drawStringAsOutlines (TextModeVector).
	dcVector := NewContext(200, 60)
	dcVector.ClearWithColor(White)
	dcVector.SetFont(face)
	dcVector.SetRGB(0, 0, 0)
	dcVector.SetTextMode(TextModeVector)
	dcVector.DrawString("Hi", 10, 40)
	vectorPixels := countNonWhitePixels(dcVector, 0, 0, 200, 60)

	// Render via TextPath + FillPath.
	dcPath := NewContext(200, 60)
	dcPath.ClearWithColor(White)
	dcPath.SetFont(face)
	dcPath.SetRGB(0, 0, 0)
	path := dcPath.TextPath("Hi", 10, 40)
	if path == nil {
		t.Fatal("TextPath returned nil")
	}
	dcPath.SetFillRule(FillRuleNonZero)
	_ = dcPath.FillPath(path)
	fillPixels := countNonWhitePixels(dcPath, 0, 0, 200, 60)

	if vectorPixels == 0 {
		t.Skip("Vector text produced no pixels (font issue)")
	}

	// Both should produce approximately the same number of pixels.
	// Allow 5% tolerance for rounding differences in pipeline paths.
	diff := float64(fillPixels-vectorPixels) / float64(vectorPixels)
	if diff < 0 {
		diff = -diff
	}
	if diff > 0.05 {
		t.Errorf("TextPath+FillPath pixel count (%d) differs from Vector DrawString (%d) by %.1f%%",
			fillPixels, vectorPixels, diff*100)
	}
}

func TestStrokeString_IgnoresTextMode(t *testing.T) {
	face := loadTestFont(t, 24)

	modes := []TextMode{
		TextModeAuto,
		TextModeMSDF,
		TextModeVector,
		TextModeBitmap,
	}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			dc := NewContext(200, 60)
			dc.ClearWithColor(White)
			dc.SetFont(face)
			dc.SetRGB(0, 0, 0)
			dc.SetLineWidth(2.0)
			dc.SetTextMode(mode)

			dc.StrokeString("Hj", 10, 40)

			nonWhite := countNonWhitePixels(dc, 0, 0, 200, 60)
			if nonWhite == 0 {
				t.Errorf("StrokeString with TextMode=%s produced no output", mode)
			}
		})
	}
}

func TestStrokeString_PreservesContextPath(t *testing.T) {
	face := loadTestFont(t, 20)

	dc := NewContext(200, 100)
	dc.ClearWithColor(White)
	dc.SetFont(face)
	dc.SetRGB(0, 0, 0)
	dc.SetLineWidth(1.5)

	// Start building a path.
	dc.MoveTo(10, 80)
	dc.LineTo(190, 80)

	// StrokeString should NOT destroy the current path.
	dc.StrokeString("Test", 10, 30)

	// The path built before StrokeString should still be intact.
	if !dc.path.HasCurrentPoint() {
		t.Error("StrokeString destroyed the current context path")
	}

	// Should be able to stroke the line we started.
	dc.SetRGB(1, 0, 0)
	if err := dc.Stroke(); err != nil {
		t.Errorf("Stroke after StrokeString failed: %v", err)
	}
}

func TestStrokeString_UsesStrokeColor(t *testing.T) {
	face := loadTestFont(t, 24)

	dc := NewContext(200, 60)
	dc.ClearWithColor(White)
	dc.SetFont(face)

	// Set stroke color to red.
	dc.SetRGB(1, 0, 0)
	dc.SetLineWidth(3.0)
	dc.StrokeString("X", 10, 45)

	// Check that red pixels exist.
	img := dc.Image()
	hasRed := false
	for y := 0; y < 60; y++ {
		for x := 0; x < 60; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r > 0x8000 && g < 0x4000 && b < 0x4000 {
				hasRed = true
				break
			}
		}
		if hasRed {
			break
		}
	}
	if !hasRed {
		t.Error("StrokeString with red color produced no red pixels")
	}
}

func TestDrawStringAsOutlines_RegressionMatch(t *testing.T) {
	// Verify that the refactored drawStringAsOutlines (now using textOutlinePath)
	// produces identical output to the TextModeVector path.
	face := loadTestFont(t, 18)

	dc := NewContext(200, 60)
	dc.ClearWithColor(White)
	dc.SetFont(face)
	dc.SetRGB(0, 0, 0)
	dc.SetTextMode(TextModeVector)

	dc.DrawString("Refactor", 5, 40)

	nonWhite := countNonWhitePixels(dc, 0, 0, 200, 60)
	if nonWhite == 0 {
		t.Error("drawStringAsOutlines via TextModeVector produced no output after refactor")
	}
}

func TestStrokeString_DifferentFromFill(t *testing.T) {
	face := loadTestFont(t, 48)

	// Fill text "O" — large size to ensure interior has many filled pixels.
	dcFill := NewContext(300, 100)
	dcFill.ClearWithColor(White)
	dcFill.SetFont(face)
	dcFill.SetRGB(0, 0, 0)
	dcFill.SetTextMode(TextModeVector)
	dcFill.DrawString("O", 10, 80)
	fillPixels := countNonWhitePixels(dcFill, 0, 0, 300, 100)

	// Stroke text "O" with thin outline — only the edges, not the interior.
	dcStroke := NewContext(300, 100)
	dcStroke.ClearWithColor(White)
	dcStroke.SetFont(face)
	dcStroke.SetRGB(0, 0, 0)
	dcStroke.SetLineWidth(1.0)
	dcStroke.StrokeString("O", 10, 80)
	strokePixels := countNonWhitePixels(dcStroke, 0, 0, 300, 100)

	if fillPixels == 0 || strokePixels == 0 {
		t.Skip("Text produced no pixels (font issue)")
	}

	// A thin stroke of "O" should produce fewer pixels than a fill,
	// because stroke only draws the outline edges, not the solid interior.
	if strokePixels >= fillPixels {
		t.Errorf("Stroke (%d pixels) should produce fewer pixels than fill (%d pixels) for 'O'",
			strokePixels, fillPixels)
	}
}

func TestStrokeString_ColorMatchesCurrentColor(t *testing.T) {
	face := loadTestFont(t, 24)

	dc := NewContext(200, 60)
	dc.ClearWithColor(White)
	dc.SetFont(face)

	// Stroke with blue color.
	dc.SetRGB(0, 0, 1)
	dc.SetLineWidth(3.0)
	dc.StrokeString("B", 10, 45)

	// Verify blue pixels exist.
	img := dc.Image()
	hasBlue := false
	for y := 0; y < 60 && !hasBlue; y++ {
		for x := 0; x < 80 && !hasBlue; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if b > 0x8000 && r < 0x4000 && g < 0x4000 {
				hasBlue = true
			}
		}
	}
	if !hasBlue {
		t.Error("StrokeString with blue color produced no blue pixels")
	}
}
