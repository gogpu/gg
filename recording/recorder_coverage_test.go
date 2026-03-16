package recording

import (
	"image"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

func TestRecorderRotateAbout(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.RotateAbout(0.5, 50, 50)
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after RotateAbout")
	}
}

func TestRecorderShear(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.Shear(0.1, 0.2)
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after Shear")
	}
}

func TestRecorderTransformCoverage(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.Transform(Matrix{A: 1, B: 0, C: 0, D: 0, E: 1, F: 0})
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after Transform")
	}
}

func TestRecorderSetStrokeBrush(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.SetStrokeBrush(gg.Solid(gg.Red))
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after SetStrokeBrush")
	}
}

func TestRecorderSetFillRGBA(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.SetFillRGBA(1, 0, 0, 0.5)
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after SetFillRGBA")
	}
}

func TestRecorderSetStrokeRGBA(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.SetStrokeRGBA(0, 1, 0, 0.5)
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after SetStrokeRGBA")
	}
}

func TestRecorderSetLineCapGG(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.SetLineCapGG(gg.LineCapRound)
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after SetLineCapGG")
	}
}

func TestRecorderSetLineJoinGG(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.SetLineJoinGG(gg.LineJoinBevel)
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after SetLineJoinGG")
	}
}

func TestRecorderNewSubPath(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.MoveTo(10, 10)
	rec.LineTo(50, 10)
	rec.NewSubPath()
	rec.MoveTo(60, 60)
	rec.LineTo(90, 90)
	rec.Fill() // Generate at least one command
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after NewSubPath + Fill")
	}
}

func TestRecorderDrawEllipticalArc(t *testing.T) {
	rec := NewRecorder(200, 200)
	rec.DrawEllipticalArc(100, 100, 50, 30, 0, 3.14)
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after DrawEllipticalArc")
	}
}

func TestRecorderClipRoundRect(t *testing.T) {
	rec := NewRecorder(200, 200)
	rec.ClipRoundRect(20, 20, 160, 160, 15)
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after ClipRoundRect")
	}
}

func TestRecorderDrawStringAnchored(t *testing.T) {
	rec := NewRecorder(200, 200)
	rec.DrawStringAnchored("Hello", 100, 100, 0.5, 0.5)
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after DrawStringAnchored")
	}
}

func TestRecorderSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 1},
		{"hello\nworld", 2},
		{"hello\r\nworld", 2},
		{"hello\rworld", 2},
		{"a\nb\nc", 3},
	}
	for _, tt := range tests {
		got := recorderSplitLines(tt.input)
		if len(got) != tt.want {
			t.Errorf("splitLines(%q) = %d lines, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestRecorderWordWrap(t *testing.T) {
	rec := NewRecorder(200, 200)
	lines := rec.WordWrap("hello world this is a test", 80)
	if len(lines) == 0 {
		t.Error("WordWrap should return at least one line")
	}
}

func TestRecorderMeasureMultilineString(t *testing.T) {
	rec := NewRecorder(200, 200)
	w, h := rec.MeasureMultilineString("hello\nworld", 1.5)
	_ = w
	_ = h
	// Just verify no panic — without a font loaded, dimensions may be 0
}

func TestRecorderDrawImageAnchored(t *testing.T) {
	rec := NewRecorder(200, 200)
	// DrawImageAnchored with nil image should not panic
	rec.DrawImageAnchored(nil, 100, 100, 0.5, 0.5)
	_ = rec.FinishRecording()
}

func TestRecorderSetFont(t *testing.T) {
	rec := NewRecorder(200, 200)
	rec.SetFont(nil) // nil font should not panic
	r := rec.FinishRecording()
	_ = r
}

func TestRecordingPlaybackComplex(t *testing.T) {
	// Exercise many command types in a single recording to maximize Playback coverage
	rec := NewRecorder(200, 200)

	// Color commands
	rec.SetRGB(1, 0, 0)
	rec.SetRGBA(0, 1, 0, 0.5)
	rec.SetHexColor("#0000FF")

	// Transform commands
	rec.Push()
	rec.Translate(10, 10)
	rec.Scale(1.5, 1.5)
	rec.Rotate(0.1)
	rec.Pop()

	// Path drawing
	rec.DrawRectangle(10, 10, 50, 50)
	rec.Fill()

	rec.DrawCircle(100, 100, 30)
	rec.Stroke()

	rec.SetLineWidth(3.0)
	rec.SetLineCap(LineCapRound)
	rec.SetLineJoin(LineJoinBevel)
	rec.SetMiterLimit(2.0)
	rec.SetFillRule(FillRuleEvenOdd)

	rec.DrawRectangle(80, 80, 40, 40)
	rec.FillPreserve()
	rec.StrokePreserve()
	rec.ClearPath()

	r := rec.FinishRecording()
	backend := &playbackMockBackend{}
	err := r.Playback(backend)
	if err != nil {
		t.Fatalf("Playback failed: %v", err)
	}
}

func TestRecordingPlaybackCoverage(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.SetRGB(1, 0, 0)
	rec.DrawCircle(50, 50, 30)
	rec.Fill()
	r := rec.FinishRecording()

	// Create a mock backend to verify playback
	backend := &playbackMockBackend{}
	err := r.Playback(backend)
	if err != nil {
		t.Fatalf("Playback failed: %v", err)
	}
	if !backend.beginCalled {
		t.Error("expected Begin to be called")
	}
	if !backend.endCalled {
		t.Error("expected End to be called")
	}
}

// playbackMockBackend implements Backend for testing Playback.
type playbackMockBackend struct {
	beginCalled bool
	endCalled   bool
}

func (m *playbackMockBackend) Begin(_, _ int) error                                  { m.beginCalled = true; return nil }
func (m *playbackMockBackend) End() error                                            { m.endCalled = true; return nil }
func (m *playbackMockBackend) Save()                                                 {}
func (m *playbackMockBackend) Restore()                                              {}
func (m *playbackMockBackend) SetTransform(_ Matrix)                                 {}
func (m *playbackMockBackend) SetClip(_ *gg.Path, _ FillRule)                        {}
func (m *playbackMockBackend) ClearClip()                                            {}
func (m *playbackMockBackend) FillPath(_ *gg.Path, _ Brush, _ FillRule)              {}
func (m *playbackMockBackend) StrokePath(_ *gg.Path, _ Brush, _ Stroke)              {}
func (m *playbackMockBackend) FillRect(_ Rect, _ Brush)                              {}
func (m *playbackMockBackend) DrawImage(_ image.Image, _, _ Rect, _ ImageOptions)    {}
func (m *playbackMockBackend) DrawText(_ string, _, _ float64, _ text.Face, _ Brush) {}

// --- Additional recorder coverage tests ---

func TestRecorderDrawStringWrapped(t *testing.T) {
	rec := NewRecorder(200, 200)
	rec.DrawStringWrapped("Hello world this is a long string that should wrap", 10, 10, 0, 0, 80, 1.5, 0) // 0 = AlignLeft
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after DrawStringWrapped")
	}
}

func TestRecorderSetDash(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.SetDash(10, 5)
	rec.MoveTo(10, 50)
	rec.LineTo(90, 50)
	rec.Stroke()
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after SetDash")
	}
}

func TestRecorderSetDashClearDash(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.SetDash(10, 5)
	rec.ClearDash()
	rec.MoveTo(10, 50)
	rec.LineTo(90, 50)
	rec.Stroke()
	r := rec.FinishRecording()
	if len(r.Commands()) == 0 {
		t.Error("expected commands after ClearDash + Stroke")
	}
}

func TestRecorderSaveToFile(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.SetRGB(1, 0, 0)
	rec.DrawRectangle(10, 10, 80, 80)
	rec.Fill()
	r := rec.FinishRecording()

	// SaveToFile is only available on raster backend
	// Just verify recording was created
	if len(r.Commands()) == 0 {
		t.Error("expected commands")
	}
}

func TestRecorderMeasureStringCoverage(t *testing.T) {
	rec := NewRecorder(200, 200)
	w, h := rec.MeasureString("Hello World Test")
	// Without a real font, dimensions may be 0 but should not panic
	_ = w
	_ = h
}

func TestRecorderPlaybackSaveRestore(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.Push()
	rec.Translate(10, 10)
	rec.Pop()
	rec.SetRGB(1, 0, 0)
	rec.DrawRectangle(0, 0, 50, 50)
	rec.Fill()

	r := rec.FinishRecording()
	backend := &playbackMockBackend{}
	err := r.Playback(backend)
	if err != nil {
		t.Fatalf("Playback failed: %v", err)
	}
}

func TestRecorderPlaybackClipCommands(t *testing.T) {
	rec := NewRecorder(100, 100)
	// Use path-based clip
	rec.DrawRectangle(10, 10, 80, 80)
	rec.Clip()
	rec.SetRGB(1, 0, 0)
	rec.DrawRectangle(0, 0, 100, 100)
	rec.Fill()
	rec.ResetClip()

	r := rec.FinishRecording()
	backend := &playbackMockBackend{}
	err := r.Playback(backend)
	if err != nil {
		t.Fatalf("Playback failed: %v", err)
	}
}

func TestRecorderPlaybackImageCommand(t *testing.T) {
	rec := NewRecorder(100, 100)
	// Create a small test image
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	rec.DrawImage(img, 10, 10)

	r := rec.FinishRecording()
	backend := &playbackMockBackend{}
	err := r.Playback(backend)
	if err != nil {
		t.Fatalf("Playback failed: %v", err)
	}
}

func TestRecorderPlaybackDashCommands(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.SetDash(10, 5)
	rec.SetDashOffset(3)
	rec.SetRGB(0, 0, 1)
	rec.MoveTo(10, 50)
	rec.LineTo(90, 50)
	rec.Stroke()
	rec.ClearDash()

	r := rec.FinishRecording()
	backend := &playbackMockBackend{}
	err := r.Playback(backend)
	if err != nil {
		t.Fatalf("Playback failed: %v", err)
	}
}
