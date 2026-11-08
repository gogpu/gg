//go:build !nogpu

package gpu

import (
	"context"
	"image"
	"image/color"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/gogpu/gg"
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestDepthClipReadback draws content under an arbitrary-path clip (GPU-CLIP-003a: stencil fill of the clip path, a
// cover pass that writes depth where the stencil is set, content tested against that depth) through the real render
// pass, reads the pixels back and checks that content shows inside the clip and nothing outside it (P1-GPU-042:
// examples/clip_path drew nothing inside its circle and star clips).
//
// The test binary registers a real backend only with a ggtest_<backend> build tag (zz_backend_*_test.go). Without an
// adapter the test skips, unless GG_REQUIRE lists the backend, as CI or a release gate sets it:
//
//	GG_REQUIRE=vulkan go test -tags ggtest_vulkan -run TestDepthClipReadback ./internal/gpu/
func TestDepthClipReadback(t *testing.T) {
	accel := newReadbackAccelerator(t)

	const w, h = 96, 96
	circle := gg.NewPath()
	circle.Circle(48, 48, 30)
	star := gg.NewPath()
	for i := 0; i < 5; i++ { // every second vertex: a self-intersecting pentagram, filled NonZero
		a := -math.Pi/2 + float64(i*2)*2*math.Pi/5
		x, y := 48+40*math.Cos(a), 50+40*math.Sin(a)
		if i == 0 {
			star.MoveTo(x, y)
		} else {
			star.LineTo(x, y)
		}
	}
	star.Close()

	rect := func(x, y, rw, rh float64) *gg.Path { p := gg.NewPath(); p.Rectangle(x, y, rw, rh); return p }
	blue := gg.RGBA{R: 0.1, G: 0.2, B: 0.9, A: 1}

	tests := []struct {
		name    string
		clip    *gg.Path
		content *gg.Path
		inside  []image.Point // must be drawn
		outside []image.Point // must stay white
	}{
		{
			name: "circle_clip_full_rect", clip: circle, content: rect(0, 0, w, h),
			inside:  []image.Point{{48, 48}, {30, 48}, {66, 48}, {48, 25}},
			outside: []image.Point{{5, 5}, {90, 90}, {20, 20}, {76, 76}},
		},
		{
			name: "circle_clip_stripe", clip: circle, content: rect(8, 40, 80, 16),
			inside:  []image.Point{{48, 48}, {25, 48}, {70, 48}},
			outside: []image.Point{{10, 48}, {86, 48}, {48, 30}, {48, 66}},
		},
		{
			name: "star_clip_full_rect", clip: star, content: rect(0, 0, w, h),
			inside:  []image.Point{{48, 50}, {48, 18}},
			outside: []image.Point{{5, 5}, {90, 5}, {48, 85}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			img := newWhiteImage(w, h)
			target := gg.GPURenderTarget{Data: img.Pix, Width: w, Height: h, Stride: img.Stride}

			accel.BeginFrame()
			b := tc.clip.Bounds()
			accel.SetClipRect(uint32(b.Min.X), uint32(b.Min.Y), uint32(b.Dx()), uint32(b.Dy())) //nolint:gosec // in-viewport test bounds
			accel.SetClipPath(tc.clip)
			paint := gg.NewPaint()
			paint.SetBrush(gg.Solid(blue))
			if err := accel.FillPath(target, tc.content, paint); err != nil {
				t.Fatalf("FillPath: %v", err)
			}
			accel.ClearClipPath()
			accel.ClearClipRect()
			if accel.PendingCount() == 0 {
				t.Fatal("nothing queued on the GPU: the draw did not reach the render pass")
			}
			if err := accel.Flush(target); err != nil {
				t.Fatalf("Flush (readback): %v", err)
			}

			for _, p := range tc.inside {
				if c := img.RGBAAt(p.X, p.Y); c.B < 200 || c.R > 60 {
					t.Errorf("inside the clip at %v: got %v, want the blue fill", p, c)
				}
			}
			for _, p := range tc.outside {
				if c := img.RGBAAt(p.X, p.Y); c.R < 250 || c.G < 250 || c.B < 250 {
					t.Errorf("outside the clip at %v: got %v, want white", p, c)
				}
			}
		})
	}
}

// TestDepthClipReadbackFrame draws one frame the way examples/clip_path does: unclipped strokes and fills around two
// clipped sections (a circle clip over stripes, a star clip over circles), all in one render pass (P1-GPU-042).
func TestDepthClipReadbackFrame(t *testing.T) {
	newReadbackAccelerator(t) // skips without a render-pass adapter
	if err := gg.RegisterAccelerator(&SDFAccelerator{}); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}
	t.Cleanup(gg.CloseAccelerator)

	for _, scale := range []float64{1, 1.5, 2} { // HiDPI windows draw at a device scale, as ggcanvas sets it
		t.Run("scale_"+strconv.FormatFloat(scale, 'f', -1, 64), func(t *testing.T) {
			drawClipPathFrame(t, scale)
		})
	}
}

func drawClipPathFrame(t *testing.T, scale float64) {
	t.Helper()
	const w, h = 400, 240
	dc := gg.NewContext(int(w*scale), int(h*scale), gg.WithDeviceScale(scale))
	defer dc.Close()
	dc.SetRGBA(0.1, 0.1, 0.15, 1)
	dc.DrawRectangle(0, 0, w, h)
	mustFill(t, dc)

	// Section 1: circle clip over stripes, its outline stroked first.
	cx1, cy1 := 100.0, 120.0
	dc.SetRGBA(1, 1, 1, 0.3)
	dc.SetLineWidth(1)
	dc.DrawCircle(cx1, cy1, 80)
	mustStroke(t, dc)
	dc.Push()
	dc.DrawCircle(cx1, cy1, 80)
	dc.Clip()
	for i := 0; i < 5; i++ {
		dc.SetRGBA(0, 0.5, 1, 0.8)
		dc.DrawRectangle(cx1-100, cy1-90+float64(i)*36, 200, 36)
		mustFill(t, dc)
	}
	dc.Pop()

	// Section 2: star clip over circles.
	cx2, cy2 := 300.0, 120.0
	star := func() {
		for i := 0; i < 10; i++ {
			r := 90.0
			if i%2 == 1 {
				r = 36
			}
			a := -math.Pi/2 + float64(i)*math.Pi/5
			if i == 0 {
				dc.MoveTo(cx2+r*math.Cos(a), cy2+r*math.Sin(a))
			} else {
				dc.LineTo(cx2+r*math.Cos(a), cy2+r*math.Sin(a))
			}
		}
		dc.ClosePath()
	}
	dc.SetRGBA(1, 1, 1, 0.3)
	star()
	mustStroke(t, dc)
	dc.Push()
	star()
	dc.Clip()
	for i := 0; i < 8; i++ {
		dc.SetRGBA(0, 0.5, 1, 0.7)
		dc.DrawCircle(cx2, cy2, 90*float64(8-i)/8)
		mustFill(t, dc)
	}
	dc.Pop()

	if err := dc.FlushGPU(); err != nil {
		t.Fatalf("FlushGPU: %v", err)
	}
	img, ok := dc.Image().(*image.RGBA)
	if !ok {
		t.Fatalf("Image() is %T, want *image.RGBA", dc.Image())
	}
	at := func(p image.Point) color.RGBA { // logical point → device pixel
		return img.RGBAAt(int(float64(p.X)*scale), int(float64(p.Y)*scale))
	}
	for _, p := range []image.Point{{100, 120}, {60, 100}, {140, 140}, {300, 120}, {300, 60}} {
		if c := at(p); c.B < 150 {
			t.Errorf("inside a clip at %v: got %v, want the blue content", p, c)
		}
	}
	for _, p := range []image.Point{{5, 120}, {195, 120}, {300, 225}, {215, 120}} {
		if c := at(p); c.B > 80 {
			t.Errorf("outside the clips at %v: got %v, want the dark background", p, c)
		}
	}
}

// TestDepthClipSharedEncoder renders a circle clip into a texture view through a shared command encoder, the path a
// gogpu window takes (ggcanvas.Render → SetSharedEncoder → FlushGPUWithView; gogpu submits the encoder later). The
// session used to drop the clip's depth pass on this path while the content still tested against the depth buffer, so
// nothing was drawn inside the clip (P1-GPU-042).
func TestDepthClipSharedEncoder(t *testing.T) {
	newReadbackAccelerator(t) // skips without a render-pass adapter
	accel := &SDFAccelerator{}
	if err := gg.RegisterAccelerator(accel); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}
	t.Cleanup(gg.CloseAccelerator)
	accel.shared.mu.Lock()
	err := accel.shared.initGPU()
	device, queue := accel.shared.device, accel.shared.queue
	accel.shared.mu.Unlock()
	if err != nil {
		t.Fatalf("initGPU: %v", err)
	}

	const w, h = 96, 96
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "depth-clip-shared-target",
		Size:          wgpu.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBGRA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	defer view.Release()

	dc := gg.NewContext(w, h)
	defer dc.Close()
	dc.SetRGBA(0.1, 0.1, 0.15, 1)
	dc.DrawRectangle(0, 0, w, h)
	mustFill(t, dc)
	dc.Push()
	dc.DrawCircle(48, 48, 30)
	dc.Clip()
	dc.SetRGBA(0, 0.5, 1, 1)
	dc.DrawRectangle(0, 0, w, h)
	mustFill(t, dc)
	dc.Pop()

	enc, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "depth-clip-shared-encoder"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	dc.SetSharedEncoder(gpucontext.NewCommandEncoder(unsafe.Pointer(enc)))           //nolint:gosec // wgpu handle for the test
	err = dc.FlushGPUWithView(gpucontext.NewTextureView(unsafe.Pointer(view)), w, h) //nolint:gosec // wgpu handle
	dc.SetSharedEncoder(gpucontext.CommandEncoder{})
	if err != nil {
		t.Fatalf("FlushGPUWithView: %v", err)
	}

	pitch := readbackRowPitch(w)
	size := uint64(pitch) * h
	staging, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "depth-clip-shared-staging",
		Size:  size,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer staging.Release()
	enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{Offset: 0, BytesPerRow: pitch, RowsPerImage: h},
		TextureBase:  wgpu.ImageCopyTexture{Texture: tex, MipLevel: 0},
		Size:         wgpu.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1},
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := queue.Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := staging.Map(context.Background(), wgpu.MapModeRead, 0, size); err != nil {
		t.Fatalf("Map: %v", err)
	}
	rng, err := staging.MappedRange(0, size)
	if err != nil {
		t.Fatalf("MappedRange: %v", err)
	}
	pix := append([]byte(nil), rng.Bytes()...)
	if err := staging.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}

	blue := func(x, y int) byte { return pix[y*int(pitch)+x*4] } // BGRA: byte 0 is blue
	for _, p := range []image.Point{{48, 48}, {30, 48}, {66, 48}, {48, 25}} {
		if b := blue(p.X, p.Y); b < 200 {
			t.Errorf("inside the clip at %v: blue %d, want the blue fill (>= 200)", p, b)
		}
	}
	for _, p := range []image.Point{{5, 5}, {90, 90}, {20, 20}, {76, 76}} {
		if b := blue(p.X, p.Y); b > 80 {
			t.Errorf("outside the clip at %v: blue %d, want the dark background", p, b)
		}
	}
}

func mustFill(t *testing.T, dc *gg.Context) {
	t.Helper()
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill: %v", err)
	}
}

func mustStroke(t *testing.T, dc *gg.Context) {
	t.Helper()
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke: %v", err)
	}
}

// newReadbackAccelerator returns an SDFAccelerator on a real GPU adapter, or skips (fails under GG_REQUIRE).
func newReadbackAccelerator(t *testing.T) *SDFAccelerator {
	t.Helper()
	accel := &SDFAccelerator{}
	if err := accel.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(accel.Close)

	accel.shared.mu.Lock()
	err := accel.shared.initGPU()
	accel.shared.mu.Unlock()
	if err != nil {
		skipUnlessRequiredf(t, "GPU adapter not available: %v", err)
	}
	// Software adapters route shapes to the CPU (strategyRasterAtlas, BUG-SW-002): nothing to read back.
	if !accel.CanAccelerate(gg.AccelCircleSDF) {
		skipUnlessRequiredf(t, "adapter has no render-pass GPU path (software adapter)")
	}
	return accel
}

func skipUnlessRequiredf(t *testing.T, format string, args ...any) {
	t.Helper()
	if req := strings.TrimSpace(os.Getenv("GG_REQUIRE")); req != "" {
		t.Fatalf("GG_REQUIRE=%s: "+format, append([]any{req}, args...)...)
	}
	t.Skipf(format, args...)
}

func newWhiteImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	return img
}
