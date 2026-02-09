//go:build !nogpu

package gpu

import (
	"image"
	"image/color"
	"log"
	"testing"

	"github.com/gogpu/gg"
)

func TestSDFPipelineCircle(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	w, h := 200, 200
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	accel := &SDFAccelerator{}
	if err := accel.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer accel.Close()

	target := gg.GPURenderTarget{
		Width:  w,
		Height: h,
		Data:   img.Pix,
	}

	shape := gg.DetectedShape{
		Kind:    gg.ShapeCircle,
		CenterX: 100,
		CenterY: 100,
		RadiusX: 60,
		RadiusY: 60,
	}
	paint := &gg.Paint{
		Brush: gg.SolidBrush{Color: gg.RGBA{R: 1, G: 0, B: 0, A: 1}},
	}

	if err := accel.FillShape(target, shape, paint); err != nil {
		t.Fatalf("FillShape: %v", err)
	}

	nonWhite := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 255 || img.Pix[i+1] != 255 || img.Pix[i+2] != 255 {
			nonWhite++
		}
	}

	t.Logf("Result: %d non-white pixels out of %d", nonWhite, w*h)
	if nonWhite == 0 {
		t.Fatal("GPU SDF produced zero non-white pixels")
	}
	t.Logf("PASS: GPU SDF circle rendered %d pixels", nonWhite)
}
