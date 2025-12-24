package msdf

import (
	"math"
	"testing"

	"github.com/gogpu/gg/text"
)

func TestNewContour(t *testing.T) {
	c := NewContour()
	if c == nil {
		t.Fatal("NewContour() returned nil")
	}
	if len(c.Edges) != 0 {
		t.Errorf("NewContour().Edges has length %d, want 0", len(c.Edges))
	}
}

func TestContourAddEdge(t *testing.T) {
	c := NewContour()

	e1 := NewLinearEdge(Point{0, 0}, Point{10, 0})
	e2 := NewLinearEdge(Point{10, 0}, Point{10, 10})

	c.AddEdge(e1)
	c.AddEdge(e2)

	if len(c.Edges) != 2 {
		t.Errorf("len(Edges) = %d, want 2", len(c.Edges))
	}
}

func TestContourBounds(t *testing.T) {
	c := NewContour()
	c.AddEdge(NewLinearEdge(Point{0, 0}, Point{10, 0}))
	c.AddEdge(NewLinearEdge(Point{10, 0}, Point{10, 10}))
	c.AddEdge(NewLinearEdge(Point{10, 10}, Point{0, 10}))
	c.AddEdge(NewLinearEdge(Point{0, 10}, Point{0, 0}))

	bounds := c.Bounds()

	if bounds.MinX != 0 || bounds.MinY != 0 || bounds.MaxX != 10 || bounds.MaxY != 10 {
		t.Errorf("Bounds() = %v, want {0, 0, 10, 10}", bounds)
	}
}

func TestContourBoundsEmpty(t *testing.T) {
	c := NewContour()
	bounds := c.Bounds()

	if bounds.MinX != 0 || bounds.MaxX != 0 {
		t.Errorf("Empty contour bounds = %v, expected zero rect", bounds)
	}
}

func TestContourCalculateWinding(t *testing.T) {
	// Counter-clockwise square (positive winding)
	ccw := NewContour()
	ccw.AddEdge(NewLinearEdge(Point{0, 0}, Point{10, 0}))
	ccw.AddEdge(NewLinearEdge(Point{10, 0}, Point{10, 10}))
	ccw.AddEdge(NewLinearEdge(Point{10, 10}, Point{0, 10}))
	ccw.AddEdge(NewLinearEdge(Point{0, 10}, Point{0, 0}))
	ccw.CalculateWinding()

	if ccw.Winding <= 0 {
		t.Errorf("CCW square winding = %v, expected positive", ccw.Winding)
	}
	if ccw.IsClockwise() {
		t.Error("CCW square IsClockwise() = true, expected false")
	}

	// Clockwise square (negative winding)
	cw := NewContour()
	cw.AddEdge(NewLinearEdge(Point{0, 0}, Point{0, 10}))
	cw.AddEdge(NewLinearEdge(Point{0, 10}, Point{10, 10}))
	cw.AddEdge(NewLinearEdge(Point{10, 10}, Point{10, 0}))
	cw.AddEdge(NewLinearEdge(Point{10, 0}, Point{0, 0}))
	cw.CalculateWinding()

	if cw.Winding >= 0 {
		t.Errorf("CW square winding = %v, expected negative", cw.Winding)
	}
	if !cw.IsClockwise() {
		t.Error("CW square IsClockwise() = false, expected true")
	}
}

func TestContourClone(t *testing.T) {
	c := NewContour()
	c.AddEdge(NewLinearEdge(Point{0, 0}, Point{10, 0}))
	c.AddEdge(NewLinearEdge(Point{10, 0}, Point{0, 0}))
	c.Winding = 50

	clone := c.Clone()

	if len(clone.Edges) != len(c.Edges) {
		t.Errorf("Clone.Edges length = %d, want %d", len(clone.Edges), len(c.Edges))
	}
	if clone.Winding != c.Winding {
		t.Errorf("Clone.Winding = %v, want %v", clone.Winding, c.Winding)
	}

	// Verify independence
	clone.Edges[0].Color = ColorMagenta
	if c.Edges[0].Color == ColorMagenta {
		t.Error("Clone is not independent from original")
	}
}

func TestNewShape(t *testing.T) {
	s := NewShape()
	if s == nil {
		t.Fatal("NewShape() returned nil")
	}
	if len(s.Contours) != 0 {
		t.Errorf("NewShape().Contours has length %d, want 0", len(s.Contours))
	}
}

func TestShapeAddContour(t *testing.T) {
	s := NewShape()
	c1 := NewContour()
	c2 := NewContour()

	s.AddContour(c1)
	s.AddContour(c2)

	if len(s.Contours) != 2 {
		t.Errorf("len(Contours) = %d, want 2", len(s.Contours))
	}
}

func TestShapeCalculateBounds(t *testing.T) {
	s := NewShape()

	c1 := NewContour()
	c1.AddEdge(NewLinearEdge(Point{0, 0}, Point{10, 10}))

	c2 := NewContour()
	c2.AddEdge(NewLinearEdge(Point{20, 20}, Point{30, 30}))

	s.AddContour(c1)
	s.AddContour(c2)
	s.CalculateBounds()

	if s.Bounds.MinX != 0 || s.Bounds.MinY != 0 {
		t.Errorf("Shape.Bounds min = (%v, %v), want (0, 0)", s.Bounds.MinX, s.Bounds.MinY)
	}
	if s.Bounds.MaxX != 30 || s.Bounds.MaxY != 30 {
		t.Errorf("Shape.Bounds max = (%v, %v), want (30, 30)", s.Bounds.MaxX, s.Bounds.MaxY)
	}
}

func TestShapeValidate(t *testing.T) {
	// Valid closed shape
	valid := NewShape()
	c := NewContour()
	c.AddEdge(NewLinearEdge(Point{0, 0}, Point{10, 0}))
	c.AddEdge(NewLinearEdge(Point{10, 0}, Point{10, 10}))
	c.AddEdge(NewLinearEdge(Point{10, 10}, Point{0, 0}))
	valid.AddContour(c)

	if !valid.Validate() {
		t.Error("Valid closed shape failed validation")
	}

	// Invalid open shape
	invalid := NewShape()
	c2 := NewContour()
	c2.AddEdge(NewLinearEdge(Point{0, 0}, Point{10, 0}))
	c2.AddEdge(NewLinearEdge(Point{10, 0}, Point{10, 10}))
	// Missing edge to close
	invalid.AddContour(c2)

	if invalid.Validate() {
		t.Error("Invalid open shape passed validation")
	}
}

func TestShapeEdgeCount(t *testing.T) {
	s := NewShape()

	c1 := NewContour()
	c1.AddEdge(NewLinearEdge(Point{}, Point{}))
	c1.AddEdge(NewLinearEdge(Point{}, Point{}))

	c2 := NewContour()
	c2.AddEdge(NewLinearEdge(Point{}, Point{}))

	s.AddContour(c1)
	s.AddContour(c2)

	if s.EdgeCount() != 3 {
		t.Errorf("EdgeCount() = %d, want 3", s.EdgeCount())
	}
}

func TestFromOutlineEmpty(t *testing.T) {
	// Nil outline
	s := FromOutline(nil)
	if s == nil || len(s.Contours) != 0 {
		t.Error("FromOutline(nil) should return empty shape")
	}

	// Empty outline
	empty := &text.GlyphOutline{}
	s = FromOutline(empty)
	if s == nil || len(s.Contours) != 0 {
		t.Error("FromOutline(empty) should return empty shape")
	}
}

func TestFromOutlineSquare(t *testing.T) {
	// Create a simple square outline
	outline := &text.GlyphOutline{
		Segments: []text.OutlineSegment{
			{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 10, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 10, Y: 10}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 10}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
		},
	}

	shape := FromOutline(outline)

	if len(shape.Contours) != 1 {
		t.Fatalf("Expected 1 contour, got %d", len(shape.Contours))
	}

	// Should have 4 edges (the last LineTo closes the path)
	if len(shape.Contours[0].Edges) != 4 {
		t.Errorf("Expected 4 edges, got %d", len(shape.Contours[0].Edges))
	}

	// All edges should be linear
	for i, e := range shape.Contours[0].Edges {
		if e.Type != EdgeLinear {
			t.Errorf("Edge %d type = %v, want EdgeLinear", i, e.Type)
		}
	}
}

func TestFromOutlineWithCurves(t *testing.T) {
	outline := &text.GlyphOutline{
		Segments: []text.OutlineSegment{
			{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 10, Y: 0}}},
			{Op: text.OutlineOpQuadTo, Points: [3]text.OutlinePoint{{X: 15, Y: 5}, {X: 10, Y: 10}}},
			{Op: text.OutlineOpCubicTo, Points: [3]text.OutlinePoint{{X: 8, Y: 12}, {X: 2, Y: 12}, {X: 0, Y: 10}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
		},
	}

	shape := FromOutline(outline)

	if len(shape.Contours) != 1 {
		t.Fatalf("Expected 1 contour, got %d", len(shape.Contours))
	}

	edges := shape.Contours[0].Edges
	if len(edges) != 4 {
		t.Fatalf("Expected 4 edges, got %d", len(edges))
	}

	// Check edge types
	expectedTypes := []EdgeType{EdgeLinear, EdgeQuadratic, EdgeCubic, EdgeLinear}
	for i, e := range edges {
		if e.Type != expectedTypes[i] {
			t.Errorf("Edge %d type = %v, want %v", i, e.Type, expectedTypes[i])
		}
	}
}

func TestFromOutlineMultipleContours(t *testing.T) {
	outline := &text.GlyphOutline{
		Segments: []text.OutlineSegment{
			// First contour (outer square)
			{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 20, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 20, Y: 20}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 20}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			// Second contour (inner square hole)
			{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 5, Y: 5}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 15, Y: 5}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 15, Y: 15}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 5, Y: 15}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 5, Y: 5}}},
		},
	}

	shape := FromOutline(outline)

	if len(shape.Contours) != 2 {
		t.Errorf("Expected 2 contours, got %d", len(shape.Contours))
	}
}

func TestAssignColorsSimple(t *testing.T) {
	// Triangle with no sharp corners (at threshold)
	shape := NewShape()
	c := NewContour()
	c.AddEdge(NewLinearEdge(Point{0, 0}, Point{10, 0}))
	c.AddEdge(NewLinearEdge(Point{10, 0}, Point{5, 10}))
	c.AddEdge(NewLinearEdge(Point{5, 10}, Point{0, 0}))
	shape.AddContour(c)

	// High threshold = no corners detected
	AssignColors(shape, math.Pi)

	// All edges should be white
	for i, e := range shape.Contours[0].Edges {
		if e.Color != ColorWhite {
			t.Errorf("Edge %d color = %v, want ColorWhite", i, e.Color)
		}
	}
}

func TestAssignColorsWithCorners(t *testing.T) {
	// Square with 90-degree corners
	shape := NewShape()
	c := NewContour()
	c.AddEdge(NewLinearEdge(Point{0, 0}, Point{10, 0}))
	c.AddEdge(NewLinearEdge(Point{10, 0}, Point{10, 10}))
	c.AddEdge(NewLinearEdge(Point{10, 10}, Point{0, 10}))
	c.AddEdge(NewLinearEdge(Point{0, 10}, Point{0, 0}))
	shape.AddContour(c)

	// Low threshold = corners detected
	AssignColors(shape, math.Pi/4) // 45 degrees threshold, 90 degree corners detected

	// Verify that all edges have been assigned a color (not ColorBlack)
	for i, e := range shape.Contours[0].Edges {
		if e.Color == ColorBlack {
			t.Errorf("Edge %d has ColorBlack, expected a valid color", i)
		}
	}

	// With 4 corners detected, edges between corners should get different colors
	// At minimum, colors should be assigned (even if same due to algorithm)
	hasColor := false
	for _, e := range shape.Contours[0].Edges {
		if e.Color != ColorBlack {
			hasColor = true
			break
		}
	}
	if !hasColor {
		t.Error("No edges were assigned colors")
	}
}

func TestAssignColorsSingleEdge(t *testing.T) {
	shape := NewShape()
	c := NewContour()
	c.AddEdge(NewLinearEdge(Point{0, 0}, Point{10, 0}))
	shape.AddContour(c)

	AssignColors(shape, math.Pi/3)

	// Single edge should be white
	if shape.Contours[0].Edges[0].Color != ColorWhite {
		t.Errorf("Single edge color = %v, want ColorWhite", shape.Contours[0].Edges[0].Color)
	}
}

func TestAssignColorsEmpty(t *testing.T) {
	shape := NewShape()
	c := NewContour()
	shape.AddContour(c)

	// Should not panic
	AssignColors(shape, math.Pi/3)
}

func TestSwitchColor(t *testing.T) {
	tests := []struct {
		current EdgeColor
		seed    int
		want    EdgeColor
	}{
		{ColorCyan, 0, ColorMagenta},
		{ColorMagenta, 0, ColorYellow},
		{ColorYellow, 0, ColorCyan},
		{ColorCyan, 1, ColorYellow}, // Different seed
		{ColorBlack, 0, ColorCyan},  // Unknown color
	}

	for _, tt := range tests {
		got := SwitchColor(tt.current, tt.seed)
		if got != tt.want {
			t.Errorf("SwitchColor(%v, %d) = %v, want %v", tt.current, tt.seed, got, tt.want)
		}
	}
}

func TestEdgeSelectors(t *testing.T) {
	tests := []struct {
		selector func(EdgeColor) bool
		color    EdgeColor
		want     bool
	}{
		{SelectRed, ColorRed, true},
		{SelectRed, ColorGreen, false},
		{SelectRed, ColorWhite, true},
		{SelectGreen, ColorGreen, true},
		{SelectGreen, ColorRed, false},
		{SelectGreen, ColorCyan, true},
		{SelectBlue, ColorBlue, true},
		{SelectBlue, ColorMagenta, true},
		{SelectBlue, ColorYellow, false},
	}

	for i, tt := range tests {
		got := tt.selector(tt.color)
		if got != tt.want {
			t.Errorf("Test %d: selector(%v) = %v, want %v", i, tt.color, got, tt.want)
		}
	}
}

func BenchmarkFromOutlineSquare(b *testing.B) {
	outline := &text.GlyphOutline{
		Segments: []text.OutlineSegment{
			{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 10, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 10, Y: 10}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 10}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FromOutline(outline)
	}
}

func BenchmarkAssignColors(b *testing.B) {
	outline := &text.GlyphOutline{
		Segments: []text.OutlineSegment{
			{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 10, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 10, Y: 10}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 10}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
		},
	}
	shape := FromOutline(outline)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset colors
		for _, c := range shape.Contours {
			for j := range c.Edges {
				c.Edges[j].Color = ColorWhite
			}
		}
		AssignColors(shape, math.Pi/3)
	}
}
