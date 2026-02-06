// fine.wgsl - Fine rasterization compute shader for sparse strips
//
// This shader calculates per-pixel anti-aliased coverage using analytic area
// computation. It processes tiles (4x4 pixels) assigned by the coarse rasterizer
// and outputs coverage values for each pixel.
//
// Algorithm: For each line segment crossing a tile, compute the exact trapezoidal
// area it covers in each pixel using analytic geometry. The accumulated area
// determines the coverage (alpha) for anti-aliasing.
//
// Workgroup layout: 16 threads (one per pixel in 4x4 tile)

// Note: Constants are inlined due to naga constant lowering limitation.
// Tile dimensions (matches CPU TileSize): TILE_SIZE = 4, TILE_PIXELS = 16
// Fill rule: FILL_NONZERO = 0, FILL_EVENODD = 1

// Line segment structure (matches Go LineSegment, padded for GPU alignment)
struct Segment {
    x0: f32,      // Start X coordinate
    y0: f32,      // Start Y coordinate
    x1: f32,      // End X coordinate
    y1: f32,      // End Y coordinate
    winding: i32, // Winding direction: +1 or -1
    tile_y0: i32, // Starting tile Y (precomputed)
    tile_y1: i32, // Ending tile Y (precomputed)
    padding: i32,
}

// Tile-segment mapping entry (from coarse rasterizer)
struct TileSegmentRef {
    tile_x: u32,      // Tile X coordinate
    tile_y: u32,      // Tile Y coordinate
    segment_idx: u32, // Index into segments array
    winding_flag: u32, // Whether this contributes winding (0 or 1)
}

// Tile info for processing
struct TileInfo {
    tile_x: u32,
    tile_y: u32,
    start_idx: u32,   // Start index in tile_segments
    count: u32,       // Number of segments for this tile
    backdrop: i32,    // Accumulated winding from left
    padding1: u32,
    padding2: u32,
    padding3: u32,
}

// Configuration parameters
struct Config {
    viewport_width: u32,
    viewport_height: u32,
    tile_columns: u32,
    tile_rows: u32,
    tile_count: u32,       // Number of tiles to process
    fill_rule: u32,        // 0 = NonZero, 1 = EvenOdd
    padding1: u32,
    padding2: u32,
}

// Bind group 0: Input data
@group(0) @binding(0) var<uniform> config: Config;
@group(0) @binding(1) var<storage, read> segments: array<Segment>;
@group(0) @binding(2) var<storage, read> tile_segments: array<TileSegmentRef>;
@group(0) @binding(3) var<storage, read> tiles: array<TileInfo>;

// Bind group 1: Output data
// Note: Using regular u32 array instead of atomic for naga compatibility.
// Each workgroup processes exactly one tile, so no atomics needed within a workgroup.
@group(1) @binding(0) var<storage, read_write> coverage: array<u32>;

// Workgroup shared memory for per-pixel winding accumulation
var<workgroup> sh_tile_winding: array<f32, 16>;

// Convert winding to coverage based on fill rule
fn winding_to_coverage(winding: f32, fill_rule: u32) -> f32 {
    var cov: f32;

    if fill_rule == 0u {  // FILL_NONZERO
        // NonZero: coverage = |winding|, clamped to [0, 1]
        cov = abs(winding);
        cov = min(cov, 1.0);
    } else {
        // EvenOdd: coverage based on fractional part
        var abs_winding: f32 = abs(winding);
        var im1: f32 = f32(i32(abs_winding * 0.5 + 0.5));
        cov = abs(abs_winding - 2.0 * im1);
        cov = min(cov, 1.0);
    }

    return cov;
}

// Compute area contribution of a segment to a single pixel
// Note: Passing individual floats instead of struct due to naga struct parameter limitation
fn compute_pixel_area(
    seg_x0: f32, seg_y0: f32, seg_x1: f32, seg_y1: f32, seg_winding: i32,
    tile_left_x: f32,
    tile_top_y: f32,
    px_x: u32,
    px_y: u32,
) -> f32 {
    // Convert to tile-relative coordinates
    var p0x: f32 = seg_x0 - tile_left_x;
    var p0y: f32 = seg_y0 - tile_top_y;
    var p1x: f32 = seg_x1 - tile_left_x;
    var p1y: f32 = seg_y1 - tile_top_y;

    // Skip horizontal segments
    if p0y == p1y {
        return 0.0;
    }

    var sign: f32 = f32(seg_winding);

    // Line is monotonic (Y0 <= Y1)
    var line_top_y: f32 = p0y;
    var line_top_x: f32 = p0x;
    var line_bottom_y: f32 = p1y;

    // Calculate slopes
    var dy: f32 = line_bottom_y - line_top_y;
    var dx: f32 = p1x - p0x;

    var y_slope: f32;
    if dx == 0.0 {
        // Vertical line
        if line_bottom_y > line_top_y {
            y_slope = 1e10;
        } else {
            y_slope = -1e10;
        }
    } else {
        y_slope = dy / dx;
    }
    var x_slope: f32 = 1.0 / y_slope;

    // Pixel row bounds
    var px_top_y: f32 = f32(px_y);
    var px_bottom_y: f32 = px_top_y + 1.0;
    var px_left_x: f32 = f32(px_x);
    var px_right_x: f32 = px_left_x + 1.0;

    // Clamp line Y range to this pixel row
    var y_min: f32 = max(line_top_y, px_top_y);
    var y_max: f32 = min(line_bottom_y, px_bottom_y);

    // Check if line crosses this row
    if y_min >= y_max {
        return 0.0;
    }

    // Calculate Y coordinates where line intersects pixel left and right edges
    var line_px_left_y: f32 = line_top_y + (px_left_x - line_top_x) * y_slope;
    var line_px_right_y: f32 = line_top_y + (px_right_x - line_top_x) * y_slope;

    // Clamp to pixel row bounds and line Y bounds
    line_px_left_y = clamp(line_px_left_y, y_min, y_max);
    line_px_right_y = clamp(line_px_right_y, y_min, y_max);

    // Calculate X coordinates at the clamped Y values
    var line_px_left_yx: f32 = line_top_x + (line_px_left_y - line_top_y) * x_slope;
    var line_px_right_yx: f32 = line_top_x + (line_px_right_y - line_top_y) * x_slope;

    // Height of line segment within this pixel
    var pixel_h: f32 = abs(line_px_right_y - line_px_left_y);

    // Trapezoidal area: area between line and pixel right edge
    var area: f32 = 0.5 * pixel_h * (2.0 * px_right_x - line_px_right_yx - line_px_left_yx);

    return area * sign;
}

// Main compute shader entry point
// One workgroup per tile, 16 threads per workgroup (one per pixel)
@compute @workgroup_size(16, 1, 1)
fn cs_fine(
    @builtin(workgroup_id) wg_id: vec3<u32>,
    @builtin(local_invocation_id) local_id: vec3<u32>,
) {
    var tile_idx: u32 = wg_id.x;

    // Bounds check
    if tile_idx >= config.tile_count {
        return;
    }

    var tile: TileInfo = tiles[tile_idx];

    // Pixel coordinates within the tile (0-15 -> 4x4 grid)
    var pixel_idx: u32 = local_id.x;
    var px_x: u32 = pixel_idx % 4u;  // TILE_SIZE = 4
    var px_y: u32 = pixel_idx / 4u;  // TILE_SIZE = 4

    // Tile pixel origin coordinates
    var tile_left_x: f32 = f32(tile.tile_x * 4u);  // TILE_SIZE = 4
    var tile_top_y: f32 = f32(tile.tile_y * 4u);   // TILE_SIZE = 4

    // Initialize winding with backdrop
    var winding: f32 = f32(tile.backdrop);

    // Process all segments for this tile
    for (var seg_i: u32 = 0u; seg_i < tile.count; seg_i = seg_i + 1u) {
        var ref: TileSegmentRef = tile_segments[tile.start_idx + seg_i];
        var seg: Segment = segments[ref.segment_idx];

        // Compute this pixel's area contribution (pass segment fields individually)
        var area: f32 = compute_pixel_area(
            seg.x0, seg.y0, seg.x1, seg.y1, seg.winding,
            tile_left_x, tile_top_y, px_x, px_y
        );
        winding = winding + area;
    }

    // Convert winding to coverage
    var cov: f32 = winding_to_coverage(winding, config.fill_rule);

    // Convert to 8-bit coverage
    var alpha: u32 = u32(cov * 255.0 + 0.5);

    // Calculate output index
    // Coverage buffer: tiles stored sequentially, each tile is 16 bytes (4x4 u8)
    // Packed as u32 (4 bytes per u32), so 4 u32s per tile
    var tile_offset: u32 = tile_idx * 4u;  // 4 u32s per tile (16 bytes)

    // Write coverage to shared memory first, then do a coordinated write
    sh_tile_winding[pixel_idx] = f32(alpha);
    workgroupBarrier();

    // Only 4 threads (one per u32) do the actual write
    if pixel_idx < 4u {
        var base: u32 = pixel_idx * 4u;
        var packed: u32 = 0u;
        packed = packed | (u32(sh_tile_winding[base + 0u]) << 0u);
        packed = packed | (u32(sh_tile_winding[base + 1u]) << 8u);
        packed = packed | (u32(sh_tile_winding[base + 2u]) << 16u);
        packed = packed | (u32(sh_tile_winding[base + 3u]) << 24u);
        coverage[tile_offset + pixel_idx] = packed;
    }
}

// Entry point for solid tiles (backdrop only, no segments)
// These tiles have uniform coverage and can be processed more efficiently
@compute @workgroup_size(16, 1, 1)
fn cs_fine_solid(
    @builtin(workgroup_id) wg_id: vec3<u32>,
    @builtin(local_invocation_id) local_id: vec3<u32>,
) {
    var tile_idx: u32 = wg_id.x;

    if tile_idx >= config.tile_count {
        return;
    }

    var tile: TileInfo = tiles[tile_idx];
    var pixel_idx: u32 = local_id.x;

    // Solid tiles have uniform coverage based on backdrop
    var cov: f32 = winding_to_coverage(f32(tile.backdrop), config.fill_rule);
    var alpha: u32 = u32(cov * 255.0 + 0.5);

    // Write coverage to shared memory
    sh_tile_winding[pixel_idx] = f32(alpha);
    workgroupBarrier();

    // Pack and write (same as cs_fine)
    if pixel_idx < 4u {
        var tile_offset: u32 = tile_idx * 4u;
        var base: u32 = pixel_idx * 4u;
        var packed: u32 = 0u;
        packed = packed | (u32(sh_tile_winding[base + 0u]) << 0u);
        packed = packed | (u32(sh_tile_winding[base + 1u]) << 8u);
        packed = packed | (u32(sh_tile_winding[base + 2u]) << 16u);
        packed = packed | (u32(sh_tile_winding[base + 3u]) << 24u);
        coverage[tile_offset + pixel_idx] = packed;
    }
}

// Entry point for clearing coverage buffer
@compute @workgroup_size(64, 1, 1)
fn cs_clear_coverage(
    @builtin(global_invocation_id) global_id: vec3<u32>,
) {
    var idx: u32 = global_id.x;
    var total_words: u32 = config.tile_count * 4u;  // 4 u32s per tile

    if idx < total_words {
        coverage[idx] = 0u;
    }
}
