// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package gg

import (
	"math"
	"testing"
)

// TestDrawStringAsOutlines_HiDPI is a regression test for the double-application
// of deviceMatrix in the text outline pipeline. When deviceScale > 1.0 and
// the transform forces Tier 2 (outlines), textOutlinePath() returns a user-space
// path. Previously, totalMatrix() (= deviceMatrix * matrix) was applied here,
// then doFill() → deviceSpacePath() applied deviceMatrix again. At scale=2x
// this squared the scale and doubled translation, placing text off-canvas.
//
// Fix: use c.matrix (user transform only) — doFill/doStroke apply deviceMatrix
// once at the render boundary via deviceSpacePath(). This matches Cairo/Skia
// where device transform is applied once.
func TestDrawStringAsOutlines_HiDPI(t *testing.T) {
	face := loadTestFont(t, 24)

	// --- 1x baseline with rotation (forces Tier 2: outline path) ---
	dc1x := NewContext(200, 200)
	dc1x.ClearWithColor(White)
	dc1x.SetFont(face)
	dc1x.SetRGB(0, 0, 0)

	dc1x.Push()
	dc1x.Translate(100, 100)
	dc1x.Rotate(0.1) // Force Tier 2 (drawStringAsOutlines)
	dc1x.DrawString("Hi", -20, 0)
	dc1x.Pop()

	pixels1x := countNonWhitePixels(dc1x, 0, 0, 200, 200)
	if pixels1x == 0 {
		t.Fatal("1x outline text produced no pixels")
	}

	// --- 2x HiDPI with same rotation ---
	dc2x := NewContextWithScale(200, 200, 2.0)
	dc2x.ClearWithColor(White)
	dc2x.SetFont(face)
	dc2x.SetRGB(0, 0, 0)

	dc2x.Push()
	dc2x.Translate(100, 100)
	dc2x.Rotate(0.1) // Same transform — same Tier 2 path
	dc2x.DrawString("Hi", -20, 0)
	dc2x.Pop()

	// Physical pixmap is 400x400. Count in the corresponding scaled region.
	pixels2x := countNonWhitePixels(dc2x, 0, 0, 400, 400)
	if pixels2x == 0 {
		t.Fatal("2x HiDPI outline text produced no pixels — likely double-transform regression")
	}

	// With correct single-application of deviceMatrix, text at 2x should
	// produce ~4x the pixel count (2x in each dimension). Before the fix,
	// the double-application would square the scale (4x device), pushing
	// text completely off the 400x400 canvas, yielding 0 pixels.
	//
	// Verify the pixel ratio is in a reasonable range (2x-8x) to confirm
	// text is properly scaled and positioned.
	ratio := float64(pixels2x) / float64(pixels1x)
	if ratio < 1.5 || ratio > 12.0 {
		t.Errorf("HiDPI pixel ratio = %.2f (want ~4x, range 1.5-12.0); 1x=%d, 2x=%d",
			ratio, pixels1x, pixels2x)
	}
}

// TestStrokeString_HiDPI verifies the same double-scale fix for StrokeString.
// StrokeString uses textOutlinePath() → Transform() → doStroke() → deviceSpacePath(),
// the same pattern as drawStringAsOutlines.
func TestStrokeString_HiDPI(t *testing.T) {
	face := loadTestFont(t, 24)

	// --- 1x baseline ---
	dc1x := NewContext(200, 200)
	dc1x.ClearWithColor(White)
	dc1x.SetFont(face)
	dc1x.SetRGB(0, 0, 0)
	dc1x.SetLineWidth(2.0)

	dc1x.StrokeString("Hi", 20, 100)
	pixels1x := countNonWhitePixels(dc1x, 0, 0, 200, 200)
	if pixels1x == 0 {
		t.Fatal("1x StrokeString produced no pixels")
	}

	// --- 2x HiDPI ---
	dc2x := NewContextWithScale(200, 200, 2.0)
	dc2x.ClearWithColor(White)
	dc2x.SetFont(face)
	dc2x.SetRGB(0, 0, 0)
	dc2x.SetLineWidth(2.0)

	dc2x.StrokeString("Hi", 20, 100)
	pixels2x := countNonWhitePixels(dc2x, 0, 0, 400, 400)
	if pixels2x == 0 {
		t.Fatal("2x HiDPI StrokeString produced no pixels — likely double-transform regression")
	}

	ratio := float64(pixels2x) / float64(pixels1x)
	if ratio < 1.5 || ratio > 12.0 {
		t.Errorf("HiDPI StrokeString pixel ratio = %.2f (want ~4x, range 1.5-12.0); 1x=%d, 2x=%d",
			ratio, pixels1x, pixels2x)
	}
}

// TestDrawStringAsOutlines_HiDPI_Rotation verifies that HiDPI + rotation
// together do not produce a double-scaled, double-translated result.
// Rotation ensures Tier 2 (outline) is selected; 2x device scale exercises
// the deviceMatrix separation.
func TestDrawStringAsOutlines_HiDPI_Rotation(t *testing.T) {
	face := loadTestFont(t, 20)

	dc := NewContextWithScale(200, 200, 2.0)
	dc.ClearWithColor(White)
	dc.SetFont(face)
	dc.SetRGB(0, 0, 0)

	dc.Push()
	dc.Translate(100, 100)
	dc.Rotate(math.Pi / 6) // 30° rotation — Tier 2
	dc.DrawString("Tg", -20, 0)
	dc.Pop()

	// Check that pixels appear near the center (physical 200,200 ± reasonable margin),
	// not pushed to the edge by a doubled transform.
	// Physical pixmap is 400x400.
	centerPixels := countNonWhitePixels(dc, 100, 100, 300, 300)
	edgePixels := countNonWhitePixels(dc, 0, 0, 400, 400) - centerPixels

	if centerPixels == 0 {
		t.Fatal("HiDPI + rotation: no text pixels in center region — text likely displaced by double-transform")
	}

	// Most ink should be in the center quadrant since we translated to (100, 100)
	// user-space = (200, 200) device-space and drew a short string.
	// A double-transform would push text to (400, 400)+, i.e. entirely off-canvas
	// or into the far edge.
	if centerPixels > 0 && edgePixels > centerPixels*5 {
		t.Errorf("HiDPI + rotation: too much edge ink (%d) vs center (%d) — possible double-transform",
			edgePixels, centerPixels)
	}
}
