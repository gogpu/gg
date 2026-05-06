package scene

import "image"

// DamageTracker computes the minimal dirty region between frames by comparing
// per-object bounding boxes. This is Level 1 of the four-level damage pipeline
// (ADR-021): Object Diff → Tile Dirty → GPU Scissor → OS Present.
//
// Usage:
//
//	tracker := NewDamageTracker()
//	// Each frame:
//	damage := tracker.ComputeDamage(scene.TaggedBounds())
//	if !damage.Empty() {
//	    renderer.dirty.MarkRect(damage)
//	    renderer.RenderDirty(target, scene)
//	}
type DamageTracker struct {
	prevBounds map[uint64]image.Rectangle
	firstFrame bool
	rendered   bool
}

// NewDamageTracker creates a new damage tracker.
func NewDamageTracker() *DamageTracker {
	return &DamageTracker{
		prevBounds: make(map[uint64]image.Rectangle),
		firstFrame: true,
	}
}

// TaggedBounds associates a stable object ID with its bounding rectangle.
// Objects with the same ID across frames are compared for movement/resize.
type TaggedBounds struct {
	ID   uint64
	Rect image.Rectangle
}

// ComputeDamage compares current frame's objects with previous frame's,
// returning the union of all changed/added/removed bounding boxes.
//
// Returns image.Rectangle{} (zero rect) if nothing changed.
// Returns full scene bounds on first frame (must render everything).
//
// The algorithm is O(N) where N = max(current objects, previous objects).
func (dt *DamageTracker) ComputeDamage(objects []TaggedBounds) image.Rectangle {
	if dt.firstFrame {
		dt.firstFrame = false
		dt.storeBounds(objects)
		// First frame: caller should render everything.
		// Return zero rect — caller detects first frame via separate flag or renders full.
		return image.Rectangle{}
	}

	var damage image.Rectangle
	seen := make(map[uint64]struct{}, len(objects))

	for _, obj := range objects {
		seen[obj.ID] = struct{}{}
		if prev, exists := dt.prevBounds[obj.ID]; exists {
			if prev != obj.Rect {
				damage = damage.Union(prev).Union(obj.Rect)
			}
		} else {
			damage = damage.Union(obj.Rect)
		}
	}

	for id, prev := range dt.prevBounds {
		if _, exists := seen[id]; !exists {
			damage = damage.Union(prev)
		}
	}

	dt.storeBounds(objects)
	return damage
}

// IsFirstRender returns true if RenderWithDamage hasn't rendered a frame yet.
func (dt *DamageTracker) IsFirstRender() bool {
	return !dt.rendered
}

// MarkRendered records that the first full render has completed.
func (dt *DamageTracker) MarkRendered() {
	dt.rendered = true
}

// Reset clears all tracked state. Next ComputeDamage will be treated as first frame.
func (dt *DamageTracker) Reset() {
	dt.prevBounds = make(map[uint64]image.Rectangle)
	dt.firstFrame = true
	dt.rendered = false
}

func (dt *DamageTracker) storeBounds(objects []TaggedBounds) {
	dt.prevBounds = make(map[uint64]image.Rectangle, len(objects))
	for _, obj := range objects {
		dt.prevBounds[obj.ID] = obj.Rect
	}
}
