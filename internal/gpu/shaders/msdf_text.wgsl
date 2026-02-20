// msdf_text.wgsl - Multi-channel Signed Distance Field Text Rendering Shader
//
// This shader renders text using MSDF technique for crisp, resolution-independent
// text rendering. MSDF encodes directional distance in RGB channels to preserve
// sharp corners that would be lost with single-channel SDF.
//
// References:
// - https://github.com/Chlumsky/msdfgen (original MSDF algorithm)
// - W3C WebGPU Shading Language spec

// ============================================================================
// Uniform structures matching Go TextUniforms type
// ============================================================================

struct TextUniforms {
    // Transform matrix: column-major mat4x4 for pixel-to-NDC conversion.
    // Stored as 4 column vectors in memory.
    transform: mat4x4<f32>,

    // Text color (RGBA, premultiplied alpha)
    color: vec4<f32>,

    // MSDF parameters
    // x: px_range - Distance range in pixels (typically 4.0)
    //    Controls how far from the edge the SDF extends in the texture
    // y: atlas_size - Texture size for screen-space derivative calculation
    // z: unused (reserved for future: outline width)
    // w: unused (reserved for future: outline softness)
    msdf_params: vec4<f32>,
}

// ============================================================================
// Vertex input/output structures
// ============================================================================

struct VertexInput {
    // Position of quad vertex in local space
    @location(0) position: vec2<f32>,

    // UV coordinates for sampling MSDF atlas
    @location(1) tex_coord: vec2<f32>,
}

struct VertexOutput {
    // Clip-space position for rasterization
    @builtin(position) position: vec4<f32>,

    // Interpolated UV for fragment shader
    @location(0) tex_coord: vec2<f32>,

    // Text color passed to fragment shader
    @location(1) color: vec4<f32>,
}

// ============================================================================
// Bindings
// ============================================================================

@group(0) @binding(0) var<uniform> uniforms: TextUniforms;
@group(0) @binding(1) var msdf_atlas: texture_2d<f32>;
@group(0) @binding(2) var msdf_sampler: sampler;

// ============================================================================
// Vertex shader
// ============================================================================

// vs_main: Transform quad vertices and pass through UV coordinates.
// Each glyph is rendered as a textured quad with 4 vertices.
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

    // Pass through texture coordinates
    out.tex_coord = in.tex_coord;

    // Pass through color (premultiplied)
    out.color = uniforms.color;

    return out;
}

// ============================================================================
// MSDF sampling functions
// ============================================================================

// median3: Compute median of three values.
// This is the key MSDF operation that recovers the signed distance from
// the three directional distance channels (R, G, B).
// Sharp corners are preserved because each channel encodes distance to
// different edge segments.
fn median3(r: f32, g: f32, b: f32) -> f32 {
    return max(min(r, g), min(max(r, g), b));
}

// ============================================================================
// Fragment shader
// ============================================================================

// fs_main: Sample MSDF atlas and compute anti-aliased alpha.
// Uses screen-space derivatives for proper scaling at any zoom level.
@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4<f32> {
    // Sample the MSDF atlas texture
    let msdf = textureSample(msdf_atlas, msdf_sampler, in.tex_coord).rgb;

    // Recover signed distance from median of RGB channels.
    // MSDF stores distance as [0, 1] range, 0.5 = on edge.
    let sd = median3(msdf.r, msdf.g, msdf.b) - 0.5;

    // Standard MSDF screenPxRange calculation (Chlumsky/msdfgen reference).
    // unitRange = pxRange / atlasSize — the fraction of the texture that is
    // the distance field range.
    let px_range = uniforms.msdf_params.x;
    let atlas_size = uniforms.msdf_params.y;
    let unit_range = vec2<f32>(px_range / atlas_size, px_range / atlas_size);

    // screenTexSize = how many screen pixels one texel covers.
    let screen_tex_size = vec2<f32>(1.0, 1.0) / fwidth(in.tex_coord);

    // screenPxRange: how many screen pixels the distance range spans.
    // max(..., 1.0) prevents artifacts on narrow/small characters where
    // the range would otherwise collapse below one pixel.
    let screen_px_range = max(0.5 * dot(unit_range, screen_tex_size), 1.0);

    // Scale signed distance to screen pixels and compute anti-aliased alpha.
    let alpha = clamp(screen_px_range * sd + 0.5, 0.0, 1.0);

    // Apply alpha to text color (premultiplied alpha output).
    return vec4<f32>(in.color.rgb * alpha, in.color.a * alpha);
}

// ============================================================================
// Alternative entry points for different rendering modes
// ============================================================================

// fs_main_outline: Render text with outline effect.
// Uses the SDF to render both fill and outline in a single pass.
@fragment
fn fs_main_outline(in: VertexOutput) -> @location(0) vec4<f32> {
    let msdf = textureSample(msdf_atlas, msdf_sampler, in.tex_coord).rgb;
    let sd = median3(msdf.r, msdf.g, msdf.b) - 0.5;

    let unit_range = vec2<f32>(uniforms.msdf_params.x / uniforms.msdf_params.y,
                              uniforms.msdf_params.x / uniforms.msdf_params.y);
    let screen_tex_size = vec2<f32>(1.0, 1.0) / fwidth(in.tex_coord);
    let screen_px_range = max(0.5 * dot(unit_range, screen_tex_size), 1.0);
    let screen_px_distance = screen_px_range * sd;

    // Outline width in screen pixels (stored in msdf_params.z)
    let outline_width = uniforms.msdf_params.z;

    // Fill alpha (inner glyph)
    let fill_alpha = clamp(screen_px_distance + 0.5, 0.0, 1.0);

    // Outline alpha (ring around glyph)
    let outline_alpha = clamp(screen_px_distance + outline_width + 0.5, 0.0, 1.0);

    // Blend: outline color where outline but not fill
    // For simplicity, using inverted fill color for outline
    let outline_color = vec4<f32>(1.0 - in.color.rgb, 1.0);

    // Composite: fill over outline
    let outline_diff = outline_alpha - fill_alpha;
    let fill = vec4<f32>(in.color.rgb * fill_alpha, in.color.a * fill_alpha);
    let outline = vec4<f32>(outline_color.rgb * outline_diff, outline_color.a * outline_diff);

    return fill + outline;
}

// fs_main_shadow: Render text with drop shadow effect.
// Samples the SDF twice with offset for shadow.
@fragment
fn fs_main_shadow(in: VertexOutput) -> @location(0) vec4<f32> {
    // Shadow offset in UV space (could be uniform parameter)
    let shadow_offset = vec2<f32>(0.002, 0.002);
    let shadow_color = vec4<f32>(0.0, 0.0, 0.0, 0.5);

    let unit_range = vec2<f32>(uniforms.msdf_params.x / uniforms.msdf_params.y,
                              uniforms.msdf_params.x / uniforms.msdf_params.y);
    let screen_tex_size = vec2<f32>(1.0, 1.0) / fwidth(in.tex_coord);
    let screen_px_range = max(0.5 * dot(unit_range, screen_tex_size), 1.0);

    // Sample shadow (offset)
    let shadow_msdf = textureSample(msdf_atlas, msdf_sampler, in.tex_coord - shadow_offset).rgb;
    let shadow_sd = median3(shadow_msdf.r, shadow_msdf.g, shadow_msdf.b) - 0.5;
    let shadow_alpha = clamp(screen_px_range * shadow_sd + 0.5, 0.0, 1.0);

    // Sample fill (no offset)
    let msdf = textureSample(msdf_atlas, msdf_sampler, in.tex_coord).rgb;
    let fill_sd = median3(msdf.r, msdf.g, msdf.b) - 0.5;
    let fill_alpha = clamp(screen_px_range * fill_sd + 0.5, 0.0, 1.0);

    // Composite: fill over shadow (Porter-Duff Source Over)
    let shadow = vec4<f32>(shadow_color.rgb * shadow_alpha, shadow_color.a * shadow_alpha);
    let fill = vec4<f32>(in.color.rgb * fill_alpha, in.color.a * fill_alpha);

    return fill + shadow * (1.0 - fill.a);
}
