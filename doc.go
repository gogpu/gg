// Package gg provides an enterprise-grade 2D graphics library for Go.
//
// # Overview
//
// gg is a Pure Go 2D graphics library with GPU acceleration, designed for
// IDEs, browsers, and graphics-intensive applications. Built with patterns
// from Skia (Chrome/Android), Cairo (GTK), and Vello (linebender), it
// delivers production-grade rendering with zero CGO dependencies.
//
// Part of the [GoGPU] ecosystem.
//
// # Quick Start
//
//	import (
//	    "github.com/gogpu/gg"
//	    "github.com/gogpu/gg/text"
//	)
//
//	dc := gg.NewContext(512, 512)
//	defer dc.Close()
//
//	dc.ClearWithColor(gg.White)
//
//	// Draw shapes
//	dc.SetHexColor("#3498db")
//	dc.DrawCircle(256, 256, 100)
//	dc.Fill()
//
//	// Text
//	source, _ := text.NewFontSourceFromFile("font.ttf")
//	defer source.Close()
//	dc.SetFont(source.Face(24))
//	dc.SetRGB(0, 0, 0)
//	dc.DrawString("Hello", 200, 260)
//
//	dc.SavePNG("output.png")
//
// # API Style
//
// The API follows the stateful context pattern (HTML5 Canvas, Cairo):
//   - Set state: SetRGB, SetFont, SetLineWidth, SetBlendMode, SetFillBrush
//   - Build path: MoveTo, LineTo, CubicTo, DrawCircle, DrawRectangle
//   - Render: Fill, Stroke, FillPreserve, StrokePreserve
//   - State stack: Push, Pop
//
// Compatible with [fogleman/gg] for easy migration.
//
// # Rendering
//
// Seven-tier GPU acceleration with automatic CPU fallback:
//   - Tier 1: SDF shapes (circles, rounded rects, ellipses)
//   - Tier 2a: Convex polygon fan tessellation
//   - Tier 2b: Stencil-then-cover (arbitrary paths)
//   - Tier 4: MSDF text (multi-channel signed distance field)
//   - Tier 5: Vello 9-stage compute pipeline (full scenes)
//   - Tier 6: Glyph mask cache (hinted bitmap text)
//   - CPU: Skia AAA pixel-perfect rasterizer (analytic anti-aliasing)
//
// # Compositing
//
// 29 W3C blend modes for direct Fill/Stroke operations:
//   - 14 Porter-Duff compositing operators
//   - 11 advanced separable modes (Multiply, Screen, Overlay, ...)
//   - 4 non-separable HSL modes (Hue, Saturation, Color, Luminosity)
//   - Layer isolation via PushLayer/PopLayer with opacity
//
// # Text
//
// Pure Go font stack with variable font support:
//   - Own cmap/hmtx/GSUB/GPOS/gvar/HVAR parsers
//   - TrueType bytecode interpreter (skrifa golden parity)
//   - MSDF + glyph mask dual-strategy GPU rendering
//   - ClearType LCD auto-detection, CJK script-aware hinting
//   - OpenType features, bidi segmentation (RTL visual reordering not yet
//     implemented), color emoji
//
// # Coordinate System
//
// Standard screen coordinates: origin (0,0) at top-left, X right, Y down.
// Angles in radians, 0 is right, counter-clockwise positive.
//
// [GoGPU]: https://github.com/gogpu
// [fogleman/gg]: https://github.com/fogleman/gg
package gg
