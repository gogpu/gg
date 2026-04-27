package scene

import (
	"testing"

	"github.com/gogpu/gg"
)

func buildChildScene(t *testing.T) *Scene {
	t.Helper()
	s := NewScene()
	brush := SolidBrush(gg.RGBA{R: 1, A: 1})
	shape := NewRectShape(0, 0, 100, 50)
	s.Fill(FillNonZero, IdentityAffine(), brush, shape)
	return s
}

func TestSceneAppendWithTranslation_ZeroOffset(t *testing.T) {
	child := buildChildScene(t)

	withZero := NewScene()
	withZero.AppendWithTranslation(child, 0, 0)

	withAppend := NewScene()
	withAppend.Append(child)

	encZ := withZero.Encoding()
	encA := withAppend.Encoding()

	if len(encZ.pathData) != len(encA.pathData) {
		t.Fatalf("pathData len: zero=%d, append=%d", len(encZ.pathData), len(encA.pathData))
	}
	for i := range encZ.pathData {
		if encZ.pathData[i] != encA.pathData[i] {
			t.Errorf("pathData[%d]: zero=%v, append=%v", i, encZ.pathData[i], encA.pathData[i])
		}
	}
}

func TestSceneAppendWithTranslation_RectOffset(t *testing.T) {
	child := NewScene()
	brush := SolidBrush(gg.RGBA{R: 1, A: 1})
	shape := NewRoundedRectShape(0, 0, 100, 50, 5)
	child.Fill(FillNonZero, IdentityAffine(), brush, shape)

	parent := NewScene()
	parent.AppendWithTranslation(child, 30, 40)

	enc := parent.Encoding()

	// Find FillRoundRect pathData: minX, minY, maxX, maxY, rx, ry
	pathIdx := 0
	for _, tag := range enc.tags {
		switch tag {
		case TagFillRoundRect:
			if enc.pathData[pathIdx] != 30 {
				t.Errorf("minX = %v, want 30", enc.pathData[pathIdx])
			}
			if enc.pathData[pathIdx+1] != 40 {
				t.Errorf("minY = %v, want 40", enc.pathData[pathIdx+1])
			}
			if enc.pathData[pathIdx+2] != 130 {
				t.Errorf("maxX = %v, want 130", enc.pathData[pathIdx+2])
			}
			if enc.pathData[pathIdx+3] != 90 {
				t.Errorf("maxY = %v, want 90", enc.pathData[pathIdx+3])
			}
			if enc.pathData[pathIdx+4] != 5 {
				t.Errorf("rx = %v, want 5 (no offset)", enc.pathData[pathIdx+4])
			}
			if enc.pathData[pathIdx+5] != 5 {
				t.Errorf("ry = %v, want 5 (no offset)", enc.pathData[pathIdx+5])
			}
			pathIdx += 6
		case TagMoveTo, TagLineTo:
			pathIdx += 2
		case TagQuadTo:
			pathIdx += 4
		case TagCubicTo:
			pathIdx += 6
		case TagBrush:
			pathIdx += 4
		}
	}
}

func TestSceneAppendWithTranslation_PathCoordinates(t *testing.T) {
	child := NewScene()
	path := NewPath()
	path.MoveTo(0, 0)
	path.LineTo(10, 0)
	path.QuadTo(15, 5, 10, 10)
	path.CubicTo(5, 15, 0, 15, 0, 10)
	path.Close()

	brush := SolidBrush(gg.RGBA{R: 0, G: 1, A: 1})
	child.Fill(FillNonZero, IdentityAffine(), brush, NewPathShape(path))

	parent := NewScene()
	parent.AppendWithTranslation(child, 50, 100)

	enc := parent.Encoding()
	pathIdx := 0
	for _, tag := range enc.tags {
		switch tag {
		case TagMoveTo:
			x, y := enc.pathData[pathIdx], enc.pathData[pathIdx+1]
			if x != 50 || y != 100 {
				t.Errorf("MoveTo = (%v,%v), want (50,100)", x, y)
			}
			pathIdx += 2
		case TagLineTo:
			x, y := enc.pathData[pathIdx], enc.pathData[pathIdx+1]
			if x != 60 || y != 100 {
				t.Errorf("LineTo = (%v,%v), want (60,100)", x, y)
			}
			pathIdx += 2
		case TagQuadTo:
			cx, cy := enc.pathData[pathIdx], enc.pathData[pathIdx+1]
			x, y := enc.pathData[pathIdx+2], enc.pathData[pathIdx+3]
			if cx != 65 || cy != 105 {
				t.Errorf("QuadTo.cp = (%v,%v), want (65,105)", cx, cy)
			}
			if x != 60 || y != 110 {
				t.Errorf("QuadTo.end = (%v,%v), want (60,110)", x, y)
			}
			pathIdx += 4
		case TagCubicTo:
			c1x, c1y := enc.pathData[pathIdx], enc.pathData[pathIdx+1]
			if c1x != 55 || c1y != 115 {
				t.Errorf("CubicTo.c1 = (%v,%v), want (55,115)", c1x, c1y)
			}
			pathIdx += 6
		case TagBrush:
			pathIdx += 4
		case TagFillRoundRect:
			pathIdx += 6
		}
	}
}

func TestSceneAppendWithTranslation_TransformsVerbatim(t *testing.T) {
	child := NewScene()
	child.PushTransform(TranslateAffine(5, 10))
	brush := SolidBrush(gg.RGBA{A: 1})
	child.Fill(FillNonZero, IdentityAffine(), brush, NewRectShape(0, 0, 10, 10))
	child.PopTransform()

	parent := NewScene()
	parent.AppendWithTranslation(child, 100, 200)

	enc := parent.Encoding()
	for i, tr := range enc.transforms {
		if tr.C == 105 || tr.F == 210 {
			t.Errorf("transform[%d].C=%v, F=%v — should NOT be offset (verbatim copy)", i, tr.C, tr.F)
		}
	}
}

func TestSceneAppendWithTranslation_BrushNotOffset(t *testing.T) {
	child := NewScene()
	brush := SolidBrush(gg.RGBA{R: 0.25, G: 0.5, B: 0.75, A: 1.0})
	child.Fill(FillNonZero, IdentityAffine(), brush, NewRectShape(0, 0, 10, 10))

	parent := NewScene()
	parent.AppendWithTranslation(child, 999, 999)

	enc := parent.Encoding()
	pathIdx := 0
	for _, tag := range enc.tags {
		switch tag {
		case TagBrush:
			r, g, b, a := enc.pathData[pathIdx], enc.pathData[pathIdx+1], enc.pathData[pathIdx+2], enc.pathData[pathIdx+3]
			if r != 0.25 || g != 0.5 || b != 0.75 || a != 1.0 {
				t.Errorf("Brush RGBA = (%v,%v,%v,%v), want (0.25,0.5,0.75,1) — NOT offset", r, g, b, a)
			}
			pathIdx += 4
		case TagMoveTo, TagLineTo:
			pathIdx += 2
		case TagQuadTo:
			pathIdx += 4
		case TagCubicTo:
			pathIdx += 6
		case TagFillRoundRect:
			pathIdx += 6
		}
	}
}

func TestSceneAppendWithTranslation_BoundsOffset(t *testing.T) {
	child := NewScene()
	brush := SolidBrush(gg.RGBA{A: 1})
	child.Fill(FillNonZero, IdentityAffine(), brush, NewRectShape(0, 0, 100, 50))

	parent := NewScene()
	parent.AppendWithTranslation(child, 30, 40)

	b := parent.Bounds()
	if b.MinX > 30 || b.MinY > 40 || b.MaxX < 130 || b.MaxY < 90 {
		t.Errorf("bounds = {%v,%v,%v,%v}, want to contain {30,40,130,90}", b.MinX, b.MinY, b.MaxX, b.MaxY)
	}
}

func TestSceneAppendWithTranslation_Nil(t *testing.T) {
	parent := NewScene()
	parent.AppendWithTranslation(nil, 100, 200)
	if !parent.IsEmpty() {
		t.Error("nil append should be no-op")
	}
}

func TestSceneAppendWithTranslation_Empty(t *testing.T) {
	parent := NewScene()
	parent.AppendWithTranslation(NewScene(), 100, 200)
	if !parent.IsEmpty() {
		t.Error("empty append should be no-op")
	}
}
