package scene

import (
	"testing"

	"github.com/gogpu/gg"
)

func TestNewDecoder(t *testing.T) {
	tests := []struct {
		name    string
		enc     *Encoding
		wantNil bool
	}{
		{
			name:    "nil encoding returns nil",
			enc:     nil,
			wantNil: true,
		},
		{
			name:    "empty encoding",
			enc:     NewEncoding(),
			wantNil: false,
		},
		{
			name: "encoding with content",
			enc: func() *Encoding {
				e := NewEncoding()
				e.encodeMoveTo(10, 20)
				return e
			}(),
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec := NewDecoder(tt.enc)
			if (dec == nil) != tt.wantNil {
				t.Errorf("NewDecoder() = %v, wantNil = %v", dec, tt.wantNil)
			}
		})
	}
}

func TestDecoder_Next(t *testing.T) {
	enc := NewEncoding()
	enc.encodeMoveTo(10, 20)
	enc.encodeLineTo(30, 40)

	dec := NewDecoder(enc)

	// First Next should return true
	if !dec.Next() {
		t.Error("First Next() should return true")
	}
	if dec.Tag() != TagMoveTo {
		t.Errorf("Expected TagMoveTo, got %v", dec.Tag())
	}

	// Second Next should return true
	if !dec.Next() {
		t.Error("Second Next() should return true")
	}
	if dec.Tag() != TagLineTo {
		t.Errorf("Expected TagLineTo, got %v", dec.Tag())
	}

	// Third Next should return false (end of stream)
	if dec.Next() {
		t.Error("Third Next() should return false")
	}
}

func TestDecoder_MoveTo(t *testing.T) {
	enc := NewEncoding()
	enc.encodeMoveTo(100.5, 200.5)

	dec := NewDecoder(enc)
	if !dec.Next() {
		t.Fatal("Expected Next() to return true")
	}

	x, y := dec.MoveTo()
	if x != 100.5 || y != 200.5 {
		t.Errorf("MoveTo() = (%v, %v), want (100.5, 200.5)", x, y)
	}
}

func TestDecoder_LineTo(t *testing.T) {
	enc := NewEncoding()
	enc.encodeLineTo(50.5, 75.5)

	dec := NewDecoder(enc)
	if !dec.Next() {
		t.Fatal("Expected Next() to return true")
	}

	x, y := dec.LineTo()
	if x != 50.5 || y != 75.5 {
		t.Errorf("LineTo() = (%v, %v), want (50.5, 75.5)", x, y)
	}
}

func TestDecoder_QuadTo(t *testing.T) {
	enc := NewEncoding()
	enc.encodeQuadTo(10, 20, 30, 40)

	dec := NewDecoder(enc)
	if !dec.Next() {
		t.Fatal("Expected Next() to return true")
	}

	cx, cy, x, y := dec.QuadTo()
	if cx != 10 || cy != 20 || x != 30 || y != 40 {
		t.Errorf("QuadTo() = (%v, %v, %v, %v), want (10, 20, 30, 40)", cx, cy, x, y)
	}
}

func TestDecoder_CubicTo(t *testing.T) {
	enc := NewEncoding()
	enc.encodeCubicTo(10, 20, 30, 40, 50, 60)

	dec := NewDecoder(enc)
	if !dec.Next() {
		t.Fatal("Expected Next() to return true")
	}

	c1x, c1y, c2x, c2y, x, y := dec.CubicTo()
	if c1x != 10 || c1y != 20 || c2x != 30 || c2y != 40 || x != 50 || y != 60 {
		t.Errorf("CubicTo() = (%v, %v, %v, %v, %v, %v), want (10, 20, 30, 40, 50, 60)",
			c1x, c1y, c2x, c2y, x, y)
	}
}

func TestDecoder_Transform(t *testing.T) {
	enc := NewEncoding()
	expected := TranslateAffine(100, 200)
	enc.EncodeTransform(expected)

	dec := NewDecoder(enc)
	if !dec.Next() {
		t.Fatal("Expected Next() to return true")
	}

	got := dec.Transform()
	if got != expected {
		t.Errorf("Transform() = %v, want %v", got, expected)
	}
}

func TestDecoder_Fill(t *testing.T) {
	enc := NewEncoding()
	brush := SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1})
	enc.EncodeFill(brush, FillEvenOdd)

	dec := NewDecoder(enc)
	if !dec.Next() {
		t.Fatal("Expected Next() to return true")
	}

	gotBrush, gotStyle := dec.Fill()
	if gotBrush.Color != brush.Color {
		t.Errorf("Fill() brush = %v, want %v", gotBrush.Color, brush.Color)
	}
	if gotStyle != FillEvenOdd {
		t.Errorf("Fill() style = %v, want %v", gotStyle, FillEvenOdd)
	}
}

func TestDecoder_Stroke(t *testing.T) {
	enc := NewEncoding()
	brush := SolidBrush(gg.RGBA{R: 0, G: 1, B: 0, A: 1})
	style := &StrokeStyle{
		Width:      3.5,
		MiterLimit: 8.0,
		Cap:        LineCapRound,
		Join:       LineJoinBevel,
	}
	enc.EncodeStroke(brush, style)

	dec := NewDecoder(enc)
	if !dec.Next() {
		t.Fatal("Expected Next() to return true")
	}

	gotBrush, gotStyle := dec.Stroke()
	if gotBrush.Color != brush.Color {
		t.Errorf("Stroke() brush = %v, want %v", gotBrush.Color, brush.Color)
	}
	if gotStyle.Width != style.Width {
		t.Errorf("Stroke() width = %v, want %v", gotStyle.Width, style.Width)
	}
	if gotStyle.MiterLimit != style.MiterLimit {
		t.Errorf("Stroke() miterLimit = %v, want %v", gotStyle.MiterLimit, style.MiterLimit)
	}
	if gotStyle.Cap != style.Cap {
		t.Errorf("Stroke() cap = %v, want %v", gotStyle.Cap, style.Cap)
	}
	if gotStyle.Join != style.Join {
		t.Errorf("Stroke() join = %v, want %v", gotStyle.Join, style.Join)
	}
}

func TestDecoder_PushLayer(t *testing.T) {
	enc := NewEncoding()
	enc.EncodePushLayer(BlendMultiply, 0.75)

	dec := NewDecoder(enc)
	if !dec.Next() {
		t.Fatal("Expected Next() to return true")
	}

	blend, alpha := dec.PushLayer()
	if blend != BlendMultiply {
		t.Errorf("PushLayer() blend = %v, want %v", blend, BlendMultiply)
	}
	if alpha != 0.75 {
		t.Errorf("PushLayer() alpha = %v, want 0.75", alpha)
	}
}

func TestDecoder_Reset(t *testing.T) {
	enc := NewEncoding()
	enc.encodeMoveTo(10, 20)
	enc.encodeLineTo(30, 40)

	dec := NewDecoder(enc)

	// Consume all commands
	for dec.Next() {
		// Advance through all tags
	}

	// Reset and verify we can iterate again
	dec.Reset(enc)

	if !dec.Next() {
		t.Error("After Reset(), Next() should return true")
	}
	if dec.Tag() != TagMoveTo {
		t.Errorf("After Reset(), first tag should be TagMoveTo, got %v", dec.Tag())
	}
}

func TestDecoder_HasMore(t *testing.T) {
	enc := NewEncoding()
	enc.encodeMoveTo(10, 20)

	dec := NewDecoder(enc)

	if !dec.HasMore() {
		t.Error("HasMore() should return true before consuming")
	}

	dec.Next() // Consume the single command

	if dec.HasMore() {
		t.Error("HasMore() should return false after consuming all")
	}
}

func TestDecoder_Peek(t *testing.T) {
	enc := NewEncoding()
	enc.encodeMoveTo(10, 20)
	enc.encodeLineTo(30, 40)

	dec := NewDecoder(enc)

	// Peek without advancing
	tag := dec.Peek()
	if tag != TagMoveTo {
		t.Errorf("Peek() = %v, want TagMoveTo", tag)
	}

	// Position should not have changed
	if dec.Position() != 0 {
		t.Error("Peek() should not advance position")
	}

	// Now advance and verify Peek shows next tag
	dec.Next()
	tag = dec.Peek()
	if tag != TagLineTo {
		t.Errorf("Peek() after Next() = %v, want TagLineTo", tag)
	}
}

func TestDecoder_CollectPath(t *testing.T) {
	enc := NewEncoding()
	enc.tags = append(enc.tags, TagBeginPath)
	enc.encodeMoveTo(0, 0)
	enc.encodeLineTo(100, 0)
	enc.encodeLineTo(100, 100)
	enc.tags = append(enc.tags, TagClosePath, TagEndPath)

	dec := NewDecoder(enc)
	dec.Next() // Skip BeginPath

	path := dec.CollectPath()

	if path == nil {
		t.Fatal("CollectPath() returned nil")
	}
	if path.VerbCount() != 4 { // MoveTo, LineTo, LineTo, Close
		t.Errorf("CollectPath() verb count = %d, want 4", path.VerbCount())
	}
}

func TestDecoder_SkipPath(t *testing.T) {
	enc := NewEncoding()
	enc.tags = append(enc.tags, TagBeginPath)
	enc.encodeMoveTo(0, 0)
	enc.encodeLineTo(100, 0)
	enc.encodeLineTo(100, 100)
	enc.tags = append(enc.tags, TagClosePath, TagEndPath)
	enc.encodeMoveTo(200, 200) // After path ends

	dec := NewDecoder(enc)
	dec.Next() // Skip BeginPath
	dec.SkipPath()

	// Should now be positioned after the path
	if !dec.HasMore() {
		t.Error("Should have more commands after SkipPath()")
	}

	if dec.Next() {
		if dec.Tag() != TagMoveTo {
			t.Errorf("After SkipPath(), next tag = %v, want TagMoveTo", dec.Tag())
		}
		x, y := dec.MoveTo()
		if x != 200 || y != 200 {
			t.Errorf("After SkipPath(), MoveTo() = (%v, %v), want (200, 200)", x, y)
		}
	}
}

func TestDecoder_ComplexScene(t *testing.T) {
	// Build a scene with multiple commands
	scene := NewScene()
	rect := NewRectShape(10, 10, 100, 50)
	scene.Fill(FillNonZero, IdentityAffine(), SolidBrush(gg.RGBA{R: 1, G: 0, B: 0, A: 1}), rect)

	circle := NewCircleShape(200, 100, 30)
	scene.Stroke(DefaultStrokeStyle(), IdentityAffine(), SolidBrush(gg.RGBA{R: 0, G: 1, B: 0, A: 1}), circle)

	enc := scene.Encoding()
	dec := NewDecoder(enc)

	// Count commands
	tagCounts := make(map[Tag]int)
	for dec.Next() {
		tagCounts[dec.Tag()]++

		// Consume data for each tag
		switch dec.Tag() {
		case TagMoveTo:
			dec.MoveTo()
		case TagLineTo:
			dec.LineTo()
		case TagQuadTo:
			dec.QuadTo()
		case TagCubicTo:
			dec.CubicTo()
		case TagTransform:
			dec.Transform()
		case TagFill:
			dec.Fill()
		case TagStroke:
			dec.Stroke()
		case TagPushLayer:
			dec.PushLayer()
		case TagImage:
			dec.Image()
		case TagBrush:
			dec.Brush()
		}
	}

	// Verify we have expected command types
	if tagCounts[TagBeginPath] < 2 {
		t.Errorf("Expected at least 2 BeginPath, got %d", tagCounts[TagBeginPath])
	}
	if tagCounts[TagFill] < 1 {
		t.Errorf("Expected at least 1 Fill, got %d", tagCounts[TagFill])
	}
	if tagCounts[TagStroke] < 1 {
		t.Errorf("Expected at least 1 Stroke, got %d", tagCounts[TagStroke])
	}
}

func TestDecoder_Encoding(t *testing.T) {
	enc := NewEncoding()
	dec := NewDecoder(enc)

	if dec.Encoding() != enc {
		t.Error("Encoding() should return the original encoding")
	}
}
