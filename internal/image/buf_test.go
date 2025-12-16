package image

import (
	"errors"
	"testing"
)

func TestNewImageBuf(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		height  int
		format  Format
		wantErr error
	}{
		{"valid RGBA8", 100, 100, FormatRGBA8, nil},
		{"valid Gray8", 50, 50, FormatGray8, nil},
		{"1x1 minimum", 1, 1, FormatRGBA8, nil},
		{"zero width", 0, 100, FormatRGBA8, ErrInvalidDimensions},
		{"zero height", 100, 0, FormatRGBA8, ErrInvalidDimensions},
		{"negative width", -1, 100, FormatRGBA8, ErrInvalidDimensions},
		{"negative height", 100, -1, FormatRGBA8, ErrInvalidDimensions},
		{"invalid format", 100, 100, Format(255), ErrInvalidFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, err := NewImageBuf(tt.width, tt.height, tt.format)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("NewImageBuf() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if buf.Width() != tt.width {
				t.Errorf("Width() = %d, want %d", buf.Width(), tt.width)
			}
			if buf.Height() != tt.height {
				t.Errorf("Height() = %d, want %d", buf.Height(), tt.height)
			}
			if buf.Format() != tt.format {
				t.Errorf("Format() = %v, want %v", buf.Format(), tt.format)
			}
			expectedStride := tt.format.RowBytes(tt.width)
			if buf.Stride() != expectedStride {
				t.Errorf("Stride() = %d, want %d", buf.Stride(), expectedStride)
			}
			expectedSize := expectedStride * tt.height
			if len(buf.Data()) != expectedSize {
				t.Errorf("len(Data()) = %d, want %d", len(buf.Data()), expectedSize)
			}
		})
	}
}

func TestNewImageBufWithStride(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		height  int
		format  Format
		stride  int
		wantErr error
	}{
		{"valid aligned stride", 100, 100, FormatRGBA8, 512, nil},
		{"minimum stride", 100, 100, FormatRGBA8, 400, nil},
		{"stride too small", 100, 100, FormatRGBA8, 300, ErrInvalidStride},
		{"zero stride", 100, 100, FormatRGBA8, 0, ErrInvalidStride},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, err := NewImageBufWithStride(tt.width, tt.height, tt.format, tt.stride)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("NewImageBufWithStride() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && buf.Stride() != tt.stride {
				t.Errorf("Stride() = %d, want %d", buf.Stride(), tt.stride)
			}
		})
	}
}

func TestFromRaw(t *testing.T) {
	// Create valid data
	width, height := 10, 10
	format := FormatRGBA8
	stride := format.RowBytes(width)
	validData := make([]byte, stride*height)

	tests := []struct {
		name    string
		data    []byte
		width   int
		height  int
		format  Format
		stride  int
		wantErr error
	}{
		{"valid data", validData, 10, 10, FormatRGBA8, 40, nil},
		{"data too small", make([]byte, 100), 10, 10, FormatRGBA8, 40, ErrDataTooSmall},
		{"invalid dimensions", validData, 0, 10, FormatRGBA8, 40, ErrInvalidDimensions},
		{"stride too small", validData, 10, 10, FormatRGBA8, 20, ErrInvalidStride},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, err := FromRaw(tt.data, tt.width, tt.height, tt.format, tt.stride)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("FromRaw() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && buf == nil {
				t.Error("FromRaw() returned nil without error")
			}
		})
	}
}

func TestImageBuf_Clone(t *testing.T) {
	original, err := NewImageBuf(10, 10, FormatRGBA8)
	if err != nil {
		t.Fatalf("Failed to create original: %v", err)
	}

	// Set some pixel data
	_ = original.SetRGBA(5, 5, 255, 128, 64, 200)

	clone := original.Clone()

	// Check dimensions match
	if clone.Width() != original.Width() || clone.Height() != original.Height() {
		t.Error("Clone dimensions don't match")
	}

	// Check data is copied, not shared
	if &clone.Data()[0] == &original.Data()[0] {
		t.Error("Clone shares data with original")
	}

	// Check pixel data is the same
	r1, g1, b1, a1 := original.GetRGBA(5, 5)
	r2, g2, b2, a2 := clone.GetRGBA(5, 5)
	if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
		t.Error("Clone pixel data doesn't match original")
	}

	// Modify clone and verify original is unchanged
	_ = clone.SetRGBA(5, 5, 0, 0, 0, 0)
	r1, g1, b1, a1 = original.GetRGBA(5, 5)
	if r1 != 255 || g1 != 128 || b1 != 64 || a1 != 200 {
		t.Error("Modifying clone affected original")
	}
}

func TestImageBuf_Bounds(t *testing.T) {
	buf, _ := NewImageBuf(100, 50, FormatRGBA8)
	w, h := buf.Bounds()
	if w != 100 || h != 50 {
		t.Errorf("Bounds() = (%d, %d), want (100, 50)", w, h)
	}
}

func TestImageBuf_RowBytes(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)

	// Valid row
	row := buf.RowBytes(5)
	if len(row) != 40 { // 10 * 4 bytes per pixel
		t.Errorf("RowBytes(5) length = %d, want 40", len(row))
	}

	// Out of bounds
	if buf.RowBytes(-1) != nil {
		t.Error("RowBytes(-1) should return nil")
	}
	if buf.RowBytes(10) != nil {
		t.Error("RowBytes(10) should return nil")
	}
}

func TestImageBuf_PixelOffset(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)

	tests := []struct {
		x, y   int
		expect int
	}{
		{0, 0, 0},
		{1, 0, 4},
		{0, 1, 40},
		{5, 5, 220}, // 5*40 + 5*4 = 200 + 20 = 220
		{-1, 0, -1},
		{10, 0, -1},
		{0, -1, -1},
		{0, 10, -1},
	}

	for _, tt := range tests {
		offset := buf.PixelOffset(tt.x, tt.y)
		if offset != tt.expect {
			t.Errorf("PixelOffset(%d, %d) = %d, want %d", tt.x, tt.y, offset, tt.expect)
		}
	}
}

func TestImageBuf_PixelBytes(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)

	// Set a pixel
	buf.Data()[0] = 255
	buf.Data()[1] = 128
	buf.Data()[2] = 64
	buf.Data()[3] = 32

	pixel := buf.PixelBytes(0, 0)
	if len(pixel) != 4 {
		t.Errorf("PixelBytes length = %d, want 4", len(pixel))
	}
	if pixel[0] != 255 || pixel[1] != 128 || pixel[2] != 64 || pixel[3] != 32 {
		t.Error("PixelBytes returned wrong data")
	}

	// Out of bounds
	if buf.PixelBytes(-1, 0) != nil {
		t.Error("PixelBytes(-1, 0) should return nil")
	}
}

func TestImageBuf_GetSetRGBA_RGBA8(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)

	// Set and get
	err := buf.SetRGBA(5, 5, 200, 150, 100, 50)
	if err != nil {
		t.Fatalf("SetRGBA failed: %v", err)
	}

	r, g, b, a := buf.GetRGBA(5, 5)
	if r != 200 || g != 150 || b != 100 || a != 50 {
		t.Errorf("GetRGBA = (%d, %d, %d, %d), want (200, 150, 100, 50)", r, g, b, a)
	}

	// Out of bounds set
	err = buf.SetRGBA(-1, 0, 0, 0, 0, 0)
	if !errors.Is(err, ErrOutOfBounds) {
		t.Error("SetRGBA with invalid coords should return ErrOutOfBounds")
	}

	// Out of bounds get
	r, g, b, a = buf.GetRGBA(-1, 0)
	if r != 0 || g != 0 || b != 0 || a != 0 {
		t.Error("GetRGBA with invalid coords should return (0,0,0,0)")
	}
}

func TestImageBuf_GetSetRGBA_BGRA8(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatBGRA8)

	// Set and get - should handle BGRA conversion
	err := buf.SetRGBA(5, 5, 200, 150, 100, 50)
	if err != nil {
		t.Fatalf("SetRGBA failed: %v", err)
	}

	r, g, b, a := buf.GetRGBA(5, 5)
	if r != 200 || g != 150 || b != 100 || a != 50 {
		t.Errorf("GetRGBA = (%d, %d, %d, %d), want (200, 150, 100, 50)", r, g, b, a)
	}

	// Check actual memory layout is BGRA
	pixel := buf.PixelBytes(5, 5)
	if pixel[0] != 100 || pixel[1] != 150 || pixel[2] != 200 || pixel[3] != 50 {
		t.Errorf("BGRA layout = (%d, %d, %d, %d), want (100, 150, 200, 50)",
			pixel[0], pixel[1], pixel[2], pixel[3])
	}
}

func TestImageBuf_GetSetRGBA_Gray8(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatGray8)

	// Set RGB - should convert to grayscale
	_ = buf.SetRGBA(0, 0, 200, 100, 50, 255)

	// Get should return gray value in all channels
	r, g, b, a := buf.GetRGBA(0, 0)
	if r != g || g != b {
		t.Errorf("Gray8 should have equal RGB, got (%d, %d, %d)", r, g, b)
	}
	if a != 255 {
		t.Errorf("Gray8 alpha should be 255, got %d", a)
	}

	// Verify luminance calculation: 0.299*200 + 0.587*100 + 0.114*50 = 59.8 + 58.7 + 5.7 = 124.2 ≈ 124
	expected := uint8((200*299 + 100*587 + 50*114) / 1000)
	if r != expected {
		t.Errorf("Gray8 luminance = %d, want %d", r, expected)
	}
}

func TestImageBuf_GetSetRGBA_RGB8(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGB8)

	_ = buf.SetRGBA(0, 0, 200, 100, 50, 128)

	r, g, b, a := buf.GetRGBA(0, 0)
	if r != 200 || g != 100 || b != 50 {
		t.Errorf("RGB8 = (%d, %d, %d), want (200, 100, 50)", r, g, b)
	}
	if a != 255 {
		t.Errorf("RGB8 alpha should be 255, got %d", a)
	}
}

func TestImageBuf_Clear(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)

	// Set some data
	buf.Fill(255, 255, 255, 255)

	// Clear
	buf.Clear()

	// All pixels should be zero
	for i := range buf.Data() {
		if buf.Data()[i] != 0 {
			t.Fatalf("Clear() didn't zero byte at index %d", i)
		}
	}
}

func TestImageBuf_Fill(t *testing.T) {
	buf, _ := NewImageBuf(5, 5, FormatRGBA8)

	buf.Fill(100, 150, 200, 250)

	for y := range 5 {
		for x := range 5 {
			r, g, b, a := buf.GetRGBA(x, y)
			if r != 100 || g != 150 || b != 200 || a != 250 {
				t.Errorf("Fill: pixel (%d,%d) = (%d,%d,%d,%d), want (100,150,200,250)",
					x, y, r, g, b, a)
			}
		}
	}
}

func TestImageBuf_PremultipliedData_NoPremul(t *testing.T) {
	// Test formats that don't need premultiplication
	tests := []Format{FormatGray8, FormatRGB8, FormatRGBAPremul, FormatBGRAPremul}

	for _, format := range tests {
		buf, _ := NewImageBuf(10, 10, format)
		premul := buf.PremultipliedData()

		// Should return the same data slice
		if &premul[0] != &buf.Data()[0] {
			t.Errorf("%s: PremultipliedData should return original data", format)
		}
	}
}

func TestImageBuf_PremultipliedData_RGBA8(t *testing.T) {
	buf, _ := NewImageBuf(2, 2, FormatRGBA8)

	// Set a pixel with 50% alpha
	_ = buf.SetRGBA(0, 0, 200, 100, 50, 128)

	// Get premultiplied data
	premul := buf.PremultipliedData()

	// Check it's different from original
	if &premul[0] == &buf.Data()[0] {
		t.Error("RGBA8 PremultipliedData should return different slice")
	}

	// Verify premultiplication: channel * alpha / 255
	// R: 200 * 128 / 255 ≈ 100
	// G: 100 * 128 / 255 ≈ 50
	// B: 50 * 128 / 255 ≈ 25
	// A: 128
	expectedR := uint8((200*128 + 127) / 255)
	expectedG := uint8((100*128 + 127) / 255)
	expectedB := uint8((50*128 + 127) / 255)

	if premul[0] != expectedR || premul[1] != expectedG || premul[2] != expectedB || premul[3] != 128 {
		t.Errorf("Premul = (%d, %d, %d, %d), want (%d, %d, %d, 128)",
			premul[0], premul[1], premul[2], premul[3],
			expectedR, expectedG, expectedB)
	}
}

func TestImageBuf_PremulCache(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)
	_ = buf.SetRGBA(0, 0, 200, 100, 50, 128)

	// Initially not cached
	if buf.IsPremulCached() {
		t.Error("Premul should not be cached initially")
	}

	// Get premul - should cache
	_ = buf.PremultipliedData()
	if !buf.IsPremulCached() {
		t.Error("Premul should be cached after PremultipliedData()")
	}

	// Invalidate cache
	buf.InvalidatePremulCache()
	if buf.IsPremulCached() {
		t.Error("Premul should not be cached after InvalidatePremulCache()")
	}
}

func TestImageBuf_SubImage(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)

	// Fill with pattern
	for y := range 10 {
		for x := range 10 {
			_ = buf.SetRGBA(x, y, uint8(x*25), uint8(y*25), 0, 255)
		}
	}

	// Create subimage
	sub := buf.SubImage(2, 2, 5, 5)
	if sub == nil {
		t.Fatal("SubImage returned nil")
	}

	// Check dimensions
	if sub.Width() != 5 || sub.Height() != 5 {
		t.Errorf("SubImage dimensions = (%d, %d), want (5, 5)", sub.Width(), sub.Height())
	}

	// Check that subimage accesses correct pixels
	// SubImage (2,2) corresponds to original (2,2) which has R=50, G=50
	r, g, _, _ := sub.GetRGBA(0, 0)
	if r != 50 || g != 50 {
		t.Errorf("SubImage pixel (0,0) = (%d, %d, _, _), want (50, 50, _, _)", r, g)
	}

	// Verify data sharing - modify sub and check original
	_ = sub.SetRGBA(0, 0, 255, 255, 255, 255)
	r, g, b, a := buf.GetRGBA(2, 2)
	if r != 255 || g != 255 || b != 255 || a != 255 {
		t.Error("SubImage modification didn't affect original")
	}
}

func TestImageBuf_SubImage_Invalid(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)

	tests := []struct {
		name                string
		x, y, width, height int
	}{
		{"negative x", -1, 0, 5, 5},
		{"negative y", 0, -1, 5, 5},
		{"zero width", 0, 0, 0, 5},
		{"zero height", 0, 0, 5, 0},
		{"exceeds right", 8, 0, 5, 5},
		{"exceeds bottom", 0, 8, 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := buf.SubImage(tt.x, tt.y, tt.width, tt.height)
			if sub != nil {
				t.Errorf("SubImage(%d, %d, %d, %d) should return nil",
					tt.x, tt.y, tt.width, tt.height)
			}
		})
	}
}

func TestImageBuf_ByteSize(t *testing.T) {
	buf, _ := NewImageBuf(100, 100, FormatRGBA8)
	expected := 100 * 100 * 4
	if buf.ByteSize() != expected {
		t.Errorf("ByteSize() = %d, want %d", buf.ByteSize(), expected)
	}
}

func TestImageBuf_IsEmpty(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)
	if buf.IsEmpty() {
		t.Error("10x10 image should not be empty")
	}

	// Create edge case with minimal dimensions
	buf2, _ := NewImageBuf(1, 1, FormatRGBA8)
	if buf2.IsEmpty() {
		t.Error("1x1 image should not be empty")
	}
}

func TestImageBuf_SetPixelBytes(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)

	pixel := []byte{100, 150, 200, 250}
	err := buf.SetPixelBytes(5, 5, pixel)
	if err != nil {
		t.Fatalf("SetPixelBytes failed: %v", err)
	}

	got := buf.PixelBytes(5, 5)
	for i, v := range got {
		if v != pixel[i] {
			t.Errorf("SetPixelBytes: byte %d = %d, want %d", i, v, pixel[i])
		}
	}

	// Out of bounds
	err = buf.SetPixelBytes(-1, 0, pixel)
	if !errors.Is(err, ErrOutOfBounds) {
		t.Error("SetPixelBytes with invalid coords should return ErrOutOfBounds")
	}
}

func TestImageBuf_Gray16(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatGray16)

	// Set a gray value
	_ = buf.SetRGBA(0, 0, 200, 200, 200, 255)

	// Get should return the value
	r, g, b, a := buf.GetRGBA(0, 0)
	if r != g || g != b {
		t.Errorf("Gray16 RGB should be equal, got (%d, %d, %d)", r, g, b)
	}
	if a != 255 {
		t.Errorf("Gray16 alpha should be 255, got %d", a)
	}
}

func BenchmarkNewImageBuf(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = NewImageBuf(1920, 1080, FormatRGBA8)
	}
}

func BenchmarkImageBuf_GetRGBA(b *testing.B) {
	buf, _ := NewImageBuf(1920, 1080, FormatRGBA8)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, _, _ = buf.GetRGBA(i%1920, (i/1920)%1080)
	}
}

func BenchmarkImageBuf_SetRGBA(b *testing.B) {
	buf, _ := NewImageBuf(1920, 1080, FormatRGBA8)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = buf.SetRGBA(i%1920, (i/1920)%1080, 128, 128, 128, 255)
	}
}

func BenchmarkImageBuf_PremultipliedData(b *testing.B) {
	buf, _ := NewImageBuf(1920, 1080, FormatRGBA8)
	buf.Fill(200, 100, 50, 128)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf.InvalidatePremulCache()
		_ = buf.PremultipliedData()
	}
}

func BenchmarkImageBuf_Clone(b *testing.B) {
	buf, _ := NewImageBuf(1920, 1080, FormatRGBA8)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = buf.Clone()
	}
}
