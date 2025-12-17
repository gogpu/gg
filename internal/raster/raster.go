// Package raster provides scanline rasterization for 2D paths.
package raster

import "math"

// RGBA represents a color (internal copy to avoid import cycle).
type RGBA struct {
	R, G, B, A float64
}

// Pixmap is an interface for writing pixels (avoids import cycle).
type Pixmap interface {
	Width() int
	Height() int
	SetPixel(x, y int, c RGBA)
}

// FillRule specifies how to determine which areas are inside a path.
type FillRule int

const (
	// FillRuleNonZero uses the non-zero winding rule.
	FillRuleNonZero FillRule = iota
	// FillRuleEvenOdd uses the even-odd rule.
	FillRuleEvenOdd
)

// Rasterizer performs scanline rasterization.
type Rasterizer struct {
	width  int
	height int
	aet    *ActiveEdgeTable
}

// NewRasterizer creates a new rasterizer for the given dimensions.
func NewRasterizer(width, height int) *Rasterizer {
	return &Rasterizer{
		width:  width,
		height: height,
		aet:    NewActiveEdgeTable(),
	}
}

// Fill rasterizes a filled path onto a pixmap.
func (r *Rasterizer) Fill(pixmap Pixmap, points []Point, fillRule FillRule, color RGBA) {
	if len(points) < 2 {
		return
	}

	// Build edge list
	edges := make([]Edge, 0, len(points))
	for i := 0; i < len(points)-1; i++ {
		p0 := points[i]
		p1 := points[i+1]

		// Skip horizontal edges
		if math.Abs(p1.Y-p0.Y) < 0.001 {
			continue
		}

		edges = append(edges, NewEdge(p0, p1))
	}

	if len(edges) == 0 {
		return
	}

	// Find y bounds
	yMin := math.MaxFloat64
	yMax := -math.MaxFloat64
	for _, e := range edges {
		yMin = math.Min(yMin, e.y0)
		yMax = math.Max(yMax, e.y1)
	}

	yMinInt := int(math.Floor(yMin))
	yMaxInt := int(math.Ceil(yMax))

	// Clamp to pixmap bounds
	if yMinInt < 0 {
		yMinInt = 0
	}
	if yMaxInt > pixmap.Height() {
		yMaxInt = pixmap.Height()
	}

	// Scanline rasterization
	for y := yMinInt; y < yMaxInt; y++ {
		scanY := float64(y) + 0.5
		r.scanline(pixmap, edges, scanY, fillRule, color)
	}
}

// scanline processes a single scanline.
func (r *Rasterizer) scanline(pixmap Pixmap, edges []Edge, y float64, fillRule FillRule, color RGBA) {
	r.aet.Clear()

	// Add edges that intersect this scanline
	for _, edge := range edges {
		if edge.y0 <= y && y < edge.y1 {
			r.aet.Add(edge)
		}
	}

	if len(r.aet.Edges()) == 0 {
		return
	}

	// Sort edges by x coordinate
	r.aet.Sort()

	// Fill spans based on fill rule
	activeEdges := r.aet.Edges()
	if fillRule == FillRuleNonZero {
		r.fillNonZero(pixmap, activeEdges, int(y), color)
	} else {
		r.fillEvenOdd(pixmap, activeEdges, int(y), color)
	}
}

// fillNonZero fills using the non-zero winding rule.
func (r *Rasterizer) fillNonZero(pixmap Pixmap, edges []ActiveEdge, y int, color RGBA) {
	winding := 0
	var x1 float64

	for i := 0; i < len(edges); i++ {
		edge := edges[i]

		if winding == 0 {
			x1 = edge.x
		}

		winding += edge.dir

		if winding == 0 {
			x2 := edge.x
			r.fillSpan(pixmap, int(x1), int(x2), y, color)
		}
	}
}

// fillEvenOdd fills using the even-odd rule.
func (r *Rasterizer) fillEvenOdd(pixmap Pixmap, edges []ActiveEdge, y int, color RGBA) {
	for i := 0; i+1 < len(edges); i += 2 {
		x1 := int(edges[i].x)
		x2 := int(edges[i+1].x)
		r.fillSpan(pixmap, x1, x2, y, color)
	}
}

// SpanFiller is an optional interface that pixmaps can implement for optimized span filling.
type SpanFiller interface {
	FillSpan(x1, x2, y int, c RGBA)
}

// fillSpan fills a horizontal span of pixels.
func (r *Rasterizer) fillSpan(pixmap Pixmap, x1, x2, y int, color RGBA) {
	if y < 0 || y >= pixmap.Height() {
		return
	}

	if x1 > x2 {
		x1, x2 = x2, x1
	}

	if x1 < 0 {
		x1 = 0
	}
	if x2 > pixmap.Width() {
		x2 = pixmap.Width()
	}

	// Try to use optimized FillSpan if available
	if spanFiller, ok := pixmap.(SpanFiller); ok {
		spanFiller.FillSpan(x1, x2, y, color)
		return
	}

	// Fallback to scalar SetPixel
	for x := x1; x < x2; x++ {
		pixmap.SetPixel(x, y, color)
	}
}

// Stroke rasterizes a stroked path.
func (r *Rasterizer) Stroke(pixmap Pixmap, points []Point, lineWidth float64, color RGBA) {
	if len(points) < 2 {
		return
	}

	if lineWidth < 1 {
		lineWidth = 1
	}

	// Simple stroke: draw thick lines between points
	for i := 0; i < len(points)-1; i++ {
		r.strokeLine(pixmap, points[i], points[i+1], lineWidth, color)
	}
}

// strokeLine draws a thick line.
func (r *Rasterizer) strokeLine(pixmap Pixmap, p0, p1 Point, width float64, color RGBA) {
	dx := p1.X - p0.X
	dy := p1.Y - p0.Y
	length := math.Sqrt(dx*dx + dy*dy)

	if length < 0.001 {
		return
	}

	// Perpendicular vector
	nx := -dy / length
	ny := dx / length

	// Offset by half width
	offset := width / 2

	// Create a quad around the line
	corners := [4]Point{
		{X: p0.X + nx*offset, Y: p0.Y + ny*offset},
		{X: p0.X - nx*offset, Y: p0.Y - ny*offset},
		{X: p1.X - nx*offset, Y: p1.Y - ny*offset},
		{X: p1.X + nx*offset, Y: p1.Y + ny*offset},
	}

	// Convert to edge list and fill
	quadPoints := []Point{corners[0], corners[1], corners[2], corners[3], corners[0]}
	r.Fill(pixmap, quadPoints, FillRuleNonZero, color)
}
