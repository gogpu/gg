//go:build !nogpu

package gpu

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

func TestMakeGlyphMaskLCDUniform(t *testing.T) {
	transform := gg.Identity()
	color := [4]float32{1.0, 0.5, 0.25, 1.0}
	atlasW := float32(1024)
	atlasH := float32(1024)

	buf := makeGlyphMaskLCDUniform(transform, color, atlasW, atlasH)

	if len(buf) != glyphMaskLCDUniformSize {
		t.Fatalf("uniform size = %d, want %d", len(buf), glyphMaskLCDUniformSize)
	}

	// Verify color at offset 64 (after mat4x4).
	colorOffset := 64
	for i, want := range color {
		got := math.Float32frombits(binary.LittleEndian.Uint32(buf[colorOffset+i*4:]))
		if got != want {
			t.Errorf("color[%d] = %f, want %f", i, got, want)
		}
	}

	// Verify atlas_size at offset 80.
	atlasSizeOffset := 80
	gotW := math.Float32frombits(binary.LittleEndian.Uint32(buf[atlasSizeOffset:]))
	gotH := math.Float32frombits(binary.LittleEndian.Uint32(buf[atlasSizeOffset+4:]))
	if gotW != atlasW {
		t.Errorf("atlas_size.x = %f, want %f", gotW, atlasW)
	}
	if gotH != atlasH {
		t.Errorf("atlas_size.y = %f, want %f", gotH, atlasH)
	}

	// Verify padding at offset 88 is zero.
	padOffset := 88
	pad0 := binary.LittleEndian.Uint32(buf[padOffset:])
	pad1 := binary.LittleEndian.Uint32(buf[padOffset+4:])
	if pad0 != 0 || pad1 != 0 {
		t.Errorf("padding = (%d, %d), want (0, 0)", pad0, pad1)
	}
}

func TestMakeGlyphMaskLCDUniform_Transform(t *testing.T) {
	// Verify the transform matrix is stored in column-major order.
	transform := gg.Matrix{A: 2, B: 0.5, C: 10, D: 0.3, E: 3, F: 20}
	color := [4]float32{1, 1, 1, 1}

	buf := makeGlyphMaskLCDUniform(transform, color, 512, 512)

	// Column 0: [A, D, 0, 0]
	col0 := [4]float32{
		math.Float32frombits(binary.LittleEndian.Uint32(buf[0:])),
		math.Float32frombits(binary.LittleEndian.Uint32(buf[4:])),
		math.Float32frombits(binary.LittleEndian.Uint32(buf[8:])),
		math.Float32frombits(binary.LittleEndian.Uint32(buf[12:])),
	}
	if col0[0] != float32(transform.A) || col0[1] != float32(transform.D) {
		t.Errorf("column 0 = %v, want [%f, %f, 0, 0]", col0, transform.A, transform.D)
	}
}

func TestHasLCDBatches(t *testing.T) {
	tests := []struct {
		name    string
		batches []GlyphMaskBatch
		want    bool
	}{
		{"empty", nil, false},
		{"all_grayscale", []GlyphMaskBatch{{IsLCD: false}, {IsLCD: false}}, false},
		{"one_lcd", []GlyphMaskBatch{{IsLCD: false}, {IsLCD: true}}, true},
		{"all_lcd", []GlyphMaskBatch{{IsLCD: true}, {IsLCD: true}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasLCDBatches(tt.batches)
			if got != tt.want {
				t.Errorf("hasLCDBatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGlyphMaskBatch_LCDFields(t *testing.T) {
	// Verify that GlyphMaskBatch holds LCD-related fields correctly.
	batch := GlyphMaskBatch{
		IsLCD:       true,
		AtlasWidth:  1024,
		AtlasHeight: 1024,
	}
	if !batch.IsLCD {
		t.Error("IsLCD should be true")
	}
	if batch.AtlasWidth != 1024 || batch.AtlasHeight != 1024 {
		t.Errorf("atlas size = (%f, %f), want (1024, 1024)", batch.AtlasWidth, batch.AtlasHeight)
	}
}

func TestGlyphMaskFrameResources_IsLCD(t *testing.T) {
	// Verify that isLCD field is propagated in frame resources.
	res := &glyphMaskFrameResources{isLCD: true}
	if !res.isLCD {
		t.Error("frame resources isLCD should be true")
	}
}

func TestSelectGlyphMaskLCD_LCDLayoutAware(t *testing.T) {
	// Verify that LCDLayoutAware interface is satisfied by SDFAccelerator.
	// This is a compile-time check via the var _ line in sdf_gpu.go,
	// but we verify the SetLCDLayout method exists and works.
	engine := NewGlyphMaskEngine()
	engine.SetLCDLayout(text.LCDLayoutRGB)
	if engine.LCDLayout() != text.LCDLayoutRGB {
		t.Errorf("LCDLayout = %v, want RGB", engine.LCDLayout())
	}
}

func TestGlyphMaskLCDUniformSize(t *testing.T) {
	// LCD uniform must be 96 bytes (16-byte aligned for WebGPU).
	if glyphMaskLCDUniformSize != 96 {
		t.Errorf("glyphMaskLCDUniformSize = %d, want 96", glyphMaskLCDUniformSize)
	}
	// Grayscale uniform must be 80 bytes.
	if glyphMaskUniformSize != 80 {
		t.Errorf("glyphMaskUniformSize = %d, want 80", glyphMaskUniformSize)
	}
	// LCD must be larger than grayscale.
	if glyphMaskLCDUniformSize <= glyphMaskUniformSize {
		t.Error("LCD uniform should be larger than grayscale uniform")
	}
}
