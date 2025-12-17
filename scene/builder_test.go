package scene

import (
	"math"
	"testing"

	"github.com/gogpu/gg"
)

func TestNewSceneBuilder(t *testing.T) {
	builder := NewSceneBuilder()

	if builder == nil {
		t.Fatal("NewSceneBuilder() returned nil")
	}
	if builder.scene == nil {
		t.Error("builder.scene should not be nil")
	}
	if builder.Scene().IsEmpty() == false {
		t.Error("new builder should have empty scene")
	}
	if !builder.transform.IsIdentity() {
		t.Error("new builder should have identity transform")
	}
}

func TestNewSceneBuilderFrom(t *testing.T) {
	// Test with nil scene
	builder := NewSceneBuilderFrom(nil)
	if builder.scene == nil {
		t.Error("NewSceneBuilderFrom(nil) should create a scene")
	}

	// Test with existing scene
	scene := NewScene()
	scene.Translate(10, 20)
	builder = NewSceneBuilderFrom(scene)

	if builder.scene != scene {
		t.Error("builder should wrap the provided scene")
	}
	if builder.transform != scene.Transform() {
		t.Error("builder should inherit scene's transform")
	}
}

func TestSceneBuilderFill(t *testing.T) {
	builder := NewSceneBuilder()

	rect := NewRectShape(10, 20, 100, 50)
	brush := SolidBrush(gg.Red)

	result := builder.Fill(rect, brush)

	// Should return same builder for chaining
	if result != builder {
		t.Error("Fill() should return the same builder")
	}

	// Scene should have content
	if builder.Scene().IsEmpty() {
		t.Error("scene should not be empty after Fill")
	}

	// Check bounds
	bounds := builder.Scene().Bounds()
	if bounds.MinX != 10 || bounds.MinY != 20 || bounds.MaxX != 110 || bounds.MaxY != 70 {
		t.Errorf("bounds = %+v, want (10,20)-(110,70)", bounds)
	}
}

func TestSceneBuilderFillWith(t *testing.T) {
	builder := NewSceneBuilder()

	rect := NewRectShape(0, 0, 100, 100)
	brush := SolidBrush(gg.Blue)

	result := builder.FillWith(rect, brush, FillEvenOdd)

	if result != builder {
		t.Error("FillWith() should return the same builder")
	}

	// Should have encoded fill command
	enc := builder.Scene().Encoding()
	hasFill := false
	for _, tag := range enc.Tags() {
		if tag == TagFill {
			hasFill = true
			break
		}
	}
	if !hasFill {
		t.Error("encoding should contain fill command")
	}
}

func TestSceneBuilderStroke(t *testing.T) {
	builder := NewSceneBuilder()

	line := NewLineShape(0, 0, 100, 100)
	brush := SolidBrush(gg.Green)

	result := builder.Stroke(line, brush, 4.0)

	if result != builder {
		t.Error("Stroke() should return the same builder")
	}

	// Scene should have content
	if builder.Scene().IsEmpty() {
		t.Error("scene should not be empty after Stroke")
	}

	// Bounds should include stroke width expansion
	bounds := builder.Scene().Bounds()
	if bounds.MinX > -2 || bounds.MinY > -2 || bounds.MaxX < 102 || bounds.MaxY < 102 {
		t.Errorf("bounds = %+v, expected stroke width expansion", bounds)
	}
}

func TestSceneBuilderStrokeWith(t *testing.T) {
	builder := NewSceneBuilder()

	circle := NewCircleShape(50, 50, 25)
	brush := SolidBrush(gg.Black)
	style := &StrokeStyle{
		Width:      2.0,
		MiterLimit: 4.0,
		Cap:        LineCapRound,
		Join:       LineJoinRound,
	}

	result := builder.StrokeWith(circle, brush, style)

	if result != builder {
		t.Error("StrokeWith() should return the same builder")
	}

	if builder.Scene().IsEmpty() {
		t.Error("scene should not be empty after StrokeWith")
	}
}

func TestSceneBuilderImage(t *testing.T) {
	builder := NewSceneBuilder()

	img := NewImage(100, 50)
	rect := Rect{MinX: 10, MinY: 20, MaxX: 210, MaxY: 120}

	result := builder.Image(img, rect)

	if result != builder {
		t.Error("Image() should return the same builder")
	}

	// Check that image was registered
	images := builder.Scene().Images()
	if len(images) != 1 {
		t.Errorf("expected 1 image, got %d", len(images))
	}
}

func TestSceneBuilderImageNil(t *testing.T) {
	builder := NewSceneBuilder()

	// Nil image should not crash
	result := builder.Image(nil, Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100})

	if result != builder {
		t.Error("Image(nil) should return the same builder")
	}
	if !builder.Scene().IsEmpty() {
		t.Error("nil image should not add content")
	}
}

func TestSceneBuilderImageEmptyRect(t *testing.T) {
	builder := NewSceneBuilder()

	img := NewImage(100, 50)

	// Empty rect should not draw
	result := builder.Image(img, EmptyRect())

	if result != builder {
		t.Error("Image(empty rect) should return the same builder")
	}
	if !builder.Scene().IsEmpty() {
		t.Error("empty rect should not add content")
	}
}

func TestSceneBuilderTransform(t *testing.T) {
	builder := NewSceneBuilder()

	transform := TranslateAffine(100, 200)
	result := builder.Transform(transform)

	if result != builder {
		t.Error("Transform() should return the same builder")
	}
	if builder.CurrentTransform() != transform {
		t.Error("transform should be set")
	}
}

func TestSceneBuilderTranslate(t *testing.T) {
	builder := NewSceneBuilder()

	result := builder.Translate(10, 20)

	if result != builder {
		t.Error("Translate() should return the same builder")
	}

	transform := builder.CurrentTransform()
	if transform.C != 10 || transform.F != 20 {
		t.Errorf("transform = %+v, want translate(10,20)", transform)
	}

	// Translations should accumulate
	builder.Translate(5, 10)
	transform = builder.CurrentTransform()
	if transform.C != 15 || transform.F != 30 {
		t.Errorf("accumulated transform = %+v, want translate(15,30)", transform)
	}
}

func TestSceneBuilderScale(t *testing.T) {
	builder := NewSceneBuilder()

	result := builder.Scale(2, 3)

	if result != builder {
		t.Error("Scale() should return the same builder")
	}

	transform := builder.CurrentTransform()
	if transform.A != 2 || transform.E != 3 {
		t.Errorf("transform = %+v, want scale(2,3)", transform)
	}
}

func TestSceneBuilderRotate(t *testing.T) {
	builder := NewSceneBuilder()

	result := builder.Rotate(float32(math.Pi / 2))

	if result != builder {
		t.Error("Rotate() should return the same builder")
	}

	transform := builder.CurrentTransform()
	// After 90 degree rotation: A ~0, B ~-1, D ~1, E ~0
	if math.Abs(float64(transform.A)) > 0.01 || math.Abs(float64(transform.B+1)) > 0.01 {
		t.Errorf("transform = %+v, expected 90 degree rotation", transform)
	}
}

func TestSceneBuilderResetTransform(t *testing.T) {
	builder := NewSceneBuilder()

	builder.Translate(10, 20).Scale(2, 2).Rotate(1.0)

	if builder.CurrentTransform().IsIdentity() {
		t.Error("transform should not be identity before reset")
	}

	result := builder.ResetTransform()

	if result != builder {
		t.Error("ResetTransform() should return the same builder")
	}
	if !builder.CurrentTransform().IsIdentity() {
		t.Error("transform should be identity after reset")
	}
}

func TestSceneBuilderLayer(t *testing.T) {
	builder := NewSceneBuilder()

	callbackCalled := false
	result := builder.Layer(BlendMultiply, 0.8, nil, func(b *SceneBuilder) {
		callbackCalled = true
		if b != builder {
			t.Error("callback should receive the same builder")
		}
		b.Fill(NewRectShape(0, 0, 100, 100), SolidBrush(gg.Red))
	})

	if result != builder {
		t.Error("Layer() should return the same builder")
	}
	if !callbackCalled {
		t.Error("callback should be called")
	}

	// Scene should have layer commands
	enc := builder.Scene().Encoding()
	hasPushLayer := false
	hasPopLayer := false
	for _, tag := range enc.Tags() {
		if tag == TagPushLayer {
			hasPushLayer = true
		}
		if tag == TagPopLayer {
			hasPopLayer = true
		}
	}
	if !hasPushLayer || !hasPopLayer {
		t.Error("encoding should contain layer commands")
	}
}

func TestSceneBuilderLayerWithClip(t *testing.T) {
	builder := NewSceneBuilder()

	clip := NewCircleShape(50, 50, 25)
	builder.Layer(BlendNormal, 1.0, clip, func(b *SceneBuilder) {
		b.Fill(NewRectShape(0, 0, 100, 100), SolidBrush(gg.Blue))
	})

	// Should have clip commands
	enc := builder.Scene().Encoding()
	hasBeginClip := false
	hasEndClip := false
	for _, tag := range enc.Tags() {
		if tag == TagBeginClip {
			hasBeginClip = true
		}
		if tag == TagEndClip {
			hasEndClip = true
		}
	}
	if !hasBeginClip || !hasEndClip {
		t.Error("encoding should contain clip commands")
	}
}

func TestSceneBuilderLayerNilCallback(t *testing.T) {
	builder := NewSceneBuilder()

	result := builder.Layer(BlendNormal, 1.0, nil, nil)

	if result != builder {
		t.Error("Layer(nil callback) should return the same builder")
	}
	// Should not add any layer commands
	if !builder.Scene().IsEmpty() {
		t.Error("nil callback should not add content")
	}
}

func TestSceneBuilderLayerTransformRestored(t *testing.T) {
	builder := NewSceneBuilder()

	builder.Translate(100, 100)
	originalTransform := builder.CurrentTransform()

	builder.Layer(BlendNormal, 1.0, nil, func(b *SceneBuilder) {
		b.Translate(50, 50) // Modify transform inside layer
	})

	if builder.CurrentTransform() != originalTransform {
		t.Error("transform should be restored after Layer callback")
	}
}

func TestSceneBuilderClip(t *testing.T) {
	builder := NewSceneBuilder()

	clip := NewRectShape(10, 10, 80, 80)
	callbackCalled := false

	result := builder.Clip(clip, func(b *SceneBuilder) {
		callbackCalled = true
		b.Fill(NewRectShape(0, 0, 100, 100), SolidBrush(gg.Red))
	})

	if result != builder {
		t.Error("Clip() should return the same builder")
	}
	if !callbackCalled {
		t.Error("callback should be called")
	}

	// Should have clip commands
	enc := builder.Scene().Encoding()
	hasBeginClip := false
	hasEndClip := false
	for _, tag := range enc.Tags() {
		if tag == TagBeginClip {
			hasBeginClip = true
		}
		if tag == TagEndClip {
			hasEndClip = true
		}
	}
	if !hasBeginClip || !hasEndClip {
		t.Error("encoding should contain clip commands")
	}
}

func TestSceneBuilderClipNilShape(t *testing.T) {
	builder := NewSceneBuilder()

	result := builder.Clip(nil, func(b *SceneBuilder) {
		b.Fill(NewRectShape(0, 0, 100, 100), SolidBrush(gg.Red))
	})

	if result != builder {
		t.Error("Clip(nil shape) should return the same builder")
	}
	// Should not add content since shape is nil
	if !builder.Scene().IsEmpty() {
		t.Error("nil shape should not add content")
	}
}

func TestSceneBuilderClipNilCallback(t *testing.T) {
	builder := NewSceneBuilder()

	result := builder.Clip(NewRectShape(0, 0, 100, 100), nil)

	if result != builder {
		t.Error("Clip(nil callback) should return the same builder")
	}
	if !builder.Scene().IsEmpty() {
		t.Error("nil callback should not add content")
	}
}

func TestSceneBuilderClipTransformRestored(t *testing.T) {
	builder := NewSceneBuilder()

	builder.Translate(100, 100)
	originalTransform := builder.CurrentTransform()

	builder.Clip(NewRectShape(0, 0, 50, 50), func(b *SceneBuilder) {
		b.Translate(50, 50) // Modify transform inside clip
	})

	if builder.CurrentTransform() != originalTransform {
		t.Error("transform should be restored after Clip callback")
	}
}

func TestSceneBuilderGroup(t *testing.T) {
	builder := NewSceneBuilder()

	callbackCalled := false
	result := builder.Group(func(b *SceneBuilder) {
		callbackCalled = true
		if b != builder {
			t.Error("callback should receive the same builder")
		}
		b.Fill(NewRectShape(0, 0, 100, 100), SolidBrush(gg.Red))
		b.Fill(NewCircleShape(50, 50, 25), SolidBrush(gg.Blue))
	})

	if result != builder {
		t.Error("Group() should return the same builder")
	}
	if !callbackCalled {
		t.Error("callback should be called")
	}

	// Should NOT have layer commands (just grouping)
	enc := builder.Scene().Encoding()
	for _, tag := range enc.Tags() {
		if tag == TagPushLayer || tag == TagPopLayer {
			t.Error("Group should not add layer commands")
		}
	}
}

func TestSceneBuilderGroupNilCallback(t *testing.T) {
	builder := NewSceneBuilder()

	result := builder.Group(nil)

	if result != builder {
		t.Error("Group(nil callback) should return the same builder")
	}
}

func TestSceneBuilderGroupTransformRestored(t *testing.T) {
	builder := NewSceneBuilder()

	builder.Translate(100, 100)
	originalTransform := builder.CurrentTransform()

	builder.Group(func(b *SceneBuilder) {
		b.Translate(50, 50) // Modify transform inside group
	})

	if builder.CurrentTransform() != originalTransform {
		t.Error("transform should be restored after Group callback")
	}
}

func TestSceneBuilderWithTransform(t *testing.T) {
	builder := NewSceneBuilder()

	builder.Translate(100, 100)
	originalTransform := builder.CurrentTransform()

	callbackCalled := false
	result := builder.WithTransform(TranslateAffine(50, 50), func(b *SceneBuilder) {
		callbackCalled = true
		// Transform should be combined
		expectedC := float32(150) // 100 + 50
		expectedF := float32(150)
		if b.CurrentTransform().C != expectedC || b.CurrentTransform().F != expectedF {
			t.Errorf("transform inside = %+v, want translate(150,150)", b.CurrentTransform())
		}
	})

	if result != builder {
		t.Error("WithTransform() should return the same builder")
	}
	if !callbackCalled {
		t.Error("callback should be called")
	}
	if builder.CurrentTransform() != originalTransform {
		t.Error("transform should be restored after WithTransform callback")
	}
}

func TestSceneBuilderWithTransformNilCallback(t *testing.T) {
	builder := NewSceneBuilder()

	result := builder.WithTransform(TranslateAffine(10, 20), nil)

	if result != builder {
		t.Error("WithTransform(nil callback) should return the same builder")
	}
}

func TestSceneBuilderBuild(t *testing.T) {
	builder := NewSceneBuilder()

	builder.Fill(NewRectShape(0, 0, 100, 100), SolidBrush(gg.Red))
	builder.Translate(50, 50)

	scene := builder.Build()

	if scene == nil {
		t.Fatal("Build() returned nil")
	}
	if scene.IsEmpty() {
		t.Error("built scene should not be empty")
	}

	// Builder should be reset
	if !builder.Scene().IsEmpty() {
		t.Error("builder's scene should be empty after Build")
	}
	if !builder.CurrentTransform().IsIdentity() {
		t.Error("builder's transform should be identity after Build")
	}

	// Built scene should still have content
	if scene.IsEmpty() {
		t.Error("built scene should retain content")
	}
}

func TestSceneBuilderReset(t *testing.T) {
	builder := NewSceneBuilder()

	builder.Fill(NewRectShape(0, 0, 100, 100), SolidBrush(gg.Red))
	builder.Translate(50, 50)

	result := builder.Reset()

	if result != builder {
		t.Error("Reset() should return the same builder")
	}
	if !builder.Scene().IsEmpty() {
		t.Error("scene should be empty after Reset")
	}
	if !builder.CurrentTransform().IsIdentity() {
		t.Error("transform should be identity after Reset")
	}
}

func TestSceneBuilderScene(t *testing.T) {
	builder := NewSceneBuilder()

	scene1 := builder.Scene()
	scene2 := builder.Scene()

	if scene1 != scene2 {
		t.Error("Scene() should return the same scene")
	}

	builder.Fill(NewRectShape(0, 0, 100, 100), SolidBrush(gg.Red))

	// Scene should reflect changes
	if builder.Scene().IsEmpty() {
		t.Error("Scene() should show current content")
	}
}

func TestSceneBuilderCurrentTransform(t *testing.T) {
	builder := NewSceneBuilder()

	if !builder.CurrentTransform().IsIdentity() {
		t.Error("initial transform should be identity")
	}

	builder.Translate(10, 20)
	transform := builder.CurrentTransform()

	if transform.C != 10 || transform.F != 20 {
		t.Errorf("CurrentTransform() = %+v, want translate(10,20)", transform)
	}
}

// Convenience method tests

func TestSceneBuilderFillRect(t *testing.T) {
	builder := NewSceneBuilder()

	result := builder.FillRect(10, 20, 100, 50, SolidBrush(gg.Red))

	if result != builder {
		t.Error("FillRect() should return the same builder")
	}

	bounds := builder.Scene().Bounds()
	if bounds.MinX != 10 || bounds.MinY != 20 || bounds.MaxX != 110 || bounds.MaxY != 70 {
		t.Errorf("bounds = %+v, want (10,20)-(110,70)", bounds)
	}
}

func TestSceneBuilderStrokeRect(t *testing.T) {
	builder := NewSceneBuilder()

	result := builder.StrokeRect(0, 0, 100, 100, SolidBrush(gg.Black), 2.0)

	if result != builder {
		t.Error("StrokeRect() should return the same builder")
	}
	if builder.Scene().IsEmpty() {
		t.Error("scene should not be empty")
	}
}

func TestSceneBuilderFillCircle(t *testing.T) {
	builder := NewSceneBuilder()

	result := builder.FillCircle(50, 50, 25, SolidBrush(gg.Blue))

	if result != builder {
		t.Error("FillCircle() should return the same builder")
	}

	bounds := builder.Scene().Bounds()
	if bounds.MinX != 25 || bounds.MinY != 25 || bounds.MaxX != 75 || bounds.MaxY != 75 {
		t.Errorf("bounds = %+v, want (25,25)-(75,75)", bounds)
	}
}

func TestSceneBuilderStrokeCircle(t *testing.T) {
	builder := NewSceneBuilder()

	result := builder.StrokeCircle(50, 50, 25, SolidBrush(gg.Black), 2.0)

	if result != builder {
		t.Error("StrokeCircle() should return the same builder")
	}
	if builder.Scene().IsEmpty() {
		t.Error("scene should not be empty")
	}
}

func TestSceneBuilderDrawLine(t *testing.T) {
	builder := NewSceneBuilder()

	result := builder.DrawLine(0, 0, 100, 100, SolidBrush(gg.Green), 1.0)

	if result != builder {
		t.Error("DrawLine() should return the same builder")
	}
	if builder.Scene().IsEmpty() {
		t.Error("scene should not be empty")
	}
}

func TestSceneBuilderFillPath(t *testing.T) {
	builder := NewSceneBuilder()

	path := NewPath().
		MoveTo(0, 0).
		LineTo(100, 0).
		LineTo(50, 100).
		Close()

	result := builder.FillPath(path, SolidBrush(gg.Red))

	if result != builder {
		t.Error("FillPath() should return the same builder")
	}
	if builder.Scene().IsEmpty() {
		t.Error("scene should not be empty")
	}
}

func TestSceneBuilderStrokePath(t *testing.T) {
	builder := NewSceneBuilder()

	path := NewPath().
		MoveTo(0, 0).
		LineTo(100, 0).
		LineTo(50, 100).
		Close()

	result := builder.StrokePath(path, SolidBrush(gg.Black), 2.0)

	if result != builder {
		t.Error("StrokePath() should return the same builder")
	}
	if builder.Scene().IsEmpty() {
		t.Error("scene should not be empty")
	}
}

// Integration tests - test fluent chaining patterns

func TestSceneBuilderFluentChaining(t *testing.T) {
	// Test the example from the task requirements
	scene := NewSceneBuilder().
		Fill(NewRectShape(0, 0, 800, 600), SolidBrush(gg.White)).
		Translate(100, 100).
		Layer(BlendMultiply, 0.8, nil, func(b *SceneBuilder) {
			b.Fill(NewCircleShape(0, 0, 50), SolidBrush(gg.Red)).
				Stroke(NewCircleShape(0, 0, 50), SolidBrush(gg.Black), 2)
		}).
		Build()

	if scene == nil {
		t.Fatal("Build() returned nil")
	}
	if scene.IsEmpty() {
		t.Error("built scene should not be empty")
	}

	// Should have multiple shapes
	enc := scene.Encoding()
	fillCount := 0
	for _, tag := range enc.Tags() {
		if tag == TagFill {
			fillCount++
		}
	}
	// Background + circle = 2 fills
	if fillCount < 2 {
		t.Errorf("expected at least 2 fills, got %d", fillCount)
	}
}

func TestSceneBuilderNestedLayers(t *testing.T) {
	scene := NewSceneBuilder().
		Layer(BlendNormal, 1.0, nil, func(b *SceneBuilder) {
			b.Fill(NewRectShape(0, 0, 100, 100), SolidBrush(gg.Red))
			b.Layer(BlendMultiply, 0.5, nil, func(b2 *SceneBuilder) {
				b2.Fill(NewCircleShape(50, 50, 25), SolidBrush(gg.Blue))
			})
		}).
		Build()

	if scene == nil {
		t.Fatal("Build() returned nil")
	}

	// Should have nested layer commands
	enc := scene.Encoding()
	pushCount := 0
	popCount := 0
	for _, tag := range enc.Tags() {
		if tag == TagPushLayer {
			pushCount++
		}
		if tag == TagPopLayer {
			popCount++
		}
	}
	if pushCount != 2 || popCount != 2 {
		t.Errorf("expected 2 push/pop pairs, got push=%d, pop=%d", pushCount, popCount)
	}
}

func TestSceneBuilderComplexScene(t *testing.T) {
	// Build a complex scene with multiple operations
	scene := NewSceneBuilder().
		// Background
		FillRect(0, 0, 800, 600, SolidBrush(gg.White)).
		// Header
		Group(func(b *SceneBuilder) {
			b.FillRect(0, 0, 800, 60, SolidBrush(gg.Blue))
		}).
		// Content with transform
		Translate(50, 100).
		Clip(NewRectShape(0, 0, 700, 400), func(b *SceneBuilder) {
			// Draw multiple items
			for i := 0; i < 5; i++ {
				x := float32(i * 140)
				b.FillRect(x, 0, 120, 100, SolidBrush(gg.Red))
			}
		}).
		// Footer with layer effect
		ResetTransform().
		Translate(0, 540).
		Layer(BlendMultiply, 0.8, nil, func(b *SceneBuilder) {
			b.FillRect(0, 0, 800, 60, SolidBrush(gg.Green))
		}).
		Build()

	if scene == nil {
		t.Fatal("Build() returned nil")
	}

	// Verify scene has content
	if scene.IsEmpty() {
		t.Error("complex scene should not be empty")
	}

	// Verify bounds encompass all content
	bounds := scene.Bounds()
	if bounds.MaxX < 700 || bounds.MaxY < 500 {
		t.Errorf("bounds = %+v, expected larger area", bounds)
	}
}

// Benchmarks

func BenchmarkSceneBuilderFill(b *testing.B) {
	builder := NewSceneBuilder()
	rect := NewRectShape(0, 0, 100, 100)
	brush := SolidBrush(gg.Red)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		builder.Fill(rect, brush)
	}
}

func BenchmarkSceneBuilderChain(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewSceneBuilder().
			FillRect(0, 0, 800, 600, SolidBrush(gg.White)).
			Translate(100, 100).
			FillCircle(0, 0, 50, SolidBrush(gg.Red)).
			Build()
	}
}

func BenchmarkSceneBuilderWithLayer(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewSceneBuilder().
			Layer(BlendMultiply, 0.8, nil, func(b *SceneBuilder) {
				b.FillCircle(50, 50, 25, SolidBrush(gg.Red))
			}).
			Build()
	}
}

func BenchmarkSceneBuilderComplex(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewSceneBuilder().
			FillRect(0, 0, 800, 600, SolidBrush(gg.White)).
			Layer(BlendNormal, 1.0, NewRectShape(50, 50, 700, 500), func(b *SceneBuilder) {
				b.Translate(100, 100)
				for j := 0; j < 10; j++ {
					b.FillCircle(float32(j*50), 0, 20, SolidBrush(gg.Red))
				}
			}).
			Build()
	}
}
