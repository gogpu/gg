package gg

import (
	"math"
	"testing"
)

// --- SoftwareRenderer tests ---

func TestNewSoftwareRenderer(t *testing.T) {
	r := NewSoftwareRenderer(100, 50)
	if r == nil {
		t.Fatal("NewSoftwareRenderer returned nil")
	}
	if r.width != 100 || r.height != 50 {
		t.Errorf("dimensions = %dx%d, want 100x50", r.width, r.height)
	}
	if r.deviceScale != 1.0 {
		t.Errorf("deviceScale = %f, want 1.0", r.deviceScale)
	}
}

func TestSoftwareRendererResize(t *testing.T) {
	r := NewSoftwareRenderer(100, 100)
	r.Resize(200, 150)
	if r.width != 200 || r.height != 150 {
		t.Errorf("after Resize: dimensions = %dx%d, want 200x150", r.width, r.height)
	}
}

func TestSoftwareRendererResizeWithDeviceScale(t *testing.T) {
	r := NewSoftwareRenderer(100, 100)
	r.SetDeviceScale(2.0)
	r.Resize(200, 200)
	if r.width != 200 || r.height != 200 {
		t.Errorf("after Resize: dimensions = %dx%d, want 200x200", r.width, r.height)
	}
	if r.deviceScale != 2.0 {
		t.Errorf("deviceScale = %f, want 2.0", r.deviceScale)
	}
}

func TestSoftwareRendererSetDeviceScale(t *testing.T) {
	tests := []struct {
		name      string
		scale     float32
		wantScale float32
	}{
		{"normal 1x", 1.0, 1.0},
		{"retina 2x", 2.0, 2.0},
		{"3x", 3.0, 3.0},
		{"zero clamps to 1", 0, 1.0},
		{"negative clamps to 1", -1.0, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewSoftwareRenderer(100, 100)
			r.SetDeviceScale(tt.scale)
			if r.deviceScale != tt.wantScale {
				t.Errorf("deviceScale = %f, want %f", r.deviceScale, tt.wantScale)
			}
		})
	}
}

// --- adaptiveThreshold tests ---

func TestAdaptiveThresholdExtended(t *testing.T) {
	tests := []struct {
		name    string
		area    float64
		wantMin int
		wantMax int
	}{
		{"zero area", 0, maxElementThreshold, maxElementThreshold},
		{"negative area", -100, maxElementThreshold, maxElementThreshold},
		{"small area (100)", 100, minElementThreshold, maxElementThreshold},
		{"medium area (10000)", 10000, minElementThreshold, maxElementThreshold},
		{"large area (1000000)", 1000000, minElementThreshold, minElementThreshold},
		{"exact 100x100", 10000, minElementThreshold, minElementThreshold + 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adaptiveThreshold(tt.area)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("adaptiveThreshold(%f) = %d, want in [%d, %d]", tt.area, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestAdaptiveThresholdClamping(t *testing.T) {
	// Very small area should hit maxElementThreshold
	got := adaptiveThreshold(1)
	if got != maxElementThreshold {
		t.Errorf("adaptiveThreshold(1) = %d, want %d", got, maxElementThreshold)
	}

	// Very large area should hit minElementThreshold
	got = adaptiveThreshold(1e10)
	if got != minElementThreshold {
		t.Errorf("adaptiveThreshold(1e10) = %d, want %d", got, minElementThreshold)
	}
}

// --- pathBounds tests ---

func TestPathBoundsEmpty(t *testing.T) {
	p := NewPath()
	minX, minY, maxX, maxY := pathBounds(p)
	if minX != 0 || minY != 0 || maxX != 0 || maxY != 0 {
		t.Errorf("pathBounds(empty) = (%f,%f,%f,%f), want (0,0,0,0)", minX, minY, maxX, maxY)
	}
}

func TestPathBoundsRectangle(t *testing.T) {
	p := NewPath()
	p.Rectangle(10, 20, 100, 50)
	minX, minY, maxX, maxY := pathBounds(p)
	if math.Abs(minX-10) > 1e-9 || math.Abs(minY-20) > 1e-9 ||
		math.Abs(maxX-110) > 1e-9 || math.Abs(maxY-70) > 1e-9 {
		t.Errorf("pathBounds(rect) = (%f,%f,%f,%f), want (10,20,110,70)", minX, minY, maxX, maxY)
	}
}

func TestPathBoundsCircle(t *testing.T) {
	p := NewPath()
	p.Circle(100, 100, 50)
	minX, minY, maxX, maxY := pathBounds(p)
	// Circle path uses cubic approximation, so bounds include control points
	if minX > 50+1 || minY > 50+1 || maxX < 149 || maxY < 149 {
		t.Errorf("pathBounds(circle) = (%f,%f,%f,%f), expected roughly (50,50,150,150)", minX, minY, maxX, maxY)
	}
}

func TestPathBoundsWithQuadCurve(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.QuadraticTo(50, 100, 100, 0)
	minX, minY, maxX, maxY := pathBounds(p)
	if minX > 0.1 || minY > 0.1 || maxX < 99 || maxY < 99 {
		t.Errorf("pathBounds(quad) = (%f,%f,%f,%f), expected includes control point", minX, minY, maxX, maxY)
	}
}

// --- shouldUseTileRasterizer tests ---

func TestShouldUseTileRasterizerExtended(t *testing.T) {
	tests := []struct {
		name     string
		makePath func() *Path
		want     bool
	}{
		{
			name:     "empty path",
			makePath: NewPath,
			want:     false,
		},
		{
			name: "small rectangle (below min area)",
			makePath: func() *Path {
				p := NewPath()
				p.Rectangle(0, 0, 5, 5)
				return p
			},
			want: false,
		},
		{
			name: "thin line (below min dimension)",
			makePath: func() *Path {
				p := NewPath()
				p.Rectangle(0, 0, 1000, 1)
				return p
			},
			want: false,
		},
		{
			name: "simple rectangle (not enough elements)",
			makePath: func() *Path {
				p := NewPath()
				p.Rectangle(0, 0, 100, 100)
				return p
			},
			want: false,
		},
		{
			name: "complex path with many elements",
			makePath: func() *Path {
				p := NewPath()
				// Generate a path with many segments
				p.MoveTo(0, 0)
				for i := 0; i < 300; i++ {
					angle := float64(i) * 2 * math.Pi / 300
					x := 200 + 150*math.Cos(angle)
					y := 200 + 150*math.Sin(angle)
					p.LineTo(x, y)
				}
				p.Close()
				return p
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldUseTileRasterizer(tt.makePath())
			if got != tt.want {
				t.Errorf("shouldUseTileRasterizer() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- solidColorFromPaint tests ---

func TestSolidColorFromPaintExtended(t *testing.T) {
	t.Run("solid brush", func(t *testing.T) {
		paint := NewPaint()
		paint.SetBrush(Solid(Red))
		color, ok := solidColorFromPaint(paint)
		if !ok {
			t.Fatal("expected solid color from SolidBrush")
		}
		if color.R != 1.0 || color.G != 0 || color.B != 0 {
			t.Errorf("color = %+v, want red", color)
		}
	})

	t.Run("non-solid brush", func(t *testing.T) {
		paint := NewPaint()
		grad := NewLinearGradientBrush(0, 0, 100, 0)
		grad.AddColorStop(0, Red)
		grad.AddColorStop(1, Blue)
		paint.SetBrush(grad)
		_, ok := solidColorFromPaint(paint)
		if ok {
			t.Error("expected non-solid for gradient brush")
		}
	})

	t.Run("solid pattern fallback", func(t *testing.T) {
		paint := NewPaint()
		paint.Brush = nil // clear brush so pattern is used
		paint.Pattern = NewSolidPattern(Blue)
		color, ok := solidColorFromPaint(paint)
		if !ok {
			t.Fatal("expected solid color from SolidPattern")
		}
		if color.B != 1.0 || color.R != 0 {
			t.Errorf("color = %+v, want blue", color)
		}
	})

	t.Run("nil brush and nil pattern", func(t *testing.T) {
		paint := NewPaint()
		paint.Brush = nil
		paint.Pattern = nil
		// With both nil, should return Black (default) or false
		color, ok := solidColorFromPaint(paint)
		if ok {
			// If it returned something, it should be Black
			if color.R != 0 || color.G != 0 || color.B != 0 {
				t.Errorf("expected black, got %+v", color)
			}
		}
	})
}

// --- applyClipCoverage tests ---

func TestApplyClipCoverage(t *testing.T) {
	tests := []struct {
		name     string
		clipFn   func(x, y float64) byte
		coverage uint8
		want     uint8
	}{
		{
			name:     "nil clip returns unchanged",
			clipFn:   nil,
			coverage: 200,
			want:     200,
		},
		{
			name:     "clip returns zero",
			clipFn:   func(_, _ float64) byte { return 0 },
			coverage: 200,
			want:     0,
		},
		{
			name:     "clip returns full",
			clipFn:   func(_, _ float64) byte { return 255 },
			coverage: 200,
			want:     200,
		},
		{
			name:     "clip returns half",
			clipFn:   func(_, _ float64) byte { return 128 },
			coverage: 200,
			want:     uint8(uint16(200) * 128 / 255),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyClipCoverage(tt.clipFn, 5, 5, tt.coverage)
			if got != tt.want {
				t.Errorf("applyClipCoverage() = %d, want %d", got, tt.want)
			}
		})
	}
}

// --- SoftwareRenderer Fill integration tests ---

func TestSoftwareRendererFillSimplePath(t *testing.T) {
	r := NewSoftwareRenderer(100, 100)
	pm := NewPixmap(100, 100)
	pm.Clear(White)

	p := NewPath()
	p.Rectangle(20, 20, 60, 60)

	paint := NewPaint()
	paint.SetBrush(Solid(Red))

	err := r.Fill(pm, p, paint)
	if err != nil {
		t.Fatalf("Fill returned error: %v", err)
	}

	// Check center is red
	center := pm.GetPixel(50, 50)
	if center.R < 0.9 {
		t.Errorf("center pixel R = %f, want >= 0.9", center.R)
	}

	// Check outside is white
	corner := pm.GetPixel(5, 5)
	if corner.R < 0.9 || corner.G < 0.9 || corner.B < 0.9 {
		t.Errorf("corner pixel = %+v, want white", corner)
	}
}

func TestSoftwareRendererFillEmptyPath(t *testing.T) {
	r := NewSoftwareRenderer(100, 100)
	pm := NewPixmap(100, 100)
	pm.Clear(White)

	p := NewPath()
	paint := NewPaint()
	paint.SetBrush(Solid(Red))

	err := r.Fill(pm, p, paint)
	if err != nil {
		t.Fatalf("Fill returned error: %v", err)
	}

	// Should remain white
	center := pm.GetPixel(50, 50)
	if center.R < 0.9 || center.G < 0.9 || center.B < 0.9 {
		t.Errorf("pixel should be white for empty path, got %+v", center)
	}
}

func TestSoftwareRendererFillEvenOdd(t *testing.T) {
	r := NewSoftwareRenderer(100, 100)
	pm := NewPixmap(100, 100)
	pm.Clear(White)

	// Create overlapping rectangles
	p := NewPath()
	p.Rectangle(10, 10, 80, 80)
	p.Rectangle(30, 30, 40, 40)

	paint := NewPaint()
	paint.SetBrush(Solid(Red))
	paint.FillRule = FillRuleEvenOdd

	err := r.Fill(pm, p, paint)
	if err != nil {
		t.Fatalf("Fill returned error: %v", err)
	}

	// Outer area should be red
	outer := pm.GetPixel(15, 15)
	if outer.R < 0.8 {
		t.Errorf("outer pixel R = %f, want >= 0.8 (EvenOdd outer)", outer.R)
	}

	// Inner area (hole) should be white with EvenOdd
	inner := pm.GetPixel(50, 50)
	if inner.G < 0.8 {
		t.Errorf("inner pixel G = %f, want >= 0.8 (EvenOdd hole)", inner.G)
	}
}

// --- SoftwareRenderer Stroke tests ---

func TestSoftwareRendererStroke(t *testing.T) {
	r := NewSoftwareRenderer(100, 100)
	pm := NewPixmap(100, 100)
	pm.Clear(White)

	p := NewPath()
	p.MoveTo(10, 50)
	p.LineTo(90, 50)

	paint := NewPaint()
	paint.SetBrush(Solid(Blue))
	paint.LineWidth = 4.0

	err := r.Stroke(pm, p, paint)
	if err != nil {
		t.Fatalf("Stroke returned error: %v", err)
	}

	// Check on the line
	onLine := pm.GetPixel(50, 50)
	if onLine.B < 0.5 {
		t.Errorf("on-line pixel B = %f, want >= 0.5", onLine.B)
	}
}

func TestSoftwareRendererStrokeMinWidth(t *testing.T) {
	r := NewSoftwareRenderer(100, 100)
	pm := NewPixmap(100, 100)
	pm.Clear(White)

	p := NewPath()
	p.MoveTo(10, 50)
	p.LineTo(90, 50)

	paint := NewPaint()
	paint.SetBrush(Solid(Red))
	paint.LineWidth = 0.1 // Below minimum, should be clamped to 1.0

	err := r.Stroke(pm, p, paint)
	if err != nil {
		t.Fatalf("Stroke returned error: %v", err)
	}

	// Should still render something visible
	onLine := pm.GetPixel(50, 50)
	if onLine.R < 0.3 {
		t.Errorf("thin stroke pixel R = %f, want visible stroke", onLine.R)
	}
}

func TestSoftwareRendererStrokeLineCaps(t *testing.T) {
	caps := []struct {
		name string
		cap  LineCap
	}{
		{"butt", LineCapButt},
		{"round", LineCapRound},
		{"square", LineCapSquare},
	}
	for _, tc := range caps {
		lineCap := tc.cap
		t.Run(tc.name, func(t *testing.T) {
			r := NewSoftwareRenderer(100, 100)
			pm := NewPixmap(100, 100)
			p := NewPath()
			p.MoveTo(20, 50)
			p.LineTo(80, 50)
			paint := NewPaint()
			paint.SetBrush(Solid(Black))
			paint.LineWidth = 6.0
			paint.LineCap = lineCap
			err := r.Stroke(pm, p, paint)
			if err != nil {
				t.Fatalf("Stroke with cap %v: %v", lineCap, err)
			}
		})
	}
}

func TestSoftwareRendererStrokeLineJoins(t *testing.T) {
	joinTests := []struct {
		name string
		join LineJoin
	}{
		{"miter", LineJoinMiter},
		{"round", LineJoinRound},
		{"bevel", LineJoinBevel},
	}
	for _, tc := range joinTests {
		join := tc.join
		t.Run(tc.name, func(t *testing.T) {
			r := NewSoftwareRenderer(100, 100)
			pm := NewPixmap(100, 100)
			p := NewPath()
			p.MoveTo(20, 80)
			p.LineTo(50, 20)
			p.LineTo(80, 80)
			paint := NewPaint()
			paint.SetBrush(Solid(Black))
			paint.LineWidth = 4.0
			paint.LineJoin = join
			err := r.Stroke(pm, p, paint)
			if err != nil {
				t.Fatalf("Stroke with join %v: %v", join, err)
			}
		})
	}
}

func TestSoftwareRendererStrokeWithDeviceScale(t *testing.T) {
	r := NewSoftwareRenderer(200, 200)
	r.SetDeviceScale(2.0)
	pm := NewPixmap(200, 200)

	p := NewPath()
	p.MoveTo(10, 100)
	p.LineTo(190, 100)

	paint := NewPaint()
	paint.SetBrush(Solid(Red))
	paint.LineWidth = 4.0
	paint.TransformScale = 2.0

	err := r.Stroke(pm, p, paint)
	if err != nil {
		t.Fatalf("Stroke with device scale: %v", err)
	}
}

func TestSoftwareRendererStrokeDashed(t *testing.T) {
	r := NewSoftwareRenderer(200, 100)
	pm := NewPixmap(200, 100)

	p := NewPath()
	p.MoveTo(10, 50)
	p.LineTo(190, 50)

	paint := NewPaint()
	paint.SetBrush(Solid(Black))
	paint.LineWidth = 2.0
	paint.Stroke = &Stroke{
		Width: 2.0,
		Dash:  NewDash(10, 5),
	}

	err := r.Stroke(pm, p, paint)
	if err != nil {
		t.Fatalf("Stroke with dash: %v", err)
	}
}

func TestSoftwareRendererStrokeDashScaled(t *testing.T) {
	r := NewSoftwareRenderer(200, 100)
	pm := NewPixmap(200, 100)

	p := NewPath()
	p.MoveTo(10, 50)
	p.LineTo(190, 50)

	paint := NewPaint()
	paint.SetBrush(Solid(Black))
	paint.LineWidth = 2.0
	paint.Stroke = &Stroke{
		Width: 2.0,
		Dash:  NewDash(10, 5),
	}
	paint.TransformScale = 2.0

	err := r.Stroke(pm, p, paint)
	if err != nil {
		t.Fatalf("Stroke with scaled dash: %v", err)
	}
}

// --- convertGGPathToCorePath tests ---

func TestConvertGGPathToCorePath(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.QuadraticTo(100, 50, 50, 100)
	p.CubicTo(25, 100, 0, 75, 0, 50)
	p.Close()

	adapter := convertGGPathToCorePath(p)
	if adapter.IsEmpty() {
		t.Error("converted path should not be empty")
	}
}

func TestConvertGGPathToCorePathEmpty(t *testing.T) {
	p := NewPath()
	adapter := convertGGPathToCorePath(p)
	if !adapter.IsEmpty() {
		t.Error("converted empty path should be empty")
	}
}

// --- convertLineCap / convertLineJoin tests ---

func TestConvertLineCap(t *testing.T) {
	tests := []struct {
		input LineCap
	}{
		{LineCapButt},
		{LineCapRound},
		{LineCapSquare},
		{LineCap(99)}, // unknown defaults to butt
	}
	for _, tt := range tests {
		_ = convertLineCap(tt.input) // just ensure no panic
	}
}

func TestConvertLineJoin(t *testing.T) {
	tests := []struct {
		input LineJoin
	}{
		{LineJoinMiter},
		{LineJoinRound},
		{LineJoinBevel},
		{LineJoin(99)}, // unknown defaults to miter
	}
	for _, tt := range tests {
		_ = convertLineJoin(tt.input) // just ensure no panic
	}
}

// --- blendCoverageSolid / blendCoveragePaint tests ---

func TestBlendCoverageSolidBoundsCheck(t *testing.T) {
	r := NewSoftwareRenderer(10, 10)
	pm := NewPixmap(10, 10)

	// Out of bounds should not panic
	r.blendCoverageSolid(pm, -1, 5, 255, Red)
	r.blendCoverageSolid(pm, 10, 5, 255, Red)
	r.blendCoverageSolid(pm, 5, -1, 255, Red)
	r.blendCoverageSolid(pm, 5, 10, 255, Red)
}

func TestBlendCoverageSolidFullOpaque(t *testing.T) {
	r := NewSoftwareRenderer(10, 10)
	pm := NewPixmap(10, 10)
	pm.Clear(White)

	r.blendCoverageSolid(pm, 5, 5, 255, Red)
	pixel := pm.GetPixel(5, 5)
	if pixel.R < 0.99 || pixel.G > 0.01 || pixel.B > 0.01 {
		t.Errorf("expected red, got %+v", pixel)
	}
}

func TestBlendCoverageSolidPartialCoverage(t *testing.T) {
	r := NewSoftwareRenderer(10, 10)
	pm := NewPixmap(10, 10)
	pm.Clear(White)

	r.blendCoverageSolid(pm, 5, 5, 128, Red)
	pixel := pm.GetPixel(5, 5)
	// Should be blended, not pure red or pure white
	if pixel.R < 0.5 || pixel.G > 0.9 {
		t.Errorf("expected blended, got %+v", pixel)
	}
}

func TestBlendCoveragePaintBoundsCheck(t *testing.T) {
	r := NewSoftwareRenderer(10, 10)
	pm := NewPixmap(10, 10)
	paint := NewPaint()
	paint.SetBrush(Solid(Red))

	// Out of bounds should not panic
	r.blendCoveragePaint(pm, -1, 5, 255, paint)
	r.blendCoveragePaint(pm, 10, 5, 255, paint)
	r.blendCoveragePaint(pm, 5, -1, 255, paint)
	r.blendCoveragePaint(pm, 5, 10, 255, paint)
}

func TestBlendCoveragePaintFullOpaque(t *testing.T) {
	r := NewSoftwareRenderer(10, 10)
	pm := NewPixmap(10, 10)
	pm.Clear(White)

	paint := NewPaint()
	paint.SetBrush(Solid(Blue))

	r.blendCoveragePaint(pm, 5, 5, 255, paint)
	pixel := pm.GetPixel(5, 5)
	if pixel.B < 0.99 || pixel.R > 0.01 || pixel.G > 0.01 {
		t.Errorf("expected blue, got %+v", pixel)
	}
}

// --- pointLineDistance tests ---

func TestPointLineDistanceSoftware(t *testing.T) {
	tests := []struct {
		name                   string
		px, py, x0, y0, x1, y1 float64
		want                   float64
	}{
		{"point on line", 50, 0, 0, 0, 100, 0, 0},
		{"point above line", 50, 10, 0, 0, 100, 0, 10},
		{"degenerate line (point)", 10, 0, 0, 0, 0, 0, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pointLineDistance(tt.px, tt.py, tt.x0, tt.y0, tt.x1, tt.y1)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("pointLineDistance() = %f, want %f", got, tt.want)
			}
		})
	}
}

// --- pathEndAt tests ---

func TestPathEndAt(t *testing.T) {
	p := NewPath()
	if pathEndAt(p, 0, 0) {
		t.Error("empty path should not end at any point")
	}

	p.MoveTo(10, 20)
	if !pathEndAt(p, 10, 20) {
		t.Error("path ending with MoveTo(10,20) should match")
	}

	p.LineTo(30, 40)
	if !pathEndAt(p, 30, 40) {
		t.Error("path ending with LineTo(30,40) should match")
	}
	if pathEndAt(p, 10, 20) {
		t.Error("path ending with LineTo(30,40) should not match (10,20)")
	}
}

// --- dashStateAtOffset tests ---

func TestDashStateAtOffset(t *testing.T) {
	pattern := []float64{10, 5}

	tests := []struct {
		name   string
		offset float64
		wantIn bool
	}{
		{"zero offset", 0, true},
		{"in first dash", 5, true},
		{"in first gap", 12, false},
		{"at pattern boundary", 15, true}, // wraps to 0
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, inDash := dashStateAtOffset(pattern, tt.offset)
			if inDash != tt.wantIn {
				t.Errorf("dashStateAtOffset(%f) inDash = %v, want %v", tt.offset, inDash, tt.wantIn)
			}
		})
	}
}

func TestDashStateAtOffsetEmptyPattern(t *testing.T) {
	_, _, inDash := dashStateAtOffset([]float64{}, 0)
	if !inDash {
		t.Error("empty pattern should always be in dash")
	}
}

// --- RasterizerMode forced tests ---

func TestSoftwareRendererForcedAnalytic(t *testing.T) {
	r := NewSoftwareRenderer(100, 100)
	r.rasterizerMode = RasterizerAnalytic
	pm := NewPixmap(100, 100)

	p := NewPath()
	p.Rectangle(10, 10, 80, 80)
	paint := NewPaint()
	paint.SetBrush(Solid(Red))

	err := r.Fill(pm, p, paint)
	if err != nil {
		t.Fatalf("Fill with forced Analytic: %v", err)
	}

	// Check center is filled
	center := pm.GetPixel(50, 50)
	if center.R < 0.8 {
		t.Errorf("center R = %f, want >= 0.8", center.R)
	}
}

func TestSoftwareRendererForcedSparseStrips(t *testing.T) {
	// Without a registered filler, forced SparseStrips falls back to analytic
	r := NewSoftwareRenderer(100, 100)
	r.rasterizerMode = RasterizerSparseStrips
	pm := NewPixmap(100, 100)

	p := NewPath()
	p.Rectangle(10, 10, 80, 80)
	paint := NewPaint()
	paint.SetBrush(Solid(Red))

	err := r.Fill(pm, p, paint)
	if err != nil {
		t.Fatalf("Fill with forced SparseStrips (no filler): %v", err)
	}
}

func TestSoftwareRendererForcedTileCompute(t *testing.T) {
	// Without a registered filler, forced TileCompute falls back to analytic
	r := NewSoftwareRenderer(100, 100)
	r.rasterizerMode = RasterizerTileCompute
	pm := NewPixmap(100, 100)

	p := NewPath()
	p.Rectangle(10, 10, 80, 80)
	paint := NewPaint()
	paint.SetBrush(Solid(Red))

	err := r.Fill(pm, p, paint)
	if err != nil {
		t.Fatalf("Fill with forced TileCompute (no filler): %v", err)
	}
}

// --- Flatten for dash tests ---

func TestFlattenQuadForDash(t *testing.T) {
	pts := flattenQuadForDash(0, 0, 50, 100, 100, 0, 0.5)
	if len(pts) < 4 { // At least start + one endpoint
		t.Errorf("flattenQuadForDash produced %d coords, want >= 4", len(pts))
	}
	// First two coords should be start point
	if pts[0] != 0 || pts[1] != 0 {
		t.Errorf("first point = (%f,%f), want (0,0)", pts[0], pts[1])
	}
	// Last two coords should be endpoint
	lastX, lastY := pts[len(pts)-2], pts[len(pts)-1]
	if math.Abs(lastX-100) > 0.5 || math.Abs(lastY-0) > 0.5 {
		t.Errorf("last point = (%f,%f), want ~(100,0)", lastX, lastY)
	}
}

func TestFlattenCubicForDash(t *testing.T) {
	pts := flattenCubicForDash(0, 0, 33, 100, 66, -100, 100, 0, 0.5)
	if len(pts) < 4 {
		t.Errorf("flattenCubicForDash produced %d coords, want >= 4", len(pts))
	}
	// First two coords should be start point
	if pts[0] != 0 || pts[1] != 0 {
		t.Errorf("first point = (%f,%f), want (0,0)", pts[0], pts[1])
	}
	// Last two should be endpoint
	lastX, lastY := pts[len(pts)-2], pts[len(pts)-1]
	if math.Abs(lastX-100) > 0.5 || math.Abs(lastY-0) > 0.5 {
		t.Errorf("last point = (%f,%f), want ~(100,0)", lastX, lastY)
	}
}

// --- dashPath tests ---

func TestDashPathNilDash(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)

	result := dashPath(p, nil)
	if result != p {
		t.Error("dashPath with nil dash should return original path")
	}
}

func TestDashPathWithClose(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(100, 100)
	p.Close()

	dash := NewDash(20, 10)
	result := dashPath(p, dash)
	if len(result.Elements()) == 0 {
		t.Error("dashed closed path should produce elements")
	}
}

func TestDashPathWithLongPath(t *testing.T) {
	// Test dashing with a long multi-segment path
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(50, 0)
	p.LineTo(100, 50)
	p.LineTo(150, 0)
	p.LineTo(200, 0)

	dash := NewDash(15, 5)
	result := dashPath(p, dash)
	if len(result.Elements()) == 0 {
		t.Error("dashed multi-segment path should produce elements")
	}
}
