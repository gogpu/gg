// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause
//
// pathtag_reduce.wgsl — Parallel reduction of PathMonoid over path tag words.
//
// CPU reference: tilecompute/pathtag.go pathtagReduce()
// Each workgroup of 256 threads processes 256 path tag words (1024 tags),
// computing a single combined PathMonoid. Results are written to reduced[].
//
// The second pass (pathtag_scan.wgsl) uses these per-workgroup sums to
// compute the full prefix scan.

// --- Shared types ---

struct PathMonoid {
    trans_ix: u32,
    path_seg_ix: u32,
    path_seg_offset: u32,
    style_ix: u32,
    path_ix: u32,
}

struct Config {
    width_in_tiles: u32,
    height_in_tiles: u32,
    target_width: u32,
    target_height: u32,
    n_drawobj: u32,
    n_path: u32,
    n_clip: u32,
    pathtag_base: u32,
    pathdata_base: u32,
    drawtag_base: u32,
    drawdata_base: u32,
    transform_base: u32,
    style_base: u32,
    n_lines: u32,
    bg_color: u32,
}

// --- Constants ---

const WG_SIZE: u32 = 256u;

// --- Bindings ---

@group(0) @binding(0) var<uniform> config: Config;
@group(0) @binding(1) var<storage, read> scene: array<u32>;
@group(0) @binding(2) var<storage, read_write> reduced: array<PathMonoid>;

// --- Workgroup shared memory ---

var<workgroup> sh_scratch: array<PathMonoid, 256>;

// --- PathMonoid operations ---

// new_path_monoid creates a PathMonoid from a packed tag word (4 tags in one u32).
// This is a direct port of newPathMonoid() from pathtag.go.
// The bit manipulation extracts counts of transforms, path segments, path data
// offsets, styles, and path markers from each of the 4 tag bytes.
fn new_path_monoid(tag_word: u32) -> PathMonoid {
    // Extract point counts from low 2 bits of each byte.
    // LineTo=0x9 -> low 2 bits = 1, QuadTo=0xA -> 2, CubicTo=0xB -> 3
    let point_count = tag_word & 0x03030303u;

    // Count segments: segments exist where point_count > 0.
    // (point_count * 7) sets bit 2 where count >= 1.
    let path_seg_ix = countOneBits((point_count * 7u) & 0x04040404u);

    // Count transforms (tag & 0x20 in each byte).
    let trans_ix = countOneBits(tag_word & 0x20202020u);

    // Compute path data offset (number of u32s of coordinate data).
    let n_points = point_count + ((tag_word >> 2u) & 0x01010101u);
    var a = n_points + (n_points & (((tag_word >> 3u) & 0x01010101u) * 15u));
    a = a + (a >> 8u);
    a = a + (a >> 16u);
    let path_seg_offset = a & 0xffu;

    // Count path markers (tag & 0x10 in each byte).
    let path_ix = countOneBits(tag_word & 0x10101010u);

    // Count style markers (tag & 0x40 in each byte).
    let style_ix = countOneBits(tag_word & 0x40404040u);

    return PathMonoid(trans_ix, path_seg_ix, path_seg_offset, style_ix, path_ix);
}

// combine merges two PathMonoids (associative operation for prefix sum).
fn combine(a: PathMonoid, b: PathMonoid) -> PathMonoid {
    return PathMonoid(
        a.trans_ix + b.trans_ix,
        a.path_seg_ix + b.path_seg_ix,
        a.path_seg_offset + b.path_seg_offset,
        a.style_ix + b.style_ix,
        a.path_ix + b.path_ix,
    );
}

// identity returns the identity PathMonoid (all zeros).
fn identity() -> PathMonoid {
    return PathMonoid(0u, 0u, 0u, 0u, 0u);
}

// --- Main entry point ---

@compute @workgroup_size(256, 1, 1)
fn main(
    @builtin(global_invocation_id) global_id: vec3<u32>,
    @builtin(local_invocation_id) local_id: vec3<u32>,
    @builtin(workgroup_id) wg_id: vec3<u32>,
) {
    // Each thread loads one tag word from the scene buffer.
    let ix = global_id.x;
    let num_tag_words = config.pathdata_base - config.pathtag_base;

    var m: PathMonoid;
    if ix < num_tag_words {
        let tag_word = scene[config.pathtag_base + ix];
        m = new_path_monoid(tag_word);
    } else {
        m = identity();
    }

    // Store in shared memory.
    sh_scratch[local_id.x] = m;
    workgroupBarrier();

    // Tree reduction: combine pairs at increasing stride.
    // After 8 iterations (2^8 = 256), thread 0 holds the total.
    for (var i = 0u; i < 8u; i = i + 1u) {
        let offset = 1u << i;
        if local_id.x >= offset {
            m = combine(sh_scratch[local_id.x - offset], m);
        }
        workgroupBarrier();
        sh_scratch[local_id.x] = m;
        workgroupBarrier();
    }

    // Thread 0 writes the workgroup result.
    if local_id.x == 0u {
        reduced[wg_id.x] = sh_scratch[WG_SIZE - 1u];
    }
}
