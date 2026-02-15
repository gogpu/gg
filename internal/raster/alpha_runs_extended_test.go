// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package raster

import (
	"testing"
)

// TestAlphaRuns_NewAndReset tests creation and reset.
func TestAlphaRuns_NewAndReset(t *testing.T) {
	ar := NewAlphaRuns(100)
	if ar.Width() != 100 {
		t.Errorf("Width = %d, want 100", ar.Width())
	}
	if !ar.IsEmpty() {
		t.Error("new AlphaRuns should be empty")
	}

	// Reset should keep it empty
	ar.Reset()
	if !ar.IsEmpty() {
		t.Error("after Reset should be empty")
	}
}

// TestAlphaRuns_ZeroWidth tests edge case with zero/negative width.
func TestAlphaRuns_ZeroWidth(t *testing.T) {
	ar := NewAlphaRuns(0)
	if ar.Width() != 1 {
		t.Errorf("Width for 0 input = %d, want 1 (minimum)", ar.Width())
	}

	ar2 := NewAlphaRuns(-5)
	if ar2.Width() != 1 {
		t.Errorf("Width for -5 input = %d, want 1 (minimum)", ar2.Width())
	}
}

// TestAlphaRuns_Add tests coverage insertion.
func TestAlphaRuns_Add(t *testing.T) {
	ar := NewAlphaRuns(100)

	// Add coverage at x=10, startAlpha=128, 5 middle pixels, endAlpha=64
	ar.Add(10, 128, 5, 64)

	// Check start alpha
	alpha := ar.GetAlpha(10)
	if alpha != 128 {
		t.Errorf("GetAlpha(10) = %d, want 128", alpha)
	}

	// Check middle pixels (should be 255)
	for x := 11; x <= 15; x++ {
		a := ar.GetAlpha(x)
		if a != 255 {
			t.Errorf("GetAlpha(%d) = %d, want 255", x, a)
		}
	}

	// Check end alpha
	alpha = ar.GetAlpha(16)
	if alpha != 64 {
		t.Errorf("GetAlpha(16) = %d, want 64", alpha)
	}

	// Outside coverage should be 0
	if a := ar.GetAlpha(0); a != 0 {
		t.Errorf("GetAlpha(0) = %d, want 0", a)
	}
	if a := ar.GetAlpha(50); a != 0 {
		t.Errorf("GetAlpha(50) = %d, want 0", a)
	}
}

// TestAlphaRuns_AddOutOfBounds tests out-of-bounds add.
func TestAlphaRuns_AddOutOfBounds(t *testing.T) {
	ar := NewAlphaRuns(50)

	// Should not panic
	ar.Add(-1, 128, 5, 64)
	ar.Add(50, 128, 5, 64)
	ar.Add(100, 128, 5, 64)
}

// TestAlphaRuns_AddWithCoverage tests coverage with custom max value.
func TestAlphaRuns_AddWithCoverage(t *testing.T) {
	ar := NewAlphaRuns(100)

	// startAlpha=0, middleCount=5, endAlpha=0, maxValue=128
	// With startAlpha=0, middle starts at x=10 and covers 5 pixels: 10,11,12,13,14
	ar.AddWithCoverage(10, 0, 5, 0, 128) // maxValue = 128 instead of 255

	// Middle pixels (10-14) should have 128
	for x := 10; x <= 14; x++ {
		a := ar.GetAlpha(x)
		if a != 128 {
			t.Errorf("GetAlpha(%d) = %d, want 128 (custom maxValue)", x, a)
		}
	}
	// Pixel 15 should be 0 (past middle range)
	if a := ar.GetAlpha(15); a != 0 {
		t.Errorf("GetAlpha(15) = %d, want 0 (past middle)", a)
	}
}

// TestAlphaRuns_AddWithCoverageOutOfBounds tests out-of-bounds with coverage.
func TestAlphaRuns_AddWithCoverageOutOfBounds(t *testing.T) {
	ar := NewAlphaRuns(50)

	// Should not panic
	ar.AddWithCoverage(-5, 128, 3, 64, 200)
	ar.AddWithCoverage(55, 128, 3, 64, 200)
}

// TestAlphaRuns_Accumulation tests that multiple Add calls accumulate.
func TestAlphaRuns_Accumulation(t *testing.T) {
	ar := NewAlphaRuns(100)

	ar.Add(10, 100, 0, 0)
	ar.SetOffset(0) // Reset offset for next add
	ar.Add(10, 100, 0, 0)

	alpha := ar.GetAlpha(10)
	if alpha != 200 {
		t.Errorf("accumulated alpha = %d, want 200 (100+100)", alpha)
	}
}

// TestAlphaRuns_OverflowAccumulation tests overflow protection.
func TestAlphaRuns_OverflowAccumulation(t *testing.T) {
	ar := NewAlphaRuns(100)

	ar.Add(10, 200, 0, 0)
	ar.SetOffset(0)
	ar.Add(10, 200, 0, 0) // 200+200 = 400, should clamp to 255

	alpha := ar.GetAlpha(10)
	if alpha != 255 {
		t.Errorf("overflow alpha = %d, want 255", alpha)
	}
}

// TestAlphaRuns_GetAlphaOutOfBounds tests out-of-bounds access.
func TestAlphaRuns_GetAlphaOutOfBounds(t *testing.T) {
	ar := NewAlphaRuns(50)

	if a := ar.GetAlpha(-1); a != 0 {
		t.Errorf("GetAlpha(-1) = %d, want 0", a)
	}
	if a := ar.GetAlpha(50); a != 0 {
		t.Errorf("GetAlpha(50) = %d, want 0", a)
	}
	if a := ar.GetAlpha(100); a != 0 {
		t.Errorf("GetAlpha(100) = %d, want 0", a)
	}
}

// TestAlphaRuns_Clear tests clearing.
func TestAlphaRuns_Clear(t *testing.T) {
	ar := NewAlphaRuns(100)
	ar.Add(10, 200, 20, 100)

	ar.Clear()
	if !ar.IsEmpty() {
		t.Error("after Clear should be empty")
	}
}

// TestAlphaRuns_CopyTo tests copying coverage to a buffer.
func TestAlphaRuns_CopyTo(t *testing.T) {
	ar := NewAlphaRuns(20)
	ar.Add(5, 128, 3, 64) // x=5: 128, x=6,7,8: 255, x=9: 64

	dst := make([]uint8, 20)
	ar.CopyTo(dst)

	if dst[5] != 128 {
		t.Errorf("dst[5] = %d, want 128", dst[5])
	}
	for x := 6; x <= 8; x++ {
		if dst[x] != 255 {
			t.Errorf("dst[%d] = %d, want 255", x, dst[x])
		}
	}
	if dst[9] != 64 {
		t.Errorf("dst[9] = %d, want 64", dst[9])
	}
	if dst[0] != 0 {
		t.Errorf("dst[0] = %d, want 0", dst[0])
	}
}

// TestAlphaRuns_CopyToSmallBuffer tests CopyTo with insufficient buffer.
func TestAlphaRuns_CopyToSmallBuffer(t *testing.T) {
	ar := NewAlphaRuns(100)
	ar.Add(10, 128, 5, 64)

	// Buffer too small - should be a no-op (not panic)
	small := make([]uint8, 5)
	ar.CopyTo(small)
	// No assertion needed; just verify no panic
}

// TestAlphaRuns_Iter tests the pixel-level iterator.
func TestAlphaRuns_Iter(t *testing.T) {
	ar := NewAlphaRuns(20)
	ar.Add(3, 128, 2, 0)

	type pixel struct {
		x     int
		alpha uint8
	}
	pixels := make([]pixel, 0, ar.Width())
	for x, alpha := range ar.Iter() {
		pixels = append(pixels, pixel{x, alpha})
	}

	// Should have at least 1 pixel with non-zero alpha
	if len(pixels) == 0 {
		t.Error("Iter() produced no pixels")
	}

	// All yielded pixels should have alpha > 0
	for _, p := range pixels {
		if p.alpha == 0 {
			t.Errorf("Iter yielded pixel at x=%d with alpha=0", p.x)
		}
	}
}

// TestAlphaRuns_IterEarlyStop tests early termination of iterator.
func TestAlphaRuns_IterEarlyStop(t *testing.T) {
	ar := NewAlphaRuns(100)
	ar.Add(5, 200, 10, 0)

	count := 0
	for range ar.Iter() {
		count++
		if count >= 3 {
			break
		}
	}
	if count != 3 {
		t.Errorf("early stop: count = %d, want 3", count)
	}
}

// TestAlphaRuns_IterRuns tests the run-level iterator.
func TestAlphaRuns_IterRuns(t *testing.T) {
	ar := NewAlphaRuns(50)
	ar.Add(10, 128, 5, 64)

	runs := make([]AlphaRun, 0, ar.Width())
	for run := range ar.IterRuns() {
		runs = append(runs, run)
	}

	if len(runs) == 0 {
		t.Fatal("IterRuns produced no runs")
	}

	// Verify runs cover the full width
	totalPixels := 0
	for _, r := range runs {
		totalPixels += r.Count
		if r.Count <= 0 {
			t.Errorf("run at x=%d has non-positive count %d", r.X, r.Count)
		}
	}
	if totalPixels != 50 {
		t.Errorf("total pixels from runs = %d, want 50 (width)", totalPixels)
	}
}

// TestAlphaRuns_IterRunsEarlyStop tests early termination of run iterator.
func TestAlphaRuns_IterRunsEarlyStop(t *testing.T) {
	ar := NewAlphaRuns(100)
	ar.Add(10, 128, 5, 64)
	ar.SetOffset(0)
	ar.Add(40, 200, 10, 0)

	count := 0
	for range ar.IterRuns() {
		count++
		if count >= 2 {
			break
		}
	}
	if count != 2 {
		t.Errorf("early stop: count = %d, want 2", count)
	}
}

// TestAlphaRuns_SetOffset tests offset management.
func TestAlphaRuns_SetOffset(t *testing.T) {
	ar := NewAlphaRuns(100)

	ar.SetOffset(0)
	ar.Add(10, 128, 5, 0)

	// Reset offset and add more
	ar.SetOffset(0)
	ar.Add(30, 200, 3, 0)

	if a := ar.GetAlpha(10); a != 128 {
		t.Errorf("first add alpha = %d, want 128", a)
	}
	if a := ar.GetAlpha(30); a != 200 {
		t.Errorf("second add alpha = %d, want 200", a)
	}
}
