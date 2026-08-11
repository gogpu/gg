package recording

import (
	"image"
	"reflect"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

// strokeRectBackend records the geometry and style supplied by Playback. It
// deliberately does not render anything, so this test exercises the recording
// command-to-backend contract without depending on a particular renderer.
type strokeRectBackend struct {
	strokeCalls     int
	saveCalls       int
	restoreCalls    int
	transform       Matrix
	transformStack  []Matrix
	strokeTransform Matrix
	path            *gg.Path
	brush           Brush
	stroke          Stroke
}

func (b *strokeRectBackend) Begin(_, _ int) error {
	b.transform = Identity()
	return nil
}
func (b *strokeRectBackend) End() error { return nil }
func (b *strokeRectBackend) Save() {
	b.saveCalls++
	b.transformStack = append(b.transformStack, b.transform)
}
func (b *strokeRectBackend) Restore() {
	b.restoreCalls++
	if len(b.transformStack) == 0 {
		return
	}
	b.transform = b.transformStack[len(b.transformStack)-1]
	b.transformStack = b.transformStack[:len(b.transformStack)-1]
}
func (b *strokeRectBackend) SetTransform(transform Matrix) { b.transform = transform }
func (b *strokeRectBackend) SetClip(*gg.Path, FillRule) {
}
func (b *strokeRectBackend) ClearClip() {}
func (b *strokeRectBackend) FillPath(*gg.Path, Brush, FillRule) {
}
func (b *strokeRectBackend) StrokePath(path *gg.Path, brush Brush, stroke Stroke) {
	b.strokeCalls++
	b.strokeTransform = b.transform
	b.path = path.Clone()
	b.brush = brush
	b.stroke = stroke.Clone()
}
func (b *strokeRectBackend) FillRect(Rect, Brush) {}
func (b *strokeRectBackend) DrawImage(image.Image, Rect, Rect, ImageOptions) {
}
func (b *strokeRectBackend) DrawText(string, float64, float64, text.Face, Brush) {
}

func TestRecordingPlaybackStrokeRectangle(t *testing.T) {
	rec := NewRecorder(100, 100)
	rec.SetStrokeRGB(0, 0, 1)
	rec.SetLineWidth(3)
	rec.SetLineCap(LineCapRound)
	rec.SetLineJoin(LineJoinBevel)
	rec.SetMiterLimit(7)
	rec.SetDash(4, 2)
	rec.Translate(10, 15)
	rec.StrokeRectangle(5, 6, 20, 30)

	backend := &strokeRectBackend{}
	if err := rec.FinishRecording().Playback(backend); err != nil {
		t.Fatalf("Playback failed: %v", err)
	}
	if backend.strokeCalls != 1 {
		t.Fatalf("StrokePath calls = %d, want 1", backend.strokeCalls)
	}
	if backend.saveCalls != 1 || backend.restoreCalls != 1 {
		t.Fatalf("backend state calls = Save %d, Restore %d; want one each", backend.saveCalls, backend.restoreCalls)
	}
	if backend.strokeTransform != Identity() {
		t.Fatalf("stroke transform = %#v, want identity", backend.strokeTransform)
	}
	if got, want := backend.transform, Translate(10, 15); got != want {
		t.Fatalf("transform after stroke = %#v, want restored %#v", got, want)
	}

	wantVerbs := []gg.PathVerb{gg.MoveTo, gg.LineTo, gg.LineTo, gg.LineTo, gg.Close}
	if got := backend.path.Verbs(); !reflect.DeepEqual(got, wantVerbs) {
		t.Fatalf("rectangle verbs = %v, want %v", got, wantVerbs)
	}
	wantCoords := []float64{15, 21, 35, 21, 35, 51, 15, 51}
	if got := backend.path.Coords(); !reflect.DeepEqual(got, wantCoords) {
		t.Fatalf("rectangle coordinates = %v, want %v", got, wantCoords)
	}
	if got, want := backend.brush, NewSolidBrush(gg.Blue); got != want {
		t.Fatalf("brush = %#v, want %#v", got, want)
	}
	wantStroke := Stroke{
		Width:       3,
		Cap:         LineCapRound,
		Join:        LineJoinBevel,
		MiterLimit:  7,
		DashPattern: []float64{4, 2},
		DashOffset:  0,
	}
	if !reflect.DeepEqual(backend.stroke, wantStroke) {
		t.Fatalf("stroke = %#v, want %#v", backend.stroke, wantStroke)
	}
}

func TestRecordingPlaybackTransformedStrokeRectangleRestoresBackendState(t *testing.T) {
	transform := Matrix{A: 0, B: -1, C: 100, D: 1, E: 0}
	rec := NewRecorder(100, 100)
	rec.SetTransform(transform)
	rec.StrokeRectangle(10, 20, 30, 40)

	backend := &strokeRectBackend{}
	if err := rec.FinishRecording().Playback(backend); err != nil {
		t.Fatalf("Playback failed: %v", err)
	}
	if backend.strokeCalls != 1 {
		t.Fatalf("StrokePath calls = %d, want 1", backend.strokeCalls)
	}
	if backend.saveCalls != 1 || backend.restoreCalls != 1 {
		t.Fatalf("backend state calls = Save %d, Restore %d; want one each", backend.saveCalls, backend.restoreCalls)
	}
	if backend.strokeTransform != Identity() {
		t.Fatalf("stroke transform = %#v, want identity", backend.strokeTransform)
	}
	if backend.transform != transform {
		t.Fatalf("transform after stroke = %#v, want restored %#v", backend.transform, transform)
	}
}

func assertStrokePathCommand(t *testing.T, r *Recording, drawing Command, wantStroke Stroke, wantCoords []float64) {
	t.Helper()

	cmd, ok := drawing.(StrokePathCommand)
	if !ok {
		t.Fatalf("drawing command = %T, want StrokePathCommand", drawing)
	}
	if cmd.Brush != BrushRef(1) {
		t.Errorf("stroke brush ref = %d, want 1", cmd.Brush)
	}
	if !reflect.DeepEqual(cmd.Stroke, wantStroke) {
		t.Errorf("stroke style = %#v, want %#v", cmd.Stroke, wantStroke)
	}
	if got, want := r.Resources().PathCount(), 1; got != want {
		t.Fatalf("path count = %d, want %d", got, want)
	}
	path := r.Resources().GetPath(cmd.Path)
	wantVerbs := []gg.PathVerb{gg.MoveTo, gg.LineTo, gg.LineTo, gg.LineTo, gg.Close}
	if got := path.Verbs(); !reflect.DeepEqual(got, wantVerbs) {
		t.Errorf("path verbs = %v, want %v", got, wantVerbs)
	}
	if got := path.Coords(); !reflect.DeepEqual(got, wantCoords) {
		t.Errorf("path coordinates = %v, want %v", got, wantCoords)
	}

	var drawingIndex = -1
	for i, recorded := range r.Commands() {
		if _, ok := recorded.(StrokePathCommand); ok {
			drawingIndex = i
			break
		}
	}
	if drawingIndex < 2 || drawingIndex+1 >= len(r.Commands()) {
		t.Fatalf("stroke path at command %d is missing its state wrapper", drawingIndex)
	}
	if _, ok := r.Commands()[drawingIndex-2].(SaveCommand); !ok {
		t.Errorf("command before identity transform = %T, want SaveCommand", r.Commands()[drawingIndex-2])
	}
	transform, ok := r.Commands()[drawingIndex-1].(SetTransformCommand)
	if !ok || transform.Matrix != Identity() {
		t.Errorf("command before stroke path = %#v, want identity SetTransformCommand", r.Commands()[drawingIndex-1])
	}
	if _, ok := r.Commands()[drawingIndex+1].(RestoreCommand); !ok {
		t.Errorf("command after stroke path = %T, want RestoreCommand", r.Commands()[drawingIndex+1])
	}
}

func assertStrokeRectCommand(t *testing.T, r *Recording, drawing Command, wantStroke Stroke, wantRect Rect) {
	t.Helper()

	cmd, ok := drawing.(StrokeRectCommand)
	if !ok {
		t.Fatalf("drawing command = %T, want StrokeRectCommand", drawing)
	}
	if cmd.Brush != BrushRef(1) {
		t.Errorf("stroke brush ref = %d, want 1", cmd.Brush)
	}
	if cmd.Rect != wantRect {
		t.Errorf("rectangle = %#v, want %#v", cmd.Rect, wantRect)
	}
	if !reflect.DeepEqual(cmd.Stroke, wantStroke) {
		t.Errorf("stroke style = %#v, want %#v", cmd.Stroke, wantStroke)
	}
	if got := r.Resources().PathCount(); got != 0 {
		t.Errorf("path count = %d, want 0", got)
	}
}

func TestRecorderStrokeRectangleCommandEncoding(t *testing.T) {
	wantBrush := NewSolidBrush(gg.Blue)
	wantStroke := Stroke{
		Width:       2.5,
		Cap:         LineCapRound,
		Join:        LineJoinBevel,
		MiterLimit:  6,
		DashPattern: []float64{4, 3},
		DashOffset:  1.5,
	}
	tests := []struct {
		name         string
		x, y, w, h   float64
		transform    Matrix
		setTransform bool
		wantPath     bool
		wantRect     Rect
		wantCoords   []float64
	}{
		{
			name:     "positive dimensions use optimized command",
			x:        10,
			y:        20,
			w:        30,
			h:        40,
			wantRect: NewRect(10, 20, 30, 40),
		},
		{
			name:     "zero width stays optimized",
			x:        10,
			y:        20,
			w:        0,
			h:        40,
			wantRect: NewRect(10, 20, 0, 40),
		},
		{
			name:      "translation stays optimized",
			x:         10,
			y:         20,
			w:         30,
			h:         40,
			transform: Translate(5, 7),
			wantRect:  NewRect(15, 27, 30, 40),
		},
		{
			name:       "positive axis scale preserves stroke metrics",
			x:          10,
			y:          20,
			w:          30,
			h:          40,
			transform:  Matrix{A: 2, E: 0.5, C: 5, F: 7},
			wantPath:   true,
			wantCoords: []float64{25, 17, 85, 17, 85, 37, 25, 37},
		},
		{
			name:         "zero scale keeps unscaled stroke metrics",
			x:            10,
			y:            20,
			w:            30,
			h:            40,
			transform:    Matrix{C: 90, F: 90},
			setTransform: true,
			wantPath:     true,
			wantCoords:   []float64{90, 90, 90, 90, 90, 90, 90, 90},
		},
		{
			name:       "negative width preserves path direction",
			x:          40,
			y:          20,
			w:          -30,
			h:          40,
			wantPath:   true,
			wantCoords: []float64{40, 20, 10, 20, 10, 60, 40, 60},
		},
		{
			name:       "negative height preserves path direction",
			x:          10,
			y:          60,
			w:          30,
			h:          -40,
			wantPath:   true,
			wantCoords: []float64{10, 60, 40, 60, 40, 20, 10, 20},
		},
		{
			name:       "negative width and height preserve path direction",
			x:          40,
			y:          60,
			w:          -30,
			h:          -40,
			wantPath:   true,
			wantCoords: []float64{40, 60, 10, 60, 10, 20, 40, 20},
		},
		{
			name:       "rotation preserves all transformed corners",
			x:          10,
			y:          20,
			w:          30,
			h:          40,
			transform:  Matrix{A: 0, B: -1, C: 100, D: 1, E: 0},
			wantPath:   true,
			wantCoords: []float64{80, 10, 80, 40, 40, 40, 40, 10},
		},
		{
			name:       "shear preserves all transformed corners",
			x:          10,
			y:          20,
			w:          30,
			h:          40,
			transform:  Matrix{A: 1, B: 0.5, D: 0.25, E: 1},
			wantPath:   true,
			wantCoords: []float64{20, 22.5, 50, 30, 70, 70, 40, 62.5},
		},
		{
			name:       "reflection preserves path direction",
			x:          10,
			y:          20,
			w:          30,
			h:          40,
			transform:  Matrix{A: -1, C: 100, E: 1},
			wantPath:   true,
			wantCoords: []float64{90, 20, 60, 20, 60, 60, 90, 60},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := NewRecorder(100, 100)
			rec.SetStrokeStyle(wantBrush)
			rec.SetLineWidth(wantStroke.Width)
			rec.SetLineCap(wantStroke.Cap)
			rec.SetLineJoin(wantStroke.Join)
			rec.SetMiterLimit(wantStroke.MiterLimit)
			rec.SetDash(wantStroke.DashPattern...)
			rec.SetDashOffset(wantStroke.DashOffset)
			transform := tt.transform
			if transform == (Matrix{}) && !tt.setTransform {
				transform = Identity()
			} else {
				rec.SetTransform(transform)
			}
			rec.StrokeRectangle(tt.x, tt.y, tt.w, tt.h)

			r := rec.FinishRecording()
			if got, want := r.Resources().BrushCount(), 2; got != want {
				t.Fatalf("brush count = %d, want %d", got, want)
			}
			if got := r.Resources().GetBrush(BrushRef(1)); got != wantBrush {
				t.Fatalf("stroke brush resource = %#v, want %#v", got, wantBrush)
			}

			var drawing Command
			for _, cmd := range r.Commands() {
				switch cmd.(type) {
				case StrokeRectCommand, StrokePathCommand:
					if drawing != nil {
						t.Fatal("recorded more than one rectangle drawing command")
					}
					drawing = cmd
				}
			}
			if drawing == nil {
				t.Fatal("missing rectangle drawing command")
			}

			wantCommandStroke := wantStroke.Clone()
			transformScale := transform.ScaleFactor()
			if transformScale <= 0 {
				transformScale = 1
			}
			wantCommandStroke.Width *= transformScale
			if transformScale > 1 {
				for i := range wantCommandStroke.DashPattern {
					wantCommandStroke.DashPattern[i] *= transformScale
				}
				wantCommandStroke.DashOffset *= transformScale
			}

			if tt.wantPath {
				assertStrokePathCommand(t, r, drawing, wantCommandStroke, tt.wantCoords)
				return
			}

			assertStrokeRectCommand(t, r, drawing, wantCommandStroke, tt.wantRect)
		})
	}
}
