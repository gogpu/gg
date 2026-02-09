// sdf_test_const.wgsl - Minimal constant-write shader for debugging.
//
// Writes a constant value (0xDEADBEEF) to every pixel in the storage buffer.
// Used to isolate whether OpStore/OpAccessChain works correctly in naga SPIR-V
// output. If readback shows 0xDEADBEEF, the store path is correct and the bug
// is in expression computation. If readback shows zeros, OpStore itself is broken.

struct Params {
    center_x: f32,
    center_y: f32,
    radius_x: f32,
    radius_y: f32,
    half_stroke_width: f32,
    is_stroked: u32,
    color_r: f32,
    color_g: f32,
    color_b: f32,
    color_a: f32,
    target_width: u32,
    target_height: u32,
}

@group(0) @binding(0) var<uniform> params: Params;
@group(0) @binding(1) var<storage, read_write> pixels: array<u32>;

@compute @workgroup_size(8, 8, 1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let x = gid.x;
    let y = gid.y;
    if x >= params.target_width || y >= params.target_height {
        return;
    }
    let idx = y * params.target_width + x;
    // Write a known constant - no computation, just a literal store.
    pixels[idx] = 0xDEADBEEFu;
}
