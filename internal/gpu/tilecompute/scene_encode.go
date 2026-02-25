// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

// Scene encoding for the Vello CPU pipeline.
// Converts structured scene data (paths, colors, transforms) into flat buffers
// consumed by subsequent pipeline stages (pathtag reduce/scan, draw leaf, etc.).
//
// This is a simplified version of Vello's Encoding, focused on solid color fills
// without gradients, images, or glyphs.
//
// Reference: vello_encoding/src/encoding.rs, vello_encoding/src/resolve.rs

package tilecompute

import "math"

// SceneEncoding holds the encoded scene in flat buffers.
// This is a simplified version of Vello's Encoding struct,
// focused on solid fills without gradients/images/glyphs.
type SceneEncoding struct {
	// PathTags contains packed path commands (4 tags per uint32).
	// Tags: LineToF32=0x9, Path=0x10, Transform=0x20, Style=0x40.
	PathTags []uint32

	// PathData contains path coordinates as float32 bits (via math.Float32bits).
	PathData []uint32

	// DrawTags contains draw commands (one uint32 per draw object).
	// DrawTagColor=0x44, DrawTagBeginClip=0x9, DrawTagEndClip=0x21.
	DrawTags []uint32

	// DrawData contains draw parameters (e.g., packed RGBA for colors).
	DrawData []uint32

	// Transforms contains affine transforms (6 float32 per transform).
	Transforms []float32

	// Styles contains fill/stroke style flags.
	// For fills: [flags_u32] where bit 0 = fill(0)/stroke(1).
	Styles []uint32

	// NumPaths is the number of paths in the scene.
	NumPaths uint32

	// NumDrawObjects is the number of draw commands.
	NumDrawObjects uint32

	// NumClips is the number of clip operations.
	NumClips uint32
}

// SceneLayout describes offsets into the packed scene buffer.
type SceneLayout struct {
	NumDrawObjects uint32
	NumPaths       uint32
	NumClips       uint32
	PathTagBase    uint32 // Offset (in uint32s) to path tags
	PathDataBase   uint32 // Offset to path data
	DrawTagBase    uint32 // Offset to draw tags
	DrawDataBase   uint32 // Offset to draw data
	TransformBase  uint32 // Offset to transforms (in uint32s, float32 stored via Float32bits)
	StyleBase      uint32 // Offset to styles
}

// PackedScene holds the resolved scene: a flat buffer + layout.
type PackedScene struct {
	Data   []uint32
	Layout SceneLayout
}

// Draw tag constants (from Vello vello_shaders/src/shared/drawtag.rs).
const (
	DrawTagNop       uint32 = 0
	DrawTagColor     uint32 = 0x44 // info_size=1 (bits 6-9), scene_size=1 (bits 2-4)
	DrawTagBeginClip uint32 = 0x9
	DrawTagEndClip   uint32 = 0x21
)

// Path tag constants (from Vello vello_shaders/src/shared/pathtag.rs).
const (
	PathTagLineToF32  uint8 = 0x9
	PathTagQuadToF32  uint8 = 0xA
	PathTagCubicToF32 uint8 = 0xB
	PathTagPath       uint8 = 0x10 // Path marker (end of path)
	PathTagTransform  uint8 = 0x20
	PathTagStyle      uint8 = 0x40
)

// pathReduceWG is the workgroup size for path tag reduce.
const pathReduceWG = 256

// EncodeScene converts a list of PathDefs into a SceneEncoding.
// Each PathDef becomes: TRANSFORM tag + STYLE tag + line segments + PATH marker.
// Each PathDef also generates a DrawTagColor draw object.
func EncodeScene(paths []PathDef) *SceneEncoding {
	enc := &SceneEncoding{}

	// Temporary buffer for raw tags (before packing into uint32 words).
	var rawTags []uint8

	for _, pd := range paths {
		// 1. Emit TRANSFORM tag — identity for now.
		rawTags = append(rawTags, PathTagTransform)
		enc.Transforms = append(enc.Transforms, 1, 0, 0, 1, 0, 0) // identity affine

		// 2. Emit STYLE tag — fill, non-zero winding.
		rawTags = append(rawTags, PathTagStyle)
		var styleFlags uint32
		if pd.FillRule == FillRuleEvenOdd {
			styleFlags = 0x02 // bit 1 = even-odd
		}
		enc.Styles = append(enc.Styles, styleFlags)

		// 3. Emit path segments as MoveTo + LineTo sequences.
		//    Vello stores MoveTo as first point, then each LineTo stores
		//    only the endpoint. If the next line's P0 != previous P1,
		//    we need a new MoveTo.
		needsMoveTo := true
		var lastPoint [2]float32

		for _, line := range pd.Lines {
			if needsMoveTo || line.P0 != lastPoint {
				// Emit MoveTo by storing the start point as part of path data.
				// In Vello, MoveTo is encoded as a LineTo from the move point
				// (the flattener emits actual line segments with P0/P1).
				// For our simplified encoding, each LineTo stores P0 and P1
				// for the FIRST segment, and only P1 for subsequent connected segments.

				// Actually, in the real Vello encoding, LineTo always stores 2 floats
				// (the endpoint x,y). MoveTo is implicit at subpath start.
				// The path data for a line segment is just the endpoint.
				// But we need the initial point too.
				// Vello's approach: first point comes from a MoveTo (which stores 1 point),
				// then each LineTo stores 1 point (the end).

				// Emit the start point of this line as path data.
				// PathTagLineToF32 with the start point.
				rawTags = append(rawTags, PathTagLineToF32)
				enc.PathData = append(enc.PathData,
					math.Float32bits(line.P0[0]),
					math.Float32bits(line.P0[1]),
				)
				needsMoveTo = false
			}

			// Emit LineTo for the end point.
			rawTags = append(rawTags, PathTagLineToF32)
			enc.PathData = append(enc.PathData,
				math.Float32bits(line.P1[0]),
				math.Float32bits(line.P1[1]),
			)
			lastPoint = line.P1
		}

		// 4. Emit PATH marker — end of this path.
		rawTags = append(rawTags, PathTagPath)

		// 5. Add draw command for this path.
		enc.DrawTags = append(enc.DrawTags, DrawTagColor)

		// Pack color as premultiplied RGBA.
		a := float32(pd.Color[3]) / 255.0
		r := uint32(float32(pd.Color[0])*a + 0.5)
		g := uint32(float32(pd.Color[1])*a + 0.5)
		b := uint32(float32(pd.Color[2])*a + 0.5)
		aU := uint32(pd.Color[3])
		packedColor := r | (g << 8) | (b << 16) | (aU << 24)
		enc.DrawData = append(enc.DrawData, packedColor)

		enc.NumPaths++
		enc.NumDrawObjects++
	}

	// Pack raw tags into uint32 words (4 tags per word, LSB-first).
	enc.PathTags = packPathTags(rawTags)

	return enc
}

// packPathTags packs raw tag bytes into uint32 words (4 tags per word, LSB-first).
// word = tag0 | (tag1 << 8) | (tag2 << 16) | (tag3 << 24)
func packPathTags(rawTags []uint8) []uint32 {
	numWords := (len(rawTags) + 3) / 4
	words := make([]uint32, numWords)
	for i, tag := range rawTags {
		wordIdx := i / 4
		shift := uint(i%4) * 8
		words[wordIdx] |= uint32(tag) << shift
	}
	return words
}

// PackScene resolves a SceneEncoding into a flat PackedScene buffer.
// This is our simplified version of Vello's resolve stage.
// The packed buffer concatenates: pathTags (padded) | pathData | drawTags | drawData | transforms | styles
func PackScene(enc *SceneEncoding) *PackedScene {
	// Pad PathTags to multiple of pathReduceWG.
	numTagWords := uint32(len(enc.PathTags))
	paddedTagWords := ((numTagWords + pathReduceWG - 1) / pathReduceWG) * pathReduceWG
	if paddedTagWords == 0 {
		paddedTagWords = pathReduceWG
	}

	paddedTags := make([]uint32, paddedTagWords)
	copy(paddedTags, enc.PathTags)

	// Convert transforms from float32 to uint32 for packing.
	transformU32 := make([]uint32, len(enc.Transforms))
	for i, f := range enc.Transforms {
		transformU32[i] = math.Float32bits(f)
	}

	// Build layout and concatenate.
	var layout SceneLayout
	layout.NumDrawObjects = enc.NumDrawObjects
	layout.NumPaths = enc.NumPaths
	layout.NumClips = enc.NumClips

	offset := uint32(0)

	layout.PathTagBase = offset
	offset += paddedTagWords

	layout.PathDataBase = offset
	offset += uint32(len(enc.PathData))

	layout.DrawTagBase = offset
	offset += uint32(len(enc.DrawTags))

	layout.DrawDataBase = offset
	offset += uint32(len(enc.DrawData))

	layout.TransformBase = offset
	offset += uint32(len(transformU32))

	layout.StyleBase = offset
	offset += uint32(len(enc.Styles))

	// Allocate and fill flat buffer.
	data := make([]uint32, offset)
	copy(data[layout.PathTagBase:], paddedTags)
	copy(data[layout.PathDataBase:], enc.PathData)
	copy(data[layout.DrawTagBase:], enc.DrawTags)
	copy(data[layout.DrawDataBase:], enc.DrawData)
	copy(data[layout.TransformBase:], transformU32)
	copy(data[layout.StyleBase:], enc.Styles)

	return &PackedScene{
		Data:   data,
		Layout: layout,
	}
}
