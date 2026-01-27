// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package surface

import (
	"image"
	"image/color"
	"testing"
)

// TestNewImageSurface tests surface creation.
func TestNewImageSurface(t *testing.T) {
	s := NewImageSurface(100, 100)
	if s == nil {
		t.Fatal("NewImageSurface returned nil")
	}
	defer s.Close()

	if s.Width() != 100 {
		t.Errorf("Width() = %d, want 100", s.Width())
	}
	if s.Height() != 100 {
		t.Errorf("Height() = %d, want 100", s.Height())
	}
}

// TestNewImageSurfaceInvalidSize tests handling of invalid dimensions.
func TestNewImageSurfaceInvalidSize(t *testing.T) {
	// Should clamp to minimum of 1x1
	s := NewImageSurface(0, 0)
	defer s.Close()

	if s.Width() != 1 || s.Height() != 1 {
		t.Errorf("expected 1x1, got %dx%d", s.Width(), s.Height())
	}
}

// TestImageSurfaceClear tests the Clear operation.
func TestImageSurfaceClear(t *testing.T) {
	s := NewImageSurface(10, 10)
	defer s.Close()

	// Clear with red
	s.Clear(color.RGBA{255, 0, 0, 255})

	img := s.Snapshot()
	if img == nil {
		t.Fatal("Snapshot returned nil")
	}

	// Check center pixel
	c := img.RGBAAt(5, 5)
	if c.R != 255 || c.G != 0 || c.B != 0 || c.A != 255 {
		t.Errorf("pixel = %v, want (255, 0, 0, 255)", c)
	}
}

// TestImageSurfaceFillRectangle tests filling a rectangle.
func TestImageSurfaceFillRectangle(t *testing.T) {
	s := NewImageSurface(100, 100)
	defer s.Close()

	// Clear with white
	s.Clear(color.White)

	// Fill a red rectangle
	path := NewPath()
	path.Rectangle(25, 25, 50, 50)
	s.Fill(path, FillStyle{Color: color.RGBA{255, 0, 0, 255}})

	img := s.Snapshot()

	// Check corner (should be white)
	c := img.RGBAAt(10, 10)
	if c.R != 255 || c.G != 255 || c.B != 255 {
		t.Errorf("corner pixel = %v, should be white", c)
	}

	// Check center (should be red)
	c = img.RGBAAt(50, 50)
	if c.R != 255 || c.G != 0 || c.B != 0 {
		t.Errorf("center pixel = %v, should be red", c)
	}
}

// TestImageSurfaceFillCircle tests filling a circle.
func TestImageSurfaceFillCircle(t *testing.T) {
	s := NewImageSurface(100, 100)
	defer s.Close()

	s.Clear(color.White)

	// Fill a blue circle
	path := NewPath()
	path.Circle(50, 50, 30)
	s.Fill(path, FillStyle{Color: color.RGBA{0, 0, 255, 255}})

	img := s.Snapshot()

	// Center should be blue
	c := img.RGBAAt(50, 50)
	if c.B < 200 {
		t.Errorf("center pixel blue = %d, should be high", c.B)
	}

	// Corner should be white
	c = img.RGBAAt(5, 5)
	if c.R != 255 || c.G != 255 || c.B != 255 {
		t.Errorf("corner pixel = %v, should be white", c)
	}
}

// TestImageSurfaceStroke tests stroking a path.
func TestImageSurfaceStroke(t *testing.T) {
	s := NewImageSurface(100, 100)
	defer s.Close()

	s.Clear(color.White)

	// Stroke a line
	path := NewPath()
	path.MoveTo(10, 50)
	path.LineTo(90, 50)
	s.Stroke(path, StrokeStyle{
		Color: color.RGBA{0, 128, 0, 255},
		Width: 4,
	})

	img := s.Snapshot()

	// Line center should have green
	c := img.RGBAAt(50, 50)
	if c.G < 100 {
		t.Errorf("line pixel green = %d, should be high (stroke may not be working)", c.G)
	}
}

// TestImageSurfaceDrawImage tests image drawing.
func TestImageSurfaceDrawImage(t *testing.T) {
	s := NewImageSurface(100, 100)
	defer s.Close()

	s.Clear(color.White)

	// Create a small red image
	srcImg := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			srcImg.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}

	// Draw it at (20, 30)
	s.DrawImage(srcImg, Pt(20, 30), nil)

	img := s.Snapshot()

	// Check a pixel in the drawn image area
	c := img.RGBAAt(25, 35)
	if c.R != 255 || c.G != 0 || c.B != 0 {
		t.Errorf("drawn image pixel = %v, should be red", c)
	}

	// Check pixel outside
	c = img.RGBAAt(5, 5)
	if c.R != 255 || c.G != 255 || c.B != 255 {
		t.Errorf("outside pixel = %v, should be white", c)
	}
}

// TestImageSurfaceFlush tests that Flush doesn't error.
func TestImageSurfaceFlush(t *testing.T) {
	s := NewImageSurface(10, 10)
	defer s.Close()

	if err := s.Flush(); err != nil {
		t.Errorf("Flush() returned error: %v", err)
	}
}

// TestImageSurfaceClose tests closing and double-close safety.
func TestImageSurfaceClose(t *testing.T) {
	s := NewImageSurface(10, 10)

	if err := s.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Double close should not panic
	if err := s.Close(); err != nil {
		t.Errorf("double Close() returned error: %v", err)
	}

	// Operations after close should be safe
	s.Clear(color.White) // Should not panic
	s.Fill(NewPath(), DefaultFillStyle())
}

// TestImageSurfaceCapabilities tests capability reporting.
func TestImageSurfaceCapabilities(t *testing.T) {
	s := NewImageSurface(10, 10)
	defer s.Close()

	caps := s.Capabilities()

	if !caps.SupportsAntialias {
		t.Error("ImageSurface should support antialiasing")
	}
}

// TestImageSurfaceImage tests direct image access.
func TestImageSurfaceImage(t *testing.T) {
	s := NewImageSurface(10, 10)
	defer s.Close()

	img := s.Image()
	if img == nil {
		t.Fatal("Image() returned nil")
	}

	bounds := img.Bounds()
	if bounds.Dx() != 10 || bounds.Dy() != 10 {
		t.Errorf("Image bounds = %v, want (0,0)-(10,10)", bounds)
	}
}

// TestImageSurfaceFromImage tests creating surface from existing image.
func TestImageSurfaceFromImage(t *testing.T) {
	// Create an image with some content
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			img.SetRGBA(x, y, color.RGBA{0, 255, 0, 255})
		}
	}

	s := NewImageSurfaceFromImage(img)
	defer s.Close()

	if s.Width() != 50 || s.Height() != 50 {
		t.Errorf("size = %dx%d, want 50x50", s.Width(), s.Height())
	}

	// Snapshot should show green
	snap := s.Snapshot()
	c := snap.RGBAAt(25, 25)
	if c.G != 255 {
		t.Errorf("pixel green = %d, want 255", c.G)
	}
}

// TestImageSurfaceFillRule tests fill rules.
func TestImageSurfaceFillRule(t *testing.T) {
	s := NewImageSurface(100, 100)
	defer s.Close()

	s.Clear(color.White)

	// Create a path with overlapping squares (self-intersecting)
	path := NewPath()
	path.Rectangle(20, 20, 60, 60) // Outer
	path.Rectangle(35, 35, 30, 30) // Inner (hole with EvenOdd)

	// Fill with EvenOdd rule
	s.Fill(path, FillStyle{
		Color: color.RGBA{255, 0, 0, 255},
		Rule:  FillRuleEvenOdd,
	})

	img := s.Snapshot()

	// Outer area should be red
	c := img.RGBAAt(25, 25)
	if c.R < 200 {
		t.Errorf("outer pixel = %v, expected red", c)
	}

	// Center (inner square) behavior depends on fill rule
	// With EvenOdd, self-intersecting areas create holes
	// With NonZero, they're filled
}

// BenchmarkImageSurfaceFillRect benchmarks rectangle filling.
func BenchmarkImageSurfaceFillRect(b *testing.B) {
	s := NewImageSurface(800, 600)
	defer s.Close()

	path := NewPath()
	path.Rectangle(100, 100, 600, 400)
	style := FillStyle{Color: color.RGBA{255, 0, 0, 255}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Fill(path, style)
	}
}

// BenchmarkImageSurfaceFillCircle benchmarks circle filling.
func BenchmarkImageSurfaceFillCircle(b *testing.B) {
	s := NewImageSurface(800, 600)
	defer s.Close()

	path := NewPath()
	path.Circle(400, 300, 200)
	style := FillStyle{Color: color.RGBA{0, 255, 0, 255}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Fill(path, style)
	}
}

// BenchmarkImageSurfaceClear benchmarks clearing.
func BenchmarkImageSurfaceClear(b *testing.B) {
	s := NewImageSurface(800, 600)
	defer s.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Clear(color.RGBA{128, 128, 128, 255})
	}
}
