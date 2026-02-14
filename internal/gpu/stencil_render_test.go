//go:build !nogpu

package gpu

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gg"
)

func TestRenderPathTriangle(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	// Build a simple triangle path.
	elements := []gg.PathElement{
		gg.MoveTo{Point: gg.Pt(10, 10)},
		gg.LineTo{Point: gg.Pt(90, 10)},
		gg.LineTo{Point: gg.Pt(50, 90)},
		gg.Close{},
	}

	target := gg.GPURenderTarget{
		Data:   make([]uint8, 100*100*4),
		Width:  100,
		Height: 100,
		Stride: 100 * 4,
	}

	color := gg.RGBA{R: 1, G: 0, B: 0, A: 1}
	err := sr.RenderPath(target, elements, color)
	if err != nil {
		t.Fatalf("RenderPath failed: %v", err)
	}

	// Noop backend returns zeroed readback data, so we verify the code path
	// executed without error rather than checking pixel values.
}

func TestRenderPathEmpty(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	target := gg.GPURenderTarget{
		Data:   make([]uint8, 64*64*4),
		Width:  64,
		Height: 64,
		Stride: 64 * 4,
	}

	// Empty path should return nil (no-op).
	err := sr.RenderPath(target, nil, gg.RGBA{R: 1, G: 0, B: 0, A: 1})
	if err != nil {
		t.Fatalf("RenderPath with nil elements should return nil, got: %v", err)
	}

	// Empty slice should also return nil.
	err = sr.RenderPath(target, []gg.PathElement{}, gg.RGBA{R: 1, G: 0, B: 0, A: 1})
	if err != nil {
		t.Fatalf("RenderPath with empty elements should return nil, got: %v", err)
	}
}

func TestRenderPathCircle(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	// Build a circle path using cubic Beziers (same as gg.Path.Circle).
	const k = 0.5522847498307936
	cx, cy, r := 50.0, 50.0, 30.0
	offset := r * k

	elements := []gg.PathElement{
		gg.MoveTo{Point: gg.Pt(cx+r, cy)},
		gg.CubicTo{Control1: gg.Pt(cx+r, cy+offset), Control2: gg.Pt(cx+offset, cy+r), Point: gg.Pt(cx, cy+r)},
		gg.CubicTo{Control1: gg.Pt(cx-offset, cy+r), Control2: gg.Pt(cx-r, cy+offset), Point: gg.Pt(cx-r, cy)},
		gg.CubicTo{Control1: gg.Pt(cx-r, cy-offset), Control2: gg.Pt(cx-offset, cy-r), Point: gg.Pt(cx, cy-r)},
		gg.CubicTo{Control1: gg.Pt(cx+offset, cy-r), Control2: gg.Pt(cx+r, cy-offset), Point: gg.Pt(cx+r, cy)},
		gg.Close{},
	}

	target := gg.GPURenderTarget{
		Data:   make([]uint8, 100*100*4),
		Width:  100,
		Height: 100,
		Stride: 100 * 4,
	}

	color := gg.RGBA{R: 0, G: 0.5, B: 1, A: 0.8}
	err := sr.RenderPath(target, elements, color)
	if err != nil {
		t.Fatalf("RenderPath circle failed: %v", err)
	}
}

func TestRenderPathPipelinesCreatedOnce(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	elements := []gg.PathElement{
		gg.MoveTo{Point: gg.Pt(0, 0)},
		gg.LineTo{Point: gg.Pt(50, 0)},
		gg.LineTo{Point: gg.Pt(25, 50)},
		gg.Close{},
	}

	target := gg.GPURenderTarget{
		Data:   make([]uint8, 64*64*4),
		Width:  64,
		Height: 64,
		Stride: 64 * 4,
	}
	color := gg.RGBA{R: 1, G: 1, B: 1, A: 1}

	// First call creates pipelines.
	if err := sr.RenderPath(target, elements, color); err != nil {
		t.Fatalf("first RenderPath failed: %v", err)
	}

	// Save pipeline references.
	stencilPipeline := sr.nonZeroStencilPipeline
	coverPipeline := sr.nonZeroCoverPipeline

	// Second call should reuse pipelines.
	if err := sr.RenderPath(target, elements, color); err != nil {
		t.Fatalf("second RenderPath failed: %v", err)
	}

	if sr.nonZeroStencilPipeline != stencilPipeline {
		t.Error("stencil pipeline was recreated unnecessarily")
	}
	if sr.nonZeroCoverPipeline != coverPipeline {
		t.Error("cover pipeline was recreated unnecessarily")
	}
}

func TestRenderPathResizesTextures(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	elements := []gg.PathElement{
		gg.MoveTo{Point: gg.Pt(5, 5)},
		gg.LineTo{Point: gg.Pt(25, 5)},
		gg.LineTo{Point: gg.Pt(15, 25)},
		gg.Close{},
	}
	color := gg.RGBA{R: 1, G: 0, B: 0, A: 1}

	// Render at 64x64.
	target1 := gg.GPURenderTarget{
		Data:   make([]uint8, 64*64*4),
		Width:  64,
		Height: 64,
		Stride: 64 * 4,
	}
	if err := sr.RenderPath(target1, elements, color); err != nil {
		t.Fatalf("RenderPath at 64x64 failed: %v", err)
	}
	w, h := sr.Size()
	if w != 64 || h != 64 {
		t.Errorf("expected size (64, 64), got (%d, %d)", w, h)
	}

	// Render at 128x96 — textures should resize.
	target2 := gg.GPURenderTarget{
		Data:   make([]uint8, 128*96*4),
		Width:  128,
		Height: 96,
		Stride: 128 * 4,
	}
	if err := sr.RenderPath(target2, elements, color); err != nil {
		t.Fatalf("RenderPath at 128x96 failed: %v", err)
	}
	w, h = sr.Size()
	if w != 128 || h != 96 {
		t.Errorf("expected size (128, 96), got (%d, %d)", w, h)
	}
}

func TestMakeStencilFillUniform(t *testing.T) {
	buf := makeStencilFillUniform(800, 600)
	if len(buf) != stencilFillUniformSize {
		t.Fatalf("expected %d bytes, got %d", stencilFillUniformSize, len(buf))
	}
	// Verify viewport values are encoded correctly.
	// Width = 800.0, Height = 600.0
	if got := decodeFloat32(buf[0:4]); got != 800.0 {
		t.Errorf("expected width 800.0, got %v", got)
	}
	if got := decodeFloat32(buf[4:8]); got != 600.0 {
		t.Errorf("expected height 600.0, got %v", got)
	}
}

func TestMakeCoverUniform(t *testing.T) {
	color := gg.RGBA{R: 1.0, G: 0.5, B: 0.25, A: 0.8}
	buf := makeCoverUniform(1920, 1080, color)
	if len(buf) != coverUniformSize {
		t.Fatalf("expected %d bytes, got %d", coverUniformSize, len(buf))
	}
	// Verify viewport.
	if got := decodeFloat32(buf[0:4]); got != 1920.0 {
		t.Errorf("expected width 1920.0, got %v", got)
	}
	if got := decodeFloat32(buf[4:8]); got != 1080.0 {
		t.Errorf("expected height 1080.0, got %v", got)
	}
	// Verify premultiplied color.
	// R = 1.0 * 0.8 = 0.8
	// G = 0.5 * 0.8 = 0.4
	// B = 0.25 * 0.8 = 0.2
	// A = 0.8
	const tolerance = 1e-6
	if got := decodeFloat32(buf[16:20]); testAbs32(got-0.8) > tolerance {
		t.Errorf("expected premul R ~0.8, got %v", got)
	}
	if got := decodeFloat32(buf[20:24]); testAbs32(got-0.4) > tolerance {
		t.Errorf("expected premul G ~0.4, got %v", got)
	}
	if got := decodeFloat32(buf[24:28]); testAbs32(got-0.2) > tolerance {
		t.Errorf("expected premul B ~0.2, got %v", got)
	}
	if got := decodeFloat32(buf[28:32]); testAbs32(got-0.8) > tolerance {
		t.Errorf("expected premul A ~0.8, got %v", got)
	}
}

func TestConvertBGRAToRGBA(t *testing.T) {
	// 2 pixels: BGRA format.
	src := []byte{
		0x10, 0x20, 0x30, 0xFF, // pixel 0: B=0x10, G=0x20, R=0x30, A=0xFF
		0xAA, 0xBB, 0xCC, 0xDD, // pixel 1: B=0xAA, G=0xBB, R=0xCC, A=0xDD
	}
	dst := make([]byte, 8)
	convertBGRAToRGBA(src, dst, 2)

	// Expected RGBA: R=0x30, G=0x20, B=0x10, A=0xFF
	if dst[0] != 0x30 || dst[1] != 0x20 || dst[2] != 0x10 || dst[3] != 0xFF {
		t.Errorf("pixel 0: expected [30 20 10 FF], got [%02X %02X %02X %02X]",
			dst[0], dst[1], dst[2], dst[3])
	}
	// Expected RGBA: R=0xCC, G=0xBB, B=0xAA, A=0xDD
	if dst[4] != 0xCC || dst[5] != 0xBB || dst[6] != 0xAA || dst[7] != 0xDD {
		t.Errorf("pixel 1: expected [CC BB AA DD], got [%02X %02X %02X %02X]",
			dst[4], dst[5], dst[6], dst[7])
	}
}

func TestFloat32SliceToBytes(t *testing.T) {
	floats := []float32{1.0, 2.0, 3.0}
	bytes := float32SliceToBytes(floats)
	if len(bytes) != 12 {
		t.Fatalf("expected 12 bytes, got %d", len(bytes))
	}

	// Verify empty slice returns nil.
	nilBytes := float32SliceToBytes(nil)
	if nilBytes != nil {
		t.Error("expected nil for empty input")
	}
	emptyBytes := float32SliceToBytes([]float32{})
	if emptyBytes != nil {
		t.Error("expected nil for zero-length input")
	}
}

// decodeFloat32 decodes a little-endian float32 from a 4-byte slice.
func decodeFloat32(b []byte) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(b))
}

// testAbs32 returns the absolute value of a float32.
func testAbs32(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}
