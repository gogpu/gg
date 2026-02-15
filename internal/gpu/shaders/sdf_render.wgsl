// sdf_render.wgsl - Vertex + Fragment shader for SDF shape rendering.
//
// Renders SDF shapes (circles, ellipses, rectangles, rounded rectangles)
// via a bounding-quad approach. Each shape becomes a screen-aligned quad;
// the fragment shader evaluates the signed distance function per pixel for
// smooth anti-aliased coverage.
//
// Shape kinds (encoded in shape_kind):
//   0 = circle/ellipse (param1=radius_x, param2=radius_y, param3=unused)
//   1 = rounded rectangle (param1=half_width, param2=half_height, param3=corner_radius)
//
// NOTE: All math avoids naga-problematic builtins (smoothstep, clamp, abs,
// min, max, select). Only sqrt() is used. See sdf_batch.wgsl header for
// the full list of naga SPIR-V backend issues.

struct Uniforms {
    viewport: vec2<f32>,   // width, height in pixels
    _pad: vec2<f32>,
}

struct VertexInput {
    @location(0) position: vec2<f32>,  // quad corner in pixel coords
    @location(1) local: vec2<f32>,     // offset from shape center
    @location(2) shape_kind: f32,      // 0=circle/ellipse, 1=rrect (as f32)
    @location(3) param1: f32,          // radius_x or half_width
    @location(4) param2: f32,          // radius_y or half_height
    @location(5) param3: f32,          // corner_radius (rrect) or 0
    @location(6) half_stroke: f32,     // half stroke width (0 for filled)
    @location(7) is_stroked: f32,      // 1.0 for stroked, 0.0 for filled
    @location(8) color: vec4<f32>,     // premultiplied RGBA
}

struct VertexOutput {
    @builtin(position) clip_position: vec4<f32>,
    @location(0) local: vec2<f32>,
    @location(1) shape_kind: f32,
    @location(2) param1: f32,
    @location(3) param2: f32,
    @location(4) param3: f32,
    @location(5) half_stroke: f32,
    @location(6) is_stroked: f32,
    @location(7) color: vec4<f32>,
}

@group(0) @binding(0) var<uniform> u: Uniforms;

@vertex
fn vs_main(in: VertexInput) -> VertexOutput {
    var out: VertexOutput;
    // Transform pixel coordinates to NDC.
    let ndc_x = in.position.x / u.viewport.x * 2.0 - 1.0;
    let ndc_y = 1.0 - in.position.y / u.viewport.y * 2.0;
    out.clip_position = vec4<f32>(ndc_x, ndc_y, 0.0, 1.0);
    out.local = in.local;
    out.shape_kind = in.shape_kind;
    out.param1 = in.param1;
    out.param2 = in.param2;
    out.param3 = in.param3;
    out.half_stroke = in.half_stroke;
    out.is_stroked = in.is_stroked;
    out.color = in.color;
    return out;
}

@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4<f32> {
    let dx = in.local.x;
    let dy = in.local.y;

    // --- Circle/ellipse SDF (kind == 0) ---
    let nx = dx / in.param1;
    let ny = dy / in.param2;
    let elen = sqrt(nx * nx + ny * ny);
    // min(param1, param2) via arithmetic
    let rdiff = in.param1 - in.param2;
    let min_r = (in.param1 + in.param2 - sqrt(rdiff * rdiff)) * 0.5;
    let d_circle = (elen - 1.0) * min_r;

    // --- Rounded rectangle SDF (kind == 1) ---
    // abs via sqrt(x*x)
    let apx = sqrt(dx * dx);
    let apy = sqrt(dy * dy);
    let qx = apx - in.param1 + in.param3;
    let qy = apy - in.param2 + in.param3;
    // max(q, 0) via (q + sqrt(q*q)) * 0.5
    let mqx = (qx + sqrt(qx * qx)) * 0.5;
    let mqy = (qy + sqrt(qy * qy)) * 0.5;
    let outside = sqrt(mqx * mqx + mqy * mqy);
    // max(qx, qy) via arithmetic
    let qdiff = qx - qy;
    let max_qxy = (qx + qy + sqrt(qdiff * qdiff)) * 0.5;
    // min(max_qxy, 0) via arithmetic
    let inside = (max_qxy - sqrt(max_qxy * max_qxy)) * 0.5;
    let d_rrect = outside + inside - in.param3;

    // Select distance based on shape kind using arithmetic.
    let kind_f = in.shape_kind;
    let kdiff = kind_f - 1.0;
    let is_rrect = (kind_f + 1.0 - sqrt(kdiff * kdiff)) * 0.5;
    let is_circle = 1.0 - is_rrect;
    let d = d_circle * is_circle + d_rrect * is_rrect;

    // --- Stroke transformation ---
    let abs_d = sqrt(d * d);
    let effective_dist = d + in.is_stroked * (abs_d - in.half_stroke - d);

    // --- Coverage via smoothstep (arithmetic implementation) ---
    let t_raw = effective_dist + 0.5;
    // clamp(t_raw, 0, 1) via arithmetic
    let t_pos = (t_raw + sqrt(t_raw * t_raw)) * 0.5;
    let t_diff = t_pos - 1.0;
    let t = (t_pos + 1.0 - sqrt(t_diff * t_diff)) * 0.5;
    let coverage = 1.0 - t * t * (3.0 - 2.0 * t);

    // Discard fully transparent pixels.
    if coverage < 1.0 / 255.0 {
        discard;
    }

    // Output premultiplied color scaled by coverage.
    return in.color * coverage;
}
