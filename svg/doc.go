// Package svg provides an SVG renderer built on top of the gg 2D graphics library.
//
// It parses a subset of SVG XML sufficient for rendering icon-style SVGs
// (as used by JetBrains IntelliJ, Material Design, etc.) and renders them
// to RGBA images using [github.com/gogpu/gg.Context].
//
// # Supported SVG Elements
//
//   - <svg> with viewBox, width, height
//   - <path> with d attribute (full SVG path command set)
//   - <circle> cx, cy, r
//   - <rect> x, y, width, height, rx, ry
//   - <ellipse> cx, cy, rx, ry
//   - <line> x1, y1, x2, y2
//   - <polygon> points
//   - <polyline> points
//   - <g> grouping with inherited attributes and transform
//
// # Supported Attributes
//
//   - fill, fill-rule, fill-opacity
//   - stroke, stroke-width, stroke-linecap, stroke-linejoin, stroke-opacity
//   - transform (translate, rotate, scale, matrix)
//   - opacity
//
// # Usage
//
//	img, err := svg.Render(svgBytes, 64, 64)
//
//	// With color override (theming):
//	img, err := svg.RenderWithColor(svgBytes, 64, 64, color.White)
//
//	// Parse once, render many times:
//	doc, err := svg.Parse(svgBytes)
//	img1 := doc.Render(16, 16)
//	img2 := doc.Render(32, 32)
//	img3 := doc.RenderWithColor(24, 24, themeColor)
package svg
