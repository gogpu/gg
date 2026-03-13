//go:build !nogpu

package gpu

import (
	"encoding/binary"
	"math"
)

// clipParamsSize is the byte size of the ClipParams uniform buffer.
// Layout: clip_rect (vec4<f32>) + clip_radius (f32) + clip_enabled (f32) + pad (vec2<f32>) = 32 bytes.
const clipParamsSize = 32

// ClipParams holds the parameters for analytic RRect clipping in fragment
// shaders. When Enabled is 1.0, the fragment shader evaluates the RRect SDF
// per pixel and multiplies the output alpha by the clip coverage.
//
// This struct is serialized to a 32-byte uniform buffer bound at @group(1)
// @binding(0) across all 5 render pipelines (SDF, convex, cover, MSDF text,
// glyph mask).
type ClipParams struct {
	// RectX1, RectY1 are the left-top corner in device pixels.
	RectX1, RectY1 float32
	// RectX2, RectY2 are the right-bottom corner in device pixels.
	RectX2, RectY2 float32
	// Radius is the corner radius in device pixels.
	Radius float32
	// Enabled is 0.0 (no clip) or 1.0 (clip active).
	Enabled float32
}

// Bytes serializes ClipParams to a 32-byte buffer suitable for GPU upload.
func (p *ClipParams) Bytes() []byte {
	buf := make([]byte, clipParamsSize)
	binary.LittleEndian.PutUint32(buf[0:4], math.Float32bits(p.RectX1))
	binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(p.RectY1))
	binary.LittleEndian.PutUint32(buf[8:12], math.Float32bits(p.RectX2))
	binary.LittleEndian.PutUint32(buf[12:16], math.Float32bits(p.RectY2))
	binary.LittleEndian.PutUint32(buf[16:20], math.Float32bits(p.Radius))
	binary.LittleEndian.PutUint32(buf[20:24], math.Float32bits(p.Enabled))
	// bytes 24..31 remain zero (padding).
	return buf
}

// NoClipParams returns a ClipParams with Enabled=0 (no clipping).
func NoClipParams() *ClipParams {
	return &ClipParams{Enabled: 0}
}
