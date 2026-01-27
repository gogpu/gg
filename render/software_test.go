// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"image/color"
	"testing"
)

func TestNewSoftwareRenderer(t *testing.T) {
	renderer := NewSoftwareRenderer()

	if renderer == nil {
		t.Fatal("NewSoftwareRenderer() returned nil")
	}
}

func TestSoftwareRendererCapabilities(t *testing.T) {
	renderer := NewSoftwareRenderer()
	caps := renderer.Capabilities()

	if caps.IsGPU {
		t.Error("SoftwareRenderer should not be GPU")
	}
	if !caps.SupportsAntialiasing {
		t.Error("SoftwareRenderer should support antialiasing")
	}
}

func TestSoftwareRendererFlush(t *testing.T) {
	renderer := NewSoftwareRenderer()

	err := renderer.Flush()
	if err != nil {
		t.Errorf("Flush() error = %v, want nil", err)
	}
}

func TestSoftwareRendererNilTarget(t *testing.T) {
	renderer := NewSoftwareRenderer()
	scene := NewScene()

	err := renderer.Render(nil, scene)
	if err == nil {
		t.Error("Render(nil, _) should return error")
	}
}

func TestSoftwareRendererNilScene(t *testing.T) {
	renderer := NewSoftwareRenderer()
	target := NewPixmapTarget(100, 100)

	err := renderer.Render(target, nil)
	if err != nil {
		t.Errorf("Render(_, nil) error = %v, want nil", err)
	}
}

func TestSoftwareRendererEmptyScene(t *testing.T) {
	renderer := NewSoftwareRenderer()
	target := NewPixmapTarget(100, 100)
	scene := NewScene()

	err := renderer.Render(target, scene)
	if err != nil {
		t.Errorf("Render() error = %v, want nil", err)
	}
}

func TestSoftwareRendererClear(t *testing.T) {
	renderer := NewSoftwareRenderer()
	target := NewPixmapTarget(10, 10)
	scene := NewScene()

	// Clear to red
	scene.Clear(color.RGBA{255, 0, 0, 255})

	err := renderer.Render(target, scene)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Check center pixel
	pixel := target.GetPixel(5, 5).(color.RGBA)
	if pixel.R != 255 || pixel.G != 0 || pixel.B != 0 || pixel.A != 255 {
		t.Errorf("Center pixel = %v, want red", pixel)
	}
}

func TestSoftwareRendererFillRectangle(t *testing.T) {
	renderer := NewSoftwareRenderer()
	target := NewPixmapTarget(100, 100)
	scene := NewScene()

	// Clear to white
	scene.Clear(color.White)

	// Fill a red rectangle in the center
	scene.SetFillColor(color.RGBA{255, 0, 0, 255})
	scene.Rectangle(25, 25, 50, 50)
	scene.Fill()

	err := renderer.Render(target, scene)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Check inside rectangle (should be red)
	inside := target.GetPixel(50, 50).(color.RGBA)
	if inside.R != 255 || inside.B != 0 {
		t.Errorf("Inside pixel = %v, want red", inside)
	}

	// Check outside rectangle (should be white)
	outside := target.GetPixel(10, 10).(color.RGBA)
	if outside.R != 255 || outside.G != 255 || outside.B != 255 {
		t.Errorf("Outside pixel = %v, want white", outside)
	}
}

func TestSoftwareRendererFillCircle(t *testing.T) {
	renderer := NewSoftwareRenderer()
	target := NewPixmapTarget(200, 200)
	scene := NewScene()

	// Clear to white
	scene.Clear(color.White)

	// Fill a blue circle in the center
	scene.SetFillColor(color.RGBA{0, 0, 255, 255})
	scene.Circle(100, 100, 50)
	scene.Fill()

	err := renderer.Render(target, scene)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Check center (should be blue)
	center := target.GetPixel(100, 100).(color.RGBA)
	if center.B != 255 || center.R != 0 {
		t.Errorf("Center pixel = %v, want blue", center)
	}

	// Check corner (should be white - outside circle)
	corner := target.GetPixel(10, 10).(color.RGBA)
	if corner.R != 255 || corner.G != 255 || corner.B != 255 {
		t.Errorf("Corner pixel = %v, want white", corner)
	}
}

func TestSoftwareRendererStroke(t *testing.T) {
	renderer := NewSoftwareRenderer()
	target := NewPixmapTarget(100, 100)
	scene := NewScene()

	// Clear to white
	scene.Clear(color.White)

	// Stroke a diagonal line
	scene.SetStrokeColor(color.RGBA{0, 255, 0, 255})
	scene.SetStrokeWidth(5)
	scene.MoveTo(10, 10)
	scene.LineTo(90, 90)
	scene.Stroke()

	err := renderer.Render(target, scene)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Check middle of line (should have some green)
	middle := target.GetPixel(50, 50).(color.RGBA)
	if middle.G == 0 {
		t.Errorf("Middle of stroke should have green, got %v", middle)
	}

	// Check corner (should be white - outside stroke)
	corner := target.GetPixel(5, 95).(color.RGBA)
	if corner.R != 255 || corner.G != 255 || corner.B != 255 {
		t.Errorf("Corner pixel = %v, want white", corner)
	}
}

func TestSoftwareRendererAlphaBlend(t *testing.T) {
	renderer := NewSoftwareRenderer()
	target := NewPixmapTarget(100, 100)
	scene := NewScene()

	// Fill with opaque red
	scene.Clear(color.RGBA{255, 0, 0, 255})

	// Overlay with semi-transparent blue
	scene.SetFillColor(color.RGBA{0, 0, 255, 128})
	scene.Rectangle(0, 0, 100, 100)
	scene.Fill()

	err := renderer.Render(target, scene)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Check blended pixel - should have both red and blue
	pixel := target.GetPixel(50, 50).(color.RGBA)

	// With 50% blue over red, we expect roughly:
	// R = 255 * 0.5 = 127-128
	// B = 255 * 0.5 = 127-128
	if pixel.R < 100 || pixel.R > 200 {
		t.Errorf("Blended R = %d, expected ~128", pixel.R)
	}
	if pixel.B < 50 || pixel.B > 180 {
		t.Errorf("Blended B = %d, expected some blue", pixel.B)
	}
}

func TestSoftwareRendererMultipleShapes(t *testing.T) {
	renderer := NewSoftwareRenderer()
	target := NewPixmapTarget(200, 200)
	scene := NewScene()

	scene.Clear(color.White)

	// Red rectangle top-left
	scene.SetFillColor(color.RGBA{255, 0, 0, 255})
	scene.Rectangle(10, 10, 50, 50)
	scene.Fill()

	// Green rectangle top-right
	scene.SetFillColor(color.RGBA{0, 255, 0, 255})
	scene.Rectangle(140, 10, 50, 50)
	scene.Fill()

	// Blue circle bottom-center
	scene.SetFillColor(color.RGBA{0, 0, 255, 255})
	scene.Circle(100, 150, 30)
	scene.Fill()

	err := renderer.Render(target, scene)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Check red rectangle
	red := target.GetPixel(30, 30).(color.RGBA)
	if red.R != 255 || red.G != 0 || red.B != 0 {
		t.Errorf("Red area = %v, want red", red)
	}

	// Check green rectangle
	green := target.GetPixel(165, 30).(color.RGBA)
	if green.R != 0 || green.G != 255 || green.B != 0 {
		t.Errorf("Green area = %v, want green", green)
	}

	// Check blue circle
	blue := target.GetPixel(100, 150).(color.RGBA)
	if blue.R != 0 || blue.G != 0 || blue.B != 255 {
		t.Errorf("Blue area = %v, want blue", blue)
	}
}

func TestSoftwareRendererGPUTargetError(t *testing.T) {
	renderer := NewSoftwareRenderer()

	// SurfaceTarget has no Pixels() - should fail
	target := NewSurfaceTarget(100, 100, 0, nil)
	scene := NewScene()

	err := renderer.Render(target, scene)
	if err == nil {
		t.Error("Render() on GPU-only target should return error")
	}
}

func TestSoftwareRendererLargeTarget(t *testing.T) {
	renderer := NewSoftwareRenderer()
	target := NewPixmapTarget(1920, 1080)
	scene := NewScene()

	scene.Clear(color.Black)
	scene.SetFillColor(color.White)
	scene.Circle(960, 540, 200)
	scene.Fill()

	err := renderer.Render(target, scene)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Sanity check center
	center := target.GetPixel(960, 540).(color.RGBA)
	if center.R != 255 || center.G != 255 || center.B != 255 {
		t.Errorf("Center = %v, want white", center)
	}
}

func BenchmarkSoftwareRendererClear(b *testing.B) {
	renderer := NewSoftwareRenderer()
	target := NewPixmapTarget(800, 600)
	scene := NewScene()
	scene.Clear(color.White)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = renderer.Render(target, scene)
	}
}

func BenchmarkSoftwareRendererFillRectangle(b *testing.B) {
	renderer := NewSoftwareRenderer()
	target := NewPixmapTarget(800, 600)
	scene := NewScene()
	scene.SetFillColor(color.RGBA{255, 0, 0, 255})
	scene.Rectangle(100, 100, 600, 400)
	scene.Fill()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = renderer.Render(target, scene)
	}
}

func BenchmarkSoftwareRendererFillCircle(b *testing.B) {
	renderer := NewSoftwareRenderer()
	target := NewPixmapTarget(800, 600)
	scene := NewScene()
	scene.SetFillColor(color.RGBA{0, 0, 255, 255})
	scene.Circle(400, 300, 200)
	scene.Fill()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = renderer.Render(target, scene)
	}
}

func BenchmarkSoftwareRendererComplexScene(b *testing.B) {
	renderer := NewSoftwareRenderer()
	target := NewPixmapTarget(800, 600)
	scene := NewScene()

	scene.Clear(color.White)

	// Add multiple shapes
	for i := 0; i < 10; i++ {
		scene.SetFillColor(color.RGBA{uint8(i * 25), uint8(255 - i*25), 128, 200})
		scene.Circle(float64(50+i*70), 300, 30)
		scene.Fill()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = renderer.Render(target, scene)
	}
}
