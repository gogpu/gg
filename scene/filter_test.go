package scene

import (
	"testing"

	"github.com/gogpu/gg"
)

func TestFilterTypeString(t *testing.T) {
	tests := []struct {
		ft   FilterType
		want string
	}{
		{FilterNone, "None"},
		{FilterBlur, "Blur"},
		{FilterDropShadow, "DropShadow"},
		{FilterColorMatrix, "ColorMatrix"},
		{FilterType(255), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.ft.String(); got != tt.want {
				t.Errorf("FilterType(%d).String() = %q, want %q", tt.ft, got, tt.want)
			}
		})
	}
}

func TestFilterTypeExpandsOutput(t *testing.T) {
	tests := []struct {
		ft   FilterType
		want bool
	}{
		{FilterNone, false},
		{FilterBlur, true},
		{FilterDropShadow, true},
		{FilterColorMatrix, false},
	}

	for _, tt := range tests {
		t.Run(tt.ft.String(), func(t *testing.T) {
			if got := tt.ft.ExpandsOutput(); got != tt.want {
				t.Errorf("FilterType(%d).ExpandsOutput() = %v, want %v", tt.ft, got, tt.want)
			}
		})
	}
}

// mockFilter is a test filter that tracks calls
type mockFilter struct {
	applyCalled  bool
	expandCalled bool
	expandAmount float32
	modifyDst    bool
}

func (f *mockFilter) Apply(src, dst *gg.Pixmap, bounds Rect) {
	f.applyCalled = true
	if f.modifyDst && dst != nil && src != nil {
		// Copy with slight modification to verify it was called
		for y := int(bounds.MinY); y < int(bounds.MaxY) && y < dst.Height(); y++ {
			for x := int(bounds.MinX); x < int(bounds.MaxX) && x < dst.Width(); x++ {
				c := src.GetPixel(x, y)
				c.R = 1.0 - c.R // Invert red channel
				dst.SetPixel(x, y, c)
			}
		}
	}
}

func (f *mockFilter) ExpandBounds(input Rect) Rect {
	f.expandCalled = true
	return Rect{
		MinX: input.MinX - f.expandAmount,
		MinY: input.MinY - f.expandAmount,
		MaxX: input.MaxX + f.expandAmount,
		MaxY: input.MaxY + f.expandAmount,
	}
}

func TestFilterChainEmpty(t *testing.T) {
	chain := NewFilterChain()

	if !chain.IsEmpty() {
		t.Error("NewFilterChain() should create empty chain")
	}

	if chain.Len() != 0 {
		t.Errorf("empty chain Len() = %d, want 0", chain.Len())
	}
}

func TestFilterChainSingleFilter(t *testing.T) {
	filter := &mockFilter{expandAmount: 5}
	chain := NewFilterChain(filter)

	if chain.IsEmpty() {
		t.Error("chain should not be empty after adding filter")
	}

	if chain.Len() != 1 {
		t.Errorf("chain Len() = %d, want 1", chain.Len())
	}

	bounds := Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100}
	expanded := chain.ExpandBounds(bounds)

	if !filter.expandCalled {
		t.Error("ExpandBounds should call filter.ExpandBounds")
	}

	if expanded.MinX != -5 || expanded.MinY != -5 {
		t.Errorf("ExpandBounds MinX/MinY = %v/%v, want -5/-5", expanded.MinX, expanded.MinY)
	}
}

func TestFilterChainMultipleFilters(t *testing.T) {
	f1 := &mockFilter{expandAmount: 5, modifyDst: true}
	f2 := &mockFilter{expandAmount: 10, modifyDst: true}
	chain := NewFilterChain(f1, f2)

	if chain.Len() != 2 {
		t.Errorf("chain Len() = %d, want 2", chain.Len())
	}

	// Test combined expansion
	bounds := Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100}
	expanded := chain.ExpandBounds(bounds)

	// First filter expands by 5, second by 10
	// -5 - 10 = -15, 100 + 5 + 10 = 115
	if expanded.MinX != -15 || expanded.MaxX != 115 {
		t.Errorf("ExpandBounds = %+v, want MinX=-15, MaxX=115", expanded)
	}
}

func TestFilterChainAdd(t *testing.T) {
	chain := NewFilterChain()
	chain.Add(&mockFilter{expandAmount: 5})
	chain.Add(nil) // Should be ignored
	chain.Add(&mockFilter{expandAmount: 10})

	if chain.Len() != 2 {
		t.Errorf("chain Len() = %d, want 2 (nil should be ignored)", chain.Len())
	}
}

func TestFilterChainApply(t *testing.T) {
	// Create source and destination pixmaps
	src := gg.NewPixmap(10, 10)
	dst := gg.NewPixmap(10, 10)

	// Fill source with red
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			src.SetPixel(x, y, gg.Red)
		}
	}

	// Create filter that inverts red channel
	filter := &mockFilter{modifyDst: true}
	chain := NewFilterChain(filter)

	bounds := Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10}
	chain.Apply(src, dst, bounds)

	if !filter.applyCalled {
		t.Error("Apply should call filter.Apply")
	}

	// Check that red was inverted to cyan (R: 1->0)
	c := dst.GetPixel(5, 5)
	if c.R > 0.01 {
		t.Errorf("pixel R = %v, want ~0 (inverted)", c.R)
	}
}

func TestFilterChainNilFilters(t *testing.T) {
	// Should handle nil filters gracefully
	chain := NewFilterChain(nil, nil, nil)

	if !chain.IsEmpty() {
		t.Error("chain with only nil filters should be empty")
	}
}

func TestCopyPixmap(t *testing.T) {
	src := gg.NewPixmap(5, 5)
	dst := gg.NewPixmap(5, 5)

	// Fill source with specific colors
	for y := 0; y < 5; y++ {
		for x := 0; x < 5; x++ {
			src.SetPixel(x, y, gg.RGBA2(float64(x)/5, float64(y)/5, 0.5, 1.0))
		}
	}

	bounds := Rect{MinX: 1, MinY: 1, MaxX: 4, MaxY: 4}
	copyPixmap(src, dst, bounds)

	// Check copied region
	for y := 1; y < 4; y++ {
		for x := 1; x < 4; x++ {
			srcC := src.GetPixel(x, y)
			dstC := dst.GetPixel(x, y)
			if !colorApproxEqual(srcC, dstC, 0.01) {
				t.Errorf("pixel (%d,%d) not copied: src=%+v, dst=%+v", x, y, srcC, dstC)
			}
		}
	}

	// Check outside region wasn't modified
	c := dst.GetPixel(0, 0)
	if c.A != 0 {
		t.Error("pixel outside bounds should not be modified")
	}
}

// colorApproxEqual compares two colors with tolerance
func colorApproxEqual(a, b gg.RGBA, tolerance float64) bool {
	return absf(a.R-b.R) < tolerance &&
		absf(a.G-b.G) < tolerance &&
		absf(a.B-b.B) < tolerance &&
		absf(a.A-b.A) < tolerance
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
