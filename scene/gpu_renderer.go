package scene

import (
	"image"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
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
// Transform handling: scene transforms are applied via dc.SetTransform()
// (direct matrix replacement) instead of dc.Push()/Pop(). This avoids
// corrupting the clip stack. Push/Pop is reserved for clip and layer
// boundaries only, ensuring that TagTransform inside a clip region does
// not accidentally pop the clip's saved state.
//
// Returns nil if the scene is empty.
func (r *GPUSceneRenderer) RenderScene(scene *Scene) error { //nolint:gocyclo,cyclop,funlen,gocognit // tag dispatch across all scene command types
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

	// Save the initial matrix so we can compose scene transforms correctly
	// and restore the original state after rendering.
	// Scene transforms use dc.SetTransform() (direct replacement) instead of
	// dc.Push()/Pop() to avoid interfering with clip Push/Pop nesting.
	baseMatrix := dc.GetTransform()

	for dec.Next() {
		switch dec.Tag() {
		case TagTransform:
			a := dec.Transform()
			// Apply scene transform composed with the context's base matrix.
			// Scene transforms are absolute (full accumulated affine from the
			// encoder, e.g. currentTransform.Multiply(perDrawTransform)), so we
			// compose with baseMatrix to preserve any parent context transform
			// (e.g. dc.Translate() called before RenderScene).
			//
			// Using Push/Pop here would corrupt clip state when TagTransform
			// appears inside a BeginClip/EndClip region: the transform's Pop
			// would undo the clip's Push, removing the clip before content is
			// drawn. SetTransform avoids this by not touching the state stack.
			sceneMatrix := gg.Matrix{
				A: float64(a.A), B: float64(a.B), C: float64(a.C),
				D: float64(a.D), E: float64(a.E), F: float64(a.F),
			}
			dc.SetTransform(baseMatrix.Multiply(sceneMatrix))

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
			if style != nil {
				if style.Width > 0 {
					dc.SetLineWidth(float64(style.Width))
				}
				dc.SetLineCap(gg.LineCap(style.Cap))
				dc.SetLineJoin(gg.LineJoin(style.Join))
				if style.MiterLimit > 0 {
					dc.SetMiterLimit(float64(style.MiterLimit))
				}
			}
			_ = dc.StrokePath(path)
			path.Clear()

		case TagPushLayer:
			blend, alpha := dec.PushLayer()
			dc.PushLayer(gg.BlendMode(blend), float64(alpha))

		case TagPopLayer:
			dc.PopLayer()

		case TagBeginClip:
			// Push state before clip so EndClip can restore the previous clip
			// level AND the transform that was active at clip time. Push/Pop
			// here is correct: clips are strictly nested and each BeginClip
			// has exactly one EndClip. No intermediate Pop can occur because
			// TagTransform uses SetTransform (not Push/Pop).
			dc.Push()
			// Detect rectangular clips → hardware scissor (ClipRect).
			// Non-rect clips → general path clip (depth buffer on GPU).
			// ClipRect uses PushRect → IsRectOnly()=true → hardware scissor.
			// Clip() uses PushPath → IsRectOnly()=false → depth clip path
			// which may not be available in all rendering contexts.
			shape := gg.DetectShape(path)
			if shape.Kind == gg.ShapeRect {
				x := shape.CenterX - shape.Width/2
				y := shape.CenterY - shape.Height/2
				dc.ClipRect(x, y, shape.Width, shape.Height)
			} else {
				dc.DrawPath(path)
				dc.Clip()
				dc.ClearPath()
			}
			path.Clear()

		case TagEndClip:
			dc.Pop()

		case TagText:
			run, glyphs, str, brush := dec.Text()
			r.resolveText(scene, run, glyphs, str, brush)

		case TagImage:
			imageIndex, imgTransform := dec.Image()
			r.resolveImage(scene, imageIndex, imgTransform)

		default:
			// Unknown tags are skipped by the decoder advancing tagIdx.
		}
	}

	// Restore the original matrix. Unlike the previous Push/Pop approach,
	// SetTransform is a direct replacement so we restore explicitly.
	dc.SetTransform(baseMatrix)

	return nil
}

// resolveText resolves a TagText glyph run. Prefers DrawShapedGlyphs (no re-shaping)
// with DrawString as fallback. Produces hinted, atlas-batched, DPI-aware text.
func (r *GPUSceneRenderer) resolveText(scene *Scene, run GlyphRunData, glyphs []GlyphEntry, str string, brush Brush) {
	if run.GlyphCount == 0 {
		return
	}

	source := scene.fontRegistry[run.FontSourceID]
	if source == nil {
		return
	}

	dc := r.dc
	applySceneBrush(dc, brush)

	prevFace := dc.Font()
	face := source.Face(float64(run.FontSize))
	dc.SetFont(face)

	if len(glyphs) > 0 {
		isCJK := run.Flags&TextFlagCJK != 0
		shaped := make([]text.ShapedGlyph, len(glyphs))
		for i, g := range glyphs {
			shaped[i] = text.ShapedGlyph{GID: g.GlyphID, X: float64(g.X), Y: float64(g.Y), IsCJK: isCJK}
		}
		dc.DrawShapedGlyphs(shaped, face, float64(run.OriginX), float64(run.OriginY))
	} else if str != "" {
		dc.DrawString(str, float64(run.OriginX), float64(run.OriginY))
	}

	dc.SetFont(prevFace)
}

// resolveImage renders a scene image through dc.DrawImage.
func (r *GPUSceneRenderer) resolveImage(scene *Scene, imageIndex uint32, transform Affine) {
	images := scene.Images()
	if int(imageIndex) >= len(images) {
		return
	}
	img := images[imageIndex]
	if img == nil || img.Width <= 0 || img.Height <= 0 || len(img.Data) < img.Width*img.Height*4 {
		return
	}

	rgba := &image.RGBA{
		Pix:    img.Data,
		Stride: img.Width * 4,
		Rect:   image.Rect(0, 0, img.Width, img.Height),
	}
	buf := gg.ImageBufFromImage(rgba)

	dc := r.dc
	dc.DrawImage(buf, float64(transform.C), float64(transform.F))
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
