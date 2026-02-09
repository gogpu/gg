// sdf_rrect.wgsl - SDF compute shader for rectangles and rounded rectangles.
//
// Evaluates a signed distance field per pixel to produce smooth anti-aliased
// rectangles/rounded rectangles. Each pixel is packed as u32 (R|G<<8|B<<16|A<<24)
// in a storage buffer. The shader reads existing pixel data, computes SDF coverage,
// and alpha-blends the shape color over the existing content.

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

// SDF for a rounded box centered at origin.
// b = half-extents, r = corner radius.
fn sdf_round_box(px: f32, py: f32, bx: f32, by: f32, r: f32) -> f32 {
    // Work in first quadrant (symmetry)
    let qx = abs(px) - bx + r;
    let qy = abs(py) - by + r;
    let outside = length(max(vec2<f32>(qx, qy), vec2<f32>(0.0)));
    let inside = min(max(qx, qy), 0.0);
    return outside + inside - r;
}

@compute @workgroup_size(8, 8, 1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let x = gid.x;
    let y = gid.y;
    if x >= params.target_width || y >= params.target_height {
        return;
    }

    // Pixel center relative to rectangle center
    let px = f32(x) + 0.5 - params.center_x;
    let py = f32(y) + 0.5 - params.center_y;

    let dist = sdf_round_box(px, py, params.half_width, params.half_height, params.corner_radius);

    // Coverage from SDF with 1-pixel AA band
    var coverage: f32;
    if params.is_stroked != 0u {
        let ring_dist = abs(dist) - params.half_stroke_width;
        coverage = 1.0 - smoothstep(-0.5, 0.5, ring_dist);
    } else {
        coverage = 1.0 - smoothstep(-0.5, 0.5, dist);
    }

    if coverage < 1.0 / 255.0 {
        return;
    }

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

    // Pack back to u32
    let ri = u32(clamp(out_r * 255.0 + 0.5, 0.0, 255.0));
    let gi = u32(clamp(out_g * 255.0 + 0.5, 0.0, 255.0));
    let bi = u32(clamp(out_b * 255.0 + 0.5, 0.0, 255.0));
    let ai = u32(clamp(out_a * 255.0 + 0.5, 0.0, 255.0));
    pixels[idx] = ri | (gi << 8u) | (bi << 16u) | (ai << 24u);
}
