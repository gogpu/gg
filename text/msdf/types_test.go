package msdf

import (
	"math"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Size != 32 {
		t.Errorf("DefaultConfig().Size = %d, want 32", config.Size)
	}
	if config.Range != 4.0 {
		t.Errorf("DefaultConfig().Range = %v, want 4.0", config.Range)
	}
	if config.AngleThreshold != math.Pi/3 {
		t.Errorf("DefaultConfig().AngleThreshold = %v, want %v", config.AngleThreshold, math.Pi/3)
	}
	if config.EdgeThreshold != 1.001 {
		t.Errorf("DefaultConfig().EdgeThreshold = %v, want 1.001", config.EdgeThreshold)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "default config is valid",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name:    "size too small",
			config:  Config{Size: 4, Range: 4.0, AngleThreshold: 1.0, EdgeThreshold: 1.001},
			wantErr: true,
		},
		{
			name:    "size too large",
			config:  Config{Size: 5000, Range: 4.0, AngleThreshold: 1.0, EdgeThreshold: 1.001},
			wantErr: true,
		},
		{
			name:    "range zero",
			config:  Config{Size: 32, Range: 0, AngleThreshold: 1.0, EdgeThreshold: 1.001},
			wantErr: true,
		},
		{
			name:    "range negative",
			config:  Config{Size: 32, Range: -1.0, AngleThreshold: 1.0, EdgeThreshold: 1.001},
			wantErr: true,
		},
		{
			name:    "angle threshold zero",
			config:  Config{Size: 32, Range: 4.0, AngleThreshold: 0, EdgeThreshold: 1.001},
			wantErr: true,
		},
		{
			name:    "angle threshold too large",
			config:  Config{Size: 32, Range: 4.0, AngleThreshold: 4.0, EdgeThreshold: 1.001},
			wantErr: true,
		},
		{
			name:    "edge threshold too small",
			config:  Config{Size: 32, Range: 4.0, AngleThreshold: 1.0, EdgeThreshold: 0.5},
			wantErr: true,
		},
		{
			name:    "valid custom config",
			config:  Config{Size: 64, Range: 8.0, AngleThreshold: math.Pi/4, EdgeThreshold: 1.5},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMSDFPixelOperations(t *testing.T) {
	msdf := &MSDF{
		Data:   make([]byte, 32*32*3),
		Width:  32,
		Height: 32,
	}

	// Test SetPixel and GetPixel
	testCases := []struct {
		x, y    int
		r, g, b byte
	}{
		{0, 0, 255, 0, 0},
		{31, 31, 0, 255, 0},
		{15, 15, 0, 0, 255},
		{10, 20, 128, 64, 32},
	}

	for _, tc := range testCases {
		msdf.SetPixel(tc.x, tc.y, tc.r, tc.g, tc.b)
		r, g, b := msdf.GetPixel(tc.x, tc.y)

		if r != tc.r || g != tc.g || b != tc.b {
			t.Errorf("GetPixel(%d, %d) = (%d, %d, %d), want (%d, %d, %d)",
				tc.x, tc.y, r, g, b, tc.r, tc.g, tc.b)
		}
	}
}

func TestMSDFPixelOffset(t *testing.T) {
	msdf := &MSDF{
		Width:  32,
		Height: 32,
	}

	tests := []struct {
		x, y int
		want int
	}{
		{0, 0, 0},
		{1, 0, 3},
		{0, 1, 96},  // 32 * 3
		{1, 1, 99},  // 32 * 3 + 3
		{31, 31, 3069}, // (31*32 + 31) * 3
	}

	for _, tt := range tests {
		got := msdf.PixelOffset(tt.x, tt.y)
		if got != tt.want {
			t.Errorf("PixelOffset(%d, %d) = %d, want %d", tt.x, tt.y, got, tt.want)
		}
	}
}

func TestMSDFCoordinateConversion(t *testing.T) {
	msdf := &MSDF{
		Width:      32,
		Height:     32,
		Bounds:     Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100},
		Scale:      0.28, // roughly 32 / (100 + 2*padding)
		TranslateX: 4.0,
		TranslateY: 4.0,
	}

	// Test OutlineToPixel
	px, py := msdf.OutlineToPixel(0, 0)
	if px != 4.0 || py != 4.0 {
		t.Errorf("OutlineToPixel(0, 0) = (%v, %v), want (4, 4)", px, py)
	}

	// Test PixelToOutline (inverse)
	ox, oy := msdf.PixelToOutline(4.0, 4.0)
	if math.Abs(ox) > 0.01 || math.Abs(oy) > 0.01 {
		t.Errorf("PixelToOutline(4, 4) = (%v, %v), want (0, 0)", ox, oy)
	}
}

func TestPointOperations(t *testing.T) {
	p := Point{X: 3, Y: 4}
	q := Point{X: 1, Y: 2}

	// Add
	sum := p.Add(q)
	if sum.X != 4 || sum.Y != 6 {
		t.Errorf("Point.Add = %v, want {4, 6}", sum)
	}

	// Sub
	diff := p.Sub(q)
	if diff.X != 2 || diff.Y != 2 {
		t.Errorf("Point.Sub = %v, want {2, 2}", diff)
	}

	// Mul
	scaled := p.Mul(2)
	if scaled.X != 6 || scaled.Y != 8 {
		t.Errorf("Point.Mul = %v, want {6, 8}", scaled)
	}

	// Dot
	dot := p.Dot(q)
	if dot != 11 { // 3*1 + 4*2
		t.Errorf("Point.Dot = %v, want 11", dot)
	}

	// Cross
	cross := p.Cross(q)
	if cross != 2 { // 3*2 - 4*1
		t.Errorf("Point.Cross = %v, want 2", cross)
	}

	// Length
	length := p.Length()
	if math.Abs(length-5) > 1e-10 { // 3-4-5 triangle
		t.Errorf("Point.Length = %v, want 5", length)
	}

	// LengthSquared
	lenSq := p.LengthSquared()
	if lenSq != 25 {
		t.Errorf("Point.LengthSquared = %v, want 25", lenSq)
	}

	// Normalized
	norm := p.Normalized()
	if math.Abs(norm.X-0.6) > 1e-10 || math.Abs(norm.Y-0.8) > 1e-10 {
		t.Errorf("Point.Normalized = %v, want {0.6, 0.8}", norm)
	}

	// Zero vector normalization
	zero := Point{}
	zeroNorm := zero.Normalized()
	if zeroNorm.X != 0 || zeroNorm.Y != 0 {
		t.Errorf("Zero.Normalized = %v, want {0, 0}", zeroNorm)
	}

	// Perpendicular
	perp := p.Perpendicular()
	if perp.X != -4 || perp.Y != 3 {
		t.Errorf("Point.Perpendicular = %v, want {-4, 3}", perp)
	}

	// Lerp
	lerp := p.Lerp(q, 0.5)
	if lerp.X != 2 || lerp.Y != 3 {
		t.Errorf("Point.Lerp(0.5) = %v, want {2, 3}", lerp)
	}

	// Angle
	angle := Point{X: 1, Y: 0}.Angle()
	if math.Abs(angle) > 1e-10 {
		t.Errorf("Point{1,0}.Angle = %v, want 0", angle)
	}
}

func TestAngleBetween(t *testing.T) {
	tests := []struct {
		a, b Point
		want float64
	}{
		{Point{1, 0}, Point{1, 0}, 0},         // Same direction
		{Point{1, 0}, Point{0, 1}, math.Pi / 2}, // 90 degrees
		{Point{1, 0}, Point{-1, 0}, math.Pi},    // 180 degrees
		{Point{1, 0}, Point{0, 0}, 0},           // Zero vector
	}

	for _, tt := range tests {
		got := AngleBetween(tt.a, tt.b)
		if math.Abs(got-tt.want) > 1e-10 {
			t.Errorf("AngleBetween(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestRectOperations(t *testing.T) {
	r := Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 50}

	// Width
	if r.Width() != 100 {
		t.Errorf("Rect.Width() = %v, want 100", r.Width())
	}

	// Height
	if r.Height() != 50 {
		t.Errorf("Rect.Height() = %v, want 50", r.Height())
	}

	// IsEmpty
	if r.IsEmpty() {
		t.Error("Rect.IsEmpty() = true for non-empty rect")
	}

	empty := Rect{MinX: 10, MinY: 10, MaxX: 5, MaxY: 5}
	if !empty.IsEmpty() {
		t.Error("Rect.IsEmpty() = false for empty rect")
	}

	// Center
	center := r.Center()
	if center.X != 50 || center.Y != 25 {
		t.Errorf("Rect.Center() = %v, want {50, 25}", center)
	}

	// Contains
	if !r.Contains(Point{50, 25}) {
		t.Error("Rect.Contains(center) = false")
	}
	if r.Contains(Point{-1, 25}) {
		t.Error("Rect.Contains(outside) = true")
	}

	// Expand
	expanded := r.Expand(10)
	if expanded.MinX != -10 || expanded.MinY != -10 || expanded.MaxX != 110 || expanded.MaxY != 60 {
		t.Errorf("Rect.Expand(10) = %v, unexpected", expanded)
	}

	// Union
	s := Rect{MinX: 50, MinY: 25, MaxX: 150, MaxY: 75}
	union := r.Union(s)
	if union.MinX != 0 || union.MinY != 0 || union.MaxX != 150 || union.MaxY != 75 {
		t.Errorf("Rect.Union = %v, unexpected", union)
	}
}

func TestSignedDistance(t *testing.T) {
	// Test Infinite
	inf := Infinite()
	if inf.Distance != math.MaxFloat64 {
		t.Errorf("Infinite().Distance = %v, want MaxFloat64", inf.Distance)
	}

	// Test IsCloserThan
	sd1 := NewSignedDistance(1.0, 0)
	sd2 := NewSignedDistance(2.0, 0)
	if !sd1.IsCloserThan(sd2) {
		t.Error("1.0 should be closer than 2.0")
	}
	if sd2.IsCloserThan(sd1) {
		t.Error("2.0 should not be closer than 1.0")
	}

	// Test with equal distance, different dot
	sd3 := NewSignedDistance(1.0, 0.5)
	sd4 := NewSignedDistance(1.0, 0.8)
	if !sd3.IsCloserThan(sd4) {
		t.Error("same distance with lower dot should be closer")
	}

	// Test Combine
	combined := sd1.Combine(sd2)
	if combined.Distance != 1.0 {
		t.Errorf("Combine should return closer distance, got %v", combined.Distance)
	}

	// Test negative distances - both have same absolute distance
	neg := NewSignedDistance(-1.0, 0)
	pos := NewSignedDistance(1.0, 0)
	// With equal absolute distance and equal dot, neither is closer
	// The IsCloserThan comparison uses absolute distance first
	// So -1 and +1 should compare based on dot (both 0, so neither is strictly closer)
	if neg.IsCloserThan(pos) && pos.IsCloserThan(neg) {
		t.Error("Equal distances with equal dot should not both be closer")
	}
}

func TestConfigError(t *testing.T) {
	err := &ConfigError{Field: "Size", Reason: "must be positive"}
	expected := "msdf: invalid config.Size: must be positive"
	if err.Error() != expected {
		t.Errorf("ConfigError.Error() = %q, want %q", err.Error(), expected)
	}
}
