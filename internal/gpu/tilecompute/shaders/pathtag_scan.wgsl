// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause
//
// pathtag_scan.wgsl — Parallel prefix scan of PathMonoid over path tag words.
//
// CPU reference: tilecompute/pathtag.go pathtagScan()
// Two-level scan: first loads per-workgroup sums from reduced[], computes
// the prefix for this workgroup, then performs an intra-workgroup inclusive
// scan. The result is an EXCLUSIVE prefix sum stored in tag_monoids[].
//
// Each tag_monoids[i] holds the sum of all PathMonoids BEFORE index i.

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
}

// --- Constants ---

const WG_SIZE: u32 = 256u;

// --- Bindings ---

@group(0) @binding(0) var<uniform> config: Config;
@group(0) @binding(1) var<storage, read> scene: array<u32>;
@group(0) @binding(2) var<storage, read> reduced: array<PathMonoid>;
@group(0) @binding(3) var<storage, read_write> tag_monoids: array<PathMonoid>;

// --- Workgroup shared memory ---

var<workgroup> sh_scratch: array<PathMonoid, 256>;

// --- PathMonoid operations ---

fn new_path_monoid(tag_word: u32) -> PathMonoid {
    let point_count = tag_word & 0x03030303u;
    let path_seg_ix = countOneBits((point_count * 7u) & 0x04040404u);
    let trans_ix = countOneBits(tag_word & 0x20202020u);
    let n_points = point_count + ((tag_word >> 2u) & 0x01010101u);
    var a = n_points + (n_points & (((tag_word >> 3u) & 0x01010101u) * 15u));
    a = a + (a >> 8u);
    a = a + (a >> 16u);
    let path_seg_offset = a & 0xffu;
    let path_ix = countOneBits(tag_word & 0x10101010u);
    let style_ix = countOneBits(tag_word & 0x40404040u);
    return PathMonoid(trans_ix, path_seg_ix, path_seg_offset, style_ix, path_ix);
}

fn combine(a: PathMonoid, b: PathMonoid) -> PathMonoid {
    return PathMonoid(
        a.trans_ix + b.trans_ix,
        a.path_seg_ix + b.path_seg_ix,
        a.path_seg_offset + b.path_seg_offset,
        a.style_ix + b.style_ix,
        a.path_ix + b.path_ix,
    );
}

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
    // Compute the exclusive prefix from all preceding workgroups.
    // This is a sequential scan over the reduced[] array up to (but not including)
    // this workgroup. For a small number of workgroups this is efficient.
    var wg_prefix = identity();
    for (var i = 0u; i < wg_id.x; i = i + 1u) {
        wg_prefix = combine(wg_prefix, reduced[i]);
    }

    // Load this thread's tag word and compute its local monoid.
    let ix = global_id.x;
    let num_tag_words = config.pathdata_base - config.pathtag_base;

    var local_m: PathMonoid;
    if ix < num_tag_words {
        let tag_word = scene[config.pathtag_base + ix];
        local_m = new_path_monoid(tag_word);
    } else {
        local_m = identity();
    }

    // Intra-workgroup inclusive prefix scan using shared memory.
    sh_scratch[local_id.x] = local_m;
    workgroupBarrier();

    for (var i = 0u; i < 8u; i = i + 1u) {
        let offset = 1u << i;
        if local_id.x >= offset {
            local_m = combine(sh_scratch[local_id.x - offset], local_m);
        }
        workgroupBarrier();
        sh_scratch[local_id.x] = local_m;
        workgroupBarrier();
    }

    // Convert inclusive scan to exclusive: exclusive[i] = inclusive[i-1] or identity.
    var exclusive: PathMonoid;
    if local_id.x == 0u {
        exclusive = identity();
    } else {
        exclusive = sh_scratch[local_id.x - 1u];
    }

    // Add the workgroup prefix to get the global exclusive prefix sum.
    let result = combine(wg_prefix, exclusive);

    // Write the result.
    if ix < num_tag_words {
        tag_monoids[ix] = result;
    }
}
