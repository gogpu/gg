//go:build !nogpu

package gpu

import (
	"math"
	"testing"

	"github.com/gogpu/gg/scene"
	"github.com/gogpu/naga"
)

// TestFlattenShaderCompilation tests that the WGSL shader compiles to SPIR-V.
func TestFlattenShaderCompilation(t *testing.T) {
	if flattenShaderWGSL == "" {
		t.Fatal("flatten shader source is empty")
	}

	// Test compilation via naga
	spirvBytes, err := naga.Compile(flattenShaderWGSL)
	if err != nil {
		errStr := err.Error()
		if contains(errStr, "runtime-sized arrays not yet implemented") {
			t.Skip("Skipping: naga doesn't yet support runtime-sized arrays")
		}
		if contains(errStr, "not yet implemented") || contains(errStr, "not supported") {
			t.Skipf("Skipping: naga feature not yet implemented: %v", err)
		}
		if contains(errStr, "lowering error") || contains(errStr, "atomic") {
			t.Skipf("Skipping: naga atomic/lowering limitation: %v", err)
		}
		t.Fatalf("failed to compile flatten shader: %v", err)
	}

	if len(spirvBytes) == 0 {
		t.Error("SPIR-V output is empty")
	}

	// Verify SPIR-V magic number (0x07230203)
	if len(spirvBytes) < 4 {
		t.Fatal("SPIR-V too short")
	}
	magic := uint32(spirvBytes[0]) |
		uint32(spirvBytes[1])<<8 |
		uint32(spirvBytes[2])<<16 |
		uint32(spirvBytes[3])<<24
	if magic != 0x07230203 {
		t.Errorf("invalid SPIR-V magic: 0x%08X, want 0x07230203", magic)
	}

	t.Logf("Flatten shader compiled to %d bytes of SPIR-V", len(spirvBytes))
}

// TestGPUPathElementConversion tests path to GPU element conversion.
func TestGPUPathElementConversion(t *testing.T) {
	path := scene.NewPath()
	path.MoveTo(10, 20)
	path.LineTo(30, 40)
	path.QuadTo(50, 60, 70, 80)
	path.CubicTo(90, 100, 110, 120, 130, 140)
	path.Close()

	// Create a mock rasterizer for conversion
	r := &GPUFlattenRasterizer{}
	elements, points := r.ConvertPathToGPU(path)

	// Check element count
	if len(elements) != 5 {
		t.Errorf("expected 5 elements, got %d", len(elements))
	}

	// Check verb types
	expectedVerbs := []uint32{
		uint32(scene.VerbMoveTo),
		uint32(scene.VerbLineTo),
		uint32(scene.VerbQuadTo),
		uint32(scene.VerbCubicTo),
		uint32(scene.VerbClose),
	}
	for i, verb := range expectedVerbs {
		if elements[i].Verb != verb {
			t.Errorf("element %d: expected verb %d, got %d", i, verb, elements[i].Verb)
		}
	}

	// Check point counts
	expectedPointCounts := []uint32{2, 2, 4, 6, 0}
	for i, count := range expectedPointCounts {
		if elements[i].PointCount != count {
			t.Errorf("element %d: expected point count %d, got %d", i, count, elements[i].PointCount)
		}
	}

	// Check total points
	expectedPointsLen := 2 + 2 + 4 + 6 // MoveTo + LineTo + QuadTo + CubicTo
	if len(points) != expectedPointsLen {
		t.Errorf("expected %d points, got %d", expectedPointsLen, len(points))
	}
}

// TestGPUCursorStateComputation tests cursor state tracking.
func TestGPUCursorStateComputation(t *testing.T) {
	path := scene.NewPath()
	path.MoveTo(10, 20)
	path.LineTo(30, 40)
	path.LineTo(50, 60)
	path.Close()

	r := &GPUFlattenRasterizer{}
	states := r.ComputeCursorStates(path)

	if len(states) != 4 {
		t.Fatalf("expected 4 states, got %d", len(states))
	}

	// State 0: Before MoveTo - cursor at origin
	if states[0].CurX != 0 || states[0].CurY != 0 {
		t.Errorf("state 0: expected cursor (0,0), got (%v,%v)", states[0].CurX, states[0].CurY)
	}

	// State 1: After MoveTo - cursor at (10,20), start at (10,20)
	if states[1].CurX != 10 || states[1].CurY != 20 {
		t.Errorf("state 1: expected cursor (10,20), got (%v,%v)", states[1].CurX, states[1].CurY)
	}
	if states[1].StartX != 10 || states[1].StartY != 20 {
		t.Errorf("state 1: expected start (10,20), got (%v,%v)", states[1].StartX, states[1].StartY)
	}

	// State 2: After first LineTo - cursor at (30,40)
	if states[2].CurX != 30 || states[2].CurY != 40 {
		t.Errorf("state 2: expected cursor (30,40), got (%v,%v)", states[2].CurX, states[2].CurY)
	}

	// State 3: After second LineTo - cursor at (50,60), still points to start (10,20)
	if states[3].CurX != 50 || states[3].CurY != 60 {
		t.Errorf("state 3: expected cursor (50,60), got (%v,%v)", states[3].CurX, states[3].CurY)
	}
	if states[3].StartX != 10 || states[3].StartY != 20 {
		t.Errorf("state 3: expected start (10,20), got (%v,%v)", states[3].StartX, states[3].StartY)
	}
}

// TestGPUCursorStateWithQuadCubic tests cursor state with curves.
func TestGPUCursorStateWithQuadCubic(t *testing.T) {
	path := scene.NewPath()
	path.MoveTo(0, 0)
	path.QuadTo(50, 0, 100, 100)              // End at (100, 100)
	path.CubicTo(150, 100, 200, 50, 250, 200) // End at (250, 200)

	r := &GPUFlattenRasterizer{}
	states := r.ComputeCursorStates(path)

	if len(states) != 3 {
		t.Fatalf("expected 3 states, got %d", len(states))
	}

	// After MoveTo: cursor at (0,0)
	if states[1].CurX != 0 || states[1].CurY != 0 {
		t.Errorf("after MoveTo: expected cursor (0,0), got (%v,%v)", states[1].CurX, states[1].CurY)
	}

	// After QuadTo: cursor at (100,100)
	if states[2].CurX != 100 || states[2].CurY != 100 {
		t.Errorf("after QuadTo: expected cursor (100,100), got (%v,%v)", states[2].CurX, states[2].CurY)
	}
}

// TestGPUAffineConversion tests affine transform conversion.
func TestGPUAffineConversion(t *testing.T) {
	tests := []struct {
		name   string
		affine scene.Affine
	}{
		{
			name:   "identity",
			affine: scene.IdentityAffine(),
		},
		{
			name:   "translation",
			affine: scene.TranslateAffine(10, 20),
		},
		{
			name:   "scale",
			affine: scene.ScaleAffine(2, 3),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gpu := ConvertAffineToGPU(tt.affine)

			// Verify the conversion matches the affine matrix layout
			if gpu.A != tt.affine.A || gpu.B != tt.affine.B ||
				gpu.C != tt.affine.C || gpu.D != tt.affine.D ||
				gpu.E != tt.affine.E || gpu.F != tt.affine.F {
				t.Errorf("affine conversion mismatch")
			}
		})
	}
}

// TestWangQuadraticEstimate tests Wang's formula for quadratic curves.
func TestWangQuadraticEstimate(t *testing.T) {
	tests := []struct {
		name      string
		x0, y0    float32
		cx, cy    float32
		x1, y1    float32
		tolerance float32
		minSegs   int
		maxSegs   int
	}{
		{
			name: "nearly linear",
			x0:   0, y0: 0,
			cx: 50, cy: 0.001,
			x1: 100, y1: 0,
			tolerance: 0.25,
			minSegs:   1,
			maxSegs:   2,
		},
		{
			name: "moderate curve",
			x0:   0, y0: 0,
			cx: 50, cy: 50,
			x1: 100, y1: 0,
			tolerance: 0.25,
			minSegs:   2,
			maxSegs:   20,
		},
		{
			name: "sharp curve",
			x0:   0, y0: 0,
			cx: 50, cy: 200,
			x1: 100, y1: 0,
			tolerance: 0.25,
			minSegs:   5,
			maxSegs:   FlattenMaxSegmentsPerCurve,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := wangQuadratic(tt.x0, tt.y0, tt.cx, tt.cy, tt.x1, tt.y1, tt.tolerance)
			if count < tt.minSegs || count > tt.maxSegs {
				t.Errorf("expected segment count in [%d, %d], got %d",
					tt.minSegs, tt.maxSegs, count)
			}
		})
	}
}

// TestWangCubicEstimate tests Wang's formula for cubic curves.
func TestWangCubicEstimate(t *testing.T) {
	tests := []struct {
		name      string
		x0, y0    float32
		c1x, c1y  float32
		c2x, c2y  float32
		x1, y1    float32
		tolerance float32
		minSegs   int
		maxSegs   int
	}{
		{
			name: "nearly linear",
			x0:   0, y0: 0,
			c1x: 33, c1y: 0.001,
			c2x: 66, c2y: 0.001,
			x1: 100, y1: 0,
			tolerance: 0.25,
			minSegs:   1,
			maxSegs:   3,
		},
		{
			name: "S curve",
			x0:   0, y0: 0,
			c1x: 0, c1y: 100,
			c2x: 100, c2y: -100,
			x1: 100, y1: 0,
			tolerance: 0.25,
			minSegs:   3,
			maxSegs:   30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := wangCubic(tt.x0, tt.y0, tt.c1x, tt.c1y, tt.c2x, tt.c2y, tt.x1, tt.y1, tt.tolerance)
			if count < tt.minSegs || count > tt.maxSegs {
				t.Errorf("expected segment count in [%d, %d], got %d",
					tt.minSegs, tt.maxSegs, count)
			}
		})
	}
}

// TestCPUFlattenAccuracy tests that CPU flattening produces accurate results.
func TestCPUFlattenAccuracy(t *testing.T) {
	// Create a circle path using cubic Bezier approximation
	path := scene.NewPath()
	path.Circle(50, 50, 40)

	r := &GPUFlattenRasterizer{
		tolerance:  FlattenTolerance,
		flattenCtx: NewFlattenContext(),
	}

	// Mark as initialized for testing
	r.initialized = true

	segments := r.flattenCPU(path, scene.IdentityAffine(), 0.25)

	if segments.Len() == 0 {
		t.Error("expected at least one segment")
	}

	// Verify segments are monotonic (Y0 <= Y1)
	for _, seg := range segments.Segments() {
		if seg.Y0 > seg.Y1 {
			t.Errorf("non-monotonic segment: Y0=%v > Y1=%v", seg.Y0, seg.Y1)
		}
	}

	// Verify winding values are valid
	for _, seg := range segments.Segments() {
		if seg.Winding != 1 && seg.Winding != -1 {
			t.Errorf("invalid winding: %d", seg.Winding)
		}
	}

	t.Logf("Circle flattened to %d segments", segments.Len())
}

// TestEstimateSegmentCount tests segment count estimation.
func TestEstimateSegmentCount(t *testing.T) {
	r := &GPUFlattenRasterizer{
		tolerance: FlattenTolerance,
	}

	tests := []struct {
		name     string
		path     *scene.Path
		minCount int
		maxCount int
	}{
		{
			name: "simple line",
			path: func() *scene.Path {
				p := scene.NewPath()
				p.MoveTo(0, 0)
				p.LineTo(100, 100)
				return p
			}(),
			minCount: 1,
			maxCount: 1,
		},
		{
			name: "quadratic curve",
			path: func() *scene.Path {
				p := scene.NewPath()
				p.MoveTo(0, 0)
				p.QuadTo(50, 100, 100, 0)
				return p
			}(),
			minCount: 2,
			maxCount: 20,
		},
		{
			name: "cubic curve",
			path: func() *scene.Path {
				p := scene.NewPath()
				p.MoveTo(0, 0)
				p.CubicTo(0, 100, 100, 100, 100, 0)
				return p
			}(),
			minCount: 2,
			maxCount: 30,
		},
		{
			name: "triangle with close",
			path: func() *scene.Path {
				p := scene.NewPath()
				p.MoveTo(0, 0)
				p.LineTo(100, 0)
				p.LineTo(50, 100)
				p.Close()
				return p
			}(),
			minCount: 3, // 2 lines + 1 close
			maxCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := r.EstimateSegmentCount(tt.path, scene.IdentityAffine(), FlattenTolerance)
			if count < tt.minCount || count > tt.maxCount {
				t.Errorf("expected count in [%d, %d], got %d",
					tt.minCount, tt.maxCount, count)
			}
		})
	}
}

// TestFlattenEmptyPath tests handling of empty paths.
func TestFlattenEmptyPath(t *testing.T) {
	r := &GPUFlattenRasterizer{
		tolerance:   FlattenTolerance,
		flattenCtx:  NewFlattenContext(),
		initialized: true,
	}

	// Test nil path
	segments := r.flattenCPU(nil, scene.IdentityAffine(), 0.25)
	if segments == nil {
		t.Error("expected non-nil result for nil path")
	}
	if segments.Len() != 0 {
		t.Errorf("expected 0 segments for nil path, got %d", segments.Len())
	}

	// Test empty path
	emptyPath := scene.NewPath()
	segments = r.flattenCPU(emptyPath, scene.IdentityAffine(), 0.25)
	if segments.Len() != 0 {
		t.Errorf("expected 0 segments for empty path, got %d", segments.Len())
	}
}

// TestFlattenWithTransform tests flattening with various transforms.
func TestFlattenWithTransform(t *testing.T) {
	// Use a closed path to avoid implicit close line
	path := scene.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(10, 10)
	path.Close() // Explicitly close to go back to (0,0)

	r := &GPUFlattenRasterizer{
		tolerance:   FlattenTolerance,
		flattenCtx:  NewFlattenContext(),
		initialized: true,
	}

	tests := []struct {
		name      string
		transform scene.Affine
		expectX1  float32
		expectY1  float32
	}{
		{
			name:      "identity",
			transform: scene.IdentityAffine(),
			expectX1:  10,
			expectY1:  10,
		},
		{
			name:      "translation",
			transform: scene.TranslateAffine(5, 5),
			expectX1:  15,
			expectY1:  15,
		},
		{
			name:      "scale 2x",
			transform: scene.ScaleAffine(2, 2),
			expectX1:  20,
			expectY1:  20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segments := r.flattenCPU(path, tt.transform, 0.25)
			// Path produces 2 segments: line from (0,0) to (10,10) and close from (10,10) to (0,0)
			if segments.Len() != 2 {
				t.Fatalf("expected 2 segments, got %d", segments.Len())
			}

			// Find the segment going to the transformed endpoint
			found := false
			for _, seg := range segments.Segments() {
				// Check if either endpoint matches the expected transformed point
				if math.Abs(float64(seg.X1-tt.expectX1)) < 0.001 &&
					math.Abs(float64(seg.Y1-tt.expectY1)) < 0.001 {
					found = true
					break
				}
				if math.Abs(float64(seg.X0-tt.expectX1)) < 0.001 &&
					math.Abs(float64(seg.Y0-tt.expectY1)) < 0.001 {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("expected to find endpoint (%v,%v) in segments", tt.expectX1, tt.expectY1)
			}
		})
	}
}

// TestByteSerializationFlatten tests byte conversion functions.
func TestByteSerializationFlatten(t *testing.T) {
	t.Run("flattenConfig", func(t *testing.T) {
		cfg := GPUFlattenConfig{
			ElementCount:   100,
			Tolerance:      0.25,
			MaxSegments:    1000,
			TileSize:       4,
			ViewportWidth:  800,
			ViewportHeight: 600,
		}
		bytes := flattenConfigToBytes(cfg)
		if len(bytes) != 32 {
			t.Errorf("expected 32 bytes, got %d", len(bytes))
		}
	})

	t.Run("affineTransform", func(t *testing.T) {
		tf := GPUAffineTransform{
			A: 1.0, B: 0.0, C: 0.0, D: 1.0, E: 10.0, F: 20.0,
		}
		bytes := affineTransformToBytes(tf)
		if len(bytes) != 32 {
			t.Errorf("expected 32 bytes, got %d", len(bytes))
		}
	})

	t.Run("pathElements", func(t *testing.T) {
		elements := []GPUPathElement{
			{Verb: 0, PointStart: 0, PointCount: 2},
			{Verb: 1, PointStart: 2, PointCount: 2},
		}
		bytes := pathElementsToBytes(elements)
		if len(bytes) != 32 { // 2 * 16 bytes
			t.Errorf("expected 32 bytes, got %d", len(bytes))
		}
	})

	t.Run("points", func(t *testing.T) {
		points := []float32{1.0, 2.0, 3.0, 4.0}
		bytes := pointsToBytes(points)
		if len(bytes) != 16 { // 4 * 4 bytes
			t.Errorf("expected 16 bytes, got %d", len(bytes))
		}
	})

	t.Run("segmentCounts", func(t *testing.T) {
		counts := []GPUSegmentCount{
			{Count: 5, Offset: 0},
			{Count: 10, Offset: 5},
		}
		bytes := segmentCountsToBytes(counts)
		if len(bytes) != 32 { // 2 * 16 bytes
			t.Errorf("expected 32 bytes, got %d", len(bytes))
		}
	})

	t.Run("cursorStates", func(t *testing.T) {
		states := []GPUCursorState{
			{CurX: 10, CurY: 20, StartX: 0, StartY: 0},
			{CurX: 30, CurY: 40, StartX: 10, StartY: 20},
		}
		bytes := cursorStatesToBytes(states)
		if len(bytes) != 32 { // 2 * 16 bytes
			t.Errorf("expected 32 bytes, got %d", len(bytes))
		}
	})
}

// TestFlattenConfigValues tests that config values are correctly set.
func TestFlattenConfigValues(t *testing.T) {
	cfg := GPUFlattenConfig{
		ElementCount:   100,
		Tolerance:      0.25,
		MaxSegments:    1000,
		TileSize:       4,
		ViewportWidth:  800,
		ViewportHeight: 600,
	}

	bytes := flattenConfigToBytes(cfg)

	// Check element count at offset 0
	elemCount := uint32(bytes[0]) | uint32(bytes[1])<<8 | uint32(bytes[2])<<16 | uint32(bytes[3])<<24
	if elemCount != 100 {
		t.Errorf("element count: expected 100, got %d", elemCount)
	}

	// Check max segments at offset 8
	maxSegs := uint32(bytes[8]) | uint32(bytes[9])<<8 | uint32(bytes[10])<<16 | uint32(bytes[11])<<24
	if maxSegs != 1000 {
		t.Errorf("max segments: expected 1000, got %d", maxSegs)
	}
}

// BenchmarkWangQuadratic benchmarks Wang's quadratic formula.
func BenchmarkWangQuadratic(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = wangQuadratic(0, 0, 50, 100, 100, 0, 0.25)
	}
}

// BenchmarkWangCubic benchmarks Wang's cubic formula.
func BenchmarkWangCubic(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = wangCubic(0, 0, 0, 100, 100, 100, 100, 0, 0.25)
	}
}

// BenchmarkCPUFlatten benchmarks CPU flattening.
func BenchmarkCPUFlatten(b *testing.B) {
	// Create a complex path with multiple curves
	path := scene.NewPath()
	path.MoveTo(0, 0)
	for i := 0; i < 100; i++ {
		x := float32(i * 10)
		path.CubicTo(x+3, 50, x+7, 50, x+10, 0)
	}

	r := &GPUFlattenRasterizer{
		tolerance:   FlattenTolerance,
		flattenCtx:  NewFlattenContext(),
		initialized: true,
	}
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.flattenCPU(path, transform, 0.25)
	}
}

// BenchmarkEstimateSegmentCount benchmarks segment estimation.
func BenchmarkEstimateSegmentCount(b *testing.B) {
	path := scene.NewPath()
	path.Circle(50, 50, 40)

	r := &GPUFlattenRasterizer{
		tolerance: FlattenTolerance,
	}
	transform := scene.IdentityAffine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.EstimateSegmentCount(path, transform, 0.25)
	}
}

// BenchmarkConvertPathToGPU benchmarks path conversion.
func BenchmarkConvertPathToGPU(b *testing.B) {
	path := scene.NewPath()
	path.MoveTo(0, 0)
	for i := 0; i < 100; i++ {
		x := float32(i * 10)
		path.LineTo(x+5, 10)
		path.QuadTo(x+7, 20, x+10, 10)
	}

	r := &GPUFlattenRasterizer{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.ConvertPathToGPU(path)
	}
}

// BenchmarkComputeCursorStates benchmarks cursor state computation.
func BenchmarkComputeCursorStates(b *testing.B) {
	path := scene.NewPath()
	path.MoveTo(0, 0)
	for i := 0; i < 100; i++ {
		x := float32(i * 10)
		path.LineTo(x+5, 10)
		path.QuadTo(x+7, 20, x+10, 10)
	}

	r := &GPUFlattenRasterizer{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.ComputeCursorStates(path)
	}
}
