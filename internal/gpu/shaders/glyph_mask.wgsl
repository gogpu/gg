// glyph_mask.wgsl - Alpha Mask Text Rendering Shader (Tier 6)
//
// Renders CPU-rasterized glyph alpha masks as textured quads. The atlas
// stores R8 (single-channel) coverage data produced by AnalyticFiller.
//
// Two rendering modes:
//   1. Grayscale (is_lcd == 0): single R8 alpha sample per pixel.
//   2. LCD/ClearType (is_lcd == 1): 3 adjacent R8 texels per pixel,
//      providing per-channel (R, G, B) alpha for subpixel rendering.
//      The glyph is stored at 3x width in the atlas.
//
// The fragment shader outputs premultiplied alpha in both cases.
//
// References:
// - Skia GrAtlasTextOp (R8 atlas compositing)
// - Chrome cc::GlyphAtlas (alpha mask cache + GPU upload)
// - FreeType LCD rendering (5-tap FIR filter)

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
    // For LCD glyphs, UVs span the 3x-wide atlas region.
    @location(1) tex_coord: vec2<f32>,

    // Per-vertex text color (RGBA, premultiplied alpha).
    @location(2) color: vec4<f32>,

    // LCD flag: 0 = grayscale, 1 = LCD subpixel rendering.
    @location(3) is_lcd: u32,
}

struct VertexOutput {
    // Clip-space position for rasterization.
    @builtin(position) position: vec4<f32>,

    // Interpolated UV for fragment shader.
    @location(0) tex_coord: vec2<f32>,

    // Interpolated text color.
    @location(1) color: vec4<f32>,

    // LCD flag (flat — no interpolation, same across triangle).
    @location(2) @interpolate(flat) is_lcd: u32,
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

    // Pass through texture coordinates and per-vertex color.
    out.tex_coord = in.tex_coord;
    out.color = in.color;
    out.is_lcd = in.is_lcd;

    return out;
}

// ============================================================================
// Fragment shader
// ============================================================================

// fs_main: Sample R8 alpha atlas and composite with text color.
//
// Grayscale mode (is_lcd == 0):
//   Single alpha sample. Output = color * alpha (premultiplied).
//
// LCD mode (is_lcd == 1):
//   The atlas stores the glyph at 3x horizontal width. The UV coordinates
//   span the full 3x region. We compute three UV positions to sample the
//   R, G, B coverage from consecutive texels:
//     - R coverage at u_left  = lerp(U0, U1, 1/6)  (center of 1st third)
//     - G coverage at u_mid   = lerp(U0, U1, 3/6)  (center of 2nd third)
//     - B coverage at u_right = lerp(U0, U1, 5/6)  (center of 3rd third)
//   Each channel gets independent alpha for per-channel subpixel blending.
@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4<f32> {
    if (in.is_lcd == 0u) {
        // Grayscale: single alpha sample from R8 atlas.
        let alpha = textureSample(atlas_texture, atlas_sampler, in.tex_coord).r;
        let a = alpha * in.color.a;
        return vec4<f32>(in.color.rgb * a, a);
    }

    // LCD subpixel: sample 3 adjacent R8 texels for per-channel coverage.
    // The UV range [U0, U1] spans 3 * logical_width texels in the atlas.
    // We want to sample at the center of each third.
    let atlas_dims = vec2<f32>(textureDimensions(atlas_texture, 0));
    let texel_u = 1.0 / atlas_dims.x;

    // The UV spans 3*W texels. We sample at relative positions 1/6, 3/6, 5/6.
    // But since tex_coord is already interpolated per-fragment, we compute
    // an offset from the center. Each logical pixel = 3 atlas texels.
    // Offset from center texel: -1 texel (R), 0 (G), +1 texel (B).
    let r_cov = textureSample(atlas_texture, atlas_sampler,
        vec2<f32>(in.tex_coord.x - texel_u, in.tex_coord.y)).r;
    let g_cov = textureSample(atlas_texture, atlas_sampler,
        in.tex_coord).r;
    let b_cov = textureSample(atlas_texture, atlas_sampler,
        vec2<f32>(in.tex_coord.x + texel_u, in.tex_coord.y)).r;

    // Per-channel alpha blending (ClearType).
    // Each channel: out_ch = color_ch * ch_coverage * color_alpha.
    let rgb_alpha = vec3<f32>(r_cov, g_cov, b_cov) * in.color.a;
    let out_rgb = in.color.rgb * rgb_alpha;

    // Output alpha = max per-channel alpha (for correct compositing).
    let out_a = max(max(out_rgb.r, out_rgb.g), out_rgb.b);

    return vec4<f32>(out_rgb, out_a);
}
