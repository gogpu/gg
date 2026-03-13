// Cover pass vertex + fragment shader for stencil-then-cover path rendering.
//
// The vertex shader transforms pixel coordinates to NDC (same as stencil fill).
// The fragment shader outputs the fill color (premultiplied alpha).
// The stencil test in the pipeline ensures only pixels with non-zero stencil
// values are colored.

struct Uniforms {
    viewport: vec2<f32>,  // width, height in pixels
    _pad: vec2<f32>,
    color: vec4<f32>,     // fill color (premultiplied alpha)
}

@group(0) @binding(0) var<uniform> u: Uniforms;

// --- RRect clip uniform (shared across all pipelines) ---
struct ClipParams {
    clip_rect: vec4<f32>,
    clip_radius: f32,
    clip_enabled: f32,
    _pad: vec2<f32>,
}
@group(1) @binding(0) var<uniform> clip: ClipParams;

fn rrect_clip_coverage(frag_pos: vec2<f32>) -> f32 {
    if clip.clip_enabled < 0.5 { return 1.0; }
    let cx = (clip.clip_rect.x + clip.clip_rect.z) * 0.5;
    let cy = (clip.clip_rect.y + clip.clip_rect.w) * 0.5;
    let hw = (clip.clip_rect.z - clip.clip_rect.x) * 0.5;
    let hh = (clip.clip_rect.w - clip.clip_rect.y) * 0.5;
    let r = clip.clip_radius;
    let dx = sqrt((frag_pos.x - cx) * (frag_pos.x - cx));
    let dy = sqrt((frag_pos.y - cy) * (frag_pos.y - cy));
    let qx = dx - hw + r;
    let qy = dy - hh + r;
    let mqx = (qx + sqrt(qx * qx)) * 0.5;
    let mqy = (qy + sqrt(qy * qy)) * 0.5;
    let outside = sqrt(mqx * mqx + mqy * mqy);
    let qdiff = qx - qy;
    let max_qxy = (qx + qy + sqrt(qdiff * qdiff)) * 0.5;
    let inside = (max_qxy - sqrt(max_qxy * max_qxy)) * 0.5;
    let d = outside + inside - r;
    let t_raw = d + 0.5;
    let t_pos = (t_raw + sqrt(t_raw * t_raw)) * 0.5;
    let t_diff = t_pos - 1.0;
    let t = (t_pos + 1.0 - sqrt(t_diff * t_diff)) * 0.5;
    return 1.0 - t * t * (3.0 - 2.0 * t);
}

struct CoverVertexOutput {
    @builtin(position) position: vec4<f32>,
}

@vertex
fn vs_main(@location(0) pos: vec2<f32>) -> CoverVertexOutput {
    var out: CoverVertexOutput;
    let ndc_x = pos.x / u.viewport.x * 2.0 - 1.0;
    let ndc_y = 1.0 - pos.y / u.viewport.y * 2.0;
    out.position = vec4<f32>(ndc_x, ndc_y, 0.0, 1.0);
    return out;
}

@fragment
fn fs_main(in: CoverVertexOutput) -> @location(0) vec4<f32> {
    let clip_cov = rrect_clip_coverage(in.position.xy);
    if clip_cov < 1.0 / 255.0 {
        discard;
    }
    return u.color * clip_cov;
}
