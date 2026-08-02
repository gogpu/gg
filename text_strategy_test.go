package gg

import (
	"math"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/gogpu/gg/text"
)

type textStrategyGlyphMaskAccelerator struct {
	*mockAccelerator
	drawTextCalls int
}

func (*textStrategyGlyphMaskAccelerator) DrawGlyphMaskText(
	GPURenderTarget, any, string, float64, float64, RGBA, Matrix, float64,
) error {
	return nil
}

func (a *textStrategyGlyphMaskAccelerator) DrawText(
	GPURenderTarget, any, string, float64, float64, RGBA, Matrix, float64,
) error {
	a.drawTextCalls++
	return nil
}

func installTextStrategyAccelerator(t *testing.T) *textStrategyGlyphMaskAccelerator {
	t.Helper()
	installed := &textStrategyGlyphMaskAccelerator{
		mockAccelerator: &mockAccelerator{name: "glyph-mask-test", canAccel: AccelText},
	}
	accelMu.Lock()
	previous := accel
	accel = installed
	accelMu.Unlock()
	t.Cleanup(func() {
		accelMu.Lock()
		accel = previous
		accelMu.Unlock()
	})
	return installed
}

func TestTextStrategyGlyphMaskDeviceThreshold(t *testing.T) {
	t.Setenv("GOGPU_TEXT_MODE", "")
	installTextStrategyAccelerator(t)

	source, err := text.NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("NewFontSource: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })

	tests := []struct {
		name        string
		faceSize    float64
		deviceScale float64
		matrix      Matrix
		deviceSize  float64
		want        TextMode
	}{
		{name: "63.99_at_1x", faceSize: 63.99, deviceScale: 1, matrix: Identity(), deviceSize: 63.99, want: TextModeGlyphMask},
		{name: "64_at_1x", faceSize: 64, deviceScale: 1, matrix: Identity(), deviceSize: 64, want: TextModeGlyphMask},
		{name: "64.01_at_1x", faceSize: 64.01, deviceScale: 1, matrix: Identity(), deviceSize: 64.01, want: TextModeAuto},
		{name: "63.99_at_2x", faceSize: 63.99 / 2, deviceScale: 2, matrix: Identity(), deviceSize: 63.99, want: TextModeGlyphMask},
		{name: "64_at_2x", faceSize: 32, deviceScale: 2, matrix: Identity(), deviceSize: 64, want: TextModeGlyphMask},
		{name: "64.01_at_2x", faceSize: 64.01 / 2, deviceScale: 2, matrix: Identity(), deviceSize: 64.01, want: TextModeAuto},
		{name: "63.99_uniform_CTM_scale", faceSize: 63.99 / 2, deviceScale: 1, matrix: Scale(2, 2), deviceSize: 63.99, want: TextModeGlyphMask},
		{name: "64_uniform_CTM_scale", faceSize: 32, deviceScale: 1, matrix: Scale(2, 2), deviceSize: 64, want: TextModeGlyphMask},
		{name: "64.01_uniform_CTM_scale", faceSize: 64.01 / 2, deviceScale: 1, matrix: Scale(2, 2), deviceSize: 64.01, want: TextModeAuto},
		{name: "translation_is_allowed", faceSize: 12, deviceScale: 1, matrix: Translate(8, 13), deviceSize: 12, want: TextModeGlyphMask},
		{name: "rotation_is_rejected", faceSize: 12, deviceScale: 1, matrix: Rotate(math.Pi / 4), deviceSize: 12 * math.Cos(math.Pi/4), want: TextModeAuto},
		{name: "skew_is_rejected", faceSize: 12, deviceScale: 1, matrix: Shear(0.25, 0), deviceSize: 12, want: TextModeAuto},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := NewContext(16, 16, WithDeviceScale(tt.deviceScale))
			t.Cleanup(func() { _ = dc.Close() })
			dc.SetFont(source.Face(tt.faceSize))
			dc.matrix = tt.matrix

			if got := dc.glyphMaskDeviceSize(); math.Abs(got-tt.deviceSize) > 1e-9 {
				t.Fatalf("glyphMaskDeviceSize() = %g, want %g", got, tt.deviceSize)
			}
			if got := dc.selectTextStrategy(); got != tt.want {
				t.Errorf("selectTextStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTextStrategyExplicitModesIgnoreAutoThresholds(t *testing.T) {
	t.Setenv("GOGPU_TEXT_MODE", "")
	dc := NewContext(16, 16)
	t.Cleanup(func() { _ = dc.Close() })

	for _, mode := range []TextMode{
		TextModeGlyphMask,
		TextModeAliased,
		TextModeMSDF,
		TextModeVector,
		TextModeBitmap,
	} {
		dc.SetTextMode(mode)
		dc.matrix = Rotate(math.Pi / 3)
		if got := dc.selectTextStrategy(); got != mode {
			t.Errorf("explicit %v selected %v", mode, got)
		}
	}
}

func TestTextStrategyAutoFallsThroughToMSDF(t *testing.T) {
	t.Setenv("GOGPU_TEXT_MODE", "")
	accelerator := installTextStrategyAccelerator(t)

	source, err := text.NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatalf("NewFontSource: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })

	dc := NewContext(16, 16)
	t.Cleanup(func() { _ = dc.Close() })
	dc.SetFont(source.Face(glyphMaskMaxSize + 0.01))
	dc.DrawString("A", 1, 12)
	if accelerator.drawTextCalls != 1 {
		t.Fatalf("MSDF DrawText calls = %d, want 1", accelerator.drawTextCalls)
	}
}

func TestGlyphMaskDeviceSizeWithoutFont(t *testing.T) {
	dc := NewContext(1, 1)
	t.Cleanup(func() { _ = dc.Close() })
	if got := dc.glyphMaskDeviceSize(); got != 0 {
		t.Fatalf("glyphMaskDeviceSize() = %g, want 0 without a font", got)
	}
}
