package gg

import (
	"math"
	"testing"
)

func TestSmoothstepCoverage(t *testing.T) {
	tests := []struct {
		name string
		sdf  float64
		want float64
	}{
		{"fully inside", -2.0, 1.0},
		{"fully outside", 2.0, 0.0},
		{"at center", 0.0, 0.5},
		{"at inner edge", -sdfAntialiasWidth, 1.0},
		{"at outer edge", sdfAntialiasWidth, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := smoothstepCoverage(tt.sdf)
			if math.Abs(got-tt.want) > 1e-6 {
				t.Errorf("smoothstepCoverage(%f) = %f, want %f", tt.sdf, got, tt.want)
			}
		})
	}
}

func TestSmoothstepCoverageMonotonic(t *testing.T) {
	// Coverage must be monotonically decreasing as sdf increases.
	prev := 1.0
	for sdf := -1.5; sdf <= 1.5; sdf += 0.01 {
		curr := smoothstepCoverage(sdf)
		if curr > prev+1e-10 {
			t.Errorf("coverage increased at sdf=%f: prev=%f, curr=%f", sdf, prev, curr)
		}
		prev = curr
	}
}

func TestSDFFilledCircleCoverage(t *testing.T) {
	cx, cy, r := 50.0, 50.0, 20.0

	tests := []struct {
		name    string
		px, py  float64
		wantMin float64
		wantMax float64
	}{
		{"center", 50, 50, 0.99, 1.01},
		{"inside", 55, 50, 0.99, 1.01},
		{"near boundary", 69.5, 50, 0.0, 0.7}, // pixel center near circle edge
		{"just outside", 71, 50, 0.0, 0.1},
		{"far outside", 100, 100, -0.01, 0.01},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SDFFilledCircleCoverage(tt.px+0.5, tt.py+0.5, cx, cy, r)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("coverage at (%f,%f) = %f, want [%f, %f]", tt.px, tt.py, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSDFCircleCoverage(t *testing.T) {
	cx, cy, r := 50.0, 50.0, 20.0
	halfW := 1.0

	tests := []struct {
		name    string
		px, py  float64
		wantMin float64
		wantMax float64
	}{
		{"on stroke center", 70, 50, 0.8, 1.01}, // on the circle at radius=20
		{"inside stroke", 69, 50, 0.8, 1.01},    // within half-width
		{"outside shape far", 100, 100, -0.01, 0.01},
		{"center of circle", 50, 50, -0.01, 0.01}, // far from stroke ring
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SDFCircleCoverage(tt.px+0.5, tt.py+0.5, cx, cy, r, halfW)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("coverage at (%f,%f) = %f, want [%f, %f]", tt.px, tt.py, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSDFFilledRRectCoverage(t *testing.T) {
	cx, cy := 50.0, 50.0
	halfW, halfH := 30.0, 20.0
	cr := 5.0

	tests := []struct {
		name    string
		px, py  float64
		wantMin float64
		wantMax float64
	}{
		{"center", 50, 50, 0.99, 1.01},
		{"inside", 40, 40, 0.99, 1.01},
		{"far outside", 100, 100, -0.01, 0.01},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SDFFilledRRectCoverage(tt.px+0.5, tt.py+0.5, cx, cy, halfW, halfH, cr)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("coverage at (%f,%f) = %f, want [%f, %f]", tt.px, tt.py, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSDFRRectCoverage(t *testing.T) {
	cx, cy := 50.0, 50.0
	halfW, halfH := 30.0, 20.0
	cr := 5.0
	halfStroke := 1.0

	// On the edge of the rrect the SDF should be ~0, so stroke coverage ~1.
	tests := []struct {
		name    string
		px, py  float64
		wantMin float64
		wantMax float64
	}{
		{"on right edge", 80, 50, 0.4, 1.01}, // right side: cx + halfW = 80
		{"center", 50, 50, -0.01, 0.01},      // far from stroke ring
		{"far outside", 100, 100, -0.01, 0.01},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SDFRRectCoverage(tt.px+0.5, tt.py+0.5, cx, cy, halfW, halfH, cr, halfStroke)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("coverage at (%f,%f) = %f, want [%f, %f]", tt.px, tt.py, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func BenchmarkSDFFilledCircleCoverage(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = SDFFilledCircleCoverage(75.5, 50.5, 50, 50, 20)
	}
}

func BenchmarkSDFRRectCoverage(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = SDFRRectCoverage(80.5, 50.5, 50, 50, 30, 20, 5, 1)
	}
}
