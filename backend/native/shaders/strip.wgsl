// strip.wgsl - Strip rasterization compute shader
//
// This shader processes sparse coverage strips and writes them to an output texture.
// Each strip represents a horizontal span of anti-aliased coverage values at a
// specific scanline. This is the core rasterization shader for the sparse strips
// GPU rendering approach.

// Strip header structure matching GPUStripHeader in Go
struct Strip {
    y: i32,              // Row index (scanline number)
    x: i32,              // Start X coordinate
    width: i32,          // Number of pixels in this strip
    coverage_offset: i32, // Offset into coverage array (byte index)
}

// Parameters for strip rasterization
struct StripParams {
    color: vec4<f32>,    // Fill color (premultiplied alpha)
    target_width: i32,   // Output texture width
    target_height: i32,  // Output texture height
    strip_count: i32,    // Total number of strips to process
    padding: i32,        // Alignment padding
}

// Storage buffers for strip data
@group(0) @binding(0) var<storage, read> strips: array<Strip>;
@group(0) @binding(1) var<storage, read> coverage: array<u32>;  // Packed u8 coverage values
@group(0) @binding(2) var<uniform> params: StripParams;
@group(0) @binding(3) var output: texture_storage_2d<rgba8unorm, write>;

// Extract a single byte from packed u32 coverage data
// Coverage values are stored as packed bytes: 4 bytes per u32
fn read_coverage(byte_index: i32) -> f32 {
    let word_index = byte_index / 4;
    let byte_offset = u32((byte_index % 4) * 8);
    let packed = coverage[word_index];
    let byte_value = (packed >> byte_offset) & 0xFFu;
    return f32(byte_value) / 255.0;
}

// Workgroup size optimized for strip processing
// Each thread processes one strip
@compute @workgroup_size(64, 1, 1)
fn cs_main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let strip_idx = i32(gid.x);

    // Bounds check: skip if beyond strip count
    if strip_idx >= params.strip_count {
        return;
    }

    // Load strip header
    let strip = strips[strip_idx];

    // Process each pixel in the strip
    for (var i: i32 = 0; i < strip.width; i = i + 1) {
        let x = strip.x + i;
        let y = strip.y;

        // Skip pixels outside the target texture bounds
        if x < 0 || x >= params.target_width || y < 0 || y >= params.target_height {
            continue;
        }

        // Read coverage value for this pixel
        let cov = read_coverage(strip.coverage_offset + i);

        // Apply coverage to the fill color
        // The color is expected to be in premultiplied alpha format
        let final_color = vec4<f32>(
            params.color.rgb * cov,
            params.color.a * cov
        );

        // Write to output texture
        textureStore(output, vec2<i32>(x, y), final_color);
    }
}

// Alternative entry point for accumulating coverage (for multi-pass rendering)
// This reads existing values and blends with Source Over
@compute @workgroup_size(64, 1, 1)
fn cs_accumulate(@builtin(global_invocation_id) gid: vec3<u32>) {
    let strip_idx = i32(gid.x);

    if strip_idx >= params.strip_count {
        return;
    }

    let strip = strips[strip_idx];

    for (var i: i32 = 0; i < strip.width; i = i + 1) {
        let x = strip.x + i;
        let y = strip.y;

        if x < 0 || x >= params.target_width || y < 0 || y >= params.target_height {
            continue;
        }

        let cov = read_coverage(strip.coverage_offset + i);

        // Source color with coverage applied
        let src = vec4<f32>(
            params.color.rgb * cov,
            params.color.a * cov
        );

        // Note: For true accumulation, we would need to read the existing value
        // from the texture. Since storage textures don't support read_write in
        // all backends, this simplified version just writes the source.
        // For full blending support, use the blend.wgsl shader in a separate pass.
        textureStore(output, vec2<i32>(x, y), src);
    }
}

// Entry point for processing strips with per-strip colors
// Assumes color data is interleaved with strip headers in an extended format
struct ColoredStrip {
    y: i32,
    x: i32,
    width: i32,
    coverage_offset: i32,
    color: vec4<f32>,
}

@group(1) @binding(0) var<storage, read> colored_strips: array<ColoredStrip>;

@compute @workgroup_size(64, 1, 1)
fn cs_colored(@builtin(global_invocation_id) gid: vec3<u32>) {
    let strip_idx = i32(gid.x);

    if strip_idx >= params.strip_count {
        return;
    }

    let strip = colored_strips[strip_idx];

    for (var i: i32 = 0; i < strip.width; i = i + 1) {
        let x = strip.x + i;
        let y = strip.y;

        if x < 0 || x >= params.target_width || y < 0 || y >= params.target_height {
            continue;
        }

        let cov = read_coverage(strip.coverage_offset + i);

        let final_color = vec4<f32>(
            strip.color.rgb * cov,
            strip.color.a * cov
        );

        textureStore(output, vec2<i32>(x, y), final_color);
    }
}
