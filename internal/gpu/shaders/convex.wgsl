// convex.wgsl - Vertex + Fragment shader for convex polygon rendering with
// per-edge analytic anti-aliasing.
//
// Each vertex carries a pixel position, a coverage value (1.0 = interior,
// 0.0 = outermost AA fringe), and a premultiplied RGBA color.
//
// The vertex shader transforms pixel coordinates to NDC.
// The fragment shader outputs color * coverage, discarding fully transparent
// fragments.

struct Uniforms {
    viewport: vec2<f32>,  // width, height in pixels
    _pad: vec2<f32>,
}

struct VertexInput {
    @location(0) position: vec2<f32>,  // pixel position
    @location(1) coverage: f32,        // 1.0 = interior, 0.0..1.0 = AA ramp
    @location(2) color: vec4<f32>,     // premultiplied RGBA
}

struct VertexOutput {
    @builtin(position) clip_position: vec4<f32>,
    @location(0) coverage: f32,
    @location(1) color: vec4<f32>,
}

@group(0) @binding(0) var<uniform> u: Uniforms;

@vertex
fn vs_main(in: VertexInput) -> VertexOutput {
    var out: VertexOutput;
    let ndc_x = in.position.x / u.viewport.x * 2.0 - 1.0;
    let ndc_y = 1.0 - in.position.y / u.viewport.y * 2.0;
    out.clip_position = vec4<f32>(ndc_x, ndc_y, 0.0, 1.0);
    out.coverage = in.coverage;
    out.color = in.color;
    return out;
}

@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4<f32> {
    if in.coverage < 1.0 / 255.0 {
        discard;
    }
    return in.color * in.coverage;
}
