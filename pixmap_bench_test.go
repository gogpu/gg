package gg

import "testing"

// BenchmarkFillSpanVsSetPixel compares FillSpan performance against repeated SetPixel calls.
func BenchmarkFillSpanVsSetPixel(b *testing.B) {
	pm := NewPixmap(1000, 1000)
	color := Red

	benchmarks := []struct {
		name   string
		pixels int
	}{
		{"10px", 10},
		{"50px", 50},
		{"100px", 100},
		{"500px", 500},
	}

	for _, bm := range benchmarks {
		// Benchmark SetPixel (scalar)
		b.Run("SetPixel_"+bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				for x := 0; x < bm.pixels; x++ {
					pm.SetPixel(x, 500, color)
				}
			}
		})

		// Benchmark FillSpan (optimized)
		b.Run("FillSpan_"+bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				pm.FillSpan(0, bm.pixels, 500, color)
			}
		})
	}
}

// BenchmarkFillSpanBlendVsSetPixel compares FillSpanBlend performance.
func BenchmarkFillSpanBlendVsSetPixel(b *testing.B) {
	pm := NewPixmap(1000, 1000)
	pm.Clear(White)
	color := RGBA2(1, 0, 0, 0.5) // Semi-transparent red

	benchmarks := []struct {
		name   string
		pixels int
	}{
		{"10px", 10},
		{"50px", 50},
		{"100px", 100},
	}

	for _, bm := range benchmarks {
		// Benchmark SetPixel with manual blending (scalar)
		b.Run("SetPixel_"+bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				for x := 0; x < bm.pixels; x++ {
					pm.SetPixel(x, 500, color)
				}
			}
		})

		// Benchmark FillSpanBlend (optimized)
		b.Run("FillSpanBlend_"+bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				pm.FillSpanBlend(0, bm.pixels, 500, color)
			}
		})
	}
}
