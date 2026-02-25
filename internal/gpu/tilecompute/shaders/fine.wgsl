// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause
//
// fine.wgsl — Per-pixel rasterization driven by PTCL command streams.
//
// CPU reference: tilecompute/fine.go fineRasterizeTile(), fillPath()
// One workgroup per tile, 256 threads = 16x16 pixels. Each thread handles
// one pixel of the tile and walks the same PTCL command stream, but computes
// pixel-specific coverage and color values.
//
// PTCL command dispatch loop (from fine.go):
//   CMD_END   -> break (done with this tile)
//   CMD_FILL  -> compute per-pixel area via fill_path
//   CMD_SOLID -> area = 1.0 for all pixels
//   CMD_COLOR -> source-over blend: fg = color * area, rgba = rgba*(1-fg.a) + fg
//   CMD_BEGIN_CLIP -> push rgba to clip stack, clear
//   CMD_END_CLIP   -> pop, composite with alpha
//
// Output: packed RGBA u32 per pixel to the output buffer.

// --- Shared types ---

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
    bg_color: u32,
}

// --- Constants ---

const TILE_WIDTH: u32 = 16u;
const TILE_HEIGHT: u32 = 16u;

const CMD_END: u32 = 0u;
const CMD_FILL: u32 = 1u;
const CMD_SOLID: u32 = 3u;
const CMD_COLOR: u32 = 5u;
const CMD_BEGIN_CLIP: u32 = 10u;
const CMD_END_CLIP: u32 = 11u;

// Maximum PTCL words per tile (must match coarse shader).
const PTCL_MAX_PER_TILE: u32 = 1024u;

// Maximum clip stack depth (in-register, no dynamic allocation on GPU).
const MAX_CLIP_DEPTH: u32 = 4u;

// --- Bindings ---

@group(0) @binding(0) var<uniform> config: Config;
@group(0) @binding(1) var<storage, read> ptcl: array<u32>;
@group(0) @binding(2) var<storage, read> segments: array<PathSegment>;
@group(0) @binding(3) var<storage, read> info: array<u32>;
@group(0) @binding(4) var<storage, read_write> output: array<u32>;

// --- fill_path: compute per-pixel area (coverage) from segments ---

// fill_path is a direct port of fine.go fillPath().
// Computes the area (coverage) for a single pixel at (pixel_x, pixel_y)
// from tile-relative path segments, given a backdrop winding number.
fn fill_path(
    pixel_x: u32,
    pixel_y: u32,
    seg_start: u32,
    seg_count: u32,
    backdrop: i32,
    even_odd: bool,
) -> f32 {
    var area = f32(backdrop);
    let px = f32(pixel_x);
    let py = f32(pixel_y);

    for (var i = 0u; i < seg_count; i = i + 1u) {
        let seg = segments[seg_start + i];
        let delta_x = seg.point1_x - seg.point0_x;
        let delta_y = seg.point1_y - seg.point0_y;

        // fine.go line 202: y = segment.Point0[1] - float32(yi)
        let y = seg.point0_y - py;
        let y0 = clamp(y, 0.0, 1.0);
        let y1 = clamp(y + delta_y, 0.0, 1.0);
        let dy = y0 - y1;

        // fine.go line 209: y_edge contribution
        var y_edge_val = 0.0;
        if delta_x > 0.0 {
            y_edge_val = clamp(py - seg.y_edge + 1.0, 0.0, 1.0);
        } else if delta_x < 0.0 {
            y_edge_val = -clamp(py - seg.y_edge + 1.0, 0.0, 1.0);
        }
        // Handle +0.0 and -0.0: if delta_x == 0, y_edge_val stays 0.

        if dy != 0.0 {
            let vec_y_recip = 1.0 / delta_y;
            let t0 = (y0 - y) * vec_y_recip;
            let t1 = (y1 - y) * vec_y_recip;
            let start_x = seg.point0_x;
            let x0 = start_x + t0 * delta_x;
            let x1 = start_x + t1 * delta_x;
            let xmin0 = min(x0, x1);
            let xmax0 = max(x0, x1);
            let xmin_val = min(xmin0 - px, 1.0) - 1e-6;
            let xmax_val = xmax0 - px;
            let b_val = min(xmax_val, 1.0);
            let c_val = max(b_val, 0.0);
            let d_val = max(xmin_val, 0.0);
            let a_val = (b_val + 0.5 * (d_val * d_val - c_val * c_val) - xmin_val) / (xmax_val - xmin_val);
            area = area + y_edge_val + a_val * dy;
        } else if y_edge_val != 0.0 {
            area = area + y_edge_val;
        }
    }

    // Apply fill rule.
    if even_odd {
        // fine.go line 242: area = abs(area - 2.0 * round(0.5 * area))
        area = abs(area - 2.0 * round(0.5 * area));
    } else {
        // fine.go line 247: area = min(abs(area), 1.0)
        area = min(abs(area), 1.0);
    }

    return area;
}

// --- Main entry point ---
// One workgroup per tile, 256 threads = 16x16 pixels.
// Dispatch: (width_in_tiles * height_in_tiles, 1, 1) workgroups.

@compute @workgroup_size(256, 1, 1)
fn main(
    @builtin(local_invocation_id) local_id: vec3<u32>,
    @builtin(workgroup_id) wg_id: vec3<u32>,
) {
    let tile_ix = wg_id.x;
    if tile_ix >= config.width_in_tiles * config.height_in_tiles {
        return;
    }

    // Pixel coordinates within the tile.
    let local_x = local_id.x % TILE_WIDTH;
    let local_y = local_id.x / TILE_WIDTH;

    // Global pixel coordinates.
    let tile_x = tile_ix % config.width_in_tiles;
    let tile_y = tile_ix / config.width_in_tiles;
    let global_x = tile_x * TILE_WIDTH + local_x;
    let global_y = tile_y * TILE_HEIGHT + local_y;

    // Initialize pixel color to background color (matching CPU reference fine.go:28-31).
    let bg_r = f32(config.bg_color & 0xffu) / 255.0;
    let bg_g = f32((config.bg_color >> 8u) & 0xffu) / 255.0;
    let bg_b = f32((config.bg_color >> 16u) & 0xffu) / 255.0;
    let bg_a = f32((config.bg_color >> 24u) & 0xffu) / 255.0;
    var rgba = vec4<f32>(bg_r * bg_a, bg_g * bg_a, bg_b * bg_a, bg_a);

    // Working area (coverage) for this pixel.
    var area = 0.0;

    // Clip stack (fixed-size array, GPU-friendly).
    var clip_stack: array<vec4<f32>, 4>; // MAX_CLIP_DEPTH = 4
    var clip_depth = 0u;

    // Walk the PTCL command stream for this tile.
    let ptcl_base = tile_ix * PTCL_MAX_PER_TILE;
    var cmd_offset = 0u;

    loop {
        let tag = ptcl[ptcl_base + cmd_offset];
        cmd_offset = cmd_offset + 1u;

        if tag == CMD_END {
            break;
        }

        if tag == CMD_FILL {
            // Read CmdFill payload: [(segCount<<1)|evenOdd, segIndex, backdrop_bits]
            let packed = ptcl[ptcl_base + cmd_offset];
            let seg_index = ptcl[ptcl_base + cmd_offset + 1u];
            let backdrop_bits = ptcl[ptcl_base + cmd_offset + 2u];
            cmd_offset = cmd_offset + 3u;

            let seg_count = packed >> 1u;
            let even_odd = (packed & 1u) != 0u;
            let backdrop = bitcast<i32>(backdrop_bits);

            area = fill_path(local_x, local_y, seg_index, seg_count, backdrop, even_odd);

        } else if tag == CMD_SOLID {
            area = 1.0;

        } else if tag == CMD_COLOR {
            // Read packed premultiplied RGBA.
            let packed_rgba = ptcl[ptcl_base + cmd_offset];
            cmd_offset = cmd_offset + 1u;

            // Unpack premultiplied RGBA from u32.
            let r = f32(packed_rgba & 0xffu) / 255.0;
            let g = f32((packed_rgba >> 8u) & 0xffu) / 255.0;
            let b = f32((packed_rgba >> 16u) & 0xffu) / 255.0;
            let a = f32((packed_rgba >> 24u) & 0xffu) / 255.0;

            // Source-over compositing: fg = color * area, rgba = rgba * (1 - fg.a) + fg.
            let fg = vec4<f32>(r * area, g * area, b * area, a * area);
            let inv = 1.0 - fg.a;
            rgba = rgba * inv + fg;

        } else if tag == CMD_BEGIN_CLIP {
            // Push current color to clip stack, clear to transparent.
            if clip_depth < MAX_CLIP_DEPTH {
                clip_stack[clip_depth] = rgba;
                clip_depth = clip_depth + 1u;
            }
            rgba = vec4<f32>(0.0, 0.0, 0.0, 0.0);

        } else if tag == CMD_END_CLIP {
            // Read CmdEndClip payload: [blend_mode, alpha_bits]
            let blend = ptcl[ptcl_base + cmd_offset];
            let alpha_bits = ptcl[ptcl_base + cmd_offset + 1u];
            cmd_offset = cmd_offset + 2u;

            let alpha = bitcast<f32>(alpha_bits);
            _ = blend; // Only source-over (blend=0) supported currently.

            // Pop saved state from clip stack.
            if clip_depth > 0u {
                clip_depth = clip_depth - 1u;
                let saved = clip_stack[clip_depth];

                // fg = rgba * area * alpha, then saved * (1 - fg.a) + fg.
                let scale = area * alpha;
                let fg = rgba * scale;
                let inv = 1.0 - fg.a;
                rgba = saved * inv + fg;
            }

        } else {
            // Unknown command: stop to avoid infinite loop.
            break;
        }
    }

    // Clamp to [0, 1].
    rgba = clamp(rgba, vec4<f32>(0.0), vec4<f32>(1.0));

    // Pack RGBA as u32: R | (G << 8) | (B << 16) | (A << 24).
    let r_u32 = u32(rgba.x * 255.0 + 0.5);
    let g_u32 = u32(rgba.y * 255.0 + 0.5);
    let b_u32 = u32(rgba.z * 255.0 + 0.5);
    let a_u32 = u32(rgba.w * 255.0 + 0.5);
    let packed = r_u32 | (g_u32 << 8u) | (b_u32 << 16u) | (a_u32 << 24u);

    // Write to output buffer.
    // Output layout: tiles stored contiguously, 256 pixels per tile.
    if global_x < config.target_width && global_y < config.target_height {
        let pixel_idx = global_y * config.target_width + global_x;
        output[pixel_idx] = packed;
    }
}
