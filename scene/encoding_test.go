package scene

import (
	"math"
	"testing"

	"github.com/gogpu/gg"
)

func TestNewEncoding(t *testing.T) {
	enc := NewEncoding()
	if enc == nil {
		t.Fatal("NewEncoding() returned nil")
	}
	if !enc.IsEmpty() {
		t.Error("new encoding should be empty")
	}
	if enc.PathCount() != 0 {
		t.Errorf("PathCount() = %d, want 0", enc.PathCount())
	}
	if enc.ShapeCount() != 0 {
		t.Errorf("ShapeCount() = %d, want 0", enc.ShapeCount())
	}
}

func TestEncodeTransform(t *testing.T) {
	tests := []struct {
		name      string
		transform Affine
	}{
		{
			name:      "identity",
			transform: IdentityAffine(),
		},
		{
			name:      "translate",
			transform: TranslateAffine(10, 20),
		},
		{
			name:      "scale",
			transform: ScaleAffine(2, 3),
		},
		{
			name:      "rotate",
			transform: RotateAffine(math.Pi / 4),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := NewEncoding()
			enc.EncodeTransform(tt.transform)

			if len(enc.Tags()) != 1 {
				t.Errorf("Tags() len = %d, want 1", len(enc.Tags()))
			}
			if enc.Tags()[0] != TagTransform {
				t.Errorf("tag = %v, want TagTransform", enc.Tags()[0])
			}
			if len(enc.Transforms()) != 1 {
				t.Errorf("Transforms() len = %d, want 1", len(enc.Transforms()))
			}

			got := enc.Transforms()[0]
			if got != tt.transform {
				t.Errorf("transform = %+v, want %+v", got, tt.transform)
			}
		})
	}
}

func TestEncodePath(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(100, 0)
	path.LineTo(100, 100)
	path.LineTo(0, 100)
	path.Close()

	enc := NewEncoding()
	enc.EncodePath(path)

	// Expected tags: BeginPath, MoveTo, LineTo, LineTo, LineTo, ClosePath, EndPath
	expectedTags := []Tag{
		TagBeginPath,
		TagMoveTo, TagLineTo, TagLineTo, TagLineTo,
		TagClosePath, TagEndPath,
	}

	if len(enc.Tags()) != len(expectedTags) {
		t.Errorf("Tags() len = %d, want %d", len(enc.Tags()), len(expectedTags))
	}

	for i, want := range expectedTags {
		if i < len(enc.Tags()) && enc.Tags()[i] != want {
			t.Errorf("tag[%d] = %v, want %v", i, enc.Tags()[i], want)
		}
	}

	// Check path data: MoveTo(2) + LineTo(2)*3 = 8 float32
	if len(enc.PathData()) != 8 {
		t.Errorf("PathData() len = %d, want 8", len(enc.PathData()))
	}

	if enc.PathCount() != 1 {
		t.Errorf("PathCount() = %d, want 1", enc.PathCount())
	}
}

func TestEncodePathWithCurves(t *testing.T) {
	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.QuadraticTo(50, 0, 100, 50) // control, end
	path.CubicTo(100, 100, 50, 100, 0, 50)
	path.Close()

	enc := NewEncoding()
	enc.EncodePath(path)

	expectedTags := []Tag{
		TagBeginPath,
		TagMoveTo, TagQuadTo, TagCubicTo, TagClosePath,
		TagEndPath,
	}

	if len(enc.Tags()) != len(expectedTags) {
		t.Errorf("Tags() len = %d, want %d", len(enc.Tags()), len(expectedTags))
	}

	// Path data: MoveTo(2) + QuadTo(4) + CubicTo(6) = 12 float32
	if len(enc.PathData()) != 12 {
		t.Errorf("PathData() len = %d, want 12", len(enc.PathData()))
	}
}

func TestEncodeFill(t *testing.T) {
	enc := NewEncoding()

	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)
	enc.EncodePath(path)

	brush := SolidBrush(gg.Red)
	enc.EncodeFill(brush, FillNonZero)

	// Should have TagFill in tags
	found := false
	for _, tag := range enc.Tags() {
		if tag == TagFill {
			found = true
			break
		}
	}
	if !found {
		t.Error("TagFill not found in tags")
	}

	if enc.ShapeCount() != 1 {
		t.Errorf("ShapeCount() = %d, want 1", enc.ShapeCount())
	}

	// Draw data should have brush index (0) and fill style (0 for NonZero)
	if len(enc.DrawData()) < 2 {
		t.Errorf("DrawData() len = %d, want >= 2", len(enc.DrawData()))
	}
}

func TestEncodeStroke(t *testing.T) {
	enc := NewEncoding()

	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(100, 100)
	enc.EncodePath(path)

	brush := SolidBrush(gg.Blue)
	style := &StrokeStyle{
		Width:      2.0,
		MiterLimit: 4.0,
		Cap:        LineCapRound,
		Join:       LineJoinRound,
	}
	enc.EncodeStroke(brush, style)

	// Should have TagStroke in tags
	found := false
	for _, tag := range enc.Tags() {
		if tag == TagStroke {
			found = true
			break
		}
	}
	if !found {
		t.Error("TagStroke not found in tags")
	}

	if enc.ShapeCount() != 1 {
		t.Errorf("ShapeCount() = %d, want 1", enc.ShapeCount())
	}

	// Draw data: brush index + width + miterLimit + cap + join = 5
	if len(enc.DrawData()) < 5 {
		t.Errorf("DrawData() len = %d, want >= 5", len(enc.DrawData()))
	}
}

func TestEncodeStrokeDefaultStyle(t *testing.T) {
	enc := NewEncoding()
	brush := SolidBrush(gg.Black)

	// Pass nil style, should use default
	enc.EncodeStroke(brush, nil)

	if enc.ShapeCount() != 1 {
		t.Errorf("ShapeCount() = %d, want 1", enc.ShapeCount())
	}
}

func TestEncodeLayers(t *testing.T) {
	enc := NewEncoding()

	enc.EncodePushLayer(BlendMultiply, 0.5)
	// ... encode content ...
	enc.EncodePopLayer()

	tags := enc.Tags()
	if len(tags) != 2 {
		t.Errorf("Tags() len = %d, want 2", len(tags))
	}
	if tags[0] != TagPushLayer {
		t.Errorf("tag[0] = %v, want TagPushLayer", tags[0])
	}
	if tags[1] != TagPopLayer {
		t.Errorf("tag[1] = %v, want TagPopLayer", tags[1])
	}

	// Draw data: blend mode + alpha = 2
	if len(enc.DrawData()) != 2 {
		t.Errorf("DrawData() len = %d, want 2", len(enc.DrawData()))
	}
}

func TestEncodeClip(t *testing.T) {
	enc := NewEncoding()

	enc.EncodeBeginClip()
	// ... encode content ...
	enc.EncodeEndClip()

	tags := enc.Tags()
	if len(tags) != 2 {
		t.Errorf("Tags() len = %d, want 2", len(tags))
	}
	if tags[0] != TagBeginClip {
		t.Errorf("tag[0] = %v, want TagBeginClip", tags[0])
	}
	if tags[1] != TagEndClip {
		t.Errorf("tag[1] = %v, want TagEndClip", tags[1])
	}
}

func TestReset(t *testing.T) {
	enc := NewEncoding()

	// Add some content
	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)
	enc.EncodePath(path)
	enc.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	// Capture capacity before reset
	tagCap := cap(enc.tags)
	pathDataCap := cap(enc.pathData)

	if enc.IsEmpty() {
		t.Error("encoding should not be empty before reset")
	}

	// Reset
	enc.Reset()

	if !enc.IsEmpty() {
		t.Error("encoding should be empty after reset")
	}
	if enc.PathCount() != 0 {
		t.Errorf("PathCount() = %d, want 0", enc.PathCount())
	}
	if enc.ShapeCount() != 0 {
		t.Errorf("ShapeCount() = %d, want 0", enc.ShapeCount())
	}

	// Verify capacity is preserved (zero-allocation reset)
	if cap(enc.tags) != tagCap {
		t.Errorf("tags capacity changed: got %d, had %d", cap(enc.tags), tagCap)
	}
	if cap(enc.pathData) != pathDataCap {
		t.Errorf("pathData capacity changed: got %d, had %d", cap(enc.pathData), pathDataCap)
	}
}

func TestBounds(t *testing.T) {
	enc := NewEncoding()

	path := gg.NewPath()
	path.MoveTo(10, 20)
	path.LineTo(100, 20)
	path.LineTo(100, 80)
	path.LineTo(10, 80)
	path.Close()
	enc.EncodePath(path)

	bounds := enc.Bounds()

	if bounds.MinX != 10 {
		t.Errorf("bounds.MinX = %f, want 10", bounds.MinX)
	}
	if bounds.MinY != 20 {
		t.Errorf("bounds.MinY = %f, want 20", bounds.MinY)
	}
	if bounds.MaxX != 100 {
		t.Errorf("bounds.MaxX = %f, want 100", bounds.MaxX)
	}
	if bounds.MaxY != 80 {
		t.Errorf("bounds.MaxY = %f, want 80", bounds.MaxY)
	}
}

func TestBoundsMultiplePaths(t *testing.T) {
	enc := NewEncoding()

	// First path
	path1 := gg.NewPath()
	path1.Rectangle(0, 0, 50, 50)
	enc.EncodePath(path1)

	// Second path
	path2 := gg.NewPath()
	path2.Rectangle(100, 100, 50, 50)
	enc.EncodePath(path2)

	bounds := enc.Bounds()

	if bounds.MinX != 0 {
		t.Errorf("bounds.MinX = %f, want 0", bounds.MinX)
	}
	if bounds.MinY != 0 {
		t.Errorf("bounds.MinY = %f, want 0", bounds.MinY)
	}
	if bounds.MaxX != 150 {
		t.Errorf("bounds.MaxX = %f, want 150", bounds.MaxX)
	}
	if bounds.MaxY != 150 {
		t.Errorf("bounds.MaxY = %f, want 150", bounds.MaxY)
	}
}

func TestHash(t *testing.T) {
	// Same content should produce same hash
	enc1 := NewEncoding()
	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)
	enc1.EncodePath(path)
	enc1.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	enc2 := NewEncoding()
	path2 := gg.NewPath()
	path2.Rectangle(0, 0, 100, 100)
	enc2.EncodePath(path2)
	enc2.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	hash1 := enc1.Hash()
	hash2 := enc2.Hash()

	if hash1 != hash2 {
		t.Errorf("same content should produce same hash: %x != %x", hash1, hash2)
	}

	// Different content should produce different hash
	enc3 := NewEncoding()
	path3 := gg.NewPath()
	path3.Rectangle(0, 0, 200, 200) // Different size
	enc3.EncodePath(path3)
	enc3.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	hash3 := enc3.Hash()

	if hash1 == hash3 {
		t.Error("different content should produce different hash")
	}
}

func TestHashStability(t *testing.T) {
	enc := NewEncoding()
	path := gg.NewPath()
	path.Circle(50, 50, 25)
	enc.EncodePath(path)
	enc.EncodeFill(SolidBrush(gg.Blue), FillEvenOdd)

	// Hash should be stable across multiple calls
	hash1 := enc.Hash()
	hash2 := enc.Hash()

	if hash1 != hash2 {
		t.Errorf("hash should be stable: %x != %x", hash1, hash2)
	}
}

func TestAppend(t *testing.T) {
	enc1 := NewEncoding()
	path1 := gg.NewPath()
	path1.Rectangle(0, 0, 50, 50)
	enc1.EncodePath(path1)
	enc1.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	enc2 := NewEncoding()
	path2 := gg.NewPath()
	path2.Rectangle(100, 100, 50, 50)
	enc2.EncodePath(path2)
	enc2.EncodeFill(SolidBrush(gg.Blue), FillNonZero)

	originalTagLen := len(enc1.Tags())
	originalPathCount := enc1.PathCount()
	originalShapeCount := enc1.ShapeCount()

	enc1.Append(enc2)

	// Tags should be combined
	if len(enc1.Tags()) != originalTagLen+len(enc2.Tags()) {
		t.Errorf("Tags() len = %d, want %d", len(enc1.Tags()), originalTagLen+len(enc2.Tags()))
	}

	// Counts should be combined
	if enc1.PathCount() != originalPathCount+enc2.PathCount() {
		t.Errorf("PathCount() = %d, want %d", enc1.PathCount(), originalPathCount+enc2.PathCount())
	}
	if enc1.ShapeCount() != originalShapeCount+enc2.ShapeCount() {
		t.Errorf("ShapeCount() = %d, want %d", enc1.ShapeCount(), originalShapeCount+enc2.ShapeCount())
	}

	// Bounds should be unioned
	bounds := enc1.Bounds()
	if bounds.MinX != 0 || bounds.MinY != 0 || bounds.MaxX != 150 || bounds.MaxY != 150 {
		t.Errorf("bounds = %+v, want (0,0)-(150,150)", bounds)
	}

	// Both brushes should be present
	if len(enc1.Brushes()) != 2 {
		t.Errorf("Brushes() len = %d, want 2", len(enc1.Brushes()))
	}
}

func TestAppendNil(t *testing.T) {
	enc := NewEncoding()
	path := gg.NewPath()
	path.Rectangle(0, 0, 50, 50)
	enc.EncodePath(path)

	originalLen := len(enc.Tags())

	// Appending nil should be a no-op
	enc.Append(nil)

	if len(enc.Tags()) != originalLen {
		t.Error("Append(nil) should not change encoding")
	}
}

func TestAppendEmpty(t *testing.T) {
	enc := NewEncoding()
	path := gg.NewPath()
	path.Rectangle(0, 0, 50, 50)
	enc.EncodePath(path)

	originalLen := len(enc.Tags())

	// Appending empty encoding should be a no-op
	enc.Append(NewEncoding())

	if len(enc.Tags()) != originalLen {
		t.Error("Append(empty) should not change encoding")
	}
}

func TestClone(t *testing.T) {
	enc := NewEncoding()
	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)
	enc.EncodePath(path)
	enc.EncodeFill(SolidBrush(gg.Red), FillNonZero)
	enc.EncodeTransform(TranslateAffine(10, 20))

	clone := enc.Clone()

	// Clone should have same content
	if len(clone.Tags()) != len(enc.Tags()) {
		t.Errorf("clone Tags() len = %d, want %d", len(clone.Tags()), len(enc.Tags()))
	}
	if clone.Hash() != enc.Hash() {
		t.Error("clone should have same hash as original")
	}

	// Clone should be independent
	clone.Reset()
	if enc.IsEmpty() {
		t.Error("resetting clone should not affect original")
	}
}

func TestIterator(t *testing.T) {
	enc := NewEncoding()
	enc.EncodeTransform(TranslateAffine(10, 20))

	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(100, 100)
	enc.EncodePath(path)

	enc.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	it := enc.NewIterator()

	// Read transform
	tag, ok := it.Next()
	if !ok || tag != TagTransform {
		t.Errorf("first tag = %v, %v, want TagTransform, true", tag, ok)
	}
	transform, ok := it.ReadTransform()
	if !ok {
		t.Error("ReadTransform() failed")
	}
	if transform.C != 10 || transform.F != 20 {
		t.Errorf("transform = %+v, want translate(10,20)", transform)
	}

	// Read BeginPath
	tag, ok = it.Next()
	if !ok || tag != TagBeginPath {
		t.Errorf("tag = %v, %v, want TagBeginPath, true", tag, ok)
	}

	// Read MoveTo
	tag, ok = it.Next()
	if !ok || tag != TagMoveTo {
		t.Errorf("tag = %v, %v, want TagMoveTo, true", tag, ok)
	}
	data := it.ReadPathData(2)
	if len(data) != 2 || data[0] != 0 || data[1] != 0 {
		t.Errorf("MoveTo data = %v, want [0, 0]", data)
	}

	// Read LineTo
	tag, ok = it.Next()
	if !ok || tag != TagLineTo {
		t.Errorf("tag = %v, %v, want TagLineTo, true", tag, ok)
	}
	data = it.ReadPathData(2)
	if len(data) != 2 || data[0] != 100 || data[1] != 100 {
		t.Errorf("LineTo data = %v, want [100, 100]", data)
	}

	// Read EndPath
	tag, _ = it.Next()
	if tag != TagEndPath {
		t.Errorf("tag = %v, want TagEndPath", tag)
	}

	// Read Fill
	tag, ok = it.Next()
	if !ok || tag != TagFill {
		t.Errorf("tag = %v, %v, want TagFill, true", tag, ok)
	}
	drawData := it.ReadDrawData(2)
	if len(drawData) != 2 {
		t.Errorf("Fill draw data len = %d, want 2", len(drawData))
	}

	// End of iteration
	_, ok = it.Next()
	if ok {
		t.Error("iteration should be complete")
	}
}

func TestIteratorReset(t *testing.T) {
	enc := NewEncoding()
	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(100, 100)
	enc.EncodePath(path)

	it := enc.NewIterator()

	// Consume some tags
	_, _ = it.Next()
	_, _ = it.Next()

	// Reset
	it.Reset()

	// Should start from beginning
	tag, ok := it.Next()
	if !ok || tag != TagBeginPath {
		t.Errorf("after reset, first tag = %v, %v, want TagBeginPath, true", tag, ok)
	}
}

func TestSize(t *testing.T) {
	enc := NewEncoding()
	initialSize := enc.Size()
	if initialSize != 0 {
		t.Errorf("empty encoding Size() = %d, want 0", initialSize)
	}

	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)
	enc.EncodePath(path)
	enc.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	size := enc.Size()
	if size <= 0 {
		t.Error("non-empty encoding should have positive size")
	}

	// Size should be less than 100 bytes per shape (performance target)
	avgSizePerShape := size / enc.ShapeCount()
	if avgSizePerShape > 100 {
		t.Errorf("average size per shape = %d bytes, want < 100", avgSizePerShape)
	}
}

func TestTagString(t *testing.T) {
	tests := []struct {
		tag  Tag
		want string
	}{
		{TagTransform, "Transform"},
		{TagBeginPath, "BeginPath"},
		{TagMoveTo, "MoveTo"},
		{TagLineTo, "LineTo"},
		{TagQuadTo, "QuadTo"},
		{TagCubicTo, "CubicTo"},
		{TagClosePath, "ClosePath"},
		{TagEndPath, "EndPath"},
		{TagFill, "Fill"},
		{TagStroke, "Stroke"},
		{TagPushLayer, "PushLayer"},
		{TagPopLayer, "PopLayer"},
		{TagBeginClip, "BeginClip"},
		{TagEndClip, "EndClip"},
		{TagBrush, "Brush"},
		{TagImage, "Image"},
		{Tag(0xFF), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.tag.String()
			if got != tt.want {
				t.Errorf("Tag(%#x).String() = %q, want %q", tt.tag, got, tt.want)
			}
		})
	}
}

func TestTagPredicates(t *testing.T) {
	pathTags := []Tag{TagBeginPath, TagMoveTo, TagLineTo, TagQuadTo, TagCubicTo, TagClosePath, TagEndPath}
	for _, tag := range pathTags {
		if !tag.IsPathCommand() {
			t.Errorf("%v.IsPathCommand() = false, want true", tag)
		}
	}

	if TagFill.IsPathCommand() {
		t.Error("TagFill.IsPathCommand() = true, want false")
	}

	if !TagFill.IsDrawCommand() {
		t.Error("TagFill.IsDrawCommand() = false, want true")
	}
	if !TagStroke.IsDrawCommand() {
		t.Error("TagStroke.IsDrawCommand() = false, want true")
	}
	if TagMoveTo.IsDrawCommand() {
		t.Error("TagMoveTo.IsDrawCommand() = true, want false")
	}

	if !TagPushLayer.IsLayerCommand() {
		t.Error("TagPushLayer.IsLayerCommand() = false, want true")
	}
	if !TagPopLayer.IsLayerCommand() {
		t.Error("TagPopLayer.IsLayerCommand() = false, want true")
	}

	if !TagBeginClip.IsClipCommand() {
		t.Error("TagBeginClip.IsClipCommand() = false, want true")
	}
	if !TagEndClip.IsClipCommand() {
		t.Error("TagEndClip.IsClipCommand() = false, want true")
	}
}

func TestTagDataSize(t *testing.T) {
	tests := []struct {
		tag  Tag
		want int
	}{
		{TagTransform, 6},
		{TagMoveTo, 2},
		{TagLineTo, 2},
		{TagQuadTo, 4},
		{TagCubicTo, 6},
		{TagBrush, 4},
		{TagBeginPath, 0},
		{TagClosePath, 0},
	}

	for _, tt := range tests {
		t.Run(tt.tag.String(), func(t *testing.T) {
			got := tt.tag.DataSize()
			if got != tt.want {
				t.Errorf("%v.DataSize() = %d, want %d", tt.tag, got, tt.want)
			}
		})
	}
}

func TestRect(t *testing.T) {
	// Empty rect
	empty := EmptyRect()
	if !empty.IsEmpty() {
		t.Error("EmptyRect() should be empty")
	}

	// Normal rect
	r := Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 50}
	if r.IsEmpty() {
		t.Error("normal rect should not be empty")
	}
	if r.Width() != 100 {
		t.Errorf("Width() = %f, want 100", r.Width())
	}
	if r.Height() != 50 {
		t.Errorf("Height() = %f, want 50", r.Height())
	}

	// Union
	r2 := Rect{MinX: 50, MinY: 25, MaxX: 150, MaxY: 75}
	union := r.Union(r2)
	if union.MinX != 0 || union.MinY != 0 || union.MaxX != 150 || union.MaxY != 75 {
		t.Errorf("Union() = %+v, want (0,0)-(150,75)", union)
	}

	// UnionPoint
	r3 := empty.UnionPoint(50, 50)
	if r3.MinX != 50 || r3.MinY != 50 || r3.MaxX != 50 || r3.MaxY != 50 {
		t.Errorf("UnionPoint() = %+v, want (50,50)-(50,50)", r3)
	}
}

func TestAffine(t *testing.T) {
	// Identity
	id := IdentityAffine()
	if !id.IsIdentity() {
		t.Error("IdentityAffine() should be identity")
	}

	// Translate
	tr := TranslateAffine(10, 20)
	x, y := tr.TransformPoint(0, 0)
	if x != 10 || y != 20 {
		t.Errorf("TranslateAffine(10,20).TransformPoint(0,0) = (%f,%f), want (10,20)", x, y)
	}

	// Scale
	sc := ScaleAffine(2, 3)
	x, y = sc.TransformPoint(10, 10)
	if x != 20 || y != 30 {
		t.Errorf("ScaleAffine(2,3).TransformPoint(10,10) = (%f,%f), want (20,30)", x, y)
	}

	// Multiply
	combined := tr.Multiply(sc)
	x, y = combined.TransformPoint(5, 5)
	// First scale: (5,5) -> (10, 15)
	// Then translate: (10, 15) -> (20, 35)
	if x != 20 || y != 35 {
		t.Errorf("combined.TransformPoint(5,5) = (%f,%f), want (20,35)", x, y)
	}
}

func TestAffineFromMatrix(t *testing.T) {
	m := gg.Matrix{A: 1, B: 2, C: 3, D: 4, E: 5, F: 6}
	a := AffineFromMatrix(m)

	if a.A != 1 || a.B != 2 || a.C != 3 || a.D != 4 || a.E != 5 || a.F != 6 {
		t.Errorf("AffineFromMatrix() = %+v, want {1,2,3,4,5,6}", a)
	}
}

func TestEncodeNilPath(t *testing.T) {
	enc := NewEncoding()
	enc.EncodePath(nil)

	if !enc.IsEmpty() {
		t.Error("encoding nil path should leave encoding empty")
	}
}

func TestEncodeEmptyPath(t *testing.T) {
	enc := NewEncoding()
	enc.EncodePath(gg.NewPath())

	if !enc.IsEmpty() {
		t.Error("encoding empty path should leave encoding empty")
	}
}

// Benchmarks

func BenchmarkEncodePath(b *testing.B) {
	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc := NewEncoding()
		enc.EncodePath(path)
	}
}

func BenchmarkEncodeComplex(b *testing.B) {
	path := gg.NewPath()
	path.Circle(50, 50, 40)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc := NewEncoding()
		enc.EncodePath(path)
		enc.EncodeFill(SolidBrush(gg.Red), FillNonZero)
	}
}

func BenchmarkReset(b *testing.B) {
	enc := NewEncoding()
	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)
	enc.EncodePath(path)
	enc.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc.Reset()
	}
}

func BenchmarkHash(b *testing.B) {
	enc := NewEncoding()
	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)
	enc.EncodePath(path)
	enc.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = enc.Hash()
	}
}

func BenchmarkAppend(b *testing.B) {
	enc1 := NewEncoding()
	path1 := gg.NewPath()
	path1.Rectangle(0, 0, 50, 50)
	enc1.EncodePath(path1)
	enc1.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	enc2 := NewEncoding()
	path2 := gg.NewPath()
	path2.Rectangle(100, 100, 50, 50)
	enc2.EncodePath(path2)
	enc2.EncodeFill(SolidBrush(gg.Blue), FillNonZero)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc := enc1.Clone()
		enc.Append(enc2)
	}
}

func BenchmarkClone(b *testing.B) {
	enc := NewEncoding()
	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)
	enc.EncodePath(path)
	enc.EncodeFill(SolidBrush(gg.Red), FillNonZero)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = enc.Clone()
	}
}
