//go:build !nogpu

package gpu

import (
	"fmt"

	"github.com/gogpu/naga"
	"github.com/gogpu/wgpu"
)

// CompileShaderToSPIRV compiles WGSL source to SPIR-V uint32 slice.
// This is the common shader compilation logic used by all GPU rasterizers.
func CompileShaderToSPIRV(wgslSource string) ([]uint32, error) {
	// Compile WGSL to SPIR-V bytes
	spirvBytes, err := naga.Compile(wgslSource)
	if err != nil {
		return nil, fmt.Errorf("failed to compile shader: %w", err)
	}

	// Convert bytes to uint32 slice for SPIR-V
	// SPIR-V is little-endian 32-bit words
	spirvCode := make([]uint32, len(spirvBytes)/4)
	for i := range spirvCode {
		spirvCode[i] = uint32(spirvBytes[i*4]) |
			uint32(spirvBytes[i*4+1])<<8 |
			uint32(spirvBytes[i*4+2])<<16 |
			uint32(spirvBytes[i*4+3])<<24
	}

	return spirvCode, nil
}

// CreateShaderModule creates a HAL shader module from SPIR-V code.
func CreateShaderModule(device *wgpu.Device, label string, spirvCode []uint32) (*wgpu.ShaderModule, error) {
	return device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: label,
		SPIRV: spirvCode,
	})
}

// DestroyGPUResources safely destroys common GPU resources.
// This is a helper for the cleanup pattern used by all GPU rasterizers.
type GPUResources struct {
	Device         *wgpu.Device
	ShaderModule   *wgpu.ShaderModule
	PipelineLayout *wgpu.PipelineLayout
	BindLayouts    []*wgpu.BindGroupLayout
	Pipelines      []*wgpu.ComputePipeline
}

// Destroy cleans up all GPU resources in the correct order.
func (r *GPUResources) Destroy() {
	if r.Device == nil {
		return
	}

	// Destroy pipelines first
	for _, p := range r.Pipelines {
		if p != nil {
			p.Release()
		}
	}

	// Destroy pipeline layout
	if r.PipelineLayout != nil {
		r.PipelineLayout.Release()
	}

	// Destroy bind group layouts
	for _, l := range r.BindLayouts {
		if l != nil {
			l.Release()
		}
	}

	// Destroy shader module
	if r.ShaderModule != nil {
		r.ShaderModule.Release()
	}
}
