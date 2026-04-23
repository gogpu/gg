package gg

import (
	"image/color"
	"math"
	"testing"
)

// Tests for pixmap.go, context methods, coverage_filler.go, software.go helpers
// that target remaining uncovered functions.

// --- Pixmap tests ---

func TestPixmap_ColorModel(t *testing.T) {
	pm := NewPixmap(10, 10)
	model := pm.ColorModel()
	if model != color.RGBAModel {
		t.Errorf("ColorModel() = %v, want color.RGBAModel", model)
	}
}

func TestPixmap_At(t *testing.T) {
	pm := NewPixmap(10, 10)
	pm.SetPixel(5, 5, Red)

	// Valid pixel
	c := pm.At(5, 5)
	if c == nil {
		t.Fatal("At(5, 5) returned nil")
	}

	// Out of bounds should return transparent
	c = pm.At(-1, 0)
	r, g, b, a := c.RGBA()
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Errorf("At(-1, 0) = (%d, %d, %d, %d), want transparent", r, g, b, a)
	}

	c = pm.At(100, 0)
	r, g, b, a = c.RGBA()
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Errorf("At(100, 0) = (%d, %d, %d, %d), want transparent", r, g, b, a)
	}
}

func TestPixmap_GetPremul_OutOfBounds(t *testing.T) {
	pm := NewPixmap(10, 10)
	r, g, b, a := pm.getPremul(-1, -1)
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Errorf("getPremul(-1, -1) = (%v, %v, %v, %v), want (0, 0, 0, 0)", r, g, b, a)
	}

	r, g, b, a = pm.getPremul(100, 100)
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Errorf("getPremul(100, 100) = (%v, %v, %v, %v), want (0, 0, 0, 0)", r, g, b, a)
	}
}

// --- CoverageFiller registration tests ---

func TestRegisterCoverageFiller(t *testing.T) {
	// Save and restore the original filler
	original := GetCoverageFiller()
	defer func() {
		coverageMu.Lock()
		coverageFiller = original
		coverageMu.Unlock()
	}()

	// Register a mock filler
	mock := &mockCoverageFiller{}
	RegisterCoverageFiller(mock)

	got := GetCoverageFiller()
	if got != mock {
		t.Error("RegisterCoverageFiller did not set the filler")
	}

	// Register nil
	RegisterCoverageFiller(nil)
	if got := GetCoverageFiller(); got != nil {
		t.Error("RegisterCoverageFiller(nil) should clear the filler")
	}
}

type mockCoverageFiller struct{}

func (m *mockCoverageFiller) FillCoverage(_ *Path, _, _ int, _ FillRule,
	_ func(x, y int, coverage uint8)) {
	// No-op mock
}

// --- Context tests for remaining 0% functions ---

func TestContext_ResizeTarget(t *testing.T) {
	dc := NewContext(100, 100)
	pm := dc.ResizeTarget()
	if pm == nil {
		t.Fatal("ResizeTarget() returned nil")
	}
	if pm.Width() != 100 || pm.Height() != 100 {
		t.Errorf("ResizeTarget() size = %dx%d, want 100x100", pm.Width(), pm.Height())
	}
}

func TestContext_FlushGPU(t *testing.T) {
	dc := NewContext(10, 10)
	// With no accelerator registered, FlushGPU should be a no-op and return nil
	err := dc.FlushGPU()
	if err != nil {
		t.Errorf("FlushGPU() = %v, want nil (no accelerator)", err)
	}
}

func TestContext_setForceSDF(t *testing.T) {
	dc := NewContext(10, 10)
	// With no accelerator registered, should not panic
	dc.setForceSDF(true)
	dc.setForceSDF(false)
}

func TestContext_SetBlendMode(t *testing.T) {
	dc := NewContext(10, 10)
	// SetBlendMode is currently a no-op but should not panic
	dc.SetBlendMode(BlendMultiply)
	dc.SetBlendMode(BlendScreen)
}

// --- Software renderer helper tests ---

func TestPointLineDistance(t *testing.T) {
	tests := []struct {
		name                   string
		px, py, x0, y0, x1, y1 float64
		want                   float64
	}{
		{"point on line", 5, 0, 0, 0, 10, 0, 0},
		{"point above horizontal line", 5, 3, 0, 0, 10, 0, 3},
		{"point left of vertical line", 0, 5, 3, 0, 3, 10, 3},
		{"45 degree line", 0, 1, 0, 0, 1, 1, math.Sqrt(2) / 2},
		{"degenerate line (point)", 3, 4, 0, 0, 0, 0, 5}, // distance to origin
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pointLineDistance(tt.px, tt.py, tt.x0, tt.y0, tt.x1, tt.y1)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("pointLineDistance = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Accelerator tests ---

func TestSetAcceleratorDeviceProvider(t *testing.T) {
	// With no accelerator registered, should not panic
	SetAcceleratorDeviceProvider(nil)
}

func TestAcceleratorCanRenderDirect(t *testing.T) {
	// With no accelerator registered, should return false
	if AcceleratorCanRenderDirect() {
		t.Error("expected false with no accelerator")
	}
}
