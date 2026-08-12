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

// inkBounds returns the axis-aligned bounding box of non-white pixels and count.
// Used by NoAA stroke regression tests (#509).
func inkBounds(pm *Pixmap) (minX, minY, maxX, maxY, count int) {
	w, h := pm.Width(), pm.Height()
	minX, minY = w, h
	maxX, maxY = -1, -1
	for y := range h {
		for x := range w {
			c := pm.GetPixel(x, y)
			// White background is ~1,1,1; ink is darker.
			if c.R >= 0.99 && c.G >= 0.99 && c.B >= 0.99 {
				continue
			}
			count++
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	return
}

// TestSetAntiAlias_StrokeCircleBounds is the #509 / #405 regression:
// SetAntiAlias(false) + Stroke() on a circle must not displace the curve
// (previously X shrunk ~4×: center 335 → ~83, width 43 → ~10).
func TestSetAntiAlias_StrokeCircleBounds(t *testing.T) {
	const (
		cx, cy, r = 335.0, 40.0, 20.0
		lineW     = 3.0
		canvasW   = 400
		canvasH   = 300
	)

	render := func(aa bool) (minX, minY, maxX, maxY, count int) {
		dc := NewContext(canvasW, canvasH)
		defer dc.Close()
		dc.SetAntiAlias(aa)
		dc.ClearWithColor(White)
		dc.SetRGB(0, 0, 0)
		dc.SetLineWidth(lineW)
		dc.DrawCircle(cx, cy, r)
		if err := dc.Stroke(); err != nil {
			t.Fatalf("Stroke(aa=%v): %v", aa, err)
		}
		return inkBounds(dc.pixmap)
	}

	aaMinX, aaMinY, aaMaxX, aaMaxY, aaCount := render(true)
	noMinX, noMinY, noMaxX, noMaxY, noCount := render(false)

	if aaCount == 0 {
		t.Fatal("AA circle stroke produced no ink")
	}
	if noCount == 0 {
		t.Fatal("NoAA circle stroke produced no ink")
	}

	// AA reference from issue #509: ~(313,18)-(356,61)
	if aaMinX < 300 || aaMaxX > 370 || aaMinY < 10 || aaMaxY > 70 {
		t.Fatalf("AA bounds=(%d,%d)-(%d,%d) outside expected circle region",
			aaMinX, aaMinY, aaMaxX, aaMaxY)
	}

	// NoAA must land near the same center — not the pre-fix ~83 X collapse.
	noCX := float64(noMinX+noMaxX) / 2
	noCY := float64(noMinY+noMaxY) / 2
	if noCX < cx-8 || noCX > cx+8 {
		t.Fatalf("NoAA stroke center X=%.1f, want ~%.0f (pre-fix bug: ~83)", noCX, cx)
	}
	if noCY < cy-8 || noCY > cy+8 {
		t.Fatalf("NoAA stroke center Y=%.1f, want ~%.0f", noCY, cy)
	}

	noW := noMaxX - noMinX + 1
	noH := noMaxY - noMinY + 1
	// Stroke width 3 around r=20 → outer diameter ~43. Collapsed bug was ~10.
	if noW < 35 || noW > 55 {
		t.Fatalf("NoAA stroke width=%d, want ~43 (pre-fix bug: ~10)", noW)
	}
	if noH < 35 || noH > 55 {
		t.Fatalf("NoAA stroke height=%d, want ~43", noH)
	}

	// NoAA bounds should be within a few pixels of AA (rounding / binary edges).
	const slack = 4
	if absInt(noMinX-aaMinX) > slack || absInt(noMaxX-aaMaxX) > slack ||
		absInt(noMinY-aaMinY) > slack || absInt(noMaxY-aaMaxY) > slack {
		t.Fatalf("NoAA bounds=(%d,%d)-(%d,%d) diverge from AA=(%d,%d)-(%d,%d) by >%dpx",
			noMinX, noMinY, noMaxX, noMaxY, aaMinX, aaMinY, aaMaxX, aaMaxY, slack)
	}
}

// TestSetAntiAlias_StrokeLineUnchanged verifies straight-line strokes stay
// correct under NoAA (control for #509 — lines were never broken).
func TestSetAntiAlias_StrokeLineUnchanged(t *testing.T) {
	render := func(aa bool) (minX, maxX, count int) {
		dc := NewContext(100, 80)
		defer dc.Close()
		dc.SetAntiAlias(aa)
		dc.ClearWithColor(White)
		dc.SetRGB(0, 0, 0)
		dc.SetLineWidth(3)
		dc.DrawLine(20, 50, 80, 50)
		if err := dc.Stroke(); err != nil {
			t.Fatalf("Stroke(aa=%v): %v", aa, err)
		}
		minX, _, maxX, _, count = inkBounds(dc.pixmap)
		return
	}

	aaMin, aaMax, aaCount := render(true)
	noMin, noMax, noCount := render(false)
	if aaCount == 0 || noCount == 0 {
		t.Fatalf("line stroke empty: aa=%d noaa=%d", aaCount, noCount)
	}
	if absInt(aaMin-noMin) > 2 || absInt(aaMax-noMax) > 2 {
		t.Fatalf("line X range AA=[%d,%d] NoAA=[%d,%d]", aaMin, aaMax, noMin, noMax)
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
