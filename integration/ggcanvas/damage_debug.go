package ggcanvas

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/gogpu/gg"
	"github.com/gogpu/gpucontext"
)

// ggDamageOverlay implements gpucontext.DamageOverlayRenderer with text labels.
//
// Renders per-source damage rects as colored flash-and-fade overlays via
// gg.Context (Fill/Stroke/DrawString). Works on ALL backends because gg
// renders through the CPU rasterizer before present.
//
// Text labels show source name, damage reason, rect count, and pixel area.
// Uses the context's current font if set; labels are silently skipped when
// no font is available. The overlay suppresses damage tracking during draw
// to prevent feedback loops (overlay drawing would dirty the overlay area).
//
// ADR-066: pluggable overlay — gogpu provides flat-color quads (no text),
// gg registers this richer renderer via SetDamageOverlayRenderer.

// Compile-time interface check.
var _ gpucontext.DamageOverlayRenderer = (*ggDamageOverlay)(nil)

const ggDamageFlashDuration = 400 * time.Millisecond

// labelFontSize is the font size used for overlay labels (physical pixels).
const labelFontSize = 11.0

// labelPadH is horizontal padding inside the label background box.
const labelPadH = 4.0

// labelPadV is vertical padding inside the label background box.
const labelPadV = 2.0

// ggDamageFlash tracks one active damage flash with per-source metadata.
type ggDamageFlash struct {
	name   string
	color  color.RGBA
	rect   image.Rectangle
	full   bool
	reason gpucontext.DamageReason
	time   time.Time
}

// ggDamageOverlay renders the damage debug overlay using gg.Context.
// Registered with gogpu via SetDamageOverlayRenderer on first Render.
type ggDamageOverlay struct {
	ctx     *gg.Context
	flashes []ggDamageFlash
}

// RenderDamageOverlay draws per-source damage overlays with text labels.
// Called by the gogpu compositor after all content renderers have finished.
func (o *ggDamageOverlay) RenderDamageOverlay(info gpucontext.DamageOverlayInfo) {
	o.updateFlashes(info.Sources)

	if len(o.flashes) == 0 {
		return
	}

	// Suppress damage tracking to avoid feedback (overlay paint triggers damage).
	o.ctx.SetDamageTracking(false)
	defer o.ctx.SetDamageTracking(true)

	now := time.Now()
	for i := range o.flashes {
		f := &o.flashes[i]
		age := now.Sub(f.time)
		if age >= ggDamageFlashDuration {
			continue
		}
		fade := 1.0 - float64(age)/float64(ggDamageFlashDuration)

		rect := f.rect
		if f.full {
			rect = image.Rect(0, 0, int(info.SurfaceWidth), int(info.SurfaceHeight))
		}

		x := float64(rect.Min.X)
		y := float64(rect.Min.Y)
		w := float64(rect.Dx())
		h := float64(rect.Dy())
		if w <= 0 || h <= 0 {
			continue
		}

		// Fill with source color at low alpha.
		o.ctx.SetRGBA(
			float64(f.color.R)/255.0,
			float64(f.color.G)/255.0,
			float64(f.color.B)/255.0,
			0.15*fade,
		)
		o.ctx.DrawRectangle(x, y, w, h)
		_ = o.ctx.Fill()

		// Anti-aliased border at higher alpha (gg AA via CPU rasterizer).
		o.ctx.SetRGBA(
			float64(f.color.R)/255.0,
			float64(f.color.G)/255.0,
			float64(f.color.B)/255.0,
			0.7*fade,
		)
		o.ctx.SetLineWidth(2)
		o.ctx.DrawRectangle(x+1, y+1, w-2, h-2)
		_ = o.ctx.Stroke()

		// Text label (only if font is set on the context).
		o.drawLabel(f, x, y, fade)
	}
}

// drawLabel renders a text label at the top-left of the damage rect.
// Skips silently if no font is set on the gg.Context.
func (o *ggDamageOverlay) drawLabel(f *ggDamageFlash, x, y, fade float64) {
	if o.ctx.Font() == nil {
		return
	}

	label := formatLabel(f)
	if label == "" {
		return
	}

	tw, th := o.ctx.MeasureString(label)
	if tw <= 0 || th <= 0 {
		return
	}

	// Dark semi-transparent background box.
	bgX := x + 2
	bgY := y + 2
	bgW := tw + labelPadH*2
	bgH := th + labelPadV*2

	o.ctx.SetRGBA(0, 0, 0, 0.6*fade)
	o.ctx.DrawRectangle(bgX, bgY, bgW, bgH)
	_ = o.ctx.Fill()

	// White text.
	o.ctx.SetRGBA(1, 1, 1, 0.9*fade)
	o.ctx.DrawString(label, bgX+labelPadH, bgY+labelPadV+th*0.85)
}

// formatLabel builds a human-readable label for a damage flash.
//
// Format examples:
//
//	"g3d: camera rotation (1 rects, 360000px)"
//	"g3d (full surface)"
//	"gg (3 rects, 12000px)"
func formatLabel(f *ggDamageFlash) string {
	if f.full {
		if f.reason.Detail != "" {
			return fmt.Sprintf("%s: %s (full surface)", f.name, f.reason.Detail)
		}
		return fmt.Sprintf("%s (full surface)", f.name)
	}

	area := f.rect.Dx() * f.rect.Dy()
	if f.reason.Detail != "" {
		return fmt.Sprintf("%s: %s (1 rect, %dpx)", f.name, f.reason.Detail, area)
	}
	return fmt.Sprintf("%s (1 rect, %dpx)", f.name, area)
}

// updateFlashes prunes expired flashes, refreshes timestamps for still-active
// rects, and adds new flashes from the current frame's source snapshots.
// "Refresh-or-create" prevents duplicate overlapping flashes for the same
// rect appearing across consecutive frames (Android SurfaceFlinger pattern).
func (o *ggDamageOverlay) updateFlashes(sources []gpucontext.DamageSourceSnapshot) {
	now := time.Now()

	// Prune expired flashes.
	alive := o.flashes[:0]
	for _, f := range o.flashes {
		if now.Sub(f.time) < ggDamageFlashDuration {
			alive = append(alive, f)
		}
	}
	o.flashes = alive

	// Add or refresh flashes from current snapshots.
	for _, snap := range sources {
		if snap.Full {
			o.refreshOrAdd(snap.Name, snap.Color, image.Rectangle{}, true, snap.Reason, now)
			continue
		}
		for _, r := range snap.Rects {
			if r.Empty() {
				continue
			}
			o.refreshOrAdd(snap.Name, snap.Color, r, false, snap.Reason, now)
		}
	}
}

// refreshOrAdd refreshes an existing flash with matching name+rect, or
// creates a new one. Prevents duplicate flashes for the same damage area
// across consecutive frames.
func (o *ggDamageOverlay) refreshOrAdd(name string, c color.RGBA, rect image.Rectangle, full bool, reason gpucontext.DamageReason, now time.Time) {
	for i := range o.flashes {
		f := &o.flashes[i]
		if f.name == name && f.rect == rect && f.full == full {
			f.time = now
			f.reason = reason
			return
		}
	}
	o.flashes = append(o.flashes, ggDamageFlash{
		name:   name,
		color:  c,
		rect:   rect,
		full:   full,
		reason: reason,
		time:   now,
	})
}

// needsAnimationFrame reports whether the overlay has active flashes that
// require additional frames for the fade-out animation. Used by ggcanvas
// to call RequestRedraw for the self-sustaining render loop (Chromium pattern).
func (o *ggDamageOverlay) needsAnimationFrame() bool {
	if len(o.flashes) == 0 {
		return false
	}
	now := time.Now()
	for _, f := range o.flashes {
		if now.Sub(f.time) < ggDamageFlashDuration {
			return true
		}
	}
	return false
}
