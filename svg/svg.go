package svg

import (
	"fmt"
	"image"
	"image/color"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

// Render parses SVG XML data and renders it to an RGBA image at the specified size.
// The SVG viewBox is scaled to fit the target width and height.
func Render(data []byte, width, height int) (*image.RGBA, error) {
	doc, err := Parse(data)
	if err != nil {
		return nil, err
	}
	return doc.Render(width, height), nil
}

// RenderWithColor parses SVG XML data and renders it with all fill and stroke
// colors replaced by the given color. This is useful for theming icon SVGs.
// Colors set to "none" are preserved (transparent areas stay transparent).
func RenderWithColor(data []byte, width, height int, c color.Color) (*image.RGBA, error) {
	doc, err := Parse(data)
	if err != nil {
		return nil, err
	}
	return doc.RenderWithColor(width, height, c), nil
}

// Render renders the document to an RGBA image at the specified size.
// The SVG viewBox is scaled to fit the target dimensions.
func (d *Document) Render(width, height int) *image.RGBA {
	dc := gg.NewContext(width, height)
	d.RenderTo(dc, 0, 0, float64(width), float64(height))
	return dc.Image().(*image.RGBA)
}

// RenderWithColor renders the document with all fill and stroke colors
// replaced by the given color. Colors set to "none" are preserved.
func (d *Document) RenderWithColor(width, height int, c color.Color) *image.RGBA {
	dc := gg.NewContext(width, height)
	d.RenderToWithColor(dc, 0, 0, float64(width), float64(height), c)
	return dc.Image().(*image.RGBA)
}

// RenderTo renders the document into an existing [gg.Context] at the specified
// position and size. The SVG viewBox is scaled to fit (x, y, width, height).
func (d *Document) RenderTo(dc *gg.Context, x, y, width, height float64) {
	d.renderInternal(dc, x, y, width, height, nil)
}

// RenderToWithColor renders the document into an existing [gg.Context] with
// all non-"none" colors replaced by the given override color.
func (d *Document) RenderToWithColor(dc *gg.Context, x, y, width, height float64, c color.Color) {
	d.renderInternal(dc, x, y, width, height, c)
}

// renderInternal is the shared rendering implementation.
func (d *Document) renderInternal(dc *gg.Context, x, y, width, height float64, overrideColor color.Color) {
	if d == nil || dc == nil || d.ViewBox.Width <= 0 || d.ViewBox.Height <= 0 || width <= 0 || height <= 0 {
		return
	}

	dc.Push()

	// Position at (x, y).
	if x != 0 || y != 0 {
		dc.Translate(x, y)
	}

	// Scale from viewBox to target size.
	sx := width / d.ViewBox.Width
	sy := height / d.ViewBox.Height
	dc.Scale(sx, sy)

	// Apply viewBox minX/minY offset.
	if d.ViewBox.MinX != 0 || d.ViewBox.MinY != 0 {
		dc.Translate(-d.ViewBox.MinX, -d.ViewBox.MinY)
	}

	// Enable stroke hinting for small canvases (icon-sized SVGs).
	// At small sizes, thin strokes land at fractional pixel positions and get
	// anti-aliased across 2-3 pixels instead of rendering as crisp 1px lines.
	// Hinting snaps stroke endpoints to pixel centers (Java2D STROKE_NORMALIZE pattern).
	maxDim := width
	if height > maxDim {
		maxDim = height
	}
	hinting := maxDim <= strokeHintMaxCanvasSize && !strokeHintingDisabled()

	state := &renderState{
		overrideColor: overrideColor,
		parentFill:    d.RootFill,
		strokeHinting: hinting,
		scaleX:        sx,
		scaleY:        sy,
		opacity:       1,
		matrix:        dc.GetTransform(),
	}

	renderElements(dc, d.Elements, state)

	dc.Pop()
}

// RenderToScene renders the document into a [scene.Scene] as vector geometry.
// Unlike [RenderTo] which rasterizes to pixels, this encodes SVG elements as
// scene Fill/Stroke commands using native shapes (CircleShape, RectShape, etc.),
// preserving vector data for GPU rendering or scene composition.
//
// The SVG viewBox is scaled to fit (x, y, width, height) via scene transforms.
func (d *Document) RenderToScene(s *scene.Scene, x, y, width, height float32) {
	d.renderToSceneInternal(s, x, y, width, height, nil)
}

// RenderToSceneWithColor renders the document into a [scene.Scene] with all
// non-"none" colors replaced by the given override color. This is the scene
// equivalent of [RenderToWithColor], useful for theming icon SVGs in a vector
// pipeline without bitmap rasterization.
func (d *Document) RenderToSceneWithColor(s *scene.Scene, x, y, width, height float32, c gg.RGBA) {
	d.renderToSceneInternal(s, x, y, width, height, &c)
}

// String returns a short description of the document for debugging.
func (d *Document) String() string {
	return fmt.Sprintf("SVG(viewBox=%.0f,%.0f,%.0f,%.0f elements=%d)",
		d.ViewBox.MinX, d.ViewBox.MinY, d.ViewBox.Width, d.ViewBox.Height,
		len(d.Elements))
}
