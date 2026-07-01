// Package text provides GPU text rendering infrastructure.
//
// This file implements the OpenType HVAR (Horizontal Metrics Variations)
// table parser. HVAR provides advance width deltas for variable fonts,
// allowing precise advance adjustment without reprocessing gvar outlines.
//
// Reference: skrifa (Google fontations)
//   - read-fonts/src/tables/hvar.rs — HVAR parser + advance_width_delta
//   - skrifa/src/metrics.rs:291-311 — advance_width flow
//
// Spec: https://learn.microsoft.com/en-us/typography/opentype/spec/hvar
package text

import (
	"encoding/binary"
	"fmt"
)

// hvarTable holds parsed HVAR (Horizontal Metrics Variations) data.
//
// Binary layout:
//
//	uint16  majorVersion (must be 1)
//	uint16  minorVersion (must be 0)
//	Offset32  itemVariationStoreOffset
//	Offset32  advanceWidthMappingOffset (may be 0 = null)
//	Offset32  lsbMappingOffset (may be 0 = null)
//	Offset32  rsbMappingOffset (may be 0 = null)
type hvarTable struct {
	ivs         *itemVariationStore
	advWidthMap *deltaSetIndexMap // nullable: nil means identity mapping
}

// advanceDelta returns the advance width delta for the given glyph ID
// and normalized variation coordinates.
//
// Matches skrifa Hvar::advance_width_delta (hvar.rs:10-21)
// and variations::advance_delta (variations.rs:1739-1757).
func (h *hvarTable) advanceDelta(glyphID uint16, coords []int16) int32 {
	if h == nil || h.ivs == nil || len(coords) == 0 {
		return 0
	}
	outer, inner := h.advWidthMap.get(glyphID)
	return h.ivs.computeDelta(outer, inner, coords)
}

// parseHVAR parses an HVAR table from raw bytes.
//
// The HVAR header is 20 bytes:
//
//	uint16  majorVersion (2)
//	uint16  minorVersion (2)
//	Offset32  ivsOffset    (4)
//	Offset32  advMapOffset (4)
//	Offset32  lsbMapOffset (4)
//	Offset32  rsbMapOffset (4)
func parseHVAR(data []byte) (*hvarTable, error) {
	if len(data) < 20 {
		return nil, fmt.Errorf("text: HVAR data too short: %d bytes (need 20)", len(data))
	}

	major := binary.BigEndian.Uint16(data[0:2])
	minor := binary.BigEndian.Uint16(data[2:4])
	if major != 1 || minor != 0 {
		return nil, fmt.Errorf("text: unsupported HVAR version %d.%d (expected 1.0)", major, minor)
	}

	ivsOffset := binary.BigEndian.Uint32(data[4:8])
	advMapOffset := binary.BigEndian.Uint32(data[8:12])
	// lsbMapOffset and rsbMapOffset are at [12:16] and [16:20] — not used for advance deltas.

	// Parse ItemVariationStore (required).
	if ivsOffset == 0 || int(ivsOffset) >= len(data) {
		return nil, fmt.Errorf("text: HVAR missing or invalid IVS offset %d", ivsOffset)
	}
	ivs, err := parseItemVariationStore(data[ivsOffset:])
	if err != nil {
		return nil, fmt.Errorf("text: HVAR IVS: %w", err)
	}

	// Parse advance width mapping (optional — may be null).
	var advWidthMap *deltaSetIndexMap
	if advMapOffset != 0 && int(advMapOffset) < len(data) {
		advWidthMap, err = parseDeltaSetIndexMap(data[advMapOffset:])
		if err != nil {
			return nil, fmt.Errorf("text: HVAR advance width mapping: %w", err)
		}
	}

	return &hvarTable{
		ivs:         ivs,
		advWidthMap: advWidthMap,
	}, nil
}

// parseFvarAxes extracts axis definitions from raw fvar table data.
// Uses direct binary parsing.
//
// fvar table layout:
//
//	uint16  majorVersion (must be 1)
//	uint16  minorVersion (must be 0)
//	Offset16 axisArrayOffset
//	uint16  reserved
//	uint16  axisCount
//	uint16  axisSize (must be 20)
//
// Each axis record (20 bytes):
//
//	Tag     axisTag (4 bytes)
//	Fixed   minValue (4 bytes, 16.16)
//	Fixed   defaultValue (4 bytes, 16.16)
//	Fixed   maxValue (4 bytes, 16.16)
//	uint16  flags
//	uint16  axisNameID
func parseFvarAxes(data []byte) []fvarAxis {
	if len(data) < 16 {
		return nil
	}

	axisArrayOffset := binary.BigEndian.Uint16(data[4:6])
	axisCount := binary.BigEndian.Uint16(data[8:10])
	axisSize := binary.BigEndian.Uint16(data[10:12])

	if axisSize < 20 || axisCount == 0 {
		return nil
	}

	start := int(axisArrayOffset)
	if start+int(axisCount)*int(axisSize) > len(data) {
		return nil
	}

	axes := make([]fvarAxis, axisCount)
	for i := range axisCount {
		off := start + int(i)*int(axisSize)
		axes[i] = fvarAxis{
			Tag:          [4]byte{data[off], data[off+1], data[off+2], data[off+3]},
			MinValue:     fixed1616ToFloat32(data[off+4:]),
			DefaultValue: fixed1616ToFloat32(data[off+8:]),
			MaxValue:     fixed1616ToFloat32(data[off+12:]),
		}
	}
	return axes
}

// fixed1616ToFloat32 reads a big-endian Fixed 16.16 value and converts to float32.
func fixed1616ToFloat32(data []byte) float32 {
	raw := int32(binary.BigEndian.Uint32(data[:4]))
	return float32(raw) / 65536.0
}
