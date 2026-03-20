//go:build !nogpu

package gpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// stencilPassthroughDepthStencil returns a DepthStencilState that passes through
// all stencil operations (no write, no test). Used by pipelines that render
// alongside the stencil-then-cover renderer but don't interact with stencil.
func stencilPassthroughDepthStencil() *wgpu.DepthStencilState {
	return &wgpu.DepthStencilState{
		Format:            gputypes.TextureFormatDepth24PlusStencil8,
		DepthWriteEnabled: false,
		DepthCompare:      gputypes.CompareFunctionAlways,
		StencilFront: wgpu.StencilFaceState{
			Compare:     gputypes.CompareFunctionAlways,
			FailOp:      wgpu.StencilOperationKeep,
			DepthFailOp: wgpu.StencilOperationKeep,
			PassOp:      wgpu.StencilOperationKeep,
		},
		StencilBack: wgpu.StencilFaceState{
			Compare:     gputypes.CompareFunctionAlways,
			FailOp:      wgpu.StencilOperationKeep,
			DepthFailOp: wgpu.StencilOperationKeep,
			PassOp:      wgpu.StencilOperationKeep,
		},
		StencilReadMask:  0x00,
		StencilWriteMask: 0x00,
	}
}

// defaultMultisample returns the standard MultisampleState (4x MSAA).
func defaultMultisample() gputypes.MultisampleState {
	return gputypes.MultisampleState{
		Count: sampleCount,
		Mask:  0xFFFFFFFF,
	}
}

// triangleListPrimitive returns the standard PrimitiveState for triangle list rendering.
func triangleListPrimitive() gputypes.PrimitiveState {
	return gputypes.PrimitiveState{
		Topology: gputypes.PrimitiveTopologyTriangleList,
		CullMode: gputypes.CullModeNone,
	}
}
