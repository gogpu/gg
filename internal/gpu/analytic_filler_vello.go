//go:build !nogpu

// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

package gpu

import (
	"github.com/gogpu/gg/internal/raster"
	"math"
)

// AnalyticFillerVello implements the Vello CPU fine rasterizer algorithm.
// This is a direct port of vello_shaders/src/cpu/fine.rs fill_path function.
//
// The algorithm computes per-pixel coverage using exact geometric calculations
// without supersampling.
type AnalyticFillerVello struct {
	width, height int
	alphaRuns     *raster.AlphaRuns
	area          []float32 // Per-pixel winding/area accumulator
}

// NewAnalyticFillerVello creates a new Vello-style analytic filler.
func NewAnalyticFillerVello(width, height int) *AnalyticFillerVello {
	return &AnalyticFillerVello{
		width:     width,
		height:    height,
		alphaRuns: raster.NewAlphaRuns(width),
		area:      make([]float32, width),
	}
}

// Reset clears the filler state for reuse.
func (af *AnalyticFillerVello) Reset() {
	af.alphaRuns.Reset()
}

// lineSegment represents a line segment for the Vello algorithm.
type lineSegment struct {
	x0, y0 float32 // Start point
	x1, y1 float32 // End point
	yEdge  float32 // Y coordinate where winding contribution starts
}

// Fill renders a path using the Vello algorithm.
func (af *AnalyticFillerVello) Fill(
	eb *raster.EdgeBuilder,
	fillRule raster.FillRule,
	callback func(y int, runs *raster.AlphaRuns),
) {
	if eb.IsEmpty() {
		return
	}

	bounds := eb.Bounds()
	aaShift := eb.AAShift()
	//nolint:gosec // aaShift is bounded
	aaScale := float32(int32(1) << uint(aaShift))

	yMin := int(math.Floor(float64(bounds.MinY)))
	yMax := int(math.Ceil(float64(bounds.MaxY)))
	if yMin < 0 {
		yMin = 0
	}
	if yMax > af.height {
		yMax = af.height
	}

	// Collect all line segments from edges
	segments := af.collectSegments(eb, aaScale)

	// Process each scanline
	for y := yMin; y < yMax; y++ {
		af.processScanlineVello(y, segments, fillRule)
		callback(y, af.alphaRuns)
	}
}

// collectSegments extracts line segments from VelloLines.
// Uses original float32 coordinates to avoid fixed-point quantization loss.
func (af *AnalyticFillerVello) collectSegments(eb *raster.EdgeBuilder, _ float32) []lineSegment {
	velloLines := eb.VelloLines()
	segments := make([]lineSegment, 0, len(velloLines))

	for i := range velloLines {
		vl := &velloLines[i]
		// VelloLine stores pixel-space coords, normalized (P0.y <= P1.y)
		x0, y0 := vl.P0[0], vl.P0[1]
		x1, y1 := vl.P1[0], vl.P1[1]

		// yEdge: for downward edges (IsDown=true), yEdge = y0 (top)
		// for upward edges (IsDown=false), yEdge = y1 (bottom)
		var yEdge float32
		if vl.IsDown {
			yEdge = y0
		} else {
			yEdge = y1
		}

		segments = append(segments, lineSegment{
			x0:    x0,
			y0:    y0,
			x1:    x1,
			y1:    y1,
			yEdge: yEdge,
		})
	}

	return segments
}

// processScanlineVello processes a single scanline using Vello's algorithm.
func (af *AnalyticFillerVello) processScanlineVello(
	y int,
	segments []lineSegment,
	fillRule raster.FillRule,
) {
	// Clear area buffer
	for i := range af.area {
		af.area[i] = 0
	}

	yf := float32(y)

	// Process each segment
	for _, seg := range segments {
		delta := [2]float32{
			seg.x1 - seg.x0,
			seg.y1 - seg.y0,
		}

		// y is segment start relative to this pixel row
		relY := seg.y0 - yf

		// Clamp y range to [0, 1] (one pixel row)
		y0 := clamp32(relY, 0, 1)
		y1 := clamp32(relY+delta[1], 0, 1)
		dy := y0 - y1

		// y_edge handles winding contribution
		// delta[0].signum() gives the direction
		var yEdgeContrib float32
		if delta[0] > 0 {
			yEdgeContrib = clamp32(yf-seg.yEdge+1.0, 0, 1)
		} else if delta[0] < 0 {
			yEdgeContrib = -clamp32(yf-seg.yEdge+1.0, 0, 1)
		}

		if dy != 0 {
			// Calculate t parameters for where segment intersects row
			vecYRecip := 1.0 / delta[1]
			t0 := (y0 - relY) * vecYRecip
			t1 := (y1 - relY) * vecYRecip

			// Calculate x positions at t0 and t1
			x0 := seg.x0 + t0*delta[0]
			x1 := seg.x0 + t1*delta[0]

			xmin0 := min32f(x0, x1)
			xmax0 := max32f(x0, x1)

			// Process each pixel in the row
			for i := 0; i < af.width; i++ {
				iF := float32(i)

				// Vello's coverage formula
				xmin := min32f(xmin0-iF, 1.0) - 1.0e-6
				xmax := xmax0 - iF
				b := min32f(xmax, 1.0)
				c := max32f(b, 0.0)
				d := max32f(xmin, 0.0)

				// Area formula from Vello
				denom := xmax - xmin
				var a float32
				if denom != 0 {
					a = (b + 0.5*(d*d-c*c) - xmin) / denom
				}

				af.area[i] += yEdgeContrib + a*dy
			}
		} else if yEdgeContrib != 0 {
			// Horizontal or near-horizontal segment
			for i := 0; i < af.width; i++ {
				af.area[i] += yEdgeContrib
			}
		}
	}

	// Apply fill rule and convert to alpha runs
	af.alphaRuns.Reset()
	var currentAlpha uint8
	runStart := 0

	for i := 0; i < af.width; i++ {
		var coverage float32
		w := af.area[i]

		switch fillRule {
		case raster.FillRuleNonZero:
			coverage = clamp32(velloAbsF32(w), 0, 1)
		case raster.FillRuleEvenOdd:
			// Even-odd: coverage based on fractional part
			absW := velloAbsF32(w)
			im1 := float32(int32(absW*0.5 + 0.5))
			coverage = clamp32(velloAbsF32(absW-2.0*im1), 0, 1)
		}

		alpha := uint8(coverage * 255.0)

		if i == 0 {
			currentAlpha = alpha
			runStart = 0
			continue
		}

		if alpha != currentAlpha {
			if currentAlpha > 0 {
				runLen := i - runStart
				af.alphaRuns.Add(runStart, currentAlpha, runLen-1, 0)
			}
			currentAlpha = alpha
			runStart = i
		}
	}

	if currentAlpha > 0 {
		runLen := af.width - runStart
		af.alphaRuns.Add(runStart, currentAlpha, runLen-1, 0)
	}
}

// velloAbsF32 returns the absolute value of a float32.
func velloAbsF32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
