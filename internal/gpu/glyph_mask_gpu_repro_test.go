//go:build !nogpu

package gpu

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/wgpu"
)

// TestGlyphMaskGPURepro renders glyph-mask text through the REAL GPU pipeline
// (Vulkan on Linux, Metal on macOS) headlessly and reads the result back to a
// PNG. It draws a red SDF circle as a CONTROL alongside the text.
//
// THE BUG: the SDF circle renders correctly, but the glyph-mask text renders
// NOTHING — even though every CPU-side input is correct (see asserts below).
// This is the same failure the live app shows (ponytail: faded/garbled/blank
// glyph-mask text; the vector-outline path is fine). Reproduced here with no
// window, so you can attach a GPU frame debugger to the test binary.
//
// To capture a Metal frame on macOS:
//  1. Build the test binary:  go test -c ./internal/gpu/ -o gmrepro.test
//  2. In Xcode: Debug > Capture GPU Frame while running ./gmrepro.test
//     (or use xcrun / the Metal HUD). Inspect the glyph-mask DrawIndexed
//     calls: bound pipeline, vertex/index buffers, the R8 atlas texture
//     contents, viewport/scissor, and why the fragments are discarded.
//
// Output PNG: $GMREPRO_OUT or ./glyph_mask_gpu_repro.png
func TestGlyphMaskGPURepro(t *testing.T) {
	device, queue, cleanup := reproRealDevice(t)
	defer cleanup()

	const W, H = 512, 48 // width MUST be a multiple of 64 (256-byte row align)
	face := reproFont(t)

	engine := NewGlyphMaskEngine()
	session := NewGPURenderSession(device, queue, testSampleCount(t, device))

	// One batch per word — mirrors the live app's per-DrawText batches.
	var batches []GlyphMaskBatch
	x := 6.0
	for _, w := range strings.Fields("Want me to stash before you") {
		var glyphs []text.ShapedGlyph
		for g := range face.Glyphs(w) {
			glyphs = append(glyphs, text.ShapedGlyph{GID: g.GID, X: g.X, Y: g.Y})
		}
		b, err := engine.LayoutShapedGlyphs(face, glyphs, x, 24, gg.RGBA{A: 1}, gg.Identity(), 1.0, false)
		if err != nil {
			t.Fatalf("LayoutShapedGlyphs %q: %v", w, err)
		}
		if len(b.Quads) > 0 {
			batches = append(batches, b)
		}
		adv, _ := text.Measure(w+" ", face)
		x += adv
	}
	if len(batches) == 0 {
		t.Fatal("no glyph-mask batches produced")
	}

	// --- Prep, exactly as GPURenderContext.Flush does it ---
	if err := engine.SyncAtlasTextures(device, queue); err != nil {
		t.Fatalf("SyncAtlasTextures: %v", err)
	}
	// Clip layout BEFORE the glyph-mask pipeline so the pipeline layout includes
	// @group(1) (idempotent; matches the live app's ordering).
	if err := session.ensureClipBindLayout(); err != nil {
		t.Fatalf("ensureClipBindLayout: %v", err)
	}
	if err := session.ensureGlyphMaskPipeline(false); err != nil {
		t.Fatalf("ensureGlyphMaskPipeline: %v", err)
	}
	for i, b := range batches {
		view := engine.PageTextureView(b.AtlasPageIndex)
		if view == nil {
			t.Fatalf("nil atlas view for batch %d", i)
		}
		session.SetGlyphMaskAtlasView(i, view, b.IsLCD)
	}

	// Sanity: the atlas actually contains rasterized coverage (it does — the CPU
	// side is correct; the bug is downstream on the GPU).
	if pg, _, _ := engine.Atlas().PageR8Data(0); pg != nil {
		nz := 0
		for _, p := range pg {
			if p != 0 {
				nz++
			}
		}
		if nz == 0 {
			t.Fatal("atlas is empty — glyphs were not rasterized")
		}
		t.Logf("atlas nonzero coverage bytes = %d (masks ARE present)", nz)
	}

	// White RGBA readback target.
	data := make([]uint8, W*H*4)
	for i := range data {
		data[i] = 255
	}
	target := gg.GPURenderTarget{Data: data, Width: W, Height: H, Stride: W * 4}

	// CONTROL: red SDF circle. Renders fine — proves the harness/backend work.
	sdf := []SDFRenderShape{{
		Kind: 0, CenterX: 40, CenterY: 24, Param1: 16, Param2: 16,
		ColorR: 1, ColorG: 0, ColorB: 0, ColorA: 1,
	}}

	// Grouped path == the live app's path (sets s.frameW/H; the simple
	// RenderFrame does NOT — a separate latent bug, do not use it here).
	group := ScissorGroup{SDFShapes: sdf, GlyphMaskBatches: batches}
	if err := session.RenderFrameGrouped(target, []ScissorGroup{group}, nil, nil); err != nil {
		t.Fatalf("RenderFrameGrouped: %v", err)
	}

	out := os.Getenv("GMREPRO_OUT")
	if out == "" {
		out = "glyph_mask_gpu_repro.png"
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img := &image.RGBA{Pix: data, Stride: W * 4, Rect: image.Rect(0, 0, W, H)}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s — EXPECTED BUG: red circle visible, text MISSING", out)

	// Count black-ish text pixels (outside the red circle) as a regression check.
	textPixels := 0
	for y := 0; y < H; y++ {
		for px := 70; px < W; px++ { // skip the circle region
			i := (y*W + px) * 4
			if data[i] < 100 && data[i+1] < 100 && data[i+2] < 100 {
				textPixels++
			}
		}
	}
	t.Logf("text (dark) pixels rendered = %d  (BUG: this is ~0; fix makes it large)", textPixels)
}

func reproRealDevice(t *testing.T) (*wgpu.Device, *wgpu.Queue, func()) {
	t.Helper()
	inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: wgpu.BackendsPrimary})
	if err != nil {
		t.Skipf("no GPU instance: %v", err)
	}
	ad, err := inst.RequestAdapter(&wgpu.RequestAdapterOptions{PowerPreference: wgpu.PowerPreferenceHighPerformance})
	if err != nil {
		t.Skipf("no adapter: %v", err)
	}
	dev, err := ad.RequestDevice(&wgpu.DeviceDescriptor{Label: "gmrepro"})
	if err != nil {
		t.Skipf("no device: %v", err)
	}
	queue := dev.Queue()
	return dev, queue, func() { dev.Release() }
}

func reproFont(t *testing.T) text.Face {
	t.Helper()
	for _, p := range []string{
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/Library/Fonts/Arial.ttf",
		`C:\Windows\Fonts\arial.ttf`,
		filepath.Join("testdata", "test.ttf"),
	} {
		if _, err := os.Stat(p); err == nil {
			if src, err := text.NewFontSourceFromFile(p); err == nil {
				return src.Face(15)
			}
		}
	}
	t.Skip("no TTF font available")
	return nil
}
