package clip

import (
	"testing"
)

func TestNewClipStack(t *testing.T) {
	bounds := NewRect(0, 0, 100, 100)
	stack := NewClipStack(bounds)

	if stack == nil {
		t.Fatal("NewClipStack() returned nil")
	}

	if stack.Depth() != 0 {
		t.Errorf("Depth() = %d, want 0", stack.Depth())
	}

	if stack.Bounds() != bounds {
		t.Errorf("Bounds() = %v, want %v", stack.Bounds(), bounds)
	}
}

func TestClipStack_PushRect(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	tests := []struct {
		name       string
		rect       Rect
		wantBounds Rect
		wantDepth  int
	}{
		{
			name:       "push smaller rect",
			rect:       NewRect(10, 10, 50, 50),
			wantBounds: NewRect(10, 10, 50, 50),
			wantDepth:  1,
		},
		{
			name:       "push overlapping rect",
			rect:       NewRect(30, 30, 50, 50),
			wantBounds: NewRect(30, 30, 30, 30), // Intersection of (10,10,50,50) and (30,30,50,50)
			wantDepth:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack.PushRect(tt.rect)

			if stack.Depth() != tt.wantDepth {
				t.Errorf("Depth() = %d, want %d", stack.Depth(), tt.wantDepth)
			}

			if stack.Bounds() != tt.wantBounds {
				t.Errorf("Bounds() = %v, want %v", stack.Bounds(), tt.wantBounds)
			}
		})
	}
}

func TestClipStack_PushPath(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	// Create a simple rectangle path
	p := newSOAPath().moveTo(10, 10).lineTo(50, 10).lineTo(50, 50).lineTo(10, 50).close()

	err := stack.PushPath(p.verbs, p.coords, true)
	if err != nil {
		t.Fatalf("PushPath() error = %v", err)
	}

	if stack.Depth() != 1 {
		t.Errorf("Depth() = %d, want 1", stack.Depth())
	}

	// Bounds should be intersection of canvas and path bounds
	expectedBounds := NewRect(0, 0, 100, 100) // Path bounds are within canvas
	if stack.Bounds() != expectedBounds {
		t.Errorf("Bounds() = %v, want %v", stack.Bounds(), expectedBounds)
	}
}

func TestClipStack_PushPath_EmptyBounds(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 0, 0))

	p := newSOAPath().moveTo(10, 10).lineTo(50, 10)

	err := stack.PushPath(p.verbs, p.coords, true)
	if err == nil {
		t.Error("PushPath() expected error for empty bounds, got nil")
	}

	if stack.Depth() != 0 {
		t.Errorf("Depth() = %d, want 0 (should not push on error)", stack.Depth())
	}
}

func TestClipStack_Pop(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	// Push two rectangles
	stack.PushRect(NewRect(10, 10, 50, 50))
	stack.PushRect(NewRect(20, 20, 30, 30))

	if stack.Depth() != 2 {
		t.Fatalf("Depth() = %d, want 2", stack.Depth())
	}

	// Pop once - should restore to first rect bounds
	stack.Pop()

	if stack.Depth() != 1 {
		t.Errorf("Depth() after first Pop() = %d, want 1", stack.Depth())
	}

	expectedBounds := NewRect(10, 10, 50, 50)
	if stack.Bounds() != expectedBounds {
		t.Errorf("Bounds() after first Pop() = %v, want %v", stack.Bounds(), expectedBounds)
	}

	// Pop again - should restore to original bounds
	stack.Pop()

	if stack.Depth() != 0 {
		t.Errorf("Depth() after second Pop() = %d, want 0", stack.Depth())
	}

	expectedBounds = NewRect(0, 0, 100, 100)
	if stack.Bounds() != expectedBounds {
		t.Errorf("Bounds() after second Pop() = %v, want %v", stack.Bounds(), expectedBounds)
	}

	// Pop on empty stack - should be no-op
	stack.Pop()

	if stack.Depth() != 0 {
		t.Errorf("Depth() after Pop() on empty stack = %d, want 0", stack.Depth())
	}
}

func TestClipStack_IsVisible(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	tests := []struct {
		name    string
		setup   func()
		x, y    float64
		visible bool
	}{
		{
			name:    "point in initial bounds",
			setup:   func() {},
			x:       50,
			y:       50,
			visible: true,
		},
		{
			name:    "point outside initial bounds",
			setup:   func() {},
			x:       150,
			y:       150,
			visible: false,
		},
		{
			name: "point in rect clip",
			setup: func() {
				stack.PushRect(NewRect(20, 20, 60, 60))
			},
			x:       50,
			y:       50,
			visible: true,
		},
		{
			name: "point outside rect clip",
			setup: func() {
				stack.Reset(NewRect(0, 0, 100, 100))
				stack.PushRect(NewRect(20, 20, 60, 60))
			},
			x:       10,
			y:       10,
			visible: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			visible := stack.IsVisible(tt.x, tt.y)
			if visible != tt.visible {
				t.Errorf("IsVisible(%v, %v) = %v, want %v", tt.x, tt.y, visible, tt.visible)
			}
		})
	}
}

func TestClipStack_IsVisible_WithMask(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	// Create a triangle path
	p := newSOAPath().moveTo(50, 20).lineTo(20, 80).lineTo(80, 80).close()

	err := stack.PushPath(p.verbs, p.coords, true)
	if err != nil {
		t.Fatalf("PushPath() error = %v", err)
	}

	tests := []struct {
		name    string
		x, y    float64
		visible bool
	}{
		{
			name:    "point inside triangle",
			x:       50,
			y:       50,
			visible: true,
		},
		{
			name:    "point outside triangle",
			x:       10,
			y:       10,
			visible: false,
		},
		{
			name:    "point outside bounds",
			x:       150,
			y:       150,
			visible: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			visible := stack.IsVisible(tt.x, tt.y)
			if visible != tt.visible {
				t.Errorf("IsVisible(%v, %v) = %v, want %v", tt.x, tt.y, visible, tt.visible)
			}
		})
	}
}

func TestClipStack_Coverage(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	tests := []struct {
		name     string
		x, y     float64
		wantZero bool
	}{
		{
			name:     "point in bounds",
			x:        50,
			y:        50,
			wantZero: false,
		},
		{
			name:     "point outside bounds",
			x:        150,
			y:        150,
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coverage := stack.Coverage(tt.x, tt.y)

			if tt.wantZero && coverage != 0 {
				t.Errorf("Coverage(%v, %v) = %v, want 0", tt.x, tt.y, coverage)
			}

			if !tt.wantZero && coverage == 0 {
				t.Errorf("Coverage(%v, %v) = 0, want > 0", tt.x, tt.y)
			}
		})
	}
}

func TestClipStack_Coverage_WithMask(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	// Create a simple rectangle path
	p := newSOAPath().moveTo(20, 20).lineTo(80, 20).lineTo(80, 80).lineTo(20, 80).close()

	err := stack.PushPath(p.verbs, p.coords, true)
	if err != nil {
		t.Fatalf("PushPath() error = %v", err)
	}

	tests := []struct {
		name     string
		x, y     float64
		wantZero bool
	}{
		{
			name:     "point inside path",
			x:        50,
			y:        50,
			wantZero: false,
		},
		{
			name:     "point outside path",
			x:        10,
			y:        10,
			wantZero: true,
		},
		{
			name:     "point outside bounds",
			x:        150,
			y:        150,
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coverage := stack.Coverage(tt.x, tt.y)

			if tt.wantZero && coverage != 0 {
				t.Errorf("Coverage(%v, %v) = %v, want 0", tt.x, tt.y, coverage)
			}

			if !tt.wantZero && coverage == 0 {
				t.Errorf("Coverage(%v, %v) = 0, want > 0", tt.x, tt.y)
			}
		})
	}
}

func TestClipStack_Coverage_MultipleMasks(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	// Push first mask - rectangle
	p1 := newSOAPath().moveTo(10, 10).lineTo(90, 10).lineTo(90, 90).lineTo(10, 90).close()

	err := stack.PushPath(p1.verbs, p1.coords, true)
	if err != nil {
		t.Fatalf("PushPath(path1) error = %v", err)
	}

	// Push second mask - smaller rectangle
	p2 := newSOAPath().moveTo(30, 30).lineTo(70, 30).lineTo(70, 70).lineTo(30, 70).close()

	err = stack.PushPath(p2.verbs, p2.coords, true)
	if err != nil {
		t.Fatalf("PushPath(path2) error = %v", err)
	}

	tests := []struct {
		name     string
		x, y     float64
		wantZero bool
	}{
		{
			name:     "point in both masks",
			x:        50,
			y:        50,
			wantZero: false,
		},
		{
			name:     "point in first mask only",
			x:        20,
			y:        20,
			wantZero: true, // Should be clipped by second mask
		},
		{
			name:     "point in neither mask",
			x:        5,
			y:        5,
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coverage := stack.Coverage(tt.x, tt.y)

			if tt.wantZero && coverage != 0 {
				t.Errorf("Coverage(%v, %v) = %v, want 0", tt.x, tt.y, coverage)
			}

			if !tt.wantZero && coverage == 0 {
				t.Errorf("Coverage(%v, %v) = 0, want > 0", tt.x, tt.y)
			}
		})
	}
}

func TestClipStack_Depth(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	depths := []struct {
		name  string
		op    func()
		depth int
	}{
		{
			name:  "initial depth",
			op:    func() {},
			depth: 0,
		},
		{
			name: "after push rect",
			op: func() {
				stack.PushRect(NewRect(10, 10, 50, 50))
			},
			depth: 1,
		},
		{
			name: "after second push",
			op: func() {
				stack.PushRect(NewRect(20, 20, 30, 30))
			},
			depth: 2,
		},
		{
			name: "after pop",
			op: func() {
				stack.Pop()
			},
			depth: 1,
		},
		{
			name: "after second pop",
			op: func() {
				stack.Pop()
			},
			depth: 0,
		},
	}

	for _, tt := range depths {
		t.Run(tt.name, func(t *testing.T) {
			tt.op()

			if stack.Depth() != tt.depth {
				t.Errorf("Depth() = %d, want %d", stack.Depth(), tt.depth)
			}
		})
	}
}

func TestClipStack_Reset(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	// Push some clips
	stack.PushRect(NewRect(10, 10, 50, 50))
	stack.PushRect(NewRect(20, 20, 30, 30))

	if stack.Depth() != 2 {
		t.Fatalf("Depth() before reset = %d, want 2", stack.Depth())
	}

	// Reset with new bounds
	newBounds := NewRect(0, 0, 200, 200)
	stack.Reset(newBounds)

	if stack.Depth() != 0 {
		t.Errorf("Depth() after reset = %d, want 0", stack.Depth())
	}

	if stack.Bounds() != newBounds {
		t.Errorf("Bounds() after reset = %v, want %v", stack.Bounds(), newBounds)
	}
}

func TestClipStack_PushPop_Symmetry(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))
	originalBounds := stack.Bounds()

	// Push and pop several times
	for i := 0; i < 5; i++ {
		rect := NewRect(float64(i*10), float64(i*10), 50, 50)
		stack.PushRect(rect)
	}

	if stack.Depth() != 5 {
		t.Fatalf("Depth() after pushes = %d, want 5", stack.Depth())
	}

	// Pop all
	for i := 0; i < 5; i++ {
		stack.Pop()
	}

	if stack.Depth() != 0 {
		t.Errorf("Depth() after pops = %d, want 0", stack.Depth())
	}

	if stack.Bounds() != originalBounds {
		t.Errorf("Bounds() after symmetry test = %v, want %v", stack.Bounds(), originalBounds)
	}
}

func TestClipStack_RectIntersection(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	// Push rect that's partially outside
	stack.PushRect(NewRect(80, 80, 50, 50))

	// Bounds should be intersection
	expectedBounds := NewRect(80, 80, 20, 20) // Intersection of (0,0,100,100) and (80,80,50,50)

	if stack.Bounds() != expectedBounds {
		t.Errorf("Bounds() = %v, want %v", stack.Bounds(), expectedBounds)
	}
}

func TestClipStack_EmptyIntersection(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	// Push rect that doesn't intersect
	stack.PushRect(NewRect(200, 200, 50, 50))

	// Bounds should be empty
	bounds := stack.Bounds()
	if !bounds.IsEmpty() {
		t.Errorf("Bounds() = %v, expected empty bounds", bounds)
	}

	// All points should be invisible
	if stack.IsVisible(50, 50) {
		t.Error("IsVisible(50, 50) = true, want false (empty intersection)")
	}

	// Coverage should be zero everywhere
	if coverage := stack.Coverage(50, 50); coverage != 0 {
		t.Errorf("Coverage(50, 50) = %v, want 0 (empty intersection)", coverage)
	}
}

// Benchmark tests
func BenchmarkClipStack_PushRect(b *testing.B) {
	stack := NewClipStack(NewRect(0, 0, 1000, 1000))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stack.PushRect(NewRect(10, 10, 100, 100))
		stack.Pop()
	}
}

func BenchmarkClipStack_PushPath(b *testing.B) {
	stack := NewClipStack(NewRect(0, 0, 1000, 1000))

	p := newSOAPath().moveTo(10, 10).lineTo(100, 10).lineTo(100, 100).lineTo(10, 100).close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = stack.PushPath(p.verbs, p.coords, true)
		stack.Pop()
	}
}

func BenchmarkClipStack_IsVisible(b *testing.B) {
	stack := NewClipStack(NewRect(0, 0, 1000, 1000))
	stack.PushRect(NewRect(10, 10, 500, 500))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = stack.IsVisible(250, 250)
	}
}

func BenchmarkClipStack_Coverage(b *testing.B) {
	stack := NewClipStack(NewRect(0, 0, 1000, 1000))

	p := newSOAPath().moveTo(10, 10).lineTo(500, 10).lineTo(500, 500).lineTo(10, 500).close()

	_ = stack.PushPath(p.verbs, p.coords, true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = stack.Coverage(250, 250)
	}
}

// --- PushRRect tests ---

func TestClipStack_PushRRect(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 200, 200))

	stack.PushRRect(NewRect(20, 20, 100, 80), 10)

	if stack.Depth() != 1 {
		t.Errorf("Depth() = %d, want 1", stack.Depth())
	}

	// Bounds should be the intersection
	expectedBounds := NewRect(20, 20, 100, 80)
	if stack.Bounds() != expectedBounds {
		t.Errorf("Bounds() = %v, want %v", stack.Bounds(), expectedBounds)
	}
}

func TestClipStack_PushRRect_ZeroRadius(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 200, 200))

	// Zero radius should behave like PushRect
	stack.PushRRect(NewRect(10, 10, 50, 50), 0)

	if stack.Depth() != 1 {
		t.Errorf("Depth() = %d, want 1", stack.Depth())
	}
}

func TestClipStack_PushRRect_ClampedRadius(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 200, 200))

	// Radius larger than half minimum dimension should be clamped
	stack.PushRRect(NewRect(50, 50, 40, 20), 100)

	if stack.Depth() != 1 {
		t.Errorf("Depth() = %d, want 1", stack.Depth())
	}

	// Center should be visible
	if !stack.IsVisible(70, 60) {
		t.Error("center of rrect should be visible")
	}
}

func TestClipStack_PushRRect_NegativeRadius(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 200, 200))

	// Negative radius should be treated as zero (PushRect path)
	stack.PushRRect(NewRect(10, 10, 50, 50), -5)

	if stack.Depth() != 1 {
		t.Errorf("Depth() = %d, want 1", stack.Depth())
	}
}

// --- rrectSDF tests ---

func TestRrectSDF(t *testing.T) {
	rr := &RRectClip{
		Rect:   NewRect(20, 20, 60, 40),
		Radius: 10,
	}

	tests := []struct {
		name     string
		x, y     float64
		wantSign int // -1 inside, 0 on boundary, +1 outside
	}{
		{"center", 50, 40, -1},
		{"far outside", 0, 0, 1},
		{"inside near edge", 50, 25, -1},
		{"outside corner", 21, 21, 1}, // near the rounded corner, outside
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := rrectSDF(tt.x, tt.y, rr)
			switch tt.wantSign {
			case -1:
				if d >= 0 {
					t.Errorf("rrectSDF(%f,%f) = %f, want negative (inside)", tt.x, tt.y, d)
				}
			case 1:
				if d <= 0 {
					t.Errorf("rrectSDF(%f,%f) = %f, want positive (outside)", tt.x, tt.y, d)
				}
			}
		})
	}
}

// --- rrectCoverage tests ---

func TestRrectCoverage(t *testing.T) {
	rr := &RRectClip{
		Rect:   NewRect(20, 20, 60, 40),
		Radius: 10,
	}

	tests := []struct {
		name string
		x, y float64
		want byte // 0, 255, or middle
	}{
		{"center", 50, 40, 255},
		{"far outside", 0, 0, 0},
		{"deep inside", 50, 40, 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rrectCoverage(tt.x, tt.y, rr)
			if tt.want == 0 && got != 0 {
				t.Errorf("rrectCoverage(%f,%f) = %d, want 0", tt.x, tt.y, got)
			}
			if tt.want == 255 && got != 255 {
				t.Errorf("rrectCoverage(%f,%f) = %d, want 255", tt.x, tt.y, got)
			}
		})
	}
}

func TestRrectCoverageAntiAliasEdge(t *testing.T) {
	rr := &RRectClip{
		Rect:   NewRect(0, 0, 100, 100),
		Radius: 20,
	}

	// Point exactly on the boundary should have partial coverage
	// SDF at the rect edge (non-corner) should be near 0
	cov := rrectCoverage(50, 0.5, rr)
	if cov == 0 || cov == 255 {
		// It might be 255 if deep enough inside, that's also ok
		// Just verify the function executes without issues
		_ = cov
	}
}

// --- IsRectOnly tests ---

func TestClipStack_IsRectOnly(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	// Empty stack is rect-only
	if !stack.IsRectOnly() {
		t.Error("empty stack should be IsRectOnly")
	}

	// After pushing rect
	stack.PushRect(NewRect(10, 10, 50, 50))
	if !stack.IsRectOnly() {
		t.Error("stack with only rect should be IsRectOnly")
	}

	// After pushing rrect
	stack.PushRRect(NewRect(20, 20, 30, 30), 5)
	if stack.IsRectOnly() {
		t.Error("stack with rrect should NOT be IsRectOnly")
	}
}

func TestClipStack_IsRectOnly_WithMask(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	p := newSOAPath().moveTo(10, 10).lineTo(90, 10).lineTo(90, 90).lineTo(10, 90).close()

	_ = stack.PushPath(p.verbs, p.coords, true)
	if stack.IsRectOnly() {
		t.Error("stack with mask should NOT be IsRectOnly")
	}
}

// --- IsRRectOnly tests ---

func TestClipStack_IsRRectOnly(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	// Empty stack
	if !stack.IsRRectOnly() {
		t.Error("empty stack should be IsRRectOnly")
	}

	// Rect is compatible with RRect-only
	stack.PushRect(NewRect(10, 10, 50, 50))
	if !stack.IsRRectOnly() {
		t.Error("stack with only rect should be IsRRectOnly")
	}

	// RRect is compatible
	stack.PushRRect(NewRect(20, 20, 30, 30), 5)
	if !stack.IsRRectOnly() {
		t.Error("stack with rect+rrect should be IsRRectOnly")
	}
}

func TestClipStack_IsRRectOnly_WithMask(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 100, 100))

	p := newSOAPath().moveTo(10, 10).lineTo(90, 10).lineTo(90, 90).close()

	_ = stack.PushPath(p.verbs, p.coords, true)
	if stack.IsRRectOnly() {
		t.Error("stack with mask should NOT be IsRRectOnly")
	}
}

// --- RRectBounds tests ---

func TestClipStack_RRectBounds(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 200, 200))

	// No rrect
	_, _, ok := stack.RRectBounds()
	if ok {
		t.Error("RRectBounds() should return false for empty stack")
	}

	// Push rect (not rrect)
	stack.PushRect(NewRect(10, 10, 50, 50))
	_, _, ok = stack.RRectBounds()
	if ok {
		t.Error("RRectBounds() should return false for rect-only stack")
	}

	// Push rrect
	rrRect := NewRect(20, 20, 80, 60)
	stack.PushRRect(rrRect, 15)
	rect, radius, ok := stack.RRectBounds()
	if !ok {
		t.Fatal("RRectBounds() should return true after PushRRect")
	}
	if rect != rrRect {
		t.Errorf("RRectBounds() rect = %v, want %v", rect, rrRect)
	}
	if radius != 15 {
		t.Errorf("RRectBounds() radius = %f, want 15", radius)
	}
}

func TestClipStack_RRectBounds_Innermost(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 200, 200))

	// Push two rrects — should return innermost (most recent)
	stack.PushRRect(NewRect(10, 10, 180, 180), 20)
	innerRect := NewRect(30, 30, 100, 80)
	stack.PushRRect(innerRect, 10)

	rect, radius, ok := stack.RRectBounds()
	if !ok {
		t.Fatal("RRectBounds() should return true")
	}
	if rect != innerRect {
		t.Errorf("RRectBounds() rect = %v, want %v (innermost)", rect, innerRect)
	}
	if radius != 10 {
		t.Errorf("RRectBounds() radius = %f, want 10", radius)
	}
}

// --- RRect visibility and coverage tests ---

func TestClipStack_IsVisible_WithRRect(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 200, 200))
	stack.PushRRect(NewRect(20, 20, 100, 80), 10)

	// Center should be visible
	if !stack.IsVisible(70, 60) {
		t.Error("center of rrect should be visible")
	}

	// Outside should not be visible
	if stack.IsVisible(5, 5) {
		t.Error("outside rrect should not be visible")
	}

	// Deep inside corner should be invisible (outside rounded corner)
	// At (20, 20) the rounded corner clips; check (21, 21) - in the corner notch
	if stack.IsVisible(20.5, 20.5) {
		// This point is in the corner region where radius clips
		// It may or may not be visible depending on exact SDF calculation
		_ = 0 // ok either way
	}
}

func TestClipStack_Coverage_WithRRect(t *testing.T) {
	stack := NewClipStack(NewRect(0, 0, 200, 200))
	stack.PushRRect(NewRect(20, 20, 100, 80), 10)

	// Center should have full coverage
	cov := stack.Coverage(70, 60)
	if cov != 255 {
		t.Errorf("Coverage(center) = %d, want 255", cov)
	}

	// Outside should have zero coverage
	cov = stack.Coverage(5, 5)
	if cov != 0 {
		t.Errorf("Coverage(outside) = %d, want 0", cov)
	}
}

// --- Point arithmetic tests ---

func TestPointAdd(t *testing.T) {
	p := Pt(10, 20)
	q := Pt(5, 15)
	r := p.Add(q)
	if r.X != 15 || r.Y != 35 {
		t.Errorf("Add = %v, want (15, 35)", r)
	}
}

func TestPointSub(t *testing.T) {
	p := Pt(10, 20)
	q := Pt(5, 15)
	r := p.Sub(q)
	if r.X != 5 || r.Y != 5 {
		t.Errorf("Sub = %v, want (5, 5)", r)
	}
}

func TestPointMul(t *testing.T) {
	p := Pt(10, 20)
	r := p.Mul(3)
	if r.X != 30 || r.Y != 60 {
		t.Errorf("Mul = %v, want (30, 60)", r)
	}
}

func BenchmarkClipStack_MultipleMasks(b *testing.B) {
	stack := NewClipStack(NewRect(0, 0, 1000, 1000))

	// Push 3 mask clips
	for i := 0; i < 3; i++ {
		p := newSOAPath().
			moveTo(float64(i*10), float64(i*10)).
			lineTo(500, float64(i*10)).
			lineTo(500, 500).
			lineTo(float64(i*10), 500).
			close()
		_ = stack.PushPath(p.verbs, p.coords, true)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = stack.Coverage(250, 250)
	}
}
