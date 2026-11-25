// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package surface

import (
	"image"
	"image/color"
	"sync"
	"sync/atomic"
	"testing"
)

// countingBackend counts Close calls so the idempotence of GPUSurface.Close is
// observable; the other methods are no-ops.
type countingBackend struct {
	closes atomic.Int32
}

func (*countingBackend) Clear(color.Color)                               {}
func (*countingBackend) Fill(*Path, FillStyle)                           {}
func (*countingBackend) Stroke(*Path, StrokeStyle)                       {}
func (*countingBackend) DrawImage(image.Image, Point, *DrawImageOptions) {}
func (*countingBackend) Flush() error                                    { return nil }

func (*countingBackend) Readback() (*image.RGBA, error) {
	return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
}

func (b *countingBackend) Close() error {
	b.closes.Add(1)
	return nil
}

// TestGPUSurfaceCloseConcurrent pins BUG-RACE-002 for the GPU surface: concurrent
// Close calls release the backend exactly once.
func TestGPUSurfaceCloseConcurrent(t *testing.T) {
	backend := &countingBackend{}
	s, err := NewGPUSurface(8, 8, backend)
	if err != nil {
		t.Fatalf("NewGPUSurface: %v", err)
	}

	const n = 32
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Close()
		}()
	}
	wg.Wait()

	if c := backend.closes.Load(); c != 1 {
		t.Errorf("backend.Close calls = %d, want 1", c)
	}
}

// TestImageSurfaceCloseConcurrent pins BUG-RACE-002 for the image surface: no
// panic, no race on the closed flag under concurrent Close.
func TestImageSurfaceCloseConcurrent(t *testing.T) {
	s := NewImageSurface(8, 8)

	const n = 32
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Close()
		}()
	}
	wg.Wait()
}
