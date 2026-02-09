// sdf_circle.wgsl - SDF compute shader for circles and ellipses.
//
// Evaluates a signed distance field per pixel to produce smooth anti-aliased
// circles/ellipses. Each pixel is packed as u32 (R|G<<8|B<<16|A<<24) in a
// storage buffer. The shader reads existing pixel data, computes SDF coverage,
// and alpha-blends the shape color over the existing content.

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

// SDF for an axis-aligned ellipse at the origin with semi-axes (a, b).
// Uses the approximation: length(p / radii) - 1, scaled by min(a, b).
fn sdf_ellipse(px: f32, py: f32, a: f32, b: f32) -> f32 {
    let nx = px / a;
    let ny = py / b;
    let d = length(vec2<f32>(nx, ny)) - 1.0;
    return d * min(a, b);
}

@compute @workgroup_size(8, 8, 1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let x = gid.x;
    let y = gid.y;
    if x >= params.target_width || y >= params.target_height {
        return;
    }

    // Pixel center
    let px = f32(x) + 0.5 - params.center_x;
    let py = f32(y) + 0.5 - params.center_y;

    let dist = sdf_ellipse(px, py, params.radius_x, params.radius_y);

    // Coverage from SDF with 1-pixel AA band
    var coverage: f32;
    if params.is_stroked != 0u {
        // Ring: |dist| - half_stroke gives distance to stroke center
        let ring_dist = abs(dist) - params.half_stroke_width;
        coverage = 1.0 - smoothstep(-0.5, 0.5, ring_dist);
    } else {
        // Filled: negative distance = inside
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

    // Read existing pixel (RGBA packed as u32)
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
