package raster

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/recording"
)

func TestRecordingRasterClipSetClip(t *testing.T) {
	rec := recording.NewRecorder(100, 100)
	rec.Translate(10, 10)
	rec.SetRGB(1, 0, 0)
	rec.DrawRectangle(0, 0, 20, 20)
	rec.Clip()
	rec.FillRectangle(0, 0, 100, 100)

	img := playbackRaster(t, rec.FinishRecording())
	assertOpaqueRed(t, img, 15, 15)
	assertTransparent(t, img, 35, 35)
}

func TestRecordingRasterClipIntersectsSuccessiveClips(t *testing.T) {
	rec := recording.NewRecorder(100, 100)
	clipRecorderRect(rec, 10, 10, 80, 80)
	clipRecorderRect(rec, 40, 40, 30, 30)
	rec.SetRGB(1, 0, 0)
	rec.FillRectangle(0, 0, 100, 100)

	img := playbackRaster(t, rec.FinishRecording())
	assertOpaqueRed(t, img, 50, 50)
	assertTransparent(t, img, 20, 20)
	assertTransparent(t, img, 80, 80)
}

func TestRecordingRasterClipClearClip(t *testing.T) {
	rec := recording.NewRecorder(100, 100)
	clipRecorderRect(rec, 20, 20, 60, 60)
	rec.SetRGB(1, 0, 0)
	rec.FillRectangle(0, 0, 100, 100)
	rec.ResetClip()
	rec.SetRGB(0, 0, 1)
	rec.FillRectangle(0, 0, 20, 100)

	img := playbackRaster(t, rec.FinishRecording())
	assertOpaqueBlue(t, img, 10, 50)
	assertOpaqueRed(t, img, 50, 50)
	assertTransparent(t, img, 90, 50)
}

func TestRecordingRasterClipClearClipRestoredByRestore(t *testing.T) {
	rec := recording.NewRecorder(100, 100)
	clipRecorderRect(rec, 10, 10, 80, 80)
	rec.Save()
	rec.ResetClip()
	rec.Restore()
	rec.SetRGB(1, 0, 0)
	rec.FillRectangle(0, 0, 100, 100)

	img := playbackRaster(t, rec.FinishRecording())
	assertOpaqueRed(t, img, 50, 50)
	assertTransparent(t, img, 5, 5)
}

func TestRecordingRasterClipSaveRestore(t *testing.T) {
	rec := recording.NewRecorder(100, 100)
	clipRecorderRect(rec, 10, 10, 80, 80)
	rec.Save()
	clipRecorderRect(rec, 30, 30, 40, 40)
	rec.Restore()
	rec.SetRGB(1, 0, 0)
	rec.FillRectangle(0, 0, 20, 100)

	img := playbackRaster(t, rec.FinishRecording())
	assertOpaqueRed(t, img, 15, 50)
	assertTransparent(t, img, 50, 50)
	assertTransparent(t, img, 5, 5)
}

func TestRecordingRasterClipHonorsFillRule(t *testing.T) {
	for _, tc := range []struct {
		name       string
		rule       recording.FillRule
		centerDraw bool
	}{
		{name: "non-zero", rule: recording.FillRuleNonZero, centerDraw: true},
		{name: "even-odd", rule: recording.FillRuleEvenOdd, centerDraw: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := recording.NewRecorder(100, 100)
			rec.SetFillRule(tc.rule)
			rec.DrawRectangle(10, 10, 80, 80)
			rec.DrawRectangle(30, 30, 40, 40)
			rec.Clip()
			rec.SetRGB(1, 0, 0)
			rec.FillRectangle(0, 0, 100, 100)

			img := playbackRaster(t, rec.FinishRecording())
			assertOpaqueRed(t, img, 15, 50)
			if tc.centerDraw {
				assertOpaqueRed(t, img, 50, 50)
			} else {
				assertTransparent(t, img, 50, 50)
			}
			assertTransparent(t, img, 5, 5)
		})
	}
}

func TestRecordingRasterClipRoundRectTransformParity(t *testing.T) {
	tests := []struct {
		name              string
		transformContext  func(*gg.Context)
		transformRecorder func(*recording.Recorder)
		points            []image.Point
	}{
		{
			name:              "translation",
			transformContext:  func(dc *gg.Context) { dc.Translate(15, 8) },
			transformRecorder: func(rec *recording.Recorder) { rec.Translate(15, 8) },
			points:            []image.Point{{X: 55, Y: 43}, {X: 25, Y: 35}},
		},
		{
			name: "scale",
			transformContext: func(dc *gg.Context) {
				dc.Translate(5, 5)
				dc.Scale(1.5, 0.75)
			},
			transformRecorder: func(rec *recording.Recorder) {
				rec.Translate(5, 5)
				rec.Scale(1.5, 0.75)
			},
			points: []image.Point{{X: 42, Y: 21}, {X: 60, Y: 50}},
		},
		{
			name: "rotation",
			transformContext: func(dc *gg.Context) {
				dc.Translate(60, 5)
				dc.Rotate(math.Pi / 6)
			},
			transformRecorder: func(rec *recording.Recorder) {
				rec.Translate(60, 5)
				rec.Rotate(math.Pi / 6)
			},
			points: []image.Point{{X: 74, Y: 33}, {X: 60, Y: 50}},
		},
		{
			name: "shear",
			transformContext: func(dc *gg.Context) {
				dc.Translate(5, 5)
				dc.Shear(0.6, 0.2)
			},
			transformRecorder: func(rec *recording.Recorder) {
				rec.Translate(5, 5)
				rec.Shear(0.6, 0.2)
			},
			points: []image.Point{{X: 52, Y: 30}, {X: 25, Y: 25}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			direct := gg.NewContext(120, 120)
			tc.transformContext(direct)
			direct.ClipRoundRect(20, 20, 40, 30, 12)
			direct.Identity()
			direct.SetRGB(1, 0, 0)
			direct.DrawRectangle(0, 0, 120, 120)
			if err := direct.Fill(); err != nil {
				t.Fatalf("direct fill failed: %v", err)
			}

			rec := recording.NewRecorder(120, 120)
			tc.transformRecorder(rec)
			rec.ClipRoundRect(20, 20, 40, 30, 12)
			rec.Identity()
			rec.SetRGB(1, 0, 0)
			rec.FillRectangle(0, 0, 120, 120)
			recorded := playbackRaster(t, rec.FinishRecording())

			for _, point := range tc.points {
				want := color.NRGBAModel.Convert(direct.Image().At(point.X, point.Y)).(color.NRGBA).A
				got := color.NRGBAModel.Convert(recorded.At(point.X, point.Y)).(color.NRGBA).A
				if got != want {
					t.Errorf("alpha at %v = %#02x, want direct alpha %#02x", point, got, want)
				}
			}
		})
	}
}

func TestRecordingRasterClipDisjointSubpaths(t *testing.T) {
	for _, rule := range []recording.FillRule{recording.FillRuleNonZero, recording.FillRuleEvenOdd} {
		name := "non-zero"
		if rule == recording.FillRuleEvenOdd {
			name = "even-odd"
		}
		t.Run(name, func(t *testing.T) {
			rec := recording.NewRecorder(100, 100)
			rec.SetFillRule(rule)
			rec.DrawRectangle(10, 10, 20, 20)
			rec.DrawRectangle(70, 70, 20, 20)
			rec.Clip()
			rec.SetRGB(1, 0, 0)
			rec.FillRectangle(0, 0, 100, 100)

			img := playbackRaster(t, rec.FinishRecording())
			assertOpaqueRed(t, img, 20, 20)
			assertOpaqueRed(t, img, 80, 80)
			assertTransparent(t, img, 50, 50)
		})
	}
}

func clipRecorderRect(rec *recording.Recorder, x, y, w, h float64) {
	rec.DrawRectangle(x, y, w, h)
	rec.Clip()
}

func playbackRaster(t *testing.T, rec *recording.Recording) image.Image {
	t.Helper()
	backend := NewBackend()
	if err := rec.Playback(backend); err != nil {
		t.Fatalf("recording playback failed: %v", err)
	}
	return backend.Image()
}

func assertOpaqueRed(t *testing.T, img image.Image, x, y int) {
	t.Helper()
	r, g, b, a := img.At(x, y).RGBA()
	if a < 0xc000 || r < 0xc000 || g > 0x1000 || b > 0x1000 {
		t.Errorf("pixel at (%d,%d) = %#04x %#04x %#04x %#04x, want opaque red", x, y, r, g, b, a)
	}
}

func assertOpaqueBlue(t *testing.T, img image.Image, x, y int) {
	t.Helper()
	r, g, b, a := img.At(x, y).RGBA()
	if a < 0xc000 || r > 0x1000 || g > 0x1000 || b < 0xc000 {
		t.Errorf("pixel at (%d,%d) = %#04x %#04x %#04x %#04x, want opaque blue", x, y, r, g, b, a)
	}
}

func assertTransparent(t *testing.T, img image.Image, x, y int) {
	t.Helper()
	rgba := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	if rgba.A > 0x10 {
		t.Errorf("pixel at (%d,%d) has alpha %#02x, want transparent", x, y, rgba.A)
	}
}
