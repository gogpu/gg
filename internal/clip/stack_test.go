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
	path := []PathElement{
		MoveTo{Point: Pt(10, 10)},
		LineTo{Point: Pt(50, 10)},
		LineTo{Point: Pt(50, 50)},
		LineTo{Point: Pt(10, 50)},
		Close{},
	}

	err := stack.PushPath(path, true)
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

	path := []PathElement{
		MoveTo{Point: Pt(10, 10)},
		LineTo{Point: Pt(50, 10)},
	}

	err := stack.PushPath(path, true)
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
	path := []PathElement{
		MoveTo{Point: Pt(50, 20)},
		LineTo{Point: Pt(20, 80)},
		LineTo{Point: Pt(80, 80)},
		Close{},
	}

	err := stack.PushPath(path, true)
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
	path := []PathElement{
		MoveTo{Point: Pt(20, 20)},
		LineTo{Point: Pt(80, 20)},
		LineTo{Point: Pt(80, 80)},
		LineTo{Point: Pt(20, 80)},
		Close{},
	}

	err := stack.PushPath(path, true)
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
	path1 := []PathElement{
		MoveTo{Point: Pt(10, 10)},
		LineTo{Point: Pt(90, 10)},
		LineTo{Point: Pt(90, 90)},
		LineTo{Point: Pt(10, 90)},
		Close{},
	}

	err := stack.PushPath(path1, true)
	if err != nil {
		t.Fatalf("PushPath(path1) error = %v", err)
	}

	// Push second mask - smaller rectangle
	path2 := []PathElement{
		MoveTo{Point: Pt(30, 30)},
		LineTo{Point: Pt(70, 30)},
		LineTo{Point: Pt(70, 70)},
		LineTo{Point: Pt(30, 70)},
		Close{},
	}

	err = stack.PushPath(path2, true)
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

	path := []PathElement{
		MoveTo{Point: Pt(10, 10)},
		LineTo{Point: Pt(100, 10)},
		LineTo{Point: Pt(100, 100)},
		LineTo{Point: Pt(10, 100)},
		Close{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = stack.PushPath(path, true)
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

	path := []PathElement{
		MoveTo{Point: Pt(10, 10)},
		LineTo{Point: Pt(500, 10)},
		LineTo{Point: Pt(500, 500)},
		LineTo{Point: Pt(10, 500)},
		Close{},
	}

	_ = stack.PushPath(path, true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = stack.Coverage(250, 250)
	}
}

func BenchmarkClipStack_MultipleMasks(b *testing.B) {
	stack := NewClipStack(NewRect(0, 0, 1000, 1000))

	// Push 3 mask clips
	for i := 0; i < 3; i++ {
		path := []PathElement{
			MoveTo{Point: Pt(float64(i*10), float64(i*10))},
			LineTo{Point: Pt(500, float64(i*10))},
			LineTo{Point: Pt(500, 500)},
			LineTo{Point: Pt(float64(i*10), 500)},
			Close{},
		}
		_ = stack.PushPath(path, true)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = stack.Coverage(250, 250)
	}
}
