// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"context"
	"math"
	"os"
	"os/exec"
	"runtime/debug"
	"testing"
	"time"
)

// Run each regression in a subprocess: an unbounded subdivision causes a
// fatal stack overflow, which recover cannot catch. Limit the child stack so
// a regression fails quickly without exhausting the test runner's memory.
func TestEdgeBuilderCurveSubdivisionTerminates(t *testing.T) {
	a := math.Float32frombits(0x4a0bba33)
	b := math.Float32frombits(0x4a0bba34)
	y := math.Float32frombits(0x43817f0a)
	cases := []struct {
		name   string
		verb   PathVerb
		points []float32
	}{
		{"cubic_horizontal", CubicTo, []float32{a, y, b, y, b, y, b, y}},
		{"cubic_vertical", CubicTo, []float32{y, a, y, b, y, b, y, b}},
		{"quad_horizontal", QuadTo, []float32{b, y, a, y, a, y}},
		{"quad_vertical", QuadTo, []float32{y, b, y, a, y, a}},
	}
	for _, tc := range cases {
		for _, sign := range []string{"positive", "negative"} {
			for _, clip := range []string{"unclipped", "clipped"} {
				t.Run(tc.name+"/"+sign+"/"+clip, func(t *testing.T) {
					if os.Getenv("GG_CURVE_SUBDIVISION_CHILD") != t.Name() {
						runCurveSubdivisionChild(t)
						return
					}
					debug.SetMaxStack(1 << 20)
					points := append([]float32(nil), tc.points...)
					if sign == "negative" {
						for i := range points {
							points[i] = -points[i]
						}
					}
					eb := NewEdgeBuilder(2)
					eb.SetFlattenCurves(false)
					if clip == "clipped" {
						eb.SetClipRect(&Rect{MinX: 0, MinY: 0, MaxX: 800, MaxY: 600})
					}
					eb.BuildFromPath(&testPath{
						verbs:  []PathVerb{MoveTo, tc.verb},
						points: points,
					}, IdentityTransform{})
					if tc.name == "cubic_horizontal" || tc.name == "quad_horizontal" {
						if !eb.IsEmpty() {
							t.Errorf("horizontal curve produced %d edges", eb.EdgeCount())
						}
					}
				})
			}
		}
	}
}

func runCurveSubdivisionChild(t *testing.T) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^"+t.Name()+"$")
	cmd.Env = append(os.Environ(), "GG_CURVE_SUBDIVISION_CHILD="+t.Name())
	if output, err := cmd.CombinedOutput(); err != nil {
		// The full stack dump repeats the same frames many times.
		if len(output) > 4096 {
			output = output[:4096]
		}
		t.Fatalf("curve subdivision did not complete: %v\n%s", err, output)
	}
}

// A curve far outside the right clip edge still contributes winding to the
// visible fill. Terminating subdivision must not discard that contour edge.
func TestEdgeBuilderLargeCurvesPreserveClippedFill(t *testing.T) {
	const a, b = 2289292.75, 2289293.0
	cases := []struct {
		name   string
		verb   PathVerb
		points []float32
	}{
		{"cubic", CubicTo, []float32{10, 10, a, 10, b, 35, b, 65, b, 90, 10, 90}},
		{"quad", QuadTo, []float32{10, 10, b, 10, a, 50, a, 90, 10, 90}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if os.Getenv("GG_CURVE_SUBDIVISION_CHILD") != t.Name() {
				runCurveSubdivisionChild(t)
				return
			}
			debug.SetMaxStack(1 << 20)
			eb := NewEdgeBuilder(2)
			eb.SetFlattenCurves(false)
			eb.SetClipRect(&Rect{MinX: -2, MinY: -2, MaxX: 102, MaxY: 102})
			eb.BuildFromPath(&testPath{
				verbs:  []PathVerb{MoveTo, LineTo, tc.verb, LineTo, Close},
				points: tc.points,
			}, IdentityTransform{})
			var pixels [100][100]uint8
			NewAnalyticFiller(100, 100).Fill(eb, FillRuleNonZero, func(y int, runs *AlphaRuns) {
				for x, alpha := range runs.Iter() {
					pixels[y][x] = alpha
				}
			})
			for y := range pixels {
				for x, got := range pixels[y] {
					var want uint8
					if x >= 10 && y >= 10 && y < 90 {
						want = 255
					}
					if got != want {
						t.Fatalf("coverage at (%d, %d) = %d, want %d", x, y, got, want)
					}
				}
			}
		})
	}
}
