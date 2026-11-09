package gg

import (
	"sync"
	"testing"
)

// claimAllAccelerator claims every operation and counts what reaches it. It
// paints nothing, so a draw it took shows up as a missing draw.
type claimAllAccelerator struct {
	mu    sync.Mutex
	calls int
}

func (a *claimAllAccelerator) Name() string                     { return "claim-all" }
func (a *claimAllAccelerator) Init() error                      { return nil }
func (a *claimAllAccelerator) Close()                           {}
func (a *claimAllAccelerator) CanAccelerate(AcceleratedOp) bool { return true }
func (a *claimAllAccelerator) Flush(GPURenderTarget) error      { return nil }

func (a *claimAllAccelerator) took() error {
	a.mu.Lock()
	a.calls++
	a.mu.Unlock()
	return nil
}

func (a *claimAllAccelerator) FillPath(GPURenderTarget, *Path, *Paint) error   { return a.took() }
func (a *claimAllAccelerator) StrokePath(GPURenderTarget, *Path, *Paint) error { return a.took() }
func (a *claimAllAccelerator) FillShape(GPURenderTarget, DetectedShape, *Paint) error {
	return a.took()
}
func (a *claimAllAccelerator) StrokeShape(GPURenderTarget, DetectedShape, *Paint) error {
	return a.took()
}

func (a *claimAllAccelerator) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

// TestGradientFillsAreLeftToTheCPU verifies that a gradient fill never reaches
// the GPU accelerator.
//
// The GPU tiers paint a draw in one color, read from the brush at (0, 0), so
// a gradient handed to them came out as a flat fill of its first stop: a
// color bar rendered as one solid block. The CPU samples the brush per pixel.
func TestGradientFillsAreLeftToTheCPU(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()
	accel := &claimAllAccelerator{}
	if err := RegisterAccelerator(accel); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(100, 40)
	defer func() { _ = dc.Close() }()
	grad := NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, RGBA{R: 1, A: 1}).
		AddColorStop(1, RGBA{B: 1, A: 1})
	dc.SetFillBrush(grad)
	// A path shape rather than a rectangle, so both the SDF and the path
	// entry points are exercised across the two tests below.
	dc.MoveTo(0, 0)
	dc.LineTo(100, 0)
	dc.LineTo(100, 40)
	dc.LineTo(0, 40)
	dc.ClosePath()
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill: %v", err)
	}

	if n := accel.count(); n != 0 {
		t.Errorf("a gradient fill reached the GPU accelerator %d times, want 0", n)
	}
	img := dc.Image()
	lr, _, lb, _ := img.At(5, 20).RGBA()
	rr, _, rb, _ := img.At(95, 20).RGBA()
	if lr <= lb || rb <= rr {
		t.Errorf("the gradient is not painted: left pixel r=%d b=%d, right pixel r=%d b=%d", lr>>8, lb>>8, rr>>8, rb>>8)
	}
}

// TestGradientStrokesAreLeftToTheCPU is the stroke side of the same rule.
func TestGradientStrokesAreLeftToTheCPU(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()
	accel := &claimAllAccelerator{}
	if err := RegisterAccelerator(accel); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(100, 40)
	defer func() { _ = dc.Close() }()
	grad := NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, RGBA{R: 1, A: 1}).
		AddColorStop(1, RGBA{B: 1, A: 1})
	dc.SetStrokeBrush(grad)
	dc.SetLineWidth(10)
	dc.MoveTo(0, 20)
	dc.LineTo(100, 20)
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke: %v", err)
	}

	if n := accel.count(); n != 0 {
		t.Errorf("a gradient stroke reached the GPU accelerator %d times, want 0", n)
	}
}

// TestSolidFillsStillReachTheGPU verifies the rule costs solid draws nothing:
// they go to the accelerator as before.
func TestSolidFillsStillReachTheGPU(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()
	accel := &claimAllAccelerator{}
	if err := RegisterAccelerator(accel); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(100, 40)
	defer func() { _ = dc.Close() }()
	dc.SetFillBrush(Solid(RGBA{G: 1, A: 1}))
	dc.DrawRectangle(10, 10, 50, 20)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if n := accel.count(); n == 0 {
		t.Error("a solid fill did not reach the GPU accelerator")
	}
}
