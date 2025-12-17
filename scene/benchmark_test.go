package scene

import (
	"testing"

	"github.com/gogpu/gg"
)

// ---------------------------------------------------------------------------
// Scene Building Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkScene_Build_100Shapes(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		scene := NewScene()
		for j := 0; j < 100; j++ {
			x := float32(j%10) * 50
			y := float32(j/10) * 50
			scene.Fill(FillNonZero, IdentityAffine(),
				SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}),
				NewRectShape(x, y, 40, 40))
		}
	}
}

func BenchmarkScene_Build_1000Shapes(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		scene := NewScene()
		for j := 0; j < 1000; j++ {
			x := float32(j%32) * 30
			y := float32(j/32) * 30
			scene.Fill(FillNonZero, IdentityAffine(),
				SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}),
				NewRectShape(x, y, 25, 25))
		}
	}
}

func BenchmarkScene_Build_10000Shapes(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		scene := NewScene()
		for j := 0; j < 10000; j++ {
			x := float32(j%100) * 10
			y := float32(j/100) * 10
			scene.Fill(FillNonZero, IdentityAffine(),
				SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}),
				NewRectShape(x, y, 8, 8))
		}
	}
}

func BenchmarkScene_Build_MixedShapes(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		scene := NewScene()
		for j := 0; j < 333; j++ {
			x := float32(j%20) * 50
			y := float32(j/20) * 50

			// Mix of shape types
			switch j % 3 {
			case 0:
				scene.Fill(FillNonZero, IdentityAffine(),
					SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}),
					NewRectShape(x, y, 40, 40))
			case 1:
				scene.Fill(FillNonZero, IdentityAffine(),
					SolidBrush(gg.RGBA{R: 0, G: 1, B: 0, A: 1}),
					NewCircleShape(x+20, y+20, 20))
			case 2:
				scene.Stroke(DefaultStrokeStyle(), IdentityAffine(),
					SolidBrush(gg.RGBA{R: 0, G: 0, B: 1, A: 1}),
					NewRoundedRectShape(x, y, 40, 40, 5))
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Rendering Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkScene_Render_HD(b *testing.B) {
	// 1920x1080
	r := NewRenderer(1920, 1080)
	if r == nil {
		b.Fatal("Failed to create renderer")
	}
	defer r.Close()

	target := gg.NewPixmap(1920, 1080)

	// Build a scene with various shapes
	scene := NewScene()
	for j := 0; j < 100; j++ {
		x := float32(j%10) * 180
		y := float32(j/10) * 100
		scene.Fill(FillNonZero, IdentityAffine(),
			SolidBrush(gg.RGBA{R: float64(j%3) * 0.5, G: float64(j%5) * 0.25, B: float64(j%7) * 0.15, A: 1}),
			NewRectShape(x, y, 160, 80))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r.MarkAllDirty()
		_ = r.Render(target, scene)
	}
}

func BenchmarkScene_Render_4K(b *testing.B) {
	// 3840x2160
	r := NewRenderer(3840, 2160)
	if r == nil {
		b.Fatal("Failed to create renderer")
	}
	defer r.Close()

	target := gg.NewPixmap(3840, 2160)

	// Build a scene
	scene := NewScene()
	for j := 0; j < 100; j++ {
		x := float32(j%10) * 380
		y := float32(j/10) * 210
		scene.Fill(FillNonZero, IdentityAffine(),
			SolidBrush(gg.RGBA{R: float64(j%3) * 0.5, G: float64(j%5) * 0.25, B: float64(j%7) * 0.15, A: 1}),
			NewRectShape(x, y, 360, 200))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r.MarkAllDirty()
		_ = r.Render(target, scene)
	}
}

func BenchmarkScene_Render_Small(b *testing.B) {
	// 512x512
	r := NewRenderer(512, 512)
	if r == nil {
		b.Fatal("Failed to create renderer")
	}
	defer r.Close()

	target := gg.NewPixmap(512, 512)

	scene := NewScene()
	for j := 0; j < 25; j++ {
		x := float32(j%5) * 100
		y := float32(j/5) * 100
		scene.Fill(FillNonZero, IdentityAffine(),
			SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}),
			NewRectShape(x, y, 90, 90))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r.MarkAllDirty()
		_ = r.Render(target, scene)
	}
}

// ---------------------------------------------------------------------------
// Dirty Region Rendering Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkScene_RenderDirty_10Percent(b *testing.B) {
	r := NewRenderer(1920, 1080)
	if r == nil {
		b.Fatal("Failed to create renderer")
	}
	defer r.Close()

	target := gg.NewPixmap(1920, 1080)

	scene := NewScene()
	for j := 0; j < 100; j++ {
		x := float32(j%10) * 180
		y := float32(j/10) * 100
		scene.Fill(FillNonZero, IdentityAffine(),
			SolidBrush(gg.RGBA{R: float64(j%3) * 0.5, G: float64(j%5) * 0.25, B: float64(j%7) * 0.15, A: 1}),
			NewRectShape(x, y, 160, 80))
	}

	// Initial render
	_ = r.Render(target, scene)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Mark ~10% of area dirty (192x108 region)
		r.MarkDirty(0, 0, 192, 108)
		_ = r.RenderDirty(target, scene, nil)
	}
}

func BenchmarkScene_RenderDirty_SingleTile(b *testing.B) {
	r := NewRenderer(1920, 1080)
	if r == nil {
		b.Fatal("Failed to create renderer")
	}
	defer r.Close()

	target := gg.NewPixmap(1920, 1080)

	scene := NewScene()
	scene.Fill(FillNonZero, IdentityAffine(),
		SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}),
		NewRectShape(0, 0, 1920, 1080))

	// Initial render
	_ = r.Render(target, scene)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Mark just one tile dirty
		r.MarkDirty(0, 0, 64, 64)
		_ = r.RenderDirty(target, scene, nil)
	}
}

// ---------------------------------------------------------------------------
// Cache Performance Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkLayerCache_Hit(b *testing.B) {
	cache := NewLayerCache(64)

	// Pre-populate cache
	pixmap := gg.NewPixmap(100, 100)
	cache.Put(12345, pixmap, 1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = cache.Get(12345)
	}
}

func BenchmarkLayerCache_Miss(b *testing.B) {
	cache := NewLayerCache(64)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = cache.Get(uint64(i))
	}
}

func BenchmarkLayerCache_PutGet(b *testing.B) {
	cache := NewLayerCache(64)
	pixmap := gg.NewPixmap(100, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		hash := uint64(i % 1000) // Cycle through 1000 keys
		cache.Put(hash, pixmap, uint64(i))
		_, _ = cache.Get(hash)
	}
}

// ---------------------------------------------------------------------------
// Encoding Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkEncoding_EncodePath(b *testing.B) {
	enc := NewEncoding()

	// Create gg.Path for encoding
	ggPath := gg.NewPath()
	ggPath.MoveTo(0, 0)
	ggPath.LineTo(100, 0)
	ggPath.LineTo(100, 100)
	ggPath.LineTo(0, 100)
	ggPath.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		enc.Reset()
		enc.EncodePath(ggPath)
	}
}

func BenchmarkEncoding_Clone(b *testing.B) {
	enc := NewEncoding()
	for j := 0; j < 100; j++ {
		path := gg.NewPath()
		path.MoveTo(float64(j*10), float64(j*10))
		path.LineTo(float64(j*10+100), float64(j*10))
		path.LineTo(float64(j*10+100), float64(j*10+100))
		path.Close()
		enc.EncodePath(path)
		enc.EncodeFill(SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}), FillNonZero)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = enc.Clone()
	}
}

// ---------------------------------------------------------------------------
// Decoder Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkDecoder_Iterate(b *testing.B) {
	enc := NewEncoding()
	for j := 0; j < 100; j++ {
		path := gg.NewPath()
		path.MoveTo(float64(j*10), float64(j*10))
		path.LineTo(float64(j*10+100), float64(j*10))
		path.LineTo(float64(j*10+100), float64(j*10+100))
		path.Close()
		enc.EncodePath(path)
		enc.EncodeFill(SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}), FillNonZero)
	}

	b.ResetTimer()
	b.ReportAllocs()

	dec := NewDecoder(enc)
	for i := 0; i < b.N; i++ {
		dec.Reset(enc)
		for dec.Next() {
			switch dec.Tag() {
			case TagMoveTo:
				_, _ = dec.MoveTo()
			case TagLineTo:
				_, _ = dec.LineTo()
			case TagFill:
				_, _ = dec.Fill()
			}
		}
	}
}

// ---------------------------------------------------------------------------
// SceneBuilder Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkSceneBuilder_Build_100Shapes(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		builder := NewSceneBuilder()
		for j := 0; j < 100; j++ {
			x := float32(j%10) * 50
			y := float32(j/10) * 50
			builder.FillRect(x, y, 40, 40, SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}))
		}
		_ = builder.Build()
	}
}

func BenchmarkSceneBuilder_WithLayers(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		builder := NewSceneBuilder()
		builder.FillRect(0, 0, 100, 100, SolidBrush(gg.RGBA{R: 1, G: 1, B: 1, A: 1}))

		builder.Layer(BlendMultiply, 0.5, nil, func(lb *SceneBuilder) {
			lb.FillCircle(50, 50, 30, SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}))
		})

		builder.Layer(BlendScreen, 0.75, nil, func(lb *SceneBuilder) {
			lb.FillRect(25, 25, 50, 50, SolidBrush(gg.RGBA{R: 0, G: 1, B: 0, A: 1}))
		})

		_ = builder.Build()
	}
}

// ---------------------------------------------------------------------------
// Path Containment Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkPath_Contains_Rectangle(b *testing.B) {
	path := NewPath().Rectangle(100, 100, 200, 200)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = path.Contains(200, 200) // Inside
		_ = path.Contains(50, 50)   // Outside
	}
}

func BenchmarkPath_Contains_Circle(b *testing.B) {
	path := NewPath().Circle(200, 200, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = path.Contains(200, 200) // Inside (center)
		_ = path.Contains(50, 50)   // Outside
	}
}

func BenchmarkPath_Contains_ComplexPath(b *testing.B) {
	// Complex path with many segments
	path := NewPath()
	path.MoveTo(0, 0)
	for j := 0; j < 100; j++ {
		x := float32(j%10) * 50
		y := float32(j/10) * 50
		path.LineTo(x, y)
	}
	path.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = path.Contains(250, 250)
	}
}

// ---------------------------------------------------------------------------
// Memory Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkScene_Memory_Large(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		scene := NewScene()
		// Build a large scene
		for j := 0; j < 1000; j++ {
			x := float32(j%32) * 30
			y := float32(j/32) * 30
			scene.Fill(FillNonZero, IdentityAffine(),
				SolidBrush(gg.RGBA{R: float64(j%256) / 255, G: float64(j%128) / 127, B: float64(j%64) / 63, A: 1}),
				NewCircleShape(x+15, y+15, 12))
		}
		// Get encoding to force flattening
		_ = scene.Encoding()
	}
}

func BenchmarkScenePool_GetPut(b *testing.B) {
	pool := NewScenePool()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		scene := pool.Get()
		scene.Fill(FillNonZero, IdentityAffine(),
			SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}),
			NewRectShape(0, 0, 100, 100))
		pool.Put(scene)
	}
}
