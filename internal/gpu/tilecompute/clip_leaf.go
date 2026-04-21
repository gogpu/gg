// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

// CPU equivalent of Vello's clip_reduce.wgsl + clip_leaf.wgsl.
// Matches BeginClip/EndClip pairs and fixes up EndClip draw monoids
// so that EndClip has access to its matching BeginClip's path_ix
// and scene_offset.
//
// On GPU, Vello uses a parallel bicyclic semigroup algorithm.
// On CPU, a simple stack-based approach is correct and efficient.
//
// Reference: vello_shaders/shader/clip_leaf.wgsl lines 187-203

package tilecompute

// clipLeafScan matches BeginClip/EndClip pairs using a stack and fixes up
// the draw monoids for each EndClip so it points to the same path and draw
// data as its matching BeginClip.
//
// This is the CRITICAL fixup step: without it, EndClip has no path_ix
// (can't emit CmdFill for clip path coverage) and no scene_offset (can't
// read blend_mode/alpha from draw data).
//
// Parameters:
//   - clipInps: clip input array from drawLeafScan (indexed by ClipIx)
//   - drawMonoids: draw monoid array to be fixed up in-place
func clipLeafScan(clipInps []ClipInp, drawMonoids []DrawMonoid) {
	var stack []int // indices into clipInps

	for i, inp := range clipInps {
		if inp.PathIx >= 0 {
			// BeginClip: push onto stack.
			stack = append(stack, i)
		} else {
			// EndClip: pop matching BeginClip.
			if len(stack) == 0 {
				continue // Malformed: unmatched EndClip, skip.
			}
			parent := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			// CRITICAL FIXUP from Vello clip_leaf.wgsl:187-203:
			// Make EndClip's draw monoid point to the same path and draw data
			// as its matching BeginClip.
			endIdx := uint32(^inp.PathIx) //nolint:gosec // Intentional: restore original draw index from complement
			parentClip := clipInps[parent]

			if endIdx < uint32(len(drawMonoids)) && parentClip.Ix < uint32(len(drawMonoids)) {
				// EndClip gets BeginClip's path_ix so coarse can look up the clip path.
				drawMonoids[endIdx].PathIx = uint32(parentClip.PathIx)
				// EndClip gets BeginClip's scene_offset so coarse can read blend_mode/alpha.
				drawMonoids[endIdx].SceneOffset = drawMonoids[parentClip.Ix].SceneOffset
			}
		}
	}
}
