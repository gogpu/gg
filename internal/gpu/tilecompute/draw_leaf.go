// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Port of draw_reduce.wgsl and draw_leaf.wgsl from Vello.
// Computes DrawMonoid prefix sums over encoded draw tags and extracts draw info.
//
// Reference: vello_shaders/src/cpu/draw_reduce.rs, draw_leaf.rs
// Variable names match Rust/WGSL originals for cross-reference.

package tilecompute

// drawReduceWG is the workgroup size for draw tag reduce.
const drawReduceWG = 256

// DrawMonoid is the monoid for draw tag prefix sum.
type DrawMonoid struct {
	PathIx      uint32 // Cumulative path index
	ClipIx      uint32 // Cumulative clip index
	SceneOffset uint32 // Cumulative offset into draw data (scene_size from tag)
	InfoOffset  uint32 // Cumulative offset into info buffer (info_size from tag)
}

// newDrawMonoid creates a DrawMonoid from a draw tag.
// The tag encodes sizes in its bit fields:
//
//	bit 0: clip flag (1 = clip operation)
//	bits 2-4: scene data size (uint32s consumed from draw data)
//	bits 6-9: info data size (uint32s produced to info buffer)
func newDrawMonoid(tag uint32) DrawMonoid {
	var pathIx uint32
	if tag != DrawTagNop {
		pathIx = 1
	}
	return DrawMonoid{
		PathIx:      pathIx,
		ClipIx:      tag & 1,          // bit 0 = clip operation
		SceneOffset: (tag >> 2) & 0x7, // bits 2-4: scene data size
		InfoOffset:  (tag >> 6) & 0xf, // bits 6-9: info data size
	}
}

// combine merges two DrawMonoids (associative operation for prefix sum).
func (m DrawMonoid) combine(other DrawMonoid) DrawMonoid {
	return DrawMonoid{
		PathIx:      m.PathIx + other.PathIx,
		ClipIx:      m.ClipIx + other.ClipIx,
		SceneOffset: m.SceneOffset + other.SceneOffset,
		InfoOffset:  m.InfoOffset + other.InfoOffset,
	}
}

// drawReduce computes one DrawMonoid per workgroup-sized block.
// This is the CPU version of draw_reduce.wgsl.
func drawReduce(scene *PackedScene) []DrawMonoid {
	numDrawObjects := scene.Layout.NumDrawObjects
	nWG := (numDrawObjects + drawReduceWG - 1) / drawReduceWG
	if nWG == 0 {
		nWG = 1
	}
	result := make([]DrawMonoid, nWG)

	for i := uint32(0); i < nWG; i++ {
		var m DrawMonoid
		for j := uint32(0); j < drawReduceWG; j++ {
			idx := i*drawReduceWG + j
			if idx < numDrawObjects {
				tag := scene.Data[scene.Layout.DrawTagBase+idx]
				m = m.combine(newDrawMonoid(tag))
			}
		}
		result[i] = m
	}
	return result
}

// drawLeafScan computes the exclusive prefix sum of DrawMonoids.
// Also extracts draw info (color, transform metadata) for each draw object.
//
// Returns:
//   - drawMonoids: one DrawMonoid per draw object (exclusive prefix sum)
//   - info: extracted draw info buffer (packed uint32s)
//
// This is the CPU version of draw_leaf.wgsl.
func drawLeafScan(scene *PackedScene, reduced []DrawMonoid) ([]DrawMonoid, []uint32) {
	numDrawObjects := scene.Layout.NumDrawObjects
	drawMonoids := make([]DrawMonoid, numDrawObjects)
	nWG := uint32(len(reduced))

	// First pass: compute exclusive prefix sums.
	prefix := DrawMonoid{}
	for i := uint32(0); i < nWG; i++ {
		m := prefix
		for j := uint32(0); j < drawReduceWG; j++ {
			idx := i*drawReduceWG + j
			if idx < numDrawObjects {
				drawMonoids[idx] = m // exclusive prefix
				tag := scene.Data[scene.Layout.DrawTagBase+idx]
				m = m.combine(newDrawMonoid(tag))
			}
		}
		prefix = prefix.combine(reduced[i])
	}

	// Compute total info size from the final prefix sum.
	totalInfoSize := prefix.InfoOffset
	info := make([]uint32, totalInfoSize)

	// Second pass: extract draw info for each draw object.
	for idx := uint32(0); idx < numDrawObjects; idx++ {
		tag := scene.Data[scene.Layout.DrawTagBase+idx]
		dm := drawMonoids[idx]

		switch tag {
		case DrawTagColor:
			// For color draws: copy the packed RGBA from draw data to info.
			// SceneOffset gives the cumulative offset into DrawData.
			sceneOff := scene.Layout.DrawDataBase + dm.SceneOffset
			if sceneOff < uint32(len(scene.Data)) && dm.InfoOffset < uint32(len(info)) {
				info[dm.InfoOffset] = scene.Data[sceneOff]
			}
		case DrawTagBeginClip, DrawTagEndClip:
			// Clip operations: info is handled differently in the full pipeline.
			// For our simplified version, no additional data is needed.
		}
	}

	return drawMonoids, info
}
