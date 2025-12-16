package gg

import (
	"testing"
)

// TestPushPopLayer tests basic layer push/pop functionality.
func TestPushPopLayer(t *testing.T) {
	dc := NewContext(100, 100)
	originalPixmap := dc.pixmap

	dc.PushLayer(BlendNormal, 1.0)

	if dc.pixmap == originalPixmap {
		t.Error("PushLayer should create a new pixmap")
	}
	if dc.layerStack == nil {
		t.Error("PushLayer should initialize layer stack")
	}
	if len(dc.layerStack.layers) != 1 {
		t.Errorf("Expected 1 layer, got %d", len(dc.layerStack.layers))
	}
	if dc.basePixmap == nil {
		t.Error("PushLayer should save base pixmap")
	}

	dc.PopLayer()

	if dc.pixmap != originalPixmap {
		t.Error("PopLayer should restore original pixmap")
	}
	if len(dc.layerStack.layers) != 0 {
		t.Errorf("Expected 0 layers after pop, got %d", len(dc.layerStack.layers))
	}
	if dc.basePixmap != nil {
		t.Error("PopLayer should clear base pixmap")
	}
}

// TestNestedLayers tests nested push/pop operations.
func TestNestedLayers(t *testing.T) {
	dc := NewContext(100, 100)

	dc.PushLayer(BlendNormal, 1.0)
	layer1Pixmap := dc.pixmap

	dc.PushLayer(BlendMultiply, 0.5)
	layer2Pixmap := dc.pixmap

	if layer1Pixmap == layer2Pixmap {
		t.Error("Nested layers should have different pixmaps")
	}
	if len(dc.layerStack.layers) != 2 {
		t.Errorf("Expected 2 layers, got %d", len(dc.layerStack.layers))
	}

	dc.PopLayer()

	if dc.pixmap != layer1Pixmap {
		t.Error("PopLayer should restore to layer 1 pixmap")
	}
	if len(dc.layerStack.layers) != 1 {
		t.Errorf("Expected 1 layer after first pop, got %d", len(dc.layerStack.layers))
	}

	dc.PopLayer()

	if len(dc.layerStack.layers) != 0 {
		t.Errorf("Expected 0 layers after second pop, got %d", len(dc.layerStack.layers))
	}
}

// TestLayerCompositing tests layer compositing with SetPixel.
func TestLayerCompositing(t *testing.T) {
	dc := NewContext(10, 10)
	dc.ClearWithColor(White)

	dc.PushLayer(BlendNormal, 1.0)
	dc.SetPixel(5, 5, RGBA{R: 1, G: 0, B: 0, A: 1})

	layerPixel := dc.pixmap.GetPixel(5, 5)
	if layerPixel.R != 1.0 {
		t.Fatalf("Layer pixel should be red, got R=%f", layerPixel.R)
	}

	dc.PopLayer()

	pixel := dc.pixmap.GetPixel(5, 5)
	tolerance := 0.1
	if abs(pixel.R-1.0) > tolerance {
		t.Errorf("Expected R ~1.0, got %f", pixel.R)
	}
}

// TestPopWithoutPush tests that PopLayer doesn't crash when no layer is pushed.
func TestPopWithoutPush(t *testing.T) {
	dc := NewContext(100, 100)
	dc.PopLayer() // Should not crash
	if dc.pixmap == nil {
		t.Error("PopLayer without PushLayer should not modify pixmap")
	}
}

// TestLayerOpacityClamping tests that opacity is clamped to [0, 1].
func TestLayerOpacityClamping(t *testing.T) {
	dc := NewContext(100, 100)

	dc.PushLayer(BlendNormal, -0.5)
	if dc.layerStack.layers[0].opacity != 0 {
		t.Errorf("Expected opacity 0, got %f", dc.layerStack.layers[0].opacity)
	}
	dc.PopLayer()

	dc.PushLayer(BlendNormal, 1.5)
	if dc.layerStack.layers[0].opacity != 1 {
		t.Errorf("Expected opacity 1, got %f", dc.layerStack.layers[0].opacity)
	}
	dc.PopLayer()

	dc.PushLayer(BlendNormal, 0.7)
	if dc.layerStack.layers[0].opacity != 0.7 {
		t.Errorf("Expected opacity 0.7, got %f", dc.layerStack.layers[0].opacity)
	}
	dc.PopLayer()
}

// TestLayerClearTransparent tests that new layers start transparent.
func TestLayerClearTransparent(t *testing.T) {
	dc := NewContext(10, 10)
	dc.ClearWithColor(White)

	dc.PushLayer(BlendNormal, 1.0)

	pixel := dc.pixmap.GetPixel(5, 5)
	if pixel.A != 0 {
		t.Errorf("New layer should be transparent, got alpha %f", pixel.A)
	}

	dc.PopLayer()
}

// TestMultipleLayerCycles tests multiple push/pop cycles.
func TestMultipleLayerCycles(t *testing.T) {
	dc := NewContext(50, 50)

	for i := 0; i < 5; i++ {
		dc.PushLayer(BlendNormal, 1.0)
		dc.SetPixel(25, 25, RGBA{R: float64(i) / 5.0, G: 0, B: 0, A: 1})
		dc.PopLayer()
	}

	if len(dc.layerStack.layers) != 0 {
		t.Errorf("Expected 0 layers after cycles, got %d", len(dc.layerStack.layers))
	}
	if dc.basePixmap != nil {
		t.Error("Expected basePixmap to be nil after all layers popped")
	}
}

// TestSetBlendMode tests the SetBlendMode method.
func TestSetBlendMode(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetBlendMode(BlendMultiply)
	dc.SetBlendMode(BlendScreen)
	dc.SetBlendMode(BlendOverlay)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
