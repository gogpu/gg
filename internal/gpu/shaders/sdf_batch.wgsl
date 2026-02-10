// sdf_batch.wgsl - Multi-pass SDF compute shader for batched shape rendering.
//
// Each dispatch processes ONE shape from the shapes array, identified by
// frame.shape_index. Multiple dispatches in a single command encoder process
// all shapes, with implicit storage buffer barriers ensuring correct
// read-after-write ordering between passes.
//
// This avoids loops entirely — naga's SPIR-V backend has a loop iteration bug
// (Bug #5) where for/loop constructs only execute the first iteration.
//
// Shape kinds:
//   0 = circle/ellipse (param1=radius_x, param2=radius_y, param3=unused)
//   1 = rounded rectangle (param1=half_width, param2=half_height, param3=corner_radius)
//
// Pixel format: u32 packed as R | G<<8 | B<<16 | A<<24 (premultiplied alpha).
//
// NOTE: All math is inlined because naga's SPIR-V backend has issues with:
// 1) smoothstep/clamp/select/abs/min/max argument reordering
// 2) Function calls with if/return patterns ("call result not found")
// 3) var initialization from expressions evaluates to zero
// 4) f32(bool) conversion unsupported
// 5) for/loop constructs only execute first iteration
// Only sqrt() is used as a builtin. All other operations use arithmetic equivalents.

struct Shape {
    kind: u32,
    center_x: f32,
    center_y: f32,
    param1: f32,
    param2: f32,
    param3: f32,
    half_stroke: f32,
    is_stroked: u32,
    color_r: f32,
    color_g: f32,
    color_b: f32,
    color_a: f32,
}

struct FrameParams {
    target_width: u32,
    target_height: u32,
    shape_index: u32,
    padding: u32,
}

@group(0) @binding(0) var<uniform> frame: FrameParams;
@group(0) @binding(1) var<storage, read> shapes: array<Shape>;
@group(0) @binding(2) var<storage, read_write> pixels: array<u32>;

@compute @workgroup_size(8, 8, 1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let x = gid.x;
    let y = gid.y;
    if x >= frame.target_width || y >= frame.target_height {
        return;
    }

    let idx = y * frame.target_width + x;

    // Read existing pixel and unpack to float RGBA.
    let existing = pixels[idx];
    let acc_r = f32(existing & 0xFFu) / 255.0;
    let acc_g = f32((existing >> 8u) & 0xFFu) / 255.0;
    let acc_b = f32((existing >> 16u) & 0xFFu) / 255.0;
    let acc_a = f32((existing >> 24u) & 0xFFu) / 255.0;

    // Pixel center in framebuffer coordinates.
    let px_world = f32(x) + 0.5;
    let py_world = f32(y) + 0.5;

    // Process the single shape at frame.shape_index.
    let s = shapes[frame.shape_index];

    // Pixel position relative to shape center.
    let dx = px_world - s.center_x;
    let dy = py_world - s.center_y;

    // ---------------------------------------------------------------
    // Compute signed distance based on shape kind.
    // kind=0: circle/ellipse, kind=1: rounded rectangle.
    // We compute both and select via arithmetic multiplication.
    // ---------------------------------------------------------------

    // --- Circle/ellipse SDF (kind == 0) ---
    let nx = dx / s.param1;
    let ny = dy / s.param2;
    let elen = sqrt(nx * nx + ny * ny);
    // min(param1, param2) via arithmetic: (a+b - sqrt((a-b)^2)) * 0.5
    let rdiff = s.param1 - s.param2;
    let min_r = (s.param1 + s.param2 - sqrt(rdiff * rdiff)) * 0.5;
    let d_circle = (elen - 1.0) * min_r;

    // --- Rounded rectangle SDF (kind == 1) ---
    // abs via sqrt(x*x) to avoid naga issues with abs()
    let apx = sqrt(dx * dx);
    let apy = sqrt(dy * dy);
    let qx = apx - s.param1 + s.param3;
    let qy = apy - s.param2 + s.param3;
    // max(q, 0) via (q + sqrt(q*q)) * 0.5
    let mqx = (qx + sqrt(qx * qx)) * 0.5;
    let mqy = (qy + sqrt(qy * qy)) * 0.5;
    let outside = sqrt(mqx * mqx + mqy * mqy);
    // max(qx, qy) via (qx + qy + sqrt((qx-qy)^2)) * 0.5
    let qdiff = qx - qy;
    let max_qxy = (qx + qy + sqrt(qdiff * qdiff)) * 0.5;
    // min(max_qxy, 0) via (v - sqrt(v*v)) * 0.5
    let inside = (max_qxy - sqrt(max_qxy * max_qxy)) * 0.5;
    let d_rrect = outside + inside - s.param3;

    // Select distance based on kind using arithmetic.
    // f32(bool) is broken in naga SPIR-V, so use min(kind_f, 1):
    //   kind=0 → is_rrect=0, is_circle=1
    //   kind=1 → is_rrect=1, is_circle=0
    let kind_f = f32(s.kind);
    let kdiff = kind_f - 1.0;
    let is_rrect = (kind_f + 1.0 - sqrt(kdiff * kdiff)) * 0.5;
    let is_circle = 1.0 - is_rrect;
    let d = d_circle * is_circle + d_rrect * is_rrect;

    // --- Stroke transformation ---
    // For stroked: effective = |d| - half_stroke
    // For filled:  effective = d
    // Combined: d + is_stroked_f * (|d| - half_stroke - d)
    let is_stroked_f = f32(s.is_stroked);
    let abs_d = sqrt(d * d);
    let effective_dist = d + is_stroked_f * (abs_d - s.half_stroke - d);

    // --- Coverage computation ---
    // Step 1: t = effective_dist + 0.5
    let t_raw = effective_dist + 0.5;

    // Step 2: clamp t_raw to [0, 1] via arithmetic.
    // max(t_raw, 0): (t_raw + sqrt(t_raw*t_raw)) * 0.5
    let t_pos = (t_raw + sqrt(t_raw * t_raw)) * 0.5;
    // min(t_pos, 1) = (a + b - sqrt((a-b)^2)) * 0.5
    let t_diff = t_pos - 1.0;
    let t = (t_pos + 1.0 - sqrt(t_diff * t_diff)) * 0.5;

    // Step 3: smoothstep polynomial on clamped t.
    // coverage = 1.0 - t*t*(3.0 - 2.0*t)
    let coverage = 1.0 - t * t * (3.0 - 2.0 * t);

    // --- Alpha blend shape onto accumulated color ---
    // Source is premultiplied: src_c = color_c * color_a * coverage
    let src_a = s.color_a * coverage;
    let src_r = s.color_r * coverage;
    let src_g = s.color_g * coverage;
    let src_b = s.color_b * coverage;

    // Source-over compositing (premultiplied alpha).
    let inv_src_a = 1.0 - src_a;
    let out_r = src_r + acc_r * inv_src_a;
    let out_g = src_g + acc_g * inv_src_a;
    let out_b = src_b + acc_b * inv_src_a;
    let out_a = src_a + acc_a * inv_src_a;

    // Pack to u32.
    pixels[idx] = u32(out_r * 255.0 + 0.5)
               | (u32(out_g * 255.0 + 0.5) << 8u)
               | (u32(out_b * 255.0 + 0.5) << 16u)
               | (u32(out_a * 255.0 + 0.5) << 24u);
}
