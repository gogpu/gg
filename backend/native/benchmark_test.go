package native

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/backend"
	"github.com/gogpu/gg/scene"
	"github.com/gogpu/wgpu/core"
)

// =============================================================================
// Pipeline Benchmarks
// =============================================================================

// BenchmarkPipelineCreation benchmarks pipeline cache creation.
func BenchmarkPipelineCreation(b *testing.B) {
	shaders := &ShaderModules{
		Blit:      ShaderModuleID(1),
		Blend:     ShaderModuleID(2),
		Strip:     ShaderModuleID(3),
		Composite: ShaderModuleID(4),
	}

	var deviceID core.DeviceID

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pc, err := NewPipelineCache(deviceID, shaders)
		if err != nil {
			b.Fatalf("NewPipelineCache failed: %v", err)
		}
		pc.Close()
	}
}

// BenchmarkBlendPipelineCache benchmarks blend pipeline caching.
func BenchmarkBlendPipelineCache(b *testing.B) {
	shaders := &ShaderModules{
		Blit:      ShaderModuleID(1),
		Blend:     ShaderModuleID(2),
		Strip:     ShaderModuleID(3),
		Composite: ShaderModuleID(4),
	}

	var deviceID core.DeviceID
	pc, err := NewPipelineCache(deviceID, shaders)
	if err != nil {
		b.Fatalf("NewPipelineCache failed: %v", err)
	}
	defer pc.Close()

	// Warm up all blend modes
	allModes := []scene.BlendMode{
		scene.BlendNormal, scene.BlendMultiply, scene.BlendScreen,
		scene.BlendOverlay, scene.BlendSourceOver, scene.BlendXor,
	}
	for _, mode := range allModes {
		_ = pc.GetBlendPipeline(mode)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Access cached pipelines
		for _, mode := range allModes {
			_ = pc.GetBlendPipeline(mode)
		}
	}
}

// =============================================================================
// Memory Manager Benchmarks
// =============================================================================

// BenchmarkMemoryAllocation benchmarks texture allocation.
func BenchmarkMemoryAllocation(b *testing.B) {
	sizes := []struct {
		name          string
		width, height int
	}{
		{"256x256", 256, 256},
		{"512x512", 512, 512},
		{"1024x1024", 1024, 1024},
	}

	for _, sz := range sizes {
		b.Run(sz.name, func(b *testing.B) {
			mm := NewMemoryManager(nil, MemoryManagerConfig{
				MaxMemoryMB: 512,
			})
			defer mm.Close()

			config := TextureConfig{
				Width:  sz.width,
				Height: sz.height,
				Format: TextureFormatRGBA8,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tex, err := mm.AllocTexture(config)
				if err != nil {
					b.Fatalf("Allocation failed: %v", err)
				}
				_ = mm.FreeTexture(tex)
			}
		})
	}
}

// BenchmarkMemoryTouch benchmarks LRU touch operations.
func BenchmarkMemoryTouch(b *testing.B) {
	mm := NewMemoryManager(nil, MemoryManagerConfig{
		MaxMemoryMB: 128,
	})
	defer mm.Close()

	// Allocate some textures
	var textures []*GPUTexture
	for i := 0; i < 50; i++ {
		tex, err := mm.AllocTexture(TextureConfig{
			Width:  128,
			Height: 128,
			Format: TextureFormatRGBA8,
		})
		if err != nil {
			break
		}
		textures = append(textures, tex)
	}

	if len(textures) == 0 {
		b.Skip("No textures allocated")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mm.TouchTexture(textures[i%len(textures)])
	}
}

// =============================================================================
// Full Scene Benchmarks
// =============================================================================

// BenchmarkSceneCreation benchmarks scene creation and encoding.
func BenchmarkSceneCreation(b *testing.B) {
	benchmarks := []struct {
		name  string
		setup func(s *scene.Scene)
	}{
		{"single_rect", func(s *scene.Scene) {
			rect := scene.NewRectShape(10, 10, 80, 80)
			s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), rect)
		}},
		{"10_rects", func(s *scene.Scene) {
			for i := 0; i < 10; i++ {
				rect := scene.NewRectShape(float32(i*10), float32(i*10), 50, 50)
				s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), rect)
			}
		}},
		{"100_rects", func(s *scene.Scene) {
			for i := 0; i < 100; i++ {
				x := float32((i % 10) * 50)
				y := float32((i / 10) * 50)
				rect := scene.NewRectShape(x, y, 40, 40)
				s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), rect)
			}
		}},
		{"with_layers", func(s *scene.Scene) {
			rect := scene.NewRectShape(0, 0, 100, 100)
			s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.White), rect)

			s.PushLayer(scene.BlendMultiply, 0.8, nil)
			circle := scene.NewCircleShape(50, 50, 30)
			s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), circle)
			s.PopLayer()
		}},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				s := scene.NewScene()
				bm.setup(s)
				_ = s.Encoding()
			}
		})
	}
}

// BenchmarkBackendInit benchmarks backend initialization.
func BenchmarkBackendInit(b *testing.B) {
	for i := 0; i < b.N; i++ {
		be := NewNativeBackend()
		if err := be.Init(); err != nil {
			b.Skipf("GPU not available: %v", err)
		}
		be.Close()
	}
}

// =============================================================================
// GPU vs Software Backend Comparison Benchmarks
// =============================================================================

// BenchmarkClear1080p compares clearing a 1080p canvas.
func BenchmarkClear1080p(b *testing.B) {
	runBackendComparison(b, 1920, 1080, func(s *scene.Scene) {
		rect := scene.NewRectShape(0, 0, 1920, 1080)
		s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.White), rect)
	})
}

// BenchmarkRect100 compares rendering 100 rectangles.
func BenchmarkRect100(b *testing.B) {
	runBackendComparison(b, 800, 600, func(s *scene.Scene) {
		for i := 0; i < 100; i++ {
			x := float32((i % 10) * 70)
			y := float32((i / 10) * 50)
			rect := scene.NewRectShape(x+10, y+10, 50, 30)
			color := gg.RGBA{
				R: float64(i%256) / 255.0,
				G: float64((i*7)%256) / 255.0,
				B: float64((i*13)%256) / 255.0,
				A: 1,
			}
			s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(color), rect)
		}
	})
}

// BenchmarkCircle50 compares rendering 50 circles.
func BenchmarkCircle50(b *testing.B) {
	runBackendComparison(b, 800, 600, func(s *scene.Scene) {
		for i := 0; i < 50; i++ {
			x := float32(50 + (i%10)*75)
			y := float32(50 + (i/10)*100)
			r := float32(20 + i%15)
			circle := scene.NewCircleShape(x, y, r)
			color := gg.RGBA{R: 0.8, G: 0.2, B: 0.4, A: 0.8}
			s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(color), circle)
		}
	})
}

// BenchmarkPathComplex compares rendering complex shapes (many overlapping circles).
func BenchmarkPathComplex(b *testing.B) {
	runBackendComparison(b, 800, 600, func(s *scene.Scene) {
		// Create many overlapping circles to simulate complex path rendering load
		for i := 0; i < 10; i++ {
			baseX := float32(50 + i*70)
			for j := 0; j < 20; j++ {
				x := baseX + float32(j*3)
				y := float32(100 + (j%4)*20)
				r := float32(10 + j%5)
				circle := scene.NewCircleShape(x, y, r)
				s.Fill(scene.FillNonZero, scene.IdentityAffine(),
					scene.SolidBrush(gg.RGBA{R: 0.3, G: 0.6, B: 0.9, A: 0.7}), circle)
			}
		}
	})
}

// BenchmarkLayers4 compares rendering with 4 blend layers.
func BenchmarkLayers4(b *testing.B) {
	runBackendComparison(b, 512, 512, func(s *scene.Scene) {
		// Background
		bg := scene.NewRectShape(0, 0, 512, 512)
		s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.White), bg)

		// Layer 1: Multiply
		s.PushLayer(scene.BlendMultiply, 0.9, nil)
		rect1 := scene.NewRectShape(50, 50, 200, 200)
		s.Fill(scene.FillNonZero, scene.IdentityAffine(),
			scene.SolidBrush(gg.RGBA{R: 1, G: 0.8, B: 0.2, A: 1}), rect1)
		s.PopLayer()

		// Layer 2: Screen
		s.PushLayer(scene.BlendScreen, 0.8, nil)
		circle1 := scene.NewCircleShape(300, 200, 100)
		s.Fill(scene.FillNonZero, scene.IdentityAffine(),
			scene.SolidBrush(gg.RGBA{R: 0.2, G: 0.6, B: 1, A: 1}), circle1)
		s.PopLayer()

		// Layer 3: Overlay
		s.PushLayer(scene.BlendOverlay, 0.7, nil)
		rect2 := scene.NewRectShape(150, 300, 250, 150)
		s.Fill(scene.FillNonZero, scene.IdentityAffine(),
			scene.SolidBrush(gg.RGBA{R: 0.9, G: 0.3, B: 0.5, A: 1}), rect2)
		s.PopLayer()

		// Layer 4: Darken
		s.PushLayer(scene.BlendDarken, 0.6, nil)
		circle2 := scene.NewCircleShape(256, 256, 150)
		s.Fill(scene.FillNonZero, scene.IdentityAffine(),
			scene.SolidBrush(gg.RGBA{R: 0.4, G: 0.8, B: 0.4, A: 1}), circle2)
		s.PopLayer()
	})
}

// BenchmarkBlendModes compares different blend mode operations.
func BenchmarkBlendModes(b *testing.B) {
	modes := []struct {
		name string
		mode scene.BlendMode
	}{
		{"Normal", scene.BlendNormal},
		{"Multiply", scene.BlendMultiply},
		{"Screen", scene.BlendScreen},
		{"Overlay", scene.BlendOverlay},
		{"SourceOver", scene.BlendSourceOver},
	}

	for _, m := range modes {
		b.Run(m.name, func(b *testing.B) {
			runBackendComparison(b, 400, 400, func(s *scene.Scene) {
				bg := scene.NewRectShape(0, 0, 400, 400)
				s.Fill(scene.FillNonZero, scene.IdentityAffine(),
					scene.SolidBrush(gg.RGBA{R: 0.5, G: 0.5, B: 0.5, A: 1}), bg)

				s.PushLayer(m.mode, 0.8, nil)
				for i := 0; i < 20; i++ {
					x := float32((i % 5) * 80)
					y := float32((i / 5) * 100)
					rect := scene.NewRectShape(x+10, y+10, 60, 80)
					s.Fill(scene.FillNonZero, scene.IdentityAffine(),
						scene.SolidBrush(gg.RGBA{R: 0.9, G: 0.3, B: 0.2, A: 1}), rect)
				}
				s.PopLayer()
			})
		})
	}
}

// runBackendComparison runs a benchmark comparing GPU and Software backends.
func runBackendComparison(b *testing.B, width, height int, setup func(s *scene.Scene)) {
	// GPU backend benchmark
	b.Run("GPU", func(b *testing.B) {
		be := NewNativeBackend()
		if err := be.Init(); err != nil {
			b.Skipf("GPU not available: %v", err)
		}
		defer be.Close()

		pm := gg.NewPixmap(width, height)

		// Build scene once
		sceneData := scene.NewScene()
		setup(sceneData)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// GPU rendering (currently stubs but measures tessellation + pipeline)
			_ = be.RenderScene(pm, sceneData)
		}
	})

	// Software backend benchmark
	b.Run("Software", func(b *testing.B) {
		be := backend.Get(backend.BackendSoftware)
		if be == nil {
			b.Skip("Software backend not available")
		}
		if err := be.Init(); err != nil {
			b.Fatalf("Software backend init failed: %v", err)
		}
		defer be.Close()

		pm := gg.NewPixmap(width, height)

		// Build scene once
		sceneData := scene.NewScene()
		setup(sceneData)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = be.RenderScene(pm, sceneData)
		}
	})
}
