package recording

import (
	"math"
	"testing"

	"github.com/gogpu/gg"
)

func TestSolidBrush(t *testing.T) {
	tests := []struct {
		name  string
		color gg.RGBA
	}{
		{"red", gg.Red},
		{"blue", gg.Blue},
		{"transparent", gg.Transparent},
		{"semi-transparent", gg.RGBA2(1, 0, 0, 0.5)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			brush := NewSolidBrush(tt.color)

			if brush.Color != tt.color {
				t.Errorf("Color = %v, want %v", brush.Color, tt.color)
			}

			// Verify it implements Brush interface
			var _ Brush = brush
		})
	}
}

func TestLinearGradientBrush(t *testing.T) {
	brush := NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, gg.Red).
		AddColorStop(0.5, gg.Yellow).
		AddColorStop(1, gg.Blue).
		SetExtend(ExtendRepeat)

	// Verify start and end points
	if brush.Start.X != 0 || brush.Start.Y != 0 {
		t.Errorf("Start = %v, want (0, 0)", brush.Start)
	}
	if brush.End.X != 100 || brush.End.Y != 0 {
		t.Errorf("End = %v, want (100, 0)", brush.End)
	}

	// Verify stops
	if len(brush.Stops) != 3 {
		t.Errorf("len(Stops) = %d, want 3", len(brush.Stops))
	}

	expectedStops := []struct {
		offset float64
		color  gg.RGBA
	}{
		{0, gg.Red},
		{0.5, gg.Yellow},
		{1, gg.Blue},
	}

	for i, expected := range expectedStops {
		if brush.Stops[i].Offset != expected.offset {
			t.Errorf("Stops[%d].Offset = %f, want %f", i, brush.Stops[i].Offset, expected.offset)
		}
		if brush.Stops[i].Color != expected.color {
			t.Errorf("Stops[%d].Color = %v, want %v", i, brush.Stops[i].Color, expected.color)
		}
	}

	// Verify extend mode
	if brush.Extend != ExtendRepeat {
		t.Errorf("Extend = %v, want ExtendRepeat", brush.Extend)
	}

	// Verify it implements Brush interface
	var _ Brush = brush
}

func TestRadialGradientBrush(t *testing.T) {
	brush := NewRadialGradientBrush(50, 50, 10, 50).
		SetFocus(30, 30).
		AddColorStop(0, gg.White).
		AddColorStop(1, gg.Black).
		SetExtend(ExtendReflect)

	// Verify center
	if brush.Center.X != 50 || brush.Center.Y != 50 {
		t.Errorf("Center = %v, want (50, 50)", brush.Center)
	}

	// Verify focus
	if brush.Focus.X != 30 || brush.Focus.Y != 30 {
		t.Errorf("Focus = %v, want (30, 30)", brush.Focus)
	}

	// Verify radii
	if brush.StartRadius != 10 {
		t.Errorf("StartRadius = %f, want 10", brush.StartRadius)
	}
	if brush.EndRadius != 50 {
		t.Errorf("EndRadius = %f, want 50", brush.EndRadius)
	}

	// Verify stops
	if len(brush.Stops) != 2 {
		t.Errorf("len(Stops) = %d, want 2", len(brush.Stops))
	}

	// Verify extend mode
	if brush.Extend != ExtendReflect {
		t.Errorf("Extend = %v, want ExtendReflect", brush.Extend)
	}

	// Verify it implements Brush interface
	var _ Brush = brush
}

func TestRadialGradientBrush_DefaultFocus(t *testing.T) {
	brush := NewRadialGradientBrush(50, 50, 0, 50)

	// Focus should default to center
	if brush.Focus != brush.Center {
		t.Errorf("Focus = %v, want %v (center)", brush.Focus, brush.Center)
	}
}

func TestSweepGradientBrush(t *testing.T) {
	startAngle := math.Pi / 4 // 45 degrees
	brush := NewSweepGradientBrush(50, 50, startAngle).
		SetEndAngle(startAngle+math.Pi). // 180 degrees sweep
		AddColorStop(0, gg.Red).
		AddColorStop(1, gg.Blue).
		SetExtend(ExtendPad)

	// Verify center
	if brush.Center.X != 50 || brush.Center.Y != 50 {
		t.Errorf("Center = %v, want (50, 50)", brush.Center)
	}

	// Verify angles
	if brush.StartAngle != startAngle {
		t.Errorf("StartAngle = %f, want %f", brush.StartAngle, startAngle)
	}

	expectedEndAngle := startAngle + math.Pi
	if math.Abs(brush.EndAngle-expectedEndAngle) > 1e-10 {
		t.Errorf("EndAngle = %f, want %f", brush.EndAngle, expectedEndAngle)
	}

	// Verify stops
	if len(brush.Stops) != 2 {
		t.Errorf("len(Stops) = %d, want 2", len(brush.Stops))
	}

	// Verify extend mode
	if brush.Extend != ExtendPad {
		t.Errorf("Extend = %v, want ExtendPad", brush.Extend)
	}

	// Verify it implements Brush interface
	var _ Brush = brush
}

func TestSweepGradientBrush_DefaultEndAngle(t *testing.T) {
	brush := NewSweepGradientBrush(50, 50, 0)

	// Default should be full rotation (2*Pi)
	expectedEndAngle := 2 * math.Pi
	if math.Abs(brush.EndAngle-expectedEndAngle) > 1e-10 {
		t.Errorf("EndAngle = %f, want %f", brush.EndAngle, expectedEndAngle)
	}
}

func TestPatternBrush(t *testing.T) {
	imageRef := ImageRef(5)
	transform := gg.Translate(10, 20).Multiply(gg.Scale(2, 2))

	brush := NewPatternBrush(imageRef).
		SetRepeat(RepeatX).
		SetTransform(transform)

	// Verify image reference
	if brush.Image != imageRef {
		t.Errorf("Image = %d, want %d", brush.Image, imageRef)
	}

	// Verify repeat mode
	if brush.Repeat != RepeatX {
		t.Errorf("Repeat = %v, want RepeatX", brush.Repeat)
	}

	// Verify transform
	if brush.Transform != transform {
		t.Errorf("Transform = %v, want %v", brush.Transform, transform)
	}

	// Verify it implements Brush interface
	var _ Brush = brush
}

func TestPatternBrush_Defaults(t *testing.T) {
	brush := NewPatternBrush(ImageRef(0))

	// Verify defaults
	if brush.Repeat != RepeatBoth {
		t.Errorf("Repeat = %v, want RepeatBoth", brush.Repeat)
	}

	if !brush.Transform.IsIdentity() {
		t.Errorf("Transform = %v, want identity", brush.Transform)
	}
}

func TestExtendMode(t *testing.T) {
	tests := []struct {
		mode ExtendMode
		want int
	}{
		{ExtendPad, 0},
		{ExtendRepeat, 1},
		{ExtendReflect, 2},
	}

	for _, tt := range tests {
		if int(tt.mode) != tt.want {
			t.Errorf("ExtendMode %v = %d, want %d", tt.mode, int(tt.mode), tt.want)
		}
	}
}

func TestRepeatMode(t *testing.T) {
	tests := []struct {
		mode RepeatMode
		want int
	}{
		{RepeatBoth, 0},
		{RepeatX, 1},
		{RepeatY, 2},
		{RepeatNone, 3},
	}

	for _, tt := range tests {
		if int(tt.mode) != tt.want {
			t.Errorf("RepeatMode %v = %d, want %d", tt.mode, int(tt.mode), tt.want)
		}
	}
}

func TestBrushFromGG_Solid(t *testing.T) {
	ggBrush := gg.Solid(gg.Red)
	brush := BrushFromGG(ggBrush)

	solid, ok := brush.(SolidBrush)
	if !ok {
		t.Fatalf("BrushFromGG(SolidBrush) returned %T, want SolidBrush", brush)
	}

	if solid.Color != gg.Red {
		t.Errorf("Color = %v, want %v", solid.Color, gg.Red)
	}
}

func TestBrushFromGG_LinearGradient(t *testing.T) {
	ggBrush := gg.NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, gg.Red).
		AddColorStop(1, gg.Blue).
		SetExtend(gg.ExtendRepeat)

	brush := BrushFromGG(ggBrush)

	linear, ok := brush.(*LinearGradientBrush)
	if !ok {
		t.Fatalf("BrushFromGG(LinearGradientBrush) returned %T, want *LinearGradientBrush", brush)
	}

	// Verify conversion
	if linear.Start.X != 0 || linear.Start.Y != 0 {
		t.Errorf("Start = %v, want (0, 0)", linear.Start)
	}
	if linear.End.X != 100 || linear.End.Y != 0 {
		t.Errorf("End = %v, want (100, 0)", linear.End)
	}
	if len(linear.Stops) != 2 {
		t.Errorf("len(Stops) = %d, want 2", len(linear.Stops))
	}
	if linear.Extend != ExtendRepeat {
		t.Errorf("Extend = %v, want ExtendRepeat", linear.Extend)
	}
}

func TestBrushFromGG_RadialGradient(t *testing.T) {
	ggBrush := gg.NewRadialGradientBrush(50, 50, 10, 50).
		SetFocus(30, 30).
		AddColorStop(0, gg.White).
		AddColorStop(1, gg.Black)

	brush := BrushFromGG(ggBrush)

	radial, ok := brush.(*RadialGradientBrush)
	if !ok {
		t.Fatalf("BrushFromGG(RadialGradientBrush) returned %T, want *RadialGradientBrush", brush)
	}

	// Verify conversion
	if radial.Center.X != 50 || radial.Center.Y != 50 {
		t.Errorf("Center = %v, want (50, 50)", radial.Center)
	}
	if radial.Focus.X != 30 || radial.Focus.Y != 30 {
		t.Errorf("Focus = %v, want (30, 30)", radial.Focus)
	}
	if radial.StartRadius != 10 {
		t.Errorf("StartRadius = %f, want 10", radial.StartRadius)
	}
	if radial.EndRadius != 50 {
		t.Errorf("EndRadius = %f, want 50", radial.EndRadius)
	}
	if len(radial.Stops) != 2 {
		t.Errorf("len(Stops) = %d, want 2", len(radial.Stops))
	}
}

func TestBrushFromGG_SweepGradient(t *testing.T) {
	ggBrush := gg.NewSweepGradientBrush(50, 50, math.Pi/4).
		AddColorStop(0, gg.Red).
		AddColorStop(1, gg.Blue)

	brush := BrushFromGG(ggBrush)

	sweep, ok := brush.(*SweepGradientBrush)
	if !ok {
		t.Fatalf("BrushFromGG(SweepGradientBrush) returned %T, want *SweepGradientBrush", brush)
	}

	// Verify conversion
	if sweep.Center.X != 50 || sweep.Center.Y != 50 {
		t.Errorf("Center = %v, want (50, 50)", sweep.Center)
	}
	if sweep.StartAngle != math.Pi/4 {
		t.Errorf("StartAngle = %f, want %f", sweep.StartAngle, math.Pi/4)
	}
	if len(sweep.Stops) != 2 {
		t.Errorf("len(Stops) = %d, want 2", len(sweep.Stops))
	}
}

func TestBrushFromGG_CustomBrush(t *testing.T) {
	// Create a custom brush via gg.CustomBrush
	// Note: gg.CustomBrush implements gg.Brush interface
	custom := gg.CustomBrush{
		Func: func(x, y float64) gg.RGBA {
			return gg.Blue
		},
		Name: "test-custom",
	}

	brush := BrushFromGG(custom)

	// Should fallback to SolidBrush with color sampled at origin
	solid, ok := brush.(SolidBrush)
	if !ok {
		t.Fatalf("BrushFromGG(CustomBrush) returned %T, want SolidBrush", brush)
	}

	if solid.Color != gg.Blue {
		t.Errorf("Color = %v, want %v (sampled at origin)", solid.Color, gg.Blue)
	}
}

func TestGradientStop(t *testing.T) {
	stop := GradientStop{
		Offset: 0.5,
		Color:  gg.Red,
	}

	if stop.Offset != 0.5 {
		t.Errorf("Offset = %f, want 0.5", stop.Offset)
	}
	if stop.Color != gg.Red {
		t.Errorf("Color = %v, want %v", stop.Color, gg.Red)
	}
}

// BenchmarkBrushFromGG benchmarks brush conversion.
func BenchmarkBrushFromGG_Solid(b *testing.B) {
	brush := gg.Solid(gg.Red)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BrushFromGG(brush)
	}
}

func BenchmarkBrushFromGG_LinearGradient(b *testing.B) {
	brush := gg.NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, gg.Red).
		AddColorStop(0.5, gg.Yellow).
		AddColorStop(1, gg.Blue)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BrushFromGG(brush)
	}
}
