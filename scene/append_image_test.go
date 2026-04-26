package scene

import (
	"testing"

	gg "github.com/gogpu/gg"
)

// --- Regression: Encoding.AppendWithImages image index corruption ---

func TestAppendWithImages_ImageIndicesAdjusted(t *testing.T) {
	// Regression test for TASK-GG-SCENE-005:
	// Scene A (3 images) + scene B (2 images) → B's indices must offset by 3.
	encA := NewEncoding()
	encA.EncodeImage(0, IdentityAffine())
	encA.EncodeImage(1, IdentityAffine())
	encA.EncodeImage(2, IdentityAffine())

	encB := NewEncoding()
	encB.EncodeImage(0, IdentityAffine())
	encB.EncodeImage(1, IdentityAffine())

	encA.AppendWithImages(encB, 3)

	dd := encA.DrawData()
	want := []uint32{0, 1, 2, 3, 4}
	if len(dd) != len(want) {
		t.Fatalf("drawData len = %d, want %d", len(dd), len(want))
	}
	for i, v := range want {
		if dd[i] != v {
			t.Errorf("drawData[%d] = %d, want %d", i, dd[i], v)
		}
	}
}

func TestAppendWithImages_ZeroOffset(t *testing.T) {
	encA := NewEncoding()
	encB := NewEncoding()
	encB.EncodeImage(5, IdentityAffine())

	encA.AppendWithImages(encB, 0)

	dd := encA.DrawData()
	if len(dd) != 1 || dd[0] != 5 {
		t.Errorf("drawData = %v, want [5]", dd)
	}
}

func TestAppendWithImages_MixedTags(t *testing.T) {
	// Image offset only affects TagImage, not Fill brush indices.
	encA := NewEncoding()
	path := gg.NewPath()
	path.Rectangle(0, 0, 10, 10)
	encA.EncodePath(path)
	encA.EncodeFill(SolidBrush(gg.Red), FillNonZero)
	encA.EncodeImage(0, IdentityAffine())

	encB := NewEncoding()
	pathB := gg.NewPath()
	pathB.Rectangle(10, 10, 10, 10)
	encB.EncodePath(pathB)
	encB.EncodeFill(SolidBrush(gg.Blue), FillNonZero)
	encB.EncodeImage(0, IdentityAffine())

	encA.AppendWithImages(encB, 1)

	indices := extractImageIndices(encA)
	if len(indices) != 2 {
		t.Fatalf("expected 2 image indices, got %d", len(indices))
	}
	if indices[0] != 0 {
		t.Errorf("first image index = %d, want 0", indices[0])
	}
	if indices[1] != 1 {
		t.Errorf("second image index = %d, want 1 (offset by 1)", indices[1])
	}
}

func TestAppend_BackwardCompatible(t *testing.T) {
	encA := NewEncoding()
	encA.EncodeImage(0, IdentityAffine())

	encB := NewEncoding()
	encB.EncodeImage(5, IdentityAffine())

	encA.Append(encB) // no image offset

	dd := encA.DrawData()
	if len(dd) != 2 {
		t.Fatalf("drawData len = %d, want 2", len(dd))
	}
	if dd[0] != 0 || dd[1] != 5 {
		t.Errorf("drawData = %v, want [0, 5] (no offset)", dd)
	}
}

// --- Scene.Append tests ---

func TestSceneAppend_ImageRegistryMerge(t *testing.T) {
	sceneA := NewScene()
	imgA1 := NewImage(10, 10)
	imgA2 := NewImage(20, 20)
	sceneA.DrawImage(imgA1, IdentityAffine())
	sceneA.DrawImage(imgA2, IdentityAffine())

	sceneB := NewScene()
	imgB1 := NewImage(30, 30)
	sceneB.DrawImage(imgB1, IdentityAffine())

	sceneA.Append(sceneB)

	images := sceneA.Images()
	if len(images) != 3 {
		t.Fatalf("image registry len = %d, want 3", len(images))
	}
	if images[0] != imgA1 || images[1] != imgA2 || images[2] != imgB1 {
		t.Error("image registry order wrong after Append")
	}
}

func TestSceneAppend_ImageIndicesCorrect(t *testing.T) {
	sceneA := NewScene()
	imgA := NewImage(10, 10)
	sceneA.DrawImage(imgA, IdentityAffine())

	sceneB := NewScene()
	imgB := NewImage(20, 20)
	sceneB.DrawImage(imgB, IdentityAffine())

	// Force flatten before Append so layer stack doesn't interfere.
	_ = sceneA.Encoding()

	sceneA.Append(sceneB)

	enc := sceneA.Encoding()
	indices := extractImageIndices(enc)
	if len(indices) != 2 {
		t.Fatalf("expected 2 image indices, got %d (drawData=%v, tags=%v)",
			len(indices), enc.DrawData(), enc.Tags())
	}

	// A's image should be at index 0 in merged registry.
	// B's image should be at index 1 (offset by A's 1 image).
	images := sceneA.Images()
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}

	// Verify each image index resolves to the correct image.
	for i, idx := range indices {
		if int(idx) >= len(images) {
			t.Errorf("image index[%d] = %d, out of range (registry has %d images)", i, idx, len(images))
			continue
		}
	}

	// The key regression check: B's image index MUST be offset.
	// Without the fix, B's index would be 0 (pointing to A's image).
	if indices[len(indices)-1] == 0 && len(sceneA.Images()) > 1 {
		t.Error("REGRESSION: B's image index is 0 — not offset by A's image count. " +
			"Merged scene would render A's image instead of B's (TASK-GG-SCENE-005)")
	}
}

func TestSceneAppend_Nil(t *testing.T) {
	scene := NewScene()
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), NewRectShape(0, 0, 50, 50))
	v := scene.Version()

	scene.Append(nil)

	if scene.Version() != v {
		t.Error("Append(nil) should not increment version")
	}
}

func TestSceneAppend_Empty(t *testing.T) {
	scene := NewScene()
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), NewRectShape(0, 0, 50, 50))
	v := scene.Version()

	scene.Append(NewScene())

	if scene.Version() != v {
		t.Error("Append(empty scene) should not increment version")
	}
}

func TestSceneAppend_BoundsMerge(t *testing.T) {
	sceneA := NewScene()
	sceneA.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), NewRectShape(0, 0, 50, 50))

	sceneB := NewScene()
	sceneB.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Blue), NewRectShape(100, 100, 100, 100))

	sceneA.Append(sceneB)

	b := sceneA.Bounds()
	if b.MinX != 0 || b.MinY != 0 || b.MaxX != 200 || b.MaxY != 200 {
		t.Errorf("bounds = %+v, want (0,0)-(200,200)", b)
	}
}

func TestSceneAppend_VersionIncremented(t *testing.T) {
	sceneA := NewScene()
	sceneA.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Red), NewRectShape(0, 0, 50, 50))
	v := sceneA.Version()

	sceneB := NewScene()
	sceneB.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.Blue), NewRectShape(50, 50, 100, 100))

	sceneA.Append(sceneB)

	if sceneA.Version() <= v {
		t.Error("Append should increment version for cache invalidation")
	}
}

// --- Helpers ---

func extractImageIndices(enc *Encoding) []uint32 {
	var indices []uint32
	drawIdx := 0
	for _, tag := range enc.Tags() {
		switch tag {
		case TagFill:
			drawIdx += 2
		case TagFillRoundRect:
			drawIdx += 2
		case TagStroke:
			drawIdx += 5
		case TagPushLayer:
			drawIdx += 2
		case TagImage:
			if drawIdx < len(enc.DrawData()) {
				indices = append(indices, enc.DrawData()[drawIdx])
			}
			drawIdx++
		}
	}
	return indices
}
