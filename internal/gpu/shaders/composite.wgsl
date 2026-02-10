// composite.wgsl - Final layer compositing shader
//
// This shader composites multiple layers into a final output image.
// Each layer has its own texture, blend mode, and opacity. Layers are
// composited from bottom to top in the order specified.

struct VertexOutput {
    @builtin(position) position: vec4<f32>,
    @location(0) uv: vec2<f32>,
}

// Layer descriptor
struct Layer {
    texture_idx: u32,    // Index into layer_textures binding array
    blend_mode: u32,     // Blend mode (matches scene.BlendMode)
    alpha: f32,          // Layer opacity (0.0 - 1.0)
    padding: f32,        // Alignment padding
}

// Composite parameters
struct CompositeParams {
    layer_count: u32,    // Number of layers to composite
    width: u32,          // Output width
    height: u32,         // Output height
    padding: u32,        // Alignment padding
}

// Bind group 0: Layer textures and sampler
// Note: binding_array requires WGSL extension or WebGPU feature
@group(0) @binding(0) var layer_textures: texture_2d_array<f32>;
@group(0) @binding(1) var layer_sampler: sampler;

// Bind group 1: Layer descriptors and parameters
@group(1) @binding(0) var<storage, read> layers: array<Layer>;
@group(1) @binding(1) var<uniform> params: CompositeParams;

// Blend mode constants (copied from blend.wgsl for self-containment)
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
const BLEND_HUE: u32 = 12u;
const BLEND_SATURATION: u32 = 13u;
const BLEND_COLOR: u32 = 14u;
const BLEND_LUMINOSITY: u32 = 15u;
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
// Blend mode implementations (simplified versions for compositing)
// ============================================================================

// Porter-Duff Source Over (standard alpha compositing)
fn source_over(src: vec4<f32>, dst: vec4<f32>) -> vec4<f32> {
    return src + dst * (1.0 - src.a);
}

// Apply separable blend mode
fn apply_blend(src: vec3<f32>, dst: vec3<f32>, mode: u32) -> vec3<f32> {
    switch mode {
        case BLEND_NORMAL: { return src; }
        case BLEND_MULTIPLY: { return src * dst; }
        case BLEND_SCREEN: { return src + dst - src * dst; }
        case BLEND_OVERLAY: {
            return mix(
                2.0 * src * dst,
                1.0 - 2.0 * (1.0 - src) * (1.0 - dst),
                step(vec3<f32>(0.5), dst)
            );
        }
        case BLEND_DARKEN: { return min(src, dst); }
        case BLEND_LIGHTEN: { return max(src, dst); }
        case BLEND_DIFFERENCE: { return abs(src - dst); }
        case BLEND_EXCLUSION: { return src + dst - 2.0 * src * dst; }
        default: { return src; }
    }
}

// Apply Porter-Duff operator
fn apply_porter_duff(src: vec4<f32>, dst: vec4<f32>, mode: u32) -> vec4<f32> {
    let sa = src.a;
    let da = dst.a;

    switch mode {
        case BLEND_CLEAR: { return vec4<f32>(0.0); }
        case BLEND_COPY: { return src; }
        case BLEND_DESTINATION: { return dst; }
        case BLEND_SOURCE_OVER: { return src + dst * (1.0 - sa); }
        case BLEND_DESTINATION_OVER: { return dst + src * (1.0 - da); }
        case BLEND_SOURCE_IN: { return src * da; }
        case BLEND_DESTINATION_IN: { return dst * sa; }
        case BLEND_SOURCE_OUT: { return src * (1.0 - da); }
        case BLEND_DESTINATION_OUT: { return dst * (1.0 - sa); }
        case BLEND_SOURCE_ATOP: { return src * da + dst * (1.0 - sa); }
        case BLEND_DESTINATION_ATOP: { return dst * sa + src * (1.0 - da); }
        case BLEND_XOR: { return src * (1.0 - da) + dst * (1.0 - sa); }
        case BLEND_PLUS: { return min(src + dst, vec4<f32>(1.0)); }
        default: { return src + dst * (1.0 - sa); }
    }
}

// Composite source onto destination with given blend mode
fn composite_layer(src: vec4<f32>, dst: vec4<f32>, mode: u32) -> vec4<f32> {
    // Porter-Duff modes (16-28)
    if mode >= BLEND_CLEAR {
        return apply_porter_duff(src, dst, mode);
    }

    // For separable blend modes, blend RGB then composite alpha
    let blended_rgb = apply_blend(src.rgb, dst.rgb, mode);
    let out_alpha = src.a + dst.a * (1.0 - src.a);

    // Mix blended result based on alpha values
    var final_rgb = blended_rgb;
    if out_alpha > 0.0 {
        final_rgb = (blended_rgb * src.a + dst.rgb * dst.a * (1.0 - src.a)) / out_alpha;
    }

    return vec4<f32>(final_rgb, out_alpha);
}

// ============================================================================
// Fragment shader - composites all layers
// ============================================================================

@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4<f32> {
    // Start with transparent black
    var result = vec4<f32>(0.0);

    // Composite each layer from bottom to top
    for (var i: u32 = 0u; i < params.layer_count; i = i + 1u) {
        let layer = layers[i];

        // Sample layer texture
        var src = textureSample(
            layer_textures,
            layer_sampler,
            in.uv,
            i32(layer.texture_idx)
        );

        // Apply layer opacity
        src = vec4<f32>(src.rgb * layer.alpha, src.a * layer.alpha);

        // Composite with current result using the layer's blend mode
        result = composite_layer(src, result, layer.blend_mode);
    }

    return result;
}

// ============================================================================
// Alternative fragment shader using separate textures (no binding array)
// ============================================================================

// For backends that don't support binding arrays, use individual texture bindings
// This version supports up to 4 layers with separate texture bindings

@group(2) @binding(0) var layer0_texture: texture_2d<f32>;
@group(2) @binding(1) var layer1_texture: texture_2d<f32>;
@group(2) @binding(2) var layer2_texture: texture_2d<f32>;
@group(2) @binding(3) var layer3_texture: texture_2d<f32>;

fn sample_layer_texture(idx: u32, uv: vec2<f32>) -> vec4<f32> {
    switch idx {
        case 0u: { return textureSample(layer0_texture, layer_sampler, uv); }
        case 1u: { return textureSample(layer1_texture, layer_sampler, uv); }
        case 2u: { return textureSample(layer2_texture, layer_sampler, uv); }
        case 3u: { return textureSample(layer3_texture, layer_sampler, uv); }
        default: { return vec4<f32>(0.0); }
    }
}

@fragment
fn fs_simple(in: VertexOutput) -> @location(0) vec4<f32> {
    var result = vec4<f32>(0.0);

    // Composite up to 4 layers
    let max_layers = min(params.layer_count, 4u);

    for (var i: u32 = 0u; i < max_layers; i = i + 1u) {
        let layer = layers[i];

        // Sample using switch-based texture selection
        var src = sample_layer_texture(layer.texture_idx, in.uv);

        // Apply layer opacity
        src = vec4<f32>(src.rgb * layer.alpha, src.a * layer.alpha);

        // Composite with current result
        result = composite_layer(src, result, layer.blend_mode);
    }

    return result;
}
