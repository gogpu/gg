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
	if len(dc.path.Elements()) != 0 {
		t.Errorf("Expected path to be cleared after Clip(), got %d elements", len(dc.path.Elements()))
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
	elemCount := len(dc.path.Elements())
	dc.ClipPreserve()

	// Path should be preserved after ClipPreserve()
	if len(dc.path.Elements()) != elemCount {
		t.Errorf("Expected path to be preserved with %d elements, got %d", elemCount, len(dc.path.Elements()))
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

func TestConvertPathElements(t *testing.T) {
	// Create a path with all element types
	path := NewPath()
	path.MoveTo(10, 10)
	path.LineTo(50, 10)
	path.QuadraticTo(70, 20, 70, 40)
	path.CubicTo(70, 60, 50, 70, 30, 70)
	path.Close()

	elements := path.Elements()
	converted := convertPathElements(elements)

	// Should have same number of elements
	if len(converted) != len(elements) {
		t.Errorf("Expected %d converted elements, got %d", len(elements), len(converted))
	}

	// Verify each type was converted correctly
	if len(converted) < 5 {
		t.Fatalf("Expected at least 5 elements, got %d", len(converted))
	}

	// Check types (order: MoveTo, LineTo, QuadTo, CubicTo, Close)
	typeNames := []string{"MoveTo", "LineTo", "QuadTo", "CubicTo", "Close"}
	for i, name := range typeNames {
		if converted[i] == nil {
			t.Errorf("Element %d (%s) is nil after conversion", i, name)
		}
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
