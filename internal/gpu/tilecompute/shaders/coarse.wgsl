// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause
//
// coarse.wgsl — Per-Tile Command List (PTCL) generation.
//
// CPU reference: tilecompute/coarse.go generatePTCLs(), emitDrawToTiles()
// For each draw object, determines which tiles it covers based on the path's
// bounding box and writes PTCL commands (CmdFill, CmdSolid, CmdColor) to
// each affected tile's command stream.
//
// PTCL encoding (from ptcl.go):
//   CmdFill:  [1, (segCount<<1)|evenOdd, segIndex, backdrop_as_u32]
//   CmdSolid: [3]
//   CmdColor: [5, packed_rgba]
//   CmdEnd:   [0]
//
// NOTE: This shader processes draw objects sequentially per tile. For the
// simplified pipeline, one thread per draw object iterates over its tiles.
// A production implementation would use binning for better parallelism.

// --- Shared types ---

struct DrawMonoid {
    path_ix: u32,
    clip_ix: u32,
    scene_offset: u32,
    info_offset: u32,
}

struct Path {
    bbox_x0: u32,
    bbox_y0: u32,
    bbox_x1: u32,
    bbox_y1: u32,
    tiles: u32,
}

struct Tile {
    backdrop: i32,
    segment_count_or_ix: u32,
}

struct PathSegment {
    point0_x: f32,
    point0_y: f32,
    point1_x: f32,
    point1_y: f32,
    y_edge: f32,
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

struct BumpAlloc {
    ptcl_offset: atomic<u32>,
}

// --- Constants ---

const CMD_END: u32 = 0u;
const CMD_FILL: u32 = 1u;
const CMD_SOLID: u32 = 3u;
const CMD_COLOR: u32 = 5u;

const DRAWTAG_COLOR: u32 = 0x44u;

// --- Bindings ---

@group(0) @binding(0) var<uniform> config: Config;
@group(0) @binding(1) var<storage, read> scene: array<u32>;
@group(0) @binding(2) var<storage, read> draw_monoids: array<DrawMonoid>;
@group(0) @binding(3) var<storage, read> info: array<u32>;
@group(0) @binding(4) var<storage, read> paths: array<Path>;
@group(0) @binding(5) var<storage, read> tiles: array<Tile>;
@group(0) @binding(6) var<storage, read> segments: array<PathSegment>;
@group(0) @binding(7) var<storage, read_write> ptcl: array<u32>;

// Per-tile PTCL write offsets. Each tile has a current write position.
// Allocated as width_in_tiles * height_in_tiles entries.
@group(0) @binding(8) var<storage, read_write> tile_ptcl_offsets: array<atomic<u32>>;

// Total segment counts per path (for tileSegRange).
@group(0) @binding(9) var<storage, read> path_total_segs: array<u32>;

// Global segment base offset per path.
@group(0) @binding(10) var<storage, read> path_seg_base: array<u32>;

// Style flags per path (bit 1 = even-odd fill rule).
@group(0) @binding(11) var<storage, read> path_styles: array<u32>;

// --- Constants for PTCL allocation ---

// Maximum number of u32 words per tile PTCL.
const PTCL_MAX_PER_TILE: u32 = 1024u;

// --- Helper: compute segment range for a tile ---

// tile_seg_range computes (seg_count, seg_start) for a tile.
// After coarse allocation, tiles with segments have segment_count_or_ix = ~segStart.
// The count is determined by finding the next tile's segStart or using total_segs.
fn tile_seg_range(
    tile: Tile,
    local_idx: u32,
    tile_count: u32,
    tiles_base: u32,
    total_segs: u32,
) -> vec2<u32> {
    let seg_start = ~tile.segment_count_or_ix;
    if i32(seg_start) < 0 {
        // No segments (segment_count_or_ix was 0, ~0 = 0xFFFFFFFF, i32 < 0).
        return vec2<u32>(0u, 0u);
    }

    // Find the end of this tile's segment range.
    var seg_end = total_segs;
    for (var next_idx = local_idx + 1u; next_idx < tile_count; next_idx = next_idx + 1u) {
        let next_start = ~tiles[tiles_base + next_idx].segment_count_or_ix;
        if i32(next_start) >= 0 {
            seg_end = next_start;
            break;
        }
    }

    if seg_end <= seg_start {
        return vec2<u32>(0u, seg_start);
    }
    return vec2<u32>(seg_end - seg_start, seg_start);
}

// --- Helper: write PTCL commands ---

fn write_ptcl_fill(
    global_tile_idx: u32,
    seg_count: u32,
    even_odd: bool,
    global_seg_start: u32,
    backdrop: i32,
) {
    var even_odd_flag = 0u;
    if even_odd {
        even_odd_flag = 1u;
    }
    let offset = atomicAdd(&tile_ptcl_offsets[global_tile_idx], 4u);
    let base = global_tile_idx * PTCL_MAX_PER_TILE + offset;
    ptcl[base] = CMD_FILL;
    ptcl[base + 1u] = (seg_count << 1u) | even_odd_flag;
    ptcl[base + 2u] = global_seg_start;
    ptcl[base + 3u] = bitcast<u32>(backdrop);
}

fn write_ptcl_solid(global_tile_idx: u32) {
    let offset = atomicAdd(&tile_ptcl_offsets[global_tile_idx], 1u);
    let base = global_tile_idx * PTCL_MAX_PER_TILE + offset;
    ptcl[base] = CMD_SOLID;
}

fn write_ptcl_color(global_tile_idx: u32, rgba: u32) {
    let offset = atomicAdd(&tile_ptcl_offsets[global_tile_idx], 2u);
    let base = global_tile_idx * PTCL_MAX_PER_TILE + offset;
    ptcl[base] = CMD_COLOR;
    ptcl[base + 1u] = rgba;
}

fn write_ptcl_end(global_tile_idx: u32) {
    let offset = atomicAdd(&tile_ptcl_offsets[global_tile_idx], 1u);
    let base = global_tile_idx * PTCL_MAX_PER_TILE + offset;
    ptcl[base] = CMD_END;
}

// --- Main entry point ---
// Each thread handles one draw object.

@compute @workgroup_size(256, 1, 1)
fn main(
    @builtin(global_invocation_id) global_id: vec3<u32>,
) {
    let draw_ix = global_id.x;
    if draw_ix >= config.n_drawobj {
        return;
    }

    let tag = scene[config.drawtag_base + draw_ix];
    if tag != DRAWTAG_COLOR {
        return;
    }

    let dm = draw_monoids[draw_ix];
    let path_ix = dm.path_ix;
    if path_ix >= config.n_path {
        return;
    }

    let path = paths[path_ix];
    let bbox_w = i32(path.bbox_x1) - i32(path.bbox_x0);
    let bbox_h = i32(path.bbox_y1) - i32(path.bbox_y0);
    if bbox_w <= 0 || bbox_h <= 0 {
        return;
    }
    let tile_count = u32(bbox_w * bbox_h);

    // Resolve draw parameters.
    let rgba = info[dm.info_offset];
    let even_odd = (path_styles[path_ix] & 0x02u) != 0u;
    let global_seg_base = path_seg_base[path_ix];
    let total_segs = path_total_segs[path_ix];

    let tiles_start = path.tiles;

    // Iterate over tiles in this path's bounding box.
    for (var ty = 0i; ty < bbox_h; ty = ty + 1) {
        for (var tx = 0i; tx < bbox_w; tx = tx + 1) {
            let local_tile_idx = u32(ty * bbox_w + tx);
            let tile_idx = tiles_start + local_tile_idx;
            let tile = tiles[tile_idx];

            let global_tx = i32(path.bbox_x0) + tx;
            let global_ty = i32(path.bbox_y0) + ty;
            if global_tx < 0 || global_tx >= i32(config.width_in_tiles) ||
               global_ty < 0 || global_ty >= i32(config.height_in_tiles) {
                continue;
            }

            // Compute segment range for this tile.
            let range = tile_seg_range(tile, local_tile_idx, tile_count, tiles_start, total_segs);
            let seg_count = range.x;
            let seg_start = range.y;

            let global_tile_idx = u32(global_ty) * config.width_in_tiles + u32(global_tx);

            if seg_count > 0u {
                write_ptcl_fill(global_tile_idx, seg_count, even_odd, global_seg_base + seg_start, tile.backdrop);
                write_ptcl_color(global_tile_idx, rgba);
            } else if tile.backdrop != 0 {
                write_ptcl_solid(global_tile_idx);
                write_ptcl_color(global_tile_idx, rgba);
            }
        }
    }
}
