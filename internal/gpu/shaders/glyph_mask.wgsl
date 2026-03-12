// glyph_mask.wgsl - Alpha Mask Text Rendering Shader (Tier 6)
//
// Renders CPU-rasterized glyph alpha masks as textured quads. The atlas
// stores R8 (single-channel) coverage data produced by AnalyticFiller.
// The fragment shader multiplies the sampled alpha by the text color
// and outputs premultiplied alpha.
//
// This is the Skia/Chrome approach: CPU rasterizes at exact pixel size
// for pixel-perfect hinting, GPU composites as alpha-textured quad.
//
// References:
// - Skia GrAtlasTextOp (R8 atlas compositing)
// - Chrome cc::GlyphAtlas (alpha mask cache + GPU upload)

// ============================================================================
// Uniform structures
// ============================================================================

struct GlyphMaskUniforms {
    // Transform matrix: column-major mat4x4 for pixel-to-NDC conversion.
    transform: mat4x4<f32>,

    // Text color (RGBA, premultiplied alpha).
    color: vec4<f32>,
}

// ============================================================================
// Vertex input/output structures
// ============================================================================

struct VertexInput {
    // Position of quad vertex in local space.
    @location(0) position: vec2<f32>,

    // UV coordinates for sampling R8 alpha atlas.
    @location(1) tex_coord: vec2<f32>,

    // Per-vertex text color (RGBA, premultiplied alpha).
    @location(2) color: vec4<f32>,
}

struct VertexOutput {
    // Clip-space position for rasterization.
    @builtin(position) position: vec4<f32>,

    // Interpolated UV for fragment shader.
    @location(0) tex_coord: vec2<f32>,

    // Interpolated text color.
    @location(1) color: vec4<f32>,
}

// ============================================================================
// Bindings
// ============================================================================

@group(0) @binding(0) var<uniform> uniforms: GlyphMaskUniforms;
@group(0) @binding(1) var atlas_texture: texture_2d<f32>;
@group(0) @binding(2) var atlas_sampler: sampler;

// ============================================================================
// Vertex shader
// ============================================================================

@vertex
fn vs_main(in: VertexInput) -> VertexOutput {
    var out: VertexOutput;

    // Apply 2D transform (using mat4x4 for alignment, treating as affine 2D).
    // Manual column multiply: naga SPIR-V backend does not yet support
    // mat4x4 * vec4 as a single binary operator.
    let p = vec4<f32>(in.position, 0.0, 1.0);
    let col0 = uniforms.transform[0];
    let col1 = uniforms.transform[1];
    let col2 = uniforms.transform[2];
    let col3 = uniforms.transform[3];
    let pos = p.x * col0 + p.y * col1 + p.z * col2 + p.w * col3;
    out.position = pos;

    // Pass through texture coordinates.
    out.tex_coord = in.tex_coord;

    // Per-vertex color (allows batching glyphs with different colors).
    out.color = in.color;

    return out;
}

// ============================================================================
// Fragment shader
// ============================================================================

// fs_main: Sample R8 alpha atlas and composite with text color.
// The atlas stores pre-rasterized coverage (256-level AA from AnalyticFiller).
// Output is premultiplied alpha: rgb = color.rgb * alpha, a = color.a * alpha.
@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4<f32> {
    // Sample the R8 atlas texture. The .r channel contains the alpha coverage.
    let alpha = textureSample(atlas_texture, atlas_sampler, in.tex_coord).r;

    // Multiply coverage by the text color's alpha for final alpha.
    let a = alpha * in.color.a;

    // Premultiplied alpha output.
    return vec4<f32>(in.color.rgb * a, a);
}
