package gg

import (
	"os"
	"testing"
)

func TestTextModeDefault(t *testing.T) {
	dc := NewContext(10, 10)
	if dc.TextMode() != TextModeAuto {
		t.Errorf("default TextMode = %v, want TextModeAuto", dc.TextMode())
	}
}

func TestTextModeSetGet(t *testing.T) {
	dc := NewContext(10, 10)

	modes := []TextMode{
		TextModeAuto,
		TextModeMSDF,
		TextModeVector,
		TextModeBitmap,
	}

	for _, mode := range modes {
		dc.SetTextMode(mode)
		got := dc.TextMode()
		if got != mode {
			t.Errorf("SetTextMode(%v) then TextMode() = %v, want %v", mode, got, mode)
		}
	}
}

func TestTextModeString(t *testing.T) {
	tests := []struct {
		mode TextMode
		want string
	}{
		{TextModeAuto, "Auto"},
		{TextModeMSDF, "MSDF"},
		{TextModeVector, "Vector"},
		{TextModeBitmap, "Bitmap"},
		{TextMode(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.want {
				t.Errorf("TextMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestSelectTextStrategy(t *testing.T) {
	dc := NewContext(10, 10)

	// Auto mode returns TextModeAuto (preserved for current behavior).
	dc.SetTextMode(TextModeAuto)
	if got := dc.selectTextStrategy(); got != TextModeAuto {
		t.Errorf("selectTextStrategy() with Auto = %v, want TextModeAuto", got)
	}

	// Forced modes are returned as-is.
	forced := []TextMode{TextModeMSDF, TextModeVector, TextModeBitmap}
	for _, mode := range forced {
		dc.SetTextMode(mode)
		if got := dc.selectTextStrategy(); got != mode {
			t.Errorf("selectTextStrategy() with %v = %v, want %v", mode, got, mode)
		}
	}
}

// findSystemFontPath returns a path to a system TTF font, or empty string if none found.
func findSystemFontPath() string {
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

// TestTextModeBitmapSkipsGPU verifies that TextModeBitmap renders text via CPU
// without attempting GPU acceleration. Since no GPU accelerator is registered
// in unit tests, we verify the bitmap path produces output.
func TestTextModeBitmapSkipsGPU(t *testing.T) {
	fontPath := findSystemFontPath()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(200, 50)
	dc.SetRGB(1, 1, 1)
	dc.Clear()

	if err := dc.LoadFontFace(fontPath, 16.0); err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}

	dc.SetRGB(0, 0, 0)
	dc.SetTextMode(TextModeBitmap)
	dc.DrawString("Hello", 10, 30)

	// Verify pixels were drawn (not all white).
	hasNonWhite := false
	for y := 0; y < dc.PixelHeight(); y++ {
		for x := 0; x < dc.PixelWidth(); x++ {
			r, g, b, _ := dc.pixmap.At(x, y).RGBA()
			if r != 0xffff || g != 0xffff || b != 0xffff {
				hasNonWhite = true
				break
			}
		}
		if hasNonWhite {
			break
		}
	}
	if !hasNonWhite {
		t.Error("TextModeBitmap DrawString produced no visible output")
	}
}

// TestTextModeMSDFTriesGPU verifies that TextModeMSDF attempts GPU rendering
// and falls back to CPU when no accelerator is registered.
func TestTextModeMSDFTriesGPU(t *testing.T) {
	fontPath := findSystemFontPath()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(200, 50)
	dc.SetRGB(1, 1, 1)
	dc.Clear()

	if err := dc.LoadFontFace(fontPath, 16.0); err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}

	dc.SetRGB(0, 0, 0)
	dc.SetTextMode(TextModeMSDF)

	// No GPU accelerator registered, so MSDF will fall back to CPU.
	// Should not panic and should still produce output.
	dc.DrawString("Hello", 10, 30)

	hasNonWhite := false
	for y := 0; y < dc.PixelHeight(); y++ {
		for x := 0; x < dc.PixelWidth(); x++ {
			r, g, b, _ := dc.pixmap.At(x, y).RGBA()
			if r != 0xffff || g != 0xffff || b != 0xffff {
				hasNonWhite = true
				break
			}
		}
		if hasNonWhite {
			break
		}
	}
	if !hasNonWhite {
		t.Error("TextModeMSDF DrawString (CPU fallback) produced no visible output")
	}
}

// TestTextModeVectorUsesOutlines verifies that TextModeVector uses the outline
// rendering path (Strategy B). Without a font source, it falls back to bitmap.
func TestTextModeVectorUsesOutlines(t *testing.T) {
	fontPath := findSystemFontPath()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(200, 50)
	dc.SetRGB(1, 1, 1)
	dc.Clear()

	if err := dc.LoadFontFace(fontPath, 16.0); err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}

	dc.SetRGB(0, 0, 0)
	dc.SetTextMode(TextModeVector)

	// Vector mode uses drawStringAsOutlines.
	// Should not panic and should produce output.
	dc.DrawString("Hello", 10, 30)

	hasNonWhite := false
	for y := 0; y < dc.PixelHeight(); y++ {
		for x := 0; x < dc.PixelWidth(); x++ {
			r, g, b, _ := dc.pixmap.At(x, y).RGBA()
			if r != 0xffff || g != 0xffff || b != 0xffff {
				hasNonWhite = true
				break
			}
		}
		if hasNonWhite {
			break
		}
	}
	if !hasNonWhite {
		t.Error("TextModeVector DrawString produced no visible output")
	}
}

// TestTextModeAutoPreservesBehavior verifies that TextModeAuto (the default)
// preserves the original DrawString behavior.
func TestTextModeAutoPreservesBehavior(t *testing.T) {
	fontPath := findSystemFontPath()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(200, 50)
	dc.SetRGB(1, 1, 1)
	dc.Clear()

	if err := dc.LoadFontFace(fontPath, 16.0); err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}

	dc.SetRGB(0, 0, 0)
	// TextModeAuto is the default, no SetTextMode needed.
	dc.DrawString("Hello", 10, 30)

	hasNonWhite := false
	for y := 0; y < dc.PixelHeight(); y++ {
		for x := 0; x < dc.PixelWidth(); x++ {
			r, g, b, _ := dc.pixmap.At(x, y).RGBA()
			if r != 0xffff || g != 0xffff || b != 0xffff {
				hasNonWhite = true
				break
			}
		}
		if hasNonWhite {
			break
		}
	}
	if !hasNonWhite {
		t.Error("TextModeAuto DrawString produced no visible output")
	}
}

// TestTextModeNoFont verifies that all text modes handle nil font gracefully.
func TestTextModeNoFont(t *testing.T) {
	modes := []TextMode{TextModeAuto, TextModeMSDF, TextModeVector, TextModeBitmap}
	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			dc := NewContext(10, 10)
			dc.SetTextMode(mode)
			// Should not panic with no font set.
			dc.DrawString("Hello", 5, 5)
			dc.DrawStringAnchored("Hello", 5, 5, 0.5, 0.5)
		})
	}
}
