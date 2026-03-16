package scene

import (
	"testing"

	"github.com/gogpu/gg"
)

// --- Scene Path tests ---

func TestScenePathReverse(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(100, 100)
	p.Close()

	rev := p.Reverse()
	if rev == nil {
		t.Fatal("Reverse returned nil")
	}
	// Verify reversed path has content by checking it's not empty
	if rev.IsEmpty() {
		t.Error("reversed path should not be empty")
	}
}

func TestScenePathReverseEmpty(t *testing.T) {
	p := NewPath()
	rev := p.Reverse()
	if rev == nil {
		t.Fatal("Reverse of empty path returned nil")
	}
}

func TestScenePathContainsRect(t *testing.T) {
	p := NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(100, 100)
	p.LineTo(0, 100)
	p.Close()

	// Point inside the rectangle
	if !p.Contains(50, 50) {
		t.Error("expected (50,50) to be inside the rectangle")
	}

	// Point outside the rectangle
	if p.Contains(150, 50) {
		t.Error("expected (150,50) to be outside the rectangle")
	}

	// Point clearly outside
	if p.Contains(-10, -10) {
		t.Error("expected (-10,-10) to be outside the rectangle")
	}
}

func TestScenePathContainsQuad(t *testing.T) {
	p := NewPath()
	p.MoveTo(50, 0)
	p.QuadTo(100, 0, 100, 50)
	p.QuadTo(100, 100, 50, 100)
	p.QuadTo(0, 100, 0, 50)
	p.QuadTo(0, 0, 50, 0)
	p.Close()

	// Center should be inside
	if !p.Contains(50, 50) {
		t.Error("expected center to be inside quad path")
	}
}

func TestScenePathContainsCubic(t *testing.T) {
	p := NewPath()
	p.MoveTo(50, 0)
	p.CubicTo(75, 0, 100, 25, 100, 50)
	p.CubicTo(100, 75, 75, 100, 50, 100)
	p.CubicTo(25, 100, 0, 75, 0, 50)
	p.CubicTo(0, 25, 25, 0, 50, 0)
	p.Close()

	// Center should be inside
	if !p.Contains(50, 50) {
		t.Error("expected center to be inside cubic path")
	}

	// Far outside should not be inside
	if p.Contains(200, 200) {
		t.Error("expected (200,200) to be outside cubic path")
	}
}

func TestScenePathContainsEmptyPath(t *testing.T) {
	p := NewPath()
	if p.Contains(0, 0) {
		t.Error("empty path should not contain any point")
	}
}

// --- SceneBuilder FillRoundRect (coverage for renderer.renderFillRoundRect) ---

func TestSceneBuilderFillRoundRectCoverage(t *testing.T) {
	builder := NewSceneBuilder()
	builder.FillRoundRect(10, 10, 80, 60, 10, 10, SolidBrush(gg.Blue))
	scene := builder.Build()
	if scene == nil {
		t.Fatal("Build returned nil")
	}
	if scene.IsEmpty() {
		t.Error("scene with FillRoundRect should not be empty")
	}
}
