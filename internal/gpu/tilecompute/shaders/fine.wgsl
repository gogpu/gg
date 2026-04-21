// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT
//
// fine.wgsl — Per-pixel rasterization driven by PTCL command streams.
//
// CPU reference: tilecompute/fine.go fineRasterizeTile(), fillPath()
//
// Thread model (matches Vello):
//   workgroup_size(4, 16, 1) = 64 threads per tile.
//   Each thread handles PIXELS_PER_THREAD=4 consecutive horizontal pixels.
//   4 threads × 4 pixels = 16 columns, 16 rows = 256 pixels per tile.
//   Dispatch: (width_in_tiles, height_in_tiles, 1) workgroups.
//
// Blend stack (matches Vello):
//   First BLEND_STACK_SPLIT=4 clip levels stored as packed u32 in registers.
//   Deeper levels spill to blend_spill SSBO. Uses pack4x8unorm/unpack4x8unorm.
//
// PTCL format (word 0 = blend_offset, word 1+ = commands):
//   CMD_END        -> break (done with this tile)
//   CMD_FILL       -> compute per-pixel area via fill_path (4 pixels per thread)
//   CMD_SOLID      -> area = 1.0 for all pixels
//   CMD_COLOR      -> source-over blend: fg = color * area, rgba = rgba*(1-fg.a) + fg
//   CMD_BEGIN_CLIP -> push rgba to blend stack (packed u32), clear
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
const PIXELS_PER_THREAD: u32 = 4u;

// The "split" point between using local memory for the blend stack and
// spilling to the blend_spill buffer. Matches Vello's BLEND_STACK_SPLIT.
const BLEND_STACK_SPLIT: u32 = 4u;

const CMD_END: u32 = 0u;
const CMD_FILL: u32 = 1u;
const CMD_SOLID: u32 = 3u;
const CMD_COLOR: u32 = 5u;
const CMD_BEGIN_CLIP: u32 = 10u;
const CMD_END_CLIP: u32 = 11u;

// Maximum PTCL words per tile (must match coarse shader).
const PTCL_MAX_PER_TILE: u32 = 1024u;

// --- Bindings ---

@group(0) @binding(0) var<uniform> config: Config;
@group(0) @binding(1) var<storage, read> ptcl: array<u32>;
@group(0) @binding(2) var<storage, read> segments: array<PathSegment>;
@group(0) @binding(3) var<storage, read> info: array<u32>;
@group(0) @binding(4) var<storage, read_write> output: array<u32>;
@group(0) @binding(5) var<storage, read_write> blend_spill: array<u32>;

// --- fill_path: compute per-pixel area (coverage) for PIXELS_PER_THREAD pixels ---
//
// Direct port of Vello fine.wgsl fill_path(). Returns area for 4 consecutive
// horizontal pixels starting at local_xy. Uses output parameter (naga #1930).
fn fill_path(
    seg_start: u32,
    seg_count: u32,
    backdrop: i32,
    even_odd: bool,
    local_xy: vec2<f32>,
    result: ptr<function, array<f32, PIXELS_PER_THREAD>>,
) {
    var area: array<f32, PIXELS_PER_THREAD>;
    let backdrop_f = f32(backdrop);
    for (var i = 0u; i < PIXELS_PER_THREAD; i = i + 1u) {
        area[i] = backdrop_f;
    }

    for (var si = 0u; si < seg_count; si = si + 1u) {
        let seg = segments[seg_start + si];
        let delta_x = seg.point1_x - seg.point0_x;
        let delta_y = seg.point1_y - seg.point0_y;

        let y = seg.point0_y - local_xy.y;
        let y0 = clamp(y, 0.0, 1.0);
        let y1 = clamp(y + delta_y, 0.0, 1.0);
        let dy = y0 - y1;

        if dy != 0.0 {
            let vec_y_recip = 1.0 / delta_y;
            let t0 = (y0 - y) * vec_y_recip;
            let t1 = (y1 - y) * vec_y_recip;
            let startx = seg.point0_x - local_xy.x;
            let x0 = startx + t0 * delta_x;
            let x1 = startx + t1 * delta_x;
            let xmin0 = min(x0, x1);
            let xmax0 = max(x0, x1);
            for (var i = 0u; i < PIXELS_PER_THREAD; i = i + 1u) {
                let i_f = f32(i);
                let xmin = min(xmin0 - i_f, 1.0) - 1e-6;
                let xmax = xmax0 - i_f;
                let b = min(xmax, 1.0);
                let c = max(b, 0.0);
                let d = max(xmin, 0.0);
                let a = (b + 0.5 * (d * d - c * c) - xmin) / (xmax - xmin);
                area[i] += a * dy;
            }
        }

        // y_edge contribution (Vello pattern).
        let y_edge = sign(delta_x) * clamp(local_xy.y - seg.y_edge + 1.0, 0.0, 1.0);
        for (var i = 0u; i < PIXELS_PER_THREAD; i = i + 1u) {
            area[i] += y_edge;
        }
    }

    // Apply fill rule.
    if even_odd {
        for (var i = 0u; i < PIXELS_PER_THREAD; i = i + 1u) {
            let a = area[i];
            area[i] = abs(a - 2.0 * round(0.5 * a));
        }
    } else {
        for (var i = 0u; i < PIXELS_PER_THREAD; i = i + 1u) {
            area[i] = min(abs(area[i]), 1.0);
        }
    }

    *result = area;
}

// --- Main entry point ---
// Thread model: workgroup_size(4, 16, 1) = 64 threads per tile.
// Each thread processes PIXELS_PER_THREAD=4 consecutive horizontal pixels.
// Dispatch: (width_in_tiles, height_in_tiles, 1) workgroups.

@compute @workgroup_size(4, 16, 1)
fn main(
    @builtin(local_invocation_id) local_id: vec3<u32>,
    @builtin(workgroup_id) wg_id: vec3<u32>,
) {
    // One workgroup per tile.
    let tile_ix = wg_id.y * config.width_in_tiles + wg_id.x;
    if tile_ix >= config.width_in_tiles * config.height_in_tiles {
        return;
    }

    // local_id.x: 0..3 (4 threads per row)
    // local_id.y: 0..15 (16 rows)
    // Each thread handles 4 consecutive horizontal pixels.
    let local_xy = vec2<f32>(f32(local_id.x * PIXELS_PER_THREAD), f32(local_id.y));

    // Global pixel coordinates (base of this thread's 4-pixel strip).
    let global_base_x = wg_id.x * TILE_WIDTH + local_id.x * PIXELS_PER_THREAD;
    let global_y = wg_id.y * TILE_HEIGHT + local_id.y;

    // Initialize pixel colors to background color.
    let bg_r = f32(config.bg_color & 0xffu) / 255.0;
    let bg_g = f32((config.bg_color >> 8u) & 0xffu) / 255.0;
    let bg_b = f32((config.bg_color >> 16u) & 0xffu) / 255.0;
    let bg_a = f32((config.bg_color >> 24u) & 0xffu) / 255.0;
    let bg = vec4<f32>(bg_r * bg_a, bg_g * bg_a, bg_b * bg_a, bg_a);

    var rgba: array<vec4<f32>, PIXELS_PER_THREAD>;
    for (var i = 0u; i < PIXELS_PER_THREAD; i = i + 1u) {
        rgba[i] = bg;
    }

    // Working area (coverage) for PIXELS_PER_THREAD pixels.
    var area: array<f32, PIXELS_PER_THREAD>;

    // Packed u32 blend stack — first BLEND_STACK_SPLIT levels in registers.
    // Deeper levels spill to blend_spill SSBO.
    var blend_stack: array<array<u32, PIXELS_PER_THREAD>, BLEND_STACK_SPLIT>;
    var clip_depth = 0u;

    // Walk the PTCL command stream for this tile.
    // Word 0 = blend_offset (for blend_spill SSBO), commands start at word 1.
    let ptcl_base = tile_ix * PTCL_MAX_PER_TILE;
    let blend_offset = ptcl[ptcl_base];
    var cmd_ix = ptcl_base + 1u;

    loop {
        let tag = ptcl[cmd_ix];
        if tag == CMD_END {
            break;
        }

        // switch generates OpSwitch in SPIR-V (structured control flow).
        // Adreno LLVM miscompiles if/else if chains inside loops (llama.cpp #5186).
        switch tag {
            case CMD_FILL: {
                // Read CmdFill payload: [(segCount<<1)|evenOdd, segIndex, backdrop_bits]
                let packed = ptcl[cmd_ix + 1u];
                let seg_index = ptcl[cmd_ix + 2u];
                let backdrop_bits = ptcl[cmd_ix + 3u];
                cmd_ix = cmd_ix + 4u;

                let seg_count = packed >> 1u;
                let even_odd = (packed & 1u) != 0u;
                let backdrop = bitcast<i32>(backdrop_bits);

                fill_path(seg_index, seg_count, backdrop, even_odd, local_xy, &area);
            }
            case CMD_SOLID: {
                for (var i = 0u; i < PIXELS_PER_THREAD; i = i + 1u) {
                    area[i] = 1.0;
                }
                cmd_ix = cmd_ix + 1u;
            }
            case CMD_COLOR: {
                // Read packed premultiplied RGBA.
                let packed_rgba = ptcl[cmd_ix + 1u];
                cmd_ix = cmd_ix + 2u;

                // Unpack premultiplied RGBA from u32.
                let fg = vec4<f32>(
                    f32(packed_rgba & 0xffu) / 255.0,
                    f32((packed_rgba >> 8u) & 0xffu) / 255.0,
                    f32((packed_rgba >> 16u) & 0xffu) / 255.0,
                    f32((packed_rgba >> 24u) & 0xffu) / 255.0,
                );

                // Source-over compositing for each of 4 pixels.
                for (var i = 0u; i < PIXELS_PER_THREAD; i = i + 1u) {
                    let fg_i = fg * area[i];
                    rgba[i] = rgba[i] * (1.0 - fg_i.a) + fg_i;
                }
            }
            case CMD_BEGIN_CLIP: {
                // Push current color to blend stack (packed u32), clear to transparent.
                if clip_depth < BLEND_STACK_SPLIT {
                    for (var i = 0u; i < PIXELS_PER_THREAD; i = i + 1u) {
                        blend_stack[clip_depth][i] = pack4x8unorm(rgba[i]);
                        rgba[i] = vec4<f32>(0.0);
                    }
                } else {
                    // Spill to blend_spill SSBO.
                    let blend_in_scratch = clip_depth - BLEND_STACK_SPLIT;
                    let local_tile_ix = local_id.x * PIXELS_PER_THREAD + local_id.y * TILE_WIDTH;
                    let local_blend_start = blend_offset + blend_in_scratch * TILE_WIDTH * TILE_HEIGHT + local_tile_ix;
                    for (var i = 0u; i < PIXELS_PER_THREAD; i = i + 1u) {
                        blend_spill[local_blend_start + i] = pack4x8unorm(rgba[i]);
                        rgba[i] = vec4<f32>(0.0);
                    }
                }
                clip_depth = clip_depth + 1u;
                cmd_ix = cmd_ix + 1u;
            }
            case CMD_END_CLIP: {
                // Read CmdEndClip payload: [blend_mode, alpha_bits]
                let blend = ptcl[cmd_ix + 1u];
                let alpha_bits = ptcl[cmd_ix + 2u];
                cmd_ix = cmd_ix + 3u;

                let alpha = bitcast<f32>(alpha_bits);
                _ = blend; // Only source-over (blend=0) supported currently.

                clip_depth = clip_depth - 1u;

                // Pop saved state from blend stack.
                for (var i = 0u; i < PIXELS_PER_THREAD; i = i + 1u) {
                    var bg_rgba: u32;
                    if clip_depth < BLEND_STACK_SPLIT {
                        bg_rgba = blend_stack[clip_depth][i];
                    } else {
                        let blend_in_scratch = clip_depth - BLEND_STACK_SPLIT;
                        let local_tile_ix = local_id.x * PIXELS_PER_THREAD + local_id.y * TILE_WIDTH;
                        let local_blend_start = blend_offset + blend_in_scratch * TILE_WIDTH * TILE_HEIGHT + local_tile_ix;
                        bg_rgba = blend_spill[local_blend_start + i];
                    }
                    let saved = unpack4x8unorm(bg_rgba);

                    // fg = rgba * area * alpha, then saved * (1 - fg.a) + fg.
                    let fg = rgba[i] * area[i] * alpha;
                    let inv = 1.0 - fg.a;
                    rgba[i] = saved * inv + fg;
                }
            }
            default: {
                // Unknown command: stop to avoid infinite loop.
                break;
            }
        }
    }

    // Clamp and write 4 pixels.
    for (var i = 0u; i < PIXELS_PER_THREAD; i = i + 1u) {
        let px = global_base_x + i;
        if px < config.target_width && global_y < config.target_height {
            let c = clamp(rgba[i], vec4<f32>(0.0), vec4<f32>(1.0));

            // Pack RGBA as u32: R | (G << 8) | (B << 16) | (A << 24).
            let r_u32 = u32(c.x * 255.0 + 0.5);
            let g_u32 = u32(c.y * 255.0 + 0.5);
            let b_u32 = u32(c.z * 255.0 + 0.5);
            let a_u32 = u32(c.w * 255.0 + 0.5);
            let packed = r_u32 | (g_u32 << 8u) | (b_u32 << 16u) | (a_u32 << 24u);

            let pixel_idx = global_y * config.target_width + px;
            output[pixel_idx] = packed;
        }
    }
}
