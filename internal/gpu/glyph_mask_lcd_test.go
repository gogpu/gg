//go:build !nogpu

package gpu

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/wgpu"
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

// compositeGlyphMaskOracle is a pure source-over reference for the glyph-mask
// shader contract. color is straight-alpha input; dst and the result are
// premultiplied. The CPU premultiplies color exactly once, then mask and clip
// coverage scale the already-premultiplied source.
func compositeGlyphMaskOracle(color gg.RGBA, mask, clip float32, dst [4]float32) [4]float32 {
	premul := color.Premultiply()
	coverage := mask * clip
	src := [4]float32{
		float32(premul.R) * coverage,
		float32(premul.G) * coverage,
		float32(premul.B) * coverage,
		float32(premul.A) * coverage,
	}
	oneMinusSrcAlpha := 1 - src[3]
	return [4]float32{
		src[0] + dst[0]*oneMinusSrcAlpha,
		src[1] + dst[1]*oneMinusSrcAlpha,
		src[2] + dst[2]*oneMinusSrcAlpha,
		src[3] + dst[3]*oneMinusSrcAlpha,
	}
}

func TestGlyphMaskPremulCompositeOracle(t *testing.T) {
	semiOrange := gg.RGBA{R: 1, G: 0.5, B: 0, A: 0.5}
	tests := []struct {
		name       string
		color      gg.RGBA
		mask, clip float32
		dst, want  [4]float32
	}{
		{name: "zero_coverage_transparent", color: semiOrange, mask: 0, clip: 1, dst: [4]float32{}, want: [4]float32{}},
		{name: "zero_coverage_white", color: semiOrange, mask: 0, clip: 1, dst: [4]float32{1, 1, 1, 1}, want: [4]float32{1, 1, 1, 1}},
		{name: "zero_alpha", color: gg.RGBA{R: 1, G: 0, B: 0, A: 0}, mask: 1, clip: 1, dst: [4]float32{0, 0, 1, 1}, want: [4]float32{0, 0, 1, 1}},
		{name: "full_coverage_transparent", color: semiOrange, mask: 1, clip: 1, dst: [4]float32{}, want: [4]float32{0.5, 0.25, 0, 0.5}},
		{name: "opaque_alpha_half_coverage", color: gg.RGBA{R: 1, G: 0, B: 0, A: 1}, mask: 0.5, clip: 1, dst: [4]float32{}, want: [4]float32{0.5, 0, 0, 0.5}},
		{name: "opaque_black_destination", color: semiOrange, mask: 1, clip: 1, dst: [4]float32{0, 0, 0, 1}, want: [4]float32{0.5, 0.25, 0, 1}},
		{name: "opaque_white_destination", color: semiOrange, mask: 1, clip: 1, dst: [4]float32{1, 1, 1, 1}, want: [4]float32{1, 0.75, 0.5, 1}},
		{name: "saturated_red_destination", color: semiOrange, mask: 1, clip: 1, dst: [4]float32{1, 0, 0, 1}, want: [4]float32{1, 0.25, 0, 1}},
		{name: "saturated_green_destination", color: semiOrange, mask: 1, clip: 1, dst: [4]float32{0, 1, 0, 1}, want: [4]float32{0.5, 0.75, 0, 1}},
		{name: "saturated_blue_destination", color: semiOrange, mask: 1, clip: 1, dst: [4]float32{0, 0, 1, 1}, want: [4]float32{0.5, 0.25, 0.5, 1}},
		{name: "clip_multiplies_coverage", color: semiOrange, mask: 1, clip: 0.5, dst: [4]float32{}, want: [4]float32{0.25, 0.125, 0, 0.25}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compositeGlyphMaskOracle(tt.color, tt.mask, tt.clip, tt.dst)
			if got != tt.want {
				t.Errorf("composite = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGlyphMaskPremulShaderContract(t *testing.T) {
	for _, required := range []string{
		"var straight_rgb = vec3<f32>(0.0);",
		"if color.a > 0.0 {",
		"straight_rgb = color.rgb / color.a;",
		"let alpha = apply_mask_gamma(raw_alpha, straight_rgb);",
		"let coverage = alpha * clip_cov;",
		"return vec4<f32>(color.rgb * coverage, color.a * coverage);",
	} {
		if !strings.Contains(glyphMaskShaderSource, required) {
			t.Errorf("grayscale shader missing one-premultiplication contract %q", required)
		}
	}
	if strings.Contains(glyphMaskShaderSource, "color.rgb * a") {
		t.Error("grayscale shader multiplies premultiplied RGB by alpha again")
	}
	if strings.Contains(glyphMaskShaderSource, "apply_mask_gamma(raw_alpha, color.rgb)") {
		t.Error("mask gamma uses opacity-dependent premultiplied luminance")
	}

	for _, forbidden := range []string{"color.r * a_r", "color.g * a_g", "color.b * a_b"} {
		if strings.Contains(glyphMaskLCDShaderSource, forbidden) {
			t.Errorf("dormant LCD shader multiplies premultiplied color twice: %q", forbidden)
		}
	}
	if !strings.Contains(glyphMaskLCDShaderSource, "scalar destination alpha is inexact") {
		t.Error("dormant LCD shader does not document its scalar-blend limitation")
	}
}

func TestGlyphMaskEnginePremultipliesBatchColor(t *testing.T) {
	source, err := text.NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("NewFontSource: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })
	face := source.Face(16)
	color := gg.RGBA{R: 0.8, G: 0.4, B: 0.2, A: 0.5}
	want := [4]float32{0.4, 0.2, 0.1, 0.5}

	var shaped []text.ShapedGlyph
	for glyph := range face.Glyphs("A") {
		shaped = append(shaped, text.ShapedGlyph{GID: glyph.GID, X: glyph.X, Y: glyph.Y})
	}

	engine := NewGlyphMaskEngine()
	engine.SetLCDLayout(text.LCDLayoutRGB)
	tests := []struct {
		name   string
		layout func() (GlyphMaskBatch, error)
	}{
		{
			name: "text",
			layout: func() (GlyphMaskBatch, error) {
				return engine.LayoutText(face, "A", 0, 20, color, gg.Identity(), 1)
			},
		},
		{
			name: "aliased",
			layout: func() (GlyphMaskBatch, error) {
				return engine.LayoutTextAliased(face, "A", 0, 20, color, gg.Identity(), 1)
			},
		},
		{
			name: "shaped",
			layout: func() (GlyphMaskBatch, error) {
				return engine.LayoutShapedGlyphs(face, shaped, 0, 20, color, gg.Identity(), 1, false)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batch, err := tt.layout()
			if err != nil {
				t.Fatalf("layout: %v", err)
			}
			if len(batch.Quads) == 0 {
				t.Fatal("layout produced no quads")
			}
			if batch.Color != want {
				t.Errorf("batch color = %v, want one CPU premultiplication %v", batch.Color, want)
			}
			if batch.IsLCD {
				t.Error("current engine produced an LCD batch")
			}
		})
	}
}

func TestGlyphMaskRecordDrawsRejectsLCDWithoutPipeline(t *testing.T) {
	pipeline := &GlyphMaskPipeline{}
	resources := &glyphMaskFrameResources{
		isLCD:     true,
		drawCalls: []glyphMaskDrawCall{{indexCount: 6}},
	}

	// A nil render pass is safe only if RecordDraws rejects the unsupported LCD
	// resource before attempting to bind the grayscale fallback pipeline.
	pipeline.RecordDraws(nil, resources, nil)
}

func TestGlyphMaskRecordDrawsRejectsLCDDepthClip(t *testing.T) {
	pipeline := &GlyphMaskPipeline{lcdPipelineWithStencil: new(wgpu.RenderPipeline)}
	resources := &glyphMaskFrameResources{
		isLCD:     true,
		drawCalls: []glyphMaskDrawCall{{indexCount: 6}},
	}

	// No LCD depth-clip pipeline exists. Reject the incompatible combination
	// rather than drawing unclipped or binding the grayscale depth pipeline.
	pipeline.RecordDraws(nil, resources, nil, true)
}
