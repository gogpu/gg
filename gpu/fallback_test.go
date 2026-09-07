//go:build !nogpu

package gpu_test

import (
	"image"
	"testing"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu"
)

// Registering the accelerator must not cost the caller its geometry.
//
// The queueing operations used to accept a draw without checking that a device
// could be had, which made Context.doStroke skip the software rasterizer; the
// failure surfaced at Flush, by which time the draw had nowhere left to go. On
// a machine with no adapter — a container, a CI runner, a VM — that turned
// every filled and stroked path into nothing at all, silently, while text
// (which checks before it queues) still came out.
//
// The property is the one the package documents: with the accelerator
// registered, a chart still renders. It holds on hardware too, where the draw
// goes to the GPU and comes back.
func TestAPathIsDrawnWithOrWithoutADevice(t *testing.T) {
	t.Logf("accelerator registered: %v", gg.Accelerator() != nil)

	draw := func(t *testing.T) image.Image {
		t.Helper()
		c := gg.NewContext(60, 40)
		t.Cleanup(func() { _ = c.Close() })
		c.ClearPath()
		c.MoveTo(5, 35)
		c.LineTo(30, 5)
		c.LineTo(55, 35)
		c.SetStroke(gg.Stroke{Width: 3})
		c.SetColor(gg.RGBA{R: 1, A: 1})
		if err := c.Stroke(); err != nil {
			t.Fatalf("Stroke: %v", err)
		}
		if err := c.FlushGPU(); err != nil && err != gg.ErrFallbackToCPU {
			t.Fatalf("FlushGPU: %v", err)
		}
		return c.Image()
	}

	img := draw(t)
	b := img.Bounds()
	first := img.At(b.Min.X, b.Min.Y)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.At(x, y) != first {
				return
			}
		}
	}
	t.Error("the stroked path is not in the buffer: every pixel is the same colour")
}
