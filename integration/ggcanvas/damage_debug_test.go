package ggcanvas

import (
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/gogpu/gpucontext"
)

func TestGGDamageOverlay_RefreshSameRect(t *testing.T) {
	o := &ggDamageOverlay{}

	r := image.Rect(170, 410, 218, 458)
	o.updateFlashes([]gpucontext.DamageSourceSnapshot{
		{Name: "gg", Color: color.RGBA{R: 0, G: 204, B: 0, A: 102}, Rects: []image.Rectangle{r}},
	})

	if len(o.flashes) != 1 {
		t.Fatalf("first update: want 1 flash, got %d", len(o.flashes))
	}
	firstTime := o.flashes[0].time

	// Same rect again — should refresh time, NOT create new flash.
	time.Sleep(time.Millisecond)
	o.updateFlashes([]gpucontext.DamageSourceSnapshot{
		{Name: "gg", Color: color.RGBA{R: 0, G: 204, B: 0, A: 102}, Rects: []image.Rectangle{r}},
	})

	if len(o.flashes) != 1 {
		t.Errorf("second update same rect: want 1 flash (refreshed), got %d", len(o.flashes))
	}
	if !o.flashes[0].time.After(firstTime) {
		t.Error("flash time should be refreshed (newer than first)")
	}
}

func TestGGDamageOverlay_DifferentRectsNotDeduped(t *testing.T) {
	o := &ggDamageOverlay{}

	r1 := image.Rect(10, 10, 50, 50)
	r2 := image.Rect(100, 100, 200, 200)

	o.updateFlashes([]gpucontext.DamageSourceSnapshot{
		{Name: "gg", Color: color.RGBA{G: 204, A: 102}, Rects: []image.Rectangle{r1}},
	})
	o.updateFlashes([]gpucontext.DamageSourceSnapshot{
		{Name: "gg", Color: color.RGBA{G: 204, A: 102}, Rects: []image.Rectangle{r2}},
	})

	if len(o.flashes) != 2 {
		t.Errorf("different rects: want 2 flashes, got %d", len(o.flashes))
	}
}

func TestGGDamageOverlay_ExpiredFlashAllowsNewForSameRect(t *testing.T) {
	o := &ggDamageOverlay{}

	r := image.Rect(10, 10, 50, 50)
	o.updateFlashes([]gpucontext.DamageSourceSnapshot{
		{Name: "gg", Color: color.RGBA{G: 204, A: 102}, Rects: []image.Rectangle{r}},
	})

	// Simulate flash expiry by backdating.
	o.flashes[0].time = time.Now().Add(-ggDamageFlashDuration - time.Millisecond)

	// Update again — expired flash pruned, same rect should create new flash.
	o.updateFlashes([]gpucontext.DamageSourceSnapshot{
		{Name: "gg", Color: color.RGBA{G: 204, A: 102}, Rects: []image.Rectangle{r}},
	})

	if len(o.flashes) != 1 {
		t.Errorf("after expiry: want 1 new flash, got %d", len(o.flashes))
	}

	// New flash should have recent time.
	if time.Since(o.flashes[0].time) > time.Second {
		t.Error("new flash time should be recent")
	}
}

func TestGGDamageOverlay_NeedsAnimationFrameFalseAfterExpiry(t *testing.T) {
	o := &ggDamageOverlay{}

	o.updateFlashes([]gpucontext.DamageSourceSnapshot{
		{Name: "gg", Color: color.RGBA{G: 204, A: 102}, Rects: []image.Rectangle{image.Rect(10, 10, 50, 50)}},
	})

	if !o.needsAnimationFrame() {
		t.Error("should need frame during active flash")
	}

	// Expire all flashes.
	o.flashes[0].time = time.Now().Add(-ggDamageFlashDuration - time.Millisecond)

	if o.needsAnimationFrame() {
		t.Error("should NOT need frame after all flashes expired")
	}
}

func TestGGDamageOverlay_FeedbackLoopBroken(t *testing.T) {
	// Simulates the feedback loop scenario:
	// Frame 1: source reports spinner rect → flash
	// Frames 2-10: source reports same rect → refresh time → 1 flash (not 10)
	// Spinner stops → no more updates → flash expires → needsAnimationFrame=false

	o := &ggDamageOverlay{}
	spinner := image.Rect(170, 410, 218, 458)
	snap := []gpucontext.DamageSourceSnapshot{
		{Name: "gg", Color: color.RGBA{G: 204, A: 102}, Rects: []image.Rectangle{spinner}},
	}

	// Frame 1
	o.updateFlashes(snap)
	if len(o.flashes) != 1 {
		t.Fatalf("frame 1: want 1 flash, got %d", len(o.flashes))
	}

	// Frames 2-10: same spinner rect every frame
	for i := 2; i <= 10; i++ {
		o.updateFlashes(snap)
	}

	// Still only 1 flash (refreshed, not duplicated)
	if len(o.flashes) != 1 {
		t.Errorf("after 10 frames: want 1 flash (refreshed), got %d", len(o.flashes))
	}

	// While spinner animates, flash stays alive (time refreshed each frame).
	if !o.needsAnimationFrame() {
		t.Error("during animation: should need frame (flash still active)")
	}

	// Spinner stops — no more updates. Flash expires after 400ms.
	o.flashes[0].time = time.Now().Add(-ggDamageFlashDuration - time.Millisecond)

	if o.needsAnimationFrame() {
		t.Error("after spinner stopped + flash expired: loop should be broken")
	}
}

func TestGGDamageOverlay_EmptyRectsIgnored(t *testing.T) {
	o := &ggDamageOverlay{}

	o.updateFlashes([]gpucontext.DamageSourceSnapshot{
		{Name: "gg", Color: color.RGBA{G: 204, A: 102}, Rects: []image.Rectangle{
			{},
			image.Rect(5, 5, 5, 5),
		}},
	})

	if len(o.flashes) != 0 {
		t.Errorf("empty rects should be ignored, got %d flashes", len(o.flashes))
	}
}

func TestGGDamageOverlay_FullSurface(t *testing.T) {
	o := &ggDamageOverlay{}

	o.updateFlashes([]gpucontext.DamageSourceSnapshot{
		{Name: "g3d", Color: color.RGBA{B: 204, A: 102}, Full: true},
	})

	if len(o.flashes) != 1 {
		t.Fatalf("full surface: want 1 flash, got %d", len(o.flashes))
	}
	if !o.flashes[0].full {
		t.Error("flash should be marked as full surface")
	}
	if o.flashes[0].rect != (image.Rectangle{}) {
		t.Error("full surface flash rect should be zero")
	}
}

func TestGGDamageOverlay_MultipleSourcesSameFrame(t *testing.T) {
	o := &ggDamageOverlay{}

	o.updateFlashes([]gpucontext.DamageSourceSnapshot{
		{Name: "gg", Color: color.RGBA{G: 204, A: 102}, Rects: []image.Rectangle{image.Rect(0, 0, 100, 100)}},
		{Name: "g3d", Color: color.RGBA{B: 204, A: 102}, Full: true},
	})

	if len(o.flashes) != 2 {
		t.Fatalf("two sources: want 2 flashes, got %d", len(o.flashes))
	}
	if o.flashes[0].name != "gg" {
		t.Errorf("first flash: want name gg, got %s", o.flashes[0].name)
	}
	if o.flashes[1].name != "g3d" {
		t.Errorf("second flash: want name g3d, got %s", o.flashes[1].name)
	}
}

func TestFormatLabel(t *testing.T) {
	tests := []struct {
		name  string
		flash ggDamageFlash
		want  string
	}{
		{
			name:  "full surface without reason",
			flash: ggDamageFlash{name: "g3d", full: true},
			want:  "g3d (full surface)",
		},
		{
			name: "full surface with reason",
			flash: ggDamageFlash{
				name:   "g3d",
				full:   true,
				reason: gpucontext.DamageReason{Category: gpucontext.DamageCategoryAnimation, Detail: "camera rotation"},
			},
			want: "g3d: camera rotation (full surface)",
		},
		{
			name: "partial without reason",
			flash: ggDamageFlash{
				name: "gg",
				rect: image.Rect(0, 0, 100, 120),
			},
			want: "gg (1 rect, 12000px)",
		},
		{
			name: "partial with reason",
			flash: ggDamageFlash{
				name:   "g3d",
				rect:   image.Rect(0, 0, 600, 600),
				reason: gpucontext.DamageReason{Category: gpucontext.DamageCategoryAnimation, Detail: "camera rotation"},
			},
			want: "g3d: camera rotation (1 rect, 360000px)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatLabel(&tt.flash)
			if got != tt.want {
				t.Errorf("formatLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGGDamageOverlay_ReasonPreserved(t *testing.T) {
	o := &ggDamageOverlay{}
	reason := gpucontext.DamageReason{
		Category: gpucontext.DamageCategoryAnimation,
		Detail:   "spinner tick",
	}

	o.updateFlashes([]gpucontext.DamageSourceSnapshot{
		{
			Name:   "ui",
			Color:  color.RGBA{R: 255, G: 165, A: 102},
			Rects:  []image.Rectangle{image.Rect(10, 10, 50, 50)},
			Reason: reason,
		},
	})

	if len(o.flashes) != 1 {
		t.Fatalf("want 1 flash, got %d", len(o.flashes))
	}
	if o.flashes[0].reason != reason {
		t.Errorf("reason not preserved: got %+v, want %+v", o.flashes[0].reason, reason)
	}
}

func TestGGDamageOverlay_DifferentSourcesSameRect(t *testing.T) {
	o := &ggDamageOverlay{}
	r := image.Rect(0, 0, 100, 100)

	o.updateFlashes([]gpucontext.DamageSourceSnapshot{
		{Name: "gg", Color: color.RGBA{G: 204, A: 102}, Rects: []image.Rectangle{r}},
		{Name: "g3d", Color: color.RGBA{B: 204, A: 102}, Rects: []image.Rectangle{r}},
	})

	// Different sources with same rect = 2 separate flashes.
	if len(o.flashes) != 2 {
		t.Errorf("different sources same rect: want 2 flashes, got %d", len(o.flashes))
	}
}

func TestFormatLabel_ZeroRect(t *testing.T) {
	f := &ggDamageFlash{name: "test", rect: image.Rectangle{}}
	got := formatLabel(f)
	want := "test (1 rect, 0px)"
	if got != want {
		t.Errorf("formatLabel zero rect = %q, want %q", got, want)
	}
}

func TestGGDamageOverlay_InterfaceCompliance(t *testing.T) {
	// Verify compile-time interface check works at test time too.
	var r gpucontext.DamageOverlayRenderer = &ggDamageOverlay{}
	if r == nil {
		t.Error("should not be nil")
	}
	// Verify the method exists with correct signature.
	_ = fmt.Sprintf("%T", r)
}
