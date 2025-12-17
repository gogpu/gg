// Package scene provides blend mode integration with internal/blend package.
package scene

import "github.com/gogpu/gg/internal/blend"

// ToInternalBlendMode converts scene.BlendMode to internal blend.BlendMode.
// This provides the mapping between the scene graph blend modes and
// the low-level pixel blending implementation.
//
// The internal blend.BlendMode uses a different enumeration order,
// so this function provides the translation.
func (mode BlendMode) ToInternalBlendMode() blend.BlendMode {
	switch mode {
	// Porter-Duff modes (internal order matches porter_duff.go)
	case BlendClear:
		return blend.BlendClear
	case BlendCopy:
		return blend.BlendSource
	case BlendDestination:
		return blend.BlendDestination
	case BlendSourceOver, BlendNormal: // BlendNormal is SourceOver
		return blend.BlendSourceOver
	case BlendDestinationOver:
		return blend.BlendDestinationOver
	case BlendSourceIn:
		return blend.BlendSourceIn
	case BlendDestinationIn:
		return blend.BlendDestinationIn
	case BlendSourceOut:
		return blend.BlendSourceOut
	case BlendDestinationOut:
		return blend.BlendDestinationOut
	case BlendSourceAtop:
		return blend.BlendSourceAtop
	case BlendDestinationAtop:
		return blend.BlendDestinationAtop
	case BlendXor:
		return blend.BlendXor
	case BlendPlus:
		return blend.BlendPlus

	// Advanced separable blend modes (from advanced.go)
	case BlendMultiply:
		return blend.BlendMultiply
	case BlendScreen:
		return blend.BlendScreen
	case BlendOverlay:
		return blend.BlendOverlay
	case BlendDarken:
		return blend.BlendDarken
	case BlendLighten:
		return blend.BlendLighten
	case BlendColorDodge:
		return blend.BlendColorDodge
	case BlendColorBurn:
		return blend.BlendColorBurn
	case BlendHardLight:
		return blend.BlendHardLight
	case BlendSoftLight:
		return blend.BlendSoftLight
	case BlendDifference:
		return blend.BlendDifference
	case BlendExclusion:
		return blend.BlendExclusion

	// HSL non-separable blend modes (from advanced.go / hsl.go)
	case BlendHue:
		return blend.BlendHue
	case BlendSaturation:
		return blend.BlendSaturation
	case BlendColor:
		return blend.BlendColor
	case BlendLuminosity:
		return blend.BlendLuminosity

	default:
		// Default to source-over for unknown modes
		return blend.BlendSourceOver
	}
}

// GetBlendFunc returns the internal blend function for this mode.
// This is a convenience method that combines ToInternalBlendMode
// with blend.GetBlendFunc.
//
// Usage:
//
//	blendFn := scene.BlendMultiply.GetBlendFunc()
//	r, g, b, a := blendFn(sr, sg, sb, sa, dr, dg, db, da)
func (mode BlendMode) GetBlendFunc() blend.BlendFunc {
	return blend.GetBlendFunc(mode.ToInternalBlendMode())
}

// BlendModeFromInternal converts an internal blend.BlendMode to scene.BlendMode.
// This is the reverse mapping for cases where you need to convert from
// internal representation back to the scene graph representation.
func BlendModeFromInternal(internal blend.BlendMode) BlendMode {
	switch internal {
	// Porter-Duff modes
	case blend.BlendClear:
		return BlendClear
	case blend.BlendSource:
		return BlendCopy
	case blend.BlendDestination:
		return BlendDestination
	case blend.BlendSourceOver:
		return BlendSourceOver
	case blend.BlendDestinationOver:
		return BlendDestinationOver
	case blend.BlendSourceIn:
		return BlendSourceIn
	case blend.BlendDestinationIn:
		return BlendDestinationIn
	case blend.BlendSourceOut:
		return BlendSourceOut
	case blend.BlendDestinationOut:
		return BlendDestinationOut
	case blend.BlendSourceAtop:
		return BlendSourceAtop
	case blend.BlendDestinationAtop:
		return BlendDestinationAtop
	case blend.BlendXor:
		return BlendXor
	case blend.BlendPlus:
		return BlendPlus
	case blend.BlendModulate:
		return BlendMultiply // Modulate is similar to Multiply

	// Advanced separable blend modes
	case blend.BlendMultiply:
		return BlendMultiply
	case blend.BlendScreen:
		return BlendScreen
	case blend.BlendOverlay:
		return BlendOverlay
	case blend.BlendDarken:
		return BlendDarken
	case blend.BlendLighten:
		return BlendLighten
	case blend.BlendColorDodge:
		return BlendColorDodge
	case blend.BlendColorBurn:
		return BlendColorBurn
	case blend.BlendHardLight:
		return BlendHardLight
	case blend.BlendSoftLight:
		return BlendSoftLight
	case blend.BlendDifference:
		return BlendDifference
	case blend.BlendExclusion:
		return BlendExclusion

	// HSL non-separable blend modes
	case blend.BlendHue:
		return BlendHue
	case blend.BlendSaturation:
		return BlendSaturation
	case blend.BlendColor:
		return BlendColor
	case blend.BlendLuminosity:
		return BlendLuminosity

	default:
		return BlendNormal
	}
}

// AllBlendModes returns a slice of all supported blend modes.
// This is useful for testing and iteration.
func AllBlendModes() []BlendMode {
	return []BlendMode{
		// Standard modes
		BlendNormal,
		BlendMultiply,
		BlendScreen,
		BlendOverlay,
		BlendDarken,
		BlendLighten,
		BlendColorDodge,
		BlendColorBurn,
		BlendHardLight,
		BlendSoftLight,
		BlendDifference,
		BlendExclusion,
		// HSL modes
		BlendHue,
		BlendSaturation,
		BlendColor,
		BlendLuminosity,
		// Porter-Duff modes
		BlendClear,
		BlendCopy,
		BlendDestination,
		BlendSourceOver,
		BlendDestinationOver,
		BlendSourceIn,
		BlendDestinationIn,
		BlendSourceOut,
		BlendDestinationOut,
		BlendSourceAtop,
		BlendDestinationAtop,
		BlendXor,
		BlendPlus,
	}
}

// PorterDuffModes returns a slice of Porter-Duff compositing modes.
func PorterDuffModes() []BlendMode {
	return []BlendMode{
		BlendClear,
		BlendCopy,
		BlendDestination,
		BlendSourceOver,
		BlendDestinationOver,
		BlendSourceIn,
		BlendDestinationIn,
		BlendSourceOut,
		BlendDestinationOut,
		BlendSourceAtop,
		BlendDestinationAtop,
		BlendXor,
		BlendPlus,
	}
}

// AdvancedModes returns a slice of advanced separable blend modes.
func AdvancedModes() []BlendMode {
	return []BlendMode{
		BlendNormal,
		BlendMultiply,
		BlendScreen,
		BlendOverlay,
		BlendDarken,
		BlendLighten,
		BlendColorDodge,
		BlendColorBurn,
		BlendHardLight,
		BlendSoftLight,
		BlendDifference,
		BlendExclusion,
	}
}

// HSLModes returns a slice of HSL-based non-separable blend modes.
func HSLModes() []BlendMode {
	return []BlendMode{
		BlendHue,
		BlendSaturation,
		BlendColor,
		BlendLuminosity,
	}
}
