// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package raster

import (
	"testing"
)

// =============================================================================
// Skia AAA formula unit tests.
//
// Every formula ported from SkScan_AAAPath.cpp is tested with expected values
// derived from Skia's reference implementation. The goal is to catch any
// accidental change to a formula — each test pins the exact numeric output.
//
// Skia source reference:
//   skia/src/core/SkScan_AAAPath.cpp (2024 Skia HEAD)
//
// Notation:
//   SK_Fixed1    = 1 << 16 = 65536  (skFixed1 in our code)
//   SK_FixedHalf = 1 << 15 = 32768  (skFixedHalf in our code)
// =============================================================================

// --- 1. trapezoidToAlpha -------------------------------------------------
// Skia: trapezoid_to_alpha (SkScan_AAAPath.cpp:547-553)
//
//   Area of trapezoid with height = 1 full pixel.
//   Two parallel sides l1, l2 (in 16.16 fixed-point).
//   Result = (l1 + l2) / 2 >> 8, clamped [0, 255].

func TestFormulaTrapezoidToAlpha(t *testing.T) {
	tests := []struct {
		name string
		l1   int32
		l2   int32
		want uint8
	}{
		// Full pixel: l1=l2=SK_Fixed1 → (65536+65536)/2 = 65536 → 65536>>8 = 256 → clamped 255
		{"full pixel both sides", skFixed1, skFixed1, 255},

		// Half pixel: l1=l2=SK_Fixed1/2 → (32768+32768)/2 = 32768 → 32768>>8 = 128
		// But int32 arithmetic: (32768 + 32768) / 2 = 32768; 32768 >> 8 = 128.
		// Skia uses integer division (truncation), so half of 65536 = 32768,
		// then >>8 = 128.
		{"half pixel both sides", skFixed1 / 2, skFixed1 / 2, 128},

		// Triangle: l1=0, l2=SK_Fixed1 → (0+65536)/2 = 32768 → 32768>>8 = 128
		{"triangle zero to full", 0, skFixed1, 128},

		// Triangle reversed: l1=SK_Fixed1, l2=0 → same result
		{"triangle full to zero", skFixed1, 0, 128},

		// Zero: l1=l2=0 → 0
		{"zero both sides", 0, 0, 0},

		// One pixel side: l1=0, l2=SK_Fixed1/2 → (0+32768)/2 = 16384 → 16384>>8 = 64
		{"triangle zero to half", 0, skFixed1 / 2, 64},

		// Negative values clamped to 0 before computation
		{"negative l1 clamped", -skFixed1, skFixed1, 128},
		{"negative l2 clamped", skFixed1, -skFixed1, 128},
		{"both negative clamped", -100, -200, 0},

		// Quarter pixel: l1=l2=SK_Fixed1/4 = 16384 → (16384+16384)/2 = 16384 → >>8 = 64
		{"quarter pixel both", skFixed1 / 4, skFixed1 / 4, 64},

		// Three-quarter pixel: l1=l2=SK_Fixed1*3/4 = 49152 → (49152+49152)/2 = 49152 → >>8 = 192
		{"three quarter both", skFixed1 * 3 / 4, skFixed1 * 3 / 4, 192},

		// Asymmetric: l1=SK_Fixed1/4, l2=SK_Fixed1*3/4 → (16384+49152)/2 = 32768 → >>8 = 128
		{"quarter and three quarter", skFixed1 / 4, skFixed1 * 3 / 4, 128},

		// Small value: l1=l2=256 → (256+256)/2 = 256 → >>8 = 1
		{"tiny value", 256, 256, 1},

		// Very small: l1=l2=128 → (128+128)/2 = 128 → >>8 = 0
		{"sub-threshold value", 128, 128, 0},

		// Over-full: l1=l2=2*SK_Fixed1 → clamped to 255
		{"over full clamped", 2 * skFixed1, 2 * skFixed1, 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trapezoidToAlpha(tt.l1, tt.l2)
			if got != tt.want {
				t.Errorf("trapezoidToAlpha(%d, %d) = %d, want %d", tt.l1, tt.l2, got, tt.want)
			}
		})
	}
}

// --- 2. partialTriangleToAlpha -------------------------------------------
// Skia: partial_triangle_to_alpha (SkScan_AAAPath.cpp:555-562)
//
//   Area of right triangle with legs a and a*b.
//   Both in 16.16 fixed-point, a clamped to [0, SK_Fixed1].
//   result = ((a>>11) * (a>>11) * (b>>11)) >> 8, masked to 8 bits.

func TestFormulaPartialTriangleToAlpha(t *testing.T) {
	tests := []struct {
		name string
		a    int32
		b    int32
		want uint8
	}{
		// a=SK_Fixed1 (65536), b=SK_Fixed1 (65536):
		//   a>>11 = 32, b>>11 = 32
		//   area = 32 * 32 * 32 = 32768
		//   result = (32768 >> 8) & 0xFF = 128 & 0xFF = 128
		{"full a full b", skFixed1, skFixed1, 128},

		// a=0 → always 0 regardless of b
		{"zero a", 0, skFixed1, 0},
		{"zero a zero b", 0, 0, 0},

		// b=0 → always 0 regardless of a
		{"zero b", skFixed1, 0, 0},

		// a=SK_Fixed1/2 (32768), b=SK_Fixed1 (65536):
		//   a>>11 = 16, b>>11 = 32
		//   area = 16 * 16 * 32 = 8192
		//   result = (8192 >> 8) & 0xFF = 32
		{"half a full b", skFixed1 / 2, skFixed1, 32},

		// a=SK_Fixed1/4 (16384), b=SK_Fixed1 (65536):
		//   a>>11 = 8, b>>11 = 32
		//   area = 8 * 8 * 32 = 2048
		//   result = (2048 >> 8) & 0xFF = 8
		{"quarter a full b", skFixed1 / 4, skFixed1, 8},

		// a=SK_Fixed1, b=SK_Fixed1/2 (32768):
		//   a>>11 = 32, b>>11 = 16
		//   area = 32 * 32 * 16 = 16384
		//   result = (16384 >> 8) & 0xFF = 64
		{"full a half b", skFixed1, skFixed1 / 2, 64},

		// Negative a → abs applied: same as positive
		{"negative a", -skFixed1, skFixed1, 128},

		// Negative b → abs applied: same as positive
		{"negative b", skFixed1, -skFixed1, 128},

		// a > SK_Fixed1 → clamped to SK_Fixed1
		{"over-full a clamped", 2 * skFixed1, skFixed1, 128},

		// Small values: a=2048 (1/32 pixel), b=SK_Fixed1:
		//   a>>11 = 1, b>>11 = 32
		//   area = 1 * 1 * 32 = 32
		//   result = (32 >> 8) & 0xFF = 0
		{"tiny a full b", 2048, skFixed1, 0},

		// a=4096 (1/16 pixel), b=SK_Fixed1:
		//   a>>11 = 2, b>>11 = 32
		//   area = 2 * 2 * 32 = 128
		//   result = (128 >> 8) & 0xFF = 0
		{"small a full b", 4096, skFixed1, 0},

		// a=8192 (1/8 pixel), b=SK_Fixed1:
		//   a>>11 = 4, b>>11 = 32
		//   area = 4 * 4 * 32 = 512
		//   result = (512 >> 8) & 0xFF = 2
		{"eighth a full b", 8192, skFixed1, 2},

		// a=SK_Fixed1*3/4 (49152), b=SK_Fixed1:
		//   a>>11 = 24, b>>11 = 32
		//   area = 24 * 24 * 32 = 18432
		//   result = (18432 >> 8) & 0xFF = 72
		{"three quarter a full b", skFixed1 * 3 / 4, skFixed1, 72},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := partialTriangleToAlpha(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("partialTriangleToAlpha(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// --- 3. getPartialAlpha8 -------------------------------------------------
// Skia: get_partial_alpha(SkAlpha, SkAlpha) (SkScan_AAAPath.cpp:565-567)
//
//   result = (alpha * fullAlpha) >> 8
//   Uses TRUNCATION, not rounding.

func TestFormulaGetPartialAlpha8(t *testing.T) {
	tests := []struct {
		name      string
		alpha     uint8
		fullAlpha uint8
		want      uint8
	}{
		// 255 * 255 = 65025; 65025 >> 8 = 254 (NOT 255 — truncation!)
		{"max times max truncation", 255, 255, 254},

		// 255 * 128 = 32640; 32640 >> 8 = 127
		{"max times half", 255, 128, 127},

		// 128 * 64 = 8192; 8192 >> 8 = 32
		{"half times quarter", 128, 64, 32},

		// 0 * 255 = 0
		{"zero alpha", 0, 255, 0},

		// 255 * 0 = 0
		{"zero full alpha", 255, 0, 0},

		// 0 * 0 = 0
		{"both zero", 0, 0, 0},

		// 1 * 255 = 255; 255 >> 8 = 0
		{"one times max", 1, 255, 0},

		// 255 * 1 = 255; 255 >> 8 = 0
		{"max times one", 255, 1, 0},

		// 128 * 128 = 16384; 16384 >> 8 = 64
		{"half times half", 128, 128, 64},

		// 64 * 64 = 4096; 4096 >> 8 = 16
		{"quarter times quarter", 64, 64, 16},

		// 200 * 100 = 20000; 20000 >> 8 = 78
		{"200 times 100", 200, 100, 78},

		// 100 * 200 = 20000; 20000 >> 8 = 78 (commutative)
		{"100 times 200 commutative", 100, 200, 78},

		// 192 * 255 = 48960; 48960 >> 8 = 191
		{"192 times 255", 192, 255, 191},

		// 2 * 128 = 256; 256 >> 8 = 1
		{"two times half", 2, 128, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPartialAlpha8(tt.alpha, tt.fullAlpha)
			if got != tt.want {
				t.Errorf("getPartialAlpha8(%d, %d) = %d, want %d",
					tt.alpha, tt.fullAlpha, got, tt.want)
			}
		})
	}
}

// --- 4. fixedToAlpha -----------------------------------------------------
// Skia: fixed_to_alpha (SkScan_AAAPath.cpp:572-575)
//
//   get_partial_alpha(0xFF, f) = (255 * f + SK_FixedHalf) >> 16
//   Uses ROUNDING (contrast with getPartialAlpha8 which truncates).

func TestFormulaFixedToAlpha(t *testing.T) {
	tests := []struct {
		name string
		f    int32
		want uint8
	}{
		// f=SK_Fixed1 (65536) → clamped to 255
		// (the function clamps f >= SK_Fixed1 to 255 before computing)
		{"full pixel", skFixed1, 255},

		// f=SK_Fixed1/2 (32768) → (255*32768 + 32768) >> 16 = (8355840 + 32768) >> 16
		// = 8388608 >> 16 = 128
		{"half pixel", skFixed1 / 2, 128},

		// f=SK_Fixed1/4 (16384) → (255*16384 + 32768) >> 16 = (4177920 + 32768) >> 16
		// = 4210688 >> 16 = 64
		{"quarter pixel", skFixed1 / 4, 64},

		// f=0 → 0
		{"zero", 0, 0},

		// f=SK_Fixed1*3/4 (49152) → (255*49152 + 32768) >> 16
		// = (12533760 + 32768) >> 16 = 12566528 >> 16 = 191
		{"three quarter pixel", skFixed1 * 3 / 4, 191},

		// Negative → clamped to 0
		{"negative clamped", -100, 0},
		{"large negative clamped", -skFixed1, 0},

		// Over-full → clamped to 255
		{"over full clamped", 2 * skFixed1, 255},

		// f=1 (smallest positive) → (255*1 + 32768) >> 16 = 33023 >> 16 = 0
		{"epsilon", 1, 0},

		// f=SK_Fixed1/8 (8192) → (255*8192 + 32768) >> 16
		// = (2088960 + 32768) >> 16 = 2121728 >> 16 = 32
		{"eighth pixel", skFixed1 / 8, 32},

		// f=SK_Fixed1*7/8 (57344) → (255*57344 + 32768) >> 16
		// = (14622720 + 32768) >> 16 = 14655488 >> 16 = 223
		{"seven eighths", skFixed1 * 7 / 8, 223},

		// Key property: 4 * fixedToAlpha(SK_Fixed1/4) = 4 * 64 = 256.
		// This exceeds 255 — safeAddAlpha handles the clamping.
		// This is expected behavior, not a bug.
		{"quarter times four overflow", skFixed1 / 4, 64},

		// f=SK_FixedHalf (32768) — same as SK_Fixed1/2
		{"fixed half", skFixedHalf, 128},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixedToAlpha(tt.f)
			if got != tt.want {
				t.Errorf("fixedToAlpha(%d) = %d, want %d", tt.f, got, tt.want)
			}
		})
	}
}

// TestFormulaFixedToAlphaRounding verifies that fixedToAlpha uses rounding
// (not truncation). This is important because getPartialAlpha8 uses truncation,
// and confusing the two would produce off-by-one alpha values.
func TestFormulaFixedToAlphaRounding(t *testing.T) {
	// Rounding test: f = 129 (just over 128 when scaled by 255/65536).
	// Without rounding: (255 * 129) >> 16 = 32895 >> 16 = 0
	// With rounding:    (255 * 129 + 32768) >> 16 = 65663 >> 16 = 1
	f := int32(129)
	got := fixedToAlpha(f)
	wantRounded := uint8(1)
	wantTruncated := uint8(0)

	if got != wantRounded {
		t.Errorf("fixedToAlpha(%d) = %d, want %d (rounded); truncation would give %d",
			f, got, wantRounded, wantTruncated)
	}
}

// --- 5. snapY (in curve_edge.go) -----------------------------------------
// Skia: SnapY (SkAnalyticEdge.h:52)
//
//   Rounds FDot16 Y to nearest 1/4 pixel boundary (with accuracy=2).
//   mask = ^((1 << (16 - 2)) - 1) = ^(0x3FFF - 1) = 0xFFFFC000
//   half = 1 << (16 - 2 - 1) = 1 << 13 = 8192
//   result = (y + 8192) & 0xFFFFC000

func TestFormulaSnapY(t *testing.T) {
	tests := []struct {
		name string
		y    FDot16
		want FDot16
	}{
		// Exact integer pixel: 15.0 → no change
		// 15.0 in FDot16 = 15 * 65536 = 983040
		// (983040 + 8192) & mask = 991232 & 0xFFFFC000 = 983040 (15.0)
		{"exact integer 15", FDot16FromFloat32(15.0), FDot16FromFloat32(15.0) & ^(int32(1)<<(16-2) - 1)},

		// 15.0 → 15.0 (no rounding needed)
		{"exact 15.0", 15 * skFixed1, 15 * skFixed1},

		// 0.0 → 0.0
		{"zero", 0, 0},

		// 1.0 → 1.0 (exact quarter boundary)
		{"one point zero", skFixed1, skFixed1},

		// 0.25 → 0.25 (exact quarter boundary: 0.25 * 65536 = 16384)
		// 16384 = 0x4000 which is on a quarter-pixel boundary
		{"exact quarter", skFixed1 / 4, skFixed1 / 4},

		// 0.5 → 0.5 (exact quarter boundary: 0.5 * 65536 = 32768)
		{"exact half", skFixed1 / 2, skFixed1 / 2},

		// 0.75 → 0.75 (exact quarter boundary: 0.75 * 65536 = 49152)
		{"exact three quarters", skFixed1 * 3 / 4, skFixed1 * 3 / 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := snapY(tt.y)
			if got != tt.want {
				t.Errorf("snapY(%d) = %d, want %d (%.4f → %.4f, expected %.4f)",
					tt.y, got, tt.want,
					float64(tt.y)/65536.0, float64(got)/65536.0, float64(tt.want)/65536.0)
			}
		})
	}
}

// TestFormulaSnapYRoundingDirection tests that values between quarter-pixel
// boundaries round correctly (to nearest quarter pixel).
func TestFormulaSnapYRoundingDirection(t *testing.T) {
	// The mask with accuracy=2:
	//   half = 1 << 13 = 8192
	//   mask = ^(0x3FFF) = 0xFFFFC000
	//
	// Quarter pixel in FDot16 = 65536/4 = 16384 = 0x4000

	tests := []struct {
		name   string
		y      FDot16
		wantFx float64 // expected value as fraction for clarity
	}{
		// y = 0.125 (8192 in FDot16) — exactly half between 0.0 and 0.25
		// (8192 + 8192) = 16384, 16384 & mask = 16384 = 0.25
		{"0.125 rounds to 0.25", 8192, 0.25},

		// y = 0.1 (6553 in FDot16) — below half of quarter-pixel
		// (6553 + 8192) = 14745, 14745 & mask = 0 → 0.0
		{"0.1 rounds to 0.0", 6553, 0.0},

		// y = 0.2 (13107 in FDot16) — above half of quarter-pixel
		// (13107 + 8192) = 21299, 21299 & mask = 16384 → 0.25
		{"0.2 rounds to 0.25", 13107, 0.25},

		// y = 0.3 (19660 in FDot16) — between 0.25 and 0.5
		// (19660 + 8192) = 27852, 27852 & mask = 16384 → 0.25
		{"0.3 rounds to 0.25", 19660, 0.25},

		// y = 0.4 (26214 in FDot16)
		// (26214 + 8192) = 34406, 34406 & mask = 32768 → 0.5
		{"0.4 rounds to 0.5", 26214, 0.5},

		// y = 0.6 (39321 in FDot16)
		// (39321 + 8192) = 47513, 47513 & mask = 32768 → 0.5
		{"0.6 rounds to 0.5", 39321, 0.5},

		// y = 0.9 (58982 in FDot16)
		// (58982 + 8192) = 67174, 67174 & mask = 65536 → 1.0
		{"0.9 rounds to 1.0", 58982, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := snapY(tt.y)
			wantFixed := int32(tt.wantFx * 65536.0)
			if got != wantFixed {
				t.Errorf("snapY(%d [%.4f]) = %d [%.4f], want %d [%.4f]",
					tt.y, float64(tt.y)/65536.0,
					got, float64(got)/65536.0,
					wantFixed, tt.wantFx)
			}
		})
	}
}

// --- 6. safeAddAlpha -----------------------------------------------------
// Additive alpha with clamping to 255 and bounds checking.

func TestFormulaSafeAddAlpha(t *testing.T) {
	tests := []struct {
		name     string
		initial  uint8
		alpha    uint8
		want     uint8
		x        int32 // pixel position
		width    int   // canvas width
		expectOp bool  // whether the operation should take effect
	}{
		// Normal add: 100 + 100 = 200
		{"normal add", 100, 100, 200, 5, 10, true},

		// Clamped: 200 + 100 = 300 → clamped to 255
		{"clamped overflow", 200, 100, 255, 5, 10, true},

		// Zero add: no change
		{"zero alpha no-op", 50, 0, 50, 5, 10, false},

		// Add to zero
		{"add to zero", 0, 50, 50, 5, 10, true},

		// Max + max → 255
		{"max plus max", 255, 255, 255, 5, 10, true},

		// Add 1 to 254 → 255
		{"just reaches max", 254, 1, 255, 5, 10, true},

		// Add 1 to 255 → still 255 (clamped)
		{"add to already max", 255, 1, 255, 5, 10, true},

		// Out of bounds: negative x → no change
		{"negative x no-op", 50, 100, 50, -1, 10, false},

		// Out of bounds: x >= width → no change
		{"x at width no-op", 50, 100, 50, 10, 10, false},
		{"x past width no-op", 50, 100, 50, 15, 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			af := NewAnalyticFiller(tt.width, 1)

			// Set initial value
			if tt.x >= 0 && int(tt.x) < tt.width {
				af.coverage[tt.x] = tt.initial
			}

			af.safeAddAlpha(tt.x, tt.alpha)

			// Check result
			if tt.x >= 0 && int(tt.x) < tt.width {
				got := af.coverage[tt.x]
				if got != tt.want {
					t.Errorf("safeAddAlpha(x=%d, alpha=%d) with initial=%d: got %d, want %d",
						tt.x, tt.alpha, tt.initial, got, tt.want)
				}
			}
		})
	}
}

// --- 7. approximateIntersection ------------------------------------------
// Skia: approximate_intersection (SkScan_AAAPath.cpp:539-545)
//
//   Approximates the X of intersection between two lines defined by:
//   line1: (l1, y) → (r1, y+1) and line2: (l2, y) → (r2, y+1).
//   result = (max(l1, l2) + min(r1, r2)) / 2

func TestFormulaApproximateIntersection(t *testing.T) {
	tests := []struct {
		name   string
		l1, r1 int32
		l2, r2 int32
		want   int32
	}{
		// Symmetric: lines from (0,y)→(100,y+1) and (100,y)→(0,y+1)
		// After normalization: l1=0,r1=100 and l2=0,r2=100
		// max(0,0) = 0, min(100,100) = 100, result = 50
		{"symmetric crossing", 0, 100, 100, 0, 50},

		// Same line: l1=l2=10, r1=r2=50
		// max(10,10) = 10, min(50,50) = 50, result = 30
		{"identical lines", 10, 50, 10, 50, 30},

		// Non-overlapping (conceptually): l1=0,r1=10 and l2=20,r2=30
		// max(0,20) = 20, min(10,30) = 10, result = (20+10)/2 = 15
		{"non overlapping", 0, 10, 20, 30, 15},

		// Reversed l > r: l1=100,r1=0 → normalized to l1=0,r1=100
		{"reversed first", 100, 0, 50, 50, 50},

		// Zero point: all zeros
		{"all zeros", 0, 0, 0, 0, 0},

		// Negative values
		{"negative values", -100, 100, 100, -100, 0},

		// SkFixed values (typical usage)
		{"fixed point values", skFixed1, skFixed1 * 3, skFixed1 * 2, skFixed1 * 4, skFixed1 * 5 / 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := approximateIntersection(tt.l1, tt.r1, tt.l2, tt.r2)
			if got != tt.want {
				t.Errorf("approximateIntersection(%d, %d, %d, %d) = %d, want %d",
					tt.l1, tt.r1, tt.l2, tt.r2, got, tt.want)
			}
		})
	}
}

// --- 8. SkFixed helper functions -----------------------------------------
// Skia: various SkFixed helpers

func TestFormulaIntToSkFixed(t *testing.T) {
	tests := []struct {
		name string
		n    int32
		want int32
	}{
		{"zero", 0, 0},
		{"one", 1, skFixed1},
		{"ten", 10, 10 * skFixed1},
		{"negative", -5, -5 * skFixed1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := intToSkFixed(tt.n)
			if got != tt.want {
				t.Errorf("intToSkFixed(%d) = %d, want %d", tt.n, got, tt.want)
			}
		})
	}
}

func TestFormulaSkFixedFloorToInt(t *testing.T) {
	tests := []struct {
		name string
		v    int32
		want int32
	}{
		{"zero", 0, 0},
		{"exact one", skFixed1, 1},
		{"one point five", skFixed1 + skFixedHalf, 1},
		{"just under one", skFixed1 - 1, 0},
		{"negative half", -skFixedHalf, -1}, // floor(-0.5) = -1
		{"negative one", -skFixed1, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skFixedFloorToInt(tt.v)
			if got != tt.want {
				t.Errorf("skFixedFloorToInt(%d) = %d, want %d", tt.v, got, tt.want)
			}
		})
	}
}

func TestFormulaSkFixedCeilToInt(t *testing.T) {
	tests := []struct {
		name string
		v    int32
		want int32
	}{
		{"zero", 0, 0},
		{"exact one", skFixed1, 1},
		{"epsilon above zero", 1, 1},
		{"just under one", skFixed1 - 1, 1},
		{"one point five", skFixed1 + skFixedHalf, 2},
		{"negative one", -skFixed1, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skFixedCeilToInt(tt.v)
			if got != tt.want {
				t.Errorf("skFixedCeilToInt(%d) = %d, want %d", tt.v, got, tt.want)
			}
		})
	}
}

func TestFormulaSkFixedFloorToFixed(t *testing.T) {
	tests := []struct {
		name string
		v    int32
		want int32
	}{
		{"zero", 0, 0},
		{"exact one", skFixed1, skFixed1},
		// 1.5 in SkFixed = 98304; floor = 65536 (1.0)
		{"one point five", skFixed1 + skFixedHalf, skFixed1},
		// 0.5 in SkFixed = 32768; floor = 0
		{"half", skFixedHalf, 0},
		// -0.5 in SkFixed = -32768; floor = -65536 (-1.0)
		{"negative half", -skFixedHalf, -skFixed1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skFixedFloorToFixed(tt.v)
			if got != tt.want {
				t.Errorf("skFixedFloorToFixed(%d) = %d, want %d", tt.v, got, tt.want)
			}
		})
	}
}

func TestFormulaSkFixedCeilToFixed(t *testing.T) {
	tests := []struct {
		name string
		v    int32
		want int32
	}{
		{"zero", 0, 0},
		{"exact one", skFixed1, skFixed1},
		// epsilon (1): ceil = 65536 (1.0)
		{"epsilon", 1, skFixed1},
		// 0.5 in SkFixed: ceil = 65536 (1.0)
		{"half", skFixedHalf, skFixed1},
		// 1.5 in SkFixed: ceil = 131072 (2.0)
		{"one point five", skFixed1 + skFixedHalf, 2 * skFixed1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skFixedCeilToFixed(tt.v)
			if got != tt.want {
				t.Errorf("skFixedCeilToFixed(%d) = %d, want %d", tt.v, got, tt.want)
			}
		})
	}
}

func TestFormulaSkFixedMul(t *testing.T) {
	tests := []struct {
		name string
		a, b int32
		want int32
	}{
		// 1.0 * 1.0 = 1.0
		{"one times one", skFixed1, skFixed1, skFixed1},
		// 0.5 * 0.5 = 0.25
		{"half times half", skFixedHalf, skFixedHalf, skFixed1 / 4},
		// 2.0 * 3.0 = 6.0
		{"two times three", 2 * skFixed1, 3 * skFixed1, 6 * skFixed1},
		// 0 * anything = 0
		{"zero times anything", 0, skFixed1, 0},
		// -1.0 * 1.0 = -1.0
		{"negative one", -skFixed1, skFixed1, -skFixed1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skFixedMul(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("skFixedMul(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// --- 9. saturatingSub8 ---------------------------------------------------

func TestFormulaSaturatingSub8(t *testing.T) {
	tests := []struct {
		name string
		a, b uint8
		want uint8
	}{
		{"normal sub", 200, 100, 100},
		{"sub to zero", 100, 100, 0},
		{"sub underflow", 50, 100, 0},
		{"zero minus anything", 0, 100, 0},
		{"max minus zero", 255, 0, 255},
		{"max minus one", 255, 1, 254},
		{"one minus max", 1, 255, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := saturatingSub8(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("saturatingSub8(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// --- 10. clampAlpha32 ----------------------------------------------------

func TestFormulaClampAlpha32(t *testing.T) {
	tests := []struct {
		name string
		v    int32
		want int32
	}{
		{"in range", 128, 128},
		{"zero", 0, 0},
		{"max", 255, 255},
		{"over max", 300, 255},
		{"negative", -10, 0},
		{"large negative", -1000, 0},
		{"large positive", 10000, 255},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampAlpha32(tt.v)
			if got != tt.want {
				t.Errorf("clampAlpha32(%d) = %d, want %d", tt.v, got, tt.want)
			}
		})
	}
}

// --- 11. sk32SatAdd ------------------------------------------------------
// Saturating 32-bit add (clamped to int32 range).

func TestFormulaSk32SatAdd(t *testing.T) {
	tests := []struct {
		name string
		a, b int32
		want int32
	}{
		{"normal add", 100, 200, 300},
		{"negative add", -100, -200, -300},
		{"overflow clamp", 0x7FFFFFFF, 1, 0x7FFFFFFF},
		{"underflow clamp", -0x7FFFFFFF, -2, -0x80000000},
		{"zero", 0, 0, 0},
		{"max plus zero", 0x7FFFFFFF, 0, 0x7FFFFFFF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sk32SatAdd(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("sk32SatAdd(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// --- 12. computeEdgeDY ---------------------------------------------------
// Skia: fDY = abs(1/slope) in SkFixed. Used by partialTriangleToAlpha.

func TestFormulaComputeEdgeDY(t *testing.T) {
	tests := []struct {
		name  string
		slope int32
		want  int32
	}{
		// Vertical line: slope=0 → fDY = maxint (infinite 1/slope)
		{"vertical slope zero", 0, 0x7FFFFFFF},

		// 45-degree slope: slope=FDot6One (64 in FDot6, which is 1.0).
		// In our representation, slope is from FDot6Div (FDot16 result).
		// slope = 1.0 in FDot16 = 65536. absSlope = 65536.
		// absSlopeFDot6 = 65536 >> (16 - 6) = 65536 >> 10 = 64
		// fDY = FDot6Div(64, 64) = (64 << 16) / 64 = 65536 = SK_Fixed1
		{"45 degree slope 1.0", skFixed1, skFixed1},

		// Negative slope: same result (abs applied)
		{"negative 45 degree", -skFixed1, skFixed1},

		// slope = 2.0 in FDot16 = 131072
		// absSlopeFDot6 = 131072 >> 10 = 128
		// fDY = FDot6Div(64, 128) = (64 << 16) / 128 = 32768 = SK_FixedHalf
		{"slope 2.0", 2 * skFixed1, skFixedHalf},

		// slope = 0.5 in FDot16 = 32768
		// absSlopeFDot6 = 32768 >> 10 = 32
		// fDY = FDot6Div(64, 32) = (64 << 16) / 32 = 131072 = 2*SK_Fixed1
		{"slope 0.5", skFixedHalf, 2 * skFixed1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeEdgeDY(tt.slope)
			if got != tt.want {
				t.Errorf("computeEdgeDY(%d) = %d, want %d", tt.slope, got, tt.want)
			}
		})
	}
}

// --- 13. sortInt32s and deduplicateInt32s ---------------------------------

func TestFormulaSortInt32s(t *testing.T) {
	tests := []struct {
		name  string
		input []int32
		want  []int32
	}{
		{"already sorted", []int32{1, 2, 3, 4, 5}, []int32{1, 2, 3, 4, 5}},
		{"reversed", []int32{5, 4, 3, 2, 1}, []int32{1, 2, 3, 4, 5}},
		{"single", []int32{42}, []int32{42}},
		{"empty", []int32{}, []int32{}},
		{"duplicates", []int32{3, 1, 4, 1, 5}, []int32{1, 1, 3, 4, 5}},
		{"negative", []int32{-3, 1, -1, 2}, []int32{-3, -1, 1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := make([]int32, len(tt.input))
			copy(s, tt.input)
			sortInt32s(s)
			if len(s) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(s), len(tt.want))
			}
			for i := range s {
				if s[i] != tt.want[i] {
					t.Errorf("index %d: got %d, want %d (full: %v)", i, s[i], tt.want[i], s)
					break
				}
			}
		})
	}
}

func TestFormulaDeduplicateInt32s(t *testing.T) {
	// deduplicateInt32s keeps values that differ by MORE than eps (128) from
	// the last kept value. s[i] - s[n-1] > eps means 128 is NOT enough —
	// need > 128 to be considered distinct.
	tests := []struct {
		name  string
		input []int32
		want  []int32
	}{
		// Values spaced 1000 apart — well beyond eps=128, all kept.
		{"well spaced", []int32{1000, 2000, 3000}, []int32{1000, 2000, 3000}},
		// Exact duplicates → removed.
		{"exact dupes", []int32{1000, 1000, 2000}, []int32{1000, 2000}},
		// 50 apart: within eps → treated as duplicate.
		{"within eps 50", []int32{1000, 1050}, []int32{1000}},
		// 128 apart: s[i]-s[n-1] = 128, NOT > 128 → still duplicate.
		{"at eps boundary 128", []int32{1000, 1128}, []int32{1000}},
		// 129 apart: > 128 → kept as distinct.
		{"beyond eps 129", []int32{1000, 1129}, []int32{1000, 1129}},
		// Chain: 1000, 1050, 1200. After keeping 1000, 1050 is within eps (50 ≤ 128).
		// Then 1200 - 1000 = 200 > 128 → kept.
		{"chain dedup", []int32{1000, 1050, 1200}, []int32{1000, 1200}},
		{"single", []int32{42}, []int32{42}},
		{"empty", []int32{}, []int32{}},
		// Typical SkFixed values: pixel boundaries 0, 65536, 131072 — far apart.
		{"skfixed pixels", []int32{0, skFixed1, 2 * skFixed1}, []int32{0, skFixed1, 2 * skFixed1}},
		// Two sub-pixel values within same pixel, 100 apart → deduped.
		{"sub pixel dedup", []int32{skFixed1, skFixed1 + 100}, []int32{skFixed1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := make([]int32, len(tt.input))
			copy(s, tt.input)
			got := deduplicateInt32s(s)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (got %v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %d, want %d (full: %v)", i, got[i], tt.want[i], got)
					break
				}
			}
		})
	}
}

// --- 14. Formula interaction tests ---------------------------------------
// Test that formulas work together correctly, matching Skia's pipeline.

func TestFormulaInteractionTrapezoidWithPartialAlpha(t *testing.T) {
	// Skia pattern: for 1-pixel wide edges, trapezoidToAlpha gives the
	// coverage, then getPartialAlpha8 scales by fullAlpha (strip height).

	// Full-height strip (fullAlpha = 255): result ≈ trapezoid value
	trap := trapezoidToAlpha(skFixed1/2, skFixed1/2) // 128
	got := getPartialAlpha8(trap, 255)               // (128*255)>>8 = 127
	if got != 127 {
		t.Errorf("trapezoid 128 scaled by 255: got %d, want 127", got)
	}

	// Half-height strip (fullAlpha = 128): result ≈ half the trapezoid value
	got2 := getPartialAlpha8(trap, 128) // (128*128)>>8 = 64
	if got2 != 64 {
		t.Errorf("trapezoid 128 scaled by 128: got %d, want 64", got2)
	}
}

func TestFormulaInteractionFixedToAlphaSubStrips(t *testing.T) {
	// Skia splits pixel rows into sub-strips. The sum of sub-strip alphas
	// for a full pixel row should equal 255 (or close to it).

	// Four quarter-pixel sub-strips:
	quarter := skFixed1 / 4 // 16384
	a1 := fixedToAlpha(quarter)
	a2 := fixedToAlpha(quarter)
	a3 := fixedToAlpha(quarter)
	a4 := fixedToAlpha(quarter)
	sum := int(a1) + int(a2) + int(a3) + int(a4) // 64*4 = 256

	// Sum is 256 which exceeds 255 — this is expected due to rounding.
	// Each quarter gives 64 (rounded), 4*64=256 > 255. Skia handles this
	// with clamping in safeAddAlpha. The point is that individual values
	// are correct.
	if a1 != 64 {
		t.Errorf("fixedToAlpha(quarter) = %d, want 64", a1)
	}
	// Sum can be up to 256 — this is by design (Skia clamps in safeAddAlpha).
	if sum < 255 || sum > 256 {
		t.Errorf("4 * fixedToAlpha(quarter) = %d, want 255 or 256", sum)
	}

	// Two half-pixel sub-strips:
	half := skFixed1 / 2
	h1 := fixedToAlpha(half)
	h2 := fixedToAlpha(half)
	sumH := int(h1) + int(h2)
	if h1 != 128 {
		t.Errorf("fixedToAlpha(half) = %d, want 128", h1)
	}
	if sumH != 256 {
		t.Errorf("2 * fixedToAlpha(half) = %d, want 256", sumH)
	}
}

// TestFormulaCommutativity verifies that commutative formulas produce
// the same result regardless of argument order.
func TestFormulaCommutativity(t *testing.T) {
	// trapezoidToAlpha(l1, l2) == trapezoidToAlpha(l2, l1)
	values := []int32{0, 100, 256, skFixed1 / 4, skFixed1 / 2, skFixed1}
	for _, l1 := range values {
		for _, l2 := range values {
			a := trapezoidToAlpha(l1, l2)
			b := trapezoidToAlpha(l2, l1)
			if a != b {
				t.Errorf("trapezoidToAlpha not commutative: (%d,%d)=%d vs (%d,%d)=%d",
					l1, l2, a, l2, l1, b)
			}
		}
	}

	// getPartialAlpha8(a, b) == getPartialAlpha8(b, a)
	alphas := []uint8{0, 1, 64, 128, 192, 255}
	for _, a := range alphas {
		for _, b := range alphas {
			x := getPartialAlpha8(a, b)
			y := getPartialAlpha8(b, a)
			if x != y {
				t.Errorf("getPartialAlpha8 not commutative: (%d,%d)=%d vs (%d,%d)=%d",
					a, b, x, b, a, y)
			}
		}
	}
}

// TestFormulaBoundaryValues tests all formulas with boundary and extreme values
// to ensure no panics or unexpected overflow.
func TestFormulaBoundaryValues(t *testing.T) {
	// trapezoidToAlpha with extreme values
	t.Run("trapezoid extremes", func(t *testing.T) {
		_ = trapezoidToAlpha(0x7FFFFFFF, 0x7FFFFFFF) // should not panic
		_ = trapezoidToAlpha(-0x7FFFFFFF, -0x7FFFFFFF)
		_ = trapezoidToAlpha(0x7FFFFFFF, -0x7FFFFFFF)
	})

	// partialTriangleToAlpha with extreme values
	t.Run("partial triangle extremes", func(t *testing.T) {
		_ = partialTriangleToAlpha(0x7FFFFFFF, 0x7FFFFFFF)
		_ = partialTriangleToAlpha(-0x7FFFFFFF, -0x7FFFFFFF)
		_ = partialTriangleToAlpha(0, 0x7FFFFFFF)
	})

	// fixedToAlpha with extreme values
	t.Run("fixedToAlpha extremes", func(t *testing.T) {
		got := fixedToAlpha(0x7FFFFFFF)
		if got != 255 {
			t.Errorf("fixedToAlpha(maxint) = %d, want 255", got)
		}
		got = fixedToAlpha(-0x7FFFFFFF)
		if got != 0 {
			t.Errorf("fixedToAlpha(minint) = %d, want 0", got)
		}
	})

	// approximateIntersection with extreme values
	t.Run("approx intersection extremes", func(t *testing.T) {
		_ = approximateIntersection(0x7FFFFFFF, -0x7FFFFFFF, -0x7FFFFFFF, 0x7FFFFFFF)
	})

	// computeEdgeDY with extreme values
	t.Run("computeEdgeDY extremes", func(t *testing.T) {
		got := computeEdgeDY(0x7FFFFFFF)
		if got <= 0 {
			t.Errorf("computeEdgeDY(maxint) = %d, want > 0", got)
		}
	})
}
