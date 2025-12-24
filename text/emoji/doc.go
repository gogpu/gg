// Package emoji provides emoji and color font support for text rendering.
//
// This package handles the detection, segmentation, and rendering of emoji
// characters in text. It supports:
//
//   - Single emoji characters (U+1F600 - U+1F64F emoticons)
//   - ZWJ (Zero-Width Joiner) sequences for complex emoji
//   - Skin tone modifiers (U+1F3FB - U+1F3FF)
//   - Regional indicator sequences for flag emoji
//   - Keycap sequences (digit + U+FE0F + U+20E3)
//   - Tag sequences for subdivision flags
//
// # Color Font Support
//
// The package supports rendering color emoji from fonts using:
//
//   - COLRv0/COLRv1 tables (layered color glyphs)
//   - Bitmap tables: sbix (Apple), CBDT/CBLC (Google)
//
// # Usage
//
// To segment text into emoji and non-emoji runs:
//
//	runs := emoji.Segment("Hello 😀 World")
//	for _, run := range runs {
//	    if run.IsEmoji {
//	        // Render with color emoji renderer
//	    } else {
//	        // Render with text renderer
//	    }
//	}
//
// To detect if a rune is an emoji:
//
//	if emoji.IsEmoji(r) {
//	    // Handle emoji character
//	}
//
// # Unicode Emoji Specification
//
// This implementation follows Unicode Technical Report #51:
// https://www.unicode.org/reports/tr51/
//
// Key concepts:
//   - Emoji_Presentation: Characters that default to emoji display
//   - Text_Presentation: Characters that default to text display
//   - Variation Selectors: U+FE0E (text) and U+FE0F (emoji)
//   - ZWJ Sequences: Multiple emoji joined by U+200D
package emoji
