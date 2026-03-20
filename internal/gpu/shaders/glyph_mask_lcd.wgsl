// glyph_mask_lcd.wgsl - LCD Subpixel (ClearType) Text Rendering Shader
//
// Renders CPU-rasterized LCD glyph masks as textured quads with per-channel
// alpha compositing. The R8 atlas stores 3 texels per logical pixel
// (R coverage, G coverage, B coverage), arranged horizontally.
//
// The fragment shader samples 3 adjacent horizontal texels to get per-channel
// coverage, then composites each color channel independently for ClearType
// subpixel rendering with 3x effective horizontal resolution.
//
// References:
// - Skia GrAtlasTextOp (separate LCD pipeline, per-channel alpha)
// - FreeType LCD rendering (3x oversampling + FIR filter)
// - DirectWrite ClearType (subpixel positioning + gamma correction)

struct GlyphMaskLCDUniforms {
    transform: mat4x4<f32>,
    color: vec4<f32>,
    atlas_size: vec2<f32>,
    _pad: vec2<f32>,
}

struct VertexInput {
    @location(0) position: vec2<f32>,
    @location(1) tex_coord: vec2<f32>,
}

struct VertexOutput {
    @builtin(position) position: vec4<f32>,
    @location(0) tex_coord: vec2<f32>,
}

@group(0) @binding(0) var<uniform> uniforms: GlyphMaskLCDUniforms;
@group(0) @binding(1) var atlas_texture: texture_2d<f32>;
@group(0) @binding(2) var atlas_sampler: sampler;

// --- RRect clip uniform (shared across all pipelines) ---
struct ClipParams {
    clip_rect: vec4<f32>,
    clip_radius: f32,
    clip_enabled: f32,
    _pad: vec2<f32>,
}
@group(1) @binding(0) var<uniform> clip: ClipParams;

fn rrect_clip_coverage(frag_pos: vec2<f32>) -> f32 {
    // Text shaders: no per-pixel SDF clip. Returns 1.0.
    // See glyph_mask.wgsl for rationale (Intel Vulkan compatibility).
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
    // The UV coordinates span the 3x-wide region in the R8 atlas.
    // The logical pixel width is 1/3 of the UV width.
    // Each logical pixel maps to 3 adjacent R8 texels: [R, G, B].
    //
    // texel_step = 1.0 / atlas_width (one R8 texel in UV space).
    // The UV center (in.tex_coord.x) is at the center of the 3-texel group
    // for each fragment.

    let texel_step = 1.0 / uniforms.atlas_size.x;

    // Sample 3 adjacent texels for R, G, B coverage.
    // The center texel (green) is at the interpolated UV.
    // Red is one texel to the left, Blue one to the right.
    let uv_center = in.tex_coord;
    let cov_r = textureSample(atlas_texture, atlas_sampler, vec2<f32>(uv_center.x - texel_step, uv_center.y)).r;
    let cov_g = textureSample(atlas_texture, atlas_sampler, uv_center).r;
    let cov_b = textureSample(atlas_texture, atlas_sampler, vec2<f32>(uv_center.x + texel_step, uv_center.y)).r;

    let clip_cov = rrect_clip_coverage(in.position.xy);
    let color = uniforms.color;

    // Per-channel premultiplied alpha compositing:
    //   output.r = color.r * cov_r * color.a * clip_cov
    //   output.g = color.g * cov_g * color.a * clip_cov
    //   output.b = color.b * cov_b * color.a * clip_cov
    //   output.a = max(cov_r, cov_g, cov_b) * color.a * clip_cov
    let a_r = cov_r * color.a * clip_cov;
    let a_g = cov_g * color.a * clip_cov;
    let a_b = cov_b * color.a * clip_cov;
    let a_max = max(a_r, max(a_g, a_b));

    return vec4<f32>(color.r * a_r, color.g * a_g, color.b * a_b, a_max);
}
