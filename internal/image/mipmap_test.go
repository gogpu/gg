package image

import (
	"math"
	"testing"
)

func TestGenerateMipmaps(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		height     int
		wantLevels int
	}{
		{
			name:       "64x64 square",
			width:      64,
			height:     64,
			wantLevels: 7, // 64, 32, 16, 8, 4, 2, 1
		},
		{
			name:       "128x64 rectangle",
			width:      128,
			height:     64,
			wantLevels: 8, // based on max(128, 64) = 128
		},
		{
			name:       "1x1 minimum",
			width:      1,
			height:     1,
			wantLevels: 1, // just the original
		},
		{
			name:       "256x256 large",
			width:      256,
			height:     256,
			wantLevels: 9, // 256, 128, 64, 32, 16, 8, 4, 2, 1
		},
		{
			name:       "100x50 odd dimensions",
			width:      100,
			height:     50,
			wantLevels: 7, // based on max(100, 50) = 100
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := NewImageBuf(tt.width, tt.height, FormatRGBA8)
			if err != nil {
				t.Fatalf("NewImageBuf() error = %v", err)
			}

			// Fill with a pattern for visual verification
			for y := 0; y < tt.height; y++ {
				for x := 0; x < tt.width; x++ {
					r := uint8((x * 255) / tt.width)
					g := uint8((y * 255) / tt.height)
					_ = src.SetRGBA(x, y, r, g, 128, 255)
				}
			}

			chain := GenerateMipmaps(src)
			if chain == nil {
				t.Fatal("GenerateMipmaps() returned nil")
			}

			if got := chain.NumLevels(); got != tt.wantLevels {
				t.Errorf("NumLevels() = %v, want %v", got, tt.wantLevels)
			}

			// Verify level 0 is the original
			if chain.Level(0) != src {
				t.Error("Level(0) should be the original image")
			}

			// Verify each level is half the previous size
			for i := 1; i < chain.NumLevels(); i++ {
				prev := chain.Level(i - 1)
				curr := chain.Level(i)

				if curr == nil {
					t.Errorf("Level(%d) is nil", i)
					continue
				}

				prevW, prevH := prev.Bounds()
				currW, currH := curr.Bounds()

				// Each dimension should be half (or 1 if previous was 1)
				wantW := max(1, prevW/2)
				wantH := max(1, prevH/2)

				if currW != wantW || currH != wantH {
					t.Errorf("Level(%d) size = %dx%d, want %dx%d",
						i, currW, currH, wantW, wantH)
				}
			}

			// Cleanup
			chain.Release()
		})
	}
}

func TestGenerateMipmaps_NilEmpty(t *testing.T) {
	tests := []struct {
		name string
		src  *ImageBuf
	}{
		{
			name: "nil source",
			src:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := GenerateMipmaps(tt.src)
			if chain != nil {
				t.Errorf("GenerateMipmaps() = %v, want nil", chain)
			}
		})
	}
}

func TestMipmapChain_Level(t *testing.T) {
	src, err := NewImageBuf(16, 16, FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf() error = %v", err)
	}

	chain := GenerateMipmaps(src)
	if chain == nil {
		t.Fatal("GenerateMipmaps() returned nil")
	}
	defer chain.Release()

	tests := []struct {
		name    string
		level   int
		wantNil bool
	}{
		{"level 0", 0, false},
		{"level 1", 1, false},
		{"negative level", -1, true},
		{"too high level", 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chain.Level(tt.level)
			if (got == nil) != tt.wantNil {
				t.Errorf("Level(%d) nil = %v, want nil = %v",
					tt.level, got == nil, tt.wantNil)
			}
		})
	}
}

func TestMipmapChain_LevelForScale(t *testing.T) {
	src, err := NewImageBuf(64, 64, FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf() error = %v", err)
	}

	chain := GenerateMipmaps(src)
	if chain == nil {
		t.Fatal("GenerateMipmaps() returned nil")
	}
	defer chain.Release()

	tests := []struct {
		name      string
		scale     float64
		wantLevel int
	}{
		{"full size", 1.0, 0},
		{"larger than original", 2.0, 0},
		{"half size", 0.5, 1},
		{"quarter size", 0.25, 2},
		{"eighth size", 0.125, 3},
		{"very small", 0.01, 6}, // clamped to max level
		{"slightly less than 1", 0.9, 0},
		{"slightly less than 0.5", 0.4, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chain.LevelForScale(tt.scale)
			if got == nil {
				t.Fatal("LevelForScale() returned nil")
			}

			// Find which level this is
			gotLevel := -1
			for i := 0; i < chain.NumLevels(); i++ {
				if chain.Level(i) == got {
					gotLevel = i
					break
				}
			}

			if gotLevel != tt.wantLevel {
				t.Errorf("LevelForScale(%v) returned level %d, want level %d",
					tt.scale, gotLevel, tt.wantLevel)
			}
		})
	}
}

func TestMipmapChain_Release(t *testing.T) {
	src, err := NewImageBuf(32, 32, FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf() error = %v", err)
	}

	chain := GenerateMipmaps(src)
	if chain == nil {
		t.Fatal("GenerateMipmaps() returned nil")
	}

	// Capture level pointers before release
	numLevels := chain.NumLevels()
	if numLevels < 2 {
		t.Fatal("Need at least 2 levels for this test")
	}

	level0 := chain.Level(0)

	// Release should not affect level 0
	chain.Release()

	if chain.Level(0) != level0 {
		t.Error("Level 0 should not be affected by Release()")
	}

	// Level 1 should be nil after release
	if chain.Level(1) != nil {
		t.Error("Level 1 should be nil after Release()")
	}

	// Should be safe to call Release() multiple times
	chain.Release()

	// Test nil chain
	var nilChain *MipmapChain
	nilChain.Release() // should not panic
}

func TestDownsample_OddDimensions(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"odd width", 7, 8},
		{"odd height", 8, 7},
		{"both odd", 7, 7},
		{"small odd", 3, 3},
		{"1x2", 1, 2},
		{"2x1", 2, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := NewImageBuf(tt.width, tt.height, FormatRGBA8)
			if err != nil {
				t.Fatalf("NewImageBuf() error = %v", err)
			}

			// Fill with white
			src.Fill(255, 255, 255, 255)

			dst := downsample(src)
			if dst == nil {
				t.Fatal("downsample() returned nil")
			}
			defer PutToDefault(dst)

			// Check dimensions
			wantW := max(1, tt.width/2)
			wantH := max(1, tt.height/2)
			gotW, gotH := dst.Bounds()

			if gotW != wantW || gotH != wantH {
				t.Errorf("downsample() size = %dx%d, want %dx%d",
					gotW, gotH, wantW, wantH)
			}

			// Verify all pixels are white (averaging white gives white)
			for y := 0; y < gotH; y++ {
				for x := 0; x < gotW; x++ {
					r, g, b, a := dst.GetRGBA(x, y)
					if r != 255 || g != 255 || b != 255 || a != 255 {
						t.Errorf("pixel (%d,%d) = (%d,%d,%d,%d), want (255,255,255,255)",
							x, y, r, g, b, a)
					}
				}
			}
		})
	}
}

func TestDownsample_AveragesCorrectly(t *testing.T) {
	// Create a 2x2 image with known values
	src, err := NewImageBuf(2, 2, FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf() error = %v", err)
	}

	// Set up a pattern:
	// (0,0) = black,  (1,0) = red
	// (0,1) = green,  (1,1) = blue
	_ = src.SetRGBA(0, 0, 0, 0, 0, 255)       // black
	_ = src.SetRGBA(1, 0, 255, 0, 0, 255)     // red
	_ = src.SetRGBA(0, 1, 0, 255, 0, 255)     // green
	_ = src.SetRGBA(1, 1, 0, 0, 255, 255)     // blue

	dst := downsample(src)
	if dst == nil {
		t.Fatal("downsample() returned nil")
	}
	defer PutToDefault(dst)

	// Result should be 1x1 with average of all 4 pixels
	gotW, gotH := dst.Bounds()
	if gotW != 1 || gotH != 1 {
		t.Fatalf("downsample() size = %dx%d, want 1x1", gotW, gotH)
	}

	r, g, b, a := dst.GetRGBA(0, 0)

	// Average: R=(0+255+0+0)/4=63, G=(0+0+255+0)/4=63, B=(0+0+0+255)/4=63
	wantR, wantG, wantB, wantA := uint8(63), uint8(63), uint8(63), uint8(255)

	if r != wantR || g != wantG || b != wantB || a != wantA {
		t.Errorf("downsample() pixel = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
			r, g, b, a, wantR, wantG, wantB, wantA)
	}
}

func TestMipmapChain_NumLevels_Nil(t *testing.T) {
	var chain *MipmapChain
	if got := chain.NumLevels(); got != 0 {
		t.Errorf("nil chain NumLevels() = %v, want 0", got)
	}
}

func TestMipmapChain_LevelForScale_Nil(t *testing.T) {
	var chain *MipmapChain
	if got := chain.LevelForScale(0.5); got != nil {
		t.Errorf("nil chain LevelForScale() = %v, want nil", got)
	}
}

// Benchmarks

func BenchmarkGenerateMipmaps(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"64x64", 64},
		{"256x256", 256},
		{"512x512", 512},
		{"1024x1024", 1024},
	}

	for _, sz := range sizes {
		b.Run(sz.name, func(b *testing.B) {
			src, err := NewImageBuf(sz.size, sz.size, FormatRGBA8)
			if err != nil {
				b.Fatalf("NewImageBuf() error = %v", err)
			}

			// Fill with gradient
			for y := 0; y < sz.size; y++ {
				for x := 0; x < sz.size; x++ {
					r := uint8((x * 255) / sz.size)
					g := uint8((y * 255) / sz.size)
					_ = src.SetRGBA(x, y, r, g, 128, 255)
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				chain := GenerateMipmaps(src)
				chain.Release()
			}
		})
	}
}

func BenchmarkDownsample(b *testing.B) {
	src, err := NewImageBuf(512, 512, FormatRGBA8)
	if err != nil {
		b.Fatalf("NewImageBuf() error = %v", err)
	}

	src.Fill(128, 128, 128, 255)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst := downsample(src)
		PutToDefault(dst)
	}
}

func BenchmarkLevelForScale(b *testing.B) {
	src, err := NewImageBuf(512, 512, FormatRGBA8)
	if err != nil {
		b.Fatalf("NewImageBuf() error = %v", err)
	}

	chain := GenerateMipmaps(src)
	defer chain.Release()

	scales := []float64{1.0, 0.75, 0.5, 0.25, 0.125}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scale := scales[i%len(scales)]
		_ = chain.LevelForScale(scale)
	}
}

// Helper test to verify the mipmap level calculation formula
func TestMipmapLevelCalculation(t *testing.T) {
	tests := []struct {
		scale     float64
		wantLevel int
	}{
		{2.0, 0},
		{1.0, 0},
		{0.9, 0},
		{0.5, 1},
		{0.4, 1},
		{0.25, 2},
		{0.2, 2},
		{0.125, 3},
		{0.1, 3},
		{0.0625, 4},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			var level int
			if tt.scale >= 1.0 {
				level = 0
			} else {
				level = int(math.Floor(-math.Log2(tt.scale)))
			}

			if level != tt.wantLevel {
				t.Errorf("scale %v: level = %d, want %d",
					tt.scale, level, tt.wantLevel)
			}
		})
	}
}
