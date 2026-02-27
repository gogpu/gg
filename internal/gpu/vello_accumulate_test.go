// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !nogpu

package gpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gg"
)

// TestVelloAccelerator_FillPathAccumulates verifies that FillPath accumulates
// paths without dispatching them immediately.
func TestVelloAccelerator_FillPathAccumulates(t *testing.T) {
	a := &VelloAccelerator{gpuReady: true}
	// Set a dispatcher that isn't nil so CanCompute returns true if needed.
	// But for accumulation tests, we only need gpuReady.

	target := makeTestTarget(100, 100)

	path1 := gg.NewPath()
	path1.MoveTo(10, 10)
	path1.LineTo(90, 50)
	path1.LineTo(10, 90)
	path1.Close()

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	err := a.FillPath(target, path1, paint)
	if err != nil {
		t.Fatalf("FillPath: unexpected error: %v", err)
	}
	if a.PendingCount() != 1 {
		t.Errorf("after 1 FillPath: expected 1 pending, got %d", a.PendingCount())
	}

	// Add another path.
	path2 := gg.NewPath()
	path2.Rectangle(20, 20, 60, 60)

	err = a.FillPath(target, path2, paint)
	if err != nil {
		t.Fatalf("FillPath: unexpected error: %v", err)
	}
	if a.PendingCount() != 2 {
		t.Errorf("after 2 FillPath: expected 2 pending, got %d", a.PendingCount())
	}
}

// TestVelloAccelerator_FillPathRejectsWhenNotReady verifies ErrFallbackToCPU
// when GPU is not initialized.
func TestVelloAccelerator_FillPathRejectsWhenNotReady(t *testing.T) {
	a := &VelloAccelerator{gpuReady: false}
	target := makeTestTarget(100, 100)

	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(10, 10)

	err := a.FillPath(target, path, gg.NewPaint())
	if !errors.Is(err, gg.ErrFallbackToCPU) {
		t.Errorf("expected ErrFallbackToCPU, got %v", err)
	}
}

// TestVelloAccelerator_FillPathSkipsEmptyPath verifies empty paths are no-ops.
func TestVelloAccelerator_FillPathSkipsEmptyPath(t *testing.T) {
	a := &VelloAccelerator{gpuReady: true}
	target := makeTestTarget(100, 100)

	// Nil path
	err := a.FillPath(target, nil, gg.NewPaint())
	if err != nil {
		t.Errorf("nil path: expected nil error, got %v", err)
	}
	if a.PendingCount() != 0 {
		t.Errorf("nil path: expected 0 pending, got %d", a.PendingCount())
	}

	// Empty path
	err = a.FillPath(target, gg.NewPath(), gg.NewPaint())
	if err != nil {
		t.Errorf("empty path: expected nil error, got %v", err)
	}
	if a.PendingCount() != 0 {
		t.Errorf("empty path: expected 0 pending, got %d", a.PendingCount())
	}
}

// TestVelloAccelerator_FillShapeAccumulates verifies FillShape converts
// shapes to paths and accumulates them.
func TestVelloAccelerator_FillShapeAccumulates(t *testing.T) {
	a := &VelloAccelerator{gpuReady: true}
	target := makeTestTarget(100, 100)

	shape := gg.DetectedShape{
		Kind:    gg.ShapeCircle,
		CenterX: 50,
		CenterY: 50,
		RadiusX: 30,
		RadiusY: 30,
	}

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Green))

	err := a.FillShape(target, shape, paint)
	if err != nil {
		t.Fatalf("FillShape: unexpected error: %v", err)
	}
	if a.PendingCount() != 1 {
		t.Errorf("expected 1 pending after FillShape, got %d", a.PendingCount())
	}
}

// TestVelloAccelerator_FillShapeRejectsUnknown verifies unknown shapes
// return ErrFallbackToCPU.
func TestVelloAccelerator_FillShapeRejectsUnknown(t *testing.T) {
	a := &VelloAccelerator{gpuReady: true}
	target := makeTestTarget(100, 100)

	shape := gg.DetectedShape{Kind: gg.ShapeUnknown}
	err := a.FillShape(target, shape, gg.NewPaint())
	if !errors.Is(err, gg.ErrFallbackToCPU) {
		t.Errorf("expected ErrFallbackToCPU for unknown shape, got %v", err)
	}
}

// TestVelloAccelerator_StrokePathAccumulates verifies stroke expansion and accumulation.
func TestVelloAccelerator_StrokePathAccumulates(t *testing.T) {
	a := &VelloAccelerator{gpuReady: true}
	target := makeTestTarget(100, 100)

	path := gg.NewPath()
	path.MoveTo(10, 50)
	path.LineTo(90, 50)

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Blue))
	s := gg.Stroke{Width: 4.0, Cap: gg.LineCapButt, Join: gg.LineJoinMiter, MiterLimit: 10.0}
	paint.SetStroke(s)

	err := a.StrokePath(target, path, paint)
	if err != nil {
		t.Fatalf("StrokePath: unexpected error: %v", err)
	}
	if a.PendingCount() != 1 {
		t.Errorf("expected 1 pending after StrokePath, got %d", a.PendingCount())
	}
}

// TestVelloAccelerator_FlushClearsPending verifies that flushLocked clears
// pending paths. Since we don't have a real GPU device in unit tests,
// we test the clearing behavior by verifying the flush returns ErrFallbackToCPU
// (no device) but still clears pending state.
func TestVelloAccelerator_FlushClearsPending(t *testing.T) {
	a := &VelloAccelerator{gpuReady: true}
	target := makeTestTarget(100, 100)

	path := gg.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(90, 90)

	if err := a.FillPath(target, path, gg.NewPaint()); err != nil {
		t.Fatalf("FillPath: %v", err)
	}
	if a.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", a.PendingCount())
	}

	// Flush without a real GPU — should fall back but still clear pending.
	err := a.Flush(target)
	if err != nil && !errors.Is(err, gg.ErrFallbackToCPU) {
		t.Fatalf("Flush: expected nil or ErrFallbackToCPU, got %v", err)
	}

	// Pending should be cleared regardless of whether dispatch succeeded.
	if a.PendingCount() != 0 {
		t.Errorf("after Flush: expected 0 pending, got %d", a.PendingCount())
	}
}

// TestVelloAccelerator_FlushNoPending verifies Flush is a no-op when empty.
func TestVelloAccelerator_FlushNoPending(t *testing.T) {
	a := &VelloAccelerator{gpuReady: true}
	target := makeTestTarget(100, 100)

	err := a.Flush(target)
	if err != nil {
		t.Errorf("Flush with no pending: expected nil, got %v", err)
	}
}

// TestVelloAccelerator_CanAccelerate verifies capability reporting.
func TestVelloAccelerator_CanAccelerate(t *testing.T) {
	a := &VelloAccelerator{}

	tests := []struct {
		op   gg.AcceleratedOp
		want bool
	}{
		{gg.AccelFill, true},
		{gg.AccelStroke, true},
		{gg.AccelScene, true},
		{gg.AccelCircleSDF, true},
		{gg.AccelRRectSDF, true},
		{gg.AccelText, false},
		{gg.AccelImage, false},
		{gg.AccelGradient, false},
	}

	for _, tt := range tests {
		got := a.CanAccelerate(tt.op)
		if got != tt.want {
			t.Errorf("CanAccelerate(%d): expected %v, got %v", tt.op, tt.want, got)
		}
	}
}

// TestVelloAccelerator_MultiplePathsMixedTypes verifies accumulation of
// different path types (fills + shapes).
func TestVelloAccelerator_MultiplePathsMixedTypes(t *testing.T) {
	a := &VelloAccelerator{gpuReady: true}
	target := makeTestTarget(200, 200)

	// Fill a rectangle path.
	rectPath := gg.NewPath()
	rectPath.Rectangle(10, 10, 80, 60)
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	if err := a.FillPath(target, rectPath, paint); err != nil {
		t.Fatalf("FillPath: %v", err)
	}

	// Fill a circle shape.
	circleShape := gg.DetectedShape{
		Kind:    gg.ShapeCircle,
		CenterX: 100,
		CenterY: 100,
		RadiusX: 40,
		RadiusY: 40,
	}
	paint2 := gg.NewPaint()
	paint2.SetBrush(gg.Solid(gg.Blue))

	if err := a.FillShape(target, circleShape, paint2); err != nil {
		t.Fatalf("FillShape: %v", err)
	}

	if a.PendingCount() != 2 {
		t.Errorf("expected 2 pending paths, got %d", a.PendingCount())
	}
}

// makeTestTarget creates a test GPURenderTarget with the given dimensions.
func makeTestTarget(w, h int) gg.GPURenderTarget {
	stride := w * 4
	return gg.GPURenderTarget{
		Data:   make([]uint8, stride*h),
		Width:  w,
		Height: h,
		Stride: stride,
	}
}
