package blend

import "testing"

// TestMulDiv255 tests the multiply and divide by 255 helper function.
func TestMulDiv255(t *testing.T) {
	tests := []struct {
		name string
		a, b byte
		want byte
	}{
		{"zero * zero", 0, 0, 0},
		{"zero * max", 0, 255, 0},
		{"max * zero", 255, 0, 0},
		{"max * max", 255, 255, 255},
		{"half * half", 128, 128, 64},
		{"255 * 128", 255, 128, 128},
		{"128 * 255", 128, 255, 128},
		{"1 * 1", 1, 1, 0},    // Rounds down
		{"10 * 10", 10, 10, 0}, // 100/255 = 0.39 -> 0
		{"100 * 100", 100, 100, 39},
		{"200 * 200", 200, 200, 157},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mulDiv255(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("mulDiv255(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestAddDiv255 tests the add with clamping helper function.
func TestAddDiv255(t *testing.T) {
	tests := []struct {
		name string
		a, b byte
		want byte
	}{
		{"zero + zero", 0, 0, 0},
		{"zero + max", 0, 255, 255},
		{"max + zero", 255, 0, 255},
		{"max + max (clamped)", 255, 255, 255},
		{"128 + 128 (clamped)", 128, 128, 255},
		{"100 + 100", 100, 100, 200},
		{"200 + 100 (clamped)", 200, 100, 255},
		{"1 + 1", 1, 1, 2},
		{"127 + 128", 127, 128, 255},
		{"50 + 60", 50, 60, 110},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addDiv255(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("addDiv255(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestMinByte tests the minimum byte helper function.
func TestMinByte(t *testing.T) {
	tests := []struct {
		name string
		a, b byte
		want byte
	}{
		{"0 vs 0", 0, 0, 0},
		{"0 vs 255", 0, 255, 0},
		{"255 vs 0", 255, 0, 0},
		{"128 vs 128", 128, 128, 128},
		{"100 vs 200", 100, 200, 100},
		{"200 vs 100", 200, 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := minByte(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("minByte(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestBlendClear tests the Clear blend mode.
func TestBlendClear(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
	}{
		{"opaque red over opaque blue", 255, 0, 0, 255, 0, 0, 255, 255},
		{"transparent over opaque", 0, 0, 0, 0, 255, 255, 255, 255},
		{"opaque over transparent", 255, 255, 255, 255, 0, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendClear(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != 0 || g != 0 || b != 0 || a != 0 {
				t.Errorf("blendClear() = (%d, %d, %d, %d), want (0, 0, 0, 0)", r, g, b, a)
			}
		})
	}
}

// TestBlendSource tests the Source blend mode.
func TestBlendSource(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
	}{
		{"opaque red over opaque blue", 255, 0, 0, 255, 0, 0, 255, 255},
		{"transparent over opaque", 0, 0, 0, 0, 255, 255, 255, 255},
		{"opaque over transparent", 255, 255, 255, 255, 0, 0, 0, 0},
		{"half-transparent green", 0, 128, 0, 128, 100, 100, 100, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendSource(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.sr || g != tt.sg || b != tt.sb || a != tt.sa {
				t.Errorf("blendSource() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.sr, tt.sg, tt.sb, tt.sa)
			}
		})
	}
}

// TestBlendDestination tests the Destination blend mode.
func TestBlendDestination(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
	}{
		{"opaque red over opaque blue", 255, 0, 0, 255, 0, 0, 255, 255},
		{"transparent over opaque", 0, 0, 0, 0, 255, 255, 255, 255},
		{"opaque over transparent", 255, 255, 255, 255, 0, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendDestination(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.dr || g != tt.dg || b != tt.db || a != tt.da {
				t.Errorf("blendDestination() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.dr, tt.dg, tt.db, tt.da)
			}
		})
	}
}

// TestBlendSourceOver tests the SourceOver blend mode (default).
func TestBlendSourceOver(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"opaque red over opaque blue",
			255, 0, 0, 255, // red source
			0, 0, 255, 255, // blue dest
			255, 0, 0, 255, // expect red (opaque source replaces)
		},
		{
			"transparent over opaque",
			0, 0, 0, 0, // transparent source
			255, 255, 255, 255, // white dest
			255, 255, 255, 255, // expect white (unchanged)
		},
		{
			"opaque over transparent",
			255, 255, 255, 255, // white source
			0, 0, 0, 0, // transparent dest
			255, 255, 255, 255, // expect white
		},
		{
			"half-transparent gray over white",
			128, 128, 128, 128, // 50% gray (premul: 128 = 255*0.5)
			255, 255, 255, 255, // white dest
			255, 255, 255, 255, // expect blend
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendSourceOver(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendSourceOver() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendDestinationOver tests the DestinationOver blend mode.
func TestBlendDestinationOver(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"opaque red over opaque blue",
			255, 0, 0, 255, // red source
			0, 0, 255, 255, // blue dest
			0, 0, 255, 255, // expect blue (opaque dest on top)
		},
		{
			"transparent source, opaque dest",
			0, 0, 0, 0, // transparent source
			255, 255, 255, 255, // white dest
			255, 255, 255, 255, // expect white
		},
		{
			"opaque source, transparent dest",
			255, 255, 255, 255, // white source
			0, 0, 0, 0, // transparent dest
			255, 255, 255, 255, // expect white
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendDestinationOver(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendDestinationOver() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendSourceIn tests the SourceIn blend mode.
func TestBlendSourceIn(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"opaque red, opaque blue",
			255, 0, 0, 255,
			0, 0, 255, 255,
			255, 0, 0, 255, // source * dest_alpha = source
		},
		{
			"opaque source, transparent dest",
			255, 255, 255, 255,
			0, 0, 0, 0,
			0, 0, 0, 0, // source * 0 = transparent
		},
		{
			"source, half-transparent dest",
			255, 0, 0, 255,
			0, 0, 0, 128,
			128, 0, 0, 128, // source * 0.5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendSourceIn(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendSourceIn() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendDestinationIn tests the DestinationIn blend mode.
func TestBlendDestinationIn(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"opaque red, opaque blue",
			255, 0, 0, 255,
			0, 0, 255, 255,
			0, 0, 255, 255, // dest * source_alpha = dest
		},
		{
			"transparent source, opaque dest",
			0, 0, 0, 0,
			255, 255, 255, 255,
			0, 0, 0, 0, // dest * 0 = transparent
		},
		{
			"half-transparent source, opaque dest",
			0, 0, 0, 128,
			0, 0, 255, 255,
			0, 0, 128, 128, // dest * 0.5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendDestinationIn(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendDestinationIn() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendSourceOut tests the SourceOut blend mode.
func TestBlendSourceOut(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"opaque source, opaque dest",
			255, 0, 0, 255,
			0, 0, 255, 255,
			0, 0, 0, 0, // source * (1 - dest_alpha) = 0
		},
		{
			"opaque source, transparent dest",
			255, 255, 255, 255,
			0, 0, 0, 0,
			255, 255, 255, 255, // source * 1 = source
		},
		{
			"opaque source, half-transparent dest",
			255, 0, 0, 255,
			0, 0, 0, 128,
			127, 0, 0, 127, // source * 0.5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendSourceOut(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendSourceOut() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendDestinationOut tests the DestinationOut blend mode.
func TestBlendDestinationOut(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"opaque source, opaque dest",
			255, 0, 0, 255,
			0, 0, 255, 255,
			0, 0, 0, 0, // dest * (1 - source_alpha) = 0
		},
		{
			"transparent source, opaque dest",
			0, 0, 0, 0,
			255, 255, 255, 255,
			255, 255, 255, 255, // dest * 1 = dest
		},
		{
			"half-transparent source, opaque dest",
			0, 0, 0, 128,
			0, 0, 255, 255,
			0, 0, 127, 127, // dest * 0.5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendDestinationOut(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendDestinationOut() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendSourceAtop tests the SourceAtop blend mode.
func TestBlendSourceAtop(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"opaque red over opaque blue",
			255, 0, 0, 255,
			0, 0, 255, 255,
			255, 0, 0, 255, // red on top, alpha = dest_alpha
		},
		{
			"opaque source, transparent dest",
			255, 255, 255, 255,
			0, 0, 0, 0,
			0, 0, 0, 0, // alpha = dest_alpha = 0
		},
		{
			"opaque source, half-transparent dest",
			255, 0, 0, 255,
			0, 0, 255, 128,
			128, 0, 0, 128, // source * dest_alpha, alpha = dest_alpha
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendSourceAtop(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendSourceAtop() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendDestinationAtop tests the DestinationAtop blend mode.
func TestBlendDestinationAtop(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"opaque red source, opaque blue dest",
			255, 0, 0, 255,
			0, 0, 255, 255,
			0, 0, 255, 255, // dest on top, alpha = source_alpha
		},
		{
			"transparent source, opaque dest",
			0, 0, 0, 0,
			255, 255, 255, 255,
			0, 0, 0, 0, // alpha = source_alpha = 0
		},
		{
			"half-transparent source, opaque dest",
			0, 0, 0, 128,
			0, 0, 255, 255,
			0, 0, 128, 128, // dest * source_alpha, alpha = source_alpha
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendDestinationAtop(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendDestinationAtop() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendXor tests the Xor blend mode.
func TestBlendXor(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"opaque source, opaque dest",
			255, 0, 0, 255,
			0, 0, 255, 255,
			0, 0, 0, 0, // both opaque = fully cancel
		},
		{
			"opaque source, transparent dest",
			255, 255, 255, 255,
			0, 0, 0, 0,
			255, 255, 255, 255, // source only
		},
		{
			"transparent source, opaque dest",
			0, 0, 0, 0,
			255, 255, 255, 255,
			255, 255, 255, 255, // dest only
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendXor(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendXor() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendPlus tests the Plus blend mode.
func TestBlendPlus(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"100 + 100",
			100, 100, 100, 100,
			100, 100, 100, 100,
			200, 200, 200, 200,
		},
		{
			"200 + 100 (clamped)",
			200, 200, 200, 200,
			100, 100, 100, 100,
			255, 255, 255, 255,
		},
		{
			"255 + 255 (clamped)",
			255, 255, 255, 255,
			255, 255, 255, 255,
			255, 255, 255, 255,
		},
		{
			"0 + 0",
			0, 0, 0, 0,
			0, 0, 0, 0,
			0, 0, 0, 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendPlus(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendPlus() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestBlendModulate tests the Modulate blend mode.
func TestBlendModulate(t *testing.T) {
	tests := []struct {
		name           string
		sr, sg, sb, sa byte
		dr, dg, db, da byte
		wr, wg, wb, wa byte
	}{
		{
			"255 * 255",
			255, 255, 255, 255,
			255, 255, 255, 255,
			255, 255, 255, 255,
		},
		{
			"255 * 128",
			255, 255, 255, 255,
			128, 128, 128, 128,
			128, 128, 128, 128,
		},
		{
			"128 * 128",
			128, 128, 128, 128,
			128, 128, 128, 128,
			64, 64, 64, 64,
		},
		{
			"anything * 0",
			255, 255, 255, 255,
			0, 0, 0, 0,
			0, 0, 0, 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, a := blendModulate(tt.sr, tt.sg, tt.sb, tt.sa, tt.dr, tt.dg, tt.db, tt.da)
			if r != tt.wr || g != tt.wg || b != tt.wb || a != tt.wa {
				t.Errorf("blendModulate() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					r, g, b, a, tt.wr, tt.wg, tt.wb, tt.wa)
			}
		})
	}
}

// TestGetBlendFunc tests that GetBlendFunc returns correct functions.
func TestGetBlendFunc(t *testing.T) {
	modes := []struct {
		mode BlendMode
		name string
	}{
		{BlendClear, "Clear"},
		{BlendSource, "Source"},
		{BlendDestination, "Destination"},
		{BlendSourceOver, "SourceOver"},
		{BlendDestinationOver, "DestinationOver"},
		{BlendSourceIn, "SourceIn"},
		{BlendDestinationIn, "DestinationIn"},
		{BlendSourceOut, "SourceOut"},
		{BlendDestinationOut, "DestinationOut"},
		{BlendSourceAtop, "SourceAtop"},
		{BlendDestinationAtop, "DestinationAtop"},
		{BlendXor, "Xor"},
		{BlendPlus, "Plus"},
		{BlendModulate, "Modulate"},
	}

	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			fn := GetBlendFunc(m.mode)
			if fn == nil {
				t.Errorf("GetBlendFunc(%d) returned nil", m.mode)
			}
		})
	}

	// Test unknown mode returns default
	t.Run("unknown mode", func(t *testing.T) {
		fn := GetBlendFunc(BlendMode(255))
		if fn == nil {
			t.Error("GetBlendFunc(unknown) returned nil, expected default")
		}
		// Verify it's the default (SourceOver) by testing behavior
		r, g, b, a := fn(255, 0, 0, 255, 0, 0, 0, 0)
		if r != 255 || g != 0 || b != 0 || a != 255 {
			t.Errorf("Unknown mode didn't return SourceOver behavior")
		}
	})
}

// TestBlendModeConstants verifies blend mode constant values.
func TestBlendModeConstants(t *testing.T) {
	tests := []struct {
		mode BlendMode
		want uint8
	}{
		{BlendClear, 0},
		{BlendSource, 1},
		{BlendDestination, 2},
		{BlendSourceOver, 3},
		{BlendDestinationOver, 4},
		{BlendSourceIn, 5},
		{BlendDestinationIn, 6},
		{BlendSourceOut, 7},
		{BlendDestinationOut, 8},
		{BlendSourceAtop, 9},
		{BlendDestinationAtop, 10},
		{BlendXor, 11},
		{BlendPlus, 12},
		{BlendModulate, 13},
	}

	for _, tt := range tests {
		if uint8(tt.mode) != tt.want {
			t.Errorf("BlendMode constant mismatch: got %d, want %d", uint8(tt.mode), tt.want)
		}
	}
}

// BenchmarkBlendSourceOver benchmarks the most common blend mode.
func BenchmarkBlendSourceOver(b *testing.B) {
	var r, g, b2, a byte
	for i := 0; i < b.N; i++ {
		r, g, b2, a = blendSourceOver(200, 100, 50, 200, 50, 100, 200, 150)
	}
	_ = r
	_ = g
	_ = b2
	_ = a
}

// BenchmarkGetBlendFunc benchmarks function lookup.
func BenchmarkGetBlendFunc(b *testing.B) {
	var fn BlendFunc
	for i := 0; i < b.N; i++ {
		fn = GetBlendFunc(BlendSourceOver)
	}
	_ = fn
}
