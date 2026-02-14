//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/wgpu/hal"
)

func TestStencilPipelineCreation(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	// Pipelines only need the device, not textures.
	err := sr.createPipelines()
	if err != nil {
		t.Fatalf("createPipelines failed: %v", err)
	}

	// Verify all shader modules are created.
	if sr.stencilFillShader == nil {
		t.Error("expected non-nil stencilFillShader")
	}
	if sr.coverShader == nil {
		t.Error("expected non-nil coverShader")
	}

	// Verify layouts are created.
	if sr.uniformLayout == nil {
		t.Error("expected non-nil uniformLayout")
	}
	if sr.stencilPipeLayout == nil {
		t.Error("expected non-nil stencilPipeLayout")
	}
	if sr.coverPipeLayout == nil {
		t.Error("expected non-nil coverPipeLayout")
	}

	// Verify render pipelines are created.
	if sr.nonZeroStencilPipeline == nil {
		t.Error("expected non-nil nonZeroStencilPipeline")
	}
	if sr.evenOddStencilPipeline == nil {
		t.Error("expected non-nil evenOddStencilPipeline")
	}
	if sr.nonZeroCoverPipeline == nil {
		t.Error("expected non-nil nonZeroCoverPipeline")
	}
}

func TestStencilPipelineDestroy(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)

	err := sr.createPipelines()
	if err != nil {
		t.Fatalf("createPipelines failed: %v", err)
	}

	// Verify pipelines exist before destroy.
	if sr.nonZeroStencilPipeline == nil {
		t.Fatal("expected non-nil nonZeroStencilPipeline before destroy")
	}

	sr.destroyPipelines()

	// Verify all pipeline resources are nil after destroy.
	if sr.stencilFillShader != nil {
		t.Error("expected nil stencilFillShader after destroyPipelines")
	}
	if sr.coverShader != nil {
		t.Error("expected nil coverShader after destroyPipelines")
	}
	if sr.uniformLayout != nil {
		t.Error("expected nil uniformLayout after destroyPipelines")
	}
	if sr.stencilPipeLayout != nil {
		t.Error("expected nil stencilPipeLayout after destroyPipelines")
	}
	if sr.coverPipeLayout != nil {
		t.Error("expected nil coverPipeLayout after destroyPipelines")
	}
	if sr.nonZeroStencilPipeline != nil {
		t.Error("expected nil nonZeroStencilPipeline after destroyPipelines")
	}
	if sr.evenOddStencilPipeline != nil {
		t.Error("expected nil evenOddStencilPipeline after destroyPipelines")
	}
	if sr.nonZeroCoverPipeline != nil {
		t.Error("expected nil nonZeroCoverPipeline after destroyPipelines")
	}

	// Double-destroy should be safe.
	sr.destroyPipelines()
}

func TestStencilPipelineDestroyBeforeCreate(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)

	// Destroying pipelines that were never created should not panic.
	sr.destroyPipelines()
}

func TestStencilPipelineFullDestroy(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)

	// Create textures and pipelines.
	err := sr.EnsureTextures(800, 600)
	if err != nil {
		t.Fatalf("EnsureTextures failed: %v", err)
	}

	err = sr.createPipelines()
	if err != nil {
		t.Fatalf("createPipelines failed: %v", err)
	}

	// Destroy() should clean up both pipelines and textures.
	sr.Destroy()

	if sr.nonZeroStencilPipeline != nil {
		t.Error("expected nil nonZeroStencilPipeline after Destroy")
	}
	if sr.evenOddStencilPipeline != nil {
		t.Error("expected nil evenOddStencilPipeline after Destroy")
	}
	if sr.nonZeroCoverPipeline != nil {
		t.Error("expected nil nonZeroCoverPipeline after Destroy")
	}
	if sr.textures.msaaTex != nil {
		t.Error("expected nil msaaTex after Destroy")
	}
	if sr.textures.stencilTex != nil {
		t.Error("expected nil stencilTex after Destroy")
	}
	if sr.textures.resolveTex != nil {
		t.Error("expected nil resolveTex after Destroy")
	}
}

func TestShaderCompilation(t *testing.T) {
	device, _, cleanup := createNoopDevice(t)
	defer cleanup()

	// Test stencil fill shader compilation.
	if stencilFillShaderSource == "" {
		t.Fatal("stencil fill shader source is empty")
	}

	stencilModule, err := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "test_stencil_fill",
		Source: hal.ShaderSource{WGSL: stencilFillShaderSource},
	})
	if err != nil {
		t.Fatalf("stencil fill shader compilation failed: %v", err)
	}
	if stencilModule == nil {
		t.Error("expected non-nil stencil fill shader module")
	}

	// Test cover shader compilation.
	if coverShaderSource == "" {
		t.Fatal("cover shader source is empty")
	}

	coverModule, err := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "test_cover",
		Source: hal.ShaderSource{WGSL: coverShaderSource},
	})
	if err != nil {
		t.Fatalf("cover shader compilation failed: %v", err)
	}
	if coverModule == nil {
		t.Error("expected non-nil cover shader module")
	}
}

func TestStencilPipelineRecreate(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	sr := NewStencilRenderer(device, queue)
	defer sr.Destroy()

	// Create, destroy, recreate -- simulates device reset scenario.
	err := sr.createPipelines()
	if err != nil {
		t.Fatalf("first createPipelines failed: %v", err)
	}

	sr.destroyPipelines()

	err = sr.createPipelines()
	if err != nil {
		t.Fatalf("second createPipelines failed: %v", err)
	}

	if sr.nonZeroStencilPipeline == nil {
		t.Error("expected non-nil nonZeroStencilPipeline after recreate")
	}
	if sr.evenOddStencilPipeline == nil {
		t.Error("expected non-nil evenOddStencilPipeline after recreate")
	}
	if sr.nonZeroCoverPipeline == nil {
		t.Error("expected non-nil nonZeroCoverPipeline after recreate")
	}
}
