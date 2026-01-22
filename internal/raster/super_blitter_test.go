package raster

import (
	"testing"
)

// mockAAPixmap implements AAPixmap for testing.
type mockAAPixmap struct {
	width  int
	height int
	pixels map[int]map[int]RGBA
	alphas map[int]map[int]uint8
}

func newMockAAPixmap(w, h int) *mockAAPixmap {
	return &mockAAPixmap{
		width:  w,
		height: h,
		pixels: make(map[int]map[int]RGBA),
		alphas: make(map[int]map[int]uint8),
	}
}

func (m *mockAAPixmap) Width() int  { return m.width }
func (m *mockAAPixmap) Height() int { return m.height }

func (m *mockAAPixmap) SetPixel(x, y int, c RGBA) {
	if m.pixels[y] == nil {
		m.pixels[y] = make(map[int]RGBA)
	}
	m.pixels[y][x] = c
}

func (m *mockAAPixmap) BlendPixelAlpha(x, y int, c RGBA, alpha uint8) {
	if m.pixels[y] == nil {
		m.pixels[y] = make(map[int]RGBA)
	}
	if m.alphas[y] == nil {
		m.alphas[y] = make(map[int]uint8)
	}
	m.pixels[y][x] = c
	m.alphas[y][x] = alpha
}

func TestSupersampleConstants(t *testing.T) {
	// Verify constants match tiny-skia's 4x supersampling
	if SupersampleShift != 2 {
		t.Errorf("SupersampleShift = %d, want 2", SupersampleShift)
	}
	if SupersampleScale != 4 {
		t.Errorf("SupersampleScale = %d, want 4", SupersampleScale)
	}
	if SupersampleMask != 3 {
		t.Errorf("SupersampleMask = %d, want 3", SupersampleMask)
	}
}

func TestNewSuperBlitter(t *testing.T) {
	tests := []struct {
		name                                             string
		boundsLeft, boundsTop, boundsRight, boundsBottom int
		clipLeft, clipTop, clipRight, clipBottom         int
		expectNil                                        bool
	}{
		{
			name:       "valid bounds within clip",
			boundsLeft: 10, boundsTop: 10, boundsRight: 50, boundsBottom: 50,
			clipLeft: 0, clipTop: 0, clipRight: 100, clipBottom: 100,
			expectNil: false,
		},
		{
			name:       "bounds outside clip",
			boundsLeft: 200, boundsTop: 200, boundsRight: 300, boundsBottom: 300,
			clipLeft: 0, clipTop: 0, clipRight: 100, clipBottom: 100,
			expectNil: true,
		},
		{
			name:       "partial overlap",
			boundsLeft: 80, boundsTop: 80, boundsRight: 120, boundsBottom: 120,
			clipLeft: 0, clipTop: 0, clipRight: 100, clipBottom: 100,
			expectNil: false,
		},
		{
			name:       "zero width after clipping",
			boundsLeft: 100, boundsTop: 10, boundsRight: 100, boundsBottom: 50,
			clipLeft: 0, clipTop: 0, clipRight: 100, clipBottom: 100,
			expectNil: true,
		},
		{
			name:       "zero height after clipping",
			boundsLeft: 10, boundsTop: 100, boundsRight: 50, boundsBottom: 100,
			clipLeft: 0, clipTop: 0, clipRight: 100, clipBottom: 100,
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pixmap := newMockAAPixmap(tt.clipRight, tt.clipBottom)
			color := RGBA{R: 1.0, G: 0.0, B: 0.0, A: 1.0}

			sb := NewSuperBlitter(
				pixmap, color,
				tt.boundsLeft, tt.boundsTop, tt.boundsRight, tt.boundsBottom,
				tt.clipLeft, tt.clipTop, tt.clipRight, tt.clipBottom,
			)

			if tt.expectNil && sb != nil {
				t.Error("expected nil SuperBlitter")
			}
			if !tt.expectNil && sb == nil {
				t.Error("expected non-nil SuperBlitter")
			}
		})
	}
}

func TestSuperBlitterBlitH(t *testing.T) {
	pixmap := newMockAAPixmap(100, 100)
	color := RGBA{R: 1.0, G: 0.0, B: 0.0, A: 1.0}

	sb := NewSuperBlitter(
		pixmap, color,
		0, 0, 100, 100,
		0, 0, 100, 100,
	)
	if sb == nil {
		t.Fatal("SuperBlitter is nil")
	}

	// Blit a span at supersampled coordinates
	// x=40 in supersampled = pixel 10
	// y=40 in supersampled = pixel 10
	sb.BlitH(40, 40, 20) // 5 pixels in normal coords

	// Flush to write to pixmap
	sb.Flush()

	// Check that pixels were written
	hasPixels := false
	for y := range pixmap.pixels {
		if len(pixmap.pixels[y]) > 0 {
			hasPixels = true
			break
		}
	}
	if !hasPixels {
		t.Error("expected pixels to be written after BlitH and Flush")
	}
}

func TestSuperBlitterFlush(t *testing.T) {
	pixmap := newMockAAPixmap(50, 50)
	color := RGBA{R: 0.0, G: 1.0, B: 0.0, A: 1.0}

	sb := NewSuperBlitter(
		pixmap, color,
		10, 10, 40, 40,
		0, 0, 50, 50,
	)
	if sb == nil {
		t.Fatal("SuperBlitter is nil")
	}

	// Multiple blits on same pixel row
	superY := uint32(44) // pixel row 11
	sb.BlitH(44, superY, 8)
	sb.BlitH(60, superY, 12)

	// Flush should write accumulated coverage
	sb.Flush()

	// Should have pixels written
	if len(pixmap.pixels) == 0 {
		t.Error("expected pixels after Flush")
	}
}

func TestSuperBlitterRowTransition(t *testing.T) {
	pixmap := newMockAAPixmap(100, 100)
	color := RGBA{R: 0.0, G: 0.0, B: 1.0, A: 1.0}

	sb := NewSuperBlitter(
		pixmap, color,
		0, 0, 100, 100,
		0, 0, 100, 100,
	)
	if sb == nil {
		t.Fatal("SuperBlitter is nil")
	}

	// Blit on row 0 (supersampled row 0-3 -> pixel row 0)
	sb.BlitH(40, 0, 20)
	sb.BlitH(40, 1, 20)
	sb.BlitH(40, 2, 20)
	sb.BlitH(40, 3, 20)

	// Move to row 1 (should auto-flush row 0)
	sb.BlitH(40, 4, 20)

	// Final flush
	sb.Flush()

	// Should have pixels on multiple rows
	if len(pixmap.pixels) == 0 {
		t.Error("expected pixels to be written")
	}
}

func TestSuperBlitterEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		x     uint32
		y     uint32
		width int
	}{
		{"zero width", 40, 40, 0},
		{"negative width", 40, 40, -5},
		{"very wide span", 0, 40, 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pixmap := newMockAAPixmap(100, 100)
			color := RGBA{R: 1.0, G: 1.0, B: 1.0, A: 1.0}

			sb := NewSuperBlitter(
				pixmap, color,
				0, 0, 100, 100,
				0, 0, 100, 100,
			)
			if sb == nil {
				t.Skip("SuperBlitter is nil")
			}

			// Should not panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("BlitH panicked: %v", r)
				}
			}()

			sb.BlitH(tt.x, tt.y, tt.width)
			sb.Flush()
		})
	}
}

func TestSuperBlitterColorPreservation(t *testing.T) {
	pixmap := newMockAAPixmap(100, 100)
	color := RGBA{R: 0.5, G: 0.25, B: 0.75, A: 0.8}

	sb := NewSuperBlitter(
		pixmap, color,
		10, 10, 50, 50,
		0, 0, 100, 100,
	)
	if sb == nil {
		t.Fatal("SuperBlitter is nil")
	}

	// Blit a full pixel (4x4 supersampled coverage)
	for subY := uint32(40); subY < 44; subY++ {
		sb.BlitH(40, subY, 16)
	}
	sb.Flush()

	// Check that the color was preserved
	for y := range pixmap.pixels {
		for x, c := range pixmap.pixels[y] {
			if c.R != color.R || c.G != color.G || c.B != color.B {
				t.Errorf("color at (%d,%d) = %v, want %v", x, y, c, color)
			}
		}
	}
}

func TestCoverageToPartialAlpha(t *testing.T) {
	tests := []struct {
		coverage uint32
	}{
		{0},
		{1},
		{2},
		{3},
		{4}, // full coverage for 4x supersample
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			alpha := coverageToPartialAlpha(tt.coverage)
			// Just verify it returns something in valid uint8 range (0-255 by type)
			_ = alpha
		})
	}
}

func BenchmarkSuperBlitterBlitH(b *testing.B) {
	pixmap := newMockAAPixmap(1000, 1000)
	color := RGBA{R: 1.0, G: 0.0, B: 0.0, A: 1.0}

	sb := NewSuperBlitter(
		pixmap, color,
		0, 0, 1000, 1000,
		0, 0, 1000, 1000,
	)
	if sb == nil {
		b.Fatal("SuperBlitter is nil")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sb.BlitH(uint32((i*4)%4000), uint32((i/1000)%4000), 100)
		if i%1000 == 999 {
			sb.Flush()
		}
	}
}

func BenchmarkSuperBlitterFullRow(b *testing.B) {
	pixmap := newMockAAPixmap(500, 500)
	color := RGBA{R: 1.0, G: 0.0, B: 0.0, A: 1.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sb := NewSuperBlitter(
			pixmap, color,
			0, 0, 500, 500,
			0, 0, 500, 500,
		)
		if sb == nil {
			continue
		}

		// Fill one row with 4 supersampled scanlines
		for y := uint32(0); y < 4; y++ {
			sb.BlitH(0, y, 2000)
		}
		sb.Flush()
	}
}
