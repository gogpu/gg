// blit.wgsl - Simple texture copy shader
//
// This shader renders a full-screen triangle and samples from a source texture.
// It's used for simple texture-to-texture copies without any blending.

struct VertexOutput {
    @builtin(position) position: vec4<f32>,
    @location(0) uv: vec2<f32>,
}

@group(0) @binding(0) var src_texture: texture_2d<f32>;
@group(0) @binding(1) var src_sampler: sampler;

// Vertex shader: generates a full-screen triangle from vertex index.
// Uses the technique of rendering a single triangle that covers the entire screen.
// Vertex 0: (-1, -1), Vertex 1: (3, -1), Vertex 2: (-1, 3)
@vertex
fn vs_main(@builtin(vertex_index) idx: u32) -> VertexOutput {
    // Full-screen triangle positions
    var positions = array<vec2<f32>, 3>(
        vec2<f32>(-1.0, -1.0),
        vec2<f32>(3.0, -1.0),
        vec2<f32>(-1.0, 3.0)
    );

    // Corresponding UVs (flipped Y for texture coordinate system)
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

// Fragment shader: samples the source texture
@fragment
fn fs_main(in: VertexOutput) -> @location(0) vec4<f32> {
    return textureSample(src_texture, src_sampler, in.uv);
}
