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

// --- Phase 2 tests ---

func TestNewLuminanceMask(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 1))
	// Pure red: Y = 0.2126*255 = 54.2 → 54
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	// Pure green: Y = 0.7152*255 = 182.4 → 182
	img.Set(1, 0, color.RGBA{0, 255, 0, 255})
	// Pure blue: Y = 0.0722*255 = 18.4 → 18
	img.Set(2, 0, color.RGBA{0, 0, 255, 255})

	mask := NewLuminanceMask(img)
	if mask == nil {
		t.Fatal("expected non-nil mask")
	}
	if mask.Width() != 3 || mask.Height() != 1 {
		t.Fatalf("expected 3x1, got %dx%d", mask.Width(), mask.Height())
	}

	// Allow ±1 tolerance for rounding.
	tests := []struct {
		x    int
		want uint8
		name string
	}{
		{0, 54, "red luminance"},
		{1, 182, "green luminance"},
		{2, 18, "blue luminance"},
	}
	for _, tt := range tests {
		got := mask.At(tt.x, 0)
		diff := int(got) - int(tt.want)
		if diff < -1 || diff > 1 {
			t.Errorf("%s: got %d, want %d (±1)", tt.name, got, tt.want)
		}
	}
}

func TestNewLuminanceMask_White(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{255, 255, 255, 255})

	mask := NewLuminanceMask(img)
	got := mask.At(0, 0)
	// White luminance should be ~255.
	if got < 254 {
		t.Errorf("white luminance should be ~255, got %d", got)
	}
}

func TestNewLuminanceMask_Black(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{0, 0, 0, 255})

	mask := NewLuminanceMask(img)
	if mask.At(0, 0) != 0 {
		t.Errorf("black luminance should be 0, got %d", mask.At(0, 0))
	}
}

func TestNewMaskFromData(t *testing.T) {
	data := []byte{0, 64, 128, 192, 255, 0, 0, 0, 0, 0, 0, 0}
	mask := NewMaskFromData(data, 4, 3)
	if mask == nil {
		t.Fatal("expected non-nil mask")
	}
	if mask.Width() != 4 || mask.Height() != 3 {
		t.Fatalf("expected 4x3, got %dx%d", mask.Width(), mask.Height())
	}
	if mask.At(0, 0) != 0 {
		t.Errorf("expected 0, got %d", mask.At(0, 0))
	}
	if mask.At(1, 0) != 64 {
		t.Errorf("expected 64, got %d", mask.At(1, 0))
	}
	if mask.At(2, 0) != 128 {
		t.Errorf("expected 128, got %d", mask.At(2, 0))
	}

	// Verify independence: modifying original data doesn't affect mask.
	data[0] = 99
	if mask.At(0, 0) != 0 {
		t.Error("mask should be independent of original data")
	}
}

func TestNewMaskFromData_InvalidLength(t *testing.T) {
	data := []byte{1, 2, 3}
	mask := NewMaskFromData(data, 2, 2) // needs 4 bytes, got 3
	if mask != nil {
		t.Error("expected nil mask for invalid data length")
	}
}

func TestNewMaskFromData_RoundTrip(t *testing.T) {
	original := NewMask(10, 10)
	original.Fill(42)
	original.Set(5, 5, 200)

	reconstructed := NewMaskFromData(original.Data(), 10, 10)
	if reconstructed == nil {
		t.Fatal("expected non-nil mask")
	}
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if reconstructed.At(x, y) != original.At(x, y) {
				t.Fatalf("mismatch at (%d,%d): got %d, want %d",
					x, y, reconstructed.At(x, y), original.At(x, y))
			}
		}
	}
}

func TestApplyMask_Basic(t *testing.T) {
	const size = 100
	dc := NewContext(size, size)

	// Fill entire canvas with opaque red.
	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, size, size)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}

	// Create a mask: left half = 255, right half = 0.
	mask := NewMask(size, size)
	for y := 0; y < size; y++ {
		for x := 0; x < size/2; x++ {
			mask.Set(x, y, 255)
		}
	}

	dc.ApplyMask(mask)
	img := dc.Image()

	// Left half should still have red.
	r25, _, _, a25 := pixelRGBA(img, 25, 50)
	if r25 == 0 || a25 == 0 {
		t.Errorf("left half should have color after mask, got r=%d a=%d", r25, a25)
	}

	// Right half should be transparent (mask=0).
	a75 := pixelAlpha(img, 75, 50)
	if a75 != 0 {
		t.Errorf("right half should be transparent after mask, got a=%d", a75)
	}
}

func TestApplyMask_NilMask(t *testing.T) {
	dc := NewContext(50, 50)
	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, 50, 50)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}

	aBefore := pixelAlpha(dc.Image(), 25, 25)

	// ApplyMask(nil) should be a no-op.
	dc.ApplyMask(nil)

	aAfter := pixelAlpha(dc.Image(), 25, 25)
	if aBefore != aAfter {
		t.Errorf("nil mask should be no-op, alpha changed from %d to %d", aBefore, aAfter)
	}
}

func TestApplyMask_PartialAlpha(t *testing.T) {
	const size = 50
	dc := NewContext(size, size)

	// Fill with opaque red.
	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, size, size)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}

	// Apply mask with 128 everywhere.
	mask := NewMask(size, size)
	mask.Fill(128)
	dc.ApplyMask(mask)

	img := dc.Image()
	a := pixelAlpha(img, 25, 25)
	// Alpha should be roughly 128/255 * 65535 ≈ 32896. Allow tolerance.
	if a > 40000 || a < 25000 {
		t.Errorf("expected roughly half alpha (~32896), got %d", a)
	}
}

// --- Phase 3 tests ---

func TestPushMaskLayer_Basic(t *testing.T) {
	const size = 100
	dc := NewContext(size, size)

	// Create mask: circle in center.
	circleMask := NewMask(size, size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - 50
			dy := float64(y) - 50
			if dx*dx+dy*dy <= 30*30 {
				circleMask.Set(x, y, 255)
			}
		}
	}

	dc.PushMaskLayer(circleMask)

	// Draw full-screen red rect into the layer.
	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, size, size)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}

	dc.PopLayer()

	img := dc.Image()

	// Center should have red.
	rCenter, _, _, aCenter := pixelRGBA(img, 50, 50)
	if rCenter == 0 || aCenter == 0 {
		t.Errorf("center should have color, got r=%d a=%d", rCenter, aCenter)
	}

	// Corner should be transparent.
	aCorner := pixelAlpha(img, 5, 5)
	if aCorner != 0 {
		t.Errorf("corner should be transparent, got a=%d", aCorner)
	}
}

func TestPushMaskLayer_Nested(t *testing.T) {
	const size = 100
	dc := NewContext(size, size)

	// Outer mask: left half.
	outerMask := NewMask(size, size)
	for y := 0; y < size; y++ {
		for x := 0; x < size/2; x++ {
			outerMask.Set(x, y, 255)
		}
	}

	// Inner mask: top half.
	innerMask := NewMask(size, size)
	for y := 0; y < size/2; y++ {
		for x := 0; x < size; x++ {
			innerMask.Set(x, y, 255)
		}
	}

	dc.PushMaskLayer(outerMask)
	dc.PushMaskLayer(innerMask)

	// Fill everything red.
	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, size, size)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}

	dc.PopLayer() // inner mask applied: only top half visible in inner layer
	dc.PopLayer() // outer mask applied: only left half visible in outer layer

	img := dc.Image()

	// Top-left quadrant: should have color (both masks allow).
	aTL := pixelAlpha(img, 25, 25)
	if aTL == 0 {
		t.Error("top-left should have color")
	}

	// Top-right: inner allows, outer blocks.
	aTR := pixelAlpha(img, 75, 25)
	if aTR != 0 {
		t.Errorf("top-right should be transparent, got a=%d", aTR)
	}

	// Bottom-left: inner blocks.
	aBL := pixelAlpha(img, 25, 75)
	if aBL != 0 {
		t.Errorf("bottom-left should be transparent, got a=%d", aBL)
	}

	// Bottom-right: both block.
	aBR := pixelAlpha(img, 75, 75)
	if aBR != 0 {
		t.Errorf("bottom-right should be transparent, got a=%d", aBR)
	}
}

func TestPushMaskLayer_NilMask(t *testing.T) {
	const size = 50
	dc := NewContext(size, size)

	dc.PushMaskLayer(nil) // nil mask = PushLayer(Normal, 1.0)

	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, size, size)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}

	dc.PopLayer()

	// Everything should be visible (no mask applied).
	a := pixelAlpha(dc.Image(), 25, 25)
	if a == 0 {
		t.Error("nil mask should not block content")
	}
}

func TestPushMaskLayer_WithSetMask(t *testing.T) {
	// PushMaskLayer + SetMask should compose: SetMask applies per-shape,
	// PushMaskLayer applies to the entire layer on pop.
	const size = 100
	dc := NewContext(size, size)

	// Layer mask: left half.
	layerMask := NewMask(size, size)
	for y := 0; y < size; y++ {
		for x := 0; x < size/2; x++ {
			layerMask.Set(x, y, 255)
		}
	}

	// Per-shape mask: top half.
	shapeMask := NewMask(size, size)
	for y := 0; y < size/2; y++ {
		for x := 0; x < size; x++ {
			shapeMask.Set(x, y, 255)
		}
	}

	dc.PushMaskLayer(layerMask)
	dc.SetMask(shapeMask)

	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, size, size)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill failed: %v", err)
	}

	dc.ClearMask()
	dc.PopLayer()

	img := dc.Image()

	// Top-left: both allow → visible.
	aTL := pixelAlpha(img, 25, 25)
	if aTL == 0 {
		t.Error("top-left should have color (both masks allow)")
	}

	// Top-right: shape allows, layer blocks → transparent.
	aTR := pixelAlpha(img, 75, 25)
	if aTR != 0 {
		t.Errorf("top-right should be transparent (layer mask blocks), got a=%d", aTR)
	}

	// Bottom-left: shape blocks drawing into layer → transparent.
	aBL := pixelAlpha(img, 25, 75)
	if aBL != 0 {
		t.Errorf("bottom-left should be transparent (shape mask blocks), got a=%d", aBL)
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
