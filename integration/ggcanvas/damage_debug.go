package ggcanvas

import (
	"image"
	"os"
	"sync"
	"time"

	"github.com/gogpu/gg"
)

// Debug damage visualization (ADR-021 Phase 6a).
// Enabled via GOGPU_DEBUG_DAMAGE=1 environment variable.
// Draws colored overlay on damage regions — like Android "Show surface updates"
// and Chrome "Paint flashing" developer tools.
//
// Flash-and-fade effect (400ms): Android SurfaceFlinger doDebugFlashRegions pattern.
// Draws via gg.Context (Fill/Stroke) — works on ALL backends (Vulkan/DX12/Metal/GLES/Software).
// Damage tracking suppressed during overlay draw to avoid self-inflating damage.
//
// Zero overhead when disabled — env var checked once at init.

var (
	debugDamageOnce    sync.Once
	debugDamageEnabled bool
)

func isDebugDamageEnabled() bool {
	debugDamageOnce.Do(func() {
		debugDamageEnabled = os.Getenv("GOGPU_DEBUG_DAMAGE") == "1"
	})
	return debugDamageEnabled
}

const damageFlashDuration = 400 * time.Millisecond

type damageFlash struct {
	rect image.Rectangle
	time time.Time
}

// damageOverlayState tracks damage flashes with fade effect.
type damageOverlayState struct {
	flashes []damageFlash
}

func (s *damageOverlayState) update(rects []image.Rectangle) {
	now := time.Now()
	alive := s.flashes[:0]
	for _, f := range s.flashes {
		if now.Sub(f.time) < damageFlashDuration {
			alive = append(alive, f)
		}
	}
	s.flashes = alive

	for _, r := range rects {
		if r.Empty() {
			continue
		}
		// Dedup: don't create a new flash if an active flash already covers
		// the same rect. This prevents feedback loops when TrackDamageRect
		// reports the same dirty boundary every frame (e.g., animated spinner).
		// Enterprise pattern: Chrome DevTools Paint Flashing deduplicates
		// paint rects per-layer, Android SurfaceFlinger draws post-composition.
		dup := false
		for i := range s.flashes {
			if s.flashes[i].rect == r {
				dup = true
				break
			}
		}
		if !dup {
			s.flashes = append(s.flashes, damageFlash{rect: r, time: now})
		}
	}
}

// drawAll renders green flash-and-fade overlay via gg.Context.
// Works on all backends. Caller must SetDamageTracking(false) before calling.
func (s *damageOverlayState) drawAll(cc *gg.Context) {
	now := time.Now()
	for _, f := range s.flashes {
		age := now.Sub(f.time)
		if age >= damageFlashDuration {
			continue
		}
		fade := 1.0 - float64(age)/float64(damageFlashDuration)

		x := float64(f.rect.Min.X)
		y := float64(f.rect.Min.Y)
		w := float64(f.rect.Dx())
		h := float64(f.rect.Dy())
		if w <= 0 || h <= 0 {
			continue
		}

		// Green fill with fade.
		cc.SetRGBA(0, 0.8, 0, 0.15*fade)
		cc.DrawRectangle(x, y, w, h)
		_ = cc.Fill()

		// Green border with fade.
		cc.SetRGBA(0, 0.9, 0, 0.7*fade)
		cc.SetLineWidth(2)
		cc.DrawRectangle(x+1, y+1, w-2, h-2)
		_ = cc.Stroke()
	}
}

func (s *damageOverlayState) needsAnimationFrame() bool {
	if len(s.flashes) == 0 {
		return false
	}
	now := time.Now()
	for _, f := range s.flashes {
		if now.Sub(f.time) < damageFlashDuration {
			return true
		}
	}
	return false
}
