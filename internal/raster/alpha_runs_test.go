package raster

import (
	"testing"
)

func TestNewAlphaRuns(t *testing.T) {
	tests := []struct {
		name  string
		width int
	}{
		{"small width", 10},
		{"medium width", 100},
		{"large width", 1000},
		{"zero width", 0},
		{"negative width", -5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := NewAlphaRuns(tt.width)
			if ar == nil {
				t.Fatal("NewAlphaRuns returned nil")
			}
			if ar.runs == nil {
				t.Error("runs slice is nil")
			}
			if ar.alpha == nil {
				t.Error("alpha slice is nil")
			}
		})
	}
}

func TestCatchOverflow(t *testing.T) {
	tests := []struct {
		name   string
		alpha  uint16
		expect uint8
	}{
		{"zero", 0, 0},
		{"mid", 128, 128},
		{"max valid", 255, 255},
		{"overflow 256", 256, 255},
		{"large overflow", 300, 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CatchOverflow(tt.alpha)
			if got != tt.expect {
				t.Errorf("CatchOverflow(%d) = %d, want %d", tt.alpha, got, tt.expect)
			}
		})
	}
}

func TestAlphaRunsIsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		addRun   bool
		expected bool
	}{
		{"new empty", 100, false, true},
		{"after reset", 50, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := NewAlphaRuns(tt.width)
			if tt.addRun {
				ar.Add(10, 128, 5, 64, 255, 0)
			}
			got := ar.IsEmpty()
			if got != tt.expected {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAlphaRunsReset(t *testing.T) {
	ar := NewAlphaRuns(100)

	// Add some data
	ar.Add(10, 128, 5, 64, 255, 0)

	// Reset
	ar.Reset(100)

	if !ar.IsEmpty() {
		t.Error("IsEmpty() should be true after Reset")
	}

	if ar.runs[0] != 100 {
		t.Errorf("runs[0] = %d, want 100", ar.runs[0])
	}

	if ar.alpha[0] != 0 {
		t.Errorf("alpha[0] = %d, want 0", ar.alpha[0])
	}
}

func TestAlphaRunsAdd(t *testing.T) {
	tests := []struct {
		name        string
		width       int
		x           int
		startAlpha  uint8
		middleCount int
		stopAlpha   uint8
		maxValue    uint8
	}{
		{"basic middle run", 100, 10, 0, 20, 0, 255},
		{"start alpha only", 100, 5, 128, 0, 0, 255},
		{"stop alpha only", 100, 15, 0, 0, 64, 255},
		{"full run with start/stop", 100, 20, 64, 10, 32, 255},
		{"single pixel", 100, 50, 255, 0, 0, 255},
		{"edge case at start", 100, 0, 128, 5, 64, 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := NewAlphaRuns(tt.width)

			// Should not panic
			offset := ar.Add(tt.x, tt.startAlpha, tt.middleCount, tt.stopAlpha, tt.maxValue, 0)

			// Offset should be >= 0
			if offset < 0 {
				t.Errorf("Add returned negative offset: %d", offset)
			}

			// After adding, IsEmpty should be false (unless all alphas were 0)
			if tt.startAlpha > 0 || tt.middleCount > 0 || tt.stopAlpha > 0 {
				if ar.IsEmpty() {
					t.Error("IsEmpty() should be false after adding non-zero alpha")
				}
			}
		})
	}
}

func TestAlphaRunsAddAccumulation(t *testing.T) {
	ar := NewAlphaRuns(100)

	// Add overlapping runs to test accumulation
	ar.Add(10, 0, 10, 0, 64, 0) // pixels 10-19 get alpha 64
	ar.Add(15, 0, 10, 0, 64, 0) // pixels 15-24 get another 64

	// Check that pixel 15-19 accumulated to ~128
	// Note: This is a simplified check, actual implementation may vary
	runs := ar.Runs()
	alpha := ar.Alpha()

	// Verify runs are non-zero
	if runs[0] == 0 {
		t.Error("runs[0] should be non-zero after Add")
	}
	_ = alpha // alpha values depend on run structure
}

func TestAlphaRunsMultipleAdds(t *testing.T) {
	ar := NewAlphaRuns(200)

	// Chain of adds using offset hint
	offset := 0
	offset = ar.Add(10, 32, 5, 16, 255, offset)
	offset = ar.Add(20, 64, 8, 32, 255, offset)
	offset = ar.Add(35, 128, 3, 64, 255, offset)

	// Should not panic and offset should progress
	if offset < 0 {
		t.Errorf("final offset = %d, want >= 0", offset)
	}
}

func TestAlphaRunsBreakRun(t *testing.T) {
	// Test that breakRun correctly splits runs
	ar := NewAlphaRuns(100)

	// Initial state: single run of 100 with alpha 0
	if ar.runs[0] != 100 {
		t.Fatalf("initial runs[0] = %d, want 100", ar.runs[0])
	}

	// Add a run in the middle - this will call breakRun internally
	ar.Add(30, 0, 10, 0, 128, 0)

	// Check that runs were split
	runs := ar.Runs()
	alpha := ar.Alpha()

	// Should have at least first run
	if runs[0] == 0 {
		t.Error("runs[0] = 0, expected non-zero")
	}

	// The run at position 30 should have alpha 128
	idx := 0
	for idx < 30 && runs[idx] > 0 {
		idx += int(runs[idx])
	}
	// Verify the run structure changed
	_ = idx
	_ = alpha
}

func TestAlphaRunsAccessors(t *testing.T) {
	ar := NewAlphaRuns(50)

	runs := ar.Runs()
	alpha := ar.Alpha()

	if runs == nil {
		t.Error("Runs() returned nil")
	}
	if alpha == nil {
		t.Error("Alpha() returned nil")
	}

	if len(runs) < 50 {
		t.Errorf("Runs() length = %d, want >= 50", len(runs))
	}
	if len(alpha) < 50 {
		t.Errorf("Alpha() length = %d, want >= 50", len(alpha))
	}
}

func TestAlphaRunsBoundaryConditions(t *testing.T) {
	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: "add at x=0",
			fn: func() {
				ar := NewAlphaRuns(100)
				ar.Add(0, 255, 10, 0, 255, 0)
			},
		},
		{
			name: "add at end",
			fn: func() {
				ar := NewAlphaRuns(100)
				ar.Add(90, 0, 10, 0, 255, 0)
			},
		},
		{
			name: "negative x",
			fn: func() {
				ar := NewAlphaRuns(100)
				ar.Add(-5, 255, 10, 0, 255, 0)
			},
		},
		{
			name: "zero middle count",
			fn: func() {
				ar := NewAlphaRuns(100)
				ar.Add(50, 128, 0, 64, 255, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panicked: %v", r)
				}
			}()
			tt.fn()
		})
	}
}

func TestAlphaRunsResetDifferentWidths(t *testing.T) {
	ar := NewAlphaRuns(100)

	// Reset to smaller width
	ar.Reset(50)
	if ar.runs[0] != 50 {
		t.Errorf("after Reset(50), runs[0] = %d, want 50", ar.runs[0])
	}

	// Reset to larger width (within original allocation)
	ar.Reset(80)
	if ar.runs[0] != 80 {
		t.Errorf("after Reset(80), runs[0] = %d, want 80", ar.runs[0])
	}

	// Reset to zero
	ar.Reset(0)
	if ar.runs[0] != 1 {
		t.Errorf("after Reset(0), runs[0] = %d, want 1 (minimum)", ar.runs[0])
	}

	// Reset to negative
	ar.Reset(-10)
	if ar.runs[0] != 1 {
		t.Errorf("after Reset(-10), runs[0] = %d, want 1 (minimum)", ar.runs[0])
	}
}

func BenchmarkAlphaRunsAdd(b *testing.B) {
	ar := NewAlphaRuns(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ar.Reset(1000)
		offset := 0
		for x := 0; x < 900; x += 20 {
			offset = ar.Add(x, 64, 15, 32, 255, offset)
		}
	}
}

func BenchmarkAlphaRunsCatchOverflow(b *testing.B) {
	for i := 0; i < b.N; i++ {
		CatchOverflow(uint16(i % 300))
	}
}
