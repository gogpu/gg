package ggcanvas

import (
	"image"
	"testing"
	"time"
)

func TestDamageOverlay_RefreshSameRect(t *testing.T) {
	var s damageOverlayState

	r := image.Rect(170, 410, 218, 458)
	s.update([]image.Rectangle{r})

	if len(s.flashes) != 1 {
		t.Fatalf("first update: want 1 flash, got %d", len(s.flashes))
	}
	firstTime := s.flashes[0].time

	// Same rect again — should refresh time, NOT create new flash.
	time.Sleep(time.Millisecond)
	s.update([]image.Rectangle{r})

	if len(s.flashes) != 1 {
		t.Errorf("second update same rect: want 1 flash (refreshed), got %d", len(s.flashes))
	}
	if !s.flashes[0].time.After(firstTime) {
		t.Error("flash time should be refreshed (newer than first)")
	}
}

func TestDamageOverlay_DifferentRectsNotDeduped(t *testing.T) {
	var s damageOverlayState

	r1 := image.Rect(10, 10, 50, 50)
	r2 := image.Rect(100, 100, 200, 200)

	s.update([]image.Rectangle{r1})
	s.update([]image.Rectangle{r2})

	if len(s.flashes) != 2 {
		t.Errorf("different rects: want 2 flashes, got %d", len(s.flashes))
	}
}

func TestDamageOverlay_ExpiredFlashAllowsNewForSameRect(t *testing.T) {
	var s damageOverlayState

	r := image.Rect(10, 10, 50, 50)
	s.update([]image.Rectangle{r})

	// Simulate flash expiry by backdating.
	s.flashes[0].time = time.Now().Add(-damageFlashDuration - time.Millisecond)

	// Update again — expired flash pruned, same rect should create new flash.
	s.update([]image.Rectangle{r})

	if len(s.flashes) != 1 {
		t.Errorf("after expiry: want 1 new flash, got %d", len(s.flashes))
	}

	// New flash should have recent time.
	if time.Since(s.flashes[0].time) > time.Second {
		t.Error("new flash time should be recent")
	}
}

func TestDamageOverlay_NeedsAnimationFrameFalseAfterExpiry(t *testing.T) {
	var s damageOverlayState

	s.update([]image.Rectangle{image.Rect(10, 10, 50, 50)})

	if !s.needsAnimationFrame() {
		t.Error("should need frame during active flash")
	}

	// Expire all flashes.
	s.flashes[0].time = time.Now().Add(-damageFlashDuration - time.Millisecond)

	if s.needsAnimationFrame() {
		t.Error("should NOT need frame after all flashes expired")
	}
}

func TestDamageOverlay_FeedbackLoopBroken(t *testing.T) {
	// Simulates the feedback loop scenario:
	// Frame 1: TrackDamageRect(spinner) → update → flash
	// Frames 2-10: TrackDamageRect(spinner) again → refresh time → 1 flash (not 10)
	// Spinner stops → no more updates → flash expires → NeedsAnimationFrame=false

	var s damageOverlayState
	spinner := image.Rect(170, 410, 218, 458)

	// Frame 1
	s.update([]image.Rectangle{spinner})
	if len(s.flashes) != 1 {
		t.Fatalf("frame 1: want 1 flash, got %d", len(s.flashes))
	}

	// Frames 2-10: same spinner rect every frame (TrackDamageRect from compositor)
	for i := 2; i <= 10; i++ {
		s.update([]image.Rectangle{spinner})
	}

	// Still only 1 flash (refreshed, not duplicated)
	if len(s.flashes) != 1 {
		t.Errorf("after 10 frames: want 1 flash (refreshed), got %d", len(s.flashes))
	}

	// While spinner animates, flash stays alive (time refreshed each frame).
	// NeedsAnimationFrame=true — but this is correct, loop doesn't grow.
	if !s.needsAnimationFrame() {
		t.Error("during animation: should need frame (flash still active)")
	}

	// Spinner stops — no more updates. Flash expires after 400ms.
	s.flashes[0].time = time.Now().Add(-damageFlashDuration - time.Millisecond)

	if s.needsAnimationFrame() {
		t.Error("after spinner stopped + flash expired: loop should be broken")
	}
}

func TestDamageOverlay_EmptyRectsIgnored(t *testing.T) {
	var s damageOverlayState

	s.update([]image.Rectangle{
		{},
		image.Rect(5, 5, 5, 5),
	})

	if len(s.flashes) != 0 {
		t.Errorf("empty rects should be ignored, got %d flashes", len(s.flashes))
	}
}
