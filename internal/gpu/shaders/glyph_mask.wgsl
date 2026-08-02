// glyph_mask.wgsl - Alpha Mask Text Rendering Shader (Tier 6)
//
// Renders CPU-rasterized glyph alpha masks as textured quads. The atlas
// stores R8 (single-channel) coverage data produced by AnalyticFiller.
//
// The fragment shader outputs premultiplied alpha. Color is passed via the
// uniform buffer already premultiplied by the CPU. The shader temporarily
// recovers straight RGB for alpha-independent mask-gamma luminance, then
// scales the premultiplied color by coverage exactly once.
//
// Mask gamma correction (Skia SkMaskGamma pattern):
//   Light text on dark backgrounds appears perceptually thinner because
//   linear blending underestimates coverage on low-luminance destinations.
//   The fragment shader applies a luminance-dependent contrast boost to
//   alpha values before compositing:
//     contrast = luminance * 0.5  (0.5 = Skia SK_GAMMA_CONTRAST default)
//     boosted  = alpha + (1 - alpha) * contrast * alpha
//   Light text gets boosted (stems appear more solid on dark bg).
//   Dark text is unchanged (no perceptual thinning on light bg).
//
// References:
// - Skia GrAtlasTextOp (R8 atlas compositing)
// - Skia SkMaskGamma.cpp:74 (apply_contrast formula)
// - Skia SkTypes.h:89 (SK_GAMMA_CONTRAST = 0.5)
// - Skia DistanceFieldAdjustTable.cpp:26-59 (GPU-side gamma rationale)
// - Chrome cc::GlyphAtlas (alpha mask cache + GPU upload)

struct GlyphMaskUniforms {
    transform: mat4x4<f32>,
    color: vec4<f32>,
}

struct VertexInput {
    @location(0) position: vec2<f32>,
    @location(1) tex_coord: vec2<f32>,
}

struct VertexOutput {
    @builtin(position) position: vec4<f32>,
    @location(0) tex_coord: vec2<f32>,
}

@group(0) @binding(0) var<uniform> uniforms: GlyphMaskUniforms;
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
    // Text shaders: no per-pixel SDF clip. Returns 1.0 (no clipping).
    //
    // Enterprise research (GPU-CLIP-002) found that NO production 2D engine
    // (Vello, Skia Graphite/Ganesh, Pathfinder, Qt RHI) computes per-pixel
    // SDF clip inside text fragment shaders. The industry-standard approach
    // is stencil-buffer clip (Skia Ganesh) or depth-buffer clip (Graphite).
    //
    // Per-pixel SDF clip (11 sqrt calls) combined with textureSample causes
    // Intel Vulkan shader compiler to generate corrupt code — text becomes
    // invisible. This is a known Intel driver limitation with register
    // pressure from complex ALU + texture sampling in the same shader.
    //
    // Text clipping is handled by:
    //   1. Hardware scissor rect (axis-aligned, free) — GPU-CLIP-001
    //   2. Stencil-buffer RRect clip (planned) — GPU-CLIP-003
    //
    // The @group(1) binding is kept for uniform pipeline layout across all
    // tiers, avoiding per-tier bind group logic in GPURenderSession.
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

// Mask gamma contrast boost (Skia SkMaskGamma apply_contrast pattern).
// Boosts edge coverage for light-on-dark text to compensate perceptual
// thinning from linear alpha blending on low-luminance destinations.
//
// The contrast is modulated by text color luminance:
//   - Light text (high lum) -> high contrast -> stems appear more solid
//   - Dark text (low lum)   -> low contrast  -> no change needed
//   - Black text (lum = 0)  -> zero contrast -> identity (no boost)
//
// Formula: boosted = alpha + (1 - alpha) * contrast * alpha
// Fixed points: alpha=0 -> 0, alpha=1 -> 1 (endpoints unchanged).
const MASK_GAMMA_MAX_CONTRAST: f32 = 0.5;  // Skia SK_GAMMA_CONTRAST default

fn apply_mask_gamma(alpha: f32, text_color: vec3<f32>) -> f32 {
    let luminance = dot(text_color, vec3<f32>(0.299, 0.587, 0.114));
    let contrast = luminance * MASK_GAMMA_MAX_CONTRAST;
    return alpha + (1.0 - alpha) * contrast * alpha;
}

@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4<f32> {
    let raw_alpha = textureSample(atlas_texture, atlas_sampler, in.tex_coord).r;
    let clip_cov = rrect_clip_coverage(in.position.xy);
    let color = uniforms.color;
    // Mask gamma depends on the source hue/luminance, not its opacity. Recover
    // straight RGB safely so translucent light text is not treated as dark.
    var straight_rgb = vec3<f32>(0.0);
    if color.a > 0.0 {
        straight_rgb = color.rgb / color.a;
    }
    let alpha = apply_mask_gamma(raw_alpha, straight_rgb);
    // The CPU supplied color is already premultiplied. Coverage scales its
    // premultiplied RGB and alpha once; multiplying RGB by color.a here would
    // introduce alpha-squared darkening.
    let coverage = alpha * clip_cov;
    return vec4<f32>(color.rgb * coverage, color.a * coverage);
}
