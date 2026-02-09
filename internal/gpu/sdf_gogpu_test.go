//go:build !nogpu

package gpu

import (
	"log"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gogpu"
)

// TestSDFWithGogpuWindow tests if GPU SDF works while gogpu window is active.
// This reproduces the real-world scenario where gogpu owns the Vulkan surface
// and the SDF accelerator runs compute on a separate device.
func TestSDFWithGogpuWindow(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	// Create gogpu app (creates Vulkan instance + device + surface)
	app := gogpu.NewApp(gogpu.DefaultConfig().
		WithTitle("SDF Test").
		WithSize(200, 200))

	// Create SDF BEFORE app.Run() — simulates gpu.init() timing
	accel := &SDFAccelerator{}
	if err := accel.Init(); err != nil {
		t.Fatalf("SDF Init: %v", err)
	}
	defer accel.Close()
	t.Log("SDF created BEFORE app.Run()")

	frameCount := 0
	var sdfResult int

	app.OnDraw(func(dc *gogpu.Context) {
		if frameCount > 0 {
			app.Quit()
			return
		}
		frameCount++

		// Match EXACT gg context parameters
		w, h := 784, 561
		data := make([]byte, w*h*4) // all zeros like gg context (transparent clear)

		target := gg.GPURenderTarget{Width: w, Height: h, Data: data}
		shape := gg.DetectedShape{Kind: gg.ShapeCircle, CenterX: 542, CenterY: 280.5, RadiusX: 30, RadiusY: 30}
		paint := &gg.Paint{Brush: gg.SolidBrush{Color: gg.RGBA{R: 1, G: 0.2, B: 0.2, A: 1}}}

		if err := accel.FillShape(target, shape, paint); err != nil {
			t.Errorf("FillShape: %v", err)
			app.Quit()
			return
		}

		nonZero := 0
		for i := 0; i < len(data); i++ {
			if data[i] != 0 {
				nonZero++
			}
		}
		sdfResult = nonZero
		t.Logf("SDF inside OnDraw: %d non-zero bytes in %d total", nonZero, len(data))
		app.Quit()
	})

	if err := app.Run(); err != nil {
		t.Fatalf("App.Run: %v", err)
	}

	if sdfResult == 0 {
		t.Fatal("GPU SDF produced zero pixels inside gogpu OnDraw!")
	}
	t.Logf("PASS: %d non-zero bytes", sdfResult)
}

// TestSDFViaGGContext tests if GPU SDF works when called through gg.Context.Fill()
func TestSDFViaGGContext(t *testing.T) {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	// Register GPU SDF accelerator (same as gpu.init())
	accel := &SDFAccelerator{}
	if err := gg.RegisterAccelerator(accel); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}
	defer func() {
		// Deregister by registering a no-op
		gg.RegisterAccelerator(nil) //nolint
	}()

	// Create gg context (same as ggcanvas)
	w, h := 200, 200
	cc := gg.NewContext(w, h)

	// Clear to transparent (same as gogpu_integration example)
	cc.SetRGBA(0, 0, 0, 0)
	cc.Clear()

	// Draw a circle (should trigger GPU SDF via tryGPUFill)
	cc.SetRGBA(1, 0, 0, 1) // red
	cc.DrawCircle(100, 100, 60)
	cc.Fill()

	// Check pixmap for non-zero pixels
	img := cc.Image()
	bounds := img.Bounds()
	nonZero := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 || a != 0 {
				nonZero++
			}
		}
	}

	t.Logf("gg.Context.Fill() → %d non-zero pixels out of %d", nonZero, w*h)

	// Now try DIRECT call on the SAME accelerator to compare
	directData := make([]byte, w*h*4)
	for i := 0; i < len(directData); i++ {
		directData[i] = 0xFF // fill with white
	}
	target := gg.GPURenderTarget{Width: w, Height: h, Data: directData}
	shape := gg.DetectedShape{Kind: gg.ShapeCircle, CenterX: 100, CenterY: 100, RadiusX: 60, RadiusY: 60}
	paint := &gg.Paint{Brush: gg.SolidBrush{Color: gg.RGBA{R: 1, G: 0, B: 0, A: 1}}}
	if err := accel.FillShape(target, shape, paint); err != nil {
		t.Fatalf("Direct FillShape: %v", err)
	}
	directNonWhite := 0
	for i := 0; i < len(directData); i += 4 {
		if directData[i] != 255 || directData[i+1] != 255 || directData[i+2] != 255 {
			directNonWhite++
		}
	}
	t.Logf("Direct FillShape → %d non-white pixels", directNonWhite)

	if nonZero == 0 && directNonWhite > 0 {
		t.Fatal("BUG: gg.Context.Fill() returns zeros but direct call works!")
	}
}
