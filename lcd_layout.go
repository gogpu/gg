package gg

import "github.com/gogpu/gg/text"

// LCDLayout describes the physical subpixel arrangement on the display.
// Most LCD monitors use horizontal RGB stripe ordering, where each pixel
// consists of three vertical subpixel columns (red, green, blue) from
// left to right. ClearType-style rendering exploits this to triple the
// effective horizontal resolution for text.
//
// Re-exported from text.LCDLayout for convenience so users do not need
// to import the text subpackage for this common setting.
type LCDLayout = text.LCDLayout

const (
	// LCDLayoutNone disables subpixel rendering (grayscale fallback).
	// This is the default.
	LCDLayoutNone = text.LCDLayoutNone

	// LCDLayoutRGB is horizontal RGB ordering (most common: Windows, most monitors).
	// Physical subpixels left-to-right: Red, Green, Blue.
	LCDLayoutRGB = text.LCDLayoutRGB

	// LCDLayoutBGR is horizontal BGR ordering (rare, some monitors).
	// Physical subpixels left-to-right: Blue, Green, Red.
	LCDLayoutBGR = text.LCDLayoutBGR
)
