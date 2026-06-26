//go:build !nogpu

package gpu

import (
	"strings"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

// TestGlyphMaskRenderFrameNonGrouped renders glyph-mask text through the
// non-grouped GPURenderSession.RenderFrame path (as opposed to
// RenderFrameGrouped). RenderFrame previously never set s.frameW/s.frameH, so
// the flush-time ortho projection for glyph-mask text divided by a zero
// viewport (2.0/0 -> Inf) and the text rendered nothing. This is the minimal
// regression guard for that divide-by-zero.
func TestGlyphMaskRenderFrameNonGrouped(t *testing.T) {
	device, queue, cleanup := reproRealDevice(t)
	defer cleanup()

	const W, H = 512, 48
	face := reproFont(t)

	engine := NewGlyphMaskEngine()
	session := NewGPURenderSession(device, queue)

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

	if err := engine.SyncAtlasTextures(device, queue); err != nil {
		t.Fatalf("SyncAtlasTextures: %v", err)
	}
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

	data := make([]uint8, W*H*4)
	for i := range data {
		data[i] = 255
	}
	target := gg.GPURenderTarget{Data: data, Width: W, Height: H, Stride: W * 4}

	// Non-grouped path — this is what previously divided by zero.
	if err := session.RenderFrame(target, nil, nil, nil, nil, batches...); err != nil {
		t.Fatalf("RenderFrame: %v", err)
	}

	dark := 0
	for y := 0; y < H; y++ {
		for px := 0; px < W; px++ {
			i := (y*W + px) * 4
			if data[i] < 100 && data[i+1] < 100 && data[i+2] < 100 {
				dark++
			}
		}
	}
	t.Logf("non-grouped RenderFrame text dark pixels = %d", dark)
	if dark < 50 {
		t.Fatalf("non-grouped RenderFrame rendered %d dark text pixels (expected many) — frameW/H not set, ortho divided by zero", dark)
	}
}
