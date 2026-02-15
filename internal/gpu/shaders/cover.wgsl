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

@vertex
fn vs_main(@location(0) pos: vec2<f32>) -> @builtin(position) vec4<f32> {
    let ndc_x = pos.x / u.viewport.x * 2.0 - 1.0;
    let ndc_y = 1.0 - pos.y / u.viewport.y * 2.0;
    return vec4<f32>(ndc_x, ndc_y, 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    return u.color;
}
