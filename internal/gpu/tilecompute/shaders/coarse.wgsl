// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause
//
// coarse.wgsl — Per-Tile Command List (PTCL) generation + segment allocation.
//
// CPU reference: tilecompute/coarse.go generatePTCLs(), emitDrawToTiles()
// GPU reference: vello_shaders/shader/coarse.wgsl write_path()
//
// For each draw object, determines which tiles it covers based on the path's
// bounding box, allocates segment slots via atomicAdd, and writes PTCL commands
// (CmdFill, CmdSolid, CmdColor) to each affected tile's command stream.
//
// CRITICAL: This shader performs segment allocation (atomicAdd on bump.segments)
// and writes inverted indices (~seg_ix) back to tiles. The subsequent path_tiling
// stage reads these inverted indices to know WHERE to write PathSegment data.
//
// Pipeline order: path_count → backdrop → **coarse** → path_tiling → fine
//
// PTCL encoding (from ptcl.go):
//   CmdFill:  [1, (segCount<<1)|evenOdd, segIndex, backdrop_as_u32]
//   CmdSolid: [3]
//   CmdColor: [5, packed_rgba]
//   CmdEnd:   [0]

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

struct BumpAlloc {
    seg_counts: atomic<u32>,
    segments: atomic<u32>,
    _pad0: atomic<u32>,
    _pad1: atomic<u32>,
}

// --- Constants ---

const CMD_END: u32 = 0u;
const CMD_FILL: u32 = 1u;
const CMD_SOLID: u32 = 3u;
const CMD_COLOR: u32 = 5u;

const DRAWTAG_COLOR: u32 = 0x44u;

// Maximum number of u32 words per tile PTCL.
const PTCL_MAX_PER_TILE: u32 = 1024u;

// --- Bindings ---
//
// Compared to the previous version:
// - tiles is now read_write (coarse writes inverted indices)
// - bump added for segment allocation (atomicAdd)
// - path_total_segs, path_seg_base, segments REMOVED (not needed here)

@group(0) @binding(0) var<uniform> config: Config;
@group(0) @binding(1) var<storage, read> scene: array<u32>;
@group(0) @binding(2) var<storage, read> draw_monoids: array<DrawMonoid>;
@group(0) @binding(3) var<storage, read> info: array<u32>;
@group(0) @binding(4) var<storage, read> paths: array<Path>;
@group(0) @binding(5) var<storage, read_write> tiles: array<Tile>;
@group(0) @binding(6) var<storage, read_write> ptcl: array<u32>;
@group(0) @binding(7) var<storage, read_write> tile_ptcl_offsets: array<atomic<u32>>;
@group(0) @binding(8) var<storage, read> path_styles: array<u32>;
@group(0) @binding(9) var<storage, read_write> bump: BumpAlloc;

// --- Helper: write_path — allocate segments + write PTCL fill/solid ---
//
// This is the key function from Vello's coarse.wgsl write_path().
// It reads the RAW segment count from tile.segment_count_or_ix (written by
// path_count), allocates segment slots via atomicAdd, writes the inverted
// index (~seg_ix) back to the tile, and emits a PTCL CmdFill command.
//
// After this function, path_tiling reads ~seg_ix from tiles to know where
// to write clipped PathSegment data.
fn write_path(
    global_tile_idx: u32,
    tile_ix: u32,
    n_segs: u32,
    backdrop: i32,
    even_odd: bool,
) {
    if n_segs != 0u {
        // Allocate segment slots — returns the starting index in segments[].
        // WORKAROUND: split declaration from atomicAdd to avoid naga SPIR-V bug
        // where "var x = atomicOp()" fails with "atomic result expression not found".
        var seg_ix = 0u;
        seg_ix = atomicAdd(&bump.segments, n_segs);

        // Write inverted index back to tiles. path_tiling will read this
        // as ~tile.segment_count_or_ix to recover seg_ix.
        tiles[tile_ix].segment_count_or_ix = ~seg_ix;

        // Emit CmdFill to PTCL.
        var even_odd_flag = select(0u, 1u, even_odd);
        let offset = atomicAdd(&tile_ptcl_offsets[global_tile_idx], 4u);
        let base = global_tile_idx * PTCL_MAX_PER_TILE + offset;
        ptcl[base] = CMD_FILL;
        ptcl[base + 1u] = (n_segs << 1u) | even_odd_flag;
        ptcl[base + 2u] = seg_ix;
        ptcl[base + 3u] = bitcast<u32>(backdrop);
    } else {
        // No segments but non-zero backdrop → fully covered tile.
        let offset = atomicAdd(&tile_ptcl_offsets[global_tile_idx], 1u);
        let base = global_tile_idx * PTCL_MAX_PER_TILE + offset;
        ptcl[base] = CMD_SOLID;
    }
}

// --- Helper: write PTCL commands ---

fn write_ptcl_color(global_tile_idx: u32, rgba: u32) {
    let offset = atomicAdd(&tile_ptcl_offsets[global_tile_idx], 2u);
    let base = global_tile_idx * PTCL_MAX_PER_TILE + offset;
    ptcl[base] = CMD_COLOR;
    ptcl[base + 1u] = rgba;
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

    // Resolve draw parameters.
    let rgba = info[dm.info_offset];
    let even_odd = (path_styles[path_ix] & 0x02u) != 0u;

    let tiles_start = path.tiles;

    // DEBUG: count active threads via bump._pad0.
    atomicAdd(&bump._pad0, 1u);

    // Iterate over tiles in this path's bounding box.
    // WORKAROUND: Use flat loop instead of nested for-loops to avoid naga
    // SPIR-V codegen bug where nested for-loop outer iteration stops after
    // first pass (only ty=0 row is processed).
    let total_tiles_count = bbox_w * bbox_h;
    for (var i = 0i; i < total_tiles_count; i = i + 1) {
        let ty = i / bbox_w;
        let tx = i - ty * bbox_w;
        let tile_ix = tiles_start + u32(i);
        let tile = tiles[tile_ix];
        let n_segs = tile.segment_count_or_ix;

        let global_tx = i32(path.bbox_x0) + tx;
        let global_ty = i32(path.bbox_y0) + ty;
        if global_tx < 0 || global_tx >= i32(config.width_in_tiles) ||
           global_ty < 0 || global_ty >= i32(config.height_in_tiles) {
            continue;
        }

        let global_tile_idx = u32(global_ty) * config.width_in_tiles + u32(global_tx);

        // DEBUG: count ALL tiles visited.
        atomicAdd(&bump._pad1, 1u);

        if n_segs > 0u || tile.backdrop != 0 {
            write_path(global_tile_idx, tile_ix, n_segs, tile.backdrop, even_odd);
            write_ptcl_color(global_tile_idx, rgba);
        }
    }
}
