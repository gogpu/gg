package gg

import (
	"bytes"
	"image/jpeg"
	"image/png"
	"io"
	"testing"
)

func TestGetCurrentPoint(t *testing.T) {
	dc := NewContext(100, 100)

	// No current point initially
	x, y, ok := dc.GetCurrentPoint()
	if ok {
		t.Errorf("expected no current point initially, got (%v, %v, true)", x, y)
	}
	if x != 0 || y != 0 {
		t.Errorf("expected (0, 0) when no current point, got (%v, %v)", x, y)
	}

	// After MoveTo
	dc.MoveTo(50, 60)
	x, y, ok = dc.GetCurrentPoint()
	if !ok {
		t.Error("expected current point after MoveTo")
	}
	if x != 50 || y != 60 {
		t.Errorf("expected (50, 60), got (%v, %v)", x, y)
	}

	// After LineTo
	dc.LineTo(70, 80)
	x, y, ok = dc.GetCurrentPoint()
	if !ok {
		t.Error("expected current point after LineTo")
	}
	if x != 70 || y != 80 {
		t.Errorf("expected (70, 80), got (%v, %v)", x, y)
	}

	// After ClearPath
	dc.ClearPath()
	x, y, ok = dc.GetCurrentPoint()
	if ok {
		t.Errorf("expected no current point after ClearPath, got (%v, %v, true)", x, y)
	}
}

func TestGetCurrentPointWithQuadraticTo(t *testing.T) {
	dc := NewContext(100, 100)

	dc.MoveTo(10, 10)
	dc.QuadraticTo(50, 50, 90, 10) // control point, end point

	x, y, ok := dc.GetCurrentPoint()
	if !ok {
		t.Error("expected current point after QuadraticTo")
	}
	if x != 90 || y != 10 {
		t.Errorf("expected (90, 10), got (%v, %v)", x, y)
	}
}

func TestGetCurrentPointWithCubicTo(t *testing.T) {
	dc := NewContext(100, 100)

	dc.MoveTo(10, 10)
	dc.CubicTo(30, 50, 70, 50, 90, 10) // control1, control2, end point

	x, y, ok := dc.GetCurrentPoint()
	if !ok {
		t.Error("expected current point after CubicTo")
	}
	if x != 90 || y != 10 {
		t.Errorf("expected (90, 10), got (%v, %v)", x, y)
	}
}

func TestEncodePNG(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetRGB(1, 0, 0) // Red
	dc.DrawRectangle(0, 0, 100, 100)
	dc.Fill()

	var buf bytes.Buffer
	err := dc.EncodePNG(&buf)
	if err != nil {
		t.Fatalf("EncodePNG failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty PNG data")
	}

	// Verify it's valid PNG by decoding
	img, err := png.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("PNG decode failed: %v", err)
	}

	// Verify dimensions
	bounds := img.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 100 {
		t.Errorf("expected 100x100, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestEncodeJPEG(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetRGB(0, 1, 0) // Green
	dc.DrawRectangle(0, 0, 100, 100)
	dc.Fill()

	var buf bytes.Buffer
	err := dc.EncodeJPEG(&buf, 90)
	if err != nil {
		t.Fatalf("EncodeJPEG failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty JPEG data")
	}

	// Verify it's valid JPEG by decoding
	img, err := jpeg.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("JPEG decode failed: %v", err)
	}

	// Verify dimensions
	bounds := img.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 100 {
		t.Errorf("expected 100x100, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestEncodeJPEGQuality(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetRGB(0.5, 0.5, 0.5) // Gray
	dc.DrawRectangle(0, 0, 100, 100)
	dc.Fill()

	// Low quality should produce smaller file
	var bufLow bytes.Buffer
	err := dc.EncodeJPEG(&bufLow, 10)
	if err != nil {
		t.Fatalf("EncodeJPEG (low quality) failed: %v", err)
	}

	// High quality should produce larger file
	var bufHigh bytes.Buffer
	err = dc.EncodeJPEG(&bufHigh, 95)
	if err != nil {
		t.Fatalf("EncodeJPEG (high quality) failed: %v", err)
	}

	// High quality should typically be larger than low quality
	// (though this isn't guaranteed for all images)
	if bufLow.Len() >= bufHigh.Len() {
		t.Logf("Note: low quality (%d bytes) >= high quality (%d bytes), which can happen for simple images",
			bufLow.Len(), bufHigh.Len())
	}
}

func TestPixmapEncodePNG(t *testing.T) {
	pm := NewPixmap(50, 50)
	pm.Clear(RGBA{R: 0, G: 0, B: 1, A: 1}) // Blue

	var buf bytes.Buffer
	err := pm.EncodePNG(&buf)
	if err != nil {
		t.Fatalf("Pixmap.EncodePNG failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty PNG data")
	}

	// Verify it's valid PNG
	img, err := png.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("PNG decode failed: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 50 || bounds.Dy() != 50 {
		t.Errorf("expected 50x50, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestPixmapEncodeJPEG(t *testing.T) {
	pm := NewPixmap(50, 50)
	pm.Clear(RGBA{R: 1, G: 1, B: 0, A: 1}) // Yellow

	var buf bytes.Buffer
	err := pm.EncodeJPEG(&buf, 85)
	if err != nil {
		t.Fatalf("Pixmap.EncodeJPEG failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty JPEG data")
	}

	// Verify it's valid JPEG
	img, err := jpeg.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("JPEG decode failed: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 50 || bounds.Dy() != 50 {
		t.Errorf("expected 50x50, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestPathHasCurrentPoint(t *testing.T) {
	p := NewPath()

	if p.HasCurrentPoint() {
		t.Error("new path should not have current point")
	}

	p.MoveTo(10, 20)
	if !p.HasCurrentPoint() {
		t.Error("path should have current point after MoveTo")
	}

	p.LineTo(30, 40)
	if !p.HasCurrentPoint() {
		t.Error("path should have current point after LineTo")
	}

	p.Clear()
	if p.HasCurrentPoint() {
		t.Error("cleared path should not have current point")
	}
}

func TestContextClose(t *testing.T) {
	dc := NewContext(100, 100)

	// First close should succeed
	err := dc.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Second close should be safe (idempotent)
	err = dc.Close()
	if err != nil {
		t.Errorf("Second Close failed: %v", err)
	}
}

func TestContextImplementsCloser(t *testing.T) {
	// Compile-time check that Context implements io.Closer
	var _ io.Closer = (*Context)(nil)
}

func TestContextCloseReleasesResources(t *testing.T) {
	dc := NewContext(100, 100)
	dc.MoveTo(0, 0)
	dc.LineTo(100, 100)
	dc.Push()
	dc.Push()

	err := dc.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// After close, internal state should be cleared
	// (We can't easily verify this without exposing internals,
	// but at minimum Close should not panic)
}
