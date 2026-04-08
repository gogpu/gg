package gg

import (
	"image"
	"image/color"
	"testing"
)

// pixelAlpha returns the alpha component (0-65535) of the pixel at (x, y).
func pixelAlpha(img image.Image, x, y int) uint32 {
	c := img.At(x, y)
	//nolint:dogsled // we only need the alpha channel
	_, _, _, a := c.RGBA()
	return a
}

// pixelRGBA returns all four components (0-65535) of the pixel at (x, y).
func pixelRGBA(img image.Image, x, y int) (r, g, b, a uint32) {
	return img.At(x, y).RGBA()
}

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

// TestSetMask_Fill verifies that SetMask modulates Fill output.
// A full-coverage rect is drawn with a half-opaque mask.
func TestSetMask_Fill(t *testing.T) {
	const size = 100
	dc := NewContext(size, size)

	// Create a mask: left half = 255 (opaque), right half = 0 (transparent).
	mask := NewMask(size, size)
	for y := 0; y < size; y++ {
		for x := 0; x < size/2; x++ {
			mask.Set(x, y, 255)
		}
	}
	dc.SetMask(mask)

	// Fill the entire canvas with red.
	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, size, size)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}

	img := dc.Image()

	// Left half should have red pixels.
	r25, _, _, a25 := pixelRGBA(img, 25, 50)
	if r25 == 0 || a25 == 0 {
		t.Errorf("left half should have color, got r=%d a=%d", r25, a25)
	}

	// Right half should be transparent (mask=0).
	a75 := pixelAlpha(img, 75, 50)
	if a75 != 0 {
		t.Errorf("right half should be transparent, got a=%d", a75)
	}
}

// TestSetMask_Fill_PartialAlpha verifies that mask values between 0 and 255
// produce proportionally modulated output.
func TestSetMask_Fill_PartialAlpha(t *testing.T) {
	const size = 100
	dc := NewContext(size, size)

	// Create a mask with 128 (half) coverage everywhere.
	mask := NewMask(size, size)
	mask.Fill(128)
	dc.SetMask(mask)

	// Fill with opaque red.
	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, size, size)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}

	img := dc.Image()
	a := pixelAlpha(img, 50, 50)
	// Alpha should be roughly 128/255 * 65535 ≈ 32896. Allow tolerance.
	if a > 40000 || a < 25000 {
		t.Errorf("expected roughly half alpha (~32896), got %d", a)
	}
}

// TestSetMask_Stroke verifies that SetMask modulates Stroke output.
func TestSetMask_Stroke(t *testing.T) {
	const size = 100
	dc := NewContext(size, size)

	// Mask: top half = 255, bottom half = 0.
	mask := NewMask(size, size)
	for y := 0; y < size/2; y++ {
		for x := 0; x < size; x++ {
			mask.Set(x, y, 255)
		}
	}
	dc.SetMask(mask)

	// Draw a horizontal line through the middle (y=50 border, with stroke width).
	dc.SetRGBA(0, 0, 1, 1)
	dc.SetLineWidth(10)
	dc.MoveTo(0, 25)
	dc.LineTo(100, 25)
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke failed: %v", err)
	}

	// Also stroke in the masked-out area.
	dc.MoveTo(0, 75)
	dc.LineTo(100, 75)
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke failed: %v", err)
	}

	img := dc.Image()

	// Top line should be visible (mask = 255).
	_, _, b25, a25 := pixelRGBA(img, 50, 25)
	if b25 == 0 || a25 == 0 {
		t.Errorf("top stroke should be visible, got b=%d a=%d", b25, a25)
	}

	// Bottom line should be invisible (mask = 0).
	a75 := pixelAlpha(img, 50, 75)
	if a75 != 0 {
		t.Errorf("bottom stroke should be transparent, got a=%d", a75)
	}
}

// TestSetMask_WithClip verifies that mask and clip compose multiplicatively.
func TestSetMask_WithClip(t *testing.T) {
	const size = 100
	dc := NewContext(size, size)

	// Clip to right half.
	dc.DrawRectangle(50, 0, 50, 100)
	dc.Clip()

	// Mask: full canvas at half coverage.
	mask := NewMask(size, size)
	mask.Fill(128)
	dc.SetMask(mask)

	// Fill entire canvas with opaque green.
	dc.SetRGBA(0, 1, 0, 1)
	dc.DrawRectangle(0, 0, size, size)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}

	img := dc.Image()

	// Left half: clipped out entirely.
	aLeft := pixelAlpha(img, 25, 50)
	if aLeft != 0 {
		t.Errorf("left half (clipped) should be transparent, got a=%d", aLeft)
	}

	// Right half: visible but at reduced alpha (mask=128).
	_, g, _, aRight := pixelRGBA(img, 75, 50)
	if g == 0 || aRight == 0 {
		t.Errorf("right half should have color, got g=%d a=%d", g, aRight)
	}
	// Alpha should be roughly 128/255 of full.
	if aRight > 40000 {
		t.Errorf("right half alpha should be reduced by mask, got a=%d", aRight)
	}
}

// TestSetMask_PushPop verifies mask is saved and restored by Push/Pop.
func TestSetMask_PushPop(t *testing.T) {
	const size = 50
	dc := NewContext(size, size)

	// Set mask.
	mask1 := NewMask(size, size)
	mask1.Fill(100)
	dc.SetMask(mask1)

	dc.Push()

	// Change mask.
	mask2 := NewMask(size, size)
	mask2.Fill(200)
	dc.SetMask(mask2)

	// Fill with mask2.
	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, size, size)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}
	img1 := dc.Image()
	a1 := pixelAlpha(img1, 25, 25)

	dc.Pop()

	// Now mask1 should be restored. Draw again on fresh context.
	dc2 := NewContext(size, size)
	dc2.SetMask(dc.GetMask())
	dc2.SetRGBA(1, 0, 0, 1)
	dc2.DrawRectangle(0, 0, size, size)
	if err := dc2.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}
	img2 := dc2.Image()
	a2 := pixelAlpha(img2, 25, 25)

	// mask2 (200) should produce more alpha than mask1 (100).
	if a1 <= a2 {
		t.Errorf("mask2 (200) should produce more alpha than mask1 (100): a1=%d a2=%d", a1, a2)
	}
}

// TestAsMask_BeforeFill verifies AsMask returns valid data before Fill.
func TestAsMask_BeforeFill(t *testing.T) {
	dc := NewContext(100, 100)
	dc.DrawCircle(50, 50, 30)

	mask := dc.AsMask()
	if mask == nil {
		t.Fatal("AsMask should return non-nil mask")
	}

	// Center of the circle should have high alpha.
	center := mask.At(50, 50)
	if center < 200 {
		t.Errorf("expected high alpha at center, got %d", center)
	}

	// Far corner should have zero alpha.
	corner := mask.At(0, 0)
	if corner > 10 {
		t.Errorf("expected near-zero alpha at corner, got %d", corner)
	}
}

// TestAsMask_AfterFill verifies AsMask returns empty mask after Fill (path cleared).
func TestAsMask_AfterFill(t *testing.T) {
	dc := NewContext(100, 100)
	dc.DrawCircle(50, 50, 30)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}

	// Path was cleared by Fill, so AsMask should produce an empty mask.
	mask := dc.AsMask()
	if mask == nil {
		t.Fatal("AsMask should return non-nil mask")
	}

	center := mask.At(50, 50)
	if center != 0 {
		t.Errorf("expected 0 at center after Fill cleared path, got %d", center)
	}
}

// TestSetMask_Rider21Reproduction reproduces the exact scenario from gg#238.
// User creates a mask from a circle, sets it, then fills a rectangle.
// Without the fix, the mask was ignored and the full rectangle was drawn.
func TestSetMask_Rider21Reproduction(t *testing.T) {
	const size = 100
	dc := NewContext(size, size)

	// Step 1: Create a circular mask.
	dc.DrawCircle(50, 50, 30)
	mask := dc.AsMask()

	// Step 2: Set the mask and fill a full-screen rectangle.
	dc.SetMask(mask)
	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, size, size)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}

	img := dc.Image()

	// Inside the circle (center) should have color.
	rCenter, _, _, aCenter := pixelRGBA(img, 50, 50)
	if rCenter == 0 || aCenter == 0 {
		t.Errorf("center of circle should have color, got r=%d a=%d", rCenter, aCenter)
	}

	// Outside the circle (corners) should be transparent.
	aCorner := pixelAlpha(img, 5, 5)
	if aCorner != 0 {
		t.Errorf("corner (outside circle mask) should be transparent, got a=%d", aCorner)
	}

	aTopRight := pixelAlpha(img, 95, 5)
	if aTopRight != 0 {
		t.Errorf("top-right (outside circle mask) should be transparent, got a=%d", aTopRight)
	}

	aBottomRight := pixelAlpha(img, 95, 95)
	if aBottomRight != 0 {
		t.Errorf("bottom-right (outside circle mask) should be transparent, got a=%d", aBottomRight)
	}
}

// TestSetMask_NoMask verifies that without mask, Fill works as before.
func TestSetMask_NoMask(t *testing.T) {
	const size = 50
	dc := NewContext(size, size)

	// No mask set — fill should work normally.
	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, size, size)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}

	img := dc.Image()
	r, _, _, a := pixelRGBA(img, 25, 25)
	if r == 0 || a == 0 {
		t.Errorf("fill without mask should produce color, got r=%d a=%d", r, a)
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
