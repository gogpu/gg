// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"image/color"
	"testing"

	"github.com/gogpu/gputypes"
)

func TestNewLayeredPixmapTarget(t *testing.T) {
	target := NewLayeredPixmapTarget(800, 600)

	if target.Width() != 800 {
		t.Errorf("Width() = %d, want 800", target.Width())
	}
	if target.Height() != 600 {
		t.Errorf("Height() = %d, want 600", target.Height())
	}
	if target.Format() != gputypes.TextureFormatRGBA8Unorm {
		t.Errorf("Format() = %v, want RGBA8Unorm", target.Format())
	}
	if target.TextureView() != nil {
		t.Error("TextureView() should be nil for CPU target")
	}
	if target.Pixels() == nil {
		t.Error("Pixels() should not be nil")
	}
	if target.Stride() != 800*4 {
		t.Errorf("Stride() = %d, want %d", target.Stride(), 800*4)
	}
	if len(target.Layers()) != 0 {
		t.Errorf("Layers() = %v, want empty", target.Layers())
	}
}

func TestLayeredPixmapTargetCreateLayer(t *testing.T) {
	target := NewLayeredPixmapTarget(100, 100)

	// Create layers at different z-orders
	layer1, err := target.CreateLayer(1)
	if err != nil {
		t.Fatalf("CreateLayer(1) error = %v", err)
	}
	if layer1 == nil {
		t.Fatal("CreateLayer(1) returned nil")
	}
	if layer1.Width() != 100 || layer1.Height() != 100 {
		t.Errorf("Layer dimensions = (%d, %d), want (100, 100)", layer1.Width(), layer1.Height())
	}

	layer2, err := target.CreateLayer(5)
	if err != nil {
		t.Fatalf("CreateLayer(5) error = %v", err)
	}
	if layer2 == nil {
		t.Fatal("CreateLayer(5) returned nil")
	}

	layer3, err := target.CreateLayer(3)
	if err != nil {
		t.Fatalf("CreateLayer(3) error = %v", err)
	}
	if layer3 == nil {
		t.Fatal("CreateLayer(3) returned nil")
	}

	// Check z-order
	layers := target.Layers()
	if len(layers) != 3 {
		t.Fatalf("Layers() length = %d, want 3", len(layers))
	}
	expected := []int{1, 3, 5}
	for i, z := range expected {
		if layers[i] != z {
			t.Errorf("Layers()[%d] = %d, want %d", i, layers[i], z)
		}
	}
}

func TestLayeredPixmapTargetCreateLayerDuplicate(t *testing.T) {
	target := NewLayeredPixmapTarget(100, 100)

	_, err := target.CreateLayer(1)
	if err != nil {
		t.Fatalf("CreateLayer(1) error = %v", err)
	}

	// Creating layer with same z should fail
	_, err = target.CreateLayer(1)
	if err == nil {
		t.Error("CreateLayer(1) should fail for duplicate z")
	}
}

func TestLayeredPixmapTargetRemoveLayer(t *testing.T) {
	target := NewLayeredPixmapTarget(100, 100)

	_, _ = target.CreateLayer(1)
	_, _ = target.CreateLayer(2)
	_, _ = target.CreateLayer(3)

	// Remove middle layer
	err := target.RemoveLayer(2)
	if err != nil {
		t.Fatalf("RemoveLayer(2) error = %v", err)
	}

	layers := target.Layers()
	if len(layers) != 2 {
		t.Fatalf("Layers() length = %d, want 2", len(layers))
	}
	expected := []int{1, 3}
	for i, z := range expected {
		if layers[i] != z {
			t.Errorf("Layers()[%d] = %d, want %d", i, layers[i], z)
		}
	}

	// Remove non-existent layer should fail
	err = target.RemoveLayer(2)
	if err == nil {
		t.Error("RemoveLayer(2) should fail for non-existent layer")
	}
}

func TestLayeredPixmapTargetSetLayerVisible(t *testing.T) {
	target := NewLayeredPixmapTarget(100, 100)

	_, _ = target.CreateLayer(1)

	// Hide layer
	target.SetLayerVisible(1, false)

	// SetLayerVisible on non-existent layer should not panic
	target.SetLayerVisible(999, true)

	// Show layer
	target.SetLayerVisible(1, true)
}

func TestLayeredPixmapTargetComposite(t *testing.T) {
	target := NewLayeredPixmapTarget(10, 10)

	// Clear base to red
	target.Clear(color.RGBA{255, 0, 0, 255})

	// Create a layer and fill with semi-transparent green
	layer, _ := target.CreateLayer(1)
	pixmap := layer.(*PixmapTarget)
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			pixmap.SetPixel(x, y, color.RGBA{0, 255, 0, 128})
		}
	}

	// Composite
	target.Composite()

	// Check result - should be a blend of red and green
	img := target.Image()
	pixel := img.RGBAAt(5, 5)

	// With alpha=128, the result should be approximately:
	// R = (0 * 128 + 255 * (255 - 128)) / 255 = ~127
	// G = (255 * 128 + 0 * (255 - 128)) / 255 = ~128
	// Note: exact values depend on compositing algorithm

	// Just verify green is present (not still pure red)
	if pixel.G == 0 {
		t.Errorf("Pixel should have green component after composite, got %v", pixel)
	}
}

func TestLayeredPixmapTargetCompositeInvisible(t *testing.T) {
	target := NewLayeredPixmapTarget(10, 10)

	// Clear base to red
	target.Clear(color.RGBA{255, 0, 0, 255})

	// Create a layer and fill with blue
	layer, _ := target.CreateLayer(1)
	pixmap := layer.(*PixmapTarget)
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			pixmap.SetPixel(x, y, color.RGBA{0, 0, 255, 255})
		}
	}

	// Hide the layer
	target.SetLayerVisible(1, false)

	// Composite
	target.Composite()

	// Check result - should still be red (invisible layer not composited)
	img := target.Image()
	pixel := img.RGBAAt(5, 5)

	if pixel.R != 255 || pixel.G != 0 || pixel.B != 0 {
		t.Errorf("Pixel should be red (invisible layer), got %v", pixel)
	}
}

func TestLayeredPixmapTargetCompositeOrder(t *testing.T) {
	target := NewLayeredPixmapTarget(10, 10)

	// Clear base to black (transparent)
	target.Clear(color.RGBA{0, 0, 0, 0})

	// Create layer at z=1, fill with red
	layer1, _ := target.CreateLayer(1)
	pixmap1 := layer1.(*PixmapTarget)
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			pixmap1.SetPixel(x, y, color.RGBA{255, 0, 0, 255})
		}
	}

	// Create layer at z=2, fill with blue (should be on top)
	layer2, _ := target.CreateLayer(2)
	pixmap2 := layer2.(*PixmapTarget)
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			pixmap2.SetPixel(x, y, color.RGBA{0, 0, 255, 255})
		}
	}

	// Composite
	target.Composite()

	// Check result - should be blue (z=2 on top)
	img := target.Image()
	pixel := img.RGBAAt(5, 5)

	if pixel.B != 255 {
		t.Errorf("Pixel should be blue (z=2 on top), got %v", pixel)
	}
}

func TestLayeredPixmapTargetClearLayer(t *testing.T) {
	target := NewLayeredPixmapTarget(10, 10)

	_, _ = target.CreateLayer(1)

	// Clear layer to green
	err := target.ClearLayer(1, color.RGBA{0, 255, 0, 255})
	if err != nil {
		t.Fatalf("ClearLayer(1) error = %v", err)
	}

	// Get layer and check pixel
	layer := target.GetLayer(1)
	pixmap := layer.(*PixmapTarget)
	pixel := pixmap.GetPixel(5, 5).(color.RGBA)

	if pixel.G != 255 {
		t.Errorf("Layer pixel should be green, got %v", pixel)
	}

	// Clear non-existent layer should fail
	err = target.ClearLayer(999, color.White)
	if err == nil {
		t.Error("ClearLayer(999) should fail for non-existent layer")
	}
}

func TestLayeredPixmapTargetGetLayer(t *testing.T) {
	target := NewLayeredPixmapTarget(100, 100)

	_, _ = target.CreateLayer(1)

	// Get existing layer
	layer := target.GetLayer(1)
	if layer == nil {
		t.Error("GetLayer(1) should return layer")
	}

	// Get non-existent layer
	layer = target.GetLayer(999)
	if layer != nil {
		t.Error("GetLayer(999) should return nil for non-existent layer")
	}
}

func TestLayeredPixmapTargetLayersReturnsNewSlice(t *testing.T) {
	target := NewLayeredPixmapTarget(100, 100)

	_, _ = target.CreateLayer(1)
	_, _ = target.CreateLayer(2)

	// Get layers and modify
	layers := target.Layers()
	layers[0] = 999

	// Original should be unchanged
	layers2 := target.Layers()
	if layers2[0] != 1 {
		t.Error("Layers() should return a copy, not the original slice")
	}
}

func TestLayeredTargetInterface(t *testing.T) {
	// Verify LayeredPixmapTarget implements LayeredTarget
	var _ LayeredTarget = (*LayeredPixmapTarget)(nil)

	// Verify it can be used through the interface
	var target LayeredTarget = NewLayeredPixmapTarget(100, 100)

	layer, err := target.CreateLayer(1)
	if err != nil {
		t.Fatalf("CreateLayer error = %v", err)
	}
	if layer.Width() != 100 {
		t.Errorf("Layer width = %d, want 100", layer.Width())
	}

	target.SetLayerVisible(1, false)
	target.Composite()

	layers := target.Layers()
	if len(layers) != 1 || layers[0] != 1 {
		t.Errorf("Layers = %v, want [1]", layers)
	}

	err = target.RemoveLayer(1)
	if err != nil {
		t.Errorf("RemoveLayer error = %v", err)
	}
}
