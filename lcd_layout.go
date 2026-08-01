package gg

import "github.com/gogpu/gg/text"

// LCDLayout describes a requested physical subpixel arrangement on the display.
// Most LCD monitors use horizontal RGB stripe ordering, where each pixel
// consists of three vertical subpixel columns (red, green, blue) from
// left to right. Backends without exact per-channel compositing may honor any
// requested layout by rendering a portable grayscale glyph mask instead.
//
// Re-exported from text.LCDLayout for convenience so users do not need
// to import the text subpackage for this common setting.
type LCDLayout = text.LCDLayout

const (
	// LCDLayoutNone requests grayscale rendering. This is the default.
	LCDLayoutNone = text.LCDLayoutNone

	// LCDLayoutRGB requests horizontal RGB ordering (most monitors).
	// Physical subpixels left-to-right: Red, Green, Blue.
	LCDLayoutRGB = text.LCDLayoutRGB

	// LCDLayoutBGR requests horizontal BGR ordering (rare, some monitors).
	// Physical subpixels left-to-right: Blue, Green, Red.
	LCDLayoutBGR = text.LCDLayoutBGR
)
