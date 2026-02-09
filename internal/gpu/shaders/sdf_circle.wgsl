// sdf_circle.wgsl - SDF compute shader for circles and ellipses.
//
// Evaluates a signed distance field per pixel to produce smooth anti-aliased
// circles/ellipses. Each pixel is packed as u32 (R|G<<8|B<<16|A<<24) in a
// storage buffer. The shader reads existing pixel data, computes SDF coverage,
// and alpha-blends the shape color over the existing content.
//
// NOTE: All math is inlined because naga's SPIR-V backend has issues with:
// 1) smoothstep/clamp/select argument reordering
// 2) Function calls with if/return patterns ("call result not found")
// So we avoid ALL builtins except sqrt() and use inline var+if instead of functions.

struct Params {
    center_x: f32,
    center_y: f32,
    radius_x: f32,
    radius_y: f32,
    half_stroke_width: f32,
    is_stroked: u32,
    color_r: f32,
    color_g: f32,
    color_b: f32,
    color_a: f32,
    target_width: u32,
    target_height: u32,
}

@group(0) @binding(0) var<uniform> params: Params;
@group(0) @binding(1) var<storage, read_write> pixels: array<u32>;

@compute @workgroup_size(8, 8, 1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let x = gid.x;
    let y = gid.y;
    if x >= params.target_width || y >= params.target_height {
        return;
    }

    // Pixel center relative to shape center
    let px = f32(x) + 0.5 - params.center_x;
    let py = f32(y) + 0.5 - params.center_y;

    // SDF for ellipse: length(p / radii) - 1, scaled by min(rx, ry)
    let nx = px / params.radius_x;
    let ny = py / params.radius_y;
    let len = sqrt(nx * nx + ny * ny);

    // min(rx, ry) via arithmetic: min(a,b) = (a+b - |a-b|) / 2
    let rdiff = params.radius_x - params.radius_y;
    let min_r = (params.radius_x + params.radius_y - sqrt(rdiff * rdiff)) * 0.5;

    let d = (len - 1.0) * min_r;

    // For stroked shapes: effective = |d| - halfStroke
    // For filled shapes: effective = d
    // Use arithmetic: is_stroked_f * (|d| - half - d) + d
    let is_stroked_f = f32(params.is_stroked);
    let abs_d = sqrt(d * d);
    let effective_dist = d + is_stroked_f * (abs_d - params.half_stroke_width - d);

    // Early return: fully outside AA band
    if effective_dist > 0.5 {
        return;
    }

    // Fully inside AA band: coverage = 1.0
    if effective_dist < -0.5 {
        // Full coverage blend
        let src_a = params.color_a;
        let src_r = params.color_r;
        let src_g = params.color_g;
        let src_b = params.color_b;
        let idx = y * params.target_width + x;
        let existing = pixels[idx];
        let dst_r = f32(existing & 0xFFu) / 255.0;
        let dst_g = f32((existing >> 8u) & 0xFFu) / 255.0;
        let dst_b = f32((existing >> 16u) & 0xFFu) / 255.0;
        let dst_a = f32((existing >> 24u) & 0xFFu) / 255.0;
        let inv_src_a = 1.0 - src_a;
        let out_r = src_r + dst_r * inv_src_a;
        let out_g = src_g + dst_g * inv_src_a;
        let out_b = src_b + dst_b * inv_src_a;
        let out_a = src_a + dst_a * inv_src_a;
        pixels[idx] = u32(out_r * 255.0 + 0.5) | (u32(out_g * 255.0 + 0.5) << 8u) | (u32(out_b * 255.0 + 0.5) << 16u) | (u32(out_a * 255.0 + 0.5) << 24u);
        return;
    }

    // AA band: -0.5 <= effective_dist <= 0.5
    // t = effective_dist + 0.5 is in range [0, 1], no clamping needed
    let t = effective_dist + 0.5;
    let coverage = 1.0 - t * t * (3.0 - 2.0 * t);

    // Source color (premultiplied alpha)
    let src_a = params.color_a * coverage;
    let src_r = params.color_r * coverage;
    let src_g = params.color_g * coverage;
    let src_b = params.color_b * coverage;

    // Read existing pixel
    let idx = y * params.target_width + x;
    let existing = pixels[idx];
    let dst_r = f32(existing & 0xFFu) / 255.0;
    let dst_g = f32((existing >> 8u) & 0xFFu) / 255.0;
    let dst_b = f32((existing >> 16u) & 0xFFu) / 255.0;
    let dst_a = f32((existing >> 24u) & 0xFFu) / 255.0;

    // Source-over compositing (premultiplied)
    let inv_src_a = 1.0 - src_a;
    let out_r = src_r + dst_r * inv_src_a;
    let out_g = src_g + dst_g * inv_src_a;
    let out_b = src_b + dst_b * inv_src_a;
    let out_a = src_a + dst_a * inv_src_a;

    // Pack back to u32 (values are in [0,1] range, no clamping needed)
    pixels[idx] = u32(out_r * 255.0 + 0.5) | (u32(out_g * 255.0 + 0.5) << 8u) | (u32(out_b * 255.0 + 0.5) << 16u) | (u32(out_a * 255.0 + 0.5) << 24u);
}
