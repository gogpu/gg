package gg

import (
	"testing"
)

// noaaPixelAlpha returns alpha channel of a pixel as uint8 (0-255).
func noaaPixelAlpha(pm *Pixmap, x, y int) uint8 {
	c := pm.GetPixel(x, y)
	a := int(c.A * 255)
	if a > 255 {
		a = 255
	}
	if a < 0 {
		a = 0
	}
	return uint8(a) //nolint:gosec // clamped above
}

func TestSetAntiAlias_BinaryPixelsOnly(t *testing.T) {
	dc := NewContext(64, 64)
	defer dc.Close()

	dc.SetAntiAlias(false)
	dc.SetRGB(1, 0, 0)

	// Draw a diagonal line (would produce gray fringe with AA)
	dc.SetLineWidth(2)
	dc.DrawLine(5, 5, 58, 58)
	dc.Stroke()

	// Draw a circle (would produce smooth edges with AA)
	dc.DrawCircle(32, 32, 20)
	dc.Fill()

	pm := dc.pixmap
	w, h := pm.Width(), pm.Height()

	for y := range h {
		for x := range w {
			a := noaaPixelAlpha(pm, x, y)
			if a != 0 && a != 255 {
				t.Fatalf("pixel (%d,%d) has gray alpha=%d; want 0 or 255 (no-AA mode)", x, y, a)
			}
		}
	}
}

func TestSetAntiAlias_AAHasGrayPixels(t *testing.T) {
	dc := NewContext(64, 64)
	defer dc.Close()

	// AA enabled (default) — diagonal line MUST produce gray pixels
	dc.SetRGB(1, 0, 0)
	dc.SetLineWidth(2)
	dc.DrawLine(5, 5, 58, 58)
	dc.Stroke()

	pm := dc.pixmap
	w, h := pm.Width(), pm.Height()

	hasGray := false
	for y := range h {
		for x := range w {
			a := noaaPixelAlpha(pm, x, y)
			if a != 0 && a != 255 {
				hasGray = true
				break
			}
		}
		if hasGray {
			break
		}
	}

	if !hasGray {
		t.Fatal("AA mode should produce gray edge pixels for diagonal line")
	}
}

func TestSetAntiAlias_PushPop(t *testing.T) {
	dc := NewContext(32, 32)
	defer dc.Close()

	if !dc.AntiAlias() {
		t.Fatal("default should be true")
	}

	dc.SetAntiAlias(false)
	dc.Push()
	dc.SetAntiAlias(true)

	if !dc.AntiAlias() {
		t.Fatal("after Push+SetAntiAlias(true) should be true")
	}

	dc.Pop()

	if dc.AntiAlias() {
		t.Fatal("after Pop should restore false")
	}
}

func TestSetAntiAlias_RectNoCoverage(t *testing.T) {
	dc := NewContext(32, 32)
	defer dc.Close()

	dc.SetAntiAlias(false)
	dc.SetRGB(0, 0, 1)

	// Axis-aligned rect at integer coords — all interior pixels fully opaque
	dc.DrawRectangle(4, 4, 16, 16)
	dc.Fill()

	pm := dc.pixmap

	// Check interior is fully opaque
	for y := 4; y < 20; y++ {
		for x := 4; x < 20; x++ {
			a := noaaPixelAlpha(pm, x, y)
			if a != 255 {
				t.Fatalf("interior pixel (%d,%d) alpha=%d; want 255", x, y, a)
			}
		}
	}

	// Check exterior is fully transparent
	for x := range 32 {
		a := noaaPixelAlpha(pm, x, 0)
		if a != 0 {
			t.Fatalf("exterior pixel (%d,0) alpha=%d; want 0", x, a)
		}
	}
}
