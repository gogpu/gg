package emoji

import (
	"errors"
	"testing"
)

func TestColor_RGBA(t *testing.T) {
	c := Color{R: 255, G: 128, B: 64, A: 255}
	r, g, b, a := c.RGBA()

	// color.Color.RGBA returns 16-bit values (0-65535)
	if r != 65535 {
		t.Errorf("R = %d, want 65535", r)
	}
	if g != 32896 { // 128 * 257
		t.Errorf("G = %d, want 32896", g)
	}
	if b != 16448 { // 64 * 257
		t.Errorf("B = %d, want 16448", b)
	}
	if a != 65535 {
		t.Errorf("A = %d, want 65535", a)
	}
}

func TestColor_ToRGBA(t *testing.T) {
	c := Color{R: 255, G: 128, B: 64, A: 200}
	rgba := c.ToRGBA()

	if rgba.R != 255 || rgba.G != 128 || rgba.B != 64 || rgba.A != 200 {
		t.Errorf("ToRGBA() = %+v, want {255, 128, 64, 200}", rgba)
	}
}

func TestColorLayer_IsForeground(t *testing.T) {
	tests := []struct {
		name         string
		paletteIndex uint16
		want         bool
	}{
		{"foreground", 0xFFFF, true},
		{"palette 0", 0, false},
		{"palette 10", 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layer := ColorLayer{PaletteIndex: tt.paletteIndex}
			if got := layer.IsForeground(); got != tt.want {
				t.Errorf("IsForeground() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRect(t *testing.T) {
	r := Rect{MinX: 10, MinY: 20, MaxX: 50, MaxY: 80}

	if got := r.Width(); got != 40 {
		t.Errorf("Width() = %v, want 40", got)
	}

	if got := r.Height(); got != 60 {
		t.Errorf("Height() = %v, want 60", got)
	}

	if r.Empty() {
		t.Error("Empty() = true, want false")
	}

	empty := Rect{MinX: 10, MinY: 20, MaxX: 10, MaxY: 20}
	if !empty.Empty() {
		t.Error("Empty() = false for zero-size rect, want true")
	}
}

func TestCOLRGlyph(t *testing.T) {
	glyph := COLRGlyph{
		GlyphID: 100,
		Layers: []ColorLayer{
			{GlyphID: 101, PaletteIndex: 0, Color: Color{R: 255, G: 0, B: 0, A: 255}},
			{GlyphID: 102, PaletteIndex: 1, Color: Color{R: 0, G: 255, B: 0, A: 255}},
		},
		Version: 0,
	}

	if glyph.GlyphID != 100 {
		t.Errorf("GlyphID = %d, want 100", glyph.GlyphID)
	}

	if len(glyph.Layers) != 2 {
		t.Errorf("len(Layers) = %d, want 2", len(glyph.Layers))
	}
}

func TestNewCOLRParser_Errors(t *testing.T) {
	tests := []struct {
		name     string
		colrData []byte
		cpalData []byte
		wantErr  error
	}{
		{
			name:     "no COLR",
			colrData: nil,
			cpalData: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			wantErr:  ErrNoCOLRTable,
		},
		{
			name:     "no CPAL",
			colrData: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			cpalData: nil,
			wantErr:  ErrNoCPALTable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCOLRParser(tt.colrData, tt.cpalData)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("NewCOLRParser() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRenderCOLRToImage_Nil(t *testing.T) {
	result := RenderCOLRToImage(nil, nil, 32, 32, Color{}.ToRGBA())
	if result != nil {
		t.Error("RenderCOLRToImage(nil) should return nil")
	}
}

func TestRenderCOLRToImage_EmptyLayers(t *testing.T) {
	glyph := &COLRGlyph{
		GlyphID: 100,
		Layers:  nil,
	}
	result := RenderCOLRToImage(glyph, nil, 32, 32, Color{}.ToRGBA())
	if result != nil {
		t.Error("RenderCOLRToImage(empty layers) should return nil")
	}
}
