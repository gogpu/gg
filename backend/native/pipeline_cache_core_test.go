package native

import (
	"errors"
	"sync"
	"testing"

	"github.com/gogpu/wgpu/types"
)

// =============================================================================
// Test Helpers
// =============================================================================

// mockShaderModule creates a test shader module with the given code hash.
func mockShaderModule(id uint64, label string, codeHash uint64) *ShaderModule {
	return &ShaderModule{
		id:       id,
		label:    label,
		codeHash: codeHash,
	}
}

// mockRenderPipelineDescriptor creates a test render pipeline descriptor.
func mockRenderPipelineDescriptor(vertexHash, fragmentHash uint64) *RenderPipelineDescriptor {
	return &RenderPipelineDescriptor{
		Label:              "test-pipeline",
		VertexShader:       mockShaderModule(1, "vertex", vertexHash),
		VertexEntryPoint:   "vs_main",
		FragmentShader:     mockShaderModule(2, "fragment", fragmentHash),
		FragmentEntryPoint: "fs_main",
		VertexBufferLayouts: []VertexBufferLayout{
			{
				ArrayStride: 32,
				StepMode:    types.VertexStepModeVertex,
				Attributes: []VertexAttribute{
					{ShaderLocation: 0, Format: types.VertexFormatFloat32x3, Offset: 0},
					{ShaderLocation: 1, Format: types.VertexFormatFloat32x2, Offset: 12},
				},
			},
		},
		PrimitiveTopology: types.PrimitiveTopologyTriangleList,
		FrontFace:         types.FrontFaceCCW,
		CullMode:          types.CullModeBack,
		ColorFormat:       types.TextureFormatBGRA8Unorm,
		DepthFormat:       types.TextureFormatDepth24PlusStencil8,
		DepthWriteEnabled: true,
		DepthCompare:      types.CompareFunctionLess,
		SampleCount:       1,
	}
}

// mockComputePipelineDescriptor creates a test compute pipeline descriptor.
func mockComputePipelineDescriptor(codeHash uint64) *ComputePipelineDescriptor {
	return &ComputePipelineDescriptor{
		Label:         "test-compute",
		ComputeShader: mockShaderModule(3, "compute", codeHash),
		EntryPoint:    "main",
	}
}

// =============================================================================
// PipelineCacheCore Tests
// =============================================================================

func TestNewPipelineCacheCore(t *testing.T) {
	cache := NewPipelineCacheCore()

	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	if cache.Size() != 0 {
		t.Errorf("expected empty cache, got size %d", cache.Size())
	}

	hits, misses := cache.Stats()
	if hits != 0 || misses != 0 {
		t.Errorf("expected zero stats, got hits=%d, misses=%d", hits, misses)
	}
}

func TestPipelineCacheCore_GetOrCreateRenderPipeline(t *testing.T) {
	cache := NewPipelineCacheCore()
	desc := mockRenderPipelineDescriptor(100, 200)

	// First call - cache miss, creates pipeline
	pipeline1, err := cache.GetOrCreateRenderPipeline(nil, desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pipeline1 == nil {
		t.Fatal("expected non-nil pipeline")
	}

	hits, misses := cache.Stats()
	if hits != 0 {
		t.Errorf("expected 0 hits, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("expected 1 miss, got %d", misses)
	}

	// Second call with same descriptor - cache hit
	pipeline2, err := cache.GetOrCreateRenderPipeline(nil, desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pipeline2 != pipeline1 {
		t.Error("expected same pipeline instance from cache")
	}

	hits, misses = cache.Stats()
	if hits != 1 {
		t.Errorf("expected 1 hit, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("expected 1 miss, got %d", misses)
	}

	// Third call with different descriptor - cache miss
	desc2 := mockRenderPipelineDescriptor(100, 300) // Different fragment shader
	pipeline3, err := cache.GetOrCreateRenderPipeline(nil, desc2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pipeline3 == pipeline1 {
		t.Error("expected different pipeline for different descriptor")
	}

	hits, misses = cache.Stats()
	if hits != 1 {
		t.Errorf("expected 1 hit, got %d", hits)
	}
	if misses != 2 {
		t.Errorf("expected 2 misses, got %d", misses)
	}
}

func TestPipelineCacheCore_GetOrCreateComputePipeline(t *testing.T) {
	cache := NewPipelineCacheCore()
	desc := mockComputePipelineDescriptor(500)

	// First call - cache miss
	pipeline1, err := cache.GetOrCreateComputePipeline(nil, desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pipeline1 == nil {
		t.Fatal("expected non-nil pipeline")
	}

	// Second call - cache hit
	pipeline2, err := cache.GetOrCreateComputePipeline(nil, desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pipeline2 != pipeline1 {
		t.Error("expected same pipeline instance from cache")
	}

	hits, misses := cache.Stats()
	if hits != 1 {
		t.Errorf("expected 1 hit, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("expected 1 miss, got %d", misses)
	}
}

func TestPipelineCacheCore_NilDescriptor(t *testing.T) {
	cache := NewPipelineCacheCore()

	// Nil render descriptor
	_, err := cache.GetOrCreateRenderPipeline(nil, nil)
	if !errors.Is(err, ErrPipelineCacheNilDescriptor) {
		t.Errorf("expected ErrPipelineCacheNilDescriptor, got %v", err)
	}

	// Nil compute descriptor
	_, err = cache.GetOrCreateComputePipeline(nil, nil)
	if !errors.Is(err, ErrPipelineCacheNilDescriptor) {
		t.Errorf("expected ErrPipelineCacheNilDescriptor, got %v", err)
	}
}

func TestPipelineCacheCore_NilShader(t *testing.T) {
	cache := NewPipelineCacheCore()

	// Nil vertex shader
	desc := &RenderPipelineDescriptor{
		VertexShader:   nil,
		FragmentShader: mockShaderModule(1, "fragment", 100),
	}
	_, err := cache.GetOrCreateRenderPipeline(nil, desc)
	if !errors.Is(err, ErrPipelineCacheNilShader) {
		t.Errorf("expected ErrPipelineCacheNilShader, got %v", err)
	}

	// Nil compute shader
	computeDesc := &ComputePipelineDescriptor{
		ComputeShader: nil,
	}
	_, err = cache.GetOrCreateComputePipeline(nil, computeDesc)
	if !errors.Is(err, ErrPipelineCacheNilShader) {
		t.Errorf("expected ErrPipelineCacheNilShader, got %v", err)
	}
}

func TestPipelineCacheCore_Size(t *testing.T) {
	cache := NewPipelineCacheCore()

	if cache.Size() != 0 {
		t.Errorf("expected size 0, got %d", cache.Size())
	}

	// Add render pipeline
	desc1 := mockRenderPipelineDescriptor(100, 200)
	_, err := cache.GetOrCreateRenderPipeline(nil, desc1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cache.Size() != 1 {
		t.Errorf("expected size 1, got %d", cache.Size())
	}
	if cache.RenderPipelineCount() != 1 {
		t.Errorf("expected render count 1, got %d", cache.RenderPipelineCount())
	}
	if cache.ComputePipelineCount() != 0 {
		t.Errorf("expected compute count 0, got %d", cache.ComputePipelineCount())
	}

	// Add compute pipeline
	desc2 := mockComputePipelineDescriptor(500)
	_, err = cache.GetOrCreateComputePipeline(nil, desc2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cache.Size() != 2 {
		t.Errorf("expected size 2, got %d", cache.Size())
	}
	if cache.ComputePipelineCount() != 1 {
		t.Errorf("expected compute count 1, got %d", cache.ComputePipelineCount())
	}
}

func TestPipelineCacheCore_Clear(t *testing.T) {
	cache := NewPipelineCacheCore()

	// Add some pipelines
	desc1 := mockRenderPipelineDescriptor(100, 200)
	desc2 := mockComputePipelineDescriptor(500)

	_, _ = cache.GetOrCreateRenderPipeline(nil, desc1)
	_, _ = cache.GetOrCreateComputePipeline(nil, desc2)

	if cache.Size() != 2 {
		t.Errorf("expected size 2 before clear, got %d", cache.Size())
	}

	// Clear
	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", cache.Size())
	}

	hits, misses := cache.Stats()
	if hits != 0 || misses != 0 {
		t.Errorf("expected zero stats after clear, got hits=%d, misses=%d", hits, misses)
	}
}

func TestPipelineCacheCore_DestroyAll(t *testing.T) {
	cache := NewPipelineCacheCore()

	// Add some pipelines
	desc1 := mockRenderPipelineDescriptor(100, 200)
	desc2 := mockComputePipelineDescriptor(500)

	pipeline1, _ := cache.GetOrCreateRenderPipeline(nil, desc1)
	pipeline2, _ := cache.GetOrCreateComputePipeline(nil, desc2)

	// DestroyAll
	cache.DestroyAll()

	if cache.Size() != 0 {
		t.Errorf("expected size 0 after destroy, got %d", cache.Size())
	}

	// Pipelines should be destroyed
	if !pipeline1.IsDestroyed() {
		t.Error("expected render pipeline to be destroyed")
	}
	if !pipeline2.IsDestroyed() {
		t.Error("expected compute pipeline to be destroyed")
	}
}

func TestPipelineCacheCore_HitRate(t *testing.T) {
	cache := NewPipelineCacheCore()

	// No requests - hit rate should be 0
	if rate := cache.HitRate(); rate != 0.0 {
		t.Errorf("expected hit rate 0.0, got %f", rate)
	}

	// 1 miss, 0 hits
	desc := mockRenderPipelineDescriptor(100, 200)
	_, _ = cache.GetOrCreateRenderPipeline(nil, desc)

	if rate := cache.HitRate(); rate != 0.0 {
		t.Errorf("expected hit rate 0.0, got %f", rate)
	}

	// 1 hit, 1 miss = 50%
	_, _ = cache.GetOrCreateRenderPipeline(nil, desc)

	rate := cache.HitRate()
	if rate < 0.49 || rate > 0.51 {
		t.Errorf("expected hit rate ~0.5, got %f", rate)
	}

	// 2 more hits = 3 hits, 1 miss = 75%
	_, _ = cache.GetOrCreateRenderPipeline(nil, desc)
	_, _ = cache.GetOrCreateRenderPipeline(nil, desc)

	rate = cache.HitRate()
	if rate < 0.74 || rate > 0.76 {
		t.Errorf("expected hit rate ~0.75, got %f", rate)
	}
}

// =============================================================================
// Hash Function Tests
// =============================================================================

func TestHashRenderPipelineDescriptor_Deterministic(t *testing.T) {
	desc := mockRenderPipelineDescriptor(100, 200)

	hash1 := HashRenderPipelineDescriptor(desc)
	hash2 := HashRenderPipelineDescriptor(desc)

	if hash1 != hash2 {
		t.Errorf("hash not deterministic: %d != %d", hash1, hash2)
	}
}

func TestHashRenderPipelineDescriptor_DifferentShaders(t *testing.T) {
	desc1 := mockRenderPipelineDescriptor(100, 200)
	desc2 := mockRenderPipelineDescriptor(100, 201) // Different fragment shader

	hash1 := HashRenderPipelineDescriptor(desc1)
	hash2 := HashRenderPipelineDescriptor(desc2)

	if hash1 == hash2 {
		t.Error("different fragment shaders should produce different hashes")
	}
}

func TestHashRenderPipelineDescriptor_DifferentTopology(t *testing.T) {
	desc1 := mockRenderPipelineDescriptor(100, 200)
	desc2 := mockRenderPipelineDescriptor(100, 200)
	desc2.PrimitiveTopology = types.PrimitiveTopologyLineList

	hash1 := HashRenderPipelineDescriptor(desc1)
	hash2 := HashRenderPipelineDescriptor(desc2)

	if hash1 == hash2 {
		t.Error("different topology should produce different hashes")
	}
}

func TestHashRenderPipelineDescriptor_DifferentFormat(t *testing.T) {
	desc1 := mockRenderPipelineDescriptor(100, 200)
	desc2 := mockRenderPipelineDescriptor(100, 200)
	desc2.ColorFormat = types.TextureFormatRGBA8Unorm

	hash1 := HashRenderPipelineDescriptor(desc1)
	hash2 := HashRenderPipelineDescriptor(desc2)

	if hash1 == hash2 {
		t.Error("different color format should produce different hashes")
	}
}

func TestHashRenderPipelineDescriptor_DifferentBlendState(t *testing.T) {
	desc1 := mockRenderPipelineDescriptor(100, 200)
	desc1.BlendState = nil

	desc2 := mockRenderPipelineDescriptor(100, 200)
	desc2.BlendState = &BlendState{
		Color: BlendComponent{
			SrcFactor: types.BlendFactorSrcAlpha,
			DstFactor: types.BlendFactorOneMinusSrcAlpha,
			Operation: types.BlendOperationAdd,
		},
		Alpha: BlendComponent{
			SrcFactor: types.BlendFactorOne,
			DstFactor: types.BlendFactorZero,
			Operation: types.BlendOperationAdd,
		},
	}

	hash1 := HashRenderPipelineDescriptor(desc1)
	hash2 := HashRenderPipelineDescriptor(desc2)

	if hash1 == hash2 {
		t.Error("different blend state should produce different hashes")
	}
}

func TestHashRenderPipelineDescriptor_DifferentVertexLayout(t *testing.T) {
	desc1 := mockRenderPipelineDescriptor(100, 200)
	desc2 := mockRenderPipelineDescriptor(100, 200)
	desc2.VertexBufferLayouts[0].ArrayStride = 64

	hash1 := HashRenderPipelineDescriptor(desc1)
	hash2 := HashRenderPipelineDescriptor(desc2)

	if hash1 == hash2 {
		t.Error("different vertex layout should produce different hashes")
	}
}

func TestHashRenderPipelineDescriptor_DifferentEntryPoint(t *testing.T) {
	desc1 := mockRenderPipelineDescriptor(100, 200)
	desc2 := mockRenderPipelineDescriptor(100, 200)
	desc2.VertexEntryPoint = "vertex_main"

	hash1 := HashRenderPipelineDescriptor(desc1)
	hash2 := HashRenderPipelineDescriptor(desc2)

	if hash1 == hash2 {
		t.Error("different entry point should produce different hashes")
	}
}

func TestHashRenderPipelineDescriptor_NilShaders(t *testing.T) {
	desc1 := &RenderPipelineDescriptor{
		VertexShader:   nil,
		FragmentShader: nil,
	}
	desc2 := &RenderPipelineDescriptor{
		VertexShader:   nil,
		FragmentShader: nil,
	}

	hash1 := HashRenderPipelineDescriptor(desc1)
	hash2 := HashRenderPipelineDescriptor(desc2)

	if hash1 != hash2 {
		t.Error("nil shaders should produce consistent hash")
	}
}

func TestHashComputePipelineDescriptor_Deterministic(t *testing.T) {
	desc := mockComputePipelineDescriptor(500)

	hash1 := HashComputePipelineDescriptor(desc)
	hash2 := HashComputePipelineDescriptor(desc)

	if hash1 != hash2 {
		t.Errorf("hash not deterministic: %d != %d", hash1, hash2)
	}
}

func TestHashComputePipelineDescriptor_DifferentShaders(t *testing.T) {
	desc1 := mockComputePipelineDescriptor(500)
	desc2 := mockComputePipelineDescriptor(501)

	hash1 := HashComputePipelineDescriptor(desc1)
	hash2 := HashComputePipelineDescriptor(desc2)

	if hash1 == hash2 {
		t.Error("different shaders should produce different hashes")
	}
}

func TestHashComputePipelineDescriptor_DifferentEntryPoint(t *testing.T) {
	desc1 := mockComputePipelineDescriptor(500)
	desc2 := mockComputePipelineDescriptor(500)
	desc2.EntryPoint = "compute_main"

	hash1 := HashComputePipelineDescriptor(desc1)
	hash2 := HashComputePipelineDescriptor(desc2)

	if hash1 == hash2 {
		t.Error("different entry point should produce different hashes")
	}
}

// =============================================================================
// ShaderModule Tests
// =============================================================================

func TestNewShaderModule(t *testing.T) {
	code := []byte{0x03, 0x02, 0x23, 0x07} // SPIR-V magic number
	module := NewShaderModule(1, "test", code, nil)

	if module.ID() != 1 {
		t.Errorf("expected ID 1, got %d", module.ID())
	}
	if module.Label() != "test" {
		t.Errorf("expected label 'test', got %s", module.Label())
	}
	if module.CodeHash() == 0 {
		t.Error("expected non-zero code hash")
	}
	if module.IsDestroyed() {
		t.Error("expected module not to be destroyed")
	}
}

func TestShaderModule_Destroy(t *testing.T) {
	module := NewShaderModule(1, "test", []byte{1, 2, 3}, nil)

	if module.IsDestroyed() {
		t.Error("expected module not to be destroyed initially")
	}

	module.Destroy()

	if !module.IsDestroyed() {
		t.Error("expected module to be destroyed")
	}
	if module.Raw() != nil {
		t.Error("expected Raw() to return nil after destroy")
	}
}

func TestShaderModule_CodeHashDeterministic(t *testing.T) {
	code := []byte{1, 2, 3, 4, 5}
	module1 := NewShaderModule(1, "test1", code, nil)
	module2 := NewShaderModule(2, "test2", code, nil)

	if module1.CodeHash() != module2.CodeHash() {
		t.Error("same code should produce same hash")
	}
}

func TestShaderModule_CodeHashDifferent(t *testing.T) {
	module1 := NewShaderModule(1, "test1", []byte{1, 2, 3}, nil)
	module2 := NewShaderModule(2, "test2", []byte{4, 5, 6}, nil)

	if module1.CodeHash() == module2.CodeHash() {
		t.Error("different code should produce different hash")
	}
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

func TestPipelineCacheCore_ConcurrentAccess(t *testing.T) {
	cache := NewPipelineCacheCore()
	desc := mockRenderPipelineDescriptor(100, 200)

	const goroutines = 100
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				pipeline, err := cache.GetOrCreateRenderPipeline(nil, desc)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if pipeline == nil {
					t.Error("expected non-nil pipeline")
					return
				}
			}
		}()
	}

	wg.Wait()

	// Should have exactly 1 pipeline cached (all goroutines used same descriptor)
	if cache.RenderPipelineCount() != 1 {
		t.Errorf("expected 1 cached pipeline, got %d", cache.RenderPipelineCount())
	}

	// Total requests = goroutines * iterations = 10000
	// 1 miss + 9999 hits expected
	hits, misses := cache.Stats()
	totalRequests := uint64(goroutines * iterations)
	if hits+misses != totalRequests {
		t.Errorf("expected %d total requests, got %d", totalRequests, hits+misses)
	}
	if misses != 1 {
		t.Errorf("expected 1 miss, got %d", misses)
	}
}

func TestPipelineCacheCore_ConcurrentDifferentDescriptors(t *testing.T) {
	cache := NewPipelineCacheCore()

	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		i := i // Capture loop variable
		go func() {
			defer wg.Done()
			// Each goroutine uses a different descriptor
			desc := mockRenderPipelineDescriptor(uint64(i), uint64(i+1000))
			pipeline, err := cache.GetOrCreateRenderPipeline(nil, desc)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if pipeline == nil {
				t.Error("expected non-nil pipeline")
				return
			}
		}()
	}

	wg.Wait()

	// Should have exactly goroutines pipelines cached
	if cache.RenderPipelineCount() != goroutines {
		t.Errorf("expected %d cached pipelines, got %d", goroutines, cache.RenderPipelineCount())
	}

	// All requests should be misses
	hits, misses := cache.Stats()
	if misses != goroutines {
		t.Errorf("expected %d misses, got %d", goroutines, misses)
	}
	if hits != 0 {
		t.Errorf("expected 0 hits, got %d", hits)
	}
}

func TestPipelineCacheCore_ConcurrentMixedOperations(t *testing.T) {
	cache := NewPipelineCacheCore()

	const goroutines = 100
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		i := i // Capture loop variable
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Mix of render and compute pipelines
				if i%2 == 0 {
					desc := mockRenderPipelineDescriptor(uint64(j%10), 200)
					_, err := cache.GetOrCreateRenderPipeline(nil, desc)
					if err != nil {
						t.Errorf("render pipeline error: %v", err)
						return
					}
				} else {
					desc := mockComputePipelineDescriptor(uint64(j % 10))
					_, err := cache.GetOrCreateComputePipeline(nil, desc)
					if err != nil {
						t.Errorf("compute pipeline error: %v", err)
						return
					}
				}
			}
		}()
	}

	wg.Wait()

	// Should have at most 10 render + 10 compute pipelines
	if cache.RenderPipelineCount() > 10 {
		t.Errorf("expected at most 10 render pipelines, got %d", cache.RenderPipelineCount())
	}
	if cache.ComputePipelineCount() > 10 {
		t.Errorf("expected at most 10 compute pipelines, got %d", cache.ComputePipelineCount())
	}
}

// =============================================================================
// Hash Helper Tests
// =============================================================================

func TestHashBytes(t *testing.T) {
	data := []byte("test data")

	hash1 := hashBytes(data)
	hash2 := hashBytes(data)

	if hash1 != hash2 {
		t.Error("hashBytes not deterministic")
	}

	differentData := []byte("different data")
	hash3 := hashBytes(differentData)

	if hash1 == hash3 {
		t.Error("different data should produce different hash")
	}
}

func TestHashBytes_Empty(t *testing.T) {
	hash1 := hashBytes([]byte{})
	hash2 := hashBytes(nil)

	if hash1 != hash2 {
		t.Error("empty and nil should produce same hash")
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkHashRenderPipelineDescriptor(b *testing.B) {
	desc := mockRenderPipelineDescriptor(100, 200)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = HashRenderPipelineDescriptor(desc)
	}
}

func BenchmarkHashComputePipelineDescriptor(b *testing.B) {
	desc := mockComputePipelineDescriptor(500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = HashComputePipelineDescriptor(desc)
	}
}

func BenchmarkPipelineCache_Hit(b *testing.B) {
	cache := NewPipelineCacheCore()
	desc := mockRenderPipelineDescriptor(100, 200)

	// Prime the cache
	_, _ = cache.GetOrCreateRenderPipeline(nil, desc)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.GetOrCreateRenderPipeline(nil, desc)
	}
}

func BenchmarkPipelineCache_Miss(b *testing.B) {
	cache := NewPipelineCacheCore()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		desc := mockRenderPipelineDescriptor(uint64(i), 200)
		_, _ = cache.GetOrCreateRenderPipeline(nil, desc)
	}
}

func BenchmarkPipelineCache_ConcurrentHit(b *testing.B) {
	cache := NewPipelineCacheCore()
	desc := mockRenderPipelineDescriptor(100, 200)

	// Prime the cache
	_, _ = cache.GetOrCreateRenderPipeline(nil, desc)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = cache.GetOrCreateRenderPipeline(nil, desc)
		}
	})
}
