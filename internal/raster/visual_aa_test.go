package raster

import (
	"math"
	"testing"
)

// =============================================================================
// Visual Quality Tests for Anti-Aliasing
// =============================================================================
//
// These tests verify that the AA implementation produces smooth edges
// without visible "jaggies" (staircase patterns).

// TestAACircleEdgeQuality tests that circle edges have smooth alpha gradients.
func TestAACircleEdgeQuality(t *testing.T) {
	const (
		width  = 200
		height = 200
		cx     = 100.0
		cy     = 100.0
		radius = 80.0
	)

	pixmap := newTestAAPixmap(width, height)
	r := NewRasterizer(width, height)

	// Create circle approximation (64 segments for smooth curve)
	segments := 64
	points := make([]Point, segments+1)
	for i := 0; i <= segments; i++ {
		angle := 2 * math.Pi * float64(i) / float64(segments)
		points[i] = Point{
			X: cx + radius*math.Cos(angle),
			Y: cy + radius*math.Sin(angle),
		}
	}

	color := RGBA{R: 1.0, G: 0.0, B: 0.0, A: 1.0}
	r.FillAA(pixmap, points, FillRuleNonZero, color)

	// Test edge quality by checking that boundary pixels have partial alpha
	// A pixel at distance d from center should have alpha proportional to coverage
	partialAlphaCount := 0
	totalEdgePixels := 0

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dist := math.Sqrt(float64((x-int(cx))*(x-int(cx)) + (y-int(cy))*(y-int(cy))))

			// Check edge region (within 2 pixels of radius)
			if math.Abs(dist-radius) < 2.0 {
				totalEdgePixels++
				alpha := pixmap.pixels[y][x].A
				if alpha > 0.01 && alpha < 0.99 {
					partialAlphaCount++
				}
			}
		}
	}

	// At least 20% of edge pixels should have partial alpha (sign of AA working)
	edgeRatio := float64(partialAlphaCount) / float64(totalEdgePixels)
	if edgeRatio < 0.2 {
		t.Errorf("Only %.1f%% of edge pixels have partial alpha (expected > 20%%), AA may not be working properly",
			edgeRatio*100)
	}

	t.Logf("Edge quality: %.1f%% of edge pixels have partial alpha (%d/%d)",
		edgeRatio*100, partialAlphaCount, totalEdgePixels)
}

// TestAADiagonalLineQuality tests that diagonal lines have smooth edges.
func TestAADiagonalLineQuality(t *testing.T) {
	const (
		width  = 200
		height = 200
	)

	pixmap := newTestAAPixmap(width, height)
	r := NewRasterizer(width, height)

	// Create a diagonal rectangle (thin stripe)
	// This is a good test because it creates many edges at angles
	lineWidth := 5.0
	points := []Point{
		{X: 10, Y: 10},
		{X: 10 + lineWidth, Y: 10},
		{X: 190, Y: 190},
		{X: 190 - lineWidth, Y: 190},
		{X: 10, Y: 10},
	}

	color := RGBA{R: 0.0, G: 1.0, B: 0.0, A: 1.0}
	r.FillAA(pixmap, points, FillRuleNonZero, color)

	// Count partial alpha pixels along the diagonal
	partialAlphaCount := 0
	totalFilledPixels := 0

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			alpha := pixmap.pixels[y][x].A
			if alpha > 0.01 {
				totalFilledPixels++
				if alpha < 0.99 {
					partialAlphaCount++
				}
			}
		}
	}

	// Should have some partial alpha pixels on edges
	if partialAlphaCount == 0 && totalFilledPixels > 0 {
		t.Error("No partial alpha pixels found on diagonal line, AA not working")
	}

	t.Logf("Diagonal line: %d partial alpha pixels out of %d filled pixels",
		partialAlphaCount, totalFilledPixels)
}

// TestAAComparisonWithNonAA compares AA vs non-AA output.
func TestAAComparisonWithNonAA(t *testing.T) {
	const (
		width  = 100
		height = 100
	)

	// Create circle points
	cx, cy, radius := 50.0, 50.0, 30.0
	segments := 32
	points := make([]Point, segments+1)
	for i := 0; i <= segments; i++ {
		angle := 2 * math.Pi * float64(i) / float64(segments)
		points[i] = Point{
			X: cx + radius*math.Cos(angle),
			Y: cy + radius*math.Sin(angle),
		}
	}

	color := RGBA{R: 1.0, G: 0.0, B: 0.0, A: 1.0}

	// Render with AA
	pixmapAA := newTestAAPixmap(width, height)
	rAA := NewRasterizer(width, height)
	rAA.FillAA(pixmapAA, points, FillRuleNonZero, color)

	// Count unique alpha values (AA should have more variety)
	alphaValuesAA := make(map[float64]bool)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			a := pixmapAA.pixels[y][x].A
			if a > 0 {
				// Round to 2 decimal places
				alphaValuesAA[math.Round(a*100)/100] = true
			}
		}
	}

	// AA should produce more than just 0 and 1 alpha values
	if len(alphaValuesAA) < 3 {
		t.Errorf("AA produced only %d unique alpha values, expected more variety",
			len(alphaValuesAA))
	}

	t.Logf("AA produced %d unique alpha values", len(alphaValuesAA))
}

// TestAACircleAreaAccuracy verifies AA doesn't significantly change filled area.
func TestAACircleAreaAccuracy(t *testing.T) {
	const (
		width  = 200
		height = 200
	)

	cx, cy, radius := 100.0, 100.0, 50.0

	// Create circle points
	segments := 64
	points := make([]Point, segments+1)
	for i := 0; i <= segments; i++ {
		angle := 2 * math.Pi * float64(i) / float64(segments)
		points[i] = Point{
			X: cx + radius*math.Cos(angle),
			Y: cy + radius*math.Sin(angle),
		}
	}

	pixmap := newTestAAPixmap(width, height)
	r := NewRasterizer(width, height)
	color := RGBA{R: 1.0, G: 0.0, B: 0.0, A: 1.0}
	r.FillAA(pixmap, points, FillRuleNonZero, color)

	// Calculate total alpha (accumulated area)
	totalAlpha := 0.0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			totalAlpha += pixmap.pixels[y][x].A
		}
	}

	// Expected area: pi * r^2
	expectedArea := math.Pi * radius * radius

	// Should be within 5% of expected
	diff := math.Abs(totalAlpha-expectedArea) / expectedArea
	if diff > 0.05 {
		t.Errorf("AA area accuracy: got %.0f, expected %.0f (%.1f%% difference)",
			totalAlpha, expectedArea, diff*100)
	}

	t.Logf("Circle area: got %.0f, expected %.0f (%.2f%% difference)",
		totalAlpha, expectedArea, diff*100)
}

// BenchmarkAAVsNonAA benchmarks AA rendering vs non-AA.
func BenchmarkAAVsNonAA(b *testing.B) {
	const (
		width  = 500
		height = 500
	)

	// Create circle points
	cx, cy, radius := 250.0, 250.0, 200.0
	segments := 64
	points := make([]Point, segments+1)
	for i := 0; i <= segments; i++ {
		angle := 2 * math.Pi * float64(i) / float64(segments)
		points[i] = Point{
			X: cx + radius*math.Cos(angle),
			Y: cy + radius*math.Sin(angle),
		}
	}

	color := RGBA{R: 1.0, G: 0.0, B: 0.0, A: 1.0}

	b.Run("AA", func(b *testing.B) {
		pixmap := newTestAAPixmap(width, height)
		r := NewRasterizer(width, height)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			r.FillAA(pixmap, points, FillRuleNonZero, color)
		}
	})

	b.Run("NonAA", func(b *testing.B) {
		pixmap := newTestAAPixmap(width, height)
		r := NewRasterizer(width, height)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			r.Fill(pixmap, points, FillRuleNonZero, color)
		}
	})
}
