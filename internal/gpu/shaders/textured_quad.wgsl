// textured_quad.wgsl - Textured Quad Rendering Shader (Tier 3)
//
// Renders image patterns as textured quads with full affine transform support.
// The vertex shader applies an ortho projection (pixel-to-NDC), and the
// fragment shader samples the image texture with bilinear filtering.
//
// Premultiplied alpha throughout: the source image is expected in premultiplied
// RGBA format, and the fragment shader applies opacity as a uniform multiplier.
//
// NOTE: All math avoids naga-problematic builtins (smoothstep, clamp, abs,
// min, max, select). Only sqrt() is used where needed.
//
// References:
// - Skia GrFillRectOp + GrTextureProxy (textured quad is fundamental GPU op)
// - Qt Quick QSGSimpleTextureNode (basic compositing primitive)
// - Vello DrawImage (scene command → atlas → textured quad)

struct ImageUniforms {
    transform: mat4x4<f32>,  // ortho projection matrix
    opacity: f32,            // opacity multiplier (0.0 to 1.0)
    _pad: vec3<f32>,
}

struct VertexInput {
    @location(0) position: vec2<f32>,   // quad corner in pixel coords
    @location(1) tex_coord: vec2<f32>,  // UV coordinates (0..1 range)
}

struct VertexOutput {
    @builtin(position) position: vec4<f32>,
    @location(0) tex_coord: vec2<f32>,
}

@group(0) @binding(0) var<uniform> uniforms: ImageUniforms;
@group(0) @binding(1) var image_texture: texture_2d<f32>;
@group(0) @binding(2) var image_sampler: sampler;

// --- RRect clip uniform (shared across all pipelines) ---
struct ClipParams {
    clip_rect: vec4<f32>,   // (left, top, right, bottom) device pixels
    clip_radius: f32,
    clip_enabled: f32,      // 0.0 = no clip, 1.0 = active
    _pad: vec2<f32>,
}
@group(1) @binding(0) var<uniform> clip: ClipParams;

fn rrect_clip_coverage(frag_pos: vec2<f32>) -> f32 {
    // Image shaders: no per-pixel SDF clip. Returns 1.0.
    //
    // Same reasoning as glyph_mask.wgsl: textureSample combined with
    // complex SDF math causes Intel Vulkan register pressure issues.
    // Image clipping is handled by hardware scissor (GPU-CLIP-001).
    // RRect clip for images will use stencil buffer (GPU-CLIP-003).
    return 1.0;
}

@vertex
fn vs_main(in: VertexInput) -> VertexOutput {
    var out: VertexOutput;
    let p = vec4<f32>(in.position, 0.0, 1.0);
    let col0 = uniforms.transform[0];
    let col1 = uniforms.transform[1];
    let col2 = uniforms.transform[2];
    let col3 = uniforms.transform[3];
    let pos = p.x * col0 + p.y * col1 + p.z * col2 + p.w * col3;
    out.position = pos;
    out.tex_coord = in.tex_coord;
    return out;
}

@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4<f32> {
    let texel = textureSample(image_texture, image_sampler, in.tex_coord);
    let clip_cov = rrect_clip_coverage(in.position.xy);

    // Apply opacity to premultiplied texel (scale all channels uniformly).
    let opacity = uniforms.opacity * clip_cov;
    return texel * opacity;
}
