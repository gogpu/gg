// Package text provides text rendering for gg.
// It implements a modern text API inspired by Ebitengine text/v2.
//
// The text rendering pipeline follows a separation of concerns:
//
//   - FontSource: Heavyweight, shared font resource (parses TTF/OTF files)
//   - Face: Lightweight font instance at a specific size
//   - FontParser: Pluggable font parsing backend (default: own Pure Go parser)
//   - Shaper: Pluggable text shaper (default: OwnShaper with GSUB/GPOS)
//
// # Example usage
//
//	// Load font (do once, share across application)
//	source, err := text.NewFontSourceFromFile("Roboto-Regular.ttf")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer source.Close()
//
//	// Create face at specific size (lightweight)
//	face := source.Face(24)
//
//	// Use with gg.Context
//	ctx := gg.NewContext(800, 600)
//	ctx.SetFont(face)
//	ctx.DrawString("Hello, GoGPU!", 100, 100)
//
// # Font Parsing
//
// The default parser is "own" — a Pure Go font parser with zero external
// dependencies (ADR-048). Custom parsers can be registered via [RegisterParser].
//
// # Text Shaping
//
// The default shaper is [OwnShaper] with GSUB/GPOS support (ligatures,
// kerning, contextual alternates). Custom shapers can be set via [SetShaper].
package text
