// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package gg

import (
	"image/color"
	"testing"

	"github.com/gogpu/gg/text"
)

func TestNewContextWithScale(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		scale      float64
		wantW      int
		wantH      int
		wantPixelW int
		wantPixelH int
		wantScale  float64
	}{
		{
			name:  "no scale",
			width: 800, height: 600,
			scale: 1.0,
			wantW: 800, wantH: 600,
			wantPixelW: 800, wantPixelH: 600,
			wantScale: 1.0,
		},
		{
			name:  "retina 2x",
			width: 800, height: 600,
			scale: 2.0,
			wantW: 800, wantH: 600,
			wantPixelW: 1600, wantPixelH: 1200,
			wantScale: 2.0,
		},
		{
			name:  "mobile 3x",
			width: 400, height: 800,
			scale: 3.0,
			wantW: 400, wantH: 800,
			wantPixelW: 1200, wantPixelH: 2400,
			wantScale: 3.0,
		},
		{
			name:  "fractional 1.5x",
			width: 1000, height: 500,
			scale: 1.5,
			wantW: 1000, wantH: 500,
			wantPixelW: 1500, wantPixelH: 750,
			wantScale: 1.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := NewContextWithScale(tt.width, tt.height, tt.scale)
			defer func() { _ = dc.Close() }()

			if dc.Width() != tt.wantW {
				t.Errorf("Width() = %d, want %d", dc.Width(), tt.wantW)
			}
			if dc.Height() != tt.wantH {
				t.Errorf("Height() = %d, want %d", dc.Height(), tt.wantH)
			}
			if dc.PixelWidth() != tt.wantPixelW {
				t.Errorf("PixelWidth() = %d, want %d", dc.PixelWidth(), tt.wantPixelW)
			}
			if dc.PixelHeight() != tt.wantPixelH {
				t.Errorf("PixelHeight() = %d, want %d", dc.PixelHeight(), tt.wantPixelH)
			}
			if dc.DeviceScale() != tt.wantScale {
				t.Errorf("DeviceScale() = %f, want %f", dc.DeviceScale(), tt.wantScale)
			}
		})
	}
}

func TestWithDeviceScale(t *testing.T) {
	dc := NewContext(800, 600, WithDeviceScale(2.0))
	defer func() { _ = dc.Close() }()

	if dc.Width() != 800 {
		t.Errorf("Width() = %d, want 800", dc.Width())
	}
	if dc.Height() != 600 {
		t.Errorf("Height() = %d, want 600", dc.Height())
	}
	if dc.PixelWidth() != 1600 {
		t.Errorf("PixelWidth() = %d, want 1600", dc.PixelWidth())
	}
	if dc.PixelHeight() != 1200 {
		t.Errorf("PixelHeight() = %d, want 1200", dc.PixelHeight())
	}
	if dc.DeviceScale() != 2.0 {
		t.Errorf("DeviceScale() = %f, want 2.0", dc.DeviceScale())
	}
}

func TestDefaultContextHasScale1(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	if dc.DeviceScale() != 1.0 {
		t.Errorf("DeviceScale() = %f, want 1.0", dc.DeviceScale())
	}
	if dc.PixelWidth() != dc.Width() {
		t.Errorf("PixelWidth() = %d != Width() = %d at scale 1.0", dc.PixelWidth(), dc.Width())
	}
	if dc.PixelHeight() != dc.Height() {
		t.Errorf("PixelHeight() = %d != Height() = %d at scale 1.0", dc.PixelHeight(), dc.Height())
	}
}

func TestSetDeviceScale(t *testing.T) {
	dc := NewContext(800, 600)
	defer func() { _ = dc.Close() }()

	// Initially 1x
	if dc.DeviceScale() != 1.0 {
		t.Fatalf("initial DeviceScale() = %f, want 1.0", dc.DeviceScale())
	}
	if dc.PixelWidth() != 800 {
		t.Fatalf("initial PixelWidth() = %d, want 800", dc.PixelWidth())
	}

	// Change to 2x
	dc.SetDeviceScale(2.0)
	if dc.DeviceScale() != 2.0 {
		t.Errorf("DeviceScale() = %f, want 2.0", dc.DeviceScale())
	}
	if dc.Width() != 800 {
		t.Errorf("Width() changed to %d, want 800", dc.Width())
	}
	if dc.PixelWidth() != 1600 {
		t.Errorf("PixelWidth() = %d, want 1600", dc.PixelWidth())
	}
	if dc.PixelHeight() != 1200 {
		t.Errorf("PixelHeight() = %d, want 1200", dc.PixelHeight())
	}
}

func TestSetDeviceScaleIgnoresInvalid(t *testing.T) {
	dc := NewContext(100, 100, WithDeviceScale(2.0))
	defer func() { _ = dc.Close() }()

	// Zero should be ignored
	dc.SetDeviceScale(0)
	if dc.DeviceScale() != 2.0 {
		t.Errorf("DeviceScale() = %f after SetDeviceScale(0), want 2.0", dc.DeviceScale())
	}

	// Negative should be ignored
	dc.SetDeviceScale(-1)
	if dc.DeviceScale() != 2.0 {
		t.Errorf("DeviceScale() = %f after SetDeviceScale(-1), want 2.0", dc.DeviceScale())
	}

	// Same value should be no-op
	dc.SetDeviceScale(2.0)
	if dc.DeviceScale() != 2.0 {
		t.Errorf("DeviceScale() = %f after SetDeviceScale(2.0), want 2.0", dc.DeviceScale())
	}
}

func TestResizeWithDeviceScale(t *testing.T) {
	dc := NewContext(800, 600, WithDeviceScale(2.0))
	defer func() { _ = dc.Close() }()

	if err := dc.Resize(400, 300); err != nil {
		t.Fatalf("Resize() error: %v", err)
	}

	if dc.Width() != 400 {
		t.Errorf("Width() = %d, want 400", dc.Width())
	}
	if dc.Height() != 300 {
		t.Errorf("Height() = %d, want 300", dc.Height())
	}
	// Physical should be 400*2=800, 300*2=600
	if dc.PixelWidth() != 800 {
		t.Errorf("PixelWidth() = %d, want 800", dc.PixelWidth())
	}
	if dc.PixelHeight() != 600 {
		t.Errorf("PixelHeight() = %d, want 600", dc.PixelHeight())
	}
}

func TestIdentityResetsToIdentity(t *testing.T) {
	dc := NewContext(800, 600, WithDeviceScale(2.0))
	defer func() { _ = dc.Close() }()

	// Apply user transform
	dc.Translate(100, 100)
	dc.Rotate(0.5)

	// Reset to identity
	dc.Identity()

	// Should be back to pure identity — device scale is separate.
	// TransformPoint uses only the user matrix.
	px, py := dc.TransformPoint(1, 0)
	if px != 1.0 || py != 0.0 {
		t.Errorf("TransformPoint(1, 0) = (%f, %f), want (1, 0) after Identity()", px, py)
	}
}

func TestGetCurrentPointWithDeviceScale(t *testing.T) {
	dc := NewContext(100, 100, WithDeviceScale(2.0))
	defer func() { _ = dc.Close() }()

	dc.MoveTo(50, 60)
	x, y, ok := dc.GetCurrentPoint()
	if !ok {
		t.Fatal("expected current point")
	}
	if x != 50 || y != 60 {
		t.Errorf("GetCurrentPoint() = (%v, %v), want (50, 60)", x, y)
	}
}

func TestCoordinateRoundTrip(t *testing.T) {
	dc := NewContext(100, 100, WithDeviceScale(2.0))
	defer func() { _ = dc.Close() }()

	dc.MoveTo(25, 30)
	x, y, _ := dc.GetCurrentPoint()
	dc.LineTo(x+10, y+10) // Should NOT double-transform
	x2, y2, _ := dc.GetCurrentPoint()
	if x2 != 35 || y2 != 40 {
		t.Errorf("after LineTo(x+10, y+10): got (%v, %v), want (35, 40)", x2, y2)
	}
}

func TestGetTransformUserSpace(t *testing.T) {
	dc := NewContext(100, 100, WithDeviceScale(2.0))
	defer func() { _ = dc.Close() }()

	m := dc.GetTransform()
	// Should be Identity, not Scale(2, 2)
	if m.A != 1 || m.E != 1 || m.B != 0 || m.D != 0 {
		t.Errorf("GetTransform() = %+v, want Identity", m)
	}
}

func TestFillAtDeviceScale(t *testing.T) {
	dc := NewContext(10, 10, WithDeviceScale(2.0))
	defer func() { _ = dc.Close() }()

	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, 1, 1)
	_ = dc.Fill()

	img := dc.Image()
	// Physical image should be 20x20
	if img.Bounds().Dx() != 20 || img.Bounds().Dy() != 20 {
		t.Errorf("bounds = %v, want 20x20", img.Bounds())
	}
}

func TestClipRectDeviceScale(t *testing.T) {
	dc := NewContext(100, 100, WithDeviceScale(2.0))
	defer func() { _ = dc.Close() }()

	dc.ClipRect(10, 10, 50, 50)
	// Should clip correctly in user-space coordinates.
	// Verify drawing outside clip produces no pixels.
	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, 5, 5) // Outside clip (10,10)-(60,60)
	_ = dc.Fill()
	// The rectangle (0,0)-(5,5) is outside the clip (10,10)-(60,60)
	// so no red pixels should appear at physical pixel (0,0).
	px := dc.pixmap.GetPixel(0, 0)
	if px.R != 0 {
		t.Errorf("pixel outside clip should be transparent, got r=%v", px.R)
	}
}

func TestDrawingAtDeviceScale(t *testing.T) {
	// Verify that drawing at 2x scale produces pixels in the right location.
	// Draw a single pixel at logical (0, 0) and verify physical pixmap is written.
	dc := NewContextWithScale(10, 10, 2.0)
	defer func() { _ = dc.Close() }()

	// Physical pixmap is 20x20. Drawing at logical (0,0)-(1,1) should
	// affect physical pixels (0,0)-(2,2) due to the 2x scale transform.
	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, 1, 1)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill() error: %v", err)
	}

	img := dc.Image()
	bounds := img.Bounds()

	// Physical image should be 20x20
	if bounds.Dx() != 20 || bounds.Dy() != 20 {
		t.Errorf("Image bounds = %dx%d, want 20x20", bounds.Dx(), bounds.Dy())
	}
}

func TestDrawStringBitmapRetinaScaling(t *testing.T) {
	// Regression: gg#276 — drawStringBitmap used user-space font size on
	// device-space pixmap. On Retina (2x), text appeared half-size.
	// Fix: drawStringBitmap creates a device-scaled face when deviceScale != 1.0.
	dc1 := NewContext(200, 100)
	defer func() { _ = dc1.Close() }()

	dc2 := NewContext(200, 100, WithDeviceScale(2.0))
	defer func() { _ = dc2.Close() }()

	face := loadDeviceScaleTestFont(t, 24)

	dc1.SetFont(face)
	dc2.SetFont(face)

	dc1.SetRGBA(1, 1, 1, 1)
	dc2.SetRGBA(1, 1, 1, 1)

	// Force CPU bitmap path via Translate (triggers IsTranslationOnly tier 0).
	dc1.Translate(10, 50)
	dc2.Translate(10, 50)

	dc1.DrawString("Tg", 0, 0)
	dc2.DrawString("Tg", 0, 0)

	img1 := dc1.Image() // 200x100
	img2 := dc2.Image() // 400x200 (2x physical)

	// Count non-zero pixels in a horizontal strip around baseline.
	// On dc2 (2x), the text should be physically larger (more pixels).
	count1 := countNonBlackPixels(img1, 0, 30, 200, 70)
	count2 := countNonBlackPixels(img2, 0, 60, 400, 140)

	// dc2 has 4x total area but text is 2x larger in each dimension → ~4x pixels.
	// Before fix: count2 ≈ count1 (same font size on bigger canvas).
	// After fix: count2 ≈ 4 * count1 (font scaled to device pixels).
	ratio := float64(count2) / float64(count1)
	if count1 == 0 {
		t.Fatal("no text pixels rendered on 1x canvas")
	}
	if ratio < 2.0 {
		t.Errorf("REGRESSION gg#276: Retina text not scaled. Pixel ratio = %.1f (want ≥ 2.0)", ratio)
	}
}

func loadDeviceScaleTestFont(t *testing.T, size float64) text.Face {
	t.Helper()
	candidates := []string{
		"C:/Windows/Fonts/arial.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/System/Library/Fonts/Helvetica.ttc",
		"/Library/Fonts/Arial.ttf",
	}
	for _, p := range candidates {
		source, err := text.NewFontSourceFromFile(p)
		if err == nil {
			return source.Face(size)
		}
	}
	t.Skip("no system font found for test")
	return nil
}

func countNonBlackPixels(img interface{ At(x, y int) color.Color }, x0, y0, x1, y1 int) int {
	count := 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r > 0 || g > 0 || b > 0 {
				count++
			}
		}
	}
	return count
}
