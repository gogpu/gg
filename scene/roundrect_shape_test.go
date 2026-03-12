package scene

import (
	"math"
	"testing"

	"github.com/gogpu/gg"
)

// ---------------------------------------------------------------------------
// RoundRectShape Constructor Tests
// ---------------------------------------------------------------------------

func TestNewRoundRectShape(t *testing.T) {
	tests := []struct {
		name   string
		rect   Rect
		rx, ry float32
		wantRX float32
		wantRY float32
	}{
		{
			name: "basic",
			rect: Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 80},
			rx:   10, ry: 10,
			wantRX: 10, wantRY: 10,
		},
		{
			name: "rx clamped to half width",
			rect: Rect{MinX: 0, MinY: 0, MaxX: 20, MaxY: 80},
			rx:   15, ry: 5,
			wantRX: 10, wantRY: 5,
		},
		{
			name: "ry clamped to half height",
			rect: Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 20},
			rx:   5, ry: 15,
			wantRX: 5, wantRY: 10,
		},
		{
			name: "both clamped to pill shape",
			rect: Rect{MinX: 10, MinY: 20, MaxX: 30, MaxY: 40},
			rx:   100, ry: 100,
			wantRX: 10, wantRY: 10,
		},
		{
			name: "negative radii clamped to zero",
			rect: Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100},
			rx:   -5, ry: -10,
			wantRX: 0, wantRY: 0,
		},
		{
			name: "zero radius is sharp rect",
			rect: Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100},
			rx:   0, ry: 0,
			wantRX: 0, wantRY: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewRoundRectShape(tt.rect, tt.rx, tt.ry)
			if s.RadiusX != tt.wantRX {
				t.Errorf("RadiusX = %v, want %v", s.RadiusX, tt.wantRX)
			}
			if s.RadiusY != tt.wantRY {
				t.Errorf("RadiusY = %v, want %v", s.RadiusY, tt.wantRY)
			}
		})
	}
}

func TestNewRoundRectShapeUniform(t *testing.T) {
	rect := Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 80}
	s := NewRoundRectShapeUniform(rect, 15)
	if s.RadiusX != 15 {
		t.Errorf("RadiusX = %v, want 15", s.RadiusX)
	}
	if s.RadiusY != 15 {
		t.Errorf("RadiusY = %v, want 15", s.RadiusY)
	}
}

// ---------------------------------------------------------------------------
// Bounds Tests
// ---------------------------------------------------------------------------

func TestRoundRectShapeBounds(t *testing.T) {
	rect := Rect{MinX: 10, MinY: 20, MaxX: 110, MaxY: 120}
	s := NewRoundRectShape(rect, 5, 5)

	bounds := s.Bounds()
	if bounds != rect {
		t.Errorf("Bounds() = %v, want %v", bounds, rect)
	}
}

// ---------------------------------------------------------------------------
// Contains Tests (SDF-based point containment)
// ---------------------------------------------------------------------------

func TestRoundRectShapeContains(t *testing.T) {
	// 100x100 rect at origin with radius 10
	rect := Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100}
	s := NewRoundRectShape(rect, 10, 10)

	tests := []struct {
		name string
		px   float32
		py   float32
		want bool
	}{
		{"center", 50, 50, true},
		{"top-left inside", 20, 20, true},
		{"top edge", 50, 1, true},
		{"left edge", 1, 50, true},
		{"outside left", -1, 50, false},
		{"outside top", 50, -1, false},
		{"outside right", 101, 50, false},
		{"outside bottom", 50, 101, false},
		// Corner region: point at (1, 1) is in the corner cutout
		// For radius=10, corner circle center is at (10, 10)
		// Distance from (1,1) to (10,10) = sqrt(81+81) = 12.73 > 10
		{"corner cutout", 1, 1, false},
		// Point at (5, 5): distance to corner center (10,10) = sqrt(25+25) = 7.07 < 10
		{"inside corner", 5, 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.Contains(tt.px, tt.py)
			if got != tt.want {
				t.Errorf("Contains(%v, %v) = %v, want %v", tt.px, tt.py, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ToPath Tests
// ---------------------------------------------------------------------------

func TestRoundRectShapeToPath(t *testing.T) {
	rect := Rect{MinX: 10, MinY: 20, MaxX: 110, MaxY: 120}
	s := NewRoundRectShape(rect, 15, 15)

	path := s.ToPath()
	if path == nil {
		t.Fatal("ToPath() returned nil")
	}
	if path.IsEmpty() {
		t.Error("ToPath() returned empty path")
	}

	// Path should have verbs (MoveTo, LineTo/CubicTo, Close)
	if len(path.Verbs()) == 0 {
		t.Error("ToPath() path has no verbs")
	}
}

// ---------------------------------------------------------------------------
// SDF Coverage Tests
// ---------------------------------------------------------------------------

func TestSdfRoundRectCoverage(t *testing.T) {
	// 100x80 rect centered at (50, 40), radius 10
	cx, cy := float32(50), float32(40)
	halfW, halfH := float32(50), float32(40)
	radius := float32(10)

	tests := []struct {
		name       string
		px, py     float32
		wantInside bool // true = coverage should be > 0.5
	}{
		{"center", 50, 40, true},
		{"well inside", 30, 30, true},
		{"just inside edge", 49.5, 0.5, true},
		{"well outside", 200, 200, false},
		{"outside corner", 0.1, 0.1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cov := sdfRoundRectCoverage(tt.px, tt.py, cx, cy, halfW, halfH, radius)
			if tt.wantInside && cov < 0.5 {
				t.Errorf("coverage = %v, expected > 0.5 for inside point", cov)
			}
			if !tt.wantInside && cov > 0.5 {
				t.Errorf("coverage = %v, expected < 0.5 for outside point", cov)
			}
		})
	}
}

func TestSmoothstepCoverage32(t *testing.T) {
	tests := []struct {
		name string
		sdf  float32
		want float32 // approximate
	}{
		{"well inside", -2.0, 1.0},
		{"well outside", 2.0, 0.0},
		{"exactly on edge", 0.0, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := smoothstepCoverage32(tt.sdf)
			if math.Abs(float64(got-tt.want)) > 0.01 {
				t.Errorf("smoothstepCoverage32(%v) = %v, want ~%v", tt.sdf, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Encoding/Decoding Roundtrip Tests
// ---------------------------------------------------------------------------

func TestRoundRectShapeEncoding(t *testing.T) {
	enc := NewEncoding()
	brush := SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1})
	rect := Rect{MinX: 10, MinY: 20, MaxX: 110, MaxY: 120}
	rx, ry := float32(15), float32(10)

	enc.EncodeFillRoundRect(brush, FillNonZero, rect, rx, ry)

	// Verify encoding state
	if enc.ShapeCount() != 1 {
		t.Errorf("ShapeCount = %d, want 1", enc.ShapeCount())
	}
	if len(enc.Tags()) != 1 || enc.Tags()[0] != TagFillRoundRect {
		t.Errorf("Tags = %v, want [TagFillRoundRect]", enc.Tags())
	}

	// Decode and verify roundtrip
	dec := NewDecoder(enc)
	if !dec.Next() {
		t.Fatal("Decoder has no commands")
	}
	if dec.Tag() != TagFillRoundRect {
		t.Errorf("Tag = %v, want TagFillRoundRect", dec.Tag())
	}

	gotBrush, gotStyle, gotRect, gotRX, gotRY := dec.FillRoundRect()
	if gotStyle != FillNonZero {
		t.Errorf("style = %v, want FillNonZero", gotStyle)
	}
	if gotRect != rect {
		t.Errorf("rect = %v, want %v", gotRect, rect)
	}
	if gotRX != rx {
		t.Errorf("rx = %v, want %v", gotRX, rx)
	}
	if gotRY != ry {
		t.Errorf("ry = %v, want %v", gotRY, ry)
	}
	if gotBrush.Color.R != 1 || gotBrush.Color.G != 0 || gotBrush.Color.B != 0 {
		t.Errorf("brush color = %v, want red", gotBrush.Color)
	}
}

func TestRoundRectShapeEncodingAppend(t *testing.T) {
	enc1 := NewEncoding()
	enc1.EncodeFillRoundRect(
		SolidBrush(gg.RGBA{R: 1, A: 1}),
		FillNonZero,
		Rect{MinX: 0, MinY: 0, MaxX: 50, MaxY: 50},
		5, 5,
	)

	enc2 := NewEncoding()
	enc2.EncodeFillRoundRect(
		SolidBrush(gg.RGBA{G: 1, A: 1}),
		FillNonZero,
		Rect{MinX: 50, MinY: 50, MaxX: 100, MaxY: 100},
		10, 10,
	)

	enc1.Append(enc2)

	if enc1.ShapeCount() != 2 {
		t.Errorf("ShapeCount after append = %d, want 2", enc1.ShapeCount())
	}
	if len(enc1.Tags()) != 2 {
		t.Errorf("tag count = %d, want 2", len(enc1.Tags()))
	}

	// Decode second command and verify brush index was adjusted
	dec := NewDecoder(enc1)
	if !dec.Next() {
		t.Fatal("no first command")
	}
	// Skip first command by reading its data
	dec.FillRoundRect()

	if !dec.Next() {
		t.Fatal("no second command")
	}
	brush2, _, _, _, _ := dec.FillRoundRect() //nolint:dogsled // testing decode of 5 return values
	if brush2.Color.G != 1 {
		t.Errorf("second brush green = %v, want 1", brush2.Color.G)
	}
}

// ---------------------------------------------------------------------------
// Scene Integration Tests
// ---------------------------------------------------------------------------

func TestSceneFillRoundRect(t *testing.T) {
	scene := NewScene()
	rect := Rect{MinX: 10, MinY: 20, MaxX: 110, MaxY: 120}
	brush := SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1})

	shape := NewRoundRectShape(rect, 15, 15)
	scene.Fill(FillNonZero, IdentityAffine(), brush, shape)

	enc := scene.Encoding()
	if enc.IsEmpty() {
		t.Error("encoding is empty after Fill with RoundRectShape")
	}
	if enc.ShapeCount() != 1 {
		t.Errorf("ShapeCount = %d, want 1", enc.ShapeCount())
	}

	// Verify it used TagFillRoundRect, not TagFill
	foundRoundRect := false
	for _, tag := range enc.Tags() {
		if tag == TagFillRoundRect {
			foundRoundRect = true
		}
		if tag == TagFill {
			t.Error("RoundRectShape should use TagFillRoundRect, not TagFill")
		}
	}
	if !foundRoundRect {
		t.Error("encoding should contain TagFillRoundRect")
	}
}

func TestSceneFillRoundRectWithTransform(t *testing.T) {
	scene := NewScene()
	rect := Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100}
	brush := SolidBrush(gg.RGBA{R: 0, G: 0, B: 1, A: 1})

	shape := NewRoundRectShape(rect, 10, 10)
	transform := TranslateAffine(50, 50)
	scene.Fill(FillNonZero, transform, brush, shape)

	enc := scene.Encoding()
	if enc.IsEmpty() {
		t.Error("encoding is empty")
	}

	// Should have a transform tag before the fill
	tags := enc.Tags()
	foundTransform := false
	foundRoundRect := false
	for _, tag := range tags {
		if tag == TagTransform {
			foundTransform = true
		}
		if tag == TagFillRoundRect {
			foundRoundRect = true
		}
	}
	if !foundTransform {
		t.Error("expected TagTransform before TagFillRoundRect")
	}
	if !foundRoundRect {
		t.Error("expected TagFillRoundRect")
	}
}

func TestSceneBuilderFillRoundRect(t *testing.T) {
	scene := NewSceneBuilder().
		FillRoundRect(10, 20, 100, 80, 15, 10, SolidBrush(gg.RGBA{R: 1, A: 1})).
		Build()

	enc := scene.Encoding()
	if enc.IsEmpty() {
		t.Error("builder scene is empty")
	}
	if enc.ShapeCount() != 1 {
		t.Errorf("ShapeCount = %d, want 1", enc.ShapeCount())
	}
}

// ---------------------------------------------------------------------------
// Tag Tests
// ---------------------------------------------------------------------------

func TestTagFillRoundRectString(t *testing.T) {
	if TagFillRoundRect.String() != "FillRoundRect" {
		t.Errorf("String() = %q, want %q", TagFillRoundRect.String(), "FillRoundRect")
	}
}

func TestTagFillRoundRectIsDrawCommand(t *testing.T) {
	if !TagFillRoundRect.IsDrawCommand() {
		t.Error("TagFillRoundRect.IsDrawCommand() should be true")
	}
}

func TestTagFillRoundRectDataSize(t *testing.T) {
	if TagFillRoundRect.DataSize() != 6 {
		t.Errorf("DataSize() = %d, want 6", TagFillRoundRect.DataSize())
	}
}

// ---------------------------------------------------------------------------
// RoundedRectShape (existing) backward compatibility
// ---------------------------------------------------------------------------

func TestRoundedRectShapeStillWorks(t *testing.T) {
	// Ensure the existing RoundedRectShape is not broken
	rrs := NewRoundedRectShape(10, 20, 100, 80, 15)
	path := rrs.ToPath()
	if path == nil || path.IsEmpty() {
		t.Error("existing RoundedRectShape.ToPath() broken")
	}
	bounds := rrs.Bounds()
	if bounds.MinX != 10 || bounds.MinY != 20 || bounds.MaxX != 110 || bounds.MaxY != 100 {
		t.Errorf("RoundedRectShape.Bounds() = %v, unexpected", bounds)
	}
}

func TestExistingRoundedRectShapeUsesPathEncoding(t *testing.T) {
	scene := NewScene()
	shape := NewRoundedRectShape(10, 20, 100, 80, 15)
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.RGBA{A: 1}), shape)

	enc := scene.Encoding()
	// RoundedRectShape (not RoundRectShape) should use path-based encoding
	for _, tag := range enc.Tags() {
		if tag == TagFillRoundRect {
			t.Error("existing RoundedRectShape should NOT use TagFillRoundRect")
		}
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkRoundRectSDF(b *testing.B) {
	scene := NewScene()
	rect := Rect{MinX: 10, MinY: 20, MaxX: 310, MaxY: 220}
	brush := SolidBrush(gg.RGBA{R: 0.2, G: 0.5, B: 0.8, A: 1})
	shape := NewRoundRectShape(rect, 20, 20)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scene.Reset()
		scene.Fill(FillNonZero, IdentityAffine(), brush, shape)
		_ = scene.Encoding()
	}
}

func BenchmarkRoundRectPath(b *testing.B) {
	scene := NewScene()
	brush := SolidBrush(gg.RGBA{R: 0.2, G: 0.5, B: 0.8, A: 1})
	shape := NewRoundedRectShape(10, 20, 300, 200, 20)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scene.Reset()
		scene.Fill(FillNonZero, IdentityAffine(), brush, shape)
		_ = scene.Encoding()
	}
}
