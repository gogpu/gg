package ggcanvas

import (
	"image"
	"os"
	"sync"
	"time"

	"github.com/gogpu/gg"
)

// Debug damage visualization (ADR-021 Phase 6).
// Enabled via GOGPU_DEBUG_DAMAGE=1 environment variable.
// Draws colored overlay on damage regions — like Android "Show surface updates"
// and Chrome "Paint flashing" developer tools.
//
// Colors:
//   - Green semi-transparent fill: area that was redrawn this frame
//   - Green border (2px): damage rect boundary
//
// Trail prevention follows Android SurfaceFlinger pattern: full recompose
// each debug frame to erase previous overlay.
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
// Android SurfaceFlinger doDebugFlashRegions pattern.
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
		if !r.Empty() {
			s.flashes = append(s.flashes, damageFlash{rect: r, time: now})
		}
	}
}

func (s *damageOverlayState) drawAll(pm *gg.Pixmap) {
	now := time.Now()
	for _, f := range s.flashes {
		age := now.Sub(f.time)
		if age >= damageFlashDuration {
			continue
		}
		fade := 1.0 - float64(age)/float64(damageFlashDuration)
		drawDamageOverlayFaded(pm, f.rect, fade)
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

// drawDamageOverlayFaded draws damage rect with fade opacity.
func drawDamageOverlayFaded(pm *gg.Pixmap, damage image.Rectangle, fade float64) {
	if damage.Empty() || pm == nil || fade <= 0 {
		return
	}

	pmW := pm.Width()
	pmH := pm.Height()
	pixels := pm.Data()

	r := damage.Intersect(image.Rect(0, 0, pmW, pmH))
	if r.Empty() {
		return
	}

	alpha := byte(60.0 * fade)
	borderAlpha := byte(180.0 * fade)

	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			idx := (y*pmW + x) * 4
			if idx+3 >= len(pixels) {
				continue
			}
			pixels[idx+0] = blendByte(pixels[idx+0], 0, alpha)
			pixels[idx+1] = blendByte(pixels[idx+1], 200, alpha)
			pixels[idx+2] = blendByte(pixels[idx+2], 0, alpha)
		}
	}

	drawBorderRect(pixels, pmW, pmH, r, 2, 0, 255, 0, borderAlpha)
}

// drawDamageOverlay draws a semi-transparent colored rectangle on the pixmap
// highlighting the damage region. Only called when GOGPU_DEBUG_DAMAGE=1.
func drawDamageOverlay(pm *gg.Pixmap, damage image.Rectangle) {
	if damage.Empty() || pm == nil {
		return
	}

	pmW := pm.Width()
	pmH := pm.Height()
	pixels := pm.Data()

	// Clamp to pixmap bounds
	r := damage.Intersect(image.Rect(0, 0, pmW, pmH))
	if r.Empty() {
		return
	}

	// Semi-transparent green fill
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			idx := (y*pmW + x) * 4
			if idx+3 >= len(pixels) {
				continue
			}
			pixels[idx+0] = blendByte(pixels[idx+0], 0, 60)   // R
			pixels[idx+1] = blendByte(pixels[idx+1], 200, 60) // G
			pixels[idx+2] = blendByte(pixels[idx+2], 0, 60)   // B
		}
	}

	// Green border (2px)
	drawBorderRect(pixels, pmW, pmH, r, 2, 0, 255, 0, 180)
}

func drawBorderRect(pixels []byte, pmW, pmH int, r image.Rectangle, thickness int, cr, cg, cb, ca byte) {
	for t := 0; t < thickness; t++ {
		// Top edge
		y := r.Min.Y + t
		if y >= 0 && y < pmH {
			for x := r.Min.X; x < r.Max.X; x++ {
				setPixel(pixels, pmW, x, y, cr, cg, cb, ca)
			}
		}
		// Bottom edge
		y = r.Max.Y - 1 - t
		if y >= 0 && y < pmH && y != r.Min.Y+t {
			for x := r.Min.X; x < r.Max.X; x++ {
				setPixel(pixels, pmW, x, y, cr, cg, cb, ca)
			}
		}
		// Left edge
		x := r.Min.X + t
		if x >= 0 && x < pmW {
			for y := r.Min.Y; y < r.Max.Y; y++ {
				setPixel(pixels, pmW, x, y, cr, cg, cb, ca)
			}
		}
		// Right edge
		x = r.Max.X - 1 - t
		if x >= 0 && x < pmW && x != r.Min.X+t {
			for y := r.Min.Y; y < r.Max.Y; y++ {
				setPixel(pixels, pmW, x, y, cr, cg, cb, ca)
			}
		}
	}
}

func setPixel(pixels []byte, width, x, y int, r, g, b, a byte) {
	if x < 0 || y < 0 || x >= width {
		return
	}
	idx := (y*width + x) * 4
	if idx+3 >= len(pixels) {
		return
	}
	// Alpha blend overlay onto existing pixel
	alpha := int(a)
	invAlpha := 255 - alpha
	pixels[idx+0] = byte((int(pixels[idx+0])*invAlpha + int(r)*alpha) / 255)
	pixels[idx+1] = byte((int(pixels[idx+1])*invAlpha + int(g)*alpha) / 255)
	pixels[idx+2] = byte((int(pixels[idx+2])*invAlpha + int(b)*alpha) / 255)
	pixels[idx+3] = clampByte(int(pixels[idx+3]) + alpha/4)
}

func blendByte(dst, src, alpha byte) byte {
	return byte((int(dst)*(255-int(alpha)) + int(src)*int(alpha)) / 255)
}

func clampByte(v int) byte {
	if v > 255 {
		return 255
	}
	return byte(v)
}
