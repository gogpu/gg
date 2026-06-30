// Package text provides GPU text rendering infrastructure.
//
// This file provides variation coordinate normalization: converting
// user-space variation values (e.g., weight=700) to F2.14 normalized
// coordinates (range -1.0 to +1.0, stored as int16 -16384 to +16384).
//
// Reference: skrifa (Google fontations) skrifa/src/metrics.rs
// Spec: https://learn.microsoft.com/en-us/typography/opentype/spec/otvaroverview#algorithm-for-normalizing-a-value
package text

// fvarAxis holds the axis definition from the fvar table, used for
// normalizing user-space variation values.
type fvarAxis struct {
	Tag          [4]byte
	MinValue     float32
	DefaultValue float32
	MaxValue     float32
}

// normalizeCoords converts user-space FontVariation values to F2.14
// normalized coordinates, one per axis in the fvar table.
//
// For each axis:
//   - If the user specified a value for the axis tag, use it
//   - Otherwise, use the axis default value
//   - Normalize to [-1.0, +1.0] range relative to (min, default, max)
//   - Store as F2.14 int16 (multiply by 16384)
//
// Matches the OpenType normalization algorithm:
// https://learn.microsoft.com/en-us/typography/opentype/spec/otvaroverview#algorithm-for-normalizing-a-value
func normalizeCoords(axes []fvarAxis, variations []FontVariation) []int16 {
	coords := make([]int16, len(axes))
	for i, axis := range axes {
		value := axis.DefaultValue
		for _, v := range variations {
			if v.Tag == axis.Tag {
				value = v.Value
				break
			}
		}
		coords[i] = normalizeValue(value, axis.MinValue, axis.DefaultValue, axis.MaxValue)
	}
	return coords
}

// normalizeValue normalizes a single axis value to F2.14 format.
//
//   - value == default  → 0
//   - value < default   → linear interpolation in [axisMin, default] → [-1.0, 0.0]
//   - value > default   → linear interpolation in [default, axisMax] → [0.0, +1.0]
//   - result is clamped to [-16384, +16384] (F2.14 range)
func normalizeValue(value, axisMin, def, axisMax float32) int16 {
	if value == def {
		return 0
	}
	var normalized float32
	if value < def {
		if def == axisMin {
			return 0
		}
		normalized = (value - def) / (def - axisMin)
	} else {
		if axisMax == def {
			return 0
		}
		normalized = (value - def) / (axisMax - def)
	}
	// Clamp to F2.14 range and convert.
	result := normalized * 16384
	if result < -16384 {
		result = -16384
	}
	if result > 16384 {
		result = 16384
	}
	return int16(result)
}
