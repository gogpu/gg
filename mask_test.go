package gg

import (
	"image"
	"image/color"
	"testing"
)

func TestNewMask(t *testing.T) {
	mask := NewMask(100, 100)
	if mask.Width() != 100 || mask.Height() != 100 {
		t.Errorf("expected 100x100, got %dx%d", mask.Width(), mask.Height())
	}

	// All values should be 0
	if mask.At(50, 50) != 0 {
		t.Errorf("expected 0, got %d", mask.At(50, 50))
	}
}

func TestMaskFill(t *testing.T) {
	mask := NewMask(100, 100)
	mask.Fill(128)

	if mask.At(50, 50) != 128 {
		t.Errorf("expected 128, got %d", mask.At(50, 50))
	}
}

func TestMaskInvert(t *testing.T) {
	mask := NewMask(100, 100)
	mask.Fill(100)
	mask.Invert()

	if mask.At(50, 50) != 155 {
		t.Errorf("expected 155, got %d", mask.At(50, 50))
	}
}

func TestMaskClone(t *testing.T) {
	mask := NewMask(100, 100)
	mask.Fill(200)

	clone := mask.Clone()
	mask.Fill(0) // Modify original

	if clone.At(50, 50) != 200 {
		t.Errorf("clone should not be affected, expected 200, got %d", clone.At(50, 50))
	}
}

func TestMaskBounds(t *testing.T) {
	mask := NewMask(100, 100)

	// Out of bounds should return 0
	if mask.At(-1, 50) != 0 {
		t.Error("expected 0 for out of bounds (negative x)")
	}
	if mask.At(100, 50) != 0 {
		t.Error("expected 0 for out of bounds (x >= width)")
	}
	if mask.At(50, -1) != 0 {
		t.Error("expected 0 for out of bounds (negative y)")
	}
	if mask.At(50, 100) != 0 {
		t.Error("expected 0 for out of bounds (y >= height)")
	}
}

func TestMaskSet(t *testing.T) {
	mask := NewMask(100, 100)

	// Set value
	mask.Set(50, 50, 128)
	if mask.At(50, 50) != 128 {
		t.Errorf("expected 128, got %d", mask.At(50, 50))
	}

	// Set out of bounds should be ignored
	mask.Set(-1, 50, 255)
	mask.Set(100, 50, 255)
	mask.Set(50, -1, 255)
	mask.Set(50, 100, 255)
	// No panic expected
}

func TestMaskClear(t *testing.T) {
	mask := NewMask(100, 100)
	mask.Fill(255)
	mask.Clear()

	if mask.At(50, 50) != 0 {
		t.Errorf("expected 0 after clear, got %d", mask.At(50, 50))
	}
}

func TestMaskBoundsRect(t *testing.T) {
	mask := NewMask(100, 200)
	bounds := mask.Bounds()

	if bounds.Min.X != 0 || bounds.Min.Y != 0 {
		t.Errorf("expected min (0,0), got (%d,%d)", bounds.Min.X, bounds.Min.Y)
	}
	if bounds.Max.X != 100 || bounds.Max.Y != 200 {
		t.Errorf("expected max (100,200), got (%d,%d)", bounds.Max.X, bounds.Max.Y)
	}
}

func TestMaskData(t *testing.T) {
	mask := NewMask(10, 10)
	mask.Set(5, 5, 100)

	data := mask.Data()
	if len(data) != 100 {
		t.Errorf("expected data length 100, got %d", len(data))
	}

	// Verify the value is at the correct offset
	if data[5*10+5] != 100 {
		t.Errorf("expected 100 at offset 55, got %d", data[55])
	}
}

func TestContextMask(t *testing.T) {
	dc := NewContext(100, 100)

	// Initially no mask
	if dc.GetMask() != nil {
		t.Error("expected nil mask initially")
	}

	// Set mask
	mask := NewMask(100, 100)
	mask.Fill(255)
	dc.SetMask(mask)

	if dc.GetMask() != mask {
		t.Error("expected mask to be set")
	}

	// Clear mask
	dc.ClearMask()
	if dc.GetMask() != nil {
		t.Error("expected nil mask after clear")
	}
}

func TestContextInvertMask(t *testing.T) {
	dc := NewContext(100, 100)

	// InvertMask with no mask should not panic
	dc.InvertMask()

	// Set and invert
	mask := NewMask(100, 100)
	mask.Fill(100)
	dc.SetMask(mask)
	dc.InvertMask()

	if dc.GetMask().At(50, 50) != 155 {
		t.Errorf("expected 155, got %d", dc.GetMask().At(50, 50))
	}
}

func TestContextAsMask(t *testing.T) {
	dc := NewContext(100, 100)
	dc.DrawCircle(50, 50, 30)

	mask := dc.AsMask()

	if mask == nil {
		t.Fatal("expected non-nil mask")
	}

	// Center of circle should have high alpha
	center := mask.At(50, 50)
	if center < 200 {
		t.Errorf("expected high alpha at center, got %d", center)
	}

	// Corner should have low/zero alpha
	corner := mask.At(0, 0)
	if corner > 50 {
		t.Errorf("expected low alpha at corner, got %d", corner)
	}
}

func TestNewMaskFromAlpha(t *testing.T) {
	// Create an image with varying alpha
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	img.Set(5, 5, color.RGBA{255, 0, 0, 200})

	mask := NewMaskFromAlpha(img)

	if mask.At(5, 5) != 200 {
		t.Errorf("expected 200, got %d", mask.At(5, 5))
	}
	if mask.At(0, 0) != 0 {
		t.Errorf("expected 0, got %d", mask.At(0, 0))
	}
}

func TestMaskPushPop(t *testing.T) {
	dc := NewContext(100, 100)

	// Set initial mask
	mask1 := NewMask(100, 100)
	mask1.Fill(100)
	dc.SetMask(mask1)

	// Push state
	dc.Push()

	// Modify mask
	dc.InvertMask()
	if dc.GetMask().At(50, 50) != 155 {
		t.Errorf("expected 155 after invert, got %d", dc.GetMask().At(50, 50))
	}

	// Pop state - should restore original mask
	dc.Pop()
	if dc.GetMask() == nil {
		t.Fatal("expected mask after pop")
	}
	if dc.GetMask().At(50, 50) != 100 {
		t.Errorf("expected 100 after pop, got %d", dc.GetMask().At(50, 50))
	}
}

func TestMaskPushPopNil(t *testing.T) {
	dc := NewContext(100, 100)

	// No mask initially
	dc.Push()

	// Set mask
	mask := NewMask(100, 100)
	mask.Fill(255)
	dc.SetMask(mask)

	// Pop should restore nil mask
	dc.Pop()
	if dc.GetMask() != nil {
		t.Error("expected nil mask after pop")
	}
}

func TestMaskNestedPushPop(t *testing.T) {
	dc := NewContext(100, 100)

	// Set initial mask
	mask1 := NewMask(100, 100)
	mask1.Fill(50)
	dc.SetMask(mask1)

	// Push first level
	dc.Push()

	// Change mask
	mask2 := NewMask(100, 100)
	mask2.Fill(100)
	dc.SetMask(mask2)

	// Push second level
	dc.Push()

	// Change mask again
	mask3 := NewMask(100, 100)
	mask3.Fill(150)
	dc.SetMask(mask3)

	// Verify current mask
	if dc.GetMask().At(50, 50) != 150 {
		t.Errorf("expected 150, got %d", dc.GetMask().At(50, 50))
	}

	// Pop to second level
	dc.Pop()
	if dc.GetMask().At(50, 50) != 100 {
		t.Errorf("expected 100, got %d", dc.GetMask().At(50, 50))
	}

	// Pop to first level
	dc.Pop()
	if dc.GetMask().At(50, 50) != 50 {
		t.Errorf("expected 50, got %d", dc.GetMask().At(50, 50))
	}
}
