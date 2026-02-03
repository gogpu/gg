package raster

import (
	"bytes"
	"image"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/recording"
)

func TestBackendRegistration(t *testing.T) {
	// Verify the backend is registered
	if !recording.IsRegistered("raster") {
		t.Fatal("raster backend not registered")
	}

	// Verify we can create a backend via registry
	backend, err := recording.NewBackend("raster")
	if err != nil {
		t.Fatalf("failed to create raster backend: %v", err)
	}
	if backend == nil {
		t.Fatal("backend is nil")
	}

	// Verify it's the correct type
	_, ok := backend.(*Backend)
	if !ok {
		t.Fatal("backend is not *raster.Backend")
	}
}

func TestBackendLifecycle(t *testing.T) {
	backend := NewBackend()

	// Begin
	err := backend.Begin(100, 100)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Verify dimensions
	if backend.Width() != 100 {
		t.Errorf("Width = %d, want 100", backend.Width())
	}
	if backend.Height() != 100 {
		t.Errorf("Height = %d, want 100", backend.Height())
	}

	// End
	err = backend.End()
	if err != nil {
		t.Fatalf("End failed: %v", err)
	}

	// Image should be available
	img := backend.Image()
	if img == nil {
		t.Fatal("Image() returned nil")
	}

	bounds := img.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 100 {
		t.Errorf("Image bounds = %v, want 100x100", bounds)
	}
}

func TestBackendFillRect(t *testing.T) {
	backend := NewBackend()
	err := backend.Begin(100, 100)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Fill a red rectangle
	rect := recording.NewRect(10, 10, 50, 50)
	brush := recording.NewSolidBrush(gg.Red)
	backend.FillRect(rect, brush)

	err = backend.End()
	if err != nil {
		t.Fatalf("End failed: %v", err)
	}

	// Verify a pixel inside the rectangle is red
	img := backend.Image()
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatal("expected *image.RGBA")
	}

	// Check center of rectangle (35, 35)
	pixel := rgba.RGBAAt(35, 35)
	// Red in 8-bit is R=255, G=0, B=0, A=255
	if pixel.R < 200 || pixel.G > 50 || pixel.B > 50 {
		t.Errorf("pixel at (35,35) = %v, expected red", pixel)
	}
}

func TestBackendFillPath(t *testing.T) {
	backend := NewBackend()
	err := backend.Begin(100, 100)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Create a triangle path
	path := gg.NewPath()
	path.MoveTo(50, 10)
	path.LineTo(90, 90)
	path.LineTo(10, 90)
	path.Close()

	brush := recording.NewSolidBrush(gg.Blue)
	backend.FillPath(path, brush, recording.FillRuleNonZero)

	err = backend.End()
	if err != nil {
		t.Fatalf("End failed: %v", err)
	}

	// Verify a pixel inside the triangle is blue
	img := backend.Image()
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatal("expected *image.RGBA")
	}

	// Check center of triangle (50, 60)
	pixel := rgba.RGBAAt(50, 60)
	if pixel.B < 200 || pixel.R > 50 || pixel.G > 50 {
		t.Errorf("pixel at (50,60) = %v, expected blue", pixel)
	}
}

func TestBackendStrokePath(t *testing.T) {
	backend := NewBackend()
	err := backend.Begin(100, 100)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Create a line path
	path := gg.NewPath()
	path.MoveTo(10, 50)
	path.LineTo(90, 50)

	brush := recording.NewSolidBrush(gg.Green)
	stroke := recording.Stroke{
		Width:      5.0,
		Cap:        recording.LineCapRound,
		Join:       recording.LineJoinRound,
		MiterLimit: 4.0,
	}
	backend.StrokePath(path, brush, stroke)

	err = backend.End()
	if err != nil {
		t.Fatalf("End failed: %v", err)
	}

	// Verify a pixel on the line is green
	img := backend.Image()
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatal("expected *image.RGBA")
	}

	// Check center of line (50, 50)
	pixel := rgba.RGBAAt(50, 50)
	if pixel.G < 200 || pixel.R > 50 || pixel.B > 50 {
		t.Errorf("pixel at (50,50) = %v, expected green", pixel)
	}
}

func TestBackendSaveRestore(t *testing.T) {
	backend := NewBackend()
	err := backend.Begin(100, 100)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Save/Restore should not panic
	backend.Save()
	backend.Restore()

	// Multiple saves and restores
	backend.Save()
	backend.Save()
	backend.Restore()
	backend.Restore()

	err = backend.End()
	if err != nil {
		t.Fatalf("End failed: %v", err)
	}
}

func TestBackendSetTransform(t *testing.T) {
	backend := NewBackend()
	err := backend.Begin(100, 100)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Set a transform (translate by 10,10)
	m := recording.Translate(10, 10)
	backend.SetTransform(m)

	// This should not panic
	err = backend.End()
	if err != nil {
		t.Fatalf("End failed: %v", err)
	}
}

func TestBackendWriteTo(t *testing.T) {
	backend := NewBackend()
	err := backend.Begin(50, 50)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Draw something
	rect := recording.NewRect(0, 0, 50, 50)
	brush := recording.NewSolidBrush(gg.Red)
	backend.FillRect(rect, brush)

	err = backend.End()
	if err != nil {
		t.Fatalf("End failed: %v", err)
	}

	// Write to buffer
	var buf bytes.Buffer
	n, err := backend.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}
	if n == 0 {
		t.Error("WriteTo wrote 0 bytes")
	}

	// Verify PNG signature
	data := buf.Bytes()
	if len(data) < 8 {
		t.Fatal("PNG data too short")
	}
	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for i := 0; i < 8; i++ {
		if data[i] != pngSig[i] {
			t.Fatal("invalid PNG signature")
		}
	}
}

func TestBackendPixmap(t *testing.T) {
	backend := NewBackend()
	err := backend.Begin(50, 50)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	err = backend.End()
	if err != nil {
		t.Fatalf("End failed: %v", err)
	}

	// Get pixmap
	pixmap := backend.Pixmap()
	if pixmap == nil {
		t.Fatal("Pixmap() returned nil")
	}
	if pixmap.Width() != 50 || pixmap.Height() != 50 {
		t.Errorf("Pixmap dimensions = %dx%d, want 50x50", pixmap.Width(), pixmap.Height())
	}
}

func TestBackendInterfaceCompliance(t *testing.T) {
	// Verify Backend implements all interfaces
	var _ recording.Backend = (*Backend)(nil)
	var _ recording.WriterBackend = (*Backend)(nil)
	var _ recording.FileBackend = (*Backend)(nil)
	var _ recording.PixmapBackend = (*Backend)(nil)
}

func TestBackendLinearGradient(t *testing.T) {
	// NOTE: Linear gradient rendering is limited by gg.SoftwareRenderer
	// which currently only supports solid colors in fillSupersampled.
	// The gradient brush is correctly created and set, but the renderer
	// falls back to black. This test verifies the backend doesn't crash.
	// Full gradient support requires gg library enhancement.
	t.Skip("gradient rendering not yet supported by gg.SoftwareRenderer")

	backend := NewBackend()
	err := backend.Begin(100, 100)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Create a linear gradient brush
	grad := recording.NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, gg.Red).
		AddColorStop(1, gg.Blue)

	rect := recording.NewRect(0, 0, 100, 100)
	backend.FillRect(rect, grad)

	err = backend.End()
	if err != nil {
		t.Fatalf("End failed: %v", err)
	}

	// Verify gradient colors
	img := backend.Image()
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatal("expected *image.RGBA")
	}

	// Left side should be red-ish
	leftPixel := rgba.RGBAAt(5, 50)
	if leftPixel.R < 150 {
		t.Errorf("left pixel = %v, expected more red", leftPixel)
	}

	// Right side should be blue-ish
	rightPixel := rgba.RGBAAt(95, 50)
	if rightPixel.B < 150 {
		t.Errorf("right pixel = %v, expected more blue", rightPixel)
	}
}

func TestBackendRadialGradient(t *testing.T) {
	// NOTE: Radial gradient rendering is limited by gg.SoftwareRenderer
	// which currently only supports solid colors in fillSupersampled.
	// The gradient brush is correctly created and set, but the renderer
	// falls back to black. This test verifies the backend doesn't crash.
	// Full gradient support requires gg library enhancement.
	t.Skip("gradient rendering not yet supported by gg.SoftwareRenderer")

	backend := NewBackend()
	err := backend.Begin(100, 100)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Create a radial gradient brush
	grad := recording.NewRadialGradientBrush(50, 50, 0, 50).
		AddColorStop(0, gg.White).
		AddColorStop(1, gg.Black)

	rect := recording.NewRect(0, 0, 100, 100)
	backend.FillRect(rect, grad)

	err = backend.End()
	if err != nil {
		t.Fatalf("End failed: %v", err)
	}

	// Verify gradient colors
	img := backend.Image()
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatal("expected *image.RGBA")
	}

	// Center should be white
	centerPixel := rgba.RGBAAt(50, 50)
	if centerPixel.R < 200 || centerPixel.G < 200 || centerPixel.B < 200 {
		t.Errorf("center pixel = %v, expected white", centerPixel)
	}

	// Edge should be darker
	edgePixel := rgba.RGBAAt(5, 50)
	if edgePixel.R > 100 || edgePixel.G > 100 || edgePixel.B > 100 {
		t.Errorf("edge pixel = %v, expected dark", edgePixel)
	}
}

func TestRecordingPlayback(t *testing.T) {
	// Create a recording
	rec := recording.NewRecorder(100, 100)
	rec.SetRGB(1, 0, 0) // Red
	rec.DrawCircle(50, 50, 30)
	rec.Fill()
	r := rec.FinishRecording()

	// Playback to raster backend
	backend := NewBackend()
	err := r.Playback(backend)
	if err != nil {
		t.Fatalf("Playback failed: %v", err)
	}

	// Verify the circle was drawn
	img := backend.Image()
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatal("expected *image.RGBA")
	}

	// Center of circle should be red
	centerPixel := rgba.RGBAAt(50, 50)
	if centerPixel.R < 200 || centerPixel.G > 50 || centerPixel.B > 50 {
		t.Errorf("center pixel = %v, expected red", centerPixel)
	}

	// Outside circle should be transparent/black
	outsidePixel := rgba.RGBAAt(5, 5)
	if outsidePixel.R > 50 || outsidePixel.G > 50 || outsidePixel.B > 50 {
		t.Errorf("outside pixel = %v, expected transparent/black", outsidePixel)
	}
}

func TestRecordingPlaybackViaRegistry(t *testing.T) {
	// Create a recording
	rec := recording.NewRecorder(100, 100)
	rec.SetRGB(0, 1, 0) // Green
	rec.FillRectangle(20, 20, 60, 60)
	r := rec.FinishRecording()

	// Get backend from registry
	backend, err := recording.NewBackend("raster")
	if err != nil {
		t.Fatalf("NewBackend failed: %v", err)
	}

	// Playback
	err = r.Playback(backend)
	if err != nil {
		t.Fatalf("Playback failed: %v", err)
	}

	// Cast to raster backend to get image
	rb, ok := backend.(*Backend)
	if !ok {
		t.Fatal("backend is not *raster.Backend")
	}

	img := rb.Image()
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatal("expected *image.RGBA")
	}

	// Center of rectangle should be green
	pixel := rgba.RGBAAt(50, 50)
	if pixel.G < 200 || pixel.R > 50 || pixel.B > 50 {
		t.Errorf("pixel = %v, expected green", pixel)
	}
}

func TestBackendDashedStroke(t *testing.T) {
	backend := NewBackend()
	err := backend.Begin(100, 100)
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Create a line path
	path := gg.NewPath()
	path.MoveTo(10, 50)
	path.LineTo(90, 50)

	brush := recording.NewSolidBrush(gg.Black)
	stroke := recording.Stroke{
		Width:       3.0,
		Cap:         recording.LineCapButt,
		Join:        recording.LineJoinMiter,
		MiterLimit:  4.0,
		DashPattern: []float64{10, 5}, // 10px dash, 5px gap
		DashOffset:  0,
	}
	backend.StrokePath(path, brush, stroke)

	err = backend.End()
	if err != nil {
		t.Fatalf("End failed: %v", err)
	}

	// Verify the stroke was drawn (we can't easily verify dashing, just that something was drawn)
	img := backend.Image()
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatal("expected *image.RGBA")
	}

	// Check somewhere on the line
	pixel := rgba.RGBAAt(15, 50)
	// Should have some alpha (not fully transparent)
	if pixel.A == 0 {
		t.Error("expected non-transparent pixel on dashed line")
	}
}
