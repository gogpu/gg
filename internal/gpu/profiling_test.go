// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !nogpu

// ADR-051 Draw Queue profiling benchmarks.
// Measures the per-draw and per-flush allocation cost of the deferred draw queue.
// Run: GOWORK=off go test -bench=BenchmarkProfile -benchmem -count=3 ./internal/gpu/ -run='^$'
// Profile: GOWORK=off go test -bench=BenchmarkProfileFullFrame -memprofile=tmp/mem.prof -cpuprofile=tmp/cpu.prof ./internal/gpu/ -run='^$'

package gpu

import (
	"testing"
	"unsafe"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/internal/stroke"
)

// --- Struct size report ---

// TestSizeofReport prints sizes of key ADR-051 structures for the profiling report.
func TestSizeofReport(t *testing.T) {
	var p gg.Paint
	var dc drawCommand
	var ds gg.DetectedShape
	var cp ClipParams

	t.Logf("sizeof(Paint)         = %d bytes", unsafe.Sizeof(p))
	t.Logf("sizeof(drawCommand)   = %d bytes", unsafe.Sizeof(dc))
	t.Logf("sizeof(DetectedShape) = %d bytes", unsafe.Sizeof(ds))
	t.Logf("sizeof(ClipParams)    = %d bytes", unsafe.Sizeof(cp))
	t.Logf("sizeof([4]uint32)     = %d bytes (clipRect)", unsafe.Sizeof([4]uint32{}))
}

// --- Paint value copy cost ---

// BenchmarkProfilePaintValueCopy measures the cost of copying a Paint struct
// by value (per enqueue). Paint is ~200 bytes with interfaces + slice headers.
func BenchmarkProfilePaintValueCopy(b *testing.B) {
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))
	b.ReportMetric(float64(unsafe.Sizeof(*paint)), "bytes/Paint")
	b.ResetTimer()
	for b.Loop() {
		p := *paint
		_ = p
	}
}

// BenchmarkProfilePaintValueCopy_WithClipMask measures Paint copy when ClipMask
// is set (slice header copy, not data copy — ClipMask shares backing array).
func BenchmarkProfilePaintValueCopy_WithClipMask(b *testing.B) {
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))
	paint.ClipMask = make([]uint8, 100*100) // 10K mask
	paint.ClipMaskW = 100
	paint.ClipMaskH = 100
	b.ResetTimer()
	for b.Loop() {
		p := *paint
		_ = p
	}
}

// --- Path.Clone() cost ---

// BenchmarkProfilePathClone_Triangle measures Path.Clone cost for a simple triangle.
func BenchmarkProfilePathClone_Triangle(b *testing.B) {
	p := gg.NewPath()
	p.MoveTo(10, 10)
	p.LineTo(90, 50)
	p.LineTo(10, 90)
	p.Close()
	b.ResetTimer()
	for b.Loop() {
		c := p.Clone()
		_ = c
	}
}

// BenchmarkProfilePathClone_Circle measures Path.Clone cost for a circle (cubic beziers).
func BenchmarkProfilePathClone_Circle(b *testing.B) {
	p := gg.NewPath()
	p.Circle(50, 50, 40)
	p.Close()
	b.ResetTimer()
	for b.Loop() {
		c := p.Clone()
		_ = c
	}
}

// BenchmarkProfilePathClone_ComplexStar measures Path.Clone cost for a 10-point star.
func BenchmarkProfilePathClone_ComplexStar(b *testing.B) {
	p := gg.NewPath()
	p.MoveTo(50, 0)
	p.LineTo(61, 35)
	p.LineTo(98, 35)
	p.LineTo(68, 57)
	p.LineTo(79, 91)
	p.LineTo(50, 70)
	p.LineTo(21, 91)
	p.LineTo(32, 57)
	p.LineTo(2, 35)
	p.LineTo(39, 35)
	p.Close()
	b.ResetTimer()
	for b.Loop() {
		c := p.Clone()
		_ = c
	}
}

// --- Clip copy cost ---

// BenchmarkProfileCopyClipRect measures the cost of copyClipRect per enqueue.
func BenchmarkProfileCopyClipRect(b *testing.B) {
	r := &[4]uint32{10, 20, 100, 200}
	b.ResetTimer()
	for b.Loop() {
		c := copyClipRect(r)
		_ = c
	}
}

// BenchmarkProfileCopyClipRRect measures the cost of copyClipRRect per enqueue.
func BenchmarkProfileCopyClipRRect(b *testing.B) {
	p := &ClipParams{RectX1: 10, RectY1: 20, RectX2: 110, RectY2: 220, Radius: 8, Enabled: 1}
	b.ResetTimer()
	for b.Loop() {
		c := copyClipRRect(p)
		_ = c
	}
}

// BenchmarkProfileCopyClipRect_Nil measures nil clip path (no copy needed).
func BenchmarkProfileCopyClipRect_Nil(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		c := copyClipRect(nil)
		_ = c
	}
}

// --- Full FillShape enqueue cost ---

// BenchmarkProfileFillShape_Enqueue measures the full FillShape enqueue path
// (value copies + append). No path clone for shapes — shapes are value types.
func BenchmarkProfileFillShape_Enqueue(b *testing.B) {
	s := NewGPUShared()
	s.strategy = strategyFull
	s.deviceReady = true
	s.gpuReady = true
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(800, 600)
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))
	shape := gg.DetectedShape{Kind: gg.ShapeCircle, CenterX: 100, CenterY: 100, RadiusX: 50, RadiusY: 50}

	b.ResetTimer()
	for b.Loop() {
		_ = rc.FillShape(target, shape, paint)
		if len(rc.pendingDraws) > 1000 {
			rc.pendingDraws = rc.pendingDraws[:0]
		}
	}
}

// BenchmarkProfileFillShape_WithClip measures FillShape enqueue with rect+rrect clip.
func BenchmarkProfileFillShape_WithClip(b *testing.B) {
	s := NewGPUShared()
	s.strategy = strategyFull
	s.deviceReady = true
	s.gpuReady = true
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(800, 600)
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))
	shape := gg.DetectedShape{Kind: gg.ShapeCircle, CenterX: 100, CenterY: 100, RadiusX: 50, RadiusY: 50}

	rc.SetClipRect(0, 0, 400, 300)
	rc.SetClipRRect(10, 10, 380, 280, 8)

	b.ResetTimer()
	for b.Loop() {
		_ = rc.FillShape(target, shape, paint)
		if len(rc.pendingDraws) > 1000 {
			rc.pendingDraws = rc.pendingDraws[:0]
		}
	}
}

// --- Full FillPath enqueue cost ---

// BenchmarkProfileFillPath_EnqueueTriangle measures FillPath enqueue for a convex triangle.
// Includes: path.Clone() + preTessellateFill (convex fast path) + clip copies + append.
func BenchmarkProfileFillPath_EnqueueTriangle(b *testing.B) {
	s := NewGPUShared()
	s.strategy = strategyFull
	s.deviceReady = true
	s.gpuReady = true
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(800, 600)
	path := gg.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(90, 50)
	path.LineTo(10, 90)
	path.Close()

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	b.ResetTimer()
	for b.Loop() {
		_ = rc.FillPath(target, path, paint)
		if len(rc.pendingDraws) > 500 {
			rc.pendingDraws = rc.pendingDraws[:0]
		}
	}
}

// BenchmarkProfileFillPath_EnqueueComplexStar measures FillPath enqueue for a complex star.
// Includes: path.Clone() + preTessellateFill (stencil tessellation) + clip copies + append.
func BenchmarkProfileFillPath_EnqueueComplexStar(b *testing.B) {
	s := NewGPUShared()
	s.strategy = strategyFull
	s.deviceReady = true
	s.gpuReady = true
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(800, 600)
	path := gg.NewPath()
	path.MoveTo(50, 0)
	path.LineTo(61, 35)
	path.LineTo(98, 35)
	path.LineTo(68, 57)
	path.LineTo(79, 91)
	path.LineTo(50, 70)
	path.LineTo(21, 91)
	path.LineTo(32, 57)
	path.LineTo(2, 35)
	path.LineTo(39, 35)
	path.Close()

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))
	paint.FillRule = gg.FillRuleEvenOdd

	b.ResetTimer()
	for b.Loop() {
		_ = rc.FillPath(target, path, paint)
		if len(rc.pendingDraws) > 500 {
			rc.pendingDraws = rc.pendingDraws[:0]
		}
	}
}

// --- StrokePath enqueue cost ---

// BenchmarkProfileStrokePath_EnqueueLine measures StrokePath enqueue.
// Includes: path.Clone() + stroke expansion + preTessellateFill + strokeResultToPath.
func BenchmarkProfileStrokePath_EnqueueLine(b *testing.B) {
	s := NewGPUShared()
	s.strategy = strategyFull
	s.deviceReady = true
	s.gpuReady = true
	rc := s.NewRenderContext()
	defer rc.Close()

	target := makeTestTarget(800, 600)
	path := gg.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(90, 90)

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))
	paint.SetStroke(gg.Stroke{Width: 4.0})

	b.ResetTimer()
	for b.Loop() {
		_ = rc.StrokePath(target, path, paint)
		if len(rc.pendingDraws) > 500 {
			rc.pendingDraws = rc.pendingDraws[:0]
		}
	}
}

// --- M-3: NewSoftwareRenderer per flush ---

// BenchmarkProfileNewSoftwareRenderer measures M-3: SoftwareRenderer creation per flush.
func BenchmarkProfileNewSoftwareRenderer(b *testing.B) {
	b.Run("100x100", func(b *testing.B) {
		for b.Loop() {
			sr := gg.NewSoftwareRenderer(100, 100)
			_ = sr
		}
	})
	b.Run("800x600", func(b *testing.B) {
		for b.Loop() {
			sr := gg.NewSoftwareRenderer(800, 600)
			_ = sr
		}
	})
	b.Run("1920x1080", func(b *testing.B) {
		for b.Loop() {
			sr := gg.NewSoftwareRenderer(1920, 1080)
			_ = sr
		}
	})
}

// --- M-2: BGRA swizzle cost ---

// BenchmarkProfileBGRASwizzle measures M-2: BGRA swizzle full-frame buffer allocation.
func BenchmarkProfileBGRASwizzle(b *testing.B) {
	b.Run("100x100", func(b *testing.B) {
		pixels := make([]byte, 100*100*4)
		for i := range pixels {
			pixels[i] = byte(i & 0xFF)
		}
		b.ResetTimer()
		for b.Loop() {
			bgra := make([]byte, len(pixels))
			for i := 0; i < len(pixels); i += 4 {
				bgra[i+0] = pixels[i+2]
				bgra[i+1] = pixels[i+1]
				bgra[i+2] = pixels[i+0]
				bgra[i+3] = pixels[i+3]
			}
			_ = bgra
		}
	})
	b.Run("800x600", func(b *testing.B) {
		pixels := make([]byte, 800*600*4)
		for i := range pixels {
			pixels[i] = byte(i & 0xFF)
		}
		b.ResetTimer()
		for b.Loop() {
			bgra := make([]byte, len(pixels))
			for i := 0; i < len(pixels); i += 4 {
				bgra[i+0] = pixels[i+2]
				bgra[i+1] = pixels[i+1]
				bgra[i+2] = pixels[i+0]
				bgra[i+3] = pixels[i+3]
			}
			_ = bgra
		}
	})
	b.Run("1920x1080", func(b *testing.B) {
		pixels := make([]byte, 1920*1080*4)
		for i := range pixels {
			pixels[i] = byte(i & 0xFF)
		}
		b.ResetTimer()
		for b.Loop() {
			bgra := make([]byte, len(pixels))
			for i := 0; i < len(pixels); i += 4 {
				bgra[i+0] = pixels[i+2]
				bgra[i+1] = pixels[i+1]
				bgra[i+2] = pixels[i+0]
				bgra[i+3] = pixels[i+3]
			}
			_ = bgra
		}
	})
}

// --- P0: Pooled BGRA swizzle (steady-state, 0 allocs) ---

// BenchmarkProfileBGRASwizzle_Pooled measures P0: BGRA swizzle with pooled buffer.
func BenchmarkProfileBGRASwizzle_Pooled(b *testing.B) {
	b.Run("800x600", func(b *testing.B) {
		pixels := make([]byte, 800*600*4)
		for i := range pixels {
			pixels[i] = byte(i & 0xFF)
		}
		rc := &GPURenderContext{shared: NewGPUShared()}
		b.ResetTimer()
		for b.Loop() {
			bgra := rc.ensureBGRABuffer(len(pixels))
			for i := 0; i < len(pixels); i += 4 {
				bgra[i+0] = pixels[i+2]
				bgra[i+1] = pixels[i+1]
				bgra[i+2] = pixels[i+0]
				bgra[i+3] = pixels[i+3]
			}
		}
	})
	b.Run("1920x1080", func(b *testing.B) {
		pixels := make([]byte, 1920*1080*4)
		for i := range pixels {
			pixels[i] = byte(i & 0xFF)
		}
		rc := &GPURenderContext{shared: NewGPUShared()}
		b.ResetTimer()
		for b.Loop() {
			bgra := rc.ensureBGRABuffer(len(pixels))
			for i := 0; i < len(pixels); i += 4 {
				bgra[i+0] = pixels[i+2]
				bgra[i+1] = pixels[i+1]
				bgra[i+2] = pixels[i+0]
				bgra[i+3] = pixels[i+3]
			}
		}
	})
}

// --- P1: Cached SoftwareRenderer (steady-state, 0 allocs) ---

// BenchmarkProfileCachedSoftwareRenderer measures P1: SoftwareRenderer reuse.
func BenchmarkProfileCachedSoftwareRenderer(b *testing.B) {
	b.Run("800x600", func(b *testing.B) {
		rc := &GPURenderContext{shared: NewGPUShared()}
		b.ResetTimer()
		for b.Loop() {
			sr := rc.getSoftwareRenderer(800, 600)
			_ = sr
		}
	})
	b.Run("1920x1080", func(b *testing.B) {
		rc := &GPURenderContext{shared: NewGPUShared()}
		b.ResetTimer()
		for b.Loop() {
			sr := rc.getSoftwareRenderer(1920, 1080)
			_ = sr
		}
	})
}

// --- Full frame simulation ---

// BenchmarkProfileFullFrame_10Shapes simulates a full frame: 10 shape enqueue + flush (CPU dispatch).
func BenchmarkProfileFullFrame_10Shapes(b *testing.B) {
	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	b.ResetTimer()
	for b.Loop() {
		rc := s.NewRenderContext()
		target := makeTestTarget(800, 600)
		for j := range 10 {
			shape := gg.DetectedShape{
				Kind: gg.ShapeCircle, CenterX: float64(j*80 + 40), CenterY: 300, RadiusX: 30, RadiusY: 30,
			}
			_ = rc.FillShape(target, shape, paint)
		}
		pm := gg.NewPixmap(800, 600)
		sr := gg.NewSoftwareRenderer(800, 600)
		rc.dispatchDrawsToSoftware(pm, sr)
		rc.Close()
	}
}

// BenchmarkProfileFullFrame_100Shapes simulates a full frame: 100 shapes enqueue + flush.
func BenchmarkProfileFullFrame_100Shapes(b *testing.B) {
	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	b.ResetTimer()
	for b.Loop() {
		rc := s.NewRenderContext()
		target := makeTestTarget(800, 600)
		for j := range 100 {
			shape := gg.DetectedShape{
				Kind: gg.ShapeCircle, CenterX: float64(j%10) * 80, CenterY: float64(j/10) * 60, RadiusX: 20, RadiusY: 20,
			}
			_ = rc.FillShape(target, shape, paint)
		}
		pm := gg.NewPixmap(800, 600)
		sr := gg.NewSoftwareRenderer(800, 600)
		rc.dispatchDrawsToSoftware(pm, sr)
		rc.Close()
	}
}

// BenchmarkProfileFullFrame_10Paths simulates 10 path fills: enqueue + flush.
func BenchmarkProfileFullFrame_10Paths(b *testing.B) {
	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Blue))

	path := gg.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(90, 50)
	path.LineTo(10, 90)
	path.Close()

	b.ResetTimer()
	for b.Loop() {
		rc := s.NewRenderContext()
		target := makeTestTarget(800, 600)
		for range 10 {
			_ = rc.FillPath(target, path, paint)
		}
		pm := gg.NewPixmap(800, 600)
		sr := gg.NewSoftwareRenderer(800, 600)
		rc.dispatchDrawsToSoftware(pm, sr)
		rc.Close()
	}
}

// BenchmarkProfileFullFrame_MixedDraws simulates a realistic UI frame:
// 20 shapes + 5 paths + varying clips.
func BenchmarkProfileFullFrame_MixedDraws(b *testing.B) {
	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	shapePaint := gg.NewPaint()
	shapePaint.SetBrush(gg.Solid(gg.Red))
	pathPaint := gg.NewPaint()
	pathPaint.SetBrush(gg.Solid(gg.Blue))

	triPath := gg.NewPath()
	triPath.MoveTo(10, 10)
	triPath.LineTo(90, 50)
	triPath.LineTo(10, 90)
	triPath.Close()

	b.ResetTimer()
	for b.Loop() {
		rc := s.NewRenderContext()
		target := makeTestTarget(800, 600)

		// 20 shapes with clip changes every 5
		for j := range 20 {
			if j%5 == 0 {
				rc.SetClipRect(uint32(j*40), 0, 200, 600)
			}
			shape := gg.DetectedShape{
				Kind: gg.ShapeCircle, CenterX: float64(j*40 + 20), CenterY: 300, RadiusX: 15, RadiusY: 15,
			}
			_ = rc.FillShape(target, shape, shapePaint)
		}
		// 5 paths
		for range 5 {
			_ = rc.FillPath(target, triPath, pathPaint)
		}

		pm := gg.NewPixmap(800, 600)
		sr := gg.NewSoftwareRenderer(800, 600)
		rc.dispatchDrawsToSoftware(pm, sr)
		rc.Close()
	}
}

// --- M-4: pendingDraws[:0] GC reference retention ---

// BenchmarkProfilePendingDrawsRetention measures M-4: whether pendingDraws[:0]
// retains references to Path clones and tessellation data.
func BenchmarkProfilePendingDrawsRetention(b *testing.B) {
	s := NewGPUShared()
	s.strategy = strategyRasterAtlas
	s.deviceReady = true
	s.gpuReady = false
	s.cpuFallback = gg.SDFAccelerator{}

	path := gg.NewPath()
	path.MoveTo(10, 10)
	path.LineTo(90, 50)
	path.LineTo(10, 90)
	path.Close()
	paint := gg.NewPaint()
	paint.SetBrush(gg.Solid(gg.Red))

	// Simulate steady-state: enqueue + dispatch + re-use slice.
	b.ResetTimer()
	for b.Loop() {
		rc := &GPURenderContext{shared: s}
		target := makeTestTarget(100, 100)

		// Enqueue
		for range 50 {
			_ = rc.FillPath(target, path, paint)
		}

		// Dispatch (clears via [:0])
		pm := gg.NewPixmap(100, 100)
		sr := gg.NewSoftwareRenderer(100, 100)
		rc.dispatchDrawsToSoftware(pm, sr)
		rc.pendingDraws = rc.pendingDraws[:0]

		// Check: old capacity still holds references
		// (this is the GC concern — we measure allocation pressure)
	}
}

// --- L-6: strokeResultToPath allocation ---

// BenchmarkProfileStrokeResultToPath measures L-6: allocation of new Path per call.
// After P3 optimization: uses a pre-allocated scratch path (0 allocs steady-state).
func BenchmarkProfileStrokeResultToPath(b *testing.B) {
	// Simulate stroke output for a simple line stroke (2 line caps + 2 side lines).
	verbs := []stroke.PathVerb{
		stroke.VerbMoveTo, stroke.VerbLineTo, stroke.VerbLineTo, stroke.VerbLineTo, stroke.VerbClose,
		stroke.VerbMoveTo, stroke.VerbLineTo, stroke.VerbClose,
	}
	coords := make([]float64, 16) // 8 points (2 coords per point)
	for i := range coords {
		coords[i] = float64(i * 10)
	}

	dst := gg.NewPath()
	b.ResetTimer()
	for b.Loop() {
		strokeResultToPath(dst, verbs, coords)
	}
}
