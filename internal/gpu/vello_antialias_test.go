// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !nogpu

package gpu

import (
	"math"
	"testing"

	"github.com/gogpu/gg"
)

// TestSnapPathToPixelGrid verifies that coordinates are rounded to integers.
func TestSnapPathToPixelGrid(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(10.3, 20.7)
	path.LineTo(50.5, 30.2)
	path.QuadraticTo(60.1, 40.9, 70.5, 50.5)
	path.CubicTo(80.1, 60.9, 90.4, 70.6, 100.5, 80.5)
	path.Close()

	snapped := snapPathToPixelGrid(path)

	// Collect coordinates from the snapped path.
	var coords []float64
	snapped.Iterate(func(verb gg.PathVerb, c []float64) {
		coords = append(coords, c...)
	})

	// Expected: all coordinates rounded to nearest integer (half away from zero).
	expected := []float64{
		10, 21, // MoveTo(10.3→10, 20.7→21)
		51, 30, // LineTo(50.5→51, 30.2→30)
		60, 41, 71, 51, // QuadTo(60.1→60, 40.9→41, 70.5→71, 50.5→51)
		80, 61, 90, 71, 101, 81, // CubicTo(80.1→80, 60.9→61, 90.4→90, 70.6→71, 100.5→101, 80.5→81)
		// Close has no coordinates
	}

	if len(coords) != len(expected) {
		t.Fatalf("coord count: got %d, want %d", len(coords), len(expected))
	}

	for i, got := range coords {
		want := expected[i]
		if got != want {
			t.Errorf("coord[%d] = %v, want %v", i, got, want)
		}
	}
}

// TestSnapPathToPixelGrid_AlreadyAligned verifies no-op for integer coords.
func TestSnapPathToPixelGrid_AlreadyAligned(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(10, 20)
	path.LineTo(100, 20)
	path.LineTo(100, 80)
	path.LineTo(10, 80)
	path.Close()

	snapped := snapPathToPixelGrid(path)

	var original, result []float64
	path.Iterate(func(_ gg.PathVerb, c []float64) {
		original = append(original, c...)
	})
	snapped.Iterate(func(_ gg.PathVerb, c []float64) {
		result = append(result, c...)
	})

	if len(original) != len(result) {
		t.Fatalf("coord count mismatch: %d vs %d", len(original), len(result))
	}
	for i := range original {
		if original[i] != result[i] {
			t.Errorf("coord[%d]: snapped %v != original %v (should be identical for integer coords)", i, result[i], original[i])
		}
	}
}

// TestVelloAccelerator_NoAA_SnapsPath verifies FillPath applies pixel snapping
// when anti-aliasing is disabled.
func TestVelloAccelerator_NoAA_SnapsPath(t *testing.T) {
	a := &VelloAccelerator{gpuReady: true, antiAlias: true}
	target := makeTestTargetForAA(100, 100)

	path := gg.NewPath()
	path.MoveTo(10.3, 20.7)
	path.LineTo(90.5, 50.2)
	path.Close()

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	// With AA enabled: path should NOT be snapped (coordinates preserve fractional).
	err := a.FillPath(target, path, paint)
	if err != nil {
		t.Fatalf("FillPath with AA: unexpected error: %v", err)
	}
	if a.PendingCount() != 1 {
		t.Fatalf("expected 1 pending path, got %d", a.PendingCount())
	}

	// Check first pending path has fractional coords (line endpoints).
	firstPath := a.pendingPaths[0]
	hasNonInteger := false
	for _, line := range firstPath.Lines {
		if math.Floor(float64(line.P0[0])) != float64(line.P0[0]) ||
			math.Floor(float64(line.P0[1])) != float64(line.P0[1]) {
			hasNonInteger = true
			break
		}
	}
	if !hasNonInteger {
		t.Error("AA enabled: expected some fractional coordinates in path lines")
	}

	// Now disable AA and add another path.
	a.pendingPaths = nil
	a.SetAntiAlias(false)

	err = a.FillPath(target, path, paint)
	if err != nil {
		t.Fatalf("FillPath no-AA: unexpected error: %v", err)
	}
	if a.PendingCount() != 1 {
		t.Fatalf("expected 1 pending path, got %d", a.PendingCount())
	}

	// Check second pending path has integer coords (all snapped).
	secondPath := a.pendingPaths[0]
	for i, line := range secondPath.Lines {
		if math.Round(float64(line.P0[0])) != float64(line.P0[0]) ||
			math.Round(float64(line.P0[1])) != float64(line.P0[1]) ||
			math.Round(float64(line.P1[0])) != float64(line.P1[0]) ||
			math.Round(float64(line.P1[1])) != float64(line.P1[1]) {
			t.Errorf("no-AA: line[%d] has non-integer coords P0=%v P1=%v",
				i, line.P0, line.P1)
		}
	}
}

// makeTestTargetForAA creates a simple GPURenderTarget for anti-alias testing.
// (makeTestTarget already declared in vello_accumulate_test.go)
func makeTestTargetForAA(w, h int) gg.GPURenderTarget {
	return gg.GPURenderTarget{
		Width:  w,
		Height: h,
		Stride: w * 4,
		Data:   make([]byte, w*h*4),
	}
}
