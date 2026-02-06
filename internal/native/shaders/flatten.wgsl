// flatten.wgsl - GPU curve flattening compute shader
//
// This shader flattens Bezier curves to monotonic line segments.
// It converts quadratic and cubic Bezier curves to line segments
// using Wang's formula to estimate the required number of segments.
//
// The flattening process has two passes:
// 1. cs_flatten_prepare - Count segments per path element
// 2. cs_flatten - Generate actual line segments
//
// Algorithm: Uses Wang's formula to estimate segment count without recursion.
// Wang's formula for quadratic: n = ceil(sqrt(max|d2| / (8 * tolerance)))
// Wang's formula for cubic: n = ceil(pow(max|d2| / tolerance, 0.25) * 0.75)
// where d2 is the second derivative maximum.
//
// Y-monotonicity is ensured by splitting curves at Y extrema.
//
// Workgroup layout: 256 threads, each processing one path element

// Note: Constants are inlined due to naga constant lowering limitation.
// WORKGROUP_SIZE = 256
// EPSILON = 0.000001
// MAX_SEGMENTS_PER_CURVE = 64
// TILE_SIZE = 4

// Path verb constants (matches scene.PathVerb)
const VERB_MOVE_TO: u32 = 0u;
const VERB_LINE_TO: u32 = 1u;
const VERB_QUAD_TO: u32 = 2u;
const VERB_CUBIC_TO: u32 = 3u;
const VERB_CLOSE: u32 = 4u;

// Path element structure
// Contains verb type and point indices
struct PathElement {
    verb: u32,          // Path verb type
    point_start: u32,   // Start index in points array (for this element's points)
    point_count: u32,   // Number of points for this element
    padding: u32,
}

// Cursor state for path traversal
// Passed from CPU to track current position per element
struct CursorState {
    cur_x: f32,     // Current cursor X
    cur_y: f32,     // Current cursor Y
    start_x: f32,   // Subpath start X (for Close)
    start_y: f32,   // Subpath start Y (for Close)
}

// Affine transform matrix
// | a c e |   | x |   | a*x + c*y + e |
// | b d f | * | y | = | b*x + d*y + f |
// | 0 0 1 |   | 1 |   |       1       |
struct AffineTransform {
    a: f32,  // Scale X / Rotate
    b: f32,  // Shear Y / Rotate
    c: f32,  // Shear X / Rotate
    d: f32,  // Scale Y / Rotate
    e: f32,  // Translate X
    f_val: f32,  // Translate Y (f is reserved in WGSL)
    padding1: f32,
    padding2: f32,
}

// Output line segment (matches GPUSegment in Go)
struct LineSegment {
    x0: f32,
    y0: f32,
    x1: f32,
    y1: f32,
    winding: i32,
    tile_y0: i32,
    tile_y1: i32,
    padding: i32,
}

// Segment count per element (for prepare pass)
struct SegmentCount {
    count: u32,    // Number of segments this element produces
    offset: u32,   // Prefix sum offset in output array
    padding1: u32,
    padding2: u32,
}

// Configuration parameters
struct FlattenConfig {
    element_count: u32,   // Number of path elements
    tolerance: f32,       // Flattening tolerance
    max_segments: u32,    // Maximum total segments
    tile_size: u32,       // Tile size in pixels (typically 4)
    viewport_width: u32,  // Viewport width
    viewport_height: u32, // Viewport height
    padding1: u32,
    padding2: u32,
}

// Atomic counter for segment allocation
struct AtomicCounter {
    count: atomic<u32>,
}

// Bind group 0: Input data
@group(0) @binding(0) var<uniform> config: FlattenConfig;
@group(0) @binding(1) var<uniform> transform: AffineTransform;
@group(0) @binding(2) var<storage, read> elements: array<PathElement>;
@group(0) @binding(3) var<storage, read> points: array<f32>;  // x, y pairs
@group(0) @binding(4) var<storage, read> cursors: array<CursorState>;  // Per-element cursor

// Bind group 1: Output data
@group(1) @binding(0) var<storage, read_write> segment_counts: array<SegmentCount>;
@group(1) @binding(1) var<storage, read_write> segments: array<LineSegment>;
@group(1) @binding(2) var<storage, read_write> segment_counter: AtomicCounter;

// Apply affine transform to a point
fn transform_point(x: f32, y: f32) -> vec2<f32> {
    let tx = transform.a * x + transform.c * y + transform.e;
    let ty = transform.b * x + transform.d * y + transform.f_val;
    return vec2<f32>(tx, ty);
}

// Helper: absolute value
fn absf(x: f32) -> f32 {
    if x < 0.0 { return -x; }
    return x;
}

// Helper: minimum
fn minf(a: f32, b: f32) -> f32 {
    if a < b { return a; }
    return b;
}

// Helper: maximum
fn maxf(a: f32, b: f32) -> f32 {
    if a > b { return a; }
    return b;
}

// Pixel to tile Y coordinate (floor division)
fn pixel_to_tile_y(py: f32) -> i32 {
    var floor_py: i32 = i32(py);
    if py < 0.0 && f32(floor_py) != py {
        floor_py = floor_py - 1;
    }
    return floor_py >> 2; // TileShift = 2 for TileSize = 4
}

// Wang's formula for quadratic Bezier segment count
// n = ceil(sqrt(max|d2| / (8 * tolerance)))
// For quadratic, d2 = 2 * (P0 - 2*P1 + P2) is constant
fn wang_quadratic(p0: vec2<f32>, p1: vec2<f32>, p2: vec2<f32>, tolerance: f32) -> u32 {
    // Second derivative (constant for quadratic)
    let d2x = p0.x - 2.0 * p1.x + p2.x;
    let d2y = p0.y - 2.0 * p1.y + p2.y;

    // Maximum deviation
    let max_d = sqrt(d2x * d2x + d2y * d2y);

    if max_d <= 0.000001 {
        return 1u;
    }

    // Wang's formula
    let n = sqrt(max_d / (8.0 * tolerance));

    // Clamp to reasonable range
    let count = u32(ceil(n));
    if count < 1u { return 1u; }
    if count > 64u { return 64u; }
    return count;
}

// Wang's formula for cubic Bezier segment count
// Uses maximum of second derivatives at endpoints
fn wang_cubic(p0: vec2<f32>, p1: vec2<f32>, p2: vec2<f32>, p3: vec2<f32>, tolerance: f32) -> u32 {
    // Second derivatives at t=0 and t=1
    // d2(0) = 6 * (P0 - 2*P1 + P2)
    // d2(1) = 6 * (P1 - 2*P2 + P3)
    let d2_0x = p0.x - 2.0 * p1.x + p2.x;
    let d2_0y = p0.y - 2.0 * p1.y + p2.y;
    let d2_1x = p1.x - 2.0 * p2.x + p3.x;
    let d2_1y = p1.y - 2.0 * p2.y + p3.y;

    let max0 = sqrt(d2_0x * d2_0x + d2_0y * d2_0y);
    let max1 = sqrt(d2_1x * d2_1x + d2_1y * d2_1y);

    var max_d: f32;
    if max0 > max1 { max_d = max0; }
    else { max_d = max1; }

    // Scale by 6 for cubic
    max_d = max_d * 6.0;

    if max_d <= 0.000001 {
        return 1u;
    }

    // Wang's formula for cubic
    // n = ceil(pow(max_d / tolerance, 0.25) * 0.75)
    // Simplified: n = ceil(sqrt(sqrt(max_d / tolerance)) * 0.75)
    let ratio = max_d / tolerance;
    let n = 0.75 * sqrt(sqrt(ratio));

    let count = u32(ceil(n));
    if count < 1u { return 1u; }
    if count > 64u { return 64u; }
    return count;
}

// Find Y extremum parameter for quadratic curve
// Returns t in (0, 1) if extremum exists, otherwise -1.0
fn quad_y_extremum(p0y: f32, p1y: f32, p2y: f32) -> f32 {
    // For quadratic: dy/dt = 0 when t = (p0y - p1y) / (p0y - 2*p1y + p2y)
    let denom = p0y - 2.0 * p1y + p2y;
    if absf(denom) <= 0.000001 {
        return -1.0;
    }
    let t = (p0y - p1y) / denom;
    if t > 0.000001 && t < 0.999999 {
        return t;
    }
    return -1.0;
}

// Find Y extrema parameters for cubic curve
// Returns up to 2 values in (0, 1)
// result.x = first t, result.y = second t, result.z = count
fn cubic_y_extrema(p0y: f32, p1y: f32, p2y: f32, p3y: f32) -> vec3<f32> {
    // dy/dt = 0 is quadratic: at^2 + bt + c = 0
    let a = p0y - 3.0 * p1y + 3.0 * p2y - p3y;
    let b = 2.0 * (p1y - 2.0 * p2y + p3y);
    let c = p2y - p3y;

    var result = vec3<f32>(-1.0, -1.0, 0.0);

    if absf(a) <= 0.000001 {
        // Linear equation
        if absf(b) <= 0.000001 {
            return result;
        }
        let t = -c / b;
        if t > 0.000001 && t < 0.999999 {
            result.x = t;
            result.z = 1.0;
        }
        return result;
    }

    let discriminant = b * b - 4.0 * a * c;
    if discriminant < 0.0 {
        return result;
    }

    let sqrt_d = sqrt(discriminant);
    let inv_2a = 1.0 / (2.0 * a);

    let t1 = (-b - sqrt_d) * inv_2a;
    let t2 = (-b + sqrt_d) * inv_2a;

    var count: f32 = 0.0;
    if t1 > 0.000001 && t1 < 0.999999 {
        result.x = t1;
        count = count + 1.0;
    }
    if t2 > 0.000001 && t2 < 0.999999 {
        if count < 0.5 {
            result.x = t2;
        } else {
            result.y = t2;
        }
        count = count + 1.0;
    }
    result.z = count;

    // Sort if we have two roots
    if result.z > 1.5 && result.x > result.y {
        let tmp = result.x;
        result.x = result.y;
        result.y = tmp;
    }

    return result;
}

// Evaluate quadratic Bezier at parameter t
fn eval_quadratic(p0: vec2<f32>, p1: vec2<f32>, p2: vec2<f32>, t: f32) -> vec2<f32> {
    let mt = 1.0 - t;
    let mt2 = mt * mt;
    let t2 = t * t;
    return vec2<f32>(
        mt2 * p0.x + 2.0 * mt * t * p1.x + t2 * p2.x,
        mt2 * p0.y + 2.0 * mt * t * p1.y + t2 * p2.y
    );
}

// Evaluate cubic Bezier at parameter t
fn eval_cubic(p0: vec2<f32>, p1: vec2<f32>, p2: vec2<f32>, p3: vec2<f32>, t: f32) -> vec2<f32> {
    let mt = 1.0 - t;
    let mt2 = mt * mt;
    let mt3 = mt2 * mt;
    let t2 = t * t;
    let t3 = t2 * t;
    return vec2<f32>(
        mt3 * p0.x + 3.0 * mt2 * t * p1.x + 3.0 * mt * t2 * p2.x + t3 * p3.x,
        mt3 * p0.y + 3.0 * mt2 * t * p1.y + 3.0 * mt * t2 * p2.y + t3 * p3.y
    );
}

// Emit a monotonic line segment to output buffer at given index
fn emit_segment_at(idx: u32, x0: f32, y0: f32, x1: f32, y1: f32) {
    if idx >= config.max_segments {
        return;
    }

    // Skip degenerate or horizontal segments
    let dy = y1 - y0;
    let dx = x1 - x0;
    if absf(dy) <= 0.000001 && absf(dx) <= 0.000001 {
        return;
    }
    if absf(dy) <= 0.000001 {
        return; // Horizontal - no winding contribution
    }

    // Determine winding from original direction
    var winding: i32 = 1;
    if y1 < y0 {
        winding = -1;
    }

    // Ensure Y0 <= Y1 (monotonic)
    var out_x0 = x0;
    var out_y0 = y0;
    var out_x1 = x1;
    var out_y1 = y1;

    if out_y1 < out_y0 {
        out_x0 = x1;
        out_y0 = y1;
        out_x1 = x0;
        out_y1 = y0;
        winding = -winding;
    }

    // Calculate tile range
    let tile_y0 = pixel_to_tile_y(out_y0);
    let tile_y1 = pixel_to_tile_y(out_y1);

    segments[idx].x0 = out_x0;
    segments[idx].y0 = out_y0;
    segments[idx].x1 = out_x1;
    segments[idx].y1 = out_y1;
    segments[idx].winding = winding;
    segments[idx].tile_y0 = tile_y0;
    segments[idx].tile_y1 = tile_y1;
    segments[idx].padding = 0;
}

// Emit a line segment with atomic allocation
fn emit_segment(p0: vec2<f32>, p1: vec2<f32>) {
    // Skip degenerate or horizontal segments
    let dy = p1.y - p0.y;
    let dx = p1.x - p0.x;
    if absf(dy) <= 0.000001 && absf(dx) <= 0.000001 {
        return;
    }
    if absf(dy) <= 0.000001 {
        return;
    }

    let idx = atomicAdd(&segment_counter.count, 1u);
    emit_segment_at(idx, p0.x, p0.y, p1.x, p1.y);
}

// Estimate segment count for quadratic (with Y-monotonicity)
fn estimate_quad_segments(p0: vec2<f32>, p1: vec2<f32>, p2: vec2<f32>) -> u32 {
    let t_ext = quad_y_extremum(p0.y, p1.y, p2.y);

    if t_ext < 0.0 {
        return wang_quadratic(p0, p1, p2, config.tolerance);
    }

    // Split at extremum - count both halves
    let p_mid = eval_quadratic(p0, p1, p2, t_ext);

    // Control points for first half (de Casteljau)
    let a = vec2<f32>(
        p0.x + t_ext * (p1.x - p0.x),
        p0.y + t_ext * (p1.y - p0.y)
    );
    let n1 = wang_quadratic(p0, a, p_mid, config.tolerance);

    // Control points for second half
    let b = vec2<f32>(
        p1.x + t_ext * (p2.x - p1.x),
        p1.y + t_ext * (p2.y - p1.y)
    );
    let n2 = wang_quadratic(p_mid, b, p2, config.tolerance);

    return n1 + n2;
}

// Estimate segment count for cubic (with Y-monotonicity)
fn estimate_cubic_segments(p0: vec2<f32>, p1: vec2<f32>, p2: vec2<f32>, p3: vec2<f32>) -> u32 {
    let extrema = cubic_y_extrema(p0.y, p1.y, p2.y, p3.y);

    // Base estimate
    let base = wang_cubic(p0, p1, p2, p3, config.tolerance);

    // Multiply by number of monotonic pieces
    let pieces = 1u + u32(extrema.z);
    return base * pieces;
}

// Prepare pass: count segments per element
@compute @workgroup_size(256, 1, 1)
fn cs_flatten_prepare(
    @builtin(global_invocation_id) global_id: vec3<u32>,
) {
    let elem_idx = global_id.x;

    if elem_idx >= config.element_count {
        return;
    }

    let elem = elements[elem_idx];
    let cursor = cursors[elem_idx];

    // Transform cursor position
    let cur = transform_point(cursor.cur_x, cursor.cur_y);

    var count = 0u;

    if elem.verb == 0u {
        // VERB_MOVE_TO
        count = 0u;
    } else if elem.verb == 1u {
        // VERB_LINE_TO
        count = 1u;
    } else if elem.verb == 2u {
        // VERB_QUAD_TO
        let p_idx = elem.point_start;
        let cx = points[p_idx];
        let cy = points[p_idx + 1u];
        let x = points[p_idx + 2u];
        let y = points[p_idx + 3u];

        let p1 = transform_point(cx, cy);
        let p2 = transform_point(x, y);

        count = estimate_quad_segments(cur, p1, p2);
    } else if elem.verb == 3u {
        // VERB_CUBIC_TO
        let p_idx = elem.point_start;
        let c1x = points[p_idx];
        let c1y = points[p_idx + 1u];
        let c2x = points[p_idx + 2u];
        let c2y = points[p_idx + 3u];
        let x = points[p_idx + 4u];
        let y = points[p_idx + 5u];

        let p1 = transform_point(c1x, c1y);
        let p2 = transform_point(c2x, c2y);
        let p3 = transform_point(x, y);

        count = estimate_cubic_segments(cur, p1, p2, p3);
    } else if elem.verb == 4u {
        // VERB_CLOSE
        // Close line if cursor != start
        let start = transform_point(cursor.start_x, cursor.start_y);
        if absf(cur.x - start.x) > 0.000001 || absf(cur.y - start.y) > 0.000001 {
            count = 1u;
        } else {
            count = 0u;
        }
    } else {
        count = 0u;
    }

    segment_counts[elem_idx].count = count;
    segment_counts[elem_idx].offset = 0u;
}

// Flatten pass: generate actual segments
// Assumes segment_counts[i].offset has been computed via prefix sum
@compute @workgroup_size(256, 1, 1)
fn cs_flatten(
    @builtin(global_invocation_id) global_id: vec3<u32>,
) {
    let elem_idx = global_id.x;

    if elem_idx >= config.element_count {
        return;
    }

    let elem = elements[elem_idx];
    let cursor = cursors[elem_idx];
    let out_offset = segment_counts[elem_idx].offset;
    let seg_count = segment_counts[elem_idx].count;

    if seg_count == 0u {
        return;
    }

    // Transform cursor position
    let cur = transform_point(cursor.cur_x, cursor.cur_y);

    if elem.verb == 1u {
        // VERB_LINE_TO
        let p_idx = elem.point_start;
        let x = points[p_idx];
        let y = points[p_idx + 1u];
        let next = transform_point(x, y);

        emit_segment_at(out_offset, cur.x, cur.y, next.x, next.y);
    } else if elem.verb == 2u {
        // VERB_QUAD_TO
        let p_idx = elem.point_start;
        let cx = points[p_idx];
        let cy = points[p_idx + 1u];
        let x = points[p_idx + 2u];
        let y = points[p_idx + 3u];

        let p1 = transform_point(cx, cy);
        let p2 = transform_point(x, y);

        // Generate line segments
        var prev = cur;
        for (var i: u32 = 1u; i <= seg_count; i = i + 1u) {
            let t = f32(i) / f32(seg_count);
            let pt = eval_quadratic(cur, p1, p2, t);
            emit_segment_at(out_offset + i - 1u, prev.x, prev.y, pt.x, pt.y);
            prev = pt;
        }
    } else if elem.verb == 3u {
        // VERB_CUBIC_TO
        let p_idx = elem.point_start;
        let c1x = points[p_idx];
        let c1y = points[p_idx + 1u];
        let c2x = points[p_idx + 2u];
        let c2y = points[p_idx + 3u];
        let x = points[p_idx + 4u];
        let y = points[p_idx + 5u];

        let p1 = transform_point(c1x, c1y);
        let p2 = transform_point(c2x, c2y);
        let p3 = transform_point(x, y);

        // Generate line segments
        var prev = cur;
        for (var i: u32 = 1u; i <= seg_count; i = i + 1u) {
            let t = f32(i) / f32(seg_count);
            let pt = eval_cubic(cur, p1, p2, p3, t);
            emit_segment_at(out_offset + i - 1u, prev.x, prev.y, pt.x, pt.y);
            prev = pt;
        }
    } else if elem.verb == 4u {
        // VERB_CLOSE
        let start = transform_point(cursor.start_x, cursor.start_y);
        emit_segment_at(out_offset, cur.x, cur.y, start.x, start.y);
    }
    // else: MOVE_TO produces no segments
}

// Clear segment counter
@compute @workgroup_size(1, 1, 1)
fn cs_clear_counter() {
    atomicStore(&segment_counter.count, 0u);
}

// Prefix sum for segment offsets (sequential, for small element counts)
// For larger paths, a parallel prefix sum would be more efficient
@compute @workgroup_size(1, 1, 1)
fn cs_prefix_sum() {
    var sum: u32 = 0u;
    for (var i: u32 = 0u; i < config.element_count; i = i + 1u) {
        let count = segment_counts[i].count;
        segment_counts[i].offset = sum;
        sum = sum + count;
    }
    atomicStore(&segment_counter.count, sum);
}

// Get total segment count (for readback)
@compute @workgroup_size(1, 1, 1)
fn cs_get_segment_count() {
    // Dummy entry point - count is read directly from segment_counter buffer
}
