// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package native

import (
	"runtime"
	"testing"

	"github.com/gogpu/gg/scene"
)

// =============================================================================
// Stress Tests for Analytic Anti-Aliasing System
// These tests verify stability under extreme conditions
// =============================================================================

// TestStress100Circles tests rendering 100 circles.
func TestStress100Circles(t *testing.T) {
	filler := NewAnalyticFiller(800, 600)
	eb := NewEdgeBuilder(2)

	// Create 100 circles
	for i := 0; i < 100; i++ {
		x := float32(50 + (i%10)*75)
		y := float32(50 + (i/10)*55)
		path := scene.NewPath().Circle(x, y, 25)
		eb.BuildFromScenePath(path, scene.IdentityAffine())
	}

	scanlineCount := 0
	filler.Fill(eb, FillRuleNonZero, func(_ int, _ *AlphaRuns) {
		scanlineCount++
	})

	if scanlineCount == 0 {
		t.Error("expected to process scanlines for 100 circles")
	}

	t.Logf("100 circles: %d edges, %d scanlines", eb.EdgeCount(), scanlineCount)
}

// TestStress1000Edges tests a path with 1000 edges.
func TestStress1000Edges(t *testing.T) {
	filler := NewAnalyticFiller(1000, 1000)
	eb := NewEdgeBuilder(2)

	// Create path with 1000 line segments
	path := scene.NewPath()
	path.MoveTo(500, 100)
	for i := 0; i < 1000; i++ {
		angle := float64(i) * 0.00628318530718 // 2*pi/1000
		r := 400 + float32(i%50)
		x := 500 + r*float32(cosApprox(angle))
		y := 500 + r*float32(sinApprox(angle))
		path.LineTo(x, y)
	}
	path.Close()

	eb.BuildFromScenePath(path, scene.IdentityAffine())

	if eb.EdgeCount() < 100 {
		t.Errorf("expected many edges, got %d", eb.EdgeCount())
	}

	scanlineCount := 0
	filler.Fill(eb, FillRuleNonZero, func(_ int, _ *AlphaRuns) {
		scanlineCount++
	})

	if scanlineCount == 0 {
		t.Error("expected to process scanlines")
	}

	t.Logf("1000-edge path: %d edges, %d scanlines", eb.EdgeCount(), scanlineCount)
}

// TestStress100QuadraticCurves tests 100 quadratic Bezier curves.
func TestStress100QuadraticCurves(t *testing.T) {
	filler := NewAnalyticFiller(800, 600)
	eb := NewEdgeBuilder(2)

	path := scene.NewPath()
	path.MoveTo(50, 300)
	for i := 0; i < 100; i++ {
		x := float32(50 + (i+1)*7)
		y := float32(300)
		cx := float32(50 + i*7 + 3)
		cy := float32(100 + (i%20)*20)
		path.QuadTo(cx, cy, x, y)
	}
	path.LineTo(750, 500)
	path.LineTo(50, 500)
	path.Close()

	eb.BuildFromScenePath(path, scene.IdentityAffine())

	if eb.QuadraticEdgeCount() < 50 {
		t.Logf("Warning: expected many quadratic edges, got %d", eb.QuadraticEdgeCount())
	}

	scanlineCount := 0
	filler.Fill(eb, FillRuleNonZero, func(_ int, _ *AlphaRuns) {
		scanlineCount++
	})

	t.Logf("100 quadratics: %d quad edges, %d scanlines",
		eb.QuadraticEdgeCount(), scanlineCount)
}

// TestStress100CubicCurves tests 100 cubic Bezier curves.
func TestStress100CubicCurves(t *testing.T) {
	filler := NewAnalyticFiller(1000, 600)
	eb := NewEdgeBuilder(2)

	path := scene.NewPath()
	path.MoveTo(50, 300)
	for i := 0; i < 100; i++ {
		x := float32(50 + (i+1)*9)
		y := float32(300)
		c1x := float32(50 + i*9 + 2)
		c1y := float32(100 + (i%10)*20)
		c2x := float32(50 + i*9 + 6)
		c2y := float32(500 - (i%10)*20)
		path.CubicTo(c1x, c1y, c2x, c2y, x, y)
	}
	path.LineTo(950, 550)
	path.LineTo(50, 550)
	path.Close()

	eb.BuildFromScenePath(path, scene.IdentityAffine())

	if eb.CubicEdgeCount() < 50 {
		t.Logf("Warning: expected many cubic edges, got %d", eb.CubicEdgeCount())
	}

	scanlineCount := 0
	filler.Fill(eb, FillRuleNonZero, func(_ int, _ *AlphaRuns) {
		scanlineCount++
	})

	t.Logf("100 cubics: %d cubic edges, %d scanlines",
		eb.CubicEdgeCount(), scanlineCount)
}

// TestStress500Curves tests a path with 500 curves (reasonable for CI with race detector).
func TestStress500Curves(t *testing.T) {
	filler := NewAnalyticFiller(500, 500)
	eb := NewEdgeBuilder(2)

	path := scene.NewPath()
	path.MoveTo(250, 50)

	// 250 quadratics
	for i := 0; i < 250; i++ {
		angle := float64(i) * 0.02513274122872 // 2*pi/250
		r := 150 + float32(i%50)
		x := 250 + r*float32(cosApprox(angle))
		y := 250 + r*float32(sinApprox(angle))
		cx := 250 + (r-25)*float32(cosApprox(angle-0.012))
		cy := 250 + (r-25)*float32(sinApprox(angle-0.012))
		path.QuadTo(cx, cy, x, y)
	}

	// 250 cubics
	for i := 0; i < 250; i++ {
		angle := float64(i) * 0.02513274122872 // 2*pi/250
		r := 100 + float32(i%50)
		x := 250 + r*float32(cosApprox(angle))
		y := 250 + r*float32(sinApprox(angle))
		c1x := 250 + (r+15)*float32(cosApprox(angle-0.008))
		c1y := 250 + (r+15)*float32(sinApprox(angle-0.008))
		c2x := 250 + (r-15)*float32(cosApprox(angle-0.004))
		c2y := 250 + (r-15)*float32(sinApprox(angle-0.004))
		path.CubicTo(c1x, c1y, c2x, c2y, x, y)
	}

	path.Close()

	eb.BuildFromScenePath(path, scene.IdentityAffine())

	totalEdges := eb.EdgeCount()
	if totalEdges < 100 {
		t.Errorf("expected many edges, got %d", totalEdges)
	}

	scanlineCount := 0
	filler.Fill(eb, FillRuleNonZero, func(_ int, _ *AlphaRuns) {
		scanlineCount++
	})

	if scanlineCount == 0 {
		t.Error("expected to process scanlines")
	}

	t.Logf("500 curves: %d total edges (%d lines, %d quads, %d cubics), %d scanlines",
		totalEdges, eb.LineEdgeCount(), eb.QuadraticEdgeCount(),
		eb.CubicEdgeCount(), scanlineCount)
}

// TestStressLargeCanvas tests rendering on a 1080p canvas.
func TestStressLargeCanvas(t *testing.T) {
	filler := NewAnalyticFiller(1920, 1080)
	eb := NewEdgeBuilder(2)

	// Large circle
	path := scene.NewPath().Circle(960, 540, 400)
	eb.BuildFromScenePath(path, scene.IdentityAffine())

	scanlineCount := 0
	filler.Fill(eb, FillRuleNonZero, func(_ int, _ *AlphaRuns) {
		scanlineCount++
	})

	if scanlineCount < 500 {
		t.Errorf("expected many scanlines for 1080p canvas, got %d", scanlineCount)
	}

	t.Logf("1080p canvas: %d scanlines", scanlineCount)
}

// TestStressNestedPaths tests deeply nested subpaths.
func TestStressNestedPaths(t *testing.T) {
	filler := NewAnalyticFiller(500, 500)
	eb := NewEdgeBuilder(2)

	// Create 50 nested rectangles
	for i := 0; i < 50; i++ {
		offset := float32(i * 5)
		path := scene.NewPath()
		path.MoveTo(offset, offset)
		path.LineTo(500-offset, offset)
		path.LineTo(500-offset, 500-offset)
		path.LineTo(offset, 500-offset)
		path.Close()
		eb.BuildFromScenePath(path, scene.IdentityAffine())
	}

	if eb.EdgeCount() < 100 {
		t.Errorf("expected many edges, got %d", eb.EdgeCount())
	}

	scanlineCount := 0
	filler.Fill(eb, FillRuleNonZero, func(_ int, _ *AlphaRuns) {
		scanlineCount++
	})

	t.Logf("50 nested rectangles: %d edges, %d scanlines",
		eb.EdgeCount(), scanlineCount)
}

// TestStressConcurrentFill tests concurrent fills (should not share state).
func TestStressConcurrentFill(t *testing.T) {
	path := scene.NewPath().Circle(100, 100, 50)

	// Create independent fillers
	const numGoroutines = 4
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- true }()

			// Each goroutine has its own EdgeBuilder and Filler
			eb := NewEdgeBuilder(2)
			eb.BuildFromScenePath(path, scene.IdentityAffine())

			filler := NewAnalyticFiller(200, 200)

			for j := 0; j < 10; j++ {
				filler.Reset()
				filler.Fill(eb, FillRuleNonZero, func(_ int, _ *AlphaRuns) {})
			}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// TestStressResetReuse tests repeated reset and reuse.
func TestStressResetReuse(t *testing.T) {
	filler := NewAnalyticFiller(500, 500)
	eb := NewEdgeBuilder(2)

	paths := []*scene.Path{
		scene.NewPath().Circle(250, 250, 100),
		createRectanglePath(100, 100, 300, 300),
		createComplexCurvesPath(),
	}

	for i := 0; i < 50; i++ {
		path := paths[i%len(paths)]
		eb.Reset()
		eb.BuildFromScenePath(path, scene.IdentityAffine())
		filler.Reset()
		filler.Fill(eb, FillRuleNonZero, func(_ int, _ *AlphaRuns) {})
	}
}

// =============================================================================
// Memory Usage Tests
// =============================================================================

// TestMemoryUsageAnalytic tests memory usage of the analytic filler.
func TestMemoryUsageAnalytic(t *testing.T) {
	// Force GC to get clean baseline
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Create filler and process path
	filler := NewAnalyticFiller(1920, 1080)
	eb := NewEdgeBuilder(2)

	path := scene.NewPath().Circle(960, 540, 400)
	eb.BuildFromScenePath(path, scene.IdentityAffine())

	filler.Fill(eb, FillRuleNonZero, func(_ int, _ *AlphaRuns) {})

	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	allocatedKB := (m2.TotalAlloc - m1.TotalAlloc) / 1024
	t.Logf("Analytic filler (1080p circle): ~%d KB allocated", allocatedKB)

	// Sanity check: should use less than 50MB for a single 1080p fill
	if allocatedKB > 50*1024 {
		t.Errorf("unexpected high memory usage: %d KB", allocatedKB)
	}
}

// TestMemoryUsageEdgeBuilder tests memory usage of the edge builder.
func TestMemoryUsageEdgeBuilder(t *testing.T) {
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	eb := NewEdgeBuilder(2)

	// Build 100 circles
	for i := 0; i < 100; i++ {
		x := float32(50 + (i%10)*100)
		y := float32(50 + (i/10)*100)
		path := scene.NewPath().Circle(x, y, 40)
		eb.BuildFromScenePath(path, scene.IdentityAffine())
	}

	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	allocatedKB := (m2.TotalAlloc - m1.TotalAlloc) / 1024
	t.Logf("EdgeBuilder (100 circles): ~%d KB allocated, %d edges",
		allocatedKB, eb.EdgeCount())

	// Should be reasonable
	if allocatedKB > 10*1024 {
		t.Errorf("unexpected high memory usage: %d KB", allocatedKB)
	}
}

// TestMemoryUsageAlphaRuns tests memory usage of AlphaRuns.
func TestMemoryUsageAlphaRuns(t *testing.T) {
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Create wide alpha runs (4K width)
	ar := NewAlphaRuns(3840)

	// Simulate heavy usage
	for i := 0; i < 1000; i++ {
		ar.Reset()
		ar.Add(100, 128, 1000, 128)
		ar.SetOffset(0)
		ar.Add(1500, 255, 500, 255)
	}

	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	allocatedKB := (m2.TotalAlloc - m1.TotalAlloc) / 1024
	t.Logf("AlphaRuns (4K width, 1000 resets): ~%d KB allocated", allocatedKB)

	// Should be very efficient with reset
	if allocatedKB > 1024 {
		t.Errorf("unexpected high memory usage for AlphaRuns: %d KB", allocatedKB)
	}
}
