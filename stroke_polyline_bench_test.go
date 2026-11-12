package gg_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/gogpu/gg"
)

// BenchmarkStrokeLongPolyline strokes a time series the way a chart does: one
// path, two points per pixel column across a wide canvas, wiggling up and down
// so that every scanline crosses hundreds of edges at once.
func BenchmarkStrokeLongPolyline(b *testing.B) {
	const w, h = 1920, 300
	dc := gg.NewContext(w, h)
	dc.SetRGB(0.2, 0.4, 0.8)
	dc.SetLineWidth(1.5)
	for b.Loop() {
		n := 2 * w
		for i := 0; i < n; i++ {
			x := float64(i) / 2
			y := h/2 + 100*math.Sin(float64(i)/3) + 20*math.Sin(float64(i)/50)
			if i == 0 {
				dc.MoveTo(x, y)
			} else {
				dc.LineTo(x, y)
			}
		}
		if err := dc.Stroke(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStrokeHatch strokes a hatching the way a chart fills a band with
// it: one path of short parallel diagonals, five pixels apart, across the
// full width.
func BenchmarkStrokeHatch(b *testing.B) {
	const w, h = 1920, 300
	for _, band := range []float64{24, 120} {
		b.Run(fmt.Sprintf("band%v", band), func(b *testing.B) {
			dc := gg.NewContext(w, h)
			dc.SetRGB(0.2, 0.2, 0.2)
			dc.SetLineWidth(1.48)
			for b.Loop() {
				for x := -band; x < w; x += 5 {
					dc.MoveTo(x, 100+band)
					dc.LineTo(x+band, 100)
				}
				if err := dc.Stroke(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
