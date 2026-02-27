package recording

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/gogpu/gg"
)

func TestNewRecorder(t *testing.T) {
	rec := NewRecorder(800, 600)

	if rec.Width() != 800 {
		t.Errorf("Width() = %d, want 800", rec.Width())
	}
	if rec.Height() != 600 {
		t.Errorf("Height() = %d, want 600", rec.Height())
	}
	if rec.currentPath == nil {
		t.Error("currentPath should not be nil")
	}
	if rec.resources == nil {
		t.Error("resources should not be nil")
	}
}

func TestRecorderFinishRecording(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.SetRGB(1, 0, 0)
	rec.DrawCircle(50, 50, 25)
	rec.Fill()

	recording := rec.FinishRecording()

	if recording == nil {
		t.Fatal("FinishRecording returned nil")
	}
	if recording.Width() != 100 {
		t.Errorf("recording.Width() = %d, want 100", recording.Width())
	}
	if recording.Height() != 100 {
		t.Errorf("recording.Height() = %d, want 100", recording.Height())
	}
	if len(recording.Commands()) == 0 {
		t.Error("recording should have commands")
	}
	if recording.Resources() == nil {
		t.Error("recording.Resources() should not be nil")
	}
}

func TestRecorderSaveRestore(t *testing.T) {
	rec := NewRecorder(100, 100)

	// Set initial state
	rec.SetRGB(1, 0, 0)
	rec.SetLineWidth(5)
	rec.SetLineCap(LineCapRound)

	// Save state
	rec.Save()

	// Modify state
	rec.SetRGB(0, 1, 0)
	rec.SetLineWidth(10)
	rec.SetLineCap(LineCapSquare)

	// Verify modified state
	if rec.lineWidth != 10 {
		t.Errorf("lineWidth = %f, want 10", rec.lineWidth)
	}
	if rec.lineCap != LineCapSquare {
		t.Errorf("lineCap = %v, want LineCapSquare", rec.lineCap)
	}

	// Restore state
	rec.Restore()

	// Verify restored state
	if rec.lineWidth != 5 {
		t.Errorf("after restore, lineWidth = %f, want 5", rec.lineWidth)
	}
	if rec.lineCap != LineCapRound {
		t.Errorf("after restore, lineCap = %v, want LineCapRound", rec.lineCap)
	}

	// Verify commands
	recording := rec.FinishRecording()
	hasRestore := false
	for _, cmd := range recording.Commands() {
		if cmd.Type() == CmdRestore {
			hasRestore = true
			break
		}
	}
	if !hasRestore {
		t.Error("recording should have RestoreCommand")
	}
}

func TestRecorderPushPop(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.SetLineWidth(3)
	rec.Push()
	rec.SetLineWidth(6)
	rec.Pop()

	if rec.lineWidth != 3 {
		t.Errorf("after Pop, lineWidth = %f, want 3", rec.lineWidth)
	}
}

func TestRecorderTransform(t *testing.T) {
	rec := NewRecorder(100, 100)

	// Test Identity
	rec.Identity()
	if !rec.transform.IsIdentity() {
		t.Error("Identity should set identity matrix")
	}

	// Test Translate
	rec.Identity()
	rec.Translate(10, 20)
	x, y := rec.TransformPoint(0, 0)
	if x != 10 || y != 20 {
		t.Errorf("after Translate(10, 20), (0,0) -> (%f, %f), want (10, 20)", x, y)
	}

	// Test Scale
	rec.Identity()
	rec.Scale(2, 3)
	x, y = rec.TransformPoint(5, 5)
	if x != 10 || y != 15 {
		t.Errorf("after Scale(2, 3), (5,5) -> (%f, %f), want (10, 15)", x, y)
	}

	// Test Rotate
	rec.Identity()
	rec.Rotate(math.Pi / 2) // 90 degrees
	x, y = rec.TransformPoint(1, 0)
	if math.Abs(x) > 1e-10 || math.Abs(y-1) > 1e-10 {
		t.Errorf("after Rotate(Pi/2), (1,0) -> (%f, %f), want (0, 1)", x, y)
	}

	// Test SetTransform
	m := Translate(100, 100)
	rec.SetTransform(m)
	if rec.GetTransform().C != 100 || rec.GetTransform().F != 100 {
		t.Error("SetTransform did not set the matrix correctly")
	}
}

func TestRecorderSetColor(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.SetRGB(1, 0, 0)
	brush, ok := rec.fillBrush.(SolidBrush)
	if !ok {
		t.Fatal("fillBrush should be SolidBrush")
	}
	if brush.Color.R != 1 || brush.Color.G != 0 || brush.Color.B != 0 {
		t.Errorf("SetRGB(1,0,0) resulted in %v", brush.Color)
	}

	rec.SetRGBA(0, 1, 0, 0.5)
	brush = rec.fillBrush.(SolidBrush)
	if brush.Color.G != 1 || brush.Color.A != 0.5 {
		t.Errorf("SetRGBA(0,1,0,0.5) resulted in %v", brush.Color)
	}

	rec.SetHexColor("#0000FF")
	brush = rec.fillBrush.(SolidBrush)
	if brush.Color.B != 1 {
		t.Errorf("SetHexColor('#0000FF') resulted in B=%f, want 1", brush.Color.B)
	}
}

func TestRecorderSetFillStrokeStyle(t *testing.T) {
	rec := NewRecorder(100, 100)

	// Test SetFillStyle
	fillBrush := NewSolidBrush(gg.Red)
	rec.SetFillStyle(fillBrush)
	if rec.fillBrush.(SolidBrush).Color != gg.Red {
		t.Error("SetFillStyle did not set fill brush correctly")
	}

	// Test SetStrokeStyle
	strokeBrush := NewSolidBrush(gg.Blue)
	rec.SetStrokeStyle(strokeBrush)
	if rec.strokeBrush.(SolidBrush).Color != gg.Blue {
		t.Error("SetStrokeStyle did not set stroke brush correctly")
	}

	// Test SetFillBrush (from gg.Brush)
	rec.SetFillBrush(gg.Solid(gg.Green))
	if rec.fillBrush.(SolidBrush).Color != gg.Green {
		t.Error("SetFillBrush did not set fill brush correctly")
	}
}

func TestRecorderLineProperties(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.SetLineWidth(5)
	if rec.lineWidth != 5 {
		t.Errorf("lineWidth = %f, want 5", rec.lineWidth)
	}

	rec.SetLineCap(LineCapRound)
	if rec.lineCap != LineCapRound {
		t.Errorf("lineCap = %v, want LineCapRound", rec.lineCap)
	}

	rec.SetLineJoin(LineJoinBevel)
	if rec.lineJoin != LineJoinBevel {
		t.Errorf("lineJoin = %v, want LineJoinBevel", rec.lineJoin)
	}

	rec.SetMiterLimit(8)
	if rec.miterLimit != 8 {
		t.Errorf("miterLimit = %f, want 8", rec.miterLimit)
	}

	rec.SetDash(5, 3, 2)
	if len(rec.dashPattern) != 3 {
		t.Errorf("dashPattern length = %d, want 3", len(rec.dashPattern))
	}

	rec.SetDashOffset(2)
	if rec.dashOffset != 2 {
		t.Errorf("dashOffset = %f, want 2", rec.dashOffset)
	}

	rec.ClearDash()
	if rec.dashPattern != nil {
		t.Error("ClearDash should set dashPattern to nil")
	}
}

func TestRecorderFillRule(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.SetFillRule(FillRuleEvenOdd)
	if rec.fillRule != FillRuleEvenOdd {
		t.Errorf("fillRule = %v, want FillRuleEvenOdd", rec.fillRule)
	}

	rec.SetFillRuleGG(gg.FillRuleNonZero)
	if rec.fillRule != FillRuleNonZero {
		t.Errorf("fillRule = %v, want FillRuleNonZero", rec.fillRule)
	}
}

func TestRecorderPathBuilding(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.MoveTo(10, 20)
	rec.LineTo(30, 40)
	rec.QuadraticTo(50, 60, 70, 80)
	rec.CubicTo(10, 20, 30, 40, 50, 60)
	rec.ClosePath()

	if len(rec.currentPath.Elements()) == 0 {
		t.Error("path should not be empty after path operations")
	}

	rec.ClearPath()
	if len(rec.currentPath.Elements()) != 0 {
		t.Error("path should be empty after ClearPath")
	}
}

func TestRecorderFill(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.SetRGB(1, 0, 0)
	rec.DrawRectangle(10, 10, 80, 80)
	rec.Fill()

	recording := rec.FinishRecording()

	// Should have FillPathCommand
	hasFillPath := false
	for _, cmd := range recording.Commands() {
		if cmd.Type() == CmdFillPath {
			hasFillPath = true
			break
		}
	}
	if !hasFillPath {
		t.Error("recording should have FillPathCommand")
	}

	// Path should be cleared after Fill
	if len(rec.currentPath.Elements()) != 0 {
		t.Error("path should be empty after Fill")
	}
}

func TestRecorderFillPreserve(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.DrawCircle(50, 50, 25)
	rec.FillPreserve()

	// Path should NOT be cleared after FillPreserve
	if len(rec.currentPath.Elements()) == 0 {
		t.Error("path should not be empty after FillPreserve")
	}
}

func TestRecorderStroke(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.SetRGB(0, 0, 1)
	rec.SetLineWidth(3)
	rec.DrawLine(10, 10, 90, 90)
	rec.Stroke()

	recording := rec.FinishRecording()

	// Should have StrokePathCommand
	hasStrokePath := false
	for _, cmd := range recording.Commands() {
		if cmd.Type() == CmdStrokePath {
			hasStrokePath = true
			strokeCmd := cmd.(StrokePathCommand)
			if strokeCmd.Stroke.Width != 3 {
				t.Errorf("stroke width = %f, want 3", strokeCmd.Stroke.Width)
			}
			break
		}
	}
	if !hasStrokePath {
		t.Error("recording should have StrokePathCommand")
	}

	// Path should be cleared after Stroke
	if len(rec.currentPath.Elements()) != 0 {
		t.Error("path should be empty after Stroke")
	}
}

func TestRecorderStrokePreserve(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.DrawLine(10, 10, 90, 90)
	rec.StrokePreserve()

	// Path should NOT be cleared after StrokePreserve
	if len(rec.currentPath.Elements()) == 0 {
		t.Error("path should not be empty after StrokePreserve")
	}
}

func TestRecorderFillStroke(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.SetFillRGB(1, 0, 0)
	rec.SetStrokeRGB(0, 0, 1)
	rec.DrawCircle(50, 50, 30)
	rec.FillStroke()

	recording := rec.FinishRecording()

	hasFill := false
	hasStroke := false
	for _, cmd := range recording.Commands() {
		if cmd.Type() == CmdFillPath {
			hasFill = true
		}
		if cmd.Type() == CmdStrokePath {
			hasStroke = true
		}
	}
	if !hasFill || !hasStroke {
		t.Error("FillStroke should produce both FillPathCommand and StrokePathCommand")
	}
}

func TestRecorderShapes(t *testing.T) {
	tests := []struct {
		name string
		draw func(*Recorder)
	}{
		{"DrawPoint", func(r *Recorder) { r.DrawPoint(50, 50, 5) }},
		{"DrawLine", func(r *Recorder) { r.DrawLine(10, 10, 90, 90) }},
		{"DrawRectangle", func(r *Recorder) { r.DrawRectangle(10, 10, 80, 80) }},
		{"DrawRoundedRectangle", func(r *Recorder) { r.DrawRoundedRectangle(10, 10, 80, 80, 10) }},
		{"DrawCircle", func(r *Recorder) { r.DrawCircle(50, 50, 40) }},
		{"DrawEllipse", func(r *Recorder) { r.DrawEllipse(50, 50, 40, 30) }},
		{"DrawArc", func(r *Recorder) { r.DrawArc(50, 50, 40, 0, math.Pi) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := NewRecorder(100, 100)
			tt.draw(rec)

			if len(rec.currentPath.Elements()) == 0 {
				t.Errorf("%s should add elements to the path", tt.name)
			}

			rec.Fill()
			recording := rec.FinishRecording()

			if len(recording.Commands()) == 0 {
				t.Errorf("%s + Fill should produce commands", tt.name)
			}
		})
	}
}

func TestRecorderOptimizedRectangles(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.SetRGB(1, 0, 0)
	rec.FillRectangle(10, 10, 80, 80)

	rec.SetRGB(0, 1, 0)
	rec.SetLineWidth(2)
	rec.StrokeRectangle(20, 20, 60, 60)

	recording := rec.FinishRecording()

	hasFillRect := false
	hasStrokeRect := false
	for _, cmd := range recording.Commands() {
		if cmd.Type() == CmdFillRect {
			hasFillRect = true
			fillCmd := cmd.(FillRectCommand)
			if fillCmd.Rect.Width() != 80 || fillCmd.Rect.Height() != 80 {
				t.Errorf("FillRect dimensions = %f x %f, want 80 x 80",
					fillCmd.Rect.Width(), fillCmd.Rect.Height())
			}
		}
		if cmd.Type() == CmdStrokeRect {
			hasStrokeRect = true
		}
	}
	if !hasFillRect {
		t.Error("should have FillRectCommand")
	}
	if !hasStrokeRect {
		t.Error("should have StrokeRectCommand")
	}
}

func TestRecorderClipping(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.DrawCircle(50, 50, 40)
	rec.Clip()

	recording := rec.FinishRecording()

	hasSetClip := false
	for _, cmd := range recording.Commands() {
		if cmd.Type() == CmdSetClip {
			hasSetClip = true
			break
		}
	}
	if !hasSetClip {
		t.Error("Clip should produce SetClipCommand")
	}

	// Path should be cleared after Clip
	if len(rec.currentPath.Elements()) != 0 {
		t.Error("path should be empty after Clip")
	}
}

func TestRecorderClipPreserve(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.DrawCircle(50, 50, 40)
	rec.ClipPreserve()

	// Path should NOT be cleared after ClipPreserve
	if len(rec.currentPath.Elements()) == 0 {
		t.Error("path should not be empty after ClipPreserve")
	}
}

func TestRecorderResetClip(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.DrawCircle(50, 50, 40)
	rec.Clip()
	rec.ResetClip()

	recording := rec.FinishRecording()

	hasClearClip := false
	for _, cmd := range recording.Commands() {
		if cmd.Type() == CmdClearClip {
			hasClearClip = true
			break
		}
	}
	if !hasClearClip {
		t.Error("ResetClip should produce ClearClipCommand")
	}
}

func TestRecorderDrawImage(t *testing.T) {
	rec := NewRecorder(100, 100)

	// Create a simple test image
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}

	rec.DrawImage(img, 10, 20)

	recording := rec.FinishRecording()

	hasDrawImage := false
	for _, cmd := range recording.Commands() {
		if cmd.Type() == CmdDrawImage {
			hasDrawImage = true
			imgCmd := cmd.(DrawImageCommand)
			if imgCmd.DstRect.MinX != 10 || imgCmd.DstRect.MinY != 20 {
				t.Errorf("DrawImage position = (%f, %f), want (10, 20)",
					imgCmd.DstRect.MinX, imgCmd.DstRect.MinY)
			}
			break
		}
	}
	if !hasDrawImage {
		t.Error("DrawImage should produce DrawImageCommand")
	}

	// Check image was added to resources
	if recording.Resources().ImageCount() != 1 {
		t.Errorf("ImageCount = %d, want 1", recording.Resources().ImageCount())
	}
}

func TestRecorderDrawImageScaled(t *testing.T) {
	rec := NewRecorder(100, 100)

	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	rec.DrawImageScaled(img, 0, 0, 100, 100)

	recording := rec.FinishRecording()

	for _, cmd := range recording.Commands() {
		if cmd.Type() == CmdDrawImage {
			imgCmd := cmd.(DrawImageCommand)
			if imgCmd.DstRect.Width() != 100 || imgCmd.DstRect.Height() != 100 {
				t.Errorf("DrawImageScaled dest size = %f x %f, want 100 x 100",
					imgCmd.DstRect.Width(), imgCmd.DstRect.Height())
			}
			return
		}
	}
	t.Error("DrawImageScaled should produce DrawImageCommand")
}

func TestRecorderDrawString(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.SetFontSize(12)
	rec.SetFontFamily("Arial")
	rec.SetRGB(0, 0, 0)
	rec.DrawString("Hello", 10, 50)

	recording := rec.FinishRecording()

	hasDrawText := false
	for _, cmd := range recording.Commands() {
		if cmd.Type() != CmdDrawText {
			continue
		}
		hasDrawText = true
		textCmd := cmd.(DrawTextCommand)
		if textCmd.Text != "Hello" {
			t.Errorf("Text = %q, want 'Hello'", textCmd.Text)
		}
		if textCmd.FontSize != 12 {
			t.Errorf("FontSize = %f, want 12", textCmd.FontSize)
		}
		if textCmd.FontFamily != "Arial" {
			t.Errorf("FontFamily = %q, want 'Arial'", textCmd.FontFamily)
		}
		break
	}
	if !hasDrawText {
		t.Error("DrawString should produce DrawTextCommand")
	}
}

func TestRecorderClear(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.SetRGB(1, 1, 1)
	rec.Clear()

	recording := rec.FinishRecording()

	hasFillRect := false
	for _, cmd := range recording.Commands() {
		if cmd.Type() == CmdFillRect {
			hasFillRect = true
			fillCmd := cmd.(FillRectCommand)
			if fillCmd.Rect.Width() != 100 || fillCmd.Rect.Height() != 100 {
				t.Errorf("Clear rect = %f x %f, want 100 x 100",
					fillCmd.Rect.Width(), fillCmd.Rect.Height())
			}
			break
		}
	}
	if !hasFillRect {
		t.Error("Clear should produce FillRectCommand covering entire canvas")
	}
}

func TestRecorderClearWithColor(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.ClearWithColor(gg.Red)

	recording := rec.FinishRecording()

	hasFillRect := false
	for _, cmd := range recording.Commands() {
		if cmd.Type() == CmdFillRect {
			hasFillRect = true
			break
		}
	}
	if !hasFillRect {
		t.Error("ClearWithColor should produce FillRectCommand")
	}
}

func TestRecorderGetCurrentPoint(t *testing.T) {
	rec := NewRecorder(100, 100)

	// No current point initially
	_, _, ok := rec.GetCurrentPoint()
	if ok {
		t.Error("GetCurrentPoint should return false before any path operations")
	}

	// After MoveTo
	rec.MoveTo(50, 60)
	x, y, ok := rec.GetCurrentPoint()
	if !ok {
		t.Error("GetCurrentPoint should return true after MoveTo")
	}
	if x != 50 || y != 60 {
		t.Errorf("GetCurrentPoint = (%f, %f), want (50, 60)", x, y)
	}
}

func TestRecorderEmptyPathOperations(t *testing.T) {
	rec := NewRecorder(100, 100)

	// These should be no-ops with empty path
	rec.Fill()
	rec.Stroke()
	rec.FillPreserve()
	rec.StrokePreserve()
	rec.Clip()
	rec.ClipPreserve()

	recording := rec.FinishRecording()

	// Should have no drawing commands
	for _, cmd := range recording.Commands() {
		switch cmd.Type() {
		case CmdFillPath, CmdStrokePath, CmdSetClip:
			t.Errorf("empty path should not produce %v command", cmd.Type())
		}
	}
}

func TestRecorderTransformAffectsPath(t *testing.T) {
	rec := NewRecorder(100, 100)

	// Apply transform
	rec.Translate(10, 20)

	// Draw a point at origin
	rec.MoveTo(0, 0)
	rec.LineTo(1, 1)
	rec.Fill()

	// The path should have transformed coordinates
	recording := rec.FinishRecording()

	path := recording.Resources().GetPath(PathRef(0))
	if path == nil {
		t.Fatal("path not found in resources")
	}

	elements := path.Elements()
	if len(elements) < 1 {
		t.Fatal("path has no elements")
	}

	// First element should be MoveTo at transformed position
	moveTo, ok := elements[0].(gg.MoveTo)
	if !ok {
		t.Fatal("first element should be MoveTo")
	}
	if moveTo.Point.X != 10 || moveTo.Point.Y != 20 {
		t.Errorf("MoveTo = (%f, %f), want (10, 20)", moveTo.Point.X, moveTo.Point.Y)
	}
}

func TestRecorderMeasureString(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.SetFontSize(16)

	w, h := rec.MeasureString("Hello")

	// Without a font face, should return approximate dimensions
	if w <= 0 || h <= 0 {
		t.Errorf("MeasureString returned non-positive dimensions: %f x %f", w, h)
	}
}

func TestRecorderResourceCounting(t *testing.T) {
	rec := NewRecorder(100, 100)

	// Set different colors to add brushes
	rec.SetRGB(1, 0, 0)
	rec.DrawCircle(50, 50, 20)
	rec.Fill()

	rec.SetRGB(0, 1, 0)
	rec.DrawCircle(50, 50, 30)
	rec.Fill()

	recording := rec.FinishRecording()
	resources := recording.Resources()

	if resources.PathCount() != 2 {
		t.Errorf("PathCount = %d, want 2", resources.PathCount())
	}
	// Brushes include default and explicitly set ones
	if resources.BrushCount() < 2 {
		t.Errorf("BrushCount = %d, want >= 2", resources.BrushCount())
	}
}

func TestRecorderDrawNilImage(t *testing.T) {
	rec := NewRecorder(100, 100)

	// Drawing nil image should be a no-op
	rec.DrawImage(nil, 0, 0)
	rec.DrawImageAnchored(nil, 0, 0, 0.5, 0.5)
	rec.DrawImageScaled(nil, 0, 0, 100, 100)

	recording := rec.FinishRecording()

	for _, cmd := range recording.Commands() {
		if cmd.Type() == CmdDrawImage {
			t.Error("drawing nil image should not produce DrawImageCommand")
		}
	}
}

func TestRecorderInvertY(t *testing.T) {
	rec := NewRecorder(100, 200)

	rec.InvertY()

	// After InvertY, y=0 should map to height, y=height should map to 0
	x, y := rec.TransformPoint(0, 0)
	if y != 200 {
		t.Errorf("after InvertY, (0,0) -> y=%f, want 200", y)
	}
	if x != 0 {
		t.Errorf("after InvertY, (0,0) -> x=%f, want 0", x)
	}

	_, y = rec.TransformPoint(0, 200)
	if y != 0 {
		t.Errorf("after InvertY, (0,200) -> y=%f, want 0", y)
	}
}

func TestRecorderNestedSaveRestore(t *testing.T) {
	rec := NewRecorder(100, 100)

	rec.SetLineWidth(1)
	rec.Save()
	rec.SetLineWidth(2)
	rec.Save()
	rec.SetLineWidth(3)

	if rec.lineWidth != 3 {
		t.Errorf("nested level 2, lineWidth = %f, want 3", rec.lineWidth)
	}

	rec.Restore()
	if rec.lineWidth != 2 {
		t.Errorf("after first restore, lineWidth = %f, want 2", rec.lineWidth)
	}

	rec.Restore()
	if rec.lineWidth != 1 {
		t.Errorf("after second restore, lineWidth = %f, want 1", rec.lineWidth)
	}
}

func TestRecorderRestoreEmptyStack(t *testing.T) {
	rec := NewRecorder(100, 100)

	// Should not panic
	rec.Restore()
	rec.Pop()

	// State should be unchanged
	if rec.lineWidth != 1 {
		t.Errorf("lineWidth = %f after Restore on empty stack, want 1", rec.lineWidth)
	}
}

func TestRecordingPlayback(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.SetRGB(1, 0, 0)
	rec.DrawCircle(50, 50, 25)
	rec.Fill()

	recording := rec.FinishRecording()

	// Verify recording has expected properties
	if recording.Width() != 100 || recording.Height() != 100 {
		t.Errorf("recording dimensions = %d x %d, want 100 x 100",
			recording.Width(), recording.Height())
	}

	// Verify commands were recorded
	cmds := recording.Commands()
	if len(cmds) == 0 {
		t.Error("expected commands to be recorded")
	}

	// Verify resources were captured
	resources := recording.Resources()
	if resources.PathCount() == 0 {
		t.Error("expected paths to be captured")
	}
	if resources.BrushCount() == 0 {
		t.Error("expected brushes to be captured")
	}
}

// Benchmark tests
func BenchmarkRecorderDrawCircle(b *testing.B) {
	rec := NewRecorder(800, 600)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.DrawCircle(400, 300, 100)
		rec.Fill()
	}
}

func BenchmarkRecorderDrawRectangle(b *testing.B) {
	rec := NewRecorder(800, 600)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.DrawRectangle(100, 100, 600, 400)
		rec.Fill()
	}
}

func BenchmarkRecorderSaveRestore(b *testing.B) {
	rec := NewRecorder(800, 600)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Save()
		rec.SetRGB(float64(i%256)/255, 0, 0)
		rec.Translate(1, 1)
		rec.Restore()
	}
}

func TestRecorderWordWrap_NoFont(t *testing.T) {
	rec := NewRecorder(400, 200)

	lines := rec.WordWrap("Hello world", 100)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line without font, got %d", len(lines))
	}
	if lines[0] != "Hello world" {
		t.Errorf("expected original string, got %q", lines[0])
	}
}

func TestRecorderMeasureMultilineString_NoFont(t *testing.T) {
	rec := NewRecorder(400, 200)

	w, h := rec.MeasureMultilineString("Hello\nWorld", 1.0)
	if w != 0 || h != 0 {
		t.Errorf("no font: expected (0, 0), got (%f, %f)", w, h)
	}
}

func TestRecorderDrawStringWrapped_NoFont(t *testing.T) {
	rec := NewRecorder(400, 200)
	rec.SetFontSize(16)

	// Should not panic
	rec.DrawStringWrapped("Hello World", 0, 0, 0, 0, 200, 1.0, 0) // AlignLeft = 0
}

func TestRecorderDrawStringWrapped_ProducesCommands(t *testing.T) {
	rec := NewRecorder(400, 200)
	rec.SetFontSize(16)

	rec.DrawStringWrapped("Hello World", 0, 0, 0, 0, 200, 1.0, 0)

	recording := rec.FinishRecording()

	// Without a font face, the text won't wrap (single line), so we get 1 DrawText
	hasDrawText := false
	for _, cmd := range recording.Commands() {
		if cmd.Type() == CmdDrawText {
			hasDrawText = true
			break
		}
	}
	if !hasDrawText {
		t.Error("DrawStringWrapped should produce at least one DrawTextCommand")
	}
}

func BenchmarkRecorderComplexScene(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := NewRecorder(800, 600)

		// Background
		rec.SetRGB(1, 1, 1)
		rec.Clear()

		// Draw multiple shapes
		for j := 0; j < 100; j++ {
			rec.Save()
			rec.Translate(float64(j*8), float64(j*6))
			rec.SetRGB(float64(j%10)/10, float64(j%5)/5, 0.5)
			rec.DrawCircle(0, 0, 20)
			rec.Fill()
			rec.Restore()
		}

		_ = rec.FinishRecording()
	}
}
