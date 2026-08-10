package gg

import (
	"math"
	"testing"
)

func TestClipRectRotated45PreservesShape(t *testing.T) {
	dc := NewContext(200, 200)
	dc.ClearWithColor(White)
	dc.Translate(100, 100)
	dc.Rotate(math.Pi / 4)
	dc.Translate(-100, -100)
	dc.ClipRect(60, 60, 80, 80)
	if dc.clipStack.IsRectOnly() {
		t.Fatal("rotated ClipRect should use an exact path clip")
	}

	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 200, 200)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill() failed: %v", err)
	}

	// The center is inside the rotated square.
	assertPixelRed(t, dc.pixmap.GetPixel(100, 100), "center")
	// A point on the top edge of the axis-aligned bounding box is outside the
	// rotated square and must remain the white background.
	assertPixelWhite(t, dc.pixmap.GetPixel(100, 40), "outside rotated square")
}

func TestClipRoundRectRotated45PreservesShape(t *testing.T) {
	dc := NewContext(200, 200)
	dc.ClearWithColor(White)
	dc.Translate(100, 100)
	dc.Rotate(math.Pi / 4)
	dc.Translate(-100, -100)
	dc.ClipRoundRect(60, 60, 80, 80, 12)
	if dc.clipStack.IsRRectOnly() {
		t.Fatal("rotated ClipRoundRect should use an exact path clip")
	}

	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 200, 200)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill() failed: %v", err)
	}

	assertPixelRed(t, dc.pixmap.GetPixel(100, 100), "center")
	assertPixelWhite(t, dc.pixmap.GetPixel(100, 40), "outside rotated rounded square")
}

func TestClipRectShearedPreservesShape(t *testing.T) {
	dc := NewContext(200, 200)
	dc.ClearWithColor(White)
	dc.Translate(100, 100)
	dc.Shear(0.5, 0)
	dc.Translate(-100, -100)
	dc.ClipRect(60, 60, 80, 80)

	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 200, 200)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill() failed: %v", err)
	}

	assertPixelRed(t, dc.pixmap.GetPixel(100, 100), "sheared clip center")
	assertPixelWhite(t, dc.pixmap.GetPixel(50, 130), "outside sheared clip")
}

func TestClipRoundRectShearedPreservesShape(t *testing.T) {
	dc := NewContext(200, 200)
	dc.ClearWithColor(White)
	dc.Translate(100, 100)
	dc.Shear(0.5, 0)
	dc.Translate(-100, -100)
	dc.ClipRoundRect(60, 60, 80, 80, 12)

	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 200, 200)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill() failed: %v", err)
	}

	assertPixelRed(t, dc.pixmap.GetPixel(100, 100), "sheared rounded clip center")
	assertPixelWhite(t, dc.pixmap.GetPixel(50, 130), "outside sheared rounded clip")
}

func TestClipRoundRectNonUniformScaleMatchesPath(t *testing.T) {
	got := renderNonUniformRoundRectClip(false)
	want := renderNonUniformRoundRectClip(true)
	assertPixmapsEqual(t, got, want)
}

func TestClipRoundRectRotatedNegativeDimensionsMatchesPath(t *testing.T) {
	tests := []struct {
		name       string
		x, y, w, h float64
	}{
		{name: "width", x: 140, y: 60, w: -80, h: 80},
		{name: "height", x: 60, y: 140, w: 80, h: -80},
		{name: "both", x: 140, y: 140, w: -80, h: -80},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderRotatedNegativeRoundRectClip(false, tt.x, tt.y, tt.w, tt.h)
			want := renderRotatedNegativeRoundRectClip(true, tt.x, tt.y, tt.w, tt.h)
			assertPixmapsEqual(t, got, want)
		})
	}
}

func TestClipRectScaleTranslatePreservesFastPath(t *testing.T) {
	dc := NewContext(200, 200)
	dc.ClearWithColor(White)
	dc.Translate(20, 30)
	dc.Scale(2, 3)
	dc.ClipRect(10, 10, 20, 20)
	if !dc.clipStack.IsRectOnly() {
		t.Fatal("axis-aligned ClipRect should keep the rectangular clip fast path")
	}

	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 200, 200)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill() failed: %v", err)
	}

	assertPixelRed(t, dc.pixmap.GetPixel(60, 90), "scaled clip center")
	assertPixelWhite(t, dc.pixmap.GetPixel(20, 20), "outside scaled clip")
}

func TestClipRoundRectScaleTranslatePreservesFastPath(t *testing.T) {
	dc := NewContext(200, 200)
	dc.ClearWithColor(White)
	dc.Translate(20, 30)
	dc.Scale(2, 2)
	dc.ClipRoundRect(10, 10, 20, 20, 4)
	if !dc.clipStack.IsRRectOnly() {
		t.Fatal("axis-aligned ClipRoundRect should keep the rounded-rectangle clip path")
	}

	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 200, 200)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill() failed: %v", err)
	}

	assertPixelRed(t, dc.pixmap.GetPixel(60, 70), "scaled rounded clip center")
	assertPixelWhite(t, dc.pixmap.GetPixel(20, 20), "outside scaled rounded clip")
}

func TestClipRectAxisAlignedReflectionKeepsFastPath(t *testing.T) {
	dc := NewContext(200, 200)
	dc.Scale(-2, 3)
	dc.ClipRect(10, 10, 20, 20)
	if !dc.clipStack.IsRectOnly() {
		t.Fatal("axis-aligned reflection should keep the rectangular clip fast path")
	}
}

func TestClipRectExactAxisSwapKeepsFastPath(t *testing.T) {
	dc := NewContext(200, 200)
	dc.SetTransform(Matrix{B: -2, D: 3})
	dc.ClipRect(10, 10, 20, 30)
	if !dc.clipStack.IsRectOnly() {
		t.Fatal("exact axis swap should keep the rectangular clip fast path")
	}
}

func TestClipRectTinyShearUsesPath(t *testing.T) {
	dc := NewContext(200, 200)
	dc.SetTransform(Matrix{A: 1, B: 5e-13, E: 1})
	dc.ClipRect(10, 10, 20, 30)
	if dc.clipStack.IsRectOnly() {
		t.Fatal("a non-zero shear must not use the rectangular clip fast path")
	}
}

func TestClipRoundRectTinyNonUniformScaleUsesPath(t *testing.T) {
	dc := NewContext(200, 200)
	dc.SetTransform(Matrix{A: 1e-13, E: 2e-13})
	dc.ClipRoundRect(10, 10, 20, 30, 4)
	if dc.clipStack.IsRRectOnly() {
		t.Fatal("non-uniform scale must not use the rounded-rectangle clip fast path")
	}
}

func TestClipRoundRectExactAxisSwapKeepsFastPath(t *testing.T) {
	dc := NewContext(200, 200)
	dc.SetTransform(Matrix{B: -2, D: 2})
	dc.ClipRoundRect(10, 10, 20, 30, 4)
	if !dc.clipStack.IsRRectOnly() {
		t.Fatal("uniform axis swap should keep the rounded-rectangle clip fast path")
	}
}

func renderNonUniformRoundRectClip(directPath bool) *Context {
	dc := NewContext(300, 300)
	dc.ClearWithColor(White)
	if directPath {
		path := NewPath()
		path.RoundedRectangle(20, 20, 100, 80, 20)
		dc.SetPath(path.Transform(Scale(2, 3)))
		dc.Clip()
		dc.SetRGB(1, 0, 0)
		dc.DrawRectangle(0, 0, 300, 300)
	} else {
		dc.Scale(2, 3)
		dc.ClipRoundRect(20, 20, 100, 80, 20)
		dc.SetTransform(Identity())
		dc.SetRGB(1, 0, 0)
		dc.DrawRectangle(0, 0, 300, 300)
	}
	_ = dc.Fill()
	return dc
}

func renderRotatedNegativeRoundRectClip(directPath bool, x, y, w, h float64) *Context {
	dc := NewContext(240, 240)
	dc.ClearWithColor(White)
	dc.Translate(120, 120)
	dc.Rotate(math.Pi / 4)
	dc.Translate(-120, -120)
	tm := dc.GetTransform()
	if directPath {
		path := NewPath()
		// All cases above describe the same 80×80 user-space rectangle at
		// (60,60), using different negative-dimension spellings.
		path.RoundedRectangle(60, 60, 80, 80, 12)
		dc.SetTransform(Identity())
		dc.SetPath(path.Transform(tm))
		dc.Clip()
		dc.SetRGB(1, 0, 0)
		dc.DrawRectangle(0, 0, 240, 240)
	} else {
		dc.ClipRoundRect(x, y, w, h, 12)
		dc.SetTransform(Identity())
		dc.SetRGB(1, 0, 0)
		dc.DrawRectangle(0, 0, 240, 240)
	}
	_ = dc.Fill()
	return dc
}

func assertPixmapsEqual(t *testing.T, got, want *Context) {
	t.Helper()
	for y := 0; y < got.pixmap.Height(); y++ {
		for x := 0; x < got.pixmap.Width(); x++ {
			gotPixel := got.pixmap.GetPixel(x, y)
			wantPixel := want.pixmap.GetPixel(x, y)
			if gotPixel != wantPixel {
				t.Fatalf("pixel mismatch at (%d,%d): got=%+v want=%+v", x, y, gotPixel, wantPixel)
			}
		}
	}
}

func assertPixelRed(t *testing.T, got RGBA, label string) {
	t.Helper()
	if got.R < 0.9 || got.G > 0.1 || got.B > 0.1 {
		t.Errorf("%s: expected red, got R=%.3f G=%.3f B=%.3f A=%.3f", label, got.R, got.G, got.B, got.A)
	}
}

func assertPixelWhite(t *testing.T, got RGBA, label string) {
	t.Helper()
	if got.R < 0.9 || got.G < 0.9 || got.B < 0.9 {
		t.Errorf("%s: expected white, got R=%.3f G=%.3f B=%.3f A=%.3f", label, got.R, got.G, got.B, got.A)
	}
}
