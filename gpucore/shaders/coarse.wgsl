// coarse.wgsl - Coarse rasterization (tile binning) compute shader
//
// This shader bins line segments into the tiles they intersect.
// For each segment, it determines which tiles the segment crosses and
// writes tile-segment mapping entries to an output buffer.
//
// Algorithm: Each thread processes one segment and determines which tiles
// it intersects using line-tile intersection tests. Results are written
// using atomics to a shared allocation counter.
//
// Workgroup layout: 256 threads, each processing one segment

// Note: Constants are inlined due to naga constant lowering limitation.
// Tile dimensions (matches CPU TileSize): TILE_SIZE = 4

// Line segment structure (matches GPUSegment in Go)
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

// Configuration parameters
struct CoarseConfig {
    viewport_width: u32,  // Viewport width in pixels
    viewport_height: u32, // Viewport height in pixels
    tile_columns: u32,    // Number of tile columns
    tile_rows: u32,       // Number of tile rows
    segment_count: u32,   // Number of segments to process
    max_entries: u32,     // Maximum number of tile entries
    padding1: u32,
    padding2: u32,
}

// Tile-segment mapping entry (output)
struct TileEntry {
    tile_x: u32,      // Tile X coordinate
    tile_y: u32,      // Tile Y coordinate
    segment_idx: u32, // Index into segments array
    winding_flag: u32, // Whether this contributes winding (0 or 1)
}

// Atomic counter for allocation
struct AtomicCounter {
    count: atomic<u32>,
}

// Bind group 0: Input data
@group(0) @binding(0) var<uniform> config: CoarseConfig;
@group(0) @binding(1) var<storage, read> segments: array<Segment>;

// Bind group 1: Output data
@group(1) @binding(0) var<storage, read_write> tile_entries: array<TileEntry>;
@group(1) @binding(1) var<storage, read_write> entry_counter: AtomicCounter;

// Helper: minimum of two floats
fn minf(a: f32, b: f32) -> f32 {
    if a < b { return a; }
    return b;
}

// Helper: maximum of two floats
fn maxf(a: f32, b: f32) -> f32 {
    if a > b { return a; }
    return b;
}

// Helper: clamp unsigned integer to range
fn clamp_u32(val: i32, min_val: i32, max_val: i32) -> u32 {
    if val < min_val { return u32(min_val); }
    if val > max_val { return u32(max_val); }
    return u32(val);
}

// Allocate space for a tile entry and write it
fn emit_tile_entry(tile_x: u32, tile_y: u32, segment_idx: u32, winding_flag: u32) {
    // Atomically allocate an entry slot
    let idx = atomicAdd(&entry_counter.count, 1u);

    // Bounds check
    if idx >= config.max_entries {
        return;
    }

    // Write the entry
    tile_entries[idx].tile_x = tile_x;
    tile_entries[idx].tile_y = tile_y;
    tile_entries[idx].segment_idx = segment_idx;
    tile_entries[idx].winding_flag = winding_flag;
}

// Process a vertical line segment
fn process_vertical_line(
    seg_idx: u32,
    x: f32,
    top_y: f32,
    bottom_y: f32,
) {
    let tile_x = clamp_u32(i32(x / 4.0), 0, i32(config.tile_columns) - 1);

    let y_top_tiles = clamp_u32(i32(top_y / 4.0), 0, i32(config.tile_rows));
    let y_bottom_tiles = clamp_u32(i32((bottom_y + 0.999999) / 4.0), 0, i32(config.tile_rows));

    if y_top_tiles >= y_bottom_tiles {
        return;
    }

    // First tile - check if line starts above tile top
    let is_start_culled = top_y < 0.0;
    if !is_start_culled {
        let winding_flag = u32(f32(y_top_tiles) * 4.0 >= top_y);
        emit_tile_entry(tile_x, y_top_tiles, seg_idx, winding_flag);
    }

    // Middle tiles - line crosses top and bottom
    var y_start = y_top_tiles;
    if !is_start_culled {
        y_start = y_start + 1u;
    }
    let y_end_idx = clamp_u32(i32(bottom_y / 4.0), 0, i32(config.tile_rows));

    for (var y = y_start; y < y_end_idx; y = y + 1u) {
        emit_tile_entry(tile_x, y, seg_idx, 1u); // Winding always true for middle tiles
    }

    // Last tile if line doesn't end exactly on tile boundary
    let bottom_floor = f32(i32(bottom_y / 4.0) * 4);
    if bottom_y != bottom_floor && y_end_idx < config.tile_rows {
        emit_tile_entry(tile_x, y_end_idx, seg_idx, 1u);
    }
}

// Process a row of tiles for a sloped line
fn process_row(
    seg_idx: u32,
    line_top_y: f32,
    line_top_x: f32,
    x_slope: f32,
    line_left_x: f32,
    line_right_x: f32,
    row_top_y: f32,
    row_bottom_y: f32,
    y_idx: u32,
    winding: u32,
    dx_dir: bool,
) {
    // Calculate X range for this row using line equation
    let row_top_x = line_top_x + (row_top_y - line_top_y) * x_slope;
    let row_bottom_x = line_top_x + (row_bottom_y - line_top_y) * x_slope;

    // Clamp to line bounds
    let row_left_x = maxf(minf(row_top_x, row_bottom_x), line_left_x);
    let row_right_x = minf(maxf(row_top_x, row_bottom_x), line_right_x);

    let x_start = clamp_u32(i32(row_left_x / 4.0), 0, i32(config.tile_columns) - 1);
    let x_end = clamp_u32(i32(row_right_x / 4.0), 0, i32(config.tile_columns) - 1);

    if x_start > x_end {
        return;
    }

    // Single tile case
    if x_start == x_end {
        emit_tile_entry(x_start, y_idx, seg_idx, winding);
        return;
    }

    // Multiple tiles
    // First tile gets winding based on direction
    if dx_dir {
        // Going right: left tile gets winding
        emit_tile_entry(x_start, y_idx, seg_idx, winding);
    } else {
        // Going left: right tile gets winding
        emit_tile_entry(x_start, y_idx, seg_idx, 0u);
    }

    // Middle tiles (no winding)
    for (var x = x_start + 1u; x < x_end; x = x + 1u) {
        emit_tile_entry(x, y_idx, seg_idx, 0u);
    }

    // Last tile
    if dx_dir {
        emit_tile_entry(x_end, y_idx, seg_idx, 0u);
    } else {
        emit_tile_entry(x_end, y_idx, seg_idx, winding);
    }
}

// Process a sloped line segment
fn process_sloped_line(
    seg_idx: u32,
    line_top_x: f32,
    line_top_y: f32,
    line_bottom_x: f32,
    line_bottom_y: f32,
    y_top_tiles: u32,
    y_bottom_tiles: u32,
) {
    let dx = line_bottom_x - line_top_x;
    let dy = line_bottom_y - line_top_y;
    let x_slope = dx / dy;

    // Determine winding direction based on slope direction
    let dx_dir = line_bottom_x >= line_top_x;

    let line_left_x = minf(line_top_x, line_bottom_x);
    let line_right_x = maxf(line_top_x, line_bottom_x);

    let is_start_culled = line_top_y < 0.0;

    // Process first row (if not culled)
    if !is_start_culled {
        let y = f32(y_top_tiles) * 4.0;
        let row_bottom_y = minf(y + 4.0, line_bottom_y);
        let winding = u32(y >= line_top_y);
        process_row(seg_idx, line_top_y, line_top_x, x_slope, line_left_x, line_right_x, y, row_bottom_y, y_top_tiles, winding, dx_dir);
    }

    // Process middle rows
    var y_start_middle = y_top_tiles;
    if !is_start_culled {
        y_start_middle = y_start_middle + 1u;
    }
    let y_end_middle = clamp_u32(i32(line_bottom_y / 4.0), 0, i32(config.tile_rows));

    for (var y = y_start_middle; y < y_end_middle; y = y + 1u) {
        let yf = f32(y) * 4.0;
        let row_bottom_y = minf(yf + 4.0, line_bottom_y);
        process_row(seg_idx, line_top_y, line_top_x, x_slope, line_left_x, line_right_x, yf, row_bottom_y, y, 1u, dx_dir);
    }

    // Process last row if line doesn't end on row boundary
    let bottom_floor = f32(i32(line_bottom_y / 4.0) * 4);
    if line_bottom_y != bottom_floor && y_end_middle < config.tile_rows {
        if is_start_culled || y_end_middle != y_top_tiles {
            let yf = f32(y_end_middle) * 4.0;
            process_row(seg_idx, line_top_y, line_top_x, x_slope, line_left_x, line_right_x, yf, line_bottom_y, y_end_middle, 1u, dx_dir);
        }
    }
}

// Main entry point: process one segment per thread
@compute @workgroup_size(256, 1, 1)
fn cs_coarse(
    @builtin(global_invocation_id) global_id: vec3<u32>,
) {
    let seg_idx = global_id.x;

    // Bounds check
    if seg_idx >= config.segment_count {
        return;
    }

    let seg = segments[seg_idx];

    // Convert to tile coordinates
    let p0x = seg.x0 / 4.0;  // TILE_SIZE = 4
    let p0y = seg.y0 / 4.0;
    let p1x = seg.x1 / 4.0;
    let p1y = seg.y1 / 4.0;

    // Determine left/right bounds
    let line_left_x = minf(p0x, p1x);
    let line_right_x = maxf(p0x, p1x);

    // Cull lines fully to the right of viewport
    if line_left_x > f32(config.tile_columns) {
        return;
    }

    // Line is monotonic (Y0 <= Y1)
    let line_top_y = p0y;
    let line_top_x = p0x;
    let line_bottom_y = p1y;
    let line_bottom_x = p1x;

    // Clamp to viewport rows
    let y_top_tiles = clamp_u32(i32(line_top_y), 0, i32(config.tile_rows));
    let y_bottom_tiles = clamp_u32(i32(line_bottom_y + 0.999999), 0, i32(config.tile_rows));

    // Skip horizontal lines or lines fully outside viewport
    if y_top_tiles >= y_bottom_tiles {
        return;
    }

    // Get tile coordinates for endpoints
    let p0_tile_x = i32(line_top_x);
    let p0_tile_y = i32(line_top_y);
    let p1_tile_x = i32(line_bottom_x);
    let p1_tile_y = i32(line_bottom_y);

    // Check if both endpoints are in the same tile
    let same_x = p0_tile_x == p1_tile_x;
    let same_y = p0_tile_y == p1_tile_y;

    if same_x && same_y {
        // Line fully contained in single tile
        let x = clamp_u32(i32(line_left_x), 0, i32(config.tile_columns) - 1);
        // Set winding if line crosses tile top edge
        let winding = u32(p0_tile_y >= i32(y_top_tiles));
        emit_tile_entry(x, y_top_tiles, seg_idx, winding);
        return;
    }

    // Handle vertical lines specially
    if line_left_x == line_right_x {
        process_vertical_line(seg_idx, seg.x0, seg.y0, seg.y1);
        return;
    }

    // General sloped line
    process_sloped_line(seg_idx, seg.x0, seg.y0, seg.x1, seg.y1, y_top_tiles, y_bottom_tiles);
}

// Entry point to clear the entry counter
@compute @workgroup_size(1, 1, 1)
fn cs_clear_counter() {
    atomicStore(&entry_counter.count, 0u);
}

// Entry point to get the entry count (for readback)
@compute @workgroup_size(1, 1, 1)
fn cs_get_count(
    @builtin(global_invocation_id) global_id: vec3<u32>,
) {
    // This is a dummy entry point - count is read directly from buffer
    // Included for potential debug/validation use
}
