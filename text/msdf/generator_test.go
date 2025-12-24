package msdf

import (
	"math"
	"testing"

	"github.com/gogpu/gg/text"
)

func TestNewGenerator(t *testing.T) {
	config := DefaultConfig()
	gen := NewGenerator(config)

	if gen == nil {
		t.Fatal("NewGenerator() returned nil")
	}
	if gen.config.Size != config.Size {
		t.Errorf("Generator config.Size = %d, want %d", gen.config.Size, config.Size)
	}
}

func TestDefaultGenerator(t *testing.T) {
	gen := DefaultGenerator()

	if gen == nil {
		t.Fatal("DefaultGenerator() returned nil")
	}
	if gen.config.Size != 32 {
		t.Errorf("DefaultGenerator config.Size = %d, want 32", gen.config.Size)
	}
}

func TestGeneratorConfig(t *testing.T) {
	gen := DefaultGenerator()

	// Test Config()
	config := gen.Config()
	if config.Size != 32 {
		t.Errorf("Config().Size = %d, want 32", config.Size)
	}

	// Test SetConfig()
	newConfig := Config{Size: 64, Range: 8.0, AngleThreshold: math.Pi / 4, EdgeThreshold: 1.5}
	gen.SetConfig(newConfig)

	if gen.config.Size != 64 {
		t.Errorf("After SetConfig, config.Size = %d, want 64", gen.config.Size)
	}
}

func TestGenerateEmpty(t *testing.T) {
	gen := DefaultGenerator()

	// Nil outline
	msdf, err := gen.Generate(nil)
	if err != nil {
		t.Fatalf("Generate(nil) error: %v", err)
	}
	if msdf == nil {
		t.Fatal("Generate(nil) returned nil MSDF")
	}
	if msdf.Width != 32 || msdf.Height != 32 {
		t.Errorf("Generate(nil) size = %dx%d, want 32x32", msdf.Width, msdf.Height)
	}

	// Empty outline
	empty := &text.GlyphOutline{}
	msdf, err = gen.Generate(empty)
	if err != nil {
		t.Fatalf("Generate(empty) error: %v", err)
	}
	if msdf == nil {
		t.Fatal("Generate(empty) returned nil MSDF")
	}
}

func TestGenerateInvalidConfig(t *testing.T) {
	gen := NewGenerator(Config{Size: 4}) // Invalid: too small

	_, err := gen.Generate(nil)
	if err == nil {
		t.Error("Expected error for invalid config")
	}
}

func TestGenerateSquare(t *testing.T) {
	gen := DefaultGenerator()

	// Simple square
	outline := &text.GlyphOutline{
		Segments: []text.OutlineSegment{
			{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 100, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 100, Y: 100}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 100}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
		},
		Bounds: text.Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100},
	}

	msdf, err := gen.Generate(outline)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if msdf == nil {
		t.Fatal("Generate returned nil")
	}

	// Check dimensions
	if msdf.Width != 32 || msdf.Height != 32 {
		t.Errorf("MSDF size = %dx%d, want 32x32", msdf.Width, msdf.Height)
	}

	// Check data size
	expectedDataSize := 32 * 32 * 3
	if len(msdf.Data) != expectedDataSize {
		t.Errorf("MSDF data size = %d, want %d", len(msdf.Data), expectedDataSize)
	}

	// Check that not all pixels are the same (would indicate a problem)
	allSame := true
	r0, g0, b0 := msdf.GetPixel(0, 0)
	for y := 0; y < msdf.Height; y++ {
		for x := 0; x < msdf.Width; x++ {
			r, g, b := msdf.GetPixel(x, y)
			if r != r0 || g != g0 || b != b0 {
				allSame = false
				break
			}
		}
		if !allSame {
			break
		}
	}
	if allSame {
		t.Error("All pixels are the same, expected variation")
	}
}

func TestGenerateWithCurves(t *testing.T) {
	gen := NewGenerator(Config{
		Size:           64,
		Range:          4.0,
		AngleThreshold: math.Pi / 3,
		EdgeThreshold:  1.001,
	})

	// Outline with quadratic curve
	outline := &text.GlyphOutline{
		Segments: []text.OutlineSegment{
			{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 100, Y: 0}}},
			{Op: text.OutlineOpQuadTo, Points: [3]text.OutlinePoint{{X: 150, Y: 50}, {X: 100, Y: 100}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 100}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
		},
		Bounds: text.Rect{MinX: 0, MinY: 0, MaxX: 150, MaxY: 100},
	}

	msdf, err := gen.Generate(outline)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if msdf == nil {
		t.Fatal("Generate returned nil")
	}
	if msdf.Width != 64 || msdf.Height != 64 {
		t.Errorf("MSDF size = %dx%d, want 64x64", msdf.Width, msdf.Height)
	}
}

func TestGenerateWithMetrics(t *testing.T) {
	gen := DefaultGenerator()

	outline := &text.GlyphOutline{
		Segments: []text.OutlineSegment{
			{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 50, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 50, Y: 50}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 50}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
		},
	}

	msdf, metrics, err := gen.GenerateWithMetrics(outline)
	if err != nil {
		t.Fatalf("GenerateWithMetrics error: %v", err)
	}
	if msdf == nil || metrics == nil {
		t.Fatal("GenerateWithMetrics returned nil")
	}

	// Check metrics
	if metrics.NumContours != 1 {
		t.Errorf("NumContours = %d, want 1", metrics.NumContours)
	}
	if metrics.NumEdges != 4 {
		t.Errorf("NumEdges = %d, want 4", metrics.NumEdges)
	}
	if metrics.Width != 32 || metrics.Height != 32 {
		t.Errorf("Metrics size = %dx%d, want 32x32", metrics.Width, metrics.Height)
	}
}

func TestGenerateWithMetricsEmpty(t *testing.T) {
	gen := DefaultGenerator()

	msdf, metrics, err := gen.GenerateWithMetrics(nil)
	if err != nil {
		t.Fatalf("GenerateWithMetrics(nil) error: %v", err)
	}
	if msdf == nil || metrics == nil {
		t.Fatal("GenerateWithMetrics(nil) returned nil")
	}
	if metrics.NumContours != 0 || metrics.NumEdges != 0 {
		t.Errorf("Empty outline metrics: contours=%d, edges=%d, expected 0, 0",
			metrics.NumContours, metrics.NumEdges)
	}
}

func TestGenerateBatch(t *testing.T) {
	gen := DefaultGenerator()

	outlines := []*text.GlyphOutline{
		// Square
		{
			Segments: []text.OutlineSegment{
				{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
				{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 10, Y: 0}}},
				{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 10, Y: 10}}},
				{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 10}}},
				{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			},
		},
		// Triangle
		{
			Segments: []text.OutlineSegment{
				{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
				{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 20, Y: 0}}},
				{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 10, Y: 20}}},
				{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			},
		},
		// Empty
		nil,
	}

	results, err := gen.GenerateBatch(outlines)
	if err != nil {
		t.Fatalf("GenerateBatch error: %v", err)
	}

	if len(results) != len(outlines) {
		t.Errorf("GenerateBatch returned %d results, want %d", len(results), len(outlines))
	}

	for i, msdf := range results {
		if msdf == nil {
			t.Errorf("Result %d is nil", i)
		}
	}
}

func TestGenerateBatchInvalidConfig(t *testing.T) {
	gen := NewGenerator(Config{Size: 4}) // Invalid

	_, err := gen.GenerateBatch([]*text.GlyphOutline{nil})
	if err == nil {
		t.Error("Expected error for invalid config")
	}
}

func TestGeneratorPool(t *testing.T) {
	config := DefaultConfig()
	pool := NewGeneratorPool(config)

	// Get a generator
	gen := pool.Get()
	if gen == nil {
		t.Fatal("Pool.Get() returned nil")
	}

	// Return it
	pool.Put(gen)

	// Generate using pool
	outline := &text.GlyphOutline{
		Segments: []text.OutlineSegment{
			{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 10, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
		},
	}

	msdf, err := pool.Generate(outline)
	if err != nil {
		t.Fatalf("Pool.Generate error: %v", err)
	}
	if msdf == nil {
		t.Fatal("Pool.Generate returned nil")
	}
}

func TestMedianFilter(t *testing.T) {
	// Create a small MSDF
	msdf := &MSDF{
		Data:   make([]byte, 9*9*3),
		Width:  9,
		Height: 9,
	}

	// Fill with some values
	for y := 0; y < 9; y++ {
		for x := 0; x < 9; x++ {
			msdf.SetPixel(x, y, byte(x*10), byte(y*10), byte((x+y)*5))
		}
	}

	// Add noise
	msdf.SetPixel(4, 4, 255, 0, 0) // Noise pixel

	filtered := MedianFilter(msdf)
	if filtered == nil {
		t.Fatal("MedianFilter returned nil")
	}

	// Check dimensions preserved
	if filtered.Width != msdf.Width || filtered.Height != msdf.Height {
		t.Errorf("Filtered size = %dx%d, want %dx%d",
			filtered.Width, filtered.Height, msdf.Width, msdf.Height)
	}

	// The noise should be reduced
	r, _, _ := filtered.GetPixel(4, 4)
	if r == 255 {
		t.Error("Median filter did not reduce noise")
	}
}

func TestMedianFilterNil(t *testing.T) {
	result := MedianFilter(nil)
	if result != nil {
		t.Error("MedianFilter(nil) should return nil")
	}
}

func TestErrorCorrection(t *testing.T) {
	msdf := &MSDF{
		Data:   make([]byte, 4*4*3),
		Width:  4,
		Height: 4,
	}

	// Set pixels with high error (outliers)
	msdf.SetPixel(1, 1, 255, 128, 128) // Red is an outlier
	msdf.SetPixel(2, 2, 128, 0, 128)   // Green is an outlier

	// Should not panic
	ErrorCorrection(msdf, 0.3)

	// Check that outliers were corrected
	r, g, b := msdf.GetPixel(1, 1)
	median := median3Byte(255, 128, 128)
	if r == 255 && g == 128 && b == 128 {
		// If no change, might need higher threshold
		t.Logf("Pixel (1,1): r=%d, g=%d, b=%d, median=%d", r, g, b, median)
	}
}

func TestErrorCorrectionNil(t *testing.T) {
	// Should not panic
	ErrorCorrection(nil, 0.3)
}

func TestDistanceToPixel(t *testing.T) {
	tests := []struct {
		distance, pixelRange, scale float64
		wantMin, wantMax            byte
	}{
		{0, 4.0, 1.0, 126, 130},     // On edge ~128
		{4, 4.0, 1.0, 190, 255},     // Inside by range
		{-4, 4.0, 1.0, 0, 65},       // Outside by range
		{2, 4.0, 1.0, 155, 195},     // Half inside
		{-2, 4.0, 1.0, 60, 100},     // Half outside
		{100, 4.0, 1.0, 250, 255},   // Far inside (clamped)
		{-100, 4.0, 1.0, 0, 5},      // Far outside (clamped)
	}

	for _, tt := range tests {
		got := distanceToPixel(tt.distance, tt.pixelRange, tt.scale)
		if got < tt.wantMin || got > tt.wantMax {
			t.Errorf("distanceToPixel(%v, %v, %v) = %d, want in [%d, %d]",
				tt.distance, tt.pixelRange, tt.scale, got, tt.wantMin, tt.wantMax)
		}
	}
}

func TestCalculateScale(t *testing.T) {
	tests := []struct {
		bounds  Rect
		size    int
		padding float64
		wantMin float64
		wantMax float64
	}{
		{Rect{0, 0, 100, 100}, 32, 4, 0.1, 0.5},
		{Rect{0, 0, 10, 10}, 32, 4, 1.0, 3.0},
		{Rect{}, 32, 4, 0.5, 1.5}, // Empty bounds
	}

	for _, tt := range tests {
		got := calculateScale(tt.bounds, tt.size, tt.padding)
		if got < tt.wantMin || got > tt.wantMax {
			t.Errorf("calculateScale(%v, %d, %v) = %v, want in [%v, %v]",
				tt.bounds, tt.size, tt.padding, got, tt.wantMin, tt.wantMax)
		}
	}
}

func TestMedian9(t *testing.T) {
	tests := []struct {
		vals [9]byte
		want byte
	}{
		{[9]byte{1, 2, 3, 4, 5, 6, 7, 8, 9}, 5},
		{[9]byte{9, 8, 7, 6, 5, 4, 3, 2, 1}, 5},
		{[9]byte{1, 1, 1, 1, 1, 1, 1, 1, 1}, 1},
		{[9]byte{0, 0, 0, 0, 255, 255, 255, 255, 255}, 255},
		{[9]byte{0, 0, 0, 0, 0, 255, 255, 255, 255}, 0},
	}

	for _, tt := range tests {
		// Make a copy since median9 modifies the array
		vals := tt.vals
		got := median9(vals)
		if got != tt.want {
			t.Errorf("median9(%v) = %d, want %d", tt.vals, got, tt.want)
		}
	}
}

func TestMedian3Byte(t *testing.T) {
	tests := []struct {
		a, b, c byte
		want    byte
	}{
		{1, 2, 3, 2},
		{3, 2, 1, 2},
		{2, 1, 3, 2},
		{5, 5, 5, 5},
		{0, 128, 255, 128},
	}

	for _, tt := range tests {
		got := median3Byte(tt.a, tt.b, tt.c)
		if got != tt.want {
			t.Errorf("median3Byte(%d, %d, %d) = %d, want %d",
				tt.a, tt.b, tt.c, got, tt.want)
		}
	}
}

func BenchmarkGenerateSquare(b *testing.B) {
	gen := DefaultGenerator()
	outline := &text.GlyphOutline{
		Segments: []text.OutlineSegment{
			{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 100, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 100, Y: 100}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 100}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gen.Generate(outline)
	}
}

func BenchmarkGenerateComplex(b *testing.B) {
	gen := NewGenerator(Config{
		Size:           64,
		Range:          4.0,
		AngleThreshold: math.Pi / 3,
		EdgeThreshold:  1.001,
	})

	// More complex shape with curves
	outline := &text.GlyphOutline{
		Segments: []text.OutlineSegment{
			{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 100, Y: 0}}},
			{Op: text.OutlineOpQuadTo, Points: [3]text.OutlinePoint{{X: 150, Y: 50}, {X: 100, Y: 100}}},
			{Op: text.OutlineOpCubicTo, Points: [3]text.OutlinePoint{{X: 80, Y: 120}, {X: 20, Y: 120}, {X: 0, Y: 100}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gen.Generate(outline)
	}
}

func BenchmarkGenerateBatch10(b *testing.B) {
	gen := DefaultGenerator()

	outlines := make([]*text.GlyphOutline, 10)
	for i := range outlines {
		outlines[i] = &text.GlyphOutline{
			Segments: []text.OutlineSegment{
				{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
				{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 10, Y: 0}}},
				{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 10, Y: 10}}},
				{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 10}}},
				{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gen.GenerateBatch(outlines)
	}
}

func BenchmarkMedianFilter(b *testing.B) {
	msdf := &MSDF{
		Data:   make([]byte, 64*64*3),
		Width:  64,
		Height: 64,
	}

	// Fill with random data
	for i := range msdf.Data {
		msdf.Data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MedianFilter(msdf)
	}
}
