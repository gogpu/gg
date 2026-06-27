//go:build darwin && !nogpu

package gpu

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/wgpu"
)

// createMetalDevice creates a REAL Metal-backed *wgpu.Device + *wgpu.Queue for
// pixel-exact integration testing on Apple Silicon. Uses public wgpu API with
// BackendsMetal filter — no HAL imports needed.
func createMetalDevice(t *testing.T) (*wgpu.Device, *wgpu.Queue, func()) {
	t.Helper()
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{
		Backends: wgpu.BackendsMetal,
	})
	if err != nil {
		t.Skipf("metal CreateInstance: %v", err)
	}
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		instance.Release()
		t.Skipf("metal RequestAdapter: %v", err)
	}
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		instance.Release()
		t.Skipf("metal RequestDevice: %v", err)
	}
	queue := device.Queue()
	cleanup := func() {
		device.Release()
		instance.Release()
	}
	return device, queue, cleanup
}

// TestMetalStencilCoverMasksToShape is the regression test for the macOS/Metal
// stencil-then-cover bug (rounded UI rendered as squares, clipped content vanished).
//
// Root cause: wgpu/hal/metal/device.go built MTLDepthStencilDescriptor with only
// the depth fields and never translated the stencil compare/ops/masks. Metal
// defaults stencilCompareFunction=Always, so the cover pass's NotEqual(ref 0)
// test ALWAYS passed and the cover quad flooded the whole path bounding box.
//
// The test renders a filled CIRCLE via the stencil-then-cover path. The cover
// geometry is the path's bounding QUAD (FanTessellator.CoverQuad). If the stencil
// test works, only the disc is painted and the bbox corners stay transparent. If
// the stencil test is ignored (the bug), the corners are painted with fill color
// — i.e. the circle renders as a square.
func TestMetalStencilCoverMasksToShape(t *testing.T) {
	device, queue, cleanup := createMetalDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	const (
		W, H   = 64, 64
		cx, cy = 32, 32
		radius = 26
	)

	path := gg.NewPath()
	path.Circle(cx, cy, radius)

	target := gg.GPURenderTarget{
		Data:   make([]uint8, W*H*4), // zero = transparent background
		Width:  W,
		Height: H,
		Stride: W * 4,
	}

	// Opaque red fill.
	color := gg.RGBA{R: 1, G: 0, B: 0, A: 1}
	if err := sr.RenderPath(target, path, color, gg.FillRuleNonZero); err != nil {
		t.Fatalf("RenderPath: %v", err)
	}

	alphaAt := func(x, y int) uint8 { return target.Data[(y*W+x)*4+3] }
	redAt := func(x, y int) uint8 { return target.Data[(y*W+x)*4+0] }

	// Sanity: the center of the disc must be painted (rendering actually ran).
	// This holds for both the buggy and fixed code, so a failure here means the
	// pipeline didn't execute at all, not that the bug is present/absent.
	if a := alphaAt(cx, cy); a < 200 {
		t.Fatalf("center (%d,%d) not painted: alpha=%d (expected ~255) — render did not run", cx, cy, a)
	}
	if r := redAt(cx, cy); r < 200 {
		t.Errorf("center (%d,%d) red=%d, expected ~255", cx, cy, r)
	}

	// Discriminating assertion: bounding-box CORNERS lie inside the cover quad
	// but OUTSIDE the disc (dist to center = sqrt(22^2+22^2) ≈ 31 > radius 26).
	// Correct stencil masking -> transparent. The bug -> opaque red (square).
	corners := [][2]int{{10, 10}, {53, 10}, {10, 53}, {53, 53}}
	for _, c := range corners {
		if a := alphaAt(c[0], c[1]); a > 32 {
			t.Errorf("corner (%d,%d): alpha=%d (red=%d) — stencil test NOT masking the cover pass; "+
				"circle rendered as a square (Metal stencil bug)",
				c[0], c[1], a, redAt(c[0], c[1]))
		}
	}

	if t.Failed() || os.Getenv("DUMP_PNG") != "" {
		dumpPNG(t, "metal_stencil_circle.png", target)
	}
}

// TestMetalStencilRoundedRectMasking mirrors the real-world symptom most directly:
// the Settings modal panel is a ROUNDED rectangle drawn via stencil-then-cover.
// With the bug the rounded corners are filled (square panel); fixed, they are clipped.
func TestMetalStencilRoundedRectMasking(t *testing.T) {
	device, queue, cleanup := createMetalDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	const W, H = 64, 64

	// Rounded rect filling most of the canvas with a generous corner radius.
	path := gg.NewPath()
	path.RoundedRectangle(6, 6, 52, 52, 16)

	target := gg.GPURenderTarget{
		Data:   make([]uint8, W*H*4),
		Width:  W,
		Height: H,
		Stride: W * 4,
	}
	color := gg.RGBA{R: 0, G: 0.6, B: 1, A: 1}
	if err := sr.RenderPath(target, path, color, gg.FillRuleNonZero); err != nil {
		t.Fatalf("RenderPath: %v", err)
	}

	alphaAt := func(x, y int) uint8 { return target.Data[(y*W+x)*4+3] }

	// Center must be filled.
	if a := alphaAt(W/2, H/2); a < 200 {
		t.Fatalf("center not painted: alpha=%d — render did not run", a)
	}
	// Extreme corners of the panel rect (8,8) lie OUTSIDE the rounded corner arc
	// (radius 16 centered at (22,22) => corner point distance ≈ 19.8 > 16), so a
	// correctly-rounded panel leaves them transparent. The bug fills them (square).
	for _, c := range [][2]int{{8, 8}, {55, 8}, {8, 55}, {55, 55}} {
		if a := alphaAt(c[0], c[1]); a > 32 {
			t.Errorf("rounded corner (%d,%d): alpha=%d — panel rendered with SQUARE corners (Metal stencil bug)",
				c[0], c[1], a)
		}
	}

	if t.Failed() || os.Getenv("DUMP_PNG") != "" {
		dumpPNG(t, "metal_stencil_roundrect.png", target)
	}
}

// TestMetalStencilEvenOddMasking exercises the even-odd fill pipeline (a separate
// MTLDepthStencilState with IncrementWrap + WriteMask=0x01). A square with a
// concentric square hole must leave the hole transparent — which only works if
// the stencil parity test is honored by Metal.
func TestMetalStencilEvenOddMasking(t *testing.T) {
	device, queue, cleanup := createMetalDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	const W, H = 64, 64

	// Outer square (8..56) with an inner square hole (24..40), even-odd => ring.
	path := gg.NewPath()
	path.MoveTo(8, 8)
	path.LineTo(56, 8)
	path.LineTo(56, 56)
	path.LineTo(8, 56)
	path.Close()
	path.MoveTo(24, 24)
	path.LineTo(40, 24)
	path.LineTo(40, 40)
	path.LineTo(24, 40)
	path.Close()

	target := gg.GPURenderTarget{
		Data:   make([]uint8, W*H*4),
		Width:  W,
		Height: H,
		Stride: W * 4,
	}
	color := gg.RGBA{R: 0, G: 1, B: 0, A: 1}
	if err := sr.RenderPath(target, path, color, gg.FillRuleEvenOdd); err != nil {
		t.Fatalf("RenderPath: %v", err)
	}

	alphaAt := func(x, y int) uint8 { return target.Data[(y*W+x)*4+3] }

	// Ring body (e.g. (12,32)) must be filled.
	if a := alphaAt(12, 32); a < 200 {
		t.Errorf("ring body (12,32): alpha=%d, expected filled", a)
	}
	// Center of the hole (32,32) must be transparent — even-odd parity = 0 inside.
	if a := alphaAt(32, 32); a > 32 {
		t.Errorf("hole center (32,32): alpha=%d — even-odd stencil parity NOT honored (Metal stencil bug)", a)
	}

	if t.Failed() || os.Getenv("DUMP_PNG") != "" {
		dumpPNG(t, "metal_stencil_evenodd.png", target)
	}
}

// dumpPNG writes target.Data (RGBA, premultiplied) to a PNG in the OS temp dir
// for visual inspection. Helps confirm the fix by eye, and captures failures.
func dumpPNG(t *testing.T, name string, target gg.GPURenderTarget) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, target.Width, target.Height))
	for y := 0; y < target.Height; y++ {
		for x := 0; x < target.Width; x++ {
			o := (y*target.Width + x) * 4
			img.Set(x, y, color.NRGBA{
				R: target.Data[o], G: target.Data[o+1], B: target.Data[o+2], A: target.Data[o+3],
			})
		}
	}
	p := filepath.Join(os.TempDir(), name)
	f, err := os.Create(p)
	if err != nil {
		t.Logf("dumpPNG create: %v", err)
		return
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Logf("dumpPNG encode: %v", err)
		return
	}
	t.Logf("wrote %s", p)
}
