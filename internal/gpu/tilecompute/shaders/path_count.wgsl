// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause
//
// path_count.wgsl — DDA tile walk, backdrop computation, and segment counting.
//
// CPU reference: tilecompute/path_count.go pathCountMain()
// GPU reference: vello_shaders/shader/path_count.wgsl
//
// Each thread processes one LineSoup segment. It performs a DDA walk across the
// tile grid, computing which tiles the segment crosses, updating backdrop values
// (winding number contributions) and counting segments per tile.
//
// This shader uses atomics for tile backdrop and segment_count updates because
// multiple threads (lines) may touch the same tile concurrently.
//
// Pipeline order: flatten → path_count → backdrop → coarse → path_tiling → fine

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
    backdrop: atomic<i32>,
    segment_count_or_ix: atomic<u32>,
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

// --- Bindings ---

@group(0) @binding(0) var<uniform> config: Config;
@group(0) @binding(1) var<storage, read> lines: array<LineSoup>;
@group(0) @binding(2) var<storage, read> paths: array<Path>;
@group(0) @binding(3) var<storage, read_write> tiles: array<Tile>;
@group(0) @binding(4) var<storage, read_write> seg_counts: array<SegmentCount>;
@group(0) @binding(5) var<storage, read_write> bump: BumpAlloc;

// --- Helper functions ---

fn span(a: f32, b: f32) -> u32 {
    return u32(max(ceil(max(a, b)) - floor(min(a, b)), 1.0));
}

// --- Main entry point ---

@compute @workgroup_size(256, 1, 1)
fn main(
    @builtin(global_invocation_id) global_id: vec3<u32>,
) {
    let line_ix = global_id.x;
    if line_ix >= config.n_lines {
        return;
    }
    let line = lines[line_ix];
    let p0 = vec2<f32>(line.p0x, line.p0y);
    let p1 = vec2<f32>(line.p1x, line.p1y);

    let is_down = p1.y >= p0.y;
    let xy0 = select(p1, p0, is_down);
    let xy1 = select(p0, p1, is_down);
    let s0 = xy0 * TILE_SCALE;
    let s1 = xy1 * TILE_SCALE;
    var count_x = span(s0.x, s1.x) - 1u;
    var count = count_x + span(s0.y, s1.y);

    let dx = abs(s1.x - s0.x);
    let dy = s1.y - s0.y;
    if dx + dy == 0.0 {
        return;
    }
    if dy == 0.0 && floor(s0.y) == s0.y {
        return;
    }
    let idxdy = 1.0 / (dx + dy);
    var a = dx * idxdy;
    let is_positive_slope = s1.x >= s0.x;
    let x_sign = select(-1.0, 1.0, is_positive_slope);
    let xt0 = floor(s0.x * x_sign);
    let c = s0.x * x_sign - xt0;
    let y0 = floor(s0.y);
    let ytop = select(y0 + 1.0, ceil(s0.y), s0.y == s1.y);

    let b = min((dy * c + dx * (ytop - s0.y)) * idxdy, ONE_MINUS_ULP);
    let robust_err = floor(a * (f32(count) - 1.0) + b) - f32(count_x);
    if robust_err != 0.0 {
        a -= ROBUST_EPSILON * sign(robust_err);
    }

    let x0 = xt0 * x_sign + select(-1.0, 0.0, is_positive_slope);

    let path = paths[line.path_ix];
    let bbox = vec4<i32>(i32(path.bbox_x0), i32(path.bbox_y0), i32(path.bbox_x1), i32(path.bbox_y1));
    let xmin = min(s0.x, s1.x);
    let stride = bbox.z - bbox.x;
    if s0.y >= f32(bbox.w) || s1.y < f32(bbox.y) || xmin >= f32(bbox.z) || stride == 0 {
        return;
    }
    // Clip line to bounding box in "i" space.
    var imin = 0u;
    if s0.y < f32(bbox.y) {
        var iminf = round(( f32(bbox.y) - y0 + b - a) / (1.0 - a)) - 1.0;
        if y0 + iminf - floor(a * iminf + b) < f32(bbox.y) {
            iminf = iminf + 1.0;
        }
        imin = u32(iminf);
    }
    var imax = count;
    if s1.y > f32(bbox.w) {
        var imaxf = round((f32(bbox.w) - y0 + b - a) / (1.0 - a)) - 1.0;
        if y0 + imaxf - floor(a * imaxf + b) < f32(bbox.w) {
            imaxf = imaxf + 1.0;
        }
        imax = u32(imaxf);
    }

    let delta = select(1, -1, is_down);
    var ymin_i = 0i;
    var ymax_i = 0i;

    if max(s0.x, s1.x) < f32(bbox.x) {
        ymin_i = i32(ceil(s0.y));
        ymax_i = i32(ceil(s1.y));
        imax = imin;
    } else {
        let fudge = select(1.0, 0.0, is_positive_slope);
        if xmin < f32(bbox.x) {
            var f_val = round((x_sign * (f32(bbox.x) - x0) - b + fudge) / a);
            if (x0 + x_sign * floor(a * f_val + b) < f32(bbox.x)) == is_positive_slope {
                f_val = f_val + 1.0;
            }
            let ynext = i32(y0 + f_val - floor(a * f_val + b) + 1.0);
            if is_positive_slope {
                if u32(f_val) > imin {
                    ymin_i = i32(y0 + select(1.0, 0.0, y0 == s0.y));
                    ymax_i = ynext;
                    imin = u32(f_val);
                }
            } else if u32(f_val) < imax {
                ymin_i = ynext;
                ymax_i = i32(ceil(s1.y));
                imax = u32(f_val);
            }
        }
        if max(s0.x, s1.x) > f32(bbox.z) {
            var f_val = round((x_sign * (f32(bbox.z) - x0) - b + fudge) / a);
            if (x0 + x_sign * floor(a * f_val + b) < f32(bbox.z)) == is_positive_slope {
                f_val = f_val + 1.0;
            }
            if is_positive_slope {
                imax = min(imax, u32(f_val));
            } else {
                imin = max(imin, u32(f_val));
            }
        }
    }

    imax = max(imin, imax);
    ymin_i = max(ymin_i, bbox.y);
    ymax_i = min(ymax_i, bbox.w);

    // Apply backdrop for left-overflow segments.
    for (var y_val = ymin_i; y_val < ymax_i; y_val = y_val + 1) {
        let base_idx = i32(path.tiles) + (y_val - bbox.y) * stride;
        atomicAdd(&tiles[base_idx].backdrop, delta);
    }

    // Allocate segment count slots.
    let n_segs = imax - imin;
    if n_segs == 0u {
        return;
    }
    let seg_base = atomicAdd(&bump.seg_counts, n_segs);

    // DDA walk.
    var last_z = floor(a * (f32(imin) - 1.0) + b);

    for (var i = imin; i < imax; i = i + 1u) {
        let zf = a * f32(i) + b;
        let z = floor(zf);
        let y_val = i32(y0 + f32(i) - z);
        let x_val = i32(x0 + x_sign * z);
        let base_idx = i32(path.tiles) + (y_val - bbox.y) * stride - bbox.x;

        // Top edge detection: did segment enter from the top of this tile?
        let top_edge = select(last_z == z, y0 == s0.y, i == 0u);
        if top_edge && x_val + 1 < bbox.z {
            let x_bump = max(x_val + 1, bbox.x);
            atomicAdd(&tiles[base_idx + x_bump].backdrop, delta);
        }

        // Count segment in this tile (atomic because multiple lines may touch same tile).
        let seg_within_slice = atomicAdd(&tiles[base_idx + x_val].segment_count_or_ix, 1u);

        // Store SegmentCount for later path_tiling pass.
        let counts_packed = (seg_within_slice << 16u) | i;
        seg_counts[seg_base + i - imin] = SegmentCount(line_ix, counts_packed);

        last_z = z;
    }
}
