package gpu

import (
	"image"
	"testing"
)

func TestComputeDamageScissor(t *testing.T) {
	tests := []struct {
		name      string
		groupClip *[4]uint32
		surfaceW  uint32
		surfaceH  uint32
		damage    image.Rectangle
		wantX     uint32
		wantY     uint32
		wantW     uint32
		wantH     uint32
		wantValid bool
	}{
		{
			name:      "damage only, no group clip",
			groupClip: nil,
			surfaceW:  800, surfaceH: 600,
			damage: image.Rect(170, 410, 218, 458),
			wantX:  170, wantY: 410, wantW: 48, wantH: 48,
			wantValid: true,
		},
		{
			name:      "damage intersects group clip",
			groupClip: &[4]uint32{150, 400, 100, 100}, // x=150, y=400, w=100, h=100
			surfaceW:  800, surfaceH: 600,
			damage: image.Rect(170, 410, 218, 458),
			wantX:  170, wantY: 410, wantW: 48, wantH: 48,
			wantValid: true,
		},
		{
			name:      "group clip smaller than damage",
			groupClip: &[4]uint32{180, 420, 20, 20}, // x=180, y=420, w=20, h=20
			surfaceW:  800, surfaceH: 600,
			damage: image.Rect(170, 410, 218, 458),
			wantX:  180, wantY: 420, wantW: 20, wantH: 20,
			wantValid: true,
		},
		{
			name:      "group clip outside damage — empty intersection",
			groupClip: &[4]uint32{0, 0, 50, 50}, // top-left corner
			surfaceW:  800, surfaceH: 600,
			damage:    image.Rect(170, 410, 218, 458), // center-bottom
			wantValid: false,
		},
		{
			name:      "damage clamped to surface bounds",
			groupClip: nil,
			surfaceW:  200, surfaceH: 200,
			damage: image.Rect(170, 180, 300, 300), // extends beyond surface
			wantX:  170, wantY: 180, wantW: 30, wantH: 20,
			wantValid: true,
		},
		{
			name:      "damage fully outside surface — empty",
			groupClip: nil,
			surfaceW:  100, surfaceH: 100,
			damage:    image.Rect(200, 200, 300, 300),
			wantValid: false,
		},
		{
			name:      "partial overlap — group clip partially in damage",
			groupClip: &[4]uint32{160, 400, 80, 80}, // x=160..240, y=400..480
			surfaceW:  800, surfaceH: 600,
			damage: image.Rect(170, 410, 218, 458), // x=170..218, y=410..458
			wantX:  170, wantY: 410, wantW: 48, wantH: 48,
			wantValid: true,
		},
		{
			name:      "full surface group clip — damage is effective scissor",
			groupClip: &[4]uint32{0, 0, 800, 600},
			surfaceW:  800, surfaceH: 600,
			damage: image.Rect(100, 100, 200, 200),
			wantX:  100, wantY: 100, wantW: 100, wantH: 100,
			wantValid: true,
		},
		{
			name:      "zero-size damage",
			groupClip: nil,
			surfaceW:  800, surfaceH: 600,
			damage:    image.Rect(100, 100, 100, 100), // zero width
			wantValid: false,
		},
		{
			name:      "negative coords in damage clamped to 0",
			groupClip: nil,
			surfaceW:  800, surfaceH: 600,
			damage: image.Rect(-10, -10, 50, 50),
			wantX:  0, wantY: 0, wantW: 50, wantH: 50,
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, w, h, valid := computeDamageScissor(tt.groupClip, tt.surfaceW, tt.surfaceH, tt.damage)
			if valid != tt.wantValid {
				t.Fatalf("valid = %v, want %v", valid, tt.wantValid)
			}
			if !valid {
				return
			}
			if x != tt.wantX || y != tt.wantY || w != tt.wantW || h != tt.wantH {
				t.Errorf("scissor = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					x, y, w, h, tt.wantX, tt.wantY, tt.wantW, tt.wantH)
			}
		})
	}
}
