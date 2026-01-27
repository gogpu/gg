// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"image"
	"image/color"
	"testing"

	"github.com/gogpu/gpucontext"
)

func TestNewPixmapTarget(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"small", 100, 100},
		{"medium", 800, 600},
		{"large", 1920, 1080},
		{"wide", 1000, 100},
		{"tall", 100, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := NewPixmapTarget(tt.width, tt.height)

			if target.Width() != tt.width {
				t.Errorf("Width() = %d, want %d", target.Width(), tt.width)
			}
			if target.Height() != tt.height {
				t.Errorf("Height() = %d, want %d", target.Height(), tt.height)
			}
			if target.Format() != gpucontext.TextureFormatRGBA8Unorm {
				t.Errorf("Format() = %v, want RGBA8Unorm", target.Format())
			}
			if target.TextureView() != nil {
				t.Error("TextureView() should be nil for CPU target")
			}
			if target.Pixels() == nil {
				t.Error("Pixels() should not be nil for CPU target")
			}
			if target.Stride() != tt.width*4 {
				t.Errorf("Stride() = %d, want %d", target.Stride(), tt.width*4)
			}
		})
	}
}

func TestPixmapTargetFromImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 150))

	// Set a pixel
	img.SetRGBA(50, 50, color.RGBA{255, 0, 0, 255})

	target := NewPixmapTargetFromImage(img)

	if target.Width() != 200 {
		t.Errorf("Width() = %d, want 200", target.Width())
	}
	if target.Height() != 150 {
		t.Errorf("Height() = %d, want 150", target.Height())
	}

	// Verify the pixel is accessible
	pixel := target.GetPixel(50, 50)
	r, g, b, a := pixel.RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 || a>>8 != 255 {
		t.Errorf("GetPixel(50, 50) = %v, want red", pixel)
	}
}

func TestPixmapTargetClear(t *testing.T) {
	target := NewPixmapTarget(10, 10)

	// Clear to blue
	target.Clear(color.RGBA{0, 0, 255, 255})

	// Check all pixels
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			pixel := target.GetPixel(x, y).(color.RGBA)
			if pixel.R != 0 || pixel.G != 0 || pixel.B != 255 || pixel.A != 255 {
				t.Errorf("Pixel at (%d, %d) = %v, want blue", x, y, pixel)
			}
		}
	}
}

func TestPixmapTargetSetGetPixel(t *testing.T) {
	target := NewPixmapTarget(100, 100)

	tests := []struct {
		x, y int
		c    color.RGBA
	}{
		{0, 0, color.RGBA{255, 0, 0, 255}},
		{99, 99, color.RGBA{0, 255, 0, 255}},
		{50, 50, color.RGBA{0, 0, 255, 128}},
	}

	for _, tt := range tests {
		target.SetPixel(tt.x, tt.y, tt.c)
		got := target.GetPixel(tt.x, tt.y).(color.RGBA)

		if got != tt.c {
			t.Errorf("GetPixel(%d, %d) = %v, want %v", tt.x, tt.y, got, tt.c)
		}
	}
}

func TestPixmapTargetResize(t *testing.T) {
	target := NewPixmapTarget(100, 100)

	// Set a pixel
	target.SetPixel(50, 50, color.RGBA{255, 0, 0, 255})

	// Resize
	target.Resize(200, 150)

	if target.Width() != 200 {
		t.Errorf("Width() = %d, want 200", target.Width())
	}
	if target.Height() != 150 {
		t.Errorf("Height() = %d, want 150", target.Height())
	}

	// Old pixel should be gone (new image)
	pixel := target.GetPixel(50, 50).(color.RGBA)
	if pixel.A != 0 {
		t.Errorf("Pixel after resize should be transparent, got %v", pixel)
	}
}

func TestPixmapTargetImage(t *testing.T) {
	target := NewPixmapTarget(100, 100)
	target.Clear(color.White)

	img := target.Image()

	if img == nil {
		t.Fatal("Image() returned nil")
	}
	if img.Bounds().Dx() != 100 || img.Bounds().Dy() != 100 {
		t.Errorf("Image bounds = %v, want (100, 100)", img.Bounds())
	}

	// Modify through image, verify through target
	img.SetRGBA(10, 10, color.RGBA{255, 0, 0, 255})
	pixel := target.GetPixel(10, 10).(color.RGBA)
	if pixel.R != 255 {
		t.Error("Image and target should share memory")
	}
}

func TestTextureTarget(t *testing.T) {
	// Test with null device handle
	target, err := NewTextureTarget(NullDeviceHandle{}, 512, 512, gpucontext.TextureFormatRGBA8Unorm)
	if err != nil {
		t.Fatalf("NewTextureTarget() error = %v", err)
	}

	if target.Width() != 512 {
		t.Errorf("Width() = %d, want 512", target.Width())
	}
	if target.Height() != 512 {
		t.Errorf("Height() = %d, want 512", target.Height())
	}
	if target.Format() != gpucontext.TextureFormatRGBA8Unorm {
		t.Errorf("Format() = %v, want RGBA8Unorm", target.Format())
	}
	if target.Pixels() != nil {
		t.Error("Pixels() should be nil for GPU target")
	}
	if target.Stride() != 0 {
		t.Errorf("Stride() = %d, want 0 for GPU target", target.Stride())
	}

	// Destroy should not panic
	target.Destroy()
}

func TestSurfaceTarget(t *testing.T) {
	target := NewSurfaceTarget(800, 600, gpucontext.TextureFormatBGRA8Unorm, nil)

	if target.Width() != 800 {
		t.Errorf("Width() = %d, want 800", target.Width())
	}
	if target.Height() != 600 {
		t.Errorf("Height() = %d, want 600", target.Height())
	}
	if target.Format() != gpucontext.TextureFormatBGRA8Unorm {
		t.Errorf("Format() = %v, want BGRA8Unorm", target.Format())
	}
	if target.Pixels() != nil {
		t.Error("Pixels() should be nil for surface target")
	}
}

func TestRenderTargetInterface(t *testing.T) {
	// Verify all target types implement RenderTarget
	textureTarget, _ := NewTextureTarget(NullDeviceHandle{}, 100, 100, gpucontext.TextureFormatRGBA8Unorm)
	targets := []RenderTarget{
		NewPixmapTarget(100, 100),
		textureTarget,
		NewSurfaceTarget(100, 100, gpucontext.TextureFormatBGRA8Unorm, nil),
	}

	for i, target := range targets {
		if target.Width() != 100 {
			t.Errorf("target[%d].Width() = %d, want 100", i, target.Width())
		}
		if target.Height() != 100 {
			t.Errorf("target[%d].Height() = %d, want 100", i, target.Height())
		}
	}
}
