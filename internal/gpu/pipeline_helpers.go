//go:build !nogpu

package gpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// stencilPassthroughDepthStencil returns a DepthStencilState that passes through
// all stencil operations (no write, no test). Used by pipelines that render
// alongside the stencil-then-cover renderer but don't interact with stencil.
//
// When depthClip is false (default), depth is also pass-through (Compare=Always).
// When depthClip is true (GPU-CLIP-003a), depth test is enabled with
// DepthCompare=LessEqual — fragments pass only where the clip path previously
// wrote a depth value >= the fragment's Z. This implements arbitrary path
// clipping via the depth buffer without touching stencil.
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

// depthClipDepthStencil returns a DepthStencilState with depth testing enabled
// for depth-based clipping (GPU-CLIP-003a). Fragments pass the depth test
// only where the depth clip pipeline previously wrote depth = 0.0.
//
// Used by content pipelines (SDF, convex, image, text, glyph mask) when a
// ScissorGroup has an active ClipPath. Content shaders output Z=0.0 (unchanged
// from their normal behavior).
//
// Depth model:
//   - DepthClearValue = 1.0 (unchanged from existing)
//   - Clip path writes Z = 0.0 → depth buffer = 0.0 where clip geometry exists
//   - Content uses DepthCompare=GreaterEqual, fragment Z = 0.0
//   - Where clip drawn:     buffer=0.0, fragment=0.0 → 0.0 >= 0.0 → PASS
//   - Where clip NOT drawn: buffer=1.0, fragment=0.0 → 0.0 >= 1.0 → FAIL
func depthClipDepthStencil() *wgpu.DepthStencilState {
	return &wgpu.DepthStencilState{
		Format:            gputypes.TextureFormatDepth24PlusStencil8,
		DepthWriteEnabled: false,
		DepthCompare:      gputypes.CompareFunctionGreaterEqual,
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
