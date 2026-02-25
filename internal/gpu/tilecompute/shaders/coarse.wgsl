// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT
//
// coarse.wgsl — Per-Tile Command List (PTCL) generation + segment allocation.
//
// CPU reference: tilecompute/coarse.go generatePTCLs(), emitDrawToTiles()
// GPU reference: vello_shaders/shader/coarse.wgsl write_path()
//
// Each thread processes one TILE and iterates over all draw objects in Z-order.
// This guarantees PTCL commands are written in correct compositing order
// (path 0 first, path 1 second, etc.) without race conditions.
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

// --- Main entry point ---
// One thread per global tile. Iterates draw objects in Z-order.
// Dispatch: ceil(total_tiles / 256) workgroups.

@compute @workgroup_size(256, 1, 1)
fn main(
    @builtin(global_invocation_id) global_id: vec3<u32>,
) {
    let tile_idx = global_id.x;
    let total_tiles = config.width_in_tiles * config.height_in_tiles;
    if tile_idx >= total_tiles {
        return;
    }

    let tile_x = tile_idx % config.width_in_tiles;
    let tile_y = tile_idx / config.width_in_tiles;

    // Local PTCL write offset — no atomics needed, one thread per tile.
    var ptcl_offset = 0u;

    // Iterate all draw objects in Z-order (path 0 first, path 1 second, ...).
    for (var draw_ix = 0u; draw_ix < config.n_drawobj; draw_ix = draw_ix + 1u) {
        let tag = scene[config.drawtag_base + draw_ix];
        if tag != DRAWTAG_COLOR {
            continue;
        }

        let dm = draw_monoids[draw_ix];
        let path_ix = dm.path_ix;
        if path_ix >= config.n_path {
            continue;
        }

        let path = paths[path_ix];

        // Check if this tile is within the path's bounding box.
        if tile_x < path.bbox_x0 || tile_x >= path.bbox_x1 ||
           tile_y < path.bbox_y0 || tile_y >= path.bbox_y1 {
            continue;
        }

        // Compute local tile index within the path's tile grid.
        let bbox_w = path.bbox_x1 - path.bbox_x0;
        let local_x = tile_x - path.bbox_x0;
        let local_y = tile_y - path.bbox_y0;
        let local_ix = local_y * bbox_w + local_x;
        let tile_ix = path.tiles + local_ix;

        let tile = tiles[tile_ix];
        let n_segs = tile.segment_count_or_ix;

        if n_segs == 0u && tile.backdrop == 0 {
            continue;
        }

        // Resolve draw parameters.
        let rgba = info[dm.info_offset];
        let even_odd = (path_styles[path_ix] & 0x02u) != 0u;

        // Write PTCL fill or solid command.
        if n_segs != 0u {
            // Allocate segment slots (global atomic — multiple tiles run in parallel).
            var seg_ix = 0u;
            seg_ix = atomicAdd(&bump.segments, n_segs);

            // Write inverted index back to tiles. path_tiling reads ~tile.segment_count_or_ix.
            tiles[tile_ix].segment_count_or_ix = ~seg_ix;

            // Emit CmdFill.
            let even_odd_flag = select(0u, 1u, even_odd);
            let base = tile_idx * PTCL_MAX_PER_TILE + ptcl_offset;
            ptcl[base] = CMD_FILL;
            ptcl[base + 1u] = (n_segs << 1u) | even_odd_flag;
            ptcl[base + 2u] = seg_ix;
            ptcl[base + 3u] = bitcast<u32>(tile.backdrop);
            ptcl_offset = ptcl_offset + 4u;
        } else {
            // No segments but non-zero backdrop → fully covered tile.
            let base = tile_idx * PTCL_MAX_PER_TILE + ptcl_offset;
            ptcl[base] = CMD_SOLID;
            ptcl_offset = ptcl_offset + 1u;
        }

        // Write CmdColor.
        let color_base = tile_idx * PTCL_MAX_PER_TILE + ptcl_offset;
        ptcl[color_base] = CMD_COLOR;
        ptcl[color_base + 1u] = rgba;
        ptcl_offset = ptcl_offset + 2u;
    }

    // CMD_END is implicit (PTCL buffer initialized to zeros).
    // Store final offset for diagnostics.
    atomicStore(&tile_ptcl_offsets[tile_idx], ptcl_offset);
}
