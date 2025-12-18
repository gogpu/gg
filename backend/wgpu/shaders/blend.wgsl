// blend.wgsl - Blend mode shader supporting all 29 blend modes
//
// Implements Porter-Duff compositing operators and advanced blend modes
// following the W3C Compositing and Blending Level 1 specification.

struct VertexOutput {
    @builtin(position) position: vec4<f32>,
    @location(0) uv: vec2<f32>,
}

struct BlendParams {
    mode: u32,      // Blend mode enum (matches scene.BlendMode)
    alpha: f32,     // Layer opacity (0.0 - 1.0)
    padding: vec2<f32>,
}

@group(0) @binding(0) var dst_texture: texture_2d<f32>;
@group(0) @binding(1) var src_texture: texture_2d<f32>;
@group(0) @binding(2) var tex_sampler: sampler;
@group(0) @binding(3) var<uniform> params: BlendParams;

// Blend mode constants matching scene.BlendMode from encoding.go
// Standard blend modes (0-11)
const BLEND_NORMAL: u32 = 0u;
const BLEND_MULTIPLY: u32 = 1u;
const BLEND_SCREEN: u32 = 2u;
const BLEND_OVERLAY: u32 = 3u;
const BLEND_DARKEN: u32 = 4u;
const BLEND_LIGHTEN: u32 = 5u;
const BLEND_COLOR_DODGE: u32 = 6u;
const BLEND_COLOR_BURN: u32 = 7u;
const BLEND_HARD_LIGHT: u32 = 8u;
const BLEND_SOFT_LIGHT: u32 = 9u;
const BLEND_DIFFERENCE: u32 = 10u;
const BLEND_EXCLUSION: u32 = 11u;

// HSL blend modes (12-15)
const BLEND_HUE: u32 = 12u;
const BLEND_SATURATION: u32 = 13u;
const BLEND_COLOR: u32 = 14u;
const BLEND_LUMINOSITY: u32 = 15u;

// Porter-Duff modes (16-28)
const BLEND_CLEAR: u32 = 16u;
const BLEND_COPY: u32 = 17u;
const BLEND_DESTINATION: u32 = 18u;
const BLEND_SOURCE_OVER: u32 = 19u;
const BLEND_DESTINATION_OVER: u32 = 20u;
const BLEND_SOURCE_IN: u32 = 21u;
const BLEND_DESTINATION_IN: u32 = 22u;
const BLEND_SOURCE_OUT: u32 = 23u;
const BLEND_DESTINATION_OUT: u32 = 24u;
const BLEND_SOURCE_ATOP: u32 = 25u;
const BLEND_DESTINATION_ATOP: u32 = 26u;
const BLEND_XOR: u32 = 27u;
const BLEND_PLUS: u32 = 28u;

// Vertex shader: generates a full-screen triangle
@vertex
fn vs_main(@builtin(vertex_index) idx: u32) -> VertexOutput {
    var positions = array<vec2<f32>, 3>(
        vec2<f32>(-1.0, -1.0),
        vec2<f32>(3.0, -1.0),
        vec2<f32>(-1.0, 3.0)
    );
    var uvs = array<vec2<f32>, 3>(
        vec2<f32>(0.0, 1.0),
        vec2<f32>(2.0, 1.0),
        vec2<f32>(0.0, -1.0)
    );

    var out: VertexOutput;
    out.position = vec4<f32>(positions[idx], 0.0, 1.0);
    out.uv = uvs[idx];
    return out;
}

// ============================================================================
// Helper functions for HSL color space
// ============================================================================

// Compute luminosity (ITU-R BT.709 coefficients)
fn luminosity(c: vec3<f32>) -> f32 {
    return 0.2126 * c.r + 0.7152 * c.g + 0.0722 * c.b;
}

// Compute saturation
fn saturation(c: vec3<f32>) -> f32 {
    return max(c.r, max(c.g, c.b)) - min(c.r, min(c.g, c.b));
}

// Clip color to valid range while preserving luminosity
fn clip_color(c: vec3<f32>) -> vec3<f32> {
    let lum = luminosity(c);
    let n = min(c.r, min(c.g, c.b));
    let x = max(c.r, max(c.g, c.b));

    var result = c;
    if n < 0.0 {
        result = lum + (result - lum) * lum / (lum - n);
    }
    if x > 1.0 {
        result = lum + (result - lum) * (1.0 - lum) / (x - lum);
    }
    return result;
}

// Set luminosity of a color
fn set_lum(c: vec3<f32>, lum: f32) -> vec3<f32> {
    let d = lum - luminosity(c);
    return clip_color(c + d);
}

// Set saturation of a color (complex algorithm)
fn set_sat(c: vec3<f32>, sat: f32) -> vec3<f32> {
    // Sort components to find min, mid, max
    var result = c;
    let min_c = min(c.r, min(c.g, c.b));
    let max_c = max(c.r, max(c.g, c.b));

    if max_c > min_c {
        // Normalize to [0, sat] range based on original saturation
        let scale = sat / (max_c - min_c);
        result = (c - min_c) * scale;
    } else {
        result = vec3<f32>(0.0);
    }
    return result;
}

// ============================================================================
// Separable blend mode functions
// ============================================================================

// Multiply: Darkens by multiplying colors
fn blend_multiply(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    return src * dst;
}

// Screen: Lightens by inverting, multiplying, inverting
fn blend_screen(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    return src + dst - src * dst;
}

// Overlay: Combines multiply and screen based on destination
fn blend_overlay(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    return mix(
        2.0 * src * dst,
        1.0 - 2.0 * (1.0 - src) * (1.0 - dst),
        step(vec3<f32>(0.5), dst)
    );
}

// Darken: Selects darker of two colors per channel
fn blend_darken(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    return min(src, dst);
}

// Lighten: Selects lighter of two colors per channel
fn blend_lighten(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    return max(src, dst);
}

// Color Dodge: Brightens destination to reflect source
fn blend_color_dodge(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    var result: vec3<f32>;
    // Per-channel: if src >= 1, result = 1; else result = min(1, dst / (1 - src))
    result.r = select(min(1.0, dst.r / (1.0 - src.r)), 1.0, src.r >= 1.0);
    result.g = select(min(1.0, dst.g / (1.0 - src.g)), 1.0, src.g >= 1.0);
    result.b = select(min(1.0, dst.b / (1.0 - src.b)), 1.0, src.b >= 1.0);
    return result;
}

// Color Burn: Darkens destination to reflect source
fn blend_color_burn(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    var result: vec3<f32>;
    // Per-channel: if src <= 0, result = 0; else result = max(0, 1 - (1 - dst) / src)
    result.r = select(max(0.0, 1.0 - (1.0 - dst.r) / src.r), 0.0, src.r <= 0.0);
    result.g = select(max(0.0, 1.0 - (1.0 - dst.g) / src.g), 0.0, src.g <= 0.0);
    result.b = select(max(0.0, 1.0 - (1.0 - dst.b) / src.b), 0.0, src.b <= 0.0);
    return result;
}

// Hard Light: Like overlay, but uses source instead of destination for decision
fn blend_hard_light(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    return mix(
        2.0 * src * dst,
        1.0 - 2.0 * (1.0 - src) * (1.0 - dst),
        step(vec3<f32>(0.5), src)
    );
}

// Soft Light: Gentler version of overlay
fn blend_soft_light(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    // W3C formula for soft light
    let d = select(
        sqrt(dst),
        ((16.0 * dst - 12.0) * dst + 4.0) * dst,
        step(dst, vec3<f32>(0.25))
    );
    return select(
        dst + (2.0 * src - 1.0) * (d - dst),
        dst - (1.0 - 2.0 * src) * dst * (1.0 - dst),
        step(src, vec3<f32>(0.5))
    );
}

// Difference: Absolute difference between colors
fn blend_difference(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    return abs(src - dst);
}

// Exclusion: Similar to difference but lower contrast
fn blend_exclusion(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    return src + dst - 2.0 * src * dst;
}

// ============================================================================
// Non-separable HSL blend modes
// ============================================================================

// Hue: Uses hue of source, saturation and luminosity of destination
fn blend_hue(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    return set_lum(set_sat(src, saturation(dst)), luminosity(dst));
}

// Saturation: Uses saturation of source, hue and luminosity of destination
fn blend_saturation(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    return set_lum(set_sat(dst, saturation(src)), luminosity(dst));
}

// Color: Uses hue and saturation of source, luminosity of destination
fn blend_color(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    return set_lum(src, luminosity(dst));
}

// Luminosity: Uses luminosity of source, hue and saturation of destination
fn blend_luminosity_mode(src: vec3<f32>, dst: vec3<f32>) -> vec3<f32> {
    return set_lum(dst, luminosity(src));
}

// ============================================================================
// Advanced blend mode dispatcher
// ============================================================================

// Apply separable blend mode to RGB components
fn blend_separable(src: vec3<f32>, dst: vec3<f32>, mode: u32) -> vec3<f32> {
    switch mode {
        case BLEND_NORMAL: {
            return src;
        }
        case BLEND_MULTIPLY: {
            return blend_multiply(src, dst);
        }
        case BLEND_SCREEN: {
            return blend_screen(src, dst);
        }
        case BLEND_OVERLAY: {
            return blend_overlay(src, dst);
        }
        case BLEND_DARKEN: {
            return blend_darken(src, dst);
        }
        case BLEND_LIGHTEN: {
            return blend_lighten(src, dst);
        }
        case BLEND_COLOR_DODGE: {
            return blend_color_dodge(src, dst);
        }
        case BLEND_COLOR_BURN: {
            return blend_color_burn(src, dst);
        }
        case BLEND_HARD_LIGHT: {
            return blend_hard_light(src, dst);
        }
        case BLEND_SOFT_LIGHT: {
            return blend_soft_light(src, dst);
        }
        case BLEND_DIFFERENCE: {
            return blend_difference(src, dst);
        }
        case BLEND_EXCLUSION: {
            return blend_exclusion(src, dst);
        }
        default: {
            return src;
        }
    }
}

// Apply non-separable HSL blend mode
fn blend_non_separable(src: vec3<f32>, dst: vec3<f32>, mode: u32) -> vec3<f32> {
    switch mode {
        case BLEND_HUE: {
            return blend_hue(src, dst);
        }
        case BLEND_SATURATION: {
            return blend_saturation(src, dst);
        }
        case BLEND_COLOR: {
            return blend_color(src, dst);
        }
        case BLEND_LUMINOSITY: {
            return blend_luminosity_mode(src, dst);
        }
        default: {
            return src;
        }
    }
}

// ============================================================================
// Porter-Duff compositing operators
// ============================================================================

// Porter-Duff compositing with premultiplied alpha
fn blend_porter_duff(src: vec4<f32>, dst: vec4<f32>, mode: u32) -> vec4<f32> {
    let sa = src.a;
    let da = dst.a;

    switch mode {
        // Clear: Both source and destination are cleared
        case BLEND_CLEAR: {
            return vec4<f32>(0.0);
        }
        // Copy (Src): Only source is used
        case BLEND_COPY: {
            return src;
        }
        // Destination (Dst): Only destination is used
        case BLEND_DESTINATION: {
            return dst;
        }
        // Source Over: Source over destination (standard alpha compositing)
        case BLEND_SOURCE_OVER: {
            return src + dst * (1.0 - sa);
        }
        // Destination Over: Destination over source
        case BLEND_DESTINATION_OVER: {
            return dst + src * (1.0 - da);
        }
        // Source In: Source clipped by destination alpha
        case BLEND_SOURCE_IN: {
            return src * da;
        }
        // Destination In: Destination clipped by source alpha
        case BLEND_DESTINATION_IN: {
            return dst * sa;
        }
        // Source Out: Source clipped by inverse destination alpha
        case BLEND_SOURCE_OUT: {
            return src * (1.0 - da);
        }
        // Destination Out: Destination clipped by inverse source alpha
        case BLEND_DESTINATION_OUT: {
            return dst * (1.0 - sa);
        }
        // Source Atop: Source atop destination
        case BLEND_SOURCE_ATOP: {
            return src * da + dst * (1.0 - sa);
        }
        // Destination Atop: Destination atop source
        case BLEND_DESTINATION_ATOP: {
            return dst * sa + src * (1.0 - da);
        }
        // Xor: Exclusive or of source and destination
        case BLEND_XOR: {
            return src * (1.0 - da) + dst * (1.0 - sa);
        }
        // Plus (Lighter): Additive blending, clamped
        case BLEND_PLUS: {
            return min(src + dst, vec4<f32>(1.0));
        }
        default: {
            return src;
        }
    }
}

// ============================================================================
// Main fragment shader
// ============================================================================

@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4<f32> {
    // Sample source and destination textures
    var src = textureSample(src_texture, tex_sampler, in.uv);
    let dst = textureSample(dst_texture, tex_sampler, in.uv);

    // Apply layer opacity to source
    src = vec4<f32>(src.rgb * params.alpha, src.a * params.alpha);

    let mode = params.mode;

    // Porter-Duff modes (16-28)
    if mode >= BLEND_CLEAR {
        return blend_porter_duff(src, dst, mode);
    }

    // HSL non-separable modes (12-15)
    if mode >= BLEND_HUE && mode <= BLEND_LUMINOSITY {
        // For HSL modes, blend RGB then composite alpha
        let rgb = blend_non_separable(src.rgb, dst.rgb, mode);
        let out_alpha = src.a + dst.a * (1.0 - src.a);
        return vec4<f32>(rgb * out_alpha, out_alpha);
    }

    // Separable blend modes (0-11)
    // Apply blend to RGB, then composite with Porter-Duff Source Over
    let blended_rgb = blend_separable(src.rgb, dst.rgb, mode);

    // Final alpha using Source Over formula
    let out_alpha = src.a + dst.a * (1.0 - src.a);

    // Mix blended result with destination based on source alpha
    // This follows the W3C compositing spec for separable modes
    let final_rgb = select(
        dst.rgb,
        blended_rgb * src.a + dst.rgb * dst.a * (1.0 - src.a),
        out_alpha > 0.0
    ) / max(out_alpha, 0.0001);

    return vec4<f32>(final_rgb * out_alpha, out_alpha);
}
