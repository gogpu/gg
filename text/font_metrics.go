// Font metrics parsers — hhea, hmtx, OS/2, head.
//
// Provides horizontal metrics (advance widths, LSBs) and font-level
// metrics (ascent, descent, line gap, xHeight, capHeight) from the
// hhea, hmtx, OS/2, and head tables.
//
// Reuses parseHmtx() from tt_glyph.go for hmtx parsing.
// Reuses parseHeadUnitsPerEm() from tt_font.go for head parsing.
//
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/hhea
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/hmtx
// Reference: https://learn.microsoft.com/en-us/typography/opentype/spec/os2
//
// This file is part of Phase 3a (ADR-048: Pure Go Font Stack).
package text

import "encoding/binary"

// hheaMetrics holds relevant fields from the hhea table.
type hheaMetrics struct {
	ascent           int16
	descent          int16
	lineGap          int16
	numberOfHMetrics int
}

// parseHheaTable parses the hhea (Horizontal Header) table.
//
// Layout (36 bytes):
//
//	uint16  majorVersion (1)
//	uint16  minorVersion (0)
//	int16   ascent                    [offset 4]
//	int16   descent                   [offset 6]
//	int16   lineGap                   [offset 8]
//	uint16  advanceWidthMax           [offset 10]
//	int16   minLeftSideBearing        [offset 12]
//	int16   minRightSideBearing       [offset 14]
//	int16   xMaxExtent                [offset 16]
//	int16   caretSlopeRise            [offset 18]
//	int16   caretSlopeRun             [offset 20]
//	int16   caretOffset               [offset 22]
//	int16   reserved[4]               [offset 24-30]
//	int16   metricDataFormat          [offset 32]
//	uint16  numberOfHMetrics          [offset 34]
func parseHheaTable(data []byte) (hheaMetrics, bool) {
	if len(data) < 36 {
		return hheaMetrics{}, false
	}
	return hheaMetrics{
		ascent:           int16(binary.BigEndian.Uint16(data[4:6])),
		descent:          int16(binary.BigEndian.Uint16(data[6:8])),
		lineGap:          int16(binary.BigEndian.Uint16(data[8:10])),
		numberOfHMetrics: int(binary.BigEndian.Uint16(data[34:36])),
	}, true
}

// os2Metrics holds relevant fields from the OS/2 table for font metrics.
type os2Metrics struct {
	version        uint16
	xAvgCharWidth  int16
	sTypoAscender  int16
	sTypoDescender int16
	sTypoLineGap   int16
	sxHeight       int16  // version >= 2
	sCapHeight     int16  // version >= 2
	usWinAscent    uint16 // fallback if sTypo is zero
	usWinDescent   uint16 // fallback if sTypo is zero
}

// parseOS2Table parses the OS/2 table for font metrics.
//
// Key offsets:
//
//	offset 0:   uint16  version
//	offset 2:   int16   xAvgCharWidth
//	offset 68:  int16   sTypoAscender
//	offset 70:  int16   sTypoDescender
//	offset 72:  int16   sTypoLineGap
//	offset 74:  uint16  usWinAscent
//	offset 76:  uint16  usWinDescent
//	offset 86:  int16   sxHeight       (version >= 2)
//	offset 88:  int16   sCapHeight     (version >= 2)
func parseOS2Table(data []byte) (os2Metrics, bool) {
	if len(data) < 78 {
		return os2Metrics{}, false
	}

	m := os2Metrics{
		version:        binary.BigEndian.Uint16(data[0:2]),
		xAvgCharWidth:  int16(binary.BigEndian.Uint16(data[2:4])),
		sTypoAscender:  int16(binary.BigEndian.Uint16(data[68:70])),
		sTypoDescender: int16(binary.BigEndian.Uint16(data[70:72])),
		sTypoLineGap:   int16(binary.BigEndian.Uint16(data[72:74])),
		usWinAscent:    binary.BigEndian.Uint16(data[74:76]),
		usWinDescent:   binary.BigEndian.Uint16(data[76:78]),
	}

	// sxHeight and sCapHeight are only in version 2+.
	if m.version >= 2 && len(data) >= 90 {
		m.sxHeight = int16(binary.BigEndian.Uint16(data[86:88]))
		m.sCapHeight = int16(binary.BigEndian.Uint16(data[88:90]))
	}

	return m, true
}

// computeFontMetrics computes scaled FontMetrics from hhea and OS/2 data.
//
// Scaling: font units * ppem / unitsPerEm = pixels.
//
// Matches sfnt behavior:
//   - Uses OS/2 sTypoAscender/Descender when available.
//   - Falls back to hhea ascent/descent if OS/2 values are zero.
//   - xHeight and capHeight from OS/2 version 2+.
//   - LineGap from OS/2 sTypoLineGap (or hhea lineGap as fallback).
func computeFontMetrics(hhea hheaMetrics, os2 os2Metrics, upem int, ppem float64) FontMetrics {
	if upem == 0 {
		return FontMetrics{}
	}
	scale := ppem / float64(upem)

	// Prefer OS/2 sTypo metrics; fall back to hhea.
	ascent := float64(os2.sTypoAscender)
	descent := float64(os2.sTypoDescender)
	lineGap := float64(os2.sTypoLineGap)

	if ascent == 0 && descent == 0 {
		// Fallback to hhea metrics.
		ascent = float64(hhea.ascent)
		descent = float64(hhea.descent)
		lineGap = float64(hhea.lineGap)
	}

	xHeight := float64(os2.sxHeight)
	capHeight := float64(os2.sCapHeight)

	return FontMetrics{
		Ascent:    ascent * scale,
		Descent:   descent * scale,
		LineGap:   lineGap * scale,
		XHeight:   xHeight * scale,
		CapHeight: capHeight * scale,
	}
}

// hmtxAdvance returns the advance width in font units for the given glyph ID.
// Glyphs beyond numHMetrics use the last advance width (monospace tail).
//
// This is a standalone lookup function for use by ownParsedFont.
// The actual hmtx parsing is done by parseHmtx() in tt_glyph.go.
func hmtxAdvance(advances []uint16, numHMetrics int, glyphID uint16) uint16 {
	gid := int(glyphID)
	if gid < numHMetrics && gid < len(advances) {
		return advances[gid]
	}
	if numHMetrics > 0 && len(advances) > 0 {
		return advances[numHMetrics-1]
	}
	return 0
}
