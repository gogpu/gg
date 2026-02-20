package scene

import (
	"math"
	"testing"

	"github.com/gogpu/gg"
)

func TestNewScene(t *testing.T) {
	scene := NewScene()
	if scene == nil {
		t.Fatal("NewScene() returned nil")
	}
	if !scene.IsEmpty() {
		t.Error("new scene should be empty")
	}
	if scene.Version() != 0 {
		t.Errorf("Version() = %d, want 0", scene.Version())
	}
	if scene.LayerDepth() != 1 {
		t.Errorf("LayerDepth() = %d, want 1 (root layer)", scene.LayerDepth())
	}
	if scene.ClipDepth() != 0 {
		t.Errorf("ClipDepth() = %d, want 0", scene.ClipDepth())
	}
	if scene.TransformDepth() != 0 {
		t.Errorf("TransformDepth() = %d, want 0", scene.TransformDepth())
	}
}

func TestSceneReset(t *testing.T) {
	scene := NewScene()

	// Add some content
	rect := NewRectShape(0, 0, 100, 100)
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), rect)
	scene.PushLayer(BlendMultiply, 0.5, nil)

	initialVersion := scene.Version()
	if scene.IsEmpty() {
		t.Error("scene should not be empty after drawing")
	}

	scene.Reset()

	if !scene.IsEmpty() {
		t.Error("scene should be empty after reset")
	}
	if scene.Version() <= initialVersion {
		t.Error("version should increment after reset")
	}
	if scene.LayerDepth() != 1 {
		t.Errorf("LayerDepth() after reset = %d, want 1", scene.LayerDepth())
	}
}

func TestSceneFill(t *testing.T) {
	scene := NewScene()
	rect := NewRectShape(10, 20, 100, 50)
	brush := SolidBrush(gg.Red)

	initialVersion := scene.Version()
	scene.Fill(FillNonZero, IdentityAffine(), brush, rect)

	if scene.Version() == initialVersion {
		t.Error("version should increment after Fill")
	}

	bounds := scene.Bounds()
	if bounds.MinX != 10 || bounds.MinY != 20 || bounds.MaxX != 110 || bounds.MaxY != 70 {
		t.Errorf("bounds = %+v, want (10,20)-(110,70)", bounds)
	}
}

func TestSceneFillWithTransform(t *testing.T) {
	scene := NewScene()
	rect := NewRectShape(0, 0, 100, 100)
	brush := SolidBrush(gg.Blue)

	// Fill with translation
	transform := TranslateAffine(50, 50)
	scene.Fill(FillNonZero, transform, brush, rect)

	bounds := scene.Bounds()
	if bounds.MinX != 50 || bounds.MinY != 50 || bounds.MaxX != 150 || bounds.MaxY != 150 {
		t.Errorf("bounds = %+v, want (50,50)-(150,150)", bounds)
	}
}

func TestSceneFillNilShape(t *testing.T) {
	scene := NewScene()
	initialVersion := scene.Version()

	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), nil)

	if scene.Version() != initialVersion {
		t.Error("version should not change when filling nil shape")
	}
}

func TestSceneStroke(t *testing.T) {
	scene := NewScene()
	line := NewLineShape(0, 0, 100, 100)
	brush := SolidBrush(gg.Green)
	style := &StrokeStyle{Width: 4.0, MiterLimit: 10, Cap: LineCapRound, Join: LineJoinRound}

	initialVersion := scene.Version()
	scene.Stroke(style, IdentityAffine(), brush, line)

	if scene.Version() == initialVersion {
		t.Error("version should increment after Stroke")
	}

	// Bounds should be expanded by stroke width
	bounds := scene.Bounds()
	// Line is (0,0)-(100,100), stroke width 4 means expand by 2 on each side
	if bounds.MinX > -2 || bounds.MinY > -2 || bounds.MaxX < 102 || bounds.MaxY < 102 {
		t.Errorf("bounds = %+v, expected to include stroke width expansion", bounds)
	}
}

func TestSceneStrokeDefaultStyle(t *testing.T) {
	scene := NewScene()
	line := NewLineShape(0, 0, 100, 100)

	// Pass nil style, should use default
	scene.Stroke(nil, IdentityAffine(), SolidBrush(gg.Black), line)

	// Should not panic and bounds should be set
	if scene.Bounds().IsEmpty() {
		t.Error("bounds should not be empty after stroke with default style")
	}
}

func TestSceneDrawImage(t *testing.T) {
	scene := NewScene()
	img := NewImage(100, 50)

	initialVersion := scene.Version()
	scene.DrawImage(img, IdentityAffine())

	if scene.Version() == initialVersion {
		t.Error("version should increment after DrawImage")
	}

	bounds := scene.Bounds()
	if bounds.MinX != 0 || bounds.MinY != 0 || bounds.MaxX != 100 || bounds.MaxY != 50 {
		t.Errorf("bounds = %+v, want (0,0)-(100,50)", bounds)
	}

	// Image should be registered
	images := scene.Images()
	if len(images) != 1 || images[0] != img {
		t.Error("image should be registered")
	}

	// Draw same image again (should not duplicate registration)
	scene.DrawImage(img, TranslateAffine(50, 0))
	if len(scene.Images()) != 1 {
		t.Error("same image should not be registered twice")
	}
}

func TestSceneDrawNilImage(t *testing.T) {
	scene := NewScene()
	initialVersion := scene.Version()

	scene.DrawImage(nil, IdentityAffine())

	if scene.Version() != initialVersion {
		t.Error("version should not change when drawing nil image")
	}
}

func TestScenePushPopLayer(t *testing.T) {
	scene := NewScene()

	// Initial depth is 1 (root layer)
	if scene.LayerDepth() != 1 {
		t.Errorf("initial LayerDepth() = %d, want 1", scene.LayerDepth())
	}

	// Push a layer
	scene.PushLayer(BlendMultiply, 0.5, nil)
	if scene.LayerDepth() != 2 {
		t.Errorf("LayerDepth() after push = %d, want 2", scene.LayerDepth())
	}

	// Draw in layer
	rect := NewRectShape(0, 0, 100, 100)
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), rect)

	// Pop layer
	ok := scene.PopLayer()
	if !ok {
		t.Error("PopLayer() = false, want true")
	}
	if scene.LayerDepth() != 1 {
		t.Errorf("LayerDepth() after pop = %d, want 1", scene.LayerDepth())
	}

	// Cannot pop root layer
	ok = scene.PopLayer()
	if ok {
		t.Error("PopLayer() root = true, want false")
	}
}

func TestSceneLayerWithClip(t *testing.T) {
	scene := NewScene()

	// Push layer with clip
	clip := NewCircleShape(50, 50, 25)
	scene.PushLayer(BlendNormal, 1.0, clip)

	// Draw in clipped layer
	rect := NewRectShape(0, 0, 100, 100)
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), rect)

	// Pop layer
	scene.PopLayer()

	// Encoding should contain clip commands
	enc := scene.Encoding()
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
		t.Error("layer with clip should encode clip commands")
	}
}

func TestSceneNestedLayers(t *testing.T) {
	scene := NewScene()

	// Push multiple layers
	scene.PushLayer(BlendMultiply, 0.8, nil)
	if scene.LayerDepth() != 2 {
		t.Errorf("LayerDepth() = %d, want 2", scene.LayerDepth())
	}

	scene.PushLayer(BlendScreen, 0.5, nil)
	if scene.LayerDepth() != 3 {
		t.Errorf("LayerDepth() = %d, want 3", scene.LayerDepth())
	}

	scene.PushLayer(BlendOverlay, 1.0, nil)
	if scene.LayerDepth() != 4 {
		t.Errorf("LayerDepth() = %d, want 4", scene.LayerDepth())
	}

	// Pop all
	scene.PopLayer()
	scene.PopLayer()
	scene.PopLayer()

	if scene.LayerDepth() != 1 {
		t.Errorf("LayerDepth() after pops = %d, want 1", scene.LayerDepth())
	}
}

func TestScenePushPopClip(t *testing.T) {
	scene := NewScene()

	if scene.ClipDepth() != 0 {
		t.Errorf("initial ClipDepth() = %d, want 0", scene.ClipDepth())
	}

	// Push clip
	clip := NewRectShape(10, 10, 80, 80)
	scene.PushClip(clip)

	if scene.ClipDepth() != 1 {
		t.Errorf("ClipDepth() after push = %d, want 1", scene.ClipDepth())
	}

	// Draw in clipped area
	rect := NewRectShape(0, 0, 100, 100)
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Blue), rect)

	// Pop clip
	ok := scene.PopClip()
	if !ok {
		t.Error("PopClip() = false, want true")
	}
	if scene.ClipDepth() != 0 {
		t.Errorf("ClipDepth() after pop = %d, want 0", scene.ClipDepth())
	}

	// Pop from empty stack
	ok = scene.PopClip()
	if ok {
		t.Error("PopClip() from empty = true, want false")
	}
}

func TestScenePushClipNil(t *testing.T) {
	scene := NewScene()
	initialVersion := scene.Version()

	scene.PushClip(nil)

	// Should not push nil clip
	if scene.ClipDepth() != 0 {
		t.Error("nil clip should not be pushed")
	}
	if scene.Version() != initialVersion {
		t.Error("version should not change for nil clip")
	}
}

func TestSceneClipBounds(t *testing.T) {
	scene := NewScene()

	// No clips - should return large bounds
	bounds := scene.ClipBounds()
	if bounds.IsEmpty() {
		t.Error("clip bounds with no clips should not be empty")
	}

	// Push clips and check combined bounds
	scene.PushClip(NewRectShape(0, 0, 100, 100))
	scene.PushClip(NewRectShape(50, 50, 100, 100))

	bounds = scene.ClipBounds()
	// Intersection should be (50,50)-(100,100)
	if bounds.MinX != 50 || bounds.MinY != 50 || bounds.MaxX != 100 || bounds.MaxY != 100 {
		t.Errorf("clip bounds = %+v, want (50,50)-(100,100)", bounds)
	}
}

func TestScenePushPopTransform(t *testing.T) {
	scene := NewScene()

	if scene.TransformDepth() != 0 {
		t.Errorf("initial TransformDepth() = %d, want 0", scene.TransformDepth())
	}

	// Initial transform is identity
	if !scene.Transform().IsIdentity() {
		t.Error("initial transform should be identity")
	}

	// Push translation
	scene.PushTransform(TranslateAffine(10, 20))
	if scene.TransformDepth() != 1 {
		t.Errorf("TransformDepth() after push = %d, want 1", scene.TransformDepth())
	}

	transform := scene.Transform()
	if transform.C != 10 || transform.F != 20 {
		t.Errorf("transform = %+v, want translate(10,20)", transform)
	}

	// Push scale (concatenated)
	scene.PushTransform(ScaleAffine(2, 2))
	if scene.TransformDepth() != 2 {
		t.Errorf("TransformDepth() after second push = %d, want 2", scene.TransformDepth())
	}

	// Pop scale
	ok := scene.PopTransform()
	if !ok {
		t.Error("PopTransform() = false, want true")
	}
	if scene.TransformDepth() != 1 {
		t.Errorf("TransformDepth() after pop = %d, want 1", scene.TransformDepth())
	}

	// Pop translation
	ok = scene.PopTransform()
	if !ok {
		t.Error("PopTransform() = false, want true")
	}
	if !scene.Transform().IsIdentity() {
		t.Error("transform should be identity after popping all")
	}

	// Pop from empty stack
	ok = scene.PopTransform()
	if ok {
		t.Error("PopTransform() from empty = true, want false")
	}
}

func TestSceneSetTransform(t *testing.T) {
	scene := NewScene()

	// Set explicit transform
	transform := TranslateAffine(100, 200)
	scene.SetTransform(transform)

	if scene.Transform() != transform {
		t.Error("Transform() should match SetTransform value")
	}

	// SetTransform doesn't affect stack
	if scene.TransformDepth() != 0 {
		t.Error("SetTransform should not push to stack")
	}
}

func TestSceneTransformHelpers(t *testing.T) {
	scene := NewScene()

	// Translate
	scene.Translate(10, 20)
	transform := scene.Transform()
	if transform.C != 10 || transform.F != 20 {
		t.Errorf("after Translate, transform = %+v, want translate(10,20)", transform)
	}

	// Reset and test Scale
	scene.SetTransform(IdentityAffine())
	scene.Scale(2, 3)
	transform = scene.Transform()
	if transform.A != 2 || transform.E != 3 {
		t.Errorf("after Scale, transform = %+v, want scale(2,3)", transform)
	}

	// Reset and test Rotate
	scene.SetTransform(IdentityAffine())
	scene.Rotate(float32(math.Pi / 2))
	transform = scene.Transform()
	// After 90 degree rotation, A should be ~0, B should be ~-1
	if math.Abs(float64(transform.A)) > 0.01 || math.Abs(float64(transform.B+1)) > 0.01 {
		t.Errorf("after Rotate(pi/2), transform = %+v", transform)
	}
}

func TestSceneEncoding(t *testing.T) {
	scene := NewScene()

	// Draw some content
	rect := NewRectShape(0, 0, 100, 100)
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), rect)

	circle := NewCircleShape(150, 50, 25)
	scene.Fill(FillEvenOdd, IdentityAffine(), SolidBrush(gg.Blue), circle)

	enc := scene.Encoding()
	if enc == nil {
		t.Fatal("Encoding() returned nil")
	}
	if enc.IsEmpty() {
		t.Error("encoding should not be empty")
	}

	// Should have path and fill commands
	hasPath := false
	hasFill := false
	for _, tag := range enc.Tags() {
		if tag == TagBeginPath {
			hasPath = true
		}
		if tag == TagFill {
			hasFill = true
		}
	}
	if !hasPath {
		t.Error("encoding should contain path commands")
	}
	if !hasFill {
		t.Error("encoding should contain fill commands")
	}
}

func TestSceneVersion(t *testing.T) {
	scene := NewScene()
	v0 := scene.Version()

	// Each operation should increment version
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), NewRectShape(0, 0, 10, 10))
	v1 := scene.Version()
	if v1 <= v0 {
		t.Error("version should increment after Fill")
	}

	scene.PushLayer(BlendNormal, 1.0, nil)
	v2 := scene.Version()
	if v2 <= v1 {
		t.Error("version should increment after PushLayer")
	}

	scene.PopLayer()
	v3 := scene.Version()
	if v3 <= v2 {
		t.Error("version should increment after PopLayer")
	}

	scene.PushTransform(IdentityAffine())
	v4 := scene.Version()
	if v4 <= v3 {
		t.Error("version should increment after PushTransform")
	}
}

func TestSceneBoundsTracking(t *testing.T) {
	scene := NewScene()

	// Draw first shape
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), NewRectShape(0, 0, 50, 50))
	bounds1 := scene.Bounds()

	// Draw second shape (should expand bounds)
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Blue), NewRectShape(100, 100, 50, 50))
	bounds2 := scene.Bounds()

	if bounds2.MinX != bounds1.MinX || bounds2.MinY != bounds1.MinY {
		t.Error("bounds min should not change when adding shape to the right/bottom")
	}
	if bounds2.MaxX <= bounds1.MaxX || bounds2.MaxY <= bounds1.MaxY {
		t.Error("bounds max should expand when adding shape to the right/bottom")
	}

	// Final bounds should encompass all content
	if bounds2.MinX != 0 || bounds2.MinY != 0 || bounds2.MaxX != 150 || bounds2.MaxY != 150 {
		t.Errorf("final bounds = %+v, want (0,0)-(150,150)", bounds2)
	}
}

func TestImage(t *testing.T) {
	img := NewImage(100, 50)

	if img.Width != 100 {
		t.Errorf("Width = %d, want 100", img.Width)
	}
	if img.Height != 50 {
		t.Errorf("Height = %d, want 50", img.Height)
	}

	bounds := img.Bounds()
	if bounds.MinX != 0 || bounds.MinY != 0 || bounds.MaxX != 100 || bounds.MaxY != 50 {
		t.Errorf("Bounds() = %+v, want (0,0)-(100,50)", bounds)
	}

	if img.IsEmpty() {
		t.Error("100x50 image should not be empty")
	}

	emptyImg := NewImage(0, 0)
	if !emptyImg.IsEmpty() {
		t.Error("0x0 image should be empty")
	}
}

func TestScenePool(t *testing.T) {
	pool := NewScenePool()

	// Get from empty pool
	s1 := pool.Get()
	if s1 == nil {
		t.Fatal("Get() returned nil")
	}

	// Add content
	s1.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), NewRectShape(0, 0, 100, 100))

	// Return to pool
	pool.Put(s1)

	// Get again (should be reset)
	s2 := pool.Get()
	if !s2.IsEmpty() {
		t.Error("scene from pool should be reset")
	}

	// Put nil should not panic
	pool.Put(nil)
}

func TestScenePoolWarmup(t *testing.T) {
	pool := NewScenePool()

	// Warmup should pre-allocate
	pool.Warmup(5)

	// Get should return pre-allocated scenes
	scenes := make([]*Scene, 5)
	for i := 0; i < 5; i++ {
		scenes[i] = pool.Get()
		if scenes[i] == nil {
			t.Fatalf("Get() after warmup returned nil at %d", i)
		}
	}

	// Return all
	for _, s := range scenes {
		pool.Put(s)
	}
}

func TestLayerKind(t *testing.T) {
	tests := []struct {
		kind           LayerKind
		name           string
		needsOffscreen bool
		isClipOnly     bool
	}{
		{LayerRegular, "Regular", false, false},
		{LayerFiltered, "Filtered", true, false},
		{LayerClip, "Clip", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.kind.String() != tt.name {
				t.Errorf("String() = %q, want %q", tt.kind.String(), tt.name)
			}
			if tt.kind.NeedsOffscreen() != tt.needsOffscreen {
				t.Errorf("NeedsOffscreen() = %v, want %v", tt.kind.NeedsOffscreen(), tt.needsOffscreen)
			}
			if tt.kind.IsClipOnly() != tt.isClipOnly {
				t.Errorf("IsClipOnly() = %v, want %v", tt.kind.IsClipOnly(), tt.isClipOnly)
			}
		})
	}
}

func TestLayerState(t *testing.T) {
	layer := NewLayerState(LayerRegular, BlendMultiply, 0.5)

	if layer.Kind != LayerRegular {
		t.Errorf("Kind = %v, want LayerRegular", layer.Kind)
	}
	if layer.BlendMode != BlendMultiply {
		t.Errorf("BlendMode = %v, want BlendMultiply", layer.BlendMode)
	}
	if layer.Alpha != 0.5 {
		t.Errorf("Alpha = %f, want 0.5", layer.Alpha)
	}

	// Alpha clamping
	clampedLayer := NewLayerState(LayerRegular, BlendNormal, 1.5)
	if clampedLayer.Alpha != 1.0 {
		t.Errorf("Alpha > 1 should be clamped to 1.0, got %f", clampedLayer.Alpha)
	}

	clampedLayer2 := NewLayerState(LayerRegular, BlendNormal, -0.5)
	if clampedLayer2.Alpha != 0.0 {
		t.Errorf("Alpha < 0 should be clamped to 0.0, got %f", clampedLayer2.Alpha)
	}
}

func TestLayerStateHelpers(t *testing.T) {
	// NewClipLayer
	clip := NewRectShape(0, 0, 100, 100)
	clipLayer := NewClipLayer(clip)
	if clipLayer.Kind != LayerClip {
		t.Error("NewClipLayer should create LayerClip")
	}
	if !clipLayer.HasClip() {
		t.Error("clip layer should have clip")
	}

	// NewFilteredLayer
	filteredLayer := NewFilteredLayer(BlendNormal, 0.8)
	if filteredLayer.Kind != LayerFiltered {
		t.Error("NewFilteredLayer should create LayerFiltered")
	}
}

func TestLayerStack(t *testing.T) {
	stack := NewLayerStack()

	// Initial state
	if stack.Depth() != 1 {
		t.Errorf("initial Depth() = %d, want 1", stack.Depth())
	}
	if !stack.IsRoot() {
		t.Error("initial stack should be at root")
	}
	if stack.Root() == nil {
		t.Error("Root() should not be nil")
	}
	if stack.Top() != stack.Root() {
		t.Error("Top() should equal Root() initially")
	}

	// Push layer
	layer := NewLayerState(LayerRegular, BlendMultiply, 0.5)
	stack.Push(layer)
	if stack.Depth() != 2 {
		t.Errorf("Depth() after push = %d, want 2", stack.Depth())
	}
	if stack.IsRoot() {
		t.Error("should not be at root after push")
	}
	if stack.Top() != layer {
		t.Error("Top() should be pushed layer")
	}

	// Pop layer
	popped := stack.Pop()
	if popped != layer {
		t.Error("Pop() should return pushed layer")
	}
	if stack.Depth() != 1 {
		t.Errorf("Depth() after pop = %d, want 1", stack.Depth())
	}

	// Cannot pop root
	popped = stack.Pop()
	if popped != nil {
		t.Error("Pop() root should return nil")
	}
}

func TestLayerStackReset(t *testing.T) {
	stack := NewLayerStack()

	// Push multiple layers
	stack.Push(NewLayerState(LayerRegular, BlendMultiply, 0.5))
	stack.Push(NewLayerState(LayerFiltered, BlendScreen, 0.8))

	if stack.Depth() != 3 {
		t.Errorf("Depth() = %d, want 3", stack.Depth())
	}

	stack.Reset()

	if stack.Depth() != 1 {
		t.Errorf("Depth() after reset = %d, want 1", stack.Depth())
	}
	if !stack.IsRoot() {
		t.Error("should be at root after reset")
	}
}

func TestClipState(t *testing.T) {
	shape := NewRectShape(10, 20, 80, 60)
	clip := NewClipState(shape, IdentityAffine())

	bounds := clip.Bounds
	if bounds.MinX != 10 || bounds.MinY != 20 || bounds.MaxX != 90 || bounds.MaxY != 80 {
		t.Errorf("bounds = %+v, want (10,20)-(90,80)", bounds)
	}

	// Test Contains
	if !clip.Contains(50, 50) {
		t.Error("Contains(50,50) = false, want true")
	}
	if clip.Contains(0, 0) {
		t.Error("Contains(0,0) = true, want false")
	}

	// Test Intersects
	if !clip.Intersects(Rect{MinX: 0, MinY: 0, MaxX: 50, MaxY: 50}) {
		t.Error("Intersects overlapping rect = false, want true")
	}
	if clip.Intersects(Rect{MinX: 100, MinY: 100, MaxX: 200, MaxY: 200}) {
		t.Error("Intersects non-overlapping rect = true, want false")
	}
}

func TestClipStateWithTransform(t *testing.T) {
	shape := NewRectShape(0, 0, 100, 100)
	transform := TranslateAffine(50, 50)
	clip := NewClipState(shape, transform)

	bounds := clip.Bounds
	if bounds.MinX != 50 || bounds.MinY != 50 || bounds.MaxX != 150 || bounds.MaxY != 150 {
		t.Errorf("transformed bounds = %+v, want (50,50)-(150,150)", bounds)
	}
}

func TestClipStack(t *testing.T) {
	stack := NewClipStack()

	// Initial state
	if !stack.IsEmpty() {
		t.Error("initial stack should be empty")
	}
	if stack.Depth() != 0 {
		t.Errorf("initial Depth() = %d, want 0", stack.Depth())
	}
	if stack.Top() != nil {
		t.Error("Top() of empty stack should be nil")
	}

	// Push clip
	clip1 := NewClipState(NewRectShape(0, 0, 100, 100), IdentityAffine())
	stack.Push(clip1)
	if stack.Depth() != 1 {
		t.Errorf("Depth() after push = %d, want 1", stack.Depth())
	}
	if stack.Top() != clip1 {
		t.Error("Top() should be pushed clip")
	}

	// Push another clip
	clip2 := NewClipState(NewRectShape(50, 50, 100, 100), IdentityAffine())
	stack.Push(clip2)
	if stack.Depth() != 2 {
		t.Errorf("Depth() = %d, want 2", stack.Depth())
	}

	// Test combined bounds (intersection)
	combined := stack.CombinedBounds()
	// (0,0)-(100,100) intersect (50,50)-(150,150) = (50,50)-(100,100)
	if combined.MinX != 50 || combined.MinY != 50 || combined.MaxX != 100 || combined.MaxY != 100 {
		t.Errorf("CombinedBounds() = %+v, want (50,50)-(100,100)", combined)
	}

	// Pop clips
	popped := stack.Pop()
	if popped != clip2 {
		t.Error("Pop() should return last pushed clip")
	}
	popped = stack.Pop()
	if popped != clip1 {
		t.Error("Pop() should return first pushed clip")
	}
	popped = stack.Pop()
	if popped != nil {
		t.Error("Pop() from empty should return nil")
	}
}

func TestClipStackReset(t *testing.T) {
	stack := NewClipStack()
	stack.Push(NewClipState(NewRectShape(0, 0, 100, 100), IdentityAffine()))
	stack.Push(NewClipState(NewRectShape(0, 0, 50, 50), IdentityAffine()))

	stack.Reset()

	if !stack.IsEmpty() {
		t.Error("stack should be empty after reset")
	}
}

// Benchmarks

func BenchmarkSceneFill(b *testing.B) {
	scene := NewScene()
	rect := NewRectShape(0, 0, 100, 100)
	brush := SolidBrush(gg.Red)
	transform := IdentityAffine()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scene.Fill(FillNonZero, transform, brush, rect)
	}
}

func BenchmarkSceneStroke(b *testing.B) {
	scene := NewScene()
	line := NewLineShape(0, 0, 100, 100)
	brush := SolidBrush(gg.Blue)
	style := DefaultStrokeStyle()
	transform := IdentityAffine()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scene.Stroke(style, transform, brush, line)
	}
}

func BenchmarkScenePushPopLayer(b *testing.B) {
	scene := NewScene()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scene.PushLayer(BlendNormal, 1.0, nil)
		scene.PopLayer()
	}
}

func BenchmarkScenePushPopTransform(b *testing.B) {
	scene := NewScene()
	transform := TranslateAffine(10, 20)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scene.PushTransform(transform)
		scene.PopTransform()
	}
}

func BenchmarkSceneComplex(b *testing.B) {
	// Simulate building a complex scene
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scene := NewScene()

		// Background
		scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.White),
			NewRectShape(0, 0, 800, 600))

		// Layer with clip
		scene.PushLayer(BlendNormal, 1.0, NewRectShape(50, 50, 700, 500))
		scene.PushTransform(TranslateAffine(100, 100))

		// Draw multiple shapes
		for j := 0; j < 10; j++ {
			scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red),
				NewCircleShape(float32(j*50), 0, 20))
		}

		scene.PopTransform()
		scene.PopLayer()

		_ = scene.Encoding()
	}
}

func BenchmarkScenePoolGetPut(b *testing.B) {
	pool := NewScenePool()
	pool.Warmup(10)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := pool.Get()
		s.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red),
			NewRectShape(0, 0, 100, 100))
		pool.Put(s)
	}
}

// TestEncodingBoundsWithTransform verifies that encoding bounds include
// transformed coordinates, not just raw path coords. This is the fix for
// gg#116: WithTransform caused invisible rendering because the tile-based
// renderer's early-out check used untransformed encoding bounds.
func TestEncodingBoundsWithTransform(t *testing.T) {
	scene := NewScene()

	// Draw a rect at (0,0)-(100,100) with translate(200,300)
	transform := TranslateAffine(200, 300)
	scene.Fill(FillNonZero, transform, SolidBrush(gg.Red), NewRectShape(0, 0, 100, 100))

	// Scene bounds should reflect transform
	sceneBounds := scene.Bounds()
	if sceneBounds.MinX != 200 || sceneBounds.MinY != 300 ||
		sceneBounds.MaxX != 300 || sceneBounds.MaxY != 400 {
		t.Errorf("scene bounds = %+v, want (200,300)-(300,400)", sceneBounds)
	}

	// Critical: encoding bounds must ALSO include the transformed coordinates.
	// Before the fix, encoding bounds only had (0,0)-(100,100) which caused
	// the renderer to skip tiles in the (200,300) area.
	enc := scene.Encoding()
	encBounds := enc.Bounds()

	// Encoding bounds must intersect with the (200,300)-(300,400) region
	if encBounds.MaxX < 300 || encBounds.MaxY < 400 {
		t.Errorf("encoding bounds = %+v, must include transformed region (200,300)-(300,400)", encBounds)
	}
}

// TestEncodingBoundsClipDoesNotExpand verifies that clip paths do not
// expand encoding bounds (clips restrict visible area, not expand it).
func TestEncodingBoundsClipDoesNotExpand(t *testing.T) {
	scene := NewScene()

	// Draw content at (0,0)-(50,50)
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), NewRectShape(0, 0, 50, 50))

	boundsBeforeClip := scene.Encoding().Bounds()

	// Push a clip that extends far beyond the content
	scene.PushClip(NewRectShape(0, 0, 1000, 1000))

	// Draw more content within the clip
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Blue), NewRectShape(10, 10, 30, 30))

	scene.PopClip()

	boundsAfterClip := scene.Encoding().Bounds()

	// The clip path itself should NOT expand the encoding bounds beyond
	// where the actual content is. The bounds should be determined by
	// the drawn content, not by the clip shape.
	if boundsAfterClip.MaxX > boundsBeforeClip.MaxX+1 || boundsAfterClip.MaxY > boundsBeforeClip.MaxY+1 {
		t.Errorf("clip expanded encoding bounds: before=%+v after=%+v (clip should restrict, not expand)",
			boundsBeforeClip, boundsAfterClip)
	}
}
