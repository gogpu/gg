// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT
//
// draw_reduce.wgsl — Parallel reduction of DrawMonoid over draw tag words.
//
// CPU reference: tilecompute/draw_leaf.go drawReduce()
// Same tree reduction pattern as pathtag_reduce, but for DrawMonoid.
// Each workgroup processes 256 draw tags, producing one combined DrawMonoid.

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

// --- Bindings ---

@group(0) @binding(0) var<uniform> config: Config;
@group(0) @binding(1) var<storage, read> scene: array<u32>;
@group(0) @binding(2) var<storage, read_write> draw_reduced: array<DrawMonoid>;

// --- Workgroup shared memory ---

var<workgroup> sh_scratch: array<DrawMonoid, 256>;

// --- DrawMonoid operations ---

// new_draw_monoid creates a DrawMonoid from a draw tag.
// Port of newDrawMonoid() from draw_leaf.go.
//   bit 0: clip flag
//   bits 2-4: scene data size (u32s consumed from draw data)
//   bits 6-9: info data size (u32s produced to info buffer)
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
    let ix = global_id.x;

    var m: DrawMonoid;
    if ix < config.n_drawobj {
        let tag = scene[config.drawtag_base + ix];
        m = new_draw_monoid(tag);
    } else {
        m = draw_identity();
    }

    sh_scratch[local_id.x] = m;
    workgroupBarrier();

    // Tree reduction.
    for (var i = 0u; i < 8u; i = i + 1u) {
        let offset = 1u << i;
        if local_id.x >= offset {
            m = combine_draw(sh_scratch[local_id.x - offset], m);
        }
        workgroupBarrier();
        sh_scratch[local_id.x] = m;
        workgroupBarrier();
    }

    // Thread 0 writes the workgroup result.
    if local_id.x == 0u {
        draw_reduced[wg_id.x] = sh_scratch[WG_SIZE - 1u];
    }
}
