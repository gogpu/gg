package gg

// TextMode controls text rendering strategy selection.
//
// The default is TextModeAuto, which selects the best strategy automatically
// based on GPU availability, transform type, and font size. Force modes allow
// callers to express their preference for specific rendering strategies.
//
// The mode is per-Context, not global. Different contexts can use different strategies.
//
// Use cases for force modes:
//   - UI text: force Vector for maximum quality at all sizes
//   - Games/animation: force MSDF for GPU-accelerated real-time scaling
//   - Export (PNG/PDF): force Bitmap for offline rendering
//   - Debugging: isolate rendering pipeline issues
type TextMode int

const (
	// TextModeAuto selects the best strategy automatically based on
	// transform, size, and GPU availability. This is the default.
	TextModeAuto TextMode = iota

	// TextModeMSDF forces GPU MSDF atlas rendering.
	// If the GPU MSDF pipeline is unavailable, falls back to CPU rendering.
	// Best for: games, animations, real-time scaling, large text.
	TextModeMSDF

	// TextModeVector forces vector path rendering through glyph outlines.
	// Provides per-pixel coverage and perfect quality at all sizes.
	// In the future, this will use the GPU compute pipeline (Vello-style).
	// Currently uses CPU outline rendering (Strategy B).
	// Best for: UI labels, quality-critical static text.
	TextModeVector

	// TextModeBitmap forces CPU bitmap rasterization.
	// Bypasses GPU entirely, using the CPU text pipeline directly.
	// Best for: PNG/PDF export, translation-only static text.
	TextModeBitmap

	// TextModeGlyphMask forces GPU glyph mask rendering (Tier 6).
	// Glyphs are CPU-rasterized into an R8 alpha atlas at the exact pixel
	// size and rendered as textured quads on the GPU. This produces
	// pixel-perfect hinted text for horizontal layouts at small sizes.
	// Best for: UI labels, small body text (<48px), horizontal-only text.
	TextModeGlyphMask

	// TextModeAliased forces aliased (non-anti-aliased) text rendering.
	// Every pixel in the glyph mask is binary: fully opaque (255) or fully
	// transparent (0). No sub-pixel coverage, no fractional alpha.
	//
	// This matches Skia's SkFont::Edging::kAlias and is independent of the
	// geometry anti-aliasing controlled by SetAntiAlias (ADR-030). Text AA
	// and geometry AA are orthogonal — you can have aliased text on
	// anti-aliased shapes, or vice versa.
	//
	// Works on both GPU and CPU paths:
	//   - GPU: Tier 6 glyph mask pipeline (R8 atlas + textured quads)
	//   - CPU: per-glyph [DrawAliased] via [GlyphMaskRasterizer.RasterizeAliased]
	//
	// Both paths use NoAAFiller (integer scanline, binary spans) for
	// pixel-identical output regardless of GPU availability.
	//
	// Best for: retro/pixel-art aesthetics, terminal emulators, bitmap
	// font emulation, development tools where crisp edges are preferred.
	TextModeAliased
)

// String returns the text mode name.
func (m TextMode) String() string {
	switch m {
	case TextModeAuto:
		return "Auto"
	case TextModeMSDF:
		return "MSDF"
	case TextModeVector:
		return "Vector"
	case TextModeBitmap:
		return "Bitmap"
	case TextModeGlyphMask:
		return "GlyphMask"
	case TextModeAliased:
		return "Aliased"
	default:
		return "Unknown"
	}
}
