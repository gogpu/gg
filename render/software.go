// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"errors"
	"image/color"

	"github.com/gogpu/gg/raster"
)

// SoftwareRenderer is a CPU-based renderer using the core/ package algorithms.
//
// This renderer provides anti-aliased 2D rendering without any GPU dependencies.
// It uses AnalyticFiller for high-quality coverage-based anti-aliasing.
//
// Performance characteristics:
//   - Single-threaded (future: parallel scanline processing)
//   - O(n) where n is the number of pixels covered
//   - Memory: O(width) for scanline buffers
//
// Example:
//
//	renderer := render.NewSoftwareRenderer()
//	target := render.NewPixmapTarget(800, 600)
//	scene := render.NewScene()
//
//	scene.SetFillColor(color.RGBA{255, 0, 0, 255})
//	scene.Circle(400, 300, 100)
//	scene.Fill()
//
//	renderer.Render(target, scene)
//	img := target.Image()
type SoftwareRenderer struct {
	// edgeBuilder is reused for path-to-edge conversion.
	edgeBuilder *raster.EdgeBuilder

	// filler is reused for analytic anti-aliasing.
	filler *raster.AnalyticFiller

	// lastWidth and lastHeight track the filler dimensions.
	lastWidth, lastHeight int
}

// NewSoftwareRenderer creates a new CPU-based software renderer.
func NewSoftwareRenderer() *SoftwareRenderer {
	return &SoftwareRenderer{
		edgeBuilder: raster.NewEdgeBuilder(2), // 4x AA quality
	}
}

// Render draws the scene to the target.
//
// This method processes each drawing command in the scene and renders
// it to the target using CPU-based rasterization.
//
// Returns an error if the target is GPU-only (no Pixels() support).
func (r *SoftwareRenderer) Render(target RenderTarget, scene *Scene) error {
	if target == nil {
		return errors.New("render: nil target")
	}

	pixels := target.Pixels()
	if pixels == nil {
		return errors.New("render: target does not support CPU rendering")
	}

	if scene == nil || scene.IsEmpty() {
		return nil
	}

	width := target.Width()
	height := target.Height()
	stride := target.Stride()

	// Ensure filler is sized correctly
	r.ensureFiller(width, height)

	// Process each command
	for _, cmd := range scene.drawCommands() {
		switch cmd.op {
		case opClear:
			r.renderClear(pixels, width, height, stride, cmd.color)

		case opFill:
			if cmd.path != nil && !cmd.path.IsEmpty() {
				r.renderFill(pixels, width, height, stride, cmd.path, cmd.color, cmd.fillRule)
			}

		case opStroke:
			if cmd.path != nil && !cmd.path.IsEmpty() {
				r.renderStroke(pixels, width, height, stride, cmd.path, cmd.color, cmd.width)
			}
		}
	}

	return nil
}

// Flush ensures all rendering is complete.
// For the software renderer, this is a no-op as operations are synchronous.
func (r *SoftwareRenderer) Flush() error {
	return nil
}

// Capabilities returns the renderer's capabilities.
func (r *SoftwareRenderer) Capabilities() RendererCapabilities {
	return RendererCapabilities{
		IsGPU:                false,
		SupportsAntialiasing: true,
		SupportsBlendModes:   false, // TODO: implement
		SupportsGradients:    false, // TODO: implement
		SupportsTextures:     false, // TODO: implement
		MaxTextureSize:       0,     // No limit
	}
}

// ensureFiller ensures the filler is sized for the target dimensions.
func (r *SoftwareRenderer) ensureFiller(width, height int) {
	if r.filler == nil || r.lastWidth != width || r.lastHeight != height {
		r.filler = raster.NewAnalyticFiller(width, height)
		r.lastWidth = width
		r.lastHeight = height
	}
}

// renderClear fills the entire target with a solid color.
func (r *SoftwareRenderer) renderClear(pixels []byte, width, height, stride int, c color.Color) {
	cr, cg, cb, ca := c.RGBA()
	// Convert from 16-bit to 8-bit (mask ensures value fits in uint8)
	//nolint:gosec // G115: mask ensures no overflow
	pr := uint8((cr >> 8) & 0xFF)
	//nolint:gosec // G115: mask ensures no overflow
	pg := uint8((cg >> 8) & 0xFF)
	//nolint:gosec // G115: mask ensures no overflow
	pb := uint8((cb >> 8) & 0xFF)
	//nolint:gosec // G115: mask ensures no overflow
	pa := uint8((ca >> 8) & 0xFF)

	for y := 0; y < height; y++ {
		rowOffset := y * stride
		for x := 0; x < width; x++ {
			offset := rowOffset + x*4
			pixels[offset] = pr
			pixels[offset+1] = pg
			pixels[offset+2] = pb
			pixels[offset+3] = pa
		}
	}
}

// renderFill fills a path with a solid color.
func (r *SoftwareRenderer) renderFill(pixels []byte, width, height, stride int, path *pathBuilder, c color.Color, fillRule raster.FillRule) {
	// Convert color to RGBA (mask ensures value fits in uint8)
	cr, cg, cb, ca := c.RGBA()
	//nolint:gosec // G115: mask ensures no overflow
	pr := uint8((cr >> 8) & 0xFF)
	//nolint:gosec // G115: mask ensures no overflow
	pg := uint8((cg >> 8) & 0xFF)
	//nolint:gosec // G115: mask ensures no overflow
	pb := uint8((cb >> 8) & 0xFF)
	//nolint:gosec // G115: mask ensures no overflow
	pa := uint8((ca >> 8) & 0xFF)

	// Build edges from path
	r.edgeBuilder.Reset()
	r.edgeBuilder.SetFlattenCurves(true) // Use line segments for reliable rendering
	r.edgeBuilder.BuildFromPath(path, raster.IdentityTransform{})

	if r.edgeBuilder.IsEmpty() {
		return
	}

	// Reset filler
	r.filler.Reset()

	// Fill using analytic anti-aliasing
	r.filler.Fill(r.edgeBuilder, fillRule, func(y int, runs *raster.AlphaRuns) {
		if y < 0 || y >= height {
			return
		}

		rowOffset := y * stride

		// Blend each run to the pixel buffer
		for run := range runs.IterRuns() {
			if run.Alpha == 0 {
				continue
			}

			for i := 0; i < run.Count; i++ {
				x := run.X + i
				if x < 0 || x >= width {
					continue
				}

				offset := rowOffset + x*4

				// Source-over alpha blending
				// out = src * srcAlpha + dst * (1 - srcAlpha)
				alpha := uint16(run.Alpha) * uint16(pa)
				alpha = (alpha + 255) >> 8 // Normalize to 0-255

				if alpha == 0 {
					continue
				}

				if alpha == 255 {
					// Fully opaque
					pixels[offset] = pr
					pixels[offset+1] = pg
					pixels[offset+2] = pb
					pixels[offset+3] = pa
				} else {
					// Alpha blend
					invAlpha := 255 - alpha

					dstR := uint16(pixels[offset])
					dstG := uint16(pixels[offset+1])
					dstB := uint16(pixels[offset+2])
					dstA := uint16(pixels[offset+3])

					outR := (uint16(pr)*alpha + dstR*invAlpha + 127) / 255
					outG := (uint16(pg)*alpha + dstG*invAlpha + 127) / 255
					outB := (uint16(pb)*alpha + dstB*invAlpha + 127) / 255
					outA := (uint16(pa)*alpha + dstA*invAlpha + 127) / 255

					//nolint:gosec // G115: mask ensures no overflow
					pixels[offset] = uint8(outR & 0xFF)
					//nolint:gosec // G115: mask ensures no overflow
					pixels[offset+1] = uint8(outG & 0xFF)
					//nolint:gosec // G115: mask ensures no overflow
					pixels[offset+2] = uint8(outB & 0xFF)
					//nolint:gosec // G115: mask ensures no overflow
					pixels[offset+3] = uint8(outA & 0xFF)
				}
			}
		}
	})
}

// renderStroke strokes a path with a solid color.
//
// Note: This is a simplified implementation that converts stroke to fill
// using path expansion. Full stroke support with line caps/joins is
// planned for a future release.
func (r *SoftwareRenderer) renderStroke(pixels []byte, width, height, stride int, path *pathBuilder, c color.Color, strokeWidth float64) {
	// For now, convert stroke to a filled path by expanding
	// This is a simplified approach - full stroke support requires
	// proper path offsetting with caps and joins

	if strokeWidth <= 0 {
		return
	}

	// Create expanded path
	expandedPath := r.expandStroke(path, strokeWidth)
	if expandedPath == nil || expandedPath.IsEmpty() {
		return
	}

	// Render as filled path
	r.renderFill(pixels, width, height, stride, expandedPath, c, raster.FillRuleNonZero)
}

// expandStroke creates a filled path from a stroked path.
// This is a simplified implementation that creates rectangles for each line segment.
func (r *SoftwareRenderer) expandStroke(path *pathBuilder, width float64) *pathBuilder {
	if len(path.verbs) == 0 || len(path.points) < 2 {
		return nil
	}

	halfWidth := width / 2.0
	expanded := &pathBuilder{
		verbs:  make([]raster.PathVerb, 0, len(path.verbs)*8),
		points: make([]float32, 0, len(path.points)*8),
	}

	var curX, curY float32
	var startX, startY float32
	pointIdx := 0

	for _, verb := range path.verbs {
		switch verb {
		case raster.VerbMoveTo:
			curX = path.points[pointIdx]
			curY = path.points[pointIdx+1]
			startX = curX
			startY = curY
			pointIdx += 2

		case raster.VerbLineTo:
			x := path.points[pointIdx]
			y := path.points[pointIdx+1]
			r.addLineStroke(expanded, curX, curY, x, y, float32(halfWidth))
			curX = x
			curY = y
			pointIdx += 2

		case raster.VerbQuadTo:
			// Simplify: treat as line to endpoint
			x := path.points[pointIdx+2]
			y := path.points[pointIdx+3]
			r.addLineStroke(expanded, curX, curY, x, y, float32(halfWidth))
			curX = x
			curY = y
			pointIdx += 4

		case raster.VerbCubicTo:
			// Simplify: treat as line to endpoint
			x := path.points[pointIdx+4]
			y := path.points[pointIdx+5]
			r.addLineStroke(expanded, curX, curY, x, y, float32(halfWidth))
			curX = x
			curY = y
			pointIdx += 6

		case raster.VerbClose:
			if curX != startX || curY != startY {
				r.addLineStroke(expanded, curX, curY, startX, startY, float32(halfWidth))
			}
			curX = startX
			curY = startY
		}
	}

	return expanded
}

// addLineStroke adds a stroked line segment as a rectangle.
func (r *SoftwareRenderer) addLineStroke(path *pathBuilder, x0, y0, x1, y1, halfWidth float32) {
	// Calculate direction vector
	dx := x1 - x0
	dy := y1 - y0

	// Length of segment
	length := sqrt32(dx*dx + dy*dy)
	if length < 1e-6 {
		return // Degenerate segment
	}

	// Perpendicular vector (normalized and scaled by half width)
	px := -dy / length * halfWidth
	py := dx / length * halfWidth

	// Four corners of the stroke rectangle
	// CCW winding for correct fill
	path.verbs = append(path.verbs,
		raster.VerbMoveTo,
		raster.VerbLineTo,
		raster.VerbLineTo,
		raster.VerbLineTo,
		raster.VerbClose,
	)
	path.points = append(path.points,
		x0+px, y0+py, // Top-left
		x1+px, y1+py, // Top-right
		x1-px, y1-py, // Bottom-right
		x0-px, y0-py, // Bottom-left
	)
}

// sqrt32 is a helper for float32 square root.
func sqrt32(x float32) float32 {
	// Use Newton-Raphson iteration
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// Ensure SoftwareRenderer implements Renderer and CapableRenderer.
var (
	_ Renderer        = (*SoftwareRenderer)(nil)
	_ CapableRenderer = (*SoftwareRenderer)(nil)
)
