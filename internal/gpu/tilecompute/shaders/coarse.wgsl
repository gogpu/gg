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
//   Word 0:    blend_offset (for blend_spill SSBO, 0 if no deep clips)
//   CmdFill:   [1, (segCount<<1)|evenOdd, segIndex, backdrop_as_u32]
//   CmdSolid:  [3]
//   CmdColor:  [5, packed_rgba]
//   CmdEnd:    [0]

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
    blend: atomic<u32>,
    _pad1: atomic<u32>,
}

// --- Constants ---

const CMD_END: u32 = 0u;
const CMD_FILL: u32 = 1u;
const CMD_SOLID: u32 = 3u;
const CMD_COLOR: u32 = 5u;
const CMD_BEGIN_CLIP: u32 = 10u;
const CMD_END_CLIP: u32 = 11u;

const DRAWTAG_COLOR: u32 = 0x44u;
const DRAWTAG_BEGIN_CLIP: u32 = 0x9u;
const DRAWTAG_END_CLIP: u32 = 0x21u;

const TILE_WIDTH: u32 = 16u;
const TILE_HEIGHT: u32 = 16u;

// The "split" point between using local memory for the blend stack and
// spilling to the blend_spill buffer. Matches Vello's BLEND_STACK_SPLIT.
const BLEND_STACK_SPLIT: u32 = 4u;

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

    // Reserve word 0 for blend_offset (set at the end).
    let blend_offset_pos = tile_idx * PTCL_MAX_PER_TILE;
    var ptcl_offset = 1u; // Start writing commands after blend_offset word.

    // Blend depth tracking for blend_spill allocation.
    var max_blend_depth = 0u;

    // Clip state tracking (per-tile). Matches CPU coarse.go tileClipState.
    var clip_depth = 0u;
    var clip_zero_depth = 0u;
    var render_blend_depth = 0u;

    // Iterate all draw objects in Z-order (path 0 first, path 1 second, ...).
    for (var draw_ix = 0u; draw_ix < config.n_drawobj; draw_ix = draw_ix + 1u) {
        let tag = scene[config.drawtag_base + draw_ix];
        let dm = draw_monoids[draw_ix];
        let path_ix = dm.path_ix;

        // --- BeginClip: push clip level, suppress draws outside clip ---
        if tag == DRAWTAG_BEGIN_CLIP {
            if clip_zero_depth > 0u {
                clip_depth = clip_depth + 1u;
                continue;
            }
            if path_ix >= config.n_path {
                clip_depth = clip_depth + 1u;
                continue;
            }
            let path = paths[path_ix];
            if tile_x < path.bbox_x0 || tile_x >= path.bbox_x1 ||
               tile_y < path.bbox_y0 || tile_y >= path.bbox_y1 {
                clip_zero_depth = clip_depth + 1u;
                clip_depth = clip_depth + 1u;
                continue;
            }
            let bbox_w = path.bbox_x1 - path.bbox_x0;
            let local_x = tile_x - path.bbox_x0;
            let local_y = tile_y - path.bbox_y0;
            let local_ix = local_y * bbox_w + local_x;
            let tile_ix_c = path.tiles + local_ix;
            let tile_c = tiles[tile_ix_c];
            if tile_c.segment_count_or_ix == 0u && tile_c.backdrop == 0 {
                clip_zero_depth = clip_depth + 1u;
                clip_depth = clip_depth + 1u;
                continue;
            }
            // Tile has clip coverage — emit CmdBeginClip.
            let base = tile_idx * PTCL_MAX_PER_TILE + ptcl_offset;
            ptcl[base] = CMD_BEGIN_CLIP;
            ptcl_offset = ptcl_offset + 1u;
            render_blend_depth = render_blend_depth + 1u;
            if render_blend_depth > max_blend_depth {
                max_blend_depth = render_blend_depth;
            }
            clip_depth = clip_depth + 1u;
            continue;
        }

        // --- EndClip: pop clip level, emit coverage + EndClip ---
        if tag == DRAWTAG_END_CLIP {
            clip_depth = clip_depth - 1u;
            if clip_zero_depth > 0u {
                if clip_depth < clip_zero_depth {
                    clip_zero_depth = 0u;
                }
                continue;
            }
            if path_ix >= config.n_path {
                continue;
            }
            let path = paths[path_ix];
            // Emit fill for clip path coverage (matches CPU emitEndClipToTiles).
            if tile_x >= path.bbox_x0 && tile_x < path.bbox_x1 &&
               tile_y >= path.bbox_y0 && tile_y < path.bbox_y1 {
                let bbox_w = path.bbox_x1 - path.bbox_x0;
                let local_x = tile_x - path.bbox_x0;
                let local_y = tile_y - path.bbox_y0;
                let local_ix = local_y * bbox_w + local_x;
                let tile_ix_e = path.tiles + local_ix;
                let tile_e = tiles[tile_ix_e];
                let n_segs_e = tile_e.segment_count_or_ix;
                let even_odd_e = (path_styles[path_ix] & 0x02u) != 0u;
                if n_segs_e != 0u {
                    var seg_ix_e = atomicAdd(&bump.segments, n_segs_e);
                    tiles[tile_ix_e].segment_count_or_ix = ~seg_ix_e;
                    let fill_base = tile_idx * PTCL_MAX_PER_TILE + ptcl_offset;
                    let eo_flag = select(0u, 1u, even_odd_e);
                    ptcl[fill_base] = CMD_FILL;
                    ptcl[fill_base + 1u] = (n_segs_e << 1u) | eo_flag;
                    ptcl[fill_base + 2u] = seg_ix_e;
                    ptcl[fill_base + 3u] = bitcast<u32>(tile_e.backdrop);
                    ptcl_offset = ptcl_offset + 4u;
                } else if tile_e.backdrop != 0 {
                    let sol_base = tile_idx * PTCL_MAX_PER_TILE + ptcl_offset;
                    ptcl[sol_base] = CMD_SOLID;
                    ptcl_offset = ptcl_offset + 1u;
                }
            }
            // Read blend_mode and alpha from draw data.
            let scene_off = config.drawdata_base + dm.scene_offset;
            let blend_val = scene[scene_off];
            let alpha_bits = scene[scene_off + 1u];
            let ec_base = tile_idx * PTCL_MAX_PER_TILE + ptcl_offset;
            ptcl[ec_base] = CMD_END_CLIP;
            ptcl[ec_base + 1u] = blend_val;
            ptcl[ec_base + 2u] = alpha_bits;
            ptcl_offset = ptcl_offset + 3u;
            render_blend_depth = render_blend_depth - 1u;
            continue;
        }

        // --- Color draw: emit fill + color (clip-aware) ---
        if tag != DRAWTAG_COLOR {
            continue;
        }
        // Suppress draws inside empty clip region.
        if clip_zero_depth > 0u {
            continue;
        }

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

    // Write blend_offset at word 0. If max_blend_depth > BLEND_STACK_SPLIT,
    // allocate space in blend_spill via atomicAdd on bump.blend.
    var blend_ix = 0u;
    if max_blend_depth > BLEND_STACK_SPLIT {
        let scratch_size = (max_blend_depth - BLEND_STACK_SPLIT) * TILE_WIDTH * TILE_HEIGHT;
        blend_ix = atomicAdd(&bump.blend, scratch_size);
    }
    ptcl[blend_offset_pos] = blend_ix;
}
