package scene

import (
	"image"
	"testing"
)

func rect(x, y, w, h int) image.Rectangle {
	return image.Rect(x, y, x+w, y+h)
}

func TestDamageTracker_FirstFrame(t *testing.T) {
	dt := NewDamageTracker()
	objects := []TaggedBounds{
		{ID: 1, Rect: rect(10, 10, 50, 50)},
		{ID: 2, Rect: rect(100, 100, 30, 30)},
	}
	damage := dt.ComputeDamage(objects)
	if !damage.Empty() {
		t.Errorf("first frame should return empty damage, got %v", damage)
	}
}

func TestDamageTracker_NothingChanged(t *testing.T) {
	dt := NewDamageTracker()
	objects := []TaggedBounds{
		{ID: 1, Rect: rect(10, 10, 50, 50)},
		{ID: 2, Rect: rect(100, 100, 30, 30)},
	}
	dt.ComputeDamage(objects)
	damage := dt.ComputeDamage(objects)
	if !damage.Empty() {
		t.Errorf("unchanged objects should produce empty damage, got %v", damage)
	}
}

func TestDamageTracker_ObjectMoved(t *testing.T) {
	dt := NewDamageTracker()
	dt.ComputeDamage([]TaggedBounds{
		{ID: 1, Rect: rect(10, 10, 40, 40)},
		{ID: 2, Rect: rect(200, 200, 30, 30)},
	})

	damage := dt.ComputeDamage([]TaggedBounds{
		{ID: 1, Rect: rect(60, 10, 40, 40)},
		{ID: 2, Rect: rect(200, 200, 30, 30)},
	})

	// Damage should cover old position (10,10,50,50) ∪ new position (60,10,100,50)
	if damage.Empty() {
		t.Fatal("moved object should produce damage")
	}
	// Must contain old position
	old := rect(10, 10, 40, 40)
	if !overlaps(damage, old) {
		t.Errorf("damage %v should overlap old position %v", damage, old)
	}
	// Must contain new position
	newPos := rect(60, 10, 40, 40)
	if !overlaps(damage, newPos) {
		t.Errorf("damage %v should overlap new position %v", damage, newPos)
	}
	// Object 2 unchanged — damage should NOT cover it
	obj2 := rect(200, 200, 30, 30)
	if overlaps(damage, obj2) {
		t.Errorf("damage %v should NOT overlap unchanged object %v", damage, obj2)
	}
}

func TestDamageTracker_ObjectAdded(t *testing.T) {
	dt := NewDamageTracker()
	dt.ComputeDamage([]TaggedBounds{
		{ID: 1, Rect: rect(10, 10, 50, 50)},
	})

	damage := dt.ComputeDamage([]TaggedBounds{
		{ID: 1, Rect: rect(10, 10, 50, 50)},
		{ID: 2, Rect: rect(200, 200, 30, 30)},
	})

	expected := rect(200, 200, 30, 30)
	if !overlaps(damage, expected) {
		t.Errorf("damage %v should overlap added object %v", damage, expected)
	}
}

func TestDamageTracker_ObjectRemoved(t *testing.T) {
	dt := NewDamageTracker()
	dt.ComputeDamage([]TaggedBounds{
		{ID: 1, Rect: rect(10, 10, 50, 50)},
		{ID: 2, Rect: rect(200, 200, 30, 30)},
	})

	damage := dt.ComputeDamage([]TaggedBounds{
		{ID: 1, Rect: rect(10, 10, 50, 50)},
	})

	// Damage must cover removed object's old position
	removed := rect(200, 200, 30, 30)
	if !overlaps(damage, removed) {
		t.Errorf("damage %v should overlap removed object %v", damage, removed)
	}
}

func TestDamageTracker_ObjectResized(t *testing.T) {
	dt := NewDamageTracker()
	dt.ComputeDamage([]TaggedBounds{
		{ID: 1, Rect: rect(10, 10, 50, 50)},
	})

	damage := dt.ComputeDamage([]TaggedBounds{
		{ID: 1, Rect: rect(10, 10, 100, 100)},
	})

	if damage.Empty() {
		t.Fatal("resized object should produce damage")
	}
	// Damage should cover both old (50×50) and new (100×100) sizes
	if damage.Dx() < 100 || damage.Dy() < 100 {
		t.Errorf("damage %v should be at least 100×100 to cover resized object", damage)
	}
}

func TestDamageTracker_EmptyScene(t *testing.T) {
	dt := NewDamageTracker()
	dt.ComputeDamage(nil)
	damage := dt.ComputeDamage(nil)
	if !damage.Empty() {
		t.Errorf("empty scene should produce empty damage, got %v", damage)
	}
}

func TestDamageTracker_AllObjectsReplaced(t *testing.T) {
	dt := NewDamageTracker()
	dt.ComputeDamage([]TaggedBounds{
		{ID: 1, Rect: rect(0, 0, 50, 50)},
	})

	damage := dt.ComputeDamage([]TaggedBounds{
		{ID: 2, Rect: rect(100, 100, 50, 50)},
	})

	// Should cover both: removed ID=1 and added ID=2
	if damage.Empty() {
		t.Fatal("replacing all objects should produce damage")
	}
	old := rect(0, 0, 50, 50)
	if !overlaps(damage, old) {
		t.Errorf("damage %v should overlap removed object %v", damage, old)
	}
	newObj := rect(100, 100, 50, 50)
	if !overlaps(damage, newObj) {
		t.Errorf("damage %v should overlap new object %v", damage, newObj)
	}
}

func TestDamageTracker_Reset(t *testing.T) {
	dt := NewDamageTracker()
	dt.ComputeDamage([]TaggedBounds{
		{ID: 1, Rect: rect(10, 10, 50, 50)},
	})
	dt.Reset()

	damage := dt.ComputeDamage([]TaggedBounds{
		{ID: 1, Rect: rect(10, 10, 50, 50)},
	})
	if !damage.Empty() {
		t.Errorf("after Reset, first frame should return empty damage, got %v", damage)
	}
}

func overlaps(r, s image.Rectangle) bool {
	return !r.Intersect(s).Empty()
}
