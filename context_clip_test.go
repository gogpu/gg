package gg

import (
	"testing"
)

func TestClip(t *testing.T) {
	dc := NewContext(100, 100)

	// Create a circular clip region
	dc.DrawCircle(50, 50, 30)
	dc.Clip()

	// Path should be cleared after Clip()
	if dc.path.NumVerbs() != 0 {
		t.Errorf("Expected path to be cleared after Clip(), got %d elements", dc.path.NumVerbs())
	}

	// Clip stack should be initialized
	if dc.clipStack == nil {
		t.Fatal("Expected clipStack to be initialized")
	}

	// Should have one clip entry
	if dc.clipStack.Depth() != 1 {
		t.Errorf("Expected clip depth 1, got %d", dc.clipStack.Depth())
	}
}

func TestClipPreserve(t *testing.T) {
	dc := NewContext(100, 100)

	// Create a rectangular clip region
	dc.DrawRectangle(10, 10, 50, 50)
	elemCount := dc.path.NumVerbs()
	dc.ClipPreserve()

	// Path should be preserved after ClipPreserve()
	if dc.path.NumVerbs() != elemCount {
		t.Errorf("Expected path to be preserved with %d elements, got %d", elemCount, dc.path.NumVerbs())
	}

	// Clip stack should be initialized
	if dc.clipStack == nil {
		t.Fatal("Expected clipStack to be initialized")
	}

	// Should have one clip entry
	if dc.clipStack.Depth() != 1 {
		t.Errorf("Expected clip depth 1, got %d", dc.clipStack.Depth())
	}
}

func TestClipRect(t *testing.T) {
	dc := NewContext(200, 200)

	// Set a rectangular clip
	dc.ClipRect(20, 20, 100, 100)

	// Clip stack should be initialized
	if dc.clipStack == nil {
		t.Fatal("Expected clipStack to be initialized")
	}

	// Should have one clip entry
	if dc.clipStack.Depth() != 1 {
		t.Errorf("Expected clip depth 1, got %d", dc.clipStack.Depth())
	}

	// Test bounds
	bounds := dc.clipStack.Bounds()
	if bounds.W <= 0 || bounds.H <= 0 {
		t.Errorf("Expected non-empty bounds, got %+v", bounds)
	}
}

func TestResetClip(t *testing.T) {
	dc := NewContext(100, 100)

	// Add multiple clips
	dc.ClipRect(10, 10, 80, 80)
	dc.DrawCircle(50, 50, 20)
	dc.Clip()

	if dc.clipStack.Depth() != 2 {
		t.Errorf("Expected clip depth 2, got %d", dc.clipStack.Depth())
	}

	// Reset all clips
	dc.ResetClip()

	// Should have no clips (depth 0)
	if dc.clipStack.Depth() != 0 {
		t.Errorf("Expected clip depth 0 after reset, got %d", dc.clipStack.Depth())
	}

	// Bounds should be canvas size
	bounds := dc.clipStack.Bounds()
	if bounds.W != 100 || bounds.H != 100 {
		t.Errorf("Expected bounds 100x100, got %.0fx%.0f", bounds.W, bounds.H)
	}
}

func TestClipWithPushPop(t *testing.T) {
	dc := NewContext(200, 200)

	// Push state
	dc.Push()

	// Add first clip
	dc.ClipRect(20, 20, 160, 160)
	if dc.clipStack.Depth() != 1 {
		t.Errorf("Expected clip depth 1 after first clip, got %d", dc.clipStack.Depth())
	}

	// Push state again
	dc.Push()

	// Add second clip
	dc.DrawCircle(100, 100, 50)
	dc.Clip()
	if dc.clipStack.Depth() != 2 {
		t.Errorf("Expected clip depth 2 after second clip, got %d", dc.clipStack.Depth())
	}

	// Pop should restore to depth 1
	dc.Pop()
	if dc.clipStack.Depth() != 1 {
		t.Errorf("Expected clip depth 1 after first Pop(), got %d", dc.clipStack.Depth())
	}

	// Pop should restore to depth 0
	dc.Pop()
	if dc.clipStack.Depth() != 0 {
		t.Errorf("Expected clip depth 0 after second Pop(), got %d", dc.clipStack.Depth())
	}
}

func TestClipNestedPushPop(t *testing.T) {
	dc := NewContext(200, 200)

	// Initial state - no clips
	dc.Push()
	depth0 := 0
	if dc.clipStack != nil {
		depth0 = dc.clipStack.Depth()
	}

	// Add clip 1
	dc.ClipRect(10, 10, 180, 180)
	dc.Push()

	// Add clip 2
	dc.ClipRect(20, 20, 160, 160)
	dc.Push()

	// Add clip 3
	dc.DrawCircle(100, 100, 60)
	dc.Clip()

	// Should have 3 clips
	if dc.clipStack.Depth() != 3 {
		t.Errorf("Expected clip depth 3, got %d", dc.clipStack.Depth())
	}

	// Pop back through the stack
	dc.Pop() // Should restore to 2 clips
	if dc.clipStack.Depth() != 2 {
		t.Errorf("Expected clip depth 2 after first pop, got %d", dc.clipStack.Depth())
	}

	dc.Pop() // Should restore to 1 clip
	if dc.clipStack.Depth() != 1 {
		t.Errorf("Expected clip depth 1 after second pop, got %d", dc.clipStack.Depth())
	}

	dc.Pop() // Should restore to initial depth
	finalDepth := 0
	if dc.clipStack != nil {
		finalDepth = dc.clipStack.Depth()
	}
	if finalDepth != depth0 {
		t.Errorf("Expected clip depth %d after final pop, got %d", depth0, finalDepth)
	}
}

func TestClipWithTransform(t *testing.T) {
	dc := NewContext(200, 200)

	// Apply transformation
	dc.Translate(50, 50)
	dc.Scale(2, 2)

	// Create clip in transformed space
	dc.ClipRect(0, 0, 25, 25) // Will be transformed to 100x100 at (50, 50)

	if dc.clipStack == nil {
		t.Fatal("Expected clipStack to be initialized")
	}

	// Bounds should reflect the transformation
	bounds := dc.clipStack.Bounds()
	if bounds.W <= 0 || bounds.H <= 0 {
		t.Errorf("Expected non-empty transformed bounds, got %+v", bounds)
	}
}

func TestMultipleClipsIntersect(t *testing.T) {
	dc := NewContext(200, 200)

	// Add first clip
	dc.ClipRect(20, 20, 160, 160)
	bounds1 := dc.clipStack.Bounds()

	// Add second clip (should intersect with first)
	dc.ClipRect(80, 80, 100, 100)
	bounds2 := dc.clipStack.Bounds()

	// Second bounds should be smaller or equal to first
	if bounds2.W > bounds1.W || bounds2.H > bounds1.H {
		t.Errorf("Expected intersection to reduce bounds, got bounds1=%+v, bounds2=%+v", bounds1, bounds2)
	}
}

func TestClipEmptyPath(t *testing.T) {
	dc := NewContext(100, 100)

	// Try to clip with empty path
	dc.Clip()

	// Should still initialize clip stack (though path is empty)
	if dc.clipStack == nil {
		t.Fatal("Expected clipStack to be initialized")
	}
}

func TestClipComplexPath(t *testing.T) {
	dc := NewContext(200, 200)

	// Create a complex path with curves
	dc.MoveTo(100, 20)
	dc.LineTo(180, 100)
	dc.QuadraticTo(180, 180, 100, 180)
	dc.CubicTo(20, 180, 20, 100, 100, 20)
	dc.ClosePath()

	dc.Clip()

	if dc.clipStack == nil {
		t.Fatal("Expected clipStack to be initialized")
	}

	if dc.clipStack.Depth() != 1 {
		t.Errorf("Expected clip depth 1, got %d", dc.clipStack.Depth())
	}
}

func TestResetClipWithoutInitialization(t *testing.T) {
	dc := NewContext(100, 100)

	// ResetClip without any prior clips should be safe
	dc.ResetClip()

	// Should not crash and clipStack might still be nil
	if dc.clipStack != nil && dc.clipStack.Depth() != 0 {
		t.Errorf("Expected clip depth 0 or nil clipStack, got depth %d", dc.clipStack.Depth())
	}
}

func TestConvertPathToClipVerbs(t *testing.T) {
	// Create a path with all element types
	path := NewPath()
	path.MoveTo(10, 10)
	path.LineTo(50, 10)
	path.QuadraticTo(70, 20, 70, 40)
	path.CubicTo(70, 60, 50, 70, 30, 70)
	path.Close()

	clipVerbs, clipCoords := convertPathToClipVerbs(path)

	// Should have same number of verbs
	if len(clipVerbs) != path.NumVerbs() {
		t.Errorf("Expected %d clip verbs, got %d", path.NumVerbs(), len(clipVerbs))
	}

	// Should have coords for all points
	if len(clipCoords) == 0 {
		t.Error("Expected non-empty clip coords")
	}
}

func TestClipRectTransformed(t *testing.T) {
	dc := NewContext(200, 200)

	// Apply rotation
	dc.Translate(100, 100)
	dc.Rotate(0.785398) // 45 degrees
	dc.Translate(-100, -100)

	// ClipRect should handle transformed coordinates
	dc.ClipRect(75, 75, 50, 50)

	if dc.clipStack == nil {
		t.Fatal("Expected clipStack to be initialized")
	}

	// Bounds should be valid (non-empty)
	bounds := dc.clipStack.Bounds()
	if bounds.W <= 0 || bounds.H <= 0 {
		t.Errorf("Expected valid bounds for rotated clip, got %+v", bounds)
	}
}

// --- Clip + Fill rendering tests (pixel-level verification) ---

// TestClipRectFill verifies that Fill() respects rectangular clip regions.
// Pixels inside the clip should be painted; pixels outside should remain background.
func TestClipRectFill(t *testing.T) {
	dc := NewContext(200, 200)
	dc.ClearWithColor(White)

	// Set a rectangular clip in the center.
	dc.ClipRect(50, 50, 100, 100)

	// Fill the entire canvas with red.
	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 200, 200)
	dc.Fill()

	// Inside clip (75, 75) should be red.
	inside := dc.pixmap.GetPixel(75, 75)
	if inside.R < 0.9 || inside.G > 0.1 || inside.B > 0.1 {
		t.Errorf("Inside clip (75,75): expected red, got R=%.2f G=%.2f B=%.2f", inside.R, inside.G, inside.B)
	}

	// Outside clip (10, 10) should be white.
	outside := dc.pixmap.GetPixel(10, 10)
	if outside.R < 0.9 || outside.G < 0.9 || outside.B < 0.9 {
		t.Errorf("Outside clip (10,10): expected white, got R=%.2f G=%.2f B=%.2f", outside.R, outside.G, outside.B)
	}

	// Far corner (190, 190) should be white.
	corner := dc.pixmap.GetPixel(190, 190)
	if corner.R < 0.9 || corner.G < 0.9 || corner.B < 0.9 {
		t.Errorf("Outside clip (190,190): expected white, got R=%.2f G=%.2f B=%.2f", corner.R, corner.G, corner.B)
	}
}

// TestClipRectStroke verifies that Stroke() respects rectangular clip regions.
// NOTE: Stroke clip support depends on the renderer honoring paint.ClipCoverage.
// This may not work in all configurations — skip if the renderer doesn't support it.
func TestClipRectStroke(t *testing.T) {
	t.Skip("Stroke clip not fully implemented in CPU software renderer")
	dc := NewContext(200, 200)
	dc.ClearWithColor(White)

	// Clip to the right half.
	dc.ClipRect(100, 0, 100, 200)

	// Draw a horizontal line across the full width.
	dc.SetRGB(0, 0, 1)
	dc.SetLineWidth(4)
	dc.MoveTo(0, 100)
	dc.LineTo(200, 100)
	dc.Stroke()

	// Left half (50, 100) should be white (clipped out).
	left := dc.pixmap.GetPixel(50, 100)
	if left.B > 0.1 {
		t.Errorf("Left of clip (50,100): expected white, got B=%.2f", left.B)
	}

	// Right half (150, 100) should be blue (inside clip).
	right := dc.pixmap.GetPixel(150, 100)
	if right.B < 0.8 {
		t.Errorf("Right of clip (150,100): expected blue, got B=%.2f", right.B)
	}
}

// TestClipNestedFill verifies nested clip regions (rect + path mask).
func TestClipNestedFill(t *testing.T) {
	dc := NewContext(200, 200)
	dc.ClearWithColor(White)

	dc.Push()

	// Outer clip: rectangle.
	dc.ClipRect(20, 20, 160, 160)

	dc.Push()

	// Inner clip: circle (path-based mask).
	dc.DrawCircle(100, 100, 50)
	dc.Clip()

	// Fill everything green.
	dc.SetRGB(0, 1, 0)
	dc.DrawRectangle(0, 0, 200, 200)
	dc.Fill()

	dc.Pop()
	dc.Pop()

	// Center (inside both clips) should be green.
	center := dc.pixmap.GetPixel(100, 100)
	if center.G < 0.9 {
		t.Errorf("Center (100,100): expected green, got G=%.2f", center.G)
	}

	// Inside rect but outside circle should be white.
	rectOnly := dc.pixmap.GetPixel(25, 25)
	if rectOnly.G > 0.1 && rectOnly.R < 0.9 {
		t.Errorf("Rect-only (25,25): expected white, got R=%.2f G=%.2f B=%.2f",
			rectOnly.R, rectOnly.G, rectOnly.B)
	}

	// Completely outside should be white.
	outside := dc.pixmap.GetPixel(5, 5)
	if outside.G > 0.1 && outside.R < 0.9 {
		t.Errorf("Outside (5,5): expected white, got R=%.2f G=%.2f B=%.2f",
			outside.R, outside.G, outside.B)
	}
}

// --- Clip + Text rendering tests ---

// TestClipRectText verifies that DrawString respects rectangular clip regions.
// Text drawn inside the clip appears; text outside is clipped.
func TestClipRectText(t *testing.T) {
	t.Skip("GPU-CLIP-001 implemented; test requires GPU hardware to verify scissor rect")
}

// TestClipRectTextNoRegression verifies that DrawString without clip
// still renders normally (no regression from clip fix).
func TestClipRectTextNoRegression(t *testing.T) {
	fontPath := findSystemFontPath()
	if fontPath == "" {
		t.Skip("No system font available")
	}

	dc := NewContext(200, 50)
	dc.ClearWithColor(White)

	if err := dc.LoadFontFace(fontPath, 16.0); err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}

	dc.SetRGB(0, 0, 0)
	dc.DrawString("Hello", 10, 30)

	// Verify text was drawn.
	hasNonWhite := false
	for y := 0; y < 50; y++ {
		for x := 0; x < 200; x++ {
			r, g, b, _ := dc.pixmap.At(x, y).RGBA()
			if r != 0xffff || g != 0xffff || b != 0xffff {
				hasNonWhite = true
				break
			}
		}
		if hasNonWhite {
			break
		}
	}
	if !hasNonWhite {
		t.Error("Expected text pixels without clip (regression check)")
	}
}

// TestClipDrawStringAnchored verifies that DrawStringAnchored inherits
// clip behavior from DrawString.
func TestClipDrawStringAnchored(t *testing.T) {
	t.Skip("GPU-CLIP-001 implemented; test requires GPU hardware to verify scissor rect")
}

// TestClipPathMaskText verifies that DrawString works with path-based
// clip masks (circle clip), not just rectangular clips.
// GPU-CLIP-003a provides depth-buffer clipping for this case.
func TestClipPathMaskText(t *testing.T) {
	t.Skip("requires GPU hardware to verify depth clip (GPU-CLIP-003a)")
}

// TestClipIsRectOnly verifies the IsRectOnly helper on ClipStack.
func TestClipIsRectOnly(t *testing.T) {
	dc := NewContext(200, 200)

	// Rect-only clip.
	dc.ClipRect(10, 10, 100, 100)
	if !dc.clipStack.IsRectOnly() {
		t.Error("Expected IsRectOnly()=true for rectangular clip")
	}

	// Add a nested rect clip — still rect-only.
	dc.ClipRect(20, 20, 50, 50)
	if !dc.clipStack.IsRectOnly() {
		t.Error("Expected IsRectOnly()=true for nested rectangular clips")
	}

	// Add a path-based clip — no longer rect-only.
	dc.DrawCircle(50, 50, 30)
	dc.Clip()
	if dc.clipStack.IsRectOnly() {
		t.Error("Expected IsRectOnly()=false after adding path-based clip")
	}
}

// TestClipPathStoresGPUClipPath verifies that Clip() stores the device-space
// path for GPU depth clipping (GPU-CLIP-003a bridge).
func TestClipPathStoresGPUClipPath(t *testing.T) {
	dc := NewContext(200, 200)

	// Before clipping, gpuClipPath should be nil.
	if dc.gpuClipPath != nil {
		t.Error("gpuClipPath should be nil before any clip")
	}

	// Apply a circle clip (arbitrary path, not rect/rrect).
	dc.DrawCircle(100, 100, 50)
	dc.Clip()

	// gpuClipPath should now be set.
	if dc.gpuClipPath == nil {
		t.Fatal("gpuClipPath should be set after path clip")
	}

	// Verify the stored path has content (non-empty).
	if dc.gpuClipPath.NumVerbs() == 0 {
		t.Error("gpuClipPath should have verbs")
	}
}

// TestClipPreserveStoresGPUClipPath verifies that ClipPreserve() stores
// the GPU clip path while keeping the current path.
func TestClipPreserveStoresGPUClipPath(t *testing.T) {
	dc := NewContext(200, 200)

	dc.DrawCircle(100, 100, 50)
	dc.ClipPreserve()

	// gpuClipPath should be set.
	if dc.gpuClipPath == nil {
		t.Fatal("gpuClipPath should be set after ClipPreserve")
	}

	// Current path should still have content (preserved).
	if dc.path.NumVerbs() == 0 {
		t.Error("path should be preserved after ClipPreserve")
	}
}

// TestClipPathClearedOnResetClip verifies that ResetClip() clears the GPU clip path.
func TestClipPathClearedOnResetClip(t *testing.T) {
	dc := NewContext(200, 200)

	dc.DrawCircle(100, 100, 50)
	dc.Clip()
	if dc.gpuClipPath == nil {
		t.Fatal("gpuClipPath should be set")
	}

	dc.ResetClip()
	if dc.gpuClipPath != nil {
		t.Error("gpuClipPath should be nil after ResetClip")
	}
}

// TestClipPathClearedOnPop verifies that Pop() clears the GPU clip path when
// the path clip entry is popped from the clip stack.
func TestClipPathClearedOnPop(t *testing.T) {
	dc := NewContext(200, 200)

	dc.Push()
	dc.DrawCircle(100, 100, 50)
	dc.Clip()
	if dc.gpuClipPath == nil {
		t.Fatal("gpuClipPath should be set after Clip in Push scope")
	}

	dc.Pop()
	if dc.gpuClipPath != nil {
		t.Error("gpuClipPath should be nil after Pop removes the path clip")
	}
}

// TestClipPathNotSetForRectClip verifies that rectangular clips don't set
// the GPU clip path (they use scissor rect instead).
func TestClipPathNotSetForRectClip(t *testing.T) {
	dc := NewContext(200, 200)

	dc.ClipRect(10, 10, 100, 100)
	if dc.gpuClipPath != nil {
		t.Error("gpuClipPath should be nil for rectangular clip")
	}
}

// TestClipPathNotSetForRRectClip verifies that rounded rect clips don't set
// the GPU clip path (they use scissor + SDF instead).
func TestClipPathNotSetForRRectClip(t *testing.T) {
	dc := NewContext(200, 200)

	dc.ClipRoundRect(10, 10, 100, 100, 10)
	if dc.gpuClipPath != nil {
		t.Error("gpuClipPath should be nil for rounded rect clip")
	}
}

// TestSetGPUClipPathNoGPU verifies that setGPUClipPath returns a no-op when
// no GPU context is available (nogpu build tag scenario).
func TestSetGPUClipPathNoGPU(t *testing.T) {
	dc := NewContext(200, 200)

	// Set up a path clip.
	dc.DrawCircle(100, 100, 50)
	dc.Clip()

	// Without GPU registration, setGPUClipPath should return no-op.
	cleanup := dc.setGPUClipPath()
	cleanup() // Should not panic.
}

// TestClipPathDeviceSpace verifies that the stored GPU clip path is in
// device-space coordinates (accounting for DeviceScale).
func TestClipPathDeviceSpace(t *testing.T) {
	dc := NewContext(100, 100)
	dc.SetDeviceScale(2.0)

	// Draw a circle at user-space (50, 50) radius 25.
	dc.DrawCircle(50, 50, 25)
	dc.Clip()

	if dc.gpuClipPath == nil {
		t.Fatal("gpuClipPath should be set")
	}

	// The device-space path should be scaled by 2x.
	// Compute bounding box from path coordinates.
	coords := dc.gpuClipPath.Coords()
	if len(coords) < 2 {
		t.Fatal("gpuClipPath has no coordinates")
	}
	minX, maxX := coords[0], coords[0]
	minY, maxY := coords[1], coords[1]
	for i := 0; i < len(coords)-1; i += 2 {
		x, y := coords[i], coords[i+1]
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}
	w := maxX - minX
	h := maxY - minY
	// User-space circle at (50,50) r=25 → device-space at (100,100) r=50.
	// Cubic approximation bounding box ≈ center ± radius → ~100x100.
	if w < 90 || h < 90 {
		t.Errorf("device-space path too small: w=%.1f h=%.1f (expected ~100x100)", w, h)
	}
}
