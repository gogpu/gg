package text

import (
	"fmt"
	"os"
	"testing"
)

func TestDebug_VarFontDefaultCoords(t *testing.T) {
	data, err := os.ReadFile(`C:\Windows\Fonts\bahnschrift.ttf`)
	if err != nil {
		t.Skip("no Bahnschrift")
	}

	parser := &ownParser{}
	font, err := parser.Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	own := font.(*ownParsedFont)

	// Check gvar deltas at default coords (wght=400)
	own.loadGvar()
	own.loadAvar()

	if own.gvar == nil {
		t.Fatal("no gvar table")
	}

	gidH := font.GlyphIndex('H')
	t.Logf("GlyphIndex('H') = %d, upem=%d", gidH, font.UnitsPerEm())

	// Normalize wght=400 (default)
	variations := []FontVariation{NewFontVariation("wght", 400)}
	coords := normalizeCoords(own.fvarAxes, variations)
	t.Logf("Normalized coords for wght=400: %v", coords)

	// Apply avar
	if own.avar != nil {
		own.avar.apply(coords)
		t.Logf("After avar: %v", coords)
	}

	// Get gvar deltas
	// Need point count from glyf
	contours, cerr := ParseGlyfContours(data, GlyphID(gidH))
	if cerr != nil {
		t.Fatal("ParseGlyfContours:", cerr)
	}
	numPoints := len(contours.Points) + 4 // +4 phantom
	t.Logf("Glyph H: %d contour points + 4 phantom = %d total", len(contours.Points), numPoints)

	// Build points array for glyphVariationDeltas (contour + phantom).
	nPts := len(contours.Points)
	points := make([][2]int32, numPoints)
	for i, pt := range contours.Points {
		points[i] = [2]int32{int32(pt.X), int32(pt.Y)}
	}
	// Phantom points (zeros for delta test).
	for i := nPts; i < numPoints; i++ {
		points[i] = [2]int32{0, 0}
	}

	dx, dy := own.gvar.glyphVariationDeltas(gidH, coords, nPts, contours.EndPts, points)
	t.Logf("gvar deltas count: dx=%d, dy=%d", len(dx), len(dy))

	// Check if any deltas are non-zero
	nonZeroDx, nonZeroDy := 0, 0
	for i, d := range dx {
		if d != 0 {
			nonZeroDx++
			if nonZeroDx <= 5 {
				fmt.Printf("  dx[%d] = %d\n", i, d)
			}
		}
	}
	for i, d := range dy {
		if d != 0 {
			nonZeroDy++
			if nonZeroDy <= 5 {
				fmt.Printf("  dy[%d] = %d\n", i, d)
			}
		}
	}
	t.Logf("Non-zero deltas at wght=400: dx=%d/%d, dy=%d/%d", nonZeroDx, len(dx), nonZeroDy, len(dy))

	if nonZeroDx > 0 || nonZeroDy > 0 {
		t.Errorf("BUG: gvar deltas should be ZERO at default coords (wght=400), got %d+%d non-zero",
			nonZeroDx, nonZeroDy)
	}

	// Also test with NO variations (nil coords)
	dx0, dy0 := own.gvar.glyphVariationDeltas(gidH, nil, nPts, contours.EndPts, points)
	nonZero0 := 0
	for _, d := range dx0 {
		if d != 0 {
			nonZero0++
		}
	}
	for _, d := range dy0 {
		if d != 0 {
			nonZero0++
		}
	}
	t.Logf("Deltas with nil coords: %d non-zero", nonZero0)
}
