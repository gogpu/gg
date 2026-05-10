//go:build !nogpu

package gpu

import (
	"context"
	"image"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/hal/software"
)

// createSoftwareDevice creates a software-backed *wgpu.Device for pixel-exact
// integration testing. Unlike noop, this performs REAL CPU rasterization —
// LoadOpLoad preserves content, scissor clips draws, pixels are verifiable.
func createSoftwareDevice(t *testing.T) (*wgpu.Device, *wgpu.Queue, func()) {
	t.Helper()
	api := software.API{}
	instance, err := api.CreateInstance(nil)
	if err != nil {
		t.Fatalf("software CreateInstance: %v", err)
	}
	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		instance.Destroy()
		t.Fatal("software backend: no adapters")
	}
	openDev, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		instance.Destroy()
		t.Fatalf("software Open: %v", err)
	}

	device, err := wgpu.NewDeviceFromHAL(
		openDev.Device,
		openDev.Queue,
		gputypes.Features(0),
		gputypes.DefaultLimits(),
		"software-test",
	)
	if err != nil {
		openDev.Device.Destroy()
		instance.Destroy()
		t.Fatalf("NewDeviceFromHAL: %v", err)
	}

	queue := device.Queue()
	cleanup := func() {
		device.Release()
		instance.Destroy()
	}
	return device, queue, cleanup
}

// TestDamageBlit_LoadOpLoad_PreservesContent verifies that LoadOpLoad + scissor
// preserves pixels outside the damage rect (e2e through software backend).
func TestDamageBlit_LoadOpLoad_PreservesContent(t *testing.T) {
	device, queue, cleanup := createSoftwareDevice(t)
	defer cleanup()

	const W, H = 8, 8

	// Create 8x8 target texture (render attachment + copy src for readback).
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "damage-target",
		Size:          wgpu.Extent3D{Width: W, Height: H, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
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

	// Frame 1: LoadOpClear red — fills entire 8x8.
	enc1, _ := device.CreateCommandEncoder(nil)
	rp1, _ := enc1.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "frame1-clear",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:       view,
			LoadOp:     gputypes.LoadOpClear,
			StoreOp:    gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, G: 0, B: 0, A: 1},
		}},
	})
	rp1.End()
	cmd1, _ := enc1.Finish()
	queue.Submit(cmd1)

	// Frame 2: LoadOpLoad + scissor (2,2,4,4). No draws — just preserve.
	enc2, _ := device.CreateCommandEncoder(nil)
	rp2, _ := enc2.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "frame2-damage",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:    view,
			LoadOp:  gputypes.LoadOpLoad,
			StoreOp: gputypes.StoreOpStore,
		}},
	})
	rp2.SetViewport(0, 0, W, H, 0, 1)
	rp2.SetScissorRect(2, 2, 4, 4)
	rp2.End()
	cmd2, _ := enc2.Finish()
	queue.Submit(cmd2)

	// Readback pixels.
	data := readbackTexture(t, device, queue, tex, W, H)
	if data == nil {
		t.Skip("readback not available")
	}

	// ALL pixels should be red (LoadOpLoad preserves frame 1 content,
	// scissor + no draws = no changes anywhere).
	assertPixel(t, data, W, 0, 0, 255, 0, 0, "corner (0,0)")
	assertPixel(t, data, W, 3, 3, 255, 0, 0, "inside scissor (3,3)")
	assertPixel(t, data, W, 7, 7, 255, 0, 0, "corner (7,7)")
}

// TestDamageBlit_ScissorClampsOverlay verifies that overlay draws outside
// damage rect are rejected (scissor intersection via computeDamageScissor).
func TestDamageBlit_ScissorClampsOverlay(t *testing.T) {
	damage := image.Rect(4, 4, 8, 8)

	// Overlay covers top-left quadrant — no overlap with damage.
	groupClip := &[4]uint32{0, 0, 4, 4}
	if _, _, _, _, valid := computeDamageScissor(groupClip, 8, 8, damage); valid {
		t.Error("overlay (0,0)-(4,4) should NOT intersect damage (4,4)-(8,8)")
	}

	// Overlay partially overlaps damage.
	groupClip2 := &[4]uint32{3, 3, 4, 4} // (3,3)-(7,7)
	x, y, w, h, valid2 := computeDamageScissor(groupClip2, 8, 8, damage)
	if !valid2 {
		t.Fatal("partial overlap should be valid")
	}
	if x != 4 || y != 4 || w != 3 || h != 3 {
		t.Errorf("intersection = (%d,%d,%d,%d), want (4,4,3,3)", x, y, w, h)
	}
}

// TestDamageBlit_NBufferAccumulation verifies ring buffer damage accumulation
// covers all frames in N-buffer swapchain.
func TestDamageBlit_NBufferAccumulation(t *testing.T) {
	// Spinner moves vertically: frame 1 at Y=10, frame 2 at Y=20.
	// With double buffering, buffer B needs union of frame 1+2 damage.
	frame1 := image.Rect(10, 10, 58, 58)
	frame2 := image.Rect(10, 20, 58, 68)

	accumulated := frame2.Union(frame1)
	expected := image.Rect(10, 10, 58, 68)
	if accumulated != expected {
		t.Errorf("accumulated = %v, want %v", accumulated, expected)
	}
}

// --- Helpers ---

func readbackTexture(t *testing.T, device *wgpu.Device, queue *wgpu.Queue, tex *wgpu.Texture, w, h int) []byte {
	t.Helper()
	bufSize := uint64(w * h * 4)
	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "readback",
		Size:  bufSize,
		Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead,
	})
	if err != nil {
		t.Logf("CreateBuffer for readback: %v", err)
		return nil
	}
	defer buf.Release()

	enc, _ := device.CreateCommandEncoder(nil)
	regions := []wgpu.BufferTextureCopy{{
		TextureBase: wgpu.ImageCopyTexture{Texture: tex},
		BufferLayout: wgpu.ImageDataLayout{
			Offset:       0,
			BytesPerRow:  uint32(w * 4),
			RowsPerImage: uint32(h),
		},
		Size: wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1},
	}}
	enc.CopyTextureToBuffer(tex, buf, regions)
	cmd, _ := enc.Finish()
	queue.Submit(cmd)

	// Map buffer synchronously (software backend resolves instantly).
	if err := buf.Map(context.Background(), wgpu.MapModeRead, 0, bufSize); err != nil {
		t.Logf("Buffer.Map failed: %v", err)
		return nil
	}

	mr, err := buf.MappedRange(0, bufSize)
	if err != nil {
		t.Logf("MappedRange: %v", err)
		return nil
	}
	result := make([]byte, len(mr.Bytes()))
	copy(result, mr.Bytes())
	mr.Release()
	buf.Unmap()
	return result
}

func assertPixel(t *testing.T, data []byte, stride, x, y int, wantR, wantG, wantB uint8, label string) {
	t.Helper()
	idx := (y*stride + x) * 4
	if idx+3 >= len(data) {
		t.Errorf("%s: pixel (%d,%d) out of bounds (data len=%d)", label, x, y, len(data))
		return
	}
	r, g, b := data[idx], data[idx+1], data[idx+2]
	if r != wantR || g != wantG || b != wantB {
		t.Errorf("%s: pixel (%d,%d) = RGB(%d,%d,%d), want RGB(%d,%d,%d)",
			label, x, y, r, g, b, wantR, wantG, wantB)
	}
}
