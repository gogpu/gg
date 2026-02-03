package recording

import (
	"image"
	"image/color"
	"testing"

	"github.com/gogpu/gg"
)

func TestNewResourcePool(t *testing.T) {
	pool := NewResourcePool()
	if pool == nil {
		t.Fatal("NewResourcePool returned nil")
	}
	if pool.PathCount() != 0 {
		t.Errorf("PathCount() = %d, want 0", pool.PathCount())
	}
	if pool.BrushCount() != 0 {
		t.Errorf("BrushCount() = %d, want 0", pool.BrushCount())
	}
	if pool.ImageCount() != 0 {
		t.Errorf("ImageCount() = %d, want 0", pool.ImageCount())
	}
	if pool.FontCount() != 0 {
		t.Errorf("FontCount() = %d, want 0", pool.FontCount())
	}
}

func TestResourcePool_AddPath(t *testing.T) {
	tests := []struct {
		name      string
		setupPath func() *gg.Path
		wantNil   bool
	}{
		{
			name: "simple rectangle",
			setupPath: func() *gg.Path {
				p := gg.NewPath()
				p.Rectangle(0, 0, 100, 100)
				return p
			},
			wantNil: false,
		},
		{
			name: "circle",
			setupPath: func() *gg.Path {
				p := gg.NewPath()
				p.Circle(50, 50, 25)
				return p
			},
			wantNil: false,
		},
		{
			name: "nil path",
			setupPath: func() *gg.Path {
				return nil
			},
			wantNil: true,
		},
		{
			name:      "empty path",
			setupPath: gg.NewPath,
			wantNil:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewResourcePool()
			path := tt.setupPath()
			ref := pool.AddPath(path)

			if pool.PathCount() != 1 {
				t.Errorf("PathCount() = %d, want 1", pool.PathCount())
			}

			got := pool.GetPath(ref)
			if tt.wantNil {
				if got != nil {
					t.Errorf("GetPath() = %v, want nil", got)
				}
			} else {
				if got == nil {
					t.Errorf("GetPath() returned nil, want path")
				}
			}
		})
	}
}

func TestResourcePool_AddPath_Clone(t *testing.T) {
	// Verify that adding a path clones it (immutability)
	pool := NewResourcePool()

	original := gg.NewPath()
	original.MoveTo(0, 0)
	original.LineTo(100, 100)

	ref := pool.AddPath(original)

	// Modify the original path
	original.LineTo(200, 200)

	// The pooled path should not be affected
	pooled := pool.GetPath(ref)
	originalElements := original.Elements()
	pooledElements := pooled.Elements()

	if len(originalElements) == len(pooledElements) {
		t.Error("Path was not cloned - modifications to original affected pooled version")
	}

	if len(pooledElements) != 2 {
		t.Errorf("Pooled path has %d elements, want 2", len(pooledElements))
	}
}

func TestResourcePool_AddPath_Multiple(t *testing.T) {
	pool := NewResourcePool()

	refs := make([]PathRef, 5)
	for i := 0; i < 5; i++ {
		p := gg.NewPath()
		p.Circle(float64(i*10), float64(i*10), float64(i+1)*5)
		refs[i] = pool.AddPath(p)
	}

	if pool.PathCount() != 5 {
		t.Errorf("PathCount() = %d, want 5", pool.PathCount())
	}

	// Verify each reference is unique and valid
	for i, ref := range refs {
		if int(ref) != i {
			t.Errorf("refs[%d] = %d, want %d", i, ref, i)
		}
		path := pool.GetPath(ref)
		if path == nil {
			t.Errorf("GetPath(refs[%d]) returned nil", i)
		}
	}
}

func TestResourcePool_GetPath_InvalidRef(t *testing.T) {
	pool := NewResourcePool()
	pool.AddPath(gg.NewPath())

	// Reference beyond pool size
	got := pool.GetPath(PathRef(100))
	if got != nil {
		t.Errorf("GetPath(invalid ref) = %v, want nil", got)
	}
}

func TestResourcePool_AddBrush(t *testing.T) {
	tests := []struct {
		name  string
		brush Brush
	}{
		{
			name:  "solid red",
			brush: NewSolidBrush(gg.Red),
		},
		{
			name:  "solid with alpha",
			brush: NewSolidBrush(gg.RGBA2(1, 0, 0, 0.5)),
		},
		{
			name: "linear gradient",
			brush: NewLinearGradientBrush(0, 0, 100, 0).
				AddColorStop(0, gg.Red).
				AddColorStop(1, gg.Blue),
		},
		{
			name: "radial gradient",
			brush: NewRadialGradientBrush(50, 50, 0, 50).
				AddColorStop(0, gg.White).
				AddColorStop(1, gg.Black),
		},
		{
			name: "sweep gradient",
			brush: NewSweepGradientBrush(50, 50, 0).
				AddColorStop(0, gg.Red).
				AddColorStop(1, gg.Red),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewResourcePool()
			ref := pool.AddBrush(tt.brush)

			if pool.BrushCount() != 1 {
				t.Errorf("BrushCount() = %d, want 1", pool.BrushCount())
			}

			got := pool.GetBrush(ref)
			if got == nil {
				t.Error("GetBrush() returned nil")
			}
		})
	}
}

func TestResourcePool_GetBrush_InvalidRef(t *testing.T) {
	pool := NewResourcePool()
	pool.AddBrush(NewSolidBrush(gg.Red))

	got := pool.GetBrush(BrushRef(100))
	if got != nil {
		t.Errorf("GetBrush(invalid ref) = %v, want nil", got)
	}
}

func TestResourcePool_AddImage(t *testing.T) {
	pool := NewResourcePool()

	// Create a simple test image
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	ref := pool.AddImage(img)

	if pool.ImageCount() != 1 {
		t.Errorf("ImageCount() = %d, want 1", pool.ImageCount())
	}

	got := pool.GetImage(ref)
	if got == nil {
		t.Error("GetImage() returned nil")
	}

	bounds := got.Bounds()
	if bounds.Dx() != 10 || bounds.Dy() != 10 {
		t.Errorf("Image bounds = %v, want 10x10", bounds)
	}
}

func TestResourcePool_AddImage_Nil(t *testing.T) {
	pool := NewResourcePool()
	ref := pool.AddImage(nil)

	if pool.ImageCount() != 1 {
		t.Errorf("ImageCount() = %d, want 1", pool.ImageCount())
	}

	got := pool.GetImage(ref)
	if got != nil {
		t.Errorf("GetImage() = %v, want nil", got)
	}
}

func TestResourcePool_GetImage_InvalidRef(t *testing.T) {
	pool := NewResourcePool()
	pool.AddImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))

	got := pool.GetImage(ImageRef(100))
	if got != nil {
		t.Errorf("GetImage(invalid ref) = %v, want nil", got)
	}
}

func TestResourcePool_AddFont(t *testing.T) {
	pool := NewResourcePool()

	// Note: We can't easily create a text.Face without a FontSource,
	// so we test with nil (which is valid to add)
	ref := pool.AddFont(nil)

	if pool.FontCount() != 1 {
		t.Errorf("FontCount() = %d, want 1", pool.FontCount())
	}

	got := pool.GetFont(ref)
	if got != nil {
		t.Errorf("GetFont() = %v, want nil", got)
	}
}

func TestResourcePool_GetFont_InvalidRef(t *testing.T) {
	pool := NewResourcePool()
	pool.AddFont(nil)

	got := pool.GetFont(FontRef(100))
	if got != nil {
		t.Errorf("GetFont(invalid ref) = %v, want nil", got)
	}
}

func TestResourcePool_Clear(t *testing.T) {
	pool := NewResourcePool()

	// Add various resources
	pool.AddPath(gg.NewPath())
	pool.AddBrush(NewSolidBrush(gg.Red))
	pool.AddImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	pool.AddFont(nil)

	if pool.PathCount() != 1 || pool.BrushCount() != 1 ||
		pool.ImageCount() != 1 || pool.FontCount() != 1 {
		t.Fatal("Resources not added correctly before Clear")
	}

	pool.Clear()

	if pool.PathCount() != 0 {
		t.Errorf("PathCount() after Clear = %d, want 0", pool.PathCount())
	}
	if pool.BrushCount() != 0 {
		t.Errorf("BrushCount() after Clear = %d, want 0", pool.BrushCount())
	}
	if pool.ImageCount() != 0 {
		t.Errorf("ImageCount() after Clear = %d, want 0", pool.ImageCount())
	}
	if pool.FontCount() != 0 {
		t.Errorf("FontCount() after Clear = %d, want 0", pool.FontCount())
	}
}

func TestResourcePool_Clone(t *testing.T) {
	pool := NewResourcePool()

	// Add resources
	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)
	pathRef := pool.AddPath(path)

	brush := NewSolidBrush(gg.Red)
	brushRef := pool.AddBrush(brush)

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	imageRef := pool.AddImage(img)

	// Clone the pool
	cloned := pool.Clone()

	// Verify counts match
	if cloned.PathCount() != pool.PathCount() {
		t.Errorf("Cloned PathCount = %d, original = %d", cloned.PathCount(), pool.PathCount())
	}
	if cloned.BrushCount() != pool.BrushCount() {
		t.Errorf("Cloned BrushCount = %d, original = %d", cloned.BrushCount(), pool.BrushCount())
	}
	if cloned.ImageCount() != pool.ImageCount() {
		t.Errorf("Cloned ImageCount = %d, original = %d", cloned.ImageCount(), pool.ImageCount())
	}

	// Verify resources are accessible
	clonedPath := cloned.GetPath(pathRef)
	if clonedPath == nil {
		t.Error("Cloned pool returned nil for path")
	}

	clonedBrush := cloned.GetBrush(brushRef)
	if clonedBrush == nil {
		t.Error("Cloned pool returned nil for brush")
	}

	clonedImage := cloned.GetImage(imageRef)
	if clonedImage == nil {
		t.Error("Cloned pool returned nil for image")
	}
}

func TestResourcePool_Clone_PathIndependence(t *testing.T) {
	pool := NewResourcePool()

	path := gg.NewPath()
	path.MoveTo(0, 0)
	path.LineTo(100, 100)
	pathRef := pool.AddPath(path)

	// Clone the pool
	cloned := pool.Clone()

	// Modify path in cloned pool (get and modify)
	clonedPath := cloned.GetPath(pathRef)
	originalPath := pool.GetPath(pathRef)

	// They should be different pointers
	if clonedPath == originalPath {
		t.Error("Cloned path points to same object as original")
	}
}

func TestReferenceTypes(t *testing.T) {
	// Verify that reference types are distinct
	var pathRef PathRef = 1
	var brushRef BrushRef = 1
	var imageRef ImageRef = 1
	var fontRef FontRef = 1

	// This is a compile-time check - the following should NOT compile:
	// pathRef = brushRef  // different types
	// We verify they're independent types by using them separately
	_ = pathRef
	_ = brushRef
	_ = imageRef
	_ = fontRef

	// Verify they can hold the same value but are type-safe
	if uint32(pathRef) != uint32(brushRef) {
		t.Error("Reference values don't match")
	}
}

// BenchmarkResourcePool_AddPath benchmarks path addition performance.
func BenchmarkResourcePool_AddPath(b *testing.B) {
	pool := NewResourcePool()
	path := gg.NewPath()
	path.Rectangle(0, 0, 100, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.AddPath(path)
	}
}

// BenchmarkResourcePool_GetPath benchmarks path retrieval performance.
func BenchmarkResourcePool_GetPath(b *testing.B) {
	pool := NewResourcePool()

	// Add 1000 paths
	for i := 0; i < 1000; i++ {
		path := gg.NewPath()
		path.Circle(float64(i), float64(i), 10)
		pool.AddPath(path)
	}

	refs := []PathRef{PathRef(0), PathRef(500), PathRef(999)}
	refIdx := 0

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.GetPath(refs[refIdx%len(refs)])
		refIdx++
	}
}

// BenchmarkResourcePool_AddBrush benchmarks brush addition performance.
func BenchmarkResourcePool_AddBrush(b *testing.B) {
	pool := NewResourcePool()
	brush := NewSolidBrush(gg.Red)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.AddBrush(brush)
	}
}

// BenchmarkResourcePool_Clone benchmarks pool cloning performance.
func BenchmarkResourcePool_Clone(b *testing.B) {
	pool := NewResourcePool()

	// Add various resources
	for i := 0; i < 100; i++ {
		path := gg.NewPath()
		path.Circle(float64(i), float64(i), 10)
		pool.AddPath(path)
	}
	for i := 0; i < 50; i++ {
		pool.AddBrush(NewSolidBrush(gg.RGBA{R: float64(i) / 50, G: 0, B: 0, A: 1}))
	}
	for i := 0; i < 10; i++ {
		pool.AddImage(image.NewRGBA(image.Rect(0, 0, 100, 100)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.Clone()
	}
}
