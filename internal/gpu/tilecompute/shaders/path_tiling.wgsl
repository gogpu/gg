// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause
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
// NOTE: Uses select() for conditional value assignments to avoid the naga
// SPIR-V backend bug where stores inside if/else blocks may not persist.
// Uses var for function call results to avoid naga inlining bug.

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

// --- Helper ---

fn span(a: f32, b: f32) -> u32 {
    return u32(max(ceil(max(a, b)) - floor(min(a, b)), 1.0));
}

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
    let p0 = vec2<f32>(line.p0x, line.p0y);
    let p1 = vec2<f32>(line.p1x, line.p1y);
    let is_down = p1.y >= p0.y;
    var xy0 = select(p1, p0, is_down);
    var xy1 = select(p0, p1, is_down);
    let s0 = xy0 * TILE_SCALE;
    let s1 = xy1 * TILE_SCALE;
    var count_x = span(s0.x, s1.x) - 1u;
    var count = count_x + span(s0.y, s1.y);

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
    // Vello: x0i = i32(xt0 * x_sign + 0.5 * (x_sign - 1.0))
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
    let tile_xy = vec2<f32>(f32(x) * f32(TILE_WIDTH), f32(y) * f32(TILE_HEIGHT));
    let tile_xy1 = tile_xy + vec2<f32>(f32(TILE_WIDTH), f32(TILE_HEIGHT));

    // --- Top clipping ---
    // CRITICAL: xy0 is mutable — top clip modifies it, bottom clip uses modified value.
    if seg_within_line > 0u {
        let z_prev = floor(a * (f32(seg_within_line) - 1.0) + b);
        if z == z_prev {
            // Top edge is clipped — entered from top of tile.
            var xt = xy0.x + (xy1.x - xy0.x) * (tile_xy.y - xy0.y) / (xy1.y - xy0.y);
            xt = clamp(xt, tile_xy.x + 1e-3, tile_xy1.x);
            xy0 = vec2<f32>(xt, tile_xy.y);
        } else {
            // Side edge is clipped — entered from left (pos slope) or right (neg slope).
            let x_clip = select(tile_xy1.x, tile_xy.x, is_positive_slope);
            var yt = xy0.y + (xy1.y - xy0.y) * (x_clip - xy0.x) / (xy1.x - xy0.x);
            yt = clamp(yt, tile_xy.y + 1e-3, tile_xy1.y);
            xy0 = vec2<f32>(x_clip, yt);
        }
    }

    // --- Bottom clipping ---
    // CRITICAL: Uses xy0 which was ALREADY MODIFIED by top clipping above!
    if seg_within_line < count - 1u {
        let z_next = floor(a * (f32(seg_within_line) + 1.0) + b);
        if z == z_next {
            // Bottom edge is clipped.
            var xt = xy0.x + (xy1.x - xy0.x) * (tile_xy1.y - xy0.y) / (xy1.y - xy0.y);
            xt = clamp(xt, tile_xy.x + 1e-3, tile_xy1.x);
            xy1 = vec2<f32>(xt, tile_xy1.y);
        } else {
            // Side edge is clipped.
            let x_clip = select(tile_xy.x, tile_xy1.x, is_positive_slope);
            var yt = xy0.y + (xy1.y - xy0.y) * (x_clip - xy0.x) / (xy1.x - xy0.x);
            yt = clamp(yt, tile_xy.y + 1e-3, tile_xy1.y);
            xy1 = vec2<f32>(x_clip, yt);
        }
    }

    // --- y_edge computation + numerical robustness ---
    var y_edge = 1e9;
    var p0out = xy0 - tile_xy;
    var p1out = xy1 - tile_xy;

    if p0out.x == 0.0 {
        if p1out.x == 0.0 {
            // Both on left edge.
            p0out.x = EPSILON;
            if p0out.y == 0.0 {
                // Entire tile.
                p1out.x = EPSILON;
                p1out.y = f32(TILE_HEIGHT);
            } else {
                // Make segment disappear.
                p1out.x = 2.0 * EPSILON;
                p1out.y = p0out.y;
            }
        } else if p0out.y == 0.0 {
            // p0 at top-left corner.
            p0out.x = EPSILON;
        } else {
            // p0 on left edge (not corner).
            y_edge = p0out.y;
        }
    } else if p1out.x == 0.0 {
        if p1out.y == 0.0 {
            // p1 at top-left corner.
            p1out.x = EPSILON;
        } else {
            // p1 on left edge (not corner).
            y_edge = p1out.y;
        }
    }

    // Pixel boundary nudging — avoid vertical lines on pixel grid.
    if p0out.x == floor(p0out.x) && p0out.x != 0.0 {
        p0out.x -= EPSILON;
    }
    if p1out.x == floor(p1out.x) && p1out.x != 0.0 {
        p1out.x -= EPSILON;
    }

    // Restore original direction (path_tiling.go line 188).
    if !is_down {
        let tmp = p0out;
        p0out = p1out;
        p1out = tmp;
    }

    // Write PathSegment.
    segments[seg_start + seg_within_slice] = PathSegment(
        p0out.x, p0out.y,
        p1out.x, p1out.y,
        y_edge,
    );
}
