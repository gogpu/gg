//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
)

func BenchmarkGPUShared_NewRenderContext(b *testing.B) {
	shared := NewGPUShared()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rc := shared.NewRenderContext()
		_ = rc
	}
}

func BenchmarkGPURenderContext_QueueShape(b *testing.B) {
	shared := NewGPUShared()
	rc := shared.NewRenderContext()
	target := gg.GPURenderTarget{Width: 800, Height: 600, Stride: 3200, Data: make([]byte, 800*600*4)}
	shape := gg.DetectedShape{Kind: gg.ShapeCircle, CenterX: 100, CenterY: 100, RadiusX: 50}
	paint := gg.NewPaint()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rc.QueueShape(target, shape, paint, false)
		if rc.PendingCount() > 1000 {
			rc.pendingShapes = rc.pendingShapes[:0]
		}
	}
}

func BenchmarkGPURenderContext_ScissorSegment(b *testing.B) {
	shared := NewGPUShared()
	rc := shared.NewRenderContext()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rc.recordScissorSegment(&[4]uint32{10, 10, 100, 100})
		if len(rc.scissorSegments) > 1000 {
			rc.scissorSegments = rc.scissorSegments[:0]
		}
	}
}

func BenchmarkTexturePool_AcquireRelease(b *testing.B) {
	pool := NewTexturePool(128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts := pool.Acquire(1920, 1080, 4)
		if ts != nil {
			pool.Release(ts)
		}
	}
}
