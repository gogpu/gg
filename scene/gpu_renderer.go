package scene

import (
	"github.com/gogpu/gg"
)

// GPUSceneRenderer provides GPU-accelerated scene rendering by decoding
// scene commands into gg.Context draw calls. The gg.Context handles
// GPU/CPU dispatch automatically via its accelerator system.
//
// This follows the Vello pattern: scene encoding is stateless, and
// rendering decodes the encoding atomically into draw calls. The GPU
// accelerator receives shapes via FillShape/FillPath and flushes them
// in a single render pass.
//
// For scenes with simple fill/stroke operations (the common case in UI
// rendering), this provides a direct GPU path that avoids the tile-based
// CPU decomposition in scene.Renderer.
//
// Usage:
//
//	gpuR := scene.NewGPUSceneRenderer(dc)
//	err := gpuR.RenderScene(myScene)
type GPUSceneRenderer struct {
	dc *gg.Context
}

// NewGPUSceneRenderer creates a GPU scene renderer that renders through
// the given gg.Context. The context's GPU accelerator (if registered)
// will handle shape rendering; CPU fallback is automatic.
func NewGPUSceneRenderer(dc *gg.Context) *GPUSceneRenderer {
	return &GPUSceneRenderer{dc: dc}
}

// RenderScene decodes scene commands and renders them through the gg.Context.
// The decoder walks the binary encoding tag-by-tag, building paths and
// dispatching fill/stroke calls that route through the GPU accelerator.
//
// Returns nil if the scene is empty.
func (r *GPUSceneRenderer) RenderScene(scene *Scene) error { //nolint:gocyclo,cyclop,funlen // tag dispatch across all scene command types
	if scene == nil {
		return nil
	}

	enc := scene.Encoding()
	if enc == nil || len(enc.Tags()) == 0 {
		return nil
	}

	dec := NewDecoder(enc)
	if dec == nil {
		return nil
	}

	dc := r.dc
	path := gg.NewPath()
	transformDepth := 0

	for dec.Next() {
		switch dec.Tag() {
		case TagTransform:
			a := dec.Transform()
			if transformDepth > 0 {
				dc.Pop()
			}
			dc.Push()
			transformDepth++
			dc.Transform(gg.Matrix{
				A: float64(a.A), B: float64(a.B), C: float64(a.C),
				D: float64(a.D), E: float64(a.E), F: float64(a.F),
			})

		case TagBeginPath:
			path.Clear()

		case TagMoveTo:
			x, y := dec.MoveTo()
			path.MoveTo(float64(x), float64(y))

		case TagLineTo:
			x, y := dec.LineTo()
			path.LineTo(float64(x), float64(y))

		case TagQuadTo:
			cx, cy, x, y := dec.QuadTo()
			path.QuadraticTo(float64(cx), float64(cy), float64(x), float64(y))

		case TagCubicTo:
			c1x, c1y, c2x, c2y, x, y := dec.CubicTo()
			path.CubicTo(float64(c1x), float64(c1y), float64(c2x), float64(c2y), float64(x), float64(y))

		case TagClosePath:
			path.Close()

		case TagEndPath:
			// Path building complete; fill/stroke tag follows.

		case TagFill:
			brush, style := dec.Fill()
			applySceneBrush(dc, brush)
			if style == FillEvenOdd {
				dc.SetFillRule(gg.FillRuleEvenOdd)
			} else {
				dc.SetFillRule(gg.FillRuleNonZero)
			}
			_ = dc.FillPath(path)
			path.Clear()

		case TagFillRoundRect:
			brush, style, rect, rx, ry := dec.FillRoundRect()
			applySceneBrush(dc, brush)
			if style == FillEvenOdd {
				dc.SetFillRule(gg.FillRuleEvenOdd)
			} else {
				dc.SetFillRule(gg.FillRuleNonZero)
			}
			radius := float64(rx)
			if ry > rx {
				radius = float64(ry)
			}
			dc.DrawRoundedRectangle(
				float64(rect.MinX), float64(rect.MinY),
				float64(rect.MaxX-rect.MinX), float64(rect.MaxY-rect.MinY),
				radius,
			)
			_ = dc.Fill()

		case TagStroke:
			brush, style := dec.Stroke()
			applySceneBrush(dc, brush)
			if style != nil && style.Width > 0 {
				dc.SetLineWidth(float64(style.Width))
			}
			_ = dc.StrokePath(path)
			path.Clear()

		case TagPushLayer:
			dc.Push()

		case TagPopLayer:
			dc.Pop()

		case TagBeginClip:
			// Push state before clip so EndClip can restore the previous clip level.
			// Without Push/Pop, ResetClip destroys ALL clips (not just innermost),
			// breaking nested clip regions (card → ListView → ScrollView).
			dc.Push()
			dc.DrawPath(path)
			dc.Clip()
			dc.ClearPath()
			path.Clear()

		case TagEndClip:
			dc.Pop()

		case TagImage:
			_, _ = dec.Image()

		default:
			// Unknown tags are skipped by the decoder advancing tagIdx.
		}
	}

	if transformDepth > 0 {
		dc.Pop()
	}

	return nil
}

// applySceneBrush sets the gg.Context color from a scene.Brush.
func applySceneBrush(dc *gg.Context, brush Brush) {
	if brush.Kind == BrushSolid {
		dc.SetRGBA(brush.Color.R, brush.Color.G, brush.Color.B, brush.Color.A)
	}
}

// CanUseGPU returns true if a GPU accelerator is registered and can
// render directly. This is used by scene.Renderer to auto-select
// the GPU path when available.
func CanUseGPU() bool {
	return gg.AcceleratorCanRenderDirect()
}
