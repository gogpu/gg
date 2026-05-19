package gg

import (
	"image"
	"testing"
)

func TestTrackDamage_HiDPI_ScalesToPhysical(t *testing.T) {
	dc := NewContext(400, 300, WithDeviceScale(2.0))
	defer dc.Close()

	dc.DrawRectangle(0, 0, 400, 300)
	dc.Fill()

	rects := dc.FrameDamage()
	if len(rects) == 0 {
		t.Fatal("expected damage rects")
	}

	r := rects[0]
	if r.Dx() != 800 || r.Dy() != 600 {
		t.Errorf("damage rect = %v; want (0,0)-(800,600) physical", r)
	}
}

func TestTrackDamage_Scale1_NoScaling(t *testing.T) {
	dc := NewContext(400, 300)
	defer dc.Close()

	dc.DrawRectangle(0, 0, 400, 300)
	dc.Fill()

	rects := dc.FrameDamage()
	if len(rects) == 0 {
		t.Fatal("expected damage rects")
	}

	r := rects[0]
	if r.Dx() != 400 || r.Dy() != 300 {
		t.Errorf("damage rect = %v; want (0,0)-(400,300)", r)
	}
}

func TestTrackDamage_HiDPI_PartialRect(t *testing.T) {
	dc := NewContext(400, 300, WithDeviceScale(2.0))
	defer dc.Close()

	dc.DrawRectangle(100, 50, 200, 150)
	dc.Fill()

	rects := dc.FrameDamage()
	if len(rects) == 0 {
		t.Fatal("expected damage rects")
	}

	r := rects[0]
	want := image.Rect(200, 100, 600, 400)
	if r != want {
		t.Errorf("damage rect = %v; want %v (physical = logical × 2)", r, want)
	}
}

func TestTrackDamageRect_HiDPI_PublicAPI(t *testing.T) {
	dc := NewContext(400, 300, WithDeviceScale(2.0))
	defer dc.Close()

	dc.TrackDamageRect(image.Rect(10, 20, 100, 80))

	rects := dc.FrameDamage()
	if len(rects) == 0 {
		t.Fatal("expected damage rects")
	}

	r := rects[0]
	want := image.Rect(20, 40, 200, 160)
	if r != want {
		t.Errorf("TrackDamageRect = %v; want %v (physical = logical × 2)", r, want)
	}
}

func TestTrackDamage_Scale3(t *testing.T) {
	dc := NewContext(400, 300, WithDeviceScale(3.0))
	defer dc.Close()

	dc.DrawRectangle(0, 0, 100, 100)
	dc.Fill()

	rects := dc.FrameDamage()
	if len(rects) == 0 {
		t.Fatal("expected damage rects")
	}

	r := rects[0]
	if r.Dx() != 300 || r.Dy() != 300 {
		t.Errorf("damage rect = %v; want 300×300 (physical = 100 × 3)", r)
	}
}

func TestTrackDamage_HiDPI_Stroke(t *testing.T) {
	dc := NewContext(400, 300, WithDeviceScale(2.0))
	defer dc.Close()

	dc.SetLineWidth(2)
	dc.DrawLine(10, 10, 200, 150)
	dc.Stroke()

	rects := dc.FrameDamage()
	if len(rects) == 0 {
		t.Fatal("expected damage rects from Stroke")
	}

	r := rects[0]
	if r.Min.X < 18 || r.Min.Y < 18 || r.Max.X > 402 || r.Max.Y > 302 {
		t.Logf("stroke damage rect = %v (physical)", r)
	}
	if r.Dx() < 2 || r.Dy() < 2 {
		t.Errorf("stroke damage rect too small: %v", r)
	}
}

func TestTrackDamage_FractionalScale(t *testing.T) {
	dc := NewContext(400, 300, WithDeviceScale(1.5))
	defer dc.Close()

	dc.DrawRectangle(0, 0, 100, 100)
	dc.Fill()

	rects := dc.FrameDamage()
	if len(rects) == 0 {
		t.Fatal("expected damage rects")
	}

	r := rects[0]
	if r.Dx() != 150 || r.Dy() != 150 {
		t.Errorf("damage rect = %v; want 150×150 (physical = 100 × 1.5)", r)
	}
}

func TestTrackDamage_FractionalCoords(t *testing.T) {
	dc := NewContext(400, 300, WithDeviceScale(2.0))
	defer dc.Close()

	dc.DrawCircle(50.5, 75.5, 30)
	dc.Fill()

	rects := dc.FrameDamage()
	if len(rects) == 0 {
		t.Fatal("expected damage rects from circle")
	}

	r := rects[0]
	// Circle at (50.5, 75.5) r=30 → bounds ~(20.5, 45.5)-(80.5, 105.5)
	// Scaled ×2 with Floor/Ceil → (41, 91)-(161, 211)
	if r.Min.X > 42 || r.Min.Y > 92 {
		t.Errorf("damage rect Min too large: %v", r)
	}
	if r.Max.X < 160 || r.Max.Y < 210 {
		t.Errorf("damage rect Max too small: %v", r)
	}
}

func TestTrackDamage_MultipleRectsScaled(t *testing.T) {
	dc := NewContext(400, 300, WithDeviceScale(2.0))
	defer dc.Close()

	dc.DrawRectangle(0, 0, 50, 50)
	dc.Fill()
	dc.DrawRectangle(200, 200, 50, 50)
	dc.Fill()

	rects := dc.FrameDamage()
	if len(rects) < 2 {
		t.Fatalf("expected ≥2 damage rects, got %d", len(rects))
	}

	r0 := rects[0]
	r1 := rects[1]

	if r0 != image.Rect(0, 0, 100, 100) {
		t.Errorf("rect[0] = %v; want (0,0)-(100,100)", r0)
	}
	if r1 != image.Rect(400, 400, 500, 500) {
		t.Errorf("rect[1] = %v; want (400,400)-(500,500)", r1)
	}
}
