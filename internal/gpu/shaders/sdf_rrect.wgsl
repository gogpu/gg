// sdf_rrect.wgsl - SDF compute shader for rectangles and rounded rectangles.
//
// Evaluates a signed distance field per pixel to produce smooth anti-aliased
// rectangles/rounded rectangles. Each pixel is packed as u32 (R|G<<8|B<<16|A<<24)
// in a storage buffer. The shader reads existing pixel data, computes SDF coverage,
// and alpha-blends the shape color over the existing content.
//
// NOTE: All math is inlined because naga's SPIR-V backend has issues with:
// 1) smoothstep/clamp/select argument reordering
// 2) Function calls with if/return patterns ("call result not found")
// So we avoid ALL builtins except sqrt() and use inline var+if instead of functions.

struct Params {
    center_x: f32,
    center_y: f32,
    half_width: f32,
    half_height: f32,
    corner_radius: f32,
    half_stroke_width: f32,
    is_stroked: u32,
    color_r: f32,
    color_g: f32,
    color_b: f32,
    color_a: f32,
    target_width: u32,
    target_height: u32,
    padding: u32,
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

    // Pixel center relative to rectangle center
    let px_raw = f32(x) + 0.5 - params.center_x;
    let py_raw = f32(y) + 0.5 - params.center_y;

    // SDF for rounded box: work in first quadrant (symmetry)
    // abs via sqrt(x*x) to avoid naga var issues
    let apx = sqrt(px_raw * px_raw);
    let apy = sqrt(py_raw * py_raw);

    let qx = apx - params.half_width + params.corner_radius;
    let qy = apy - params.half_height + params.corner_radius;

    // max(q, 0) via (q + |q|) / 2 = (q + sqrt(q*q)) / 2
    let mqx = (qx + sqrt(qx * qx)) * 0.5;
    let mqy = (qy + sqrt(qy * qy)) * 0.5;

    let outside = sqrt(mqx * mqx + mqy * mqy);

    // max(qx, qy) via (qx + qy + |qx - qy|) / 2
    let qdiff = qx - qy;
    let max_qxy = (qx + qy + sqrt(qdiff * qdiff)) * 0.5;

    // min(max_qxy, 0) via (v - |v|) / 2
    let inside = (max_qxy - sqrt(max_qxy * max_qxy)) * 0.5;

    let dist = outside + inside - params.corner_radius;

    // For stroked shapes: effective = |d| - halfStroke
    // For filled shapes: effective = d
    let is_stroked_f = f32(params.is_stroked);
    let abs_dist = sqrt(dist * dist);
    let effective_dist = dist + is_stroked_f * (abs_dist - params.half_stroke_width - dist);

    // Early return: fully outside AA band
    if effective_dist > 0.5 {
        return;
    }

    // Fully inside AA band: coverage = 1.0
    if effective_dist < -0.5 {
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

    pixels[idx] = u32(out_r * 255.0 + 0.5) | (u32(out_g * 255.0 + 0.5) << 8u) | (u32(out_b * 255.0 + 0.5) << 16u) | (u32(out_a * 255.0 + 0.5) << 24u);
}
