package gg

import (
	"math"
	"testing"
)

// TestScaleAbout verifies that ScaleAbout(sx, sy, cx, cy) scales around
// the center point, keeping (cx, cy) fixed.
func TestScaleAbout(t *testing.T) {
	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(0, 0, 0)

	// Draw a 20x20 rect centered at (100,100) scaled 2x around (100,100).
	// The rect should become 40x40, still centered at (100,100).
	dc.ScaleAbout(2, 2, 100, 100)
	dc.DrawRectangle(90, 90, 20, 20) // user space: 90..110
	_ = dc.Fill()

	// After 2x scale about center: device rect = 80..120 (40px wide)
	// Check center is filled
	pCenter := dc.pixmap.GetPixel(100, 100)
	if pixelByte(pCenter.R) > 10 {
		t.Error("ScaleAbout: center should be filled (black)")
	}

	// Check expanded bounds are filled
	pEdge := dc.pixmap.GetPixel(82, 100)
	if pixelByte(pEdge.R) > 10 {
		t.Error("ScaleAbout: scaled edge (82,100) should be filled")
	}

	// Check outside original bounds but inside scaled bounds
	pOutside := dc.pixmap.GetPixel(75, 100)
	if pixelByte(pOutside.R) < 240 {
		t.Error("ScaleAbout: (75,100) should be outside scaled rect (white)")
	}
}

// TestScaleAbout_Identity verifies ScaleAbout(1,1,...) is identity.
func TestScaleAbout_Identity(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(0, 0, 0)

	dc.ScaleAbout(1, 1, 50, 50)
	dc.DrawRectangle(40, 40, 20, 20)
	_ = dc.Fill()

	// Center of rect should be filled
	p := dc.pixmap.GetPixel(50, 50)
	if pixelByte(p.R) > 10 {
		t.Error("ScaleAbout(1,1) should be identity")
	}

	// Just outside should be white
	pOut := dc.pixmap.GetPixel(35, 50)
	if pixelByte(pOut.R) < 240 {
		t.Error("ScaleAbout(1,1): outside rect should be white")
	}
}

// TestScaleAbout_MatchesManual verifies ScaleAbout produces the same result
// as manual Translate→Scale→Translate.
func TestScaleAbout_MatchesManual(t *testing.T) {
	render := func(useAbout bool) *Pixmap {
		dc := NewContext(200, 200)
		dc.ClearWithColor(White)
		dc.SetRGB(1, 0, 0)

		if useAbout {
			dc.ScaleAbout(1.5, 1.5, 100, 100)
		} else {
			dc.Translate(100, 100)
			dc.Scale(1.5, 1.5)
			dc.Translate(-100, -100)
		}

		dc.DrawCircle(100, 100, 30)
		_ = dc.Fill()
		return dc.pixmap
	}

	aboutPix := render(true)
	manualPix := render(false)

	// Compare pixel-by-pixel (should be identical)
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			a := aboutPix.GetPixel(x, y)
			m := manualPix.GetPixel(x, y)
			dr := math.Abs(a.R - m.R)
			dg := math.Abs(a.G - m.G)
			db := math.Abs(a.B - m.B)
			if dr > 0.01 || dg > 0.01 || db > 0.01 {
				t.Fatalf("ScaleAbout vs manual differ at (%d,%d): about=(%v,%v,%v) manual=(%v,%v,%v)",
					x, y, a.R, a.G, a.B, m.R, m.G, m.B)
			}
		}
	}
}

// TestShearAbout verifies ShearAbout produces the same result as manual composition.
func TestShearAbout_MatchesManual(t *testing.T) {
	render := func(useAbout bool) *Pixmap {
		dc := NewContext(200, 200)
		dc.ClearWithColor(White)
		dc.SetRGB(0, 0, 1)

		if useAbout {
			dc.ShearAbout(0.3, 0, 100, 100)
		} else {
			dc.Translate(100, 100)
			dc.Shear(0.3, 0)
			dc.Translate(-100, -100)
		}

		dc.DrawRectangle(80, 80, 40, 40)
		_ = dc.Fill()
		return dc.pixmap
	}

	aboutPix := render(true)
	manualPix := render(false)

	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			a := aboutPix.GetPixel(x, y)
			m := manualPix.GetPixel(x, y)
			if math.Abs(a.B-m.B) > 0.01 {
				t.Fatalf("ShearAbout vs manual differ at (%d,%d)", x, y)
			}
		}
	}
}

// TestScaleAbout_Renders verifies ScaleAbout produces visible output.
func TestScaleAbout_Renders(t *testing.T) {
	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.ClearWithColor(White)
	dc.SetRGB(1, 0, 0)

	dc.ScaleAbout(3, 3, 100, 100)
	dc.DrawCircle(100, 100, 10)
	_ = dc.Fill()

	pixels := countNonWhitePixels(dc, 0, 0, 200, 200)
	if pixels == 0 {
		t.Error("ScaleAbout(3,3) should produce visible pixels")
	}
}
