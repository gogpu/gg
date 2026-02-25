// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT
//
// draw_leaf.wgsl — Parallel prefix scan of DrawMonoid + draw info extraction.
//
// CPU reference: tilecompute/draw_leaf.go drawLeafScan()
// Two-level scan (same pattern as pathtag_scan) producing exclusive prefix
// sums of DrawMonoid for each draw object. Additionally extracts per-draw
// info: for DrawTagColor (0x44), copies the packed RGBA from draw data to
// the info buffer at the monoid's InfoOffset.

// --- Shared types ---

struct DrawMonoid {
    path_ix: u32,
    clip_ix: u32,
    scene_offset: u32,
    info_offset: u32,
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
const DRAWTAG_NOP: u32 = 0u;
const DRAWTAG_COLOR: u32 = 0x44u;

// --- Bindings ---

@group(0) @binding(0) var<uniform> config: Config;
@group(0) @binding(1) var<storage, read> scene: array<u32>;
@group(0) @binding(2) var<storage, read> draw_reduced: array<DrawMonoid>;
@group(0) @binding(3) var<storage, read_write> draw_monoids: array<DrawMonoid>;
@group(0) @binding(4) var<storage, read_write> info: array<u32>;

// --- Workgroup shared memory ---

var<workgroup> sh_scratch: array<DrawMonoid, 256>;

// --- DrawMonoid operations ---

fn new_draw_monoid(tag: u32) -> DrawMonoid {
    var path_ix = 0u;
    if tag != DRAWTAG_NOP {
        path_ix = 1u;
    }
    return DrawMonoid(
        path_ix,
        tag & 1u,
        (tag >> 2u) & 0x7u,
        (tag >> 6u) & 0xfu,
    );
}

fn combine_draw(a: DrawMonoid, b: DrawMonoid) -> DrawMonoid {
    return DrawMonoid(
        a.path_ix + b.path_ix,
        a.clip_ix + b.clip_ix,
        a.scene_offset + b.scene_offset,
        a.info_offset + b.info_offset,
    );
}

fn draw_identity() -> DrawMonoid {
    return DrawMonoid(0u, 0u, 0u, 0u);
}

// --- Main entry point ---

@compute @workgroup_size(256, 1, 1)
fn main(
    @builtin(global_invocation_id) global_id: vec3<u32>,
    @builtin(local_invocation_id) local_id: vec3<u32>,
    @builtin(workgroup_id) wg_id: vec3<u32>,
) {
    // Compute prefix from all preceding workgroups.
    var wg_prefix = draw_identity();
    for (var i = 0u; i < wg_id.x; i = i + 1u) {
        wg_prefix = combine_draw(wg_prefix, draw_reduced[i]);
    }

    // Load this thread's draw tag.
    let ix = global_id.x;

    var local_m: DrawMonoid;
    if ix < config.n_drawobj {
        let tag = scene[config.drawtag_base + ix];
        local_m = new_draw_monoid(tag);
    } else {
        local_m = draw_identity();
    }

    // Intra-workgroup inclusive prefix scan.
    sh_scratch[local_id.x] = local_m;
    workgroupBarrier();

    for (var i = 0u; i < 8u; i = i + 1u) {
        let offset = 1u << i;
        if local_id.x >= offset {
            local_m = combine_draw(sh_scratch[local_id.x - offset], local_m);
        }
        workgroupBarrier();
        sh_scratch[local_id.x] = local_m;
        workgroupBarrier();
    }

    // Convert inclusive scan to exclusive.
    var exclusive: DrawMonoid;
    if local_id.x == 0u {
        exclusive = draw_identity();
    } else {
        exclusive = sh_scratch[local_id.x - 1u];
    }

    // Add the workgroup prefix to get the global exclusive prefix.
    let result = combine_draw(wg_prefix, exclusive);

    if ix < config.n_drawobj {
        // Write the exclusive prefix sum.
        draw_monoids[ix] = result;

        // Extract draw info for this draw object.
        let tag = scene[config.drawtag_base + ix];
        if tag == DRAWTAG_COLOR {
            // For color draws: copy packed RGBA from draw data to info buffer.
            // SceneOffset is the cumulative offset into draw data.
            let scene_off = config.drawdata_base + result.scene_offset;
            info[result.info_offset] = scene[scene_off];
        }
    }
}
