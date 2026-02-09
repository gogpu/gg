package gpu

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
	"github.com/gogpu/wgpu/core"
)

// =============================================================================
// Backend Integration Tests
// =============================================================================

// TestBackendIntegration tests the full rendering pipeline from backend to scene.
func TestBackendIntegration(t *testing.T) {
	b := NewNativeBackend()

	if err := b.Init(); err != nil {
		t.Skipf("GPU not available: %v (expected in CI/test environments)", err)
	}
	defer b.Close()

	// Create a simple scene
	s := scene.NewScene()
	rect := scene.NewRectShape(10, 10, 80, 80)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), rect)

	// Create target pixmap
	pm := gg.NewPixmap(100, 100)

	// Render scene
	err := b.RenderScene(pm, s)
	// Note: Currently returns nil (GPU pipeline runs as stubs)
	// or error if GPU is truly unavailable
	if err != nil {
		t.Logf("RenderScene returned: %v (expected until full GPU support)", err)
	}
}

// TestBackendWithComplexScene tests rendering a more complex scene.
func TestBackendWithComplexScene(t *testing.T) {
	b := NewNativeBackend()

	if err := b.Init(); err != nil {
		t.Skipf("GPU not available: %v", err)
	}
	defer b.Close()

	// Create scene with multiple elements
	s := scene.NewScene()

	// Background
	bg := scene.NewRectShape(0, 0, 200, 200)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.White), bg)

	// Circle with transform
	circle := scene.NewCircleShape(100, 100, 50)
	s.Fill(scene.FillEvenOdd, scene.TranslateAffine(10, 10), scene.SolidBrush(gg.Blue), circle)

	// Rectangle with different fill rule
	rect := scene.NewRectShape(50, 50, 100, 100)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Green), rect)

	// Render
	pm := gg.NewPixmap(200, 200)
	err := b.RenderScene(pm, s)
	if err != nil {
		t.Logf("RenderScene error: %v", err)
	}
}

// TestBackendWithLayers tests layer push/pop operations.
func TestBackendWithLayers(t *testing.T) {
	b := NewNativeBackend()

	if err := b.Init(); err != nil {
		t.Skipf("GPU not available: %v", err)
	}
	defer b.Close()

	// Create scene with layers
	s := scene.NewScene()

	// Base layer content
	rect := scene.NewRectShape(0, 0, 100, 100)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.White), rect)

	// Push layer with blend mode
	s.PushLayer(scene.BlendMultiply, 0.8, nil)

	// Layer content
	circle := scene.NewCircleShape(50, 50, 30)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), circle)

	// Pop layer
	s.PopLayer()

	// Render
	pm := gg.NewPixmap(100, 100)
	err := b.RenderScene(pm, s)
	if err != nil {
		t.Logf("RenderScene with layers error: %v", err)
	}
}

// TestBackendWithNestedLayers tests nested layer operations.
func TestBackendWithNestedLayers(t *testing.T) {
	b := NewNativeBackend()

	if err := b.Init(); err != nil {
		t.Skipf("GPU not available: %v", err)
	}
	defer b.Close()

	s := scene.NewScene()

	// Background
	bg := scene.NewRectShape(0, 0, 100, 100)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.White), bg)

	// First layer
	s.PushLayer(scene.BlendScreen, 0.9, nil)
	rect1 := scene.NewRectShape(10, 10, 80, 80)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), rect1)

	// Nested layer
	s.PushLayer(scene.BlendMultiply, 0.7, nil)
	circle := scene.NewCircleShape(50, 50, 25)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Blue), circle)
	s.PopLayer()

	s.PopLayer()

	pm := gg.NewPixmap(100, 100)
	err := b.RenderScene(pm, s)
	if err != nil {
		t.Logf("RenderScene with nested layers error: %v", err)
	}
}

// =============================================================================
// Memory Integration Tests
// =============================================================================

// TestMemoryIntegration tests texture allocation under load.
func TestMemoryIntegration(t *testing.T) {
	mm := NewMemoryManager(nil, MemoryManagerConfig{
		MaxMemoryMB:       64,
		EvictionThreshold: 0.8,
	})
	defer mm.Close()

	// Allocate multiple textures
	var textures []*GPUTexture
	for i := 0; i < 20; i++ {
		tex, err := mm.AllocTexture(TextureConfig{
			Width:  256,
			Height: 256,
			Format: TextureFormatRGBA8,
			Label:  "test-texture",
		})
		if err != nil {
			t.Logf("Allocation %d failed: %v (may be budget limit)", i, err)
			break
		}
		textures = append(textures, tex)
	}

	if len(textures) == 0 {
		t.Fatal("Should have allocated at least one texture")
	}

	t.Logf("Allocated %d textures", len(textures))

	// Check stats
	stats := mm.Stats()
	t.Logf("Memory stats: %s", stats.String())

	if stats.TextureCount != len(textures) {
		t.Errorf("TextureCount = %d, want %d", stats.TextureCount, len(textures))
	}

	// Touch some textures (simulating use)
	for i := 0; i < len(textures) && i < 5; i++ {
		mm.TouchTexture(textures[i])
	}

	// Free half
	for i := 0; i < len(textures)/2; i++ {
		if err := mm.FreeTexture(textures[i]); err != nil {
			t.Errorf("FreeTexture failed: %v", err)
		}
	}

	// Check stats again
	stats = mm.Stats()
	expectedCount := len(textures) - len(textures)/2
	if stats.TextureCount != expectedCount {
		t.Errorf("After free TextureCount = %d, want %d",
			stats.TextureCount, expectedCount)
	}
}

// TestMemoryWithEviction tests that LRU eviction works correctly.
func TestMemoryWithEviction(t *testing.T) {
	// Small budget to force eviction
	mm := NewMemoryManager(nil, MemoryManagerConfig{
		MaxMemoryMB:       16,
		EvictionThreshold: 0.5, // 8 MB threshold
	})
	defer mm.Close()

	// Track allocations
	allocatedCount := 0

	// Allocate until we hit budget - with LRU eviction, older allocations
	// may be evicted to make room for new ones
	for i := 0; i < 20; i++ {
		_, err := mm.AllocTexture(TextureConfig{
			Width:  512,
			Height: 512,
			Format: TextureFormatRGBA8, // 1 MB each
		})
		if err != nil {
			t.Logf("Allocation %d failed (expected at budget): %v", i, err)
			break
		}
		allocatedCount++
	}

	stats := mm.Stats()
	t.Logf("After filling: %s", stats.String())

	// Verify that the used memory never exceeds budget
	if stats.UsedBytes > stats.TotalBytes {
		t.Errorf("Used bytes %d exceeds total budget %d",
			stats.UsedBytes, stats.TotalBytes)
	}

	// Verify we have reasonable texture count (16 max due to 1MB each in 16MB budget)
	if stats.TextureCount > 16 {
		t.Errorf("Texture count %d exceeds budget capacity of 16", stats.TextureCount)
	}

	// Log eviction stats
	if stats.EvictionCount > 0 {
		t.Logf("Eviction occurred: %d textures evicted", stats.EvictionCount)
	}
}

// =============================================================================
// Pipeline Integration Tests
// =============================================================================

// TestPipelineIntegration tests the full pipeline creation and caching.
func TestPipelineIntegration(t *testing.T) {
	shaders := &ShaderModules{
		Blit:      ShaderModuleID(1),
		Blend:     ShaderModuleID(2),
		Strip:     ShaderModuleID(3),
		Composite: ShaderModuleID(4),
	}

	var testDeviceID = (*DeviceID)(nil)
	pc, err := NewPipelineCache(testDeviceID.Zero(), shaders)
	if err != nil {
		t.Fatalf("NewPipelineCache failed: %v", err)
	}
	defer pc.Close()

	// Test all blend modes are supported
	blendModes := []scene.BlendMode{
		scene.BlendNormal,
		scene.BlendMultiply,
		scene.BlendScreen,
		scene.BlendOverlay,
		scene.BlendDarken,
		scene.BlendLighten,
		scene.BlendColorDodge,
		scene.BlendColorBurn,
		scene.BlendHardLight,
		scene.BlendSoftLight,
		scene.BlendDifference,
		scene.BlendExclusion,
		scene.BlendHue,
		scene.BlendSaturation,
		scene.BlendColor,
		scene.BlendLuminosity,
		scene.BlendClear,
		scene.BlendCopy,
		scene.BlendDestination,
		scene.BlendSourceOver,
		scene.BlendDestinationOver,
		scene.BlendSourceIn,
		scene.BlendDestinationIn,
		scene.BlendSourceOut,
		scene.BlendDestinationOut,
		scene.BlendSourceAtop,
		scene.BlendDestinationAtop,
		scene.BlendXor,
		scene.BlendPlus,
	}

	for _, mode := range blendModes {
		p := pc.GetBlendPipeline(mode)
		if p == InvalidPipelineID {
			t.Errorf("GetBlendPipeline(%v) returned invalid pipeline", mode)
		}
	}

	// Verify caching works
	initialCount := pc.BlendPipelineCount()
	for _, mode := range blendModes {
		_ = pc.GetBlendPipeline(mode)
	}
	if pc.BlendPipelineCount() != initialCount {
		t.Error("Blend pipelines not being cached")
	}
}

// DeviceID is a helper type to get zero value for tests.
type DeviceID struct{}

// Zero returns a zero-value core.DeviceID.
func (d *DeviceID) Zero() core.DeviceID {
	var id core.DeviceID
	return id
}

// =============================================================================
// Full Pipeline Integration Tests
// =============================================================================

// TestFullRenderPipeline tests the complete render pipeline data flow.
func TestFullRenderPipeline(t *testing.T) {
	b := NewNativeBackend()

	if err := b.Init(); err != nil {
		t.Skipf("GPU not available: %v", err)
	}
	defer b.Close()

	// Test different scene complexities
	scenarios := []struct {
		name   string
		width  int
		height int
		setup  func(s *scene.Scene)
	}{
		{
			name:   "simple_rect",
			width:  100,
			height: 100,
			setup: func(s *scene.Scene) {
				rect := scene.NewRectShape(10, 10, 80, 80)
				s.Fill(scene.FillNonZero, scene.IdentityAffine(),
					scene.SolidBrush(gg.Red), rect)
			},
		},
		{
			name:   "circle_with_layer",
			width:  200,
			height: 200,
			setup: func(s *scene.Scene) {
				bg := scene.NewRectShape(0, 0, 200, 200)
				s.Fill(scene.FillNonZero, scene.IdentityAffine(),
					scene.SolidBrush(gg.White), bg)

				s.PushLayer(scene.BlendMultiply, 0.8, nil)
				circle := scene.NewCircleShape(100, 100, 75)
				s.Fill(scene.FillNonZero, scene.IdentityAffine(),
					scene.SolidBrush(gg.Blue), circle)
				s.PopLayer()
			},
		},
		{
			name:   "multiple_shapes",
			width:  256,
			height: 256,
			setup: func(s *scene.Scene) {
				// Add 10 shapes
				for i := 0; i < 10; i++ {
					x := float32(i * 20)
					rect := scene.NewRectShape(x, x, 50, 50)
					s.Fill(scene.FillNonZero, scene.IdentityAffine(),
						scene.SolidBrush(gg.RGB(float64(i)*0.1, 0.4, 0.8)), rect)
				}
			},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			s := scene.NewScene()
			sc.setup(s)

			pm := gg.NewPixmap(sc.width, sc.height)
			err := b.RenderScene(pm, s)
			if err != nil {
				t.Logf("RenderScene error: %v (may be expected)", err)
			}
		})
	}
}

// =============================================================================
// Concurrent Scene Rendering Tests
// =============================================================================

// TestBackendConcurrentSceneRendering tests concurrent scene rendering.
func TestBackendConcurrentSceneRendering(t *testing.T) {
	b := NewNativeBackend()

	if err := b.Init(); err != nil {
		t.Skipf("GPU not available: %v", err)
	}
	defer b.Close()

	const goroutines = 5

	done := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			s := scene.NewScene()
			rect := scene.NewRectShape(float32(idx*10), float32(idx*10), 50, 50)
			s.Fill(scene.FillNonZero, scene.IdentityAffine(),
				scene.SolidBrush(gg.Red), rect)

			pm := gg.NewPixmap(100, 100)
			err := b.RenderScene(pm, s)
			done <- err
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < goroutines; i++ {
		err := <-done
		if err != nil {
			t.Logf("Concurrent render %d error: %v (may be expected)", i, err)
		}
	}
}
