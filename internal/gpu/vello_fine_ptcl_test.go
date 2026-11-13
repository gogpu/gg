//go:build !nogpu

package gpu

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestVelloFine_UnknownCommandEndsTile runs the Vello fine stage alone on the
// software adapter with a PTCL whose first command has an unknown tag. The
// shader must stop walking that tile and still write it, as the CPU fine
// (tilecompute.fineRasterizeTile) does: every pixel keeps the background.
//
// A `break` in the switch's default arm leaves only the switch (WGSL § break
// statement), so the loop used to re-read the same tag forever: a GPU hangs,
// and the software interpreter stops the invocation at its loop cap with the
// tile never written.
func TestVelloFine_UnknownCommandEndsTile(t *testing.T) {
	device, queue := fallbackComputeDevice(t)

	const (
		tileW, tileH = 16, 16
		bgColor      = uint32(0xff996633) // packed R | G<<8 | B<<16 | A<<24
		unknownTag   = uint32(99)
	)

	storage := func(label string, size uint64, usage gputypes.BufferUsage) *wgpu.Buffer {
		t.Helper()
		b, err := device.CreateBuffer(&wgpu.BufferDescriptor{Label: label, Size: size, Usage: usage})
		if err != nil {
			t.Fatalf("CreateBuffer %s: %v", label, err)
		}
		t.Cleanup(b.Release)
		return b
	}
	write := func(b *wgpu.Buffer, words []uint32) {
		t.Helper()
		if err := queue.WriteBuffer(b, 0, uint32SliceToBytes(words)); err != nil {
			t.Fatalf("WriteBuffer: %v", err)
		}
	}

	cpuStorage := gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst
	config := make([]uint32, 15) // Config in fine.wgsl: 15 u32 fields
	config[0], config[1] = 1, 1  // width_in_tiles, height_in_tiles
	config[2], config[3] = tileW, tileH
	config[14] = bgColor

	cfgBuf := storage("config", uint64(len(config)*4), gputypes.BufferUsageUniform|gputypes.BufferUsageCopyDst)
	write(cfgBuf, config)
	ptclBuf := storage("ptcl", 1024*4, cpuStorage) // PTCL_MAX_PER_TILE words for tile 0
	// Word 0 is the blend-spill offset, word 1 the first command, word 2 CMD_END.
	write(ptclBuf, []uint32{0, unknownTag, 0})
	segBuf := storage("segments", 5*4, cpuStorage)
	infoBuf := storage("info", 16, cpuStorage)
	outBuf := storage("output", tileW*tileH*4, gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	spillBuf := storage("blend_spill", 16, cpuStorage)

	module, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "fine", WGSL: shaderVelloFine})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer module.Release()
	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "fine_bgl", Entries: stageBindGroupLayoutEntries(VelloStageFine),
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label: "fine_pl", BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	defer layout.Release()
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label: "fine", Layout: layout, Module: module, EntryPoint: "main",
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	defer pipeline.Release()
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "fine_bg",
		Layout: bgl,
		Entries: stageBindGroupEntries(VelloStageFine, &VelloComputeBuffers{
			Config: cfgBuf, PTCL: ptclBuf, Segments: segBuf, Info: infoBuf, Output: outBuf, BlendSpill: spillBuf,
		}),
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer bg.Release()

	readBuf := storage("readback", tileW*tileH*4, gputypes.BufferUsageMapRead|gputypes.BufferUsageCopyDst)
	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch(1, 1, 1) // one tile: workgroup_size(4, 16) covers 16x16 pixels
	if err := pass.End(); err != nil {
		t.Fatalf("End compute pass: %v", err)
	}
	enc.CopyBufferToBuffer(outBuf, 0, readBuf, 0, tileW*tileH*4)
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// The software adapter runs the shader inside Submit. On the broken shader
	// it never returns, so bound the wait instead of hanging the test binary.
	done := make(chan error, 1)
	go func() {
		_, err := queue.Submit(cmd)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("fine stage did not finish: an unknown PTCL command does not end the tile's command loop")
	}

	size := uint64(tileW * tileH * 4)
	if err := readBuf.Map(context.Background(), wgpu.MapModeRead, 0, size); err != nil {
		t.Fatalf("Map: %v", err)
	}
	mr, err := readBuf.MappedRange(0, size)
	if err != nil {
		t.Fatalf("MappedRange: %v", err)
	}
	px := append([]byte(nil), mr.Bytes()...)
	mr.Release()
	if err := readBuf.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}

	for i := 0; i < tileW*tileH; i++ {
		if got := binary.LittleEndian.Uint32(px[i*4:]); got != bgColor {
			t.Fatalf("pixel %d = %#08x, want background %#08x", i, got, bgColor)
		}
	}
}

// fallbackComputeDevice opens wgpu's software adapter (the SPIR-V interpreter,
// registered by the hal/software import in this package's tests) through the
// public API, so the device carries the adapter's compute downlevel flags.
func fallbackComputeDevice(t *testing.T) (*wgpu.Device, *wgpu.Queue) {
	t.Helper()
	inst, err := wgpu.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	t.Cleanup(inst.Release)
	adapter, err := inst.RequestAdapter(&wgpu.RequestAdapterOptions{ForceFallbackAdapter: true})
	if err != nil {
		t.Fatalf("RequestAdapter(ForceFallbackAdapter): %v", err)
	}
	t.Cleanup(adapter.Release)
	if info := adapter.Info(); info.Backend != gputypes.BackendEmpty {
		t.Fatalf("fallback adapter is %q on backend %v, want wgpu's software adapter", info.Name, info.Backend)
	}
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Fatalf("RequestDevice: %v", err)
	}
	t.Cleanup(device.Release)
	queue := device.Queue()
	return device, queue
}
