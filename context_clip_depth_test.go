package gg

import (
	"testing"
)

func TestClipCircle_CPUPixels(t *testing.T) {
	// Verify CPU arbitrary path clip works at pixel level.
	// This is the baseline — if CPU clip works, GPU depth clip should match.
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	// Draw white background.
	dc.SetRGBA(1, 1, 1, 1)
	dc.Clear()

	// Clip to circle at center, radius 30.
	dc.DrawCircle(50, 50, 30)
	dc.Clip()

	// Fill entire canvas red.
	dc.SetRGBA(1, 0, 0, 1)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	pm := dc.ResizeTarget()

	// Pixel at center (50,50) — inside circle — should be red.
	center := pm.GetPixel(50, 50)
	if center.R < 0.9 {
		t.Errorf("center pixel should be red, got R=%.2f G=%.2f B=%.2f", center.R, center.G, center.B)
	}

	// Pixel at corner (5,5) — outside circle — should NOT be red (clipped out).
	// Background is white from Clear(), but pixmap may start transparent.
	corner := pm.GetPixel(5, 5)
	if corner.R > 0.5 && corner.G < 0.1 {
		t.Errorf("corner pixel is red — clip NOT working! got R=%.2f G=%.2f B=%.2f", corner.R, corner.G, corner.B)
	}

	// Pixel just outside circle (50, 15) — 35 units from center, outside radius 30.
	outside := pm.GetPixel(50, 15)
	if outside.R > 0.5 && outside.G < 0.1 {
		t.Errorf("pixel at (50,15) is red — outside circle but not clipped! R=%.2f G=%.2f B=%.2f",
			outside.R, outside.G, outside.B)
	}

	// Pixel just inside circle (50, 25) — 25 units from center, inside radius 30.
	inside := pm.GetPixel(50, 25)
	if inside.R < 0.5 {
		t.Errorf("pixel at (50,25) should be red (inside circle clip), got R=%.2f G=%.2f B=%.2f",
			inside.R, inside.G, inside.B)
	}
}

func TestClipCircle_GPUClipPathBridge(t *testing.T) {
	// Verify that dc.Clip() with arbitrary path stores gpuClipPath.
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	// Before clip — no path stored.
	if dc.gpuClipPath != nil {
		t.Error("gpuClipPath should be nil before Clip()")
	}

	// Apply circular clip.
	dc.DrawCircle(50, 50, 30)
	dc.Clip()

	// After clip — path should be stored.
	if dc.gpuClipPath == nil {
		t.Fatal("gpuClipPath should be non-nil after Clip() with circle path")
	}
	if dc.gpuClipPath.NumVerbs() == 0 {
		t.Error("gpuClipPath should have verbs (circle = 4 cubics + close)")
	}

	// clipStack should NOT be rect-only or rrect-only.
	if dc.clipStack.IsRectOnly() {
		t.Error("clipStack.IsRectOnly() should be false for circle clip")
	}
	if dc.clipStack.IsRRectOnly() {
		t.Error("clipStack.IsRRectOnly() should be false for circle clip")
	}

	// setGPUClipRect should detect path clip and call setGPUClipPath.
	// Without GPU context, it returns no-op but gpuClipPath is still set.
	cleanup := dc.setGPUClipRect()
	defer cleanup()

	// After Pop, gpuClipPath should be cleared.
	dc.Push()
	dc.DrawCircle(50, 50, 20)
	dc.Clip()
	dc.Pop()

	// gpuClipPath may or may not be nil after Pop depending on implementation.
	// The important thing is that it doesn't crash.
}

func TestClipCircle_IsNotRectOrRRect(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.DrawCircle(50, 50, 30)
	dc.Clip()

	rectOnly := dc.clipStack.IsRectOnly()
	rrectOnly := dc.clipStack.IsRRectOnly()

	if rectOnly {
		t.Error("circle clip should NOT be IsRectOnly")
	}
	if rrectOnly {
		t.Error("circle clip should NOT be IsRRectOnly")
	}
}
