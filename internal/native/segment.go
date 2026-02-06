package native

import (
	"math"
)

// LineSegment represents a monotonic line segment for tile processing.
// A monotonic segment has consistent Y direction (always going down or up).
// This simplifies tile intersection calculations.
type LineSegment struct {
	// Start point (X0, Y0) and end point (X1, Y1).
	// For monotonic segments, Y0 <= Y1 (always going down).
	X0, Y0, X1, Y1 float32

	// Winding direction: +1 for left-to-right, -1 for right-to-left.
	// Determined by original path direction before monotonic split.
	Winding int8

	// TileY0, TileY1 are the tile row range this segment spans.
	// Precomputed for efficient coarse rasterization.
	TileY0, TileY1 int32
}

// NewLineSegment creates a new line segment, ensuring Y0 <= Y1.
// The winding is adjusted if the segment is flipped.
func NewLineSegment(x0, y0, x1, y1 float32, winding int8) LineSegment {
	seg := LineSegment{
		X0:      x0,
		Y0:      y0,
		X1:      x1,
		Y1:      y1,
		Winding: winding,
	}

	// Ensure Y0 <= Y1 (monotonic going down)
	if y1 < y0 {
		seg.X0, seg.X1 = x1, x0
		seg.Y0, seg.Y1 = y1, y0
		seg.Winding = -winding
	}

	// Precompute tile range
	seg.TileY0, _ = PixelToTileF(seg.X0, seg.Y0)
	_, seg.TileY0 = PixelToTileF(seg.X0, seg.Y0)
	_, seg.TileY1 = PixelToTileF(seg.X1, seg.Y1)

	return seg
}

// IsHorizontal returns true if the segment is approximately horizontal.
func (s *LineSegment) IsHorizontal() bool {
	const epsilon = 1e-6
	return math.Abs(float64(s.Y1-s.Y0)) < epsilon
}

// IsVertical returns true if the segment is approximately vertical.
func (s *LineSegment) IsVertical() bool {
	const epsilon = 1e-6
	return math.Abs(float64(s.X1-s.X0)) < epsilon
}

// DeltaX returns the X distance of the segment.
func (s *LineSegment) DeltaX() float32 {
	return s.X1 - s.X0
}

// DeltaY returns the Y distance of the segment.
func (s *LineSegment) DeltaY() float32 {
	return s.Y1 - s.Y0
}

// Slope returns the slope (dx/dy) of the segment.
// Returns 0 for horizontal segments.
func (s *LineSegment) Slope() float32 {
	dy := s.Y1 - s.Y0
	if dy == 0 {
		return 0
	}
	return (s.X1 - s.X0) / dy
}

// InverseSlope returns dy/dx. Returns a large value for vertical segments.
func (s *LineSegment) InverseSlope() float32 {
	dx := s.X1 - s.X0
	if dx == 0 {
		if s.Y1 > s.Y0 {
			return 1e10
		}
		return -1e10
	}
	return (s.Y1 - s.Y0) / dx
}

// XAtY returns the X coordinate at a given Y.
// Assumes Y is within the segment's Y range.
func (s *LineSegment) XAtY(y float32) float32 {
	dy := s.Y1 - s.Y0
	if dy == 0 {
		return s.X0
	}
	t := (y - s.Y0) / dy
	return s.X0 + t*(s.X1-s.X0)
}

// YAtX returns the Y coordinate at a given X.
// Assumes X is within the segment's X range.
func (s *LineSegment) YAtX(x float32) float32 {
	dx := s.X1 - s.X0
	if dx == 0 {
		return s.Y0
	}
	t := (x - s.X0) / dx
	return s.Y0 + t*(s.Y1-s.Y0)
}

// Bounds returns the bounding box of the segment.
func (s *LineSegment) Bounds() (minX, minY, maxX, maxY float32) {
	if s.X0 < s.X1 {
		minX, maxX = s.X0, s.X1
	} else {
		minX, maxX = s.X1, s.X0
	}
	// Y0 <= Y1 is guaranteed for monotonic segments
	minY, maxY = s.Y0, s.Y1
	return
}

// CrossesTileRow returns true if the segment crosses the given tile row.
func (s *LineSegment) CrossesTileRow(tileY int32) bool {
	rowTop := float32(tileY << TileShift)
	rowBottom := float32((tileY + 1) << TileShift)
	return s.Y0 < rowBottom && s.Y1 > rowTop
}

// TileXRange returns the range of tile columns this segment touches at a given tile row.
func (s *LineSegment) TileXRange(tileY int32) (minTileX, maxTileX int32) {
	rowTop := float32(tileY << TileShift)
	rowBottom := float32((tileY + 1) << TileShift)

	// Clamp Y range to this row
	y0 := s.Y0
	y1 := s.Y1
	if y0 < rowTop {
		y0 = rowTop
	}
	if y1 > rowBottom {
		y1 = rowBottom
	}

	// Get X range at these Y values
	x0 := s.XAtY(y0)
	x1 := s.XAtY(y1)

	if x0 > x1 {
		x0, x1 = x1, x0
	}

	minTileX, _ = PixelToTileF(x0, y0)
	maxTileX, _ = PixelToTileF(x1, y1)

	return minTileX, maxTileX
}

// SegmentList is a collection of line segments.
type SegmentList struct {
	segments []LineSegment
}

// NewSegmentList creates a new empty segment list.
func NewSegmentList() *SegmentList {
	return &SegmentList{
		segments: make([]LineSegment, 0, 256),
	}
}

// Reset clears the list for reuse.
func (sl *SegmentList) Reset() {
	sl.segments = sl.segments[:0]
}

// Add adds a segment to the list.
func (sl *SegmentList) Add(seg LineSegment) {
	sl.segments = append(sl.segments, seg)
}

// AddLine adds a line segment from (x0,y0) to (x1,y1).
func (sl *SegmentList) AddLine(x0, y0, x1, y1 float32, winding int8) {
	// Skip degenerate segments
	const epsilon = 1e-6
	if math.Abs(float64(y1-y0)) < epsilon && math.Abs(float64(x1-x0)) < epsilon {
		return
	}

	// Skip horizontal segments (they don't contribute to winding)
	if math.Abs(float64(y1-y0)) < epsilon {
		return
	}

	sl.Add(NewLineSegment(x0, y0, x1, y1, winding))
}

// Len returns the number of segments.
func (sl *SegmentList) Len() int {
	return len(sl.segments)
}

// Segments returns the slice of segments.
func (sl *SegmentList) Segments() []LineSegment {
	return sl.segments
}

// Bounds returns the bounding box of all segments.
func (sl *SegmentList) Bounds() (minX, minY, maxX, maxY float32) {
	if len(sl.segments) == 0 {
		return 0, 0, 0, 0
	}

	minX, minY, maxX, maxY = sl.segments[0].Bounds()

	for i := 1; i < len(sl.segments); i++ {
		x0, y0, x1, y1 := sl.segments[i].Bounds()
		if x0 < minX {
			minX = x0
		}
		if y0 < minY {
			minY = y0
		}
		if x1 > maxX {
			maxX = x1
		}
		if y1 > maxY {
			maxY = y1
		}
	}

	return
}

// TileYRange returns the range of tile rows that segments span.
func (sl *SegmentList) TileYRange() (minTileY, maxTileY int32) {
	if len(sl.segments) == 0 {
		return 0, 0
	}

	_, minY, _, maxY := sl.Bounds()
	_, minTileY = PixelToTileF(0, minY)
	_, maxTileY = PixelToTileF(0, maxY)

	return minTileY, maxTileY
}

// SortByTileY sorts segments by their starting tile Y coordinate.
// This enables efficient row-by-row processing.
func (sl *SegmentList) SortByTileY() {
	// Update tile ranges
	for i := range sl.segments {
		_, sl.segments[i].TileY0 = PixelToTileF(sl.segments[i].X0, sl.segments[i].Y0)
		_, sl.segments[i].TileY1 = PixelToTileF(sl.segments[i].X1, sl.segments[i].Y1)
	}

	// Insertion sort (stable, good for nearly-sorted data)
	for i := 1; i < len(sl.segments); i++ {
		j := i
		for j > 0 && sl.segments[j].TileY0 < sl.segments[j-1].TileY0 {
			sl.segments[j], sl.segments[j-1] = sl.segments[j-1], sl.segments[j]
			j--
		}
	}
}

// SegmentsInTileRow returns segments that cross a given tile row.
// The list should be sorted by TileY0 for efficient access.
func (sl *SegmentList) SegmentsInTileRow(tileY int32) []LineSegment {
	var result []LineSegment
	for i := range sl.segments {
		seg := &sl.segments[i]
		if seg.TileY0 <= tileY && seg.TileY1 >= tileY {
			result = append(result, *seg)
		}
		// Early exit if we've passed all relevant segments
		if seg.TileY0 > tileY {
			break
		}
	}
	return result
}
