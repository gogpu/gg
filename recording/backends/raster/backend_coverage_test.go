package raster

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/recording"
)

func TestBackendApplyBrushLinearGradient(t *testing.T) {
	backend := NewBackend()
	err := backend.Begin(100, 100)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	// Linear gradient brush
	grad := recording.NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, gg.Red).
		AddColorStop(1, gg.Blue)

	rect := recording.NewRect(0, 0, 100, 100)
	backend.FillRect(rect, grad)

	err = backend.End()
	if err != nil {
		t.Fatalf("End: %v", err)
	}
}

func TestBackendApplyBrushRadialGradient(t *testing.T) {
	backend := NewBackend()
	err := backend.Begin(100, 100)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	grad := recording.NewRadialGradientBrush(50, 50, 0, 50).
		AddColorStop(0, gg.White).
		AddColorStop(1, gg.Black)

	rect := recording.NewRect(0, 0, 100, 100)
	backend.FillRect(rect, grad)

	_ = backend.End()
}

func TestBackendApplyBrushSweepGradient(t *testing.T) {
	backend := NewBackend()
	err := backend.Begin(100, 100)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	grad := recording.NewSweepGradientBrush(50, 50, 0).
		AddColorStop(0, gg.Red).
		AddColorStop(0.5, gg.Green).
		AddColorStop(1, gg.Blue)

	rect := recording.NewRect(0, 0, 100, 100)
	backend.FillRect(rect, grad)

	_ = backend.End()
}

func TestBackendApplyStrokeBrush(t *testing.T) {
	backend := NewBackend()
	err := backend.Begin(100, 100)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	// Stroke with gradient
	grad := recording.NewLinearGradientBrush(0, 0, 100, 0).
		AddColorStop(0, gg.Red).
		AddColorStop(1, gg.Blue)

	path := gg.NewPath()
	path.MoveTo(10, 50)
	path.LineTo(90, 50)

	stroke := recording.Stroke{
		Width:      3.0,
		Cap:        recording.LineCapSquare,
		Join:       recording.LineJoinBevel,
		MiterLimit: 4.0,
	}
	backend.StrokePath(path, grad, stroke)

	_ = backend.End()
}

func TestBackendSetClip(t *testing.T) {
	backend := NewBackend()
	err := backend.Begin(100, 100)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	path := gg.NewPath()
	path.Rectangle(10, 10, 80, 80)
	backend.SetClip(path, recording.FillRuleNonZero)

	// SetClip with nil should not panic
	backend.SetClip(nil, recording.FillRuleNonZero)

	// ClearClip should not panic
	backend.ClearClip()

	_ = backend.End()
}

func TestBackendFillPathNil(t *testing.T) {
	backend := NewBackend()
	_ = backend.Begin(100, 100)

	// Nil path should not panic
	backend.FillPath(nil, recording.NewSolidBrush(gg.Red), recording.FillRuleNonZero)

	_ = backend.End()
}

func TestBackendStrokePathNil(t *testing.T) {
	backend := NewBackend()
	_ = backend.Begin(100, 100)

	// Nil path should not panic
	backend.StrokePath(nil, recording.NewSolidBrush(gg.Red), recording.Stroke{Width: 2})

	_ = backend.End()
}

func TestBackendDrawImageNil(t *testing.T) {
	backend := NewBackend()
	_ = backend.Begin(100, 100)

	// Nil image should not panic
	backend.DrawImage(nil, recording.NewRect(0, 0, 50, 50), recording.NewRect(0, 0, 50, 50), recording.ImageOptions{})

	_ = backend.End()
}

func TestBackendDrawText(t *testing.T) {
	backend := NewBackend()
	_ = backend.Begin(100, 100)

	// DrawText with nil face should not panic
	backend.DrawText("hello", 10, 10, nil, recording.NewSolidBrush(gg.Black))

	_ = backend.End()
}

func TestBackendFillRuleEvenOdd(t *testing.T) {
	backend := NewBackend()
	_ = backend.Begin(100, 100)

	path := gg.NewPath()
	path.Rectangle(10, 10, 80, 80)
	backend.FillPath(path, recording.NewSolidBrush(gg.Red), recording.FillRuleEvenOdd)

	_ = backend.End()
}

func TestBackendSavePNG(t *testing.T) {
	backend := NewBackend()
	_ = backend.Begin(10, 10)

	rect := recording.NewRect(0, 0, 10, 10)
	backend.FillRect(rect, recording.NewSolidBrush(gg.Red))

	_ = backend.End()

	// SavePNG to non-existent directory should fail
	err := backend.SavePNG("/nonexistent/path/test.png")
	if err == nil {
		t.Error("SavePNG to invalid path should return error")
	}
}

func TestBackendConvertFillRule(t *testing.T) {
	if convertFillRule(recording.FillRuleNonZero) != gg.FillRuleNonZero {
		t.Error("FillRuleNonZero conversion failed")
	}
	if convertFillRule(recording.FillRuleEvenOdd) != gg.FillRuleEvenOdd {
		t.Error("FillRuleEvenOdd conversion failed")
	}
}

func TestBackendConvertLineCap(t *testing.T) {
	if convertLineCap(recording.LineCapButt) != gg.LineCapButt {
		t.Error("LineCapButt conversion failed")
	}
	if convertLineCap(recording.LineCapRound) != gg.LineCapRound {
		t.Error("LineCapRound conversion failed")
	}
	if convertLineCap(recording.LineCapSquare) != gg.LineCapSquare {
		t.Error("LineCapSquare conversion failed")
	}
}

func TestBackendConvertLineJoin(t *testing.T) {
	if convertLineJoin(recording.LineJoinMiter) != gg.LineJoinMiter {
		t.Error("LineJoinMiter conversion failed")
	}
	if convertLineJoin(recording.LineJoinRound) != gg.LineJoinRound {
		t.Error("LineJoinRound conversion failed")
	}
	if convertLineJoin(recording.LineJoinBevel) != gg.LineJoinBevel {
		t.Error("LineJoinBevel conversion failed")
	}
}
