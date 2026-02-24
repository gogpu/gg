// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Port of pathtag_reduce.wgsl and pathtag_scan.wgsl from Vello.
// Computes PathMonoid prefix sums over encoded path tags.
//
// Reference: vello_shaders/src/cpu/pathtag_reduce.rs, pathtag_scan.rs
// Variable names match Rust/WGSL originals for cross-reference.

package tilecompute

import "math/bits"

// PathMonoid is the monoid for path tag prefix sum.
// Each field accumulates counts from the start of the path tag stream.
type PathMonoid struct {
	TransIx       uint32 // Cumulative transform index
	PathSegIx     uint32 // Cumulative path segment count
	PathSegOffset uint32 // Cumulative offset into path data (in uint32s)
	StyleIx       uint32 // Cumulative style index
	PathIx        uint32 // Cumulative path index
}

// newPathMonoid creates a PathMonoid from a packed tag word (4 tags in one uint32).
// This is the EXACT Vello algorithm from pathtag.rs — do NOT simplify.
func newPathMonoid(tagWord uint32) PathMonoid {
	// Extract point counts from low 2 bits of each byte.
	// LineTo=0x9 → low 2 bits = 1, QuadTo=0xA → 2, CubicTo=0xB → 3
	pointCount := tagWord & 0x03030303

	// Count segments: segments exist where point_count > 0.
	// (point_count * 7) sets bit 2 where count >= 1.
	pathSegIx := uint32(bits.OnesCount32((pointCount * 7) & 0x04040404))

	// Count transforms (tag & 0x20).
	transIx := uint32(bits.OnesCount32(tagWord & 0x20202020))

	// Compute path data offset (number of uint32s of coordinate data).
	// n_points = point_count + adjustment for cubic vs quad.
	// For LineTo: point_count=1, bit2=0, bit3=1 → n_points=1, a=1*2=2 (x,y)
	// For QuadTo: point_count=2, bit2=0, bit3=1 → n_points=2, a=2*2=4 (2 points)
	// For CubicTo: point_count=3, bit2=0, bit3=1 → n_points=3, a=3*2=6 (3 points)
	// Actually the formula accounts for the fact that each point is 2 floats.
	nPoints := pointCount + ((tagWord >> 2) & 0x01010101)
	a := nPoints + (nPoints & (((tagWord >> 3) & 0x01010101) * 15))
	a += a >> 8
	a += a >> 16
	pathSegOffset := a & 0xff

	// Count path markers (tag & 0x10).
	pathIx := uint32(bits.OnesCount32(tagWord & 0x10101010))

	// Count style markers (tag & 0x40).
	styleIx := uint32(bits.OnesCount32(tagWord & 0x40404040))

	return PathMonoid{
		TransIx:       transIx,
		PathSegIx:     pathSegIx,
		PathSegOffset: pathSegOffset,
		StyleIx:       styleIx,
		PathIx:        pathIx,
	}
}

// combine merges two PathMonoids (associative operation for prefix sum).
func (m PathMonoid) combine(other PathMonoid) PathMonoid {
	return PathMonoid{
		TransIx:       m.TransIx + other.TransIx,
		PathSegIx:     m.PathSegIx + other.PathSegIx,
		PathSegOffset: m.PathSegOffset + other.PathSegOffset,
		StyleIx:       m.StyleIx + other.StyleIx,
		PathIx:        m.PathIx + other.PathIx,
	}
}

// pathtagReduce computes one PathMonoid per workgroup-sized block.
// This is the CPU version of pathtag_reduce.wgsl.
func pathtagReduce(scene *PackedScene) []PathMonoid {
	numTagWords := numPathTagWords(scene)
	nWG := (numTagWords + pathReduceWG - 1) / pathReduceWG
	result := make([]PathMonoid, nWG)

	for i := uint32(0); i < nWG; i++ {
		var m PathMonoid
		for j := uint32(0); j < pathReduceWG; j++ {
			idx := i*pathReduceWG + j
			if idx < numTagWords {
				tag := scene.Data[scene.Layout.PathTagBase+idx]
				m = m.combine(newPathMonoid(tag))
			}
		}
		result[i] = m
	}
	return result
}

// pathtagScan computes the exclusive prefix sum of PathMonoids.
// Returns one PathMonoid per tag word (4 tags).
// This is the CPU version of pathtag_scan.wgsl.
//
// Each result[idx] is the EXCLUSIVE prefix (sum of all elements before idx).
func pathtagScan(scene *PackedScene, reduced []PathMonoid) []PathMonoid {
	numTagWords := numPathTagWords(scene)
	result := make([]PathMonoid, numTagWords)
	nWG := uint32(len(reduced))

	prefix := PathMonoid{}
	for i := uint32(0); i < nWG; i++ {
		m := prefix
		for j := uint32(0); j < pathReduceWG; j++ {
			idx := i*pathReduceWG + j
			if idx < numTagWords {
				result[idx] = m // exclusive prefix (before this element)
				tag := scene.Data[scene.Layout.PathTagBase+idx]
				m = m.combine(newPathMonoid(tag))
			}
		}
		prefix = prefix.combine(reduced[i])
	}
	return result
}

// numPathTagWords returns the number of path tag words in the packed scene.
// This is the padded count (multiple of pathReduceWG).
func numPathTagWords(scene *PackedScene) uint32 {
	// PathTagBase..PathDataBase span contains the padded tag words.
	return scene.Layout.PathDataBase - scene.Layout.PathTagBase
}
