package gg

import "testing"

// BenchmarkPixmap_Clear benchmarks clearing pixmaps of various sizes.
func BenchmarkPixmap_Clear(b *testing.B) {
	sizes := []struct {
		name   string
		width  int
		height int
	}{
		{"100x100", 100, 100},
		{"512x512", 512, 512},
		{"1000x1000", 1000, 1000},
		{"1920x1080", 1920, 1080},
		{"2048x2048", 2048, 2048},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			pm := NewPixmap(size.width, size.height)
			color := Red
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				pm.Clear(color)
			}
			// Report MB/s
			pixels := int64(size.width * size.height)
			b.SetBytes(pixels * 4) // 4 bytes per pixel (RGBA)
		})
	}
}

// BenchmarkPixmap_FillSpanVsSetPixel compares FillSpan against SetPixel.
// This validates the optimization in Week 4 (rasterizer with FillSpan).
func BenchmarkPixmap_FillSpanVsSetPixel(b *testing.B) {
	pm := NewPixmap(2000, 1000)
	color := Red

	spans := []struct {
		name   string
		pixels int
	}{
		{"10px", 10},
		{"50px", 50},
		{"100px", 100},
		{"500px", 500},
		{"1000px", 1000},
	}

	for _, span := range spans {
		// Benchmark SetPixel (scalar, baseline)
		b.Run("SetPixel_"+span.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				for x := 0; x < span.pixels; x++ {
					pm.SetPixel(x, 500, color)
				}
			}
			b.SetBytes(int64(span.pixels * 4))
		})

		// Benchmark FillSpan (optimized)
		b.Run("FillSpan_"+span.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				pm.FillSpan(0, span.pixels, 500, color)
			}
			b.SetBytes(int64(span.pixels * 4))
		})
	}
}

// BenchmarkDraw_FillRect benchmarks rectangle filling at various sizes.
func BenchmarkDraw_FillRect(b *testing.B) {
	ctx := NewContext(2000, 2000)
	ctx.SetRGBA(1, 0, 0, 1)

	rects := []struct {
		name string
		size int
	}{
		{"10x10", 10},
		{"50x50", 50},
		{"100x100", 100},
		{"500x500", 500},
		{"1000x1000", 1000},
	}

	for _, rect := range rects {
		b.Run(rect.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ctx.DrawRectangle(0, 0, float64(rect.size), float64(rect.size))
				ctx.Fill()
			}
			pixels := int64(rect.size * rect.size)
			b.SetBytes(pixels * 4)
		})
	}
}

// BenchmarkDraw_FillCircle benchmarks circle filling at various sizes.
func BenchmarkDraw_FillCircle(b *testing.B) {
	ctx := NewContext(2000, 2000)
	ctx.SetRGBA(0, 0, 1, 1)

	circles := []struct {
		name   string
		radius float64
	}{
		{"r10", 10},
		{"r50", 50},
		{"r100", 100},
		{"r250", 250},
		{"r500", 500},
	}

	for _, circle := range circles {
		b.Run(circle.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ctx.DrawCircle(1000, 1000, circle.radius)
				ctx.Fill()
			}
			// Approximate area for bytes calculation
			area := int64(3.14159 * circle.radius * circle.radius)
			b.SetBytes(area * 4)
		})
	}
}

// BenchmarkDraw_StrokePath benchmarks path stroking.
func BenchmarkDraw_StrokePath(b *testing.B) {
	ctx := NewContext(1000, 1000)
	ctx.SetRGBA(0, 1, 0, 1)
	ctx.SetLineWidth(5)

	paths := []struct {
		name     string
		segments int
	}{
		{"10_segments", 10},
		{"50_segments", 50},
		{"100_segments", 100},
	}

	for _, path := range paths {
		b.Run(path.name, func(b *testing.B) {
			// Create a complex path with many segments
			for i := 0; i < path.segments; i++ {
				x := float64(i * 10)
				y := float64((i % 2) * 100)
				if i == 0 {
					ctx.MoveTo(x, y)
				} else {
					ctx.LineTo(x, y)
				}
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ctx.Stroke()
			}
		})
	}
}

// BenchmarkDraw_ComplexScene benchmarks a realistic drawing scenario.
func BenchmarkDraw_ComplexScene(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ctx := NewContext(1000, 1000)

		// Background
		ctx.SetRGBA(1, 1, 1, 1)
		ctx.Clear()

		// Draw multiple shapes
		ctx.SetRGBA(1, 0, 0, 0.8)
		ctx.DrawRectangle(100, 100, 200, 150)
		ctx.Fill()

		ctx.SetRGBA(0, 1, 0, 0.8)
		ctx.DrawCircle(500, 500, 100)
		ctx.Fill()

		ctx.SetRGBA(0, 0, 1, 0.8)
		ctx.DrawRegularPolygon(6, 700, 300, 80, 0)
		ctx.Fill()

		// Stroked path
		ctx.SetRGBA(0, 0, 0, 1)
		ctx.SetLineWidth(3)
		ctx.MoveTo(100, 800)
		ctx.LineTo(300, 900)
		ctx.LineTo(500, 850)
		ctx.LineTo(700, 900)
		ctx.Stroke()
	}
	b.SetBytes(1000 * 1000 * 4) // Full canvas
}

// BenchmarkAlphaBlending benchmarks transparent shape compositing.
func BenchmarkAlphaBlending(b *testing.B) {
	ctx := NewContext(1000, 1000)
	ctx.SetRGBA(1, 1, 1, 1)
	ctx.Clear()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Draw overlapping semi-transparent rectangles
		ctx.SetRGBA(1, 0, 0, 0.5)
		ctx.DrawRectangle(100, 100, 400, 400)
		ctx.Fill()

		ctx.SetRGBA(0, 1, 0, 0.5)
		ctx.DrawRectangle(200, 200, 400, 400)
		ctx.Fill()

		ctx.SetRGBA(0, 0, 1, 0.5)
		ctx.DrawRectangle(300, 300, 400, 400)
		ctx.Fill()
	}
	b.SetBytes(1000 * 1000 * 4)
}
