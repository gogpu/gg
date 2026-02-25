// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package gg

import "testing"

func TestContextResize(t *testing.T) {
	tests := []struct {
		name       string
		initWidth  int
		initHeight int
		newWidth   int
		newHeight  int
		wantErr    bool
	}{
		{"grow width", 100, 100, 200, 100, false},
		{"grow height", 100, 100, 100, 200, false},
		{"grow both", 100, 100, 200, 200, false},
		{"shrink width", 200, 200, 100, 200, false},
		{"shrink height", 200, 200, 200, 100, false},
		{"shrink both", 200, 200, 100, 100, false},
		{"same size noop", 100, 100, 100, 100, false},
		{"zero width", 100, 100, 0, 100, true},
		{"zero height", 100, 100, 100, 0, true},
		{"negative width", 100, 100, -1, 100, true},
		{"negative height", 100, 100, 100, -1, true},
		{"both zero", 100, 100, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext(tt.initWidth, tt.initHeight)
			defer func() { _ = ctx.Close() }()

			err := ctx.Resize(tt.newWidth, tt.newHeight)

			if (err != nil) != tt.wantErr {
				t.Errorf("Resize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if ctx.Width() != tt.newWidth {
					t.Errorf("Width() = %d, want %d", ctx.Width(), tt.newWidth)
				}
				if ctx.Height() != tt.newHeight {
					t.Errorf("Height() = %d, want %d", ctx.Height(), tt.newHeight)
				}
			}
		})
	}
}

func TestContextResizeSameSizeIsNoop(t *testing.T) {
	ctx := NewContext(100, 100)
	defer func() { _ = ctx.Close() }()

	// Draw something
	ctx.SetRGB(1, 0, 0)
	ctx.DrawRectangle(10, 10, 20, 20)
	_ = ctx.Fill()

	// Get original pixmap pointer
	originalPixmap := ctx.pixmap

	// Resize to same dimensions
	err := ctx.Resize(100, 100)
	if err != nil {
		t.Fatalf("Resize() error = %v", err)
	}

	// Pixmap should be the same object (no reallocation)
	if ctx.pixmap != originalPixmap {
		t.Error("Resize with same dimensions should not reallocate pixmap")
	}
}

func TestContextResizeReallocatesPixmap(t *testing.T) {
	ctx := NewContext(100, 100)
	defer func() { _ = ctx.Close() }()

	// Draw something
	ctx.SetRGB(1, 0, 0)
	ctx.DrawRectangle(10, 10, 20, 20)
	_ = ctx.Fill()

	// Get original pixmap pointer
	originalPixmap := ctx.pixmap

	// Resize to different dimensions
	err := ctx.Resize(200, 150)
	if err != nil {
		t.Fatalf("Resize() error = %v", err)
	}

	// Pixmap should be a new object
	if ctx.pixmap == originalPixmap {
		t.Error("Resize with different dimensions should reallocate pixmap")
	}
}

func TestContextResizePreservesTransformStack(t *testing.T) {
	ctx := NewContext(100, 100)
	defer func() { _ = ctx.Close() }()

	// Save current state and apply transform
	ctx.Push()
	ctx.Translate(50, 50)
	ctx.Rotate(0.5)

	// Get current transform
	matrixBefore := ctx.GetTransform()

	// Resize
	err := ctx.Resize(200, 200)
	if err != nil {
		t.Fatalf("Resize() error = %v", err)
	}

	// Transform should be preserved
	matrixAfter := ctx.GetTransform()
	if matrixBefore != matrixAfter {
		t.Error("Resize should preserve transformation matrix")
	}

	// Pop should work
	ctx.Pop()
}

func TestContextResizeClearsPath(t *testing.T) {
	ctx := NewContext(100, 100)
	defer func() { _ = ctx.Close() }()

	// Start drawing a path
	ctx.MoveTo(10, 10)
	ctx.LineTo(50, 50)

	// Resize
	err := ctx.Resize(200, 200)
	if err != nil {
		t.Fatalf("Resize() error = %v", err)
	}

	// Path should be cleared
	_, _, ok := ctx.GetCurrentPoint()
	if ok {
		t.Error("Resize should clear the current path")
	}
}

func TestContextResizeResetsClip(t *testing.T) {
	ctx := NewContext(100, 100)
	defer func() { _ = ctx.Close() }()

	// Apply a clip
	ctx.DrawRectangle(10, 10, 50, 50)
	ctx.Clip()

	// Resize
	err := ctx.Resize(200, 200)
	if err != nil {
		t.Fatalf("Resize() error = %v", err)
	}

	// Clip should be reset (full rect)
	// We can verify by checking that clipStack is nil
	if ctx.clipStack != nil {
		t.Error("Resize should reset clip stack")
	}
}
