package text

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func appendU16(b []byte, v uint16) []byte {
	return binary.BigEndian.AppendUint16(b, v)
}

func buildSimpleSquare() []byte {
	var b []byte
	b = appendU16(b, 1)       // numberOfContours
	b = appendU16(b, 0)       // xMin
	b = appendU16(b, 0)       // yMin
	b = appendU16(b, 100)     // xMax
	b = appendU16(b, 100)     // yMax
	b = appendU16(b, 3)       // endPtsOfContours[0]
	b = appendU16(b, 0)       // instructionLength
	b = append(b, 1, 1, 1, 1) // flags: on-curve, word deltas
	for _, dx := range []int16{0, 100, 0, -100} {
		b = appendU16(b, uint16(dx))
	}
	for _, dy := range []int16{0, 0, 100, 0} {
		b = appendU16(b, uint16(dy))
	}
	return b
}

func compositeHeader() []byte {
	var b []byte
	b = appendU16(b, 0xFFFF) // numberOfContours = -1
	b = appendU16(b, 0)      // xMin
	b = appendU16(b, 0)      // yMin
	b = appendU16(b, 200)    // xMax
	b = appendU16(b, 200)    // yMax
	return b
}

func buildFixtureFont(records ...[]byte) (glyfData, locaData []byte) {
	loca := make([]uint16, 1, 1+len(records))
	loca[0] = 0
	for _, rec := range records {
		glyfData = append(glyfData, rec...)
		if len(glyfData)%2 != 0 {
			glyfData = append(glyfData, 0)
		}
		loca = append(loca, uint16(len(glyfData)/2)) //nolint:gosec // fixture size is tiny
	}
	for _, off := range loca {
		locaData = appendU16(locaData, off)
	}
	return glyfData, locaData
}

func TestSimpleGlyph_Unchanged(t *testing.T) {
	glyf, loca := buildFixtureFont(buildSimpleSquare())
	got, err := extractGlyfContourOwn(glyf, loca, 0, false)
	if err != nil {
		t.Fatalf("extractGlyfContourOwn: %v", err)
	}
	if got == nil || got.IsComposite || len(got.Points) != 4 {
		t.Fatalf("simple glyph parsing changed: %+v", got)
	}
}

func TestCompositeContours_TwoComponents(t *testing.T) {
	// Sibling reuse of the same component gid must not trip cycle detection.
	comp := compositeHeader()
	comp = appendU16(comp, compositeArgsAreXY|compositeMoreComponents)
	comp = appendU16(comp, 0)
	comp = append(comp, 0, 0) // dx=0, dy=0
	comp = appendU16(comp, compositeArgsAreXY)
	comp = appendU16(comp, 0)
	comp = append(comp, 10, 120) // dx=10, dy=120

	glyf, loca := buildFixtureFont(buildSimpleSquare(), comp)
	got, err := extractGlyfContourOwn(glyf, loca, 1, false)
	if err != nil {
		t.Fatalf("extractGlyfContourOwn: %v", err)
	}
	if got == nil || !got.IsComposite {
		t.Fatalf("expected composite contours, got %+v", got)
	}
	if len(got.Points) != 8 {
		t.Fatalf("merged points = %d, want 8", len(got.Points))
	}
	if got.EndPts[0] != 3 || got.EndPts[1] != 7 {
		t.Fatalf("EndPts = %v, want [3 7]", got.EndPts)
	}
	if p := got.Points[4]; p.X != 10 || p.Y != 120 {
		t.Fatalf("second component origin = (%d,%d), want (10,120)", p.X, p.Y)
	}
	if !got.Points[0].OnCurve {
		t.Fatalf("on-curve flag lost in merge")
	}
}

func TestCompositeContours_Scale(t *testing.T) {
	comp := compositeHeader()
	comp = appendU16(comp, compositeArgsAreXY|compositeHaveScale)
	comp = appendU16(comp, 0)
	comp = append(comp, 0, 0)
	comp = appendU16(comp, 0x2000) // F2Dot14 0.5

	glyf, loca := buildFixtureFont(buildSimpleSquare(), comp)
	got, err := extractGlyfContourOwn(glyf, loca, 1, false)
	if err != nil {
		t.Fatalf("extractGlyfContourOwn: %v", err)
	}
	if got == nil || len(got.Points) != 4 {
		t.Fatalf("expected 4 points, got %+v", got)
	}
	if p := got.Points[2]; p.X != 50 || p.Y != 50 {
		t.Fatalf("scaled corner = (%d,%d), want (50,50)", p.X, p.Y)
	}
}

func TestCompositeContours_TwoByTwo(t *testing.T) {
	comp := compositeHeader()
	comp = appendU16(comp, compositeArgsAreXY|compositeHave2x2)
	comp = appendU16(comp, 0)
	comp = append(comp, 0, 0)
	comp = appendU16(comp, 0x0000) // xx = 0
	comp = appendU16(comp, 0xC000) // xy = -1
	comp = appendU16(comp, 0x4000) // yx = 1
	comp = appendU16(comp, 0x0000) // yy = 0

	glyf, loca := buildFixtureFont(buildSimpleSquare(), comp)
	got, err := extractGlyfContourOwn(glyf, loca, 1, false)
	if err != nil {
		t.Fatalf("extractGlyfContourOwn: %v", err)
	}
	if got == nil || len(got.Points) != 4 {
		t.Fatalf("expected 4 points, got %+v", got)
	}
	// (100, 100) → (-100, 100)
	if p := got.Points[2]; p.X != -100 || p.Y != 100 {
		t.Fatalf("rotated corner = (%d,%d), want (-100,100)", p.X, p.Y)
	}
}

func TestCompositeContours_Nested(t *testing.T) {
	inner := compositeHeader()
	inner = appendU16(inner, compositeArgsAreXY|compositeMoreComponents)
	inner = appendU16(inner, 0)
	inner = append(inner, 0, 0)
	inner = appendU16(inner, compositeArgsAreXY)
	inner = appendU16(inner, 0)
	inner = append(inner, 10, 120)

	outer := compositeHeader()
	outer = appendU16(outer, compositeArgsAreXY)
	outer = appendU16(outer, 1)
	outer = append(outer, 5, 7)

	glyf, loca := buildFixtureFont(buildSimpleSquare(), inner, outer)
	got, err := extractGlyfContourOwn(glyf, loca, 2, false)
	if err != nil {
		t.Fatalf("extractGlyfContourOwn: %v", err)
	}
	if got == nil || len(got.Points) != 8 {
		t.Fatalf("expected 8 merged points, got %+v", got)
	}
	if p := got.Points[4]; p.X != 15 || p.Y != 127 {
		t.Fatalf("nested offset composition = (%d,%d), want (15,127)", p.X, p.Y)
	}
}

func TestCompositeContours_SelfCycle_DegradesGracefully(t *testing.T) {
	comp := compositeHeader()
	comp = appendU16(comp, compositeArgsAreXY)
	comp = appendU16(comp, 1)
	comp = append(comp, 0, 0)

	glyf, loca := buildFixtureFont(buildSimpleSquare(), comp)
	got, err := extractGlyfContourOwn(glyf, loca, 1, false)
	if err != nil {
		t.Fatalf("cycle should degrade, not error: %v", err)
	}
	if got != nil {
		t.Fatalf("self-referencing composite should resolve to nil, got %+v", got)
	}
}

func buildFanOutComposite(childGID uint16, count int) []byte {
	comp := compositeHeader()
	for i := range count {
		flags := uint16(compositeArgsAreXY)
		if i < count-1 {
			flags |= compositeMoreComponents
		}
		comp = appendU16(comp, flags)
		comp = appendU16(comp, childGID)
		comp = append(comp, 0, 0) // dx=0, dy=0
	}
	return comp
}

func TestCompositeContours_FanOutBudgetBounded(t *testing.T) {
	const levels = compositeRecursionLimit
	const branch = 6

	records := make([][]byte, levels+1)
	for i := range levels {
		records[i] = buildFanOutComposite(uint16(i+1), branch) //nolint:gosec // fixture size is tiny
	}
	records[levels] = nil // empty leaf glyph

	glyf, loca := buildFixtureFont(records...)

	start := time.Now()
	got, err := extractGlyfContourOwn(glyf, loca, 0, false)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("fan-out resolution took %s — work budget did not bound it", elapsed)
	}
	if err == nil {
		t.Fatalf("expected a budget-exceeded error, got nil error and %+v", got)
	}
	if !strings.Contains(err.Error(), "budget") {
		t.Fatalf("expected a budget-exceeded error, got: %v", err)
	}
}

func newFixtureTTGlyphLoader(records ...[]byte) *ttGlyphLoader {
	var glyfData []byte
	offsets := make([]glyfOffset, len(records))
	for i, rec := range records {
		start := len(glyfData)
		glyfData = append(glyfData, rec...)
		offsets[i] = glyfOffset{offset: uint32(start), length: uint32(len(rec))} //nolint:gosec // fixture size is tiny
	}
	return &ttGlyphLoader{
		font:    &ttFontProgram{},
		tables:  map[string][]byte{"glyf": glyfData},
		glyfOff: offsets,
		hmtxAdv: []uint16{100},
		hmtxLSB: []int16{0},
		numHMtx: 1,
	}
}

func TestTTLoadCompositeGlyphOutline_SelfCycle_DegradesGracefully(t *testing.T) {
	comp := compositeHeader()
	comp = appendU16(comp, compositeArgsAreXY)
	comp = appendU16(comp, 1)
	comp = append(comp, 0, 0)

	l := newFixtureTTGlyphLoader(buildSimpleSquare(), comp)
	got, err := l.loadGlyphOutline(1, 1<<16) // scale 1.0 in 16.16
	if err != nil {
		t.Fatalf("cycle should degrade, not error: %v", err)
	}
	if got == nil {
		t.Fatalf("expected an empty-glyph outline, got nil")
	}
	if len(got.contours) != 0 {
		t.Fatalf("expected no contours from a fully-cyclic composite, got %v", got.contours)
	}
}

func TestTTLoadCompositeGlyphOutline_FanOutBudgetBounded(t *testing.T) {
	const levels = compositeRecursionLimit
	const branch = 6

	records := make([][]byte, levels+1)
	for i := range levels {
		records[i] = buildFanOutComposite(uint16(i+1), branch) //nolint:gosec // fixture size is tiny
	}
	records[levels] = nil // empty leaf glyph

	l := newFixtureTTGlyphLoader(records...)

	start := time.Now()
	got, err := l.loadGlyphOutline(0, 1<<16)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("fan-out resolution took %s — work budget did not bound it", elapsed)
	}
	if err == nil {
		t.Fatalf("expected a budget-exceeded error, got nil error and %+v", got)
	}
	if !strings.Contains(err.Error(), "budget") {
		t.Fatalf("expected a budget-exceeded error, got: %v", err)
	}
}
