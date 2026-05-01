// Depth clip shader for GPU-CLIP-003a: depth-based arbitrary path clipping.
//
// Renders clip path geometry to the depth buffer ONLY (no color output).
// Writes depth = 0.0 where clip geometry exists, leaving depth = 1.0 (clear)
// where clip is absent.
//
// Content pipelines then use DepthCompare=GreaterEqual with fragment Z=0.0:
//   Where clip drawn:     buffer=0.0, fragment=0.0 => 0.0 >= 0.0 => PASS
//   Where clip NOT drawn: buffer=1.0, fragment=0.0 => 0.0 >= 1.0 => FAIL
//
// This follows the Flutter Impeller / Skia Graphite pattern:
//   - Depth buffer for clip discrimination
//   - Stencil buffer exclusively for Tier 2b path fill
//   - No stencil/depth conflict

struct Uniforms {
    viewport: vec2<f32>,    // width, height in pixels
    _pad: vec2<f32>,
}

@group(0) @binding(0) var<uniform> u: Uniforms;

@vertex
fn vs_main(@location(0) pos: vec2<f32>) -> @builtin(position) vec4<f32> {
    let ndc_x = pos.x / u.viewport.x * 2.0 - 1.0;
    let ndc_y = 1.0 - pos.y / u.viewport.y * 2.0;
    // Z = 0.0: marks clip region. Clear value is 1.0, so this creates
    // the depth contrast needed for GreaterEqual testing.
    return vec4<f32>(ndc_x, ndc_y, 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    // No color output -- WriteMask=None on the pipeline.
    // Fragment shader required for backend compatibility.
    return vec4<f32>(0.0, 0.0, 0.0, 0.0);
}
