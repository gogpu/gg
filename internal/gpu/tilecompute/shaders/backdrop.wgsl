// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause
//
// backdrop.wgsl — Left-to-right backdrop prefix sum per tile row.
//
// CPU reference: tilecompute/rasterizer.go lines 89-99 (backdrop prefix sum),
//                tilecompute/coarse.go runPathStages() lines 292-299.
//
// For each path, the backdrop values computed by path_count represent the
// winding number delta at each tile boundary. This shader accumulates them
// left-to-right so that each tile's backdrop is the total winding number
// from the left edge of the bounding box to (and including) that tile.
//
// Each workgroup handles one path. Thread 0 performs the sequential scan
// for each row of the path's tile grid.

// --- Shared types ---

struct Path {
    bbox_x0: u32,
    bbox_y0: u32,
    bbox_x1: u32,
    bbox_y1: u32,
    tiles: u32,
}

// Non-atomic Tile for read/write access in the backdrop pass.
// By this stage, path_count has finished, so no concurrent writes occur
// within a single path's tile region.
struct Tile {
    backdrop: i32,
    segment_count_or_ix: u32,
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

// --- Bindings ---

@group(0) @binding(0) var<uniform> config: Config;
@group(0) @binding(1) var<storage, read> paths: array<Path>;
@group(0) @binding(2) var<storage, read_write> tiles: array<Tile>;

// --- Main entry point ---
// Dispatched with (n_path, 1, 1) workgroups.
// Each workgroup handles one path (thread 0 does the work).

@compute @workgroup_size(256, 1, 1)
fn main(
    @builtin(local_invocation_id) local_id: vec3<u32>,
    @builtin(workgroup_id) wg_id: vec3<u32>,
) {
    // Only thread 0 of each workgroup does work.
    if local_id.x != 0u {
        return;
    }

    let path_ix = wg_id.x;
    if path_ix >= config.n_path {
        return;
    }

    let path = paths[path_ix];
    let bbox_w = i32(path.bbox_x1) - i32(path.bbox_x0);
    let bbox_h = i32(path.bbox_y1) - i32(path.bbox_y0);

    if bbox_w <= 0 || bbox_h <= 0 {
        return;
    }

    let tiles_base = path.tiles;

    // For each row, accumulate backdrop left-to-right.
    // WORKAROUND: avoid nested for-loops (naga SPIR-V codegen bug —
    // outer loop stops after first iteration). Use row-at-a-time with
    // manual row tracking.
    var row_y = 0i;
    var sum = 0i;
    let total_tiles_count = bbox_w * bbox_h;
    for (var i = 0i; i < total_tiles_count; i = i + 1) {
        let cur_y = i / bbox_w;
        // Reset sum at the start of each new row.
        if cur_y != row_y {
            row_y = cur_y;
            sum = 0i;
        }
        let idx = tiles_base + u32(i);
        sum = sum + tiles[idx].backdrop;
        tiles[idx].backdrop = sum;
    }
}
