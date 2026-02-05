package image

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestFromStdImage_RGBA(t *testing.T) {
	// Create a standard RGBA image
	rgba := image.NewRGBA(image.Rect(0, 0, 10, 10))
	rgba.Set(5, 5, color.RGBA{R: 200, G: 100, B: 50, A: 255})

	buf := FromStdImage(rgba)

	if buf.Width() != 10 || buf.Height() != 10 {
		t.Errorf("Dimensions = (%d, %d), want (10, 10)", buf.Width(), buf.Height())
	}

	r, g, b, a := buf.GetRGBA(5, 5)
	if r != 200 || g != 100 || b != 50 || a != 255 {
		t.Errorf("Pixel = (%d, %d, %d, %d), want (200, 100, 50, 255)", r, g, b, a)
	}
}

func TestFromStdImage_NRGBA(t *testing.T) {
	nrgba := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	nrgba.Set(3, 3, color.NRGBA{R: 128, G: 64, B: 32, A: 200})

	buf := FromStdImage(nrgba)

	if buf.Width() != 10 || buf.Height() != 10 {
		t.Errorf("Dimensions = (%d, %d), want (10, 10)", buf.Width(), buf.Height())
	}

	r, g, b, a := buf.GetRGBA(3, 3)
	if r != 128 || g != 64 || b != 32 || a != 200 {
		t.Errorf("Pixel = (%d, %d, %d, %d), want (128, 64, 32, 200)", r, g, b, a)
	}
}

func TestFromStdImage_Gray(t *testing.T) {
	gray := image.NewGray(image.Rect(0, 0, 10, 10))
	gray.SetGray(5, 5, color.Gray{Y: 128})

	buf := FromStdImage(gray)

	if buf.Format() != FormatRGBA8 {
		t.Logf("Format = %v (converted from Gray)", buf.Format())
	}

	r, g, b, a := buf.GetRGBA(5, 5)
	if r != 128 || g != 128 || b != 128 || a != 255 {
		t.Errorf("Pixel = (%d, %d, %d, %d), want (128, 128, 128, 255)", r, g, b, a)
	}
}

func TestToStdImage_RGBA8(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)
	_ = buf.SetRGBA(5, 5, 200, 100, 50, 255)

	img := buf.ToStdImage()

	// RGBA8 is non-premultiplied, so should return NRGBA
	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		t.Fatalf("ToStdImage() returned %T, want *image.NRGBA", img)
	}

	c := nrgba.NRGBAAt(5, 5)
	if c.R != 200 || c.G != 100 || c.B != 50 || c.A != 255 {
		t.Errorf("Pixel = %v, want {200, 100, 50, 255}", c)
	}
}

func TestToStdImage_Gray8(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatGray8)
	buf.Data()[5*10+5] = 128 // Direct set gray value

	img := buf.ToStdImage()

	gray, ok := img.(*image.Gray)
	if !ok {
		t.Fatalf("ToStdImage() returned %T, want *image.Gray", img)
	}

	c := gray.GrayAt(5, 5)
	if c.Y != 128 {
		t.Errorf("Pixel = %d, want 128", c.Y)
	}
}

func TestToStdImage_BGRA8(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatBGRA8)
	_ = buf.SetRGBA(5, 5, 200, 100, 50, 255)

	img := buf.ToStdImage()

	// BGRA8 is non-premultiplied, so should return NRGBA
	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		t.Fatalf("ToStdImage() returned %T, want *image.NRGBA", img)
	}

	c := nrgba.NRGBAAt(5, 5)
	if c.R != 200 || c.G != 100 || c.B != 50 || c.A != 255 {
		t.Errorf("Pixel = %v, want {200, 100, 50, 255}", c)
	}
}

func TestToStdImage_RGB8(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGB8)
	_ = buf.SetRGBA(5, 5, 200, 100, 50, 255)

	img := buf.ToStdImage()

	// RGB8 expands to NRGBA (opaque)
	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		t.Fatalf("ToStdImage() returned %T, want *image.NRGBA", img)
	}

	c := nrgba.NRGBAAt(5, 5)
	if c.R != 200 || c.G != 100 || c.B != 50 || c.A != 255 {
		t.Errorf("Pixel = %v, want {200, 100, 50, 255}", c)
	}
}

func TestEncodePNG_DecodePNG(t *testing.T) {
	// Create an image with known data
	buf, _ := NewImageBuf(32, 32, FormatRGBA8)
	for y := range 32 {
		for x := range 32 {
			_ = buf.SetRGBA(x, y, uint8(x*8), uint8(y*8), 128, 255)
		}
	}

	// Encode to PNG
	var encoded bytes.Buffer
	if err := buf.EncodePNG(&encoded); err != nil {
		t.Fatalf("EncodePNG failed: %v", err)
	}

	// Decode back
	decoded, err := DecodePNG(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("DecodePNG failed: %v", err)
	}

	// Verify dimensions
	if decoded.Width() != 32 || decoded.Height() != 32 {
		t.Errorf("Dimensions = (%d, %d), want (32, 32)", decoded.Width(), decoded.Height())
	}

	// Verify pixel data (sample a few pixels)
	testPixels := [][2]int{{0, 0}, {15, 15}, {31, 31}}
	for _, p := range testPixels {
		origR, origG, origB, origA := buf.GetRGBA(p[0], p[1])
		decR, decG, decB, decA := decoded.GetRGBA(p[0], p[1])
		if origR != decR || origG != decG || origB != decB || origA != decA {
			t.Errorf("Pixel (%d,%d): original=(%d,%d,%d,%d), decoded=(%d,%d,%d,%d)",
				p[0], p[1], origR, origG, origB, origA, decR, decG, decB, decA)
		}
	}
}

func TestEncodeJPEG_DecodeJPEG(t *testing.T) {
	// Create an image
	buf, _ := NewImageBuf(32, 32, FormatRGBA8)
	buf.Fill(100, 150, 200, 255)

	// Encode to JPEG with high quality
	var encoded bytes.Buffer
	if err := buf.EncodeJPEG(&encoded, 95); err != nil {
		t.Fatalf("EncodeJPEG failed: %v", err)
	}

	// Decode back
	decoded, err := DecodeJPEG(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("DecodeJPEG failed: %v", err)
	}

	// Verify dimensions
	if decoded.Width() != 32 || decoded.Height() != 32 {
		t.Errorf("Dimensions = (%d, %d), want (32, 32)", decoded.Width(), decoded.Height())
	}

	// JPEG is lossy, so just check approximate values
	r, g, b, _ := decoded.GetRGBA(16, 16)
	if r < 90 || r > 110 || g < 140 || g > 160 || b < 190 || b > 210 {
		t.Errorf("JPEG pixel too different from original: got (%d,%d,%d)", r, g, b)
	}
}

func TestEncodeJPEG_QualityBounds(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)

	// Quality < 1 should be clamped to 1
	var encoded1 bytes.Buffer
	if err := buf.EncodeJPEG(&encoded1, 0); err != nil {
		t.Fatalf("EncodeJPEG(quality=0) failed: %v", err)
	}

	// Quality > 100 should be clamped to 100
	var encoded2 bytes.Buffer
	if err := buf.EncodeJPEG(&encoded2, 150); err != nil {
		t.Fatalf("EncodeJPEG(quality=150) failed: %v", err)
	}
}

func TestLoadImageFromBytes(t *testing.T) {
	// Create a PNG in memory
	rgba := image.NewRGBA(image.Rect(0, 0, 10, 10))
	rgba.Set(5, 5, color.RGBA{R: 255, G: 0, B: 0, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, rgba); err != nil {
		t.Fatalf("Failed to create test PNG: %v", err)
	}

	// Load from bytes
	loaded, err := LoadImageFromBytes(buf.Bytes())
	if err != nil {
		t.Fatalf("LoadImageFromBytes failed: %v", err)
	}

	r, g, b, a := loaded.GetRGBA(5, 5)
	if r != 255 || g != 0 || b != 0 || a != 255 {
		t.Errorf("Pixel = (%d,%d,%d,%d), want (255,0,0,255)", r, g, b, a)
	}
}

func TestLoadImageFromBytes_Empty(t *testing.T) {
	_, err := LoadImageFromBytes(nil)
	if !errors.Is(err, ErrEmptyData) {
		t.Errorf("LoadImageFromBytes(nil) = %v, want ErrEmptyData", err)
	}

	_, err = LoadImageFromBytes([]byte{})
	if !errors.Is(err, ErrEmptyData) {
		t.Errorf("LoadImageFromBytes([]) = %v, want ErrEmptyData", err)
	}
}

func TestEncodeToBytes(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)
	buf.Fill(128, 128, 128, 255)

	data, err := buf.EncodeToBytes()
	if err != nil {
		t.Fatalf("EncodeToBytes failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("EncodeToBytes returned empty data")
	}

	// Verify it's valid PNG by decoding
	_, err = LoadImageFromBytes(data)
	if err != nil {
		t.Errorf("EncodeToBytes produced invalid PNG: %v", err)
	}
}

func TestEncodeToJPEGBytes(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)
	buf.Fill(128, 128, 128, 255)

	data, err := buf.EncodeToJPEGBytes(85)
	if err != nil {
		t.Fatalf("EncodeToJPEGBytes failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("EncodeToJPEGBytes returned empty data")
	}
}

func TestSavePNG_LoadPNG(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "image_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create and save image
	buf, _ := NewImageBuf(20, 20, FormatRGBA8)
	buf.Fill(255, 128, 64, 200)

	path := filepath.Join(tmpDir, "test.png")
	if err := buf.SavePNG(path); err != nil {
		t.Fatalf("SavePNG failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("SavePNG didn't create file")
	}

	// Load and verify
	loaded, err := LoadPNG(path)
	if err != nil {
		t.Fatalf("LoadPNG failed: %v", err)
	}

	if loaded.Width() != 20 || loaded.Height() != 20 {
		t.Errorf("Loaded dimensions = (%d,%d), want (20,20)", loaded.Width(), loaded.Height())
	}

	r, g, b, a := loaded.GetRGBA(10, 10)
	if r != 255 || g != 128 || b != 64 || a != 200 {
		t.Errorf("Loaded pixel = (%d,%d,%d,%d), want (255,128,64,200)", r, g, b, a)
	}
}

func TestSaveJPEG_LoadJPEG(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "image_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	buf, _ := NewImageBuf(20, 20, FormatRGBA8)
	buf.Fill(100, 150, 200, 255)

	path := filepath.Join(tmpDir, "test.jpg")
	if err := buf.SaveJPEG(path, 90); err != nil {
		t.Fatalf("SaveJPEG failed: %v", err)
	}

	loaded, err := LoadJPEG(path)
	if err != nil {
		t.Fatalf("LoadJPEG failed: %v", err)
	}

	if loaded.Width() != 20 || loaded.Height() != 20 {
		t.Errorf("Loaded dimensions = (%d,%d), want (20,20)", loaded.Width(), loaded.Height())
	}
}

func TestLoadImage_AutoDetect(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "image_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create PNG
	buf, _ := NewImageBuf(10, 10, FormatRGBA8)
	pngPath := filepath.Join(tmpDir, "test.png")
	_ = buf.SavePNG(pngPath)

	// Create JPEG
	jpgPath := filepath.Join(tmpDir, "test.jpg")
	_ = buf.SaveJPEG(jpgPath, 80)

	// Test auto-detection
	tests := []string{pngPath, jpgPath}
	for _, path := range tests {
		loaded, err := LoadImage(path)
		if err != nil {
			t.Errorf("LoadImage(%s) failed: %v", path, err)
			continue
		}
		if loaded.Width() != 10 || loaded.Height() != 10 {
			t.Errorf("LoadImage(%s) dimensions wrong", path)
		}
	}
}

func TestLoadPNG_NotFound(t *testing.T) {
	_, err := LoadPNG("/nonexistent/path/image.png")
	if err == nil {
		t.Error("LoadPNG should fail for non-existent file")
	}
}

func TestLoadJPEG_NotFound(t *testing.T) {
	_, err := LoadJPEG("/nonexistent/path/image.jpg")
	if err == nil {
		t.Error("LoadJPEG should fail for non-existent file")
	}
}

func TestDecode_InvalidData(t *testing.T) {
	_, err := Decode(bytes.NewReader([]byte("not an image")))
	if err == nil {
		t.Error("Decode should fail for invalid data")
	}
}

func TestToStdImage_Gray16(t *testing.T) {
	buf, _ := NewImageBuf(10, 10, FormatGray16)
	// Set a gray16 value (stored as little-endian in our format)
	buf.Data()[0] = 0x00 // low byte
	buf.Data()[1] = 0x80 // high byte = 128

	img := buf.ToStdImage()

	gray16, ok := img.(*image.Gray16)
	if !ok {
		t.Fatalf("ToStdImage() returned %T, want *image.Gray16", img)
	}

	// Gray16 uses big-endian, so high byte should come first
	c := gray16.Gray16At(0, 0)
	if c.Y>>8 != 0x80 {
		t.Errorf("Gray16 value = %#x, want high byte 0x80", c.Y)
	}
}

func readTestWebP(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/test.webp")
	if err != nil {
		t.Fatalf("Failed to read testdata/test.webp: %v", err)
	}
	return data
}

func TestDecodeWebP(t *testing.T) {
	data := readTestWebP(t)

	decoded, err := DecodeWebP(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DecodeWebP failed: %v", err)
	}

	if decoded.Width() != 75 || decoded.Height() != 100 {
		t.Errorf("Dimensions = (%d, %d), want (75, 100)", decoded.Width(), decoded.Height())
	}
}

func TestDecodeWebP_InvalidData(t *testing.T) {
	_, err := DecodeWebP(bytes.NewReader([]byte("not a webp image")))
	if err == nil {
		t.Error("DecodeWebP should fail for invalid data")
	}
}

func TestLoadWebP_NotFound(t *testing.T) {
	_, err := LoadWebP("/nonexistent/path/image.webp")
	if err == nil {
		t.Error("LoadWebP should fail for non-existent file")
	}
}

func TestLoadImage_WebP(t *testing.T) {
	loaded, err := LoadImage("testdata/test.webp")
	if err != nil {
		t.Fatalf("LoadImage(.webp) failed: %v", err)
	}

	if loaded.Width() != 75 || loaded.Height() != 100 {
		t.Errorf("Dimensions = (%d, %d), want (75, 100)", loaded.Width(), loaded.Height())
	}
}

func TestWebPViaAutoDetect(t *testing.T) {
	data := readTestWebP(t)

	loaded, err := LoadImageFromBytes(data)
	if err != nil {
		t.Fatalf("LoadImageFromBytes(WebP) failed: %v", err)
	}

	if loaded.Width() != 75 || loaded.Height() != 100 {
		t.Errorf("Dimensions = (%d, %d), want (75, 100)", loaded.Width(), loaded.Height())
	}
}

func BenchmarkFromStdImage_RGBA(b *testing.B) {
	rgba := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = FromStdImage(rgba)
	}
}

func BenchmarkToStdImage_RGBA8(b *testing.B) {
	buf, _ := NewImageBuf(1920, 1080, FormatRGBA8)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = buf.ToStdImage()
	}
}

func BenchmarkEncodePNG(b *testing.B) {
	buf, _ := NewImageBuf(256, 256, FormatRGBA8)
	buf.Fill(128, 128, 128, 255)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var encoded bytes.Buffer
		_ = buf.EncodePNG(&encoded)
	}
}

func BenchmarkDecodePNG(b *testing.B) {
	buf, _ := NewImageBuf(256, 256, FormatRGBA8)
	buf.Fill(128, 128, 128, 255)

	var encoded bytes.Buffer
	_ = buf.EncodePNG(&encoded)
	data := encoded.Bytes()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = DecodePNG(bytes.NewReader(data))
	}
}
