// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT
//
// path_tiling.wgsl — Clip line segments to tile boundaries + yEdge computation.
//
// CPU reference: tilecompute/path_tiling.go pathTilingMain()
// GPU reference: vello_shaders/shader/path_tiling.wgsl
//
// Each thread processes one SegmentCount record (produced by path_count).
// It recomputes the DDA parameters to determine which tile this segment
// belongs to, clips the line to the tile boundaries, computes the y_edge
// value, and writes a PathSegment to the segments buffer.
//
// Pipeline order: path_count → backdrop → coarse → **path_tiling** → fine
//
// CRITICAL: This shader reads inverted indices (~seg_ix) from tiles,
// written by the coarse stage. It uses seg_start = ~tile.segment_count_or_ix
// to determine WHERE to write each PathSegment in the segments buffer.
//
// WORKAROUND STATUS (2026-02-25):
// This shader uses two workarounds for naga SPIR-V backend bugs:
//
// 1. select() instead of if/else for conditional assignments (NAGA-SPV-007).
//    Root cause: prologue pre-computes var inits using stale local variables.
//    Fix applied in naga/wgsl/lower.go (init splitting), verified via SPIR-V
//    disassembly. However, runtime still shows 12.5% pixel diff — an unknown
//    residual issue persists. Workaround remains until NAGA-SPV-008 is resolved.
//    See: naga/docs/dev/kanban/0-backlog/NAGA-SPV-008-runtime-residual-prologue.md
//
// 2. let-chain (no vec2 var reassignment) to avoid silently dropped stores.
//    Also related to NAGA-SPV-007 investigation.
//
// Removal tracked in: gg/docs/dev/kanban/0-backlog/GG-COMPUTE-002-remove-path-tiling-workaround.md

// --- Shared types ---

struct LineSoup {
    path_ix: u32,
    p0x: f32,
    p0y: f32,
    p1x: f32,
    p1y: f32,
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

struct SegmentCount {
    line_ix: u32,
    counts: u32,
}

struct PathSegment {
    point0_x: f32,
    point0_y: f32,
    point1_x: f32,
    point1_y: f32,
    y_edge: f32,
}

struct BumpAlloc {
    seg_counts: atomic<u32>,
    segments: atomic<u32>,
    _pad0: atomic<u32>,
    _pad1: atomic<u32>,
}

// --- Constants ---

const TILE_WIDTH: u32 = 16u;
const TILE_HEIGHT: u32 = 16u;
const TILE_SCALE: f32 = 0.0625; // 1.0 / 16.0
const ONE_MINUS_ULP: f32 = 0.99999994;
const ROBUST_EPSILON: f32 = 2e-7;
const EPSILON: f32 = 1e-6;

// --- Bindings ---
//
// Matches Vello path_tiling.wgsl binding layout.

@group(0) @binding(0) var<uniform> config: Config;
@group(0) @binding(1) var<storage, read_write> bump: BumpAlloc;
@group(0) @binding(2) var<storage, read> seg_counts: array<SegmentCount>;
@group(0) @binding(3) var<storage, read> lines: array<LineSoup>;
@group(0) @binding(4) var<storage, read> paths: array<Path>;
@group(0) @binding(5) var<storage, read> tiles: array<Tile>;
@group(0) @binding(6) var<storage, read_write> segments: array<PathSegment>;

// --- Main entry point ---
// One invocation per SegmentCount record. Total = bump.seg_counts.

@compute @workgroup_size(256, 1, 1)
fn main(
    @builtin(global_invocation_id) global_id: vec3<u32>,
) {
    let n_segments = atomicLoad(&bump.seg_counts);
    if global_id.x >= n_segments {
        return;
    }

    let seg_count = seg_counts[global_id.x];
    let line = lines[seg_count.line_ix];
    let counts = seg_count.counts;
    let seg_within_slice = counts >> 16u;
    let seg_within_line = counts & 0xffffu;

    // Recompute DDA parameters (identical to path_count).
    // WORKAROUND: Use only let (no var reassignment for vec2) to avoid
    // naga SPIR-V codegen bug where vec2 var reassignment is silently ignored.
    let p0 = vec2<f32>(line.p0x, line.p0y);
    let p1 = vec2<f32>(line.p1x, line.p1y);
    let is_down = p1.y >= p0.y;
    let xy0_raw = select(p1, p0, is_down);
    let xy1_raw = select(p0, p1, is_down);
    let s0 = xy0_raw * TILE_SCALE;
    let s1 = xy1_raw * TILE_SCALE;
    // WORKAROUND: span() inlined to avoid naga SPIR-V function inlining bug.
    var count_x = u32(max(ceil(max(s0.x, s1.x)) - floor(min(s0.x, s1.x)), 1.0)) - 1u;
    var count = count_x + u32(max(ceil(max(s0.y, s1.y)) - floor(min(s0.y, s1.y)), 1.0));

    let dx = abs(s1.x - s0.x);
    let dy = s1.y - s0.y;
    let idxdy = 1.0 / (dx + dy);
    var a = dx * idxdy;
    let is_positive_slope = s1.x >= s0.x;
    let x_sign = select(-1.0, 1.0, is_positive_slope);
    let xt0 = floor(s0.x * x_sign);
    let c = s0.x * x_sign - xt0;
    let y0i = floor(s0.y);
    let ytop = select(y0i + 1.0, ceil(s0.y), s0.y == s1.y);
    let b = min((dy * c + dx * (ytop - s0.y)) * idxdy, ONE_MINUS_ULP);
    let robust_err = floor(a * (f32(count) - 1.0) + b) - f32(count_x);
    if robust_err != 0.0 {
        a -= ROBUST_EPSILON * sign(robust_err);
    }
    let x0i = i32(xt0 * x_sign + 0.5 * (x_sign - 1.0));

    // Compute tile coordinates for this segment.
    let z = floor(a * f32(seg_within_line) + b);
    let x = x0i + i32(x_sign * z);
    let y = i32(y0i + f32(seg_within_line) - z);

    // Look up path and tile.
    let path = paths[line.path_ix];
    let bbox = vec4<i32>(i32(path.bbox_x0), i32(path.bbox_y0), i32(path.bbox_x1), i32(path.bbox_y1));
    let stride = bbox.z - bbox.x;
    let tile_ix = i32(path.tiles) + (y - bbox.y) * stride + x - bbox.x;
    let tile = tiles[tile_ix];

    // Read inverted index written by coarse stage.
    let seg_start = ~tile.segment_count_or_ix;
    if i32(seg_start) < 0 {
        return;
    }

    // Tile boundaries in pixel coordinates.
    let tile_x_f = f32(x) * f32(TILE_WIDTH);
    let tile_y_f = f32(y) * f32(TILE_HEIGHT);
    let tile_x1_f = tile_x_f + f32(TILE_WIDTH);
    let tile_y1_f = tile_y_f + f32(TILE_HEIGHT);

    // WORKAROUND: ALL clipping uses let-chain (no vec2 var reassignment) to avoid
    // naga SPIR-V codegen bug where vec2 var stores are silently dropped.
    // Also uses select() instead of if/else to avoid naga store-at-merge-point bug.

    let dy_full = xy1_raw.y - xy0_raw.y;
    let dx_full = xy1_raw.x - xy0_raw.x;
    let safe_dy = select(dy_full, 1.0, abs(dy_full) < EPSILON);
    let safe_dx = select(dx_full, 1.0, abs(dx_full) < EPSILON);

    // --- Top clipping ---
    let do_top = seg_within_line > 0u;
    var z_prev_val = 0.0;
    z_prev_val = floor(a * (f32(seg_within_line) - 1.0) + b);
    let is_top_edge = z == z_prev_val;

    let xt_top = clamp(
        xy0_raw.x + dx_full * (tile_y_f - xy0_raw.y) / safe_dy,
        tile_x_f + 1e-3, tile_x1_f
    );
    let top_edge_x = xt_top;
    let top_edge_y = tile_y_f;

    let x_clip_top = select(tile_x1_f, tile_x_f, is_positive_slope);
    let yt_top = clamp(
        xy0_raw.y + dy_full * (x_clip_top - xy0_raw.x) / safe_dx,
        tile_y_f + 1e-3, tile_y1_f
    );
    let side_edge_x = x_clip_top;
    let side_edge_y = yt_top;

    // Choose top vs side entry point.
    let clip_top_x = select(side_edge_x, top_edge_x, is_top_edge);
    let clip_top_y = select(side_edge_y, top_edge_y, is_top_edge);
    // Apply only if this segment needs top clipping.
    let xy0_x = select(xy0_raw.x, clip_top_x, do_top);
    let xy0_y = select(xy0_raw.y, clip_top_y, do_top);

    // --- Bottom clipping (uses top-clipped xy0) ---
    let do_bottom = seg_within_line < count - 1u;
    var z_next_val = 0.0;
    z_next_val = floor(a * (f32(seg_within_line) + 1.0) + b);
    let is_bottom_edge = z == z_next_val;

    let dy_bc = xy1_raw.y - xy0_y;
    let dx_bc = xy1_raw.x - xy0_x;
    let safe_dy_bc = select(dy_bc, 1.0, abs(dy_bc) < EPSILON);
    let safe_dx_bc = select(dx_bc, 1.0, abs(dx_bc) < EPSILON);

    let xt_bot = clamp(
        xy0_x + dx_bc * (tile_y1_f - xy0_y) / safe_dy_bc,
        tile_x_f + 1e-3, tile_x1_f
    );
    let bot_edge_x = xt_bot;
    let bot_edge_y = tile_y1_f;

    let x_clip_bot = select(tile_x_f, tile_x1_f, is_positive_slope);
    let yt_bot = clamp(
        xy0_y + dy_bc * (x_clip_bot - xy0_x) / safe_dx_bc,
        tile_y_f + 1e-3, tile_y1_f
    );
    let side_bot_x = x_clip_bot;
    let side_bot_y = yt_bot;

    let clip_bot_x = select(side_bot_x, bot_edge_x, is_bottom_edge);
    let clip_bot_y = select(side_bot_y, bot_edge_y, is_bottom_edge);
    let xy1_x = select(xy1_raw.x, clip_bot_x, do_bottom);
    let xy1_y = select(xy1_raw.y, clip_bot_y, do_bottom);

    // --- Tile-local coordinates ---
    let p0x = xy0_x - tile_x_f;
    let p0y = xy0_y - tile_y_f;
    let p1x = xy1_x - tile_x_f;
    let p1y = xy1_y - tile_y_f;

    // --- y_edge computation ---
    let p0_on_left = p0x == 0.0;
    let p1_on_left = p1x == 0.0;
    let p0_at_top = p0y == 0.0;
    let p1_at_top = p1y == 0.0;

    var y_edge = 1e9;
    y_edge = select(y_edge, p0y, p0_on_left && !p1_on_left && !p0_at_top);
    y_edge = select(y_edge, p1y, !p0_on_left && p1_on_left && !p1_at_top);

    // Robustness cases for segments on the left edge.
    let case_1a = p0_on_left && p1_on_left && p0_at_top;
    let case_1b = p0_on_left && p1_on_left && !p0_at_top;
    let case_4 = !p0_on_left && p1_on_left && p1_at_top;

    // Nudge p0 off left edge.
    let out_p0x = select(p0x, EPSILON, p0_on_left);

    // p1 adjustments for special cases.
    let p1x_after_1a = select(p1x, EPSILON, case_1a);
    let p1y_after_1a = select(p1y, f32(TILE_HEIGHT), case_1a);
    let p1x_after_1b = select(p1x_after_1a, 2.0 * EPSILON, case_1b);
    let p1y_after_1b = select(p1y_after_1a, select(p0y, EPSILON, p0_on_left), case_1b);
    let out_p1x = select(p1x_after_1b, EPSILON, case_4);
    let out_p1y = p1y_after_1b;

    // Pixel boundary nudging.
    let nudge_p0 = out_p0x == floor(out_p0x) && out_p0x != 0.0;
    let final_p0x = select(out_p0x, out_p0x - EPSILON, nudge_p0);
    let final_p0y = p0y;
    let nudge_p1 = out_p1x == floor(out_p1x) && out_p1x != 0.0;
    let final_p1x = select(out_p1x, out_p1x - EPSILON, nudge_p1);
    let final_p1y = out_p1y;

    // Restore original direction (swap if line goes up).
    let wr_p0x = select(final_p0x, final_p1x, !is_down);
    let wr_p0y = select(final_p0y, final_p1y, !is_down);
    let wr_p1x = select(final_p1x, final_p0x, !is_down);
    let wr_p1y = select(final_p1y, final_p0y, !is_down);

    // Write PathSegment.
    segments[seg_start + seg_within_slice] = PathSegment(
        wr_p0x, wr_p0y,
        wr_p1x, wr_p1y,
        y_edge,
    );
}
