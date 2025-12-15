// Package text provides text rendering for gg.
// It implements a modern text API inspired by Ebitengine text/v2.
//
// The text rendering pipeline follows a separation of concerns:
//
//   - FontSource: Heavyweight, shared font resource (parses TTF/OTF files)
//   - Face: Lightweight font instance at a specific size
//   - FontParser: Pluggable font parsing backend (default: golang.org/x/image)
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
// # Pluggable Parser Backend
//
// The font parsing is abstracted through the FontParser interface.
// By default, golang.org/x/image/font/opentype is used.
// Custom parsers can be registered for alternative implementations:
//
//	// Register a custom parser
//	text.RegisterParser("myparser", myCustomParser)
//
//	// Use the custom parser
//	source, err := text.NewFontSource(data, text.WithParser("myparser"))
//
// This design allows:
//   - Easy migration to different font libraries
//   - Pure Go implementations without external dependencies
//   - Custom font formats or optimized parsers
package text
