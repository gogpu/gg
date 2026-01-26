package gg

import (
	"testing"
)

// mockRenderer is a test renderer for DI testing.
type mockRenderer struct {
	fillCalled   bool
	strokeCalled bool
}

func (m *mockRenderer) Fill(pixmap *Pixmap, path *Path, paint *Paint) error {
	m.fillCalled = true
	return nil
}

func (m *mockRenderer) Stroke(pixmap *Pixmap, path *Path, paint *Paint) error {
	m.strokeCalled = true
	return nil
}

// TestNewContextDefault tests that NewContext uses software renderer by default.
func TestNewContextDefault(t *testing.T) {
	dc := NewContext(100, 100)
	if dc == nil {
		t.Fatal("NewContext returned nil")
	}

	// Verify dimensions
	if dc.Width() != 100 {
		t.Errorf("Width() = %d, want 100", dc.Width())
	}
	if dc.Height() != 100 {
		t.Errorf("Height() = %d, want 100", dc.Height())
	}

	// Verify renderer is set (should be SoftwareRenderer)
	if dc.renderer == nil {
		t.Error("renderer is nil, expected SoftwareRenderer")
	}
}

// TestNewContextWithRenderer tests dependency injection of custom renderer.
func TestNewContextWithRenderer(t *testing.T) {
	mock := &mockRenderer{}

	dc := NewContext(100, 100, WithRenderer(mock))
	if dc == nil {
		t.Fatal("NewContext returned nil")
	}

	// Verify custom renderer is used
	if dc.renderer != mock {
		t.Error("renderer is not the injected mock renderer")
	}

	// Test that drawing uses the injected renderer
	dc.DrawCircle(50, 50, 25)
	dc.Fill()

	if !mock.fillCalled {
		t.Error("mock.Fill was not called")
	}
}

// TestNewContextWithPixmap tests dependency injection of custom pixmap.
func TestNewContextWithPixmap(t *testing.T) {
	customPixmap := NewPixmap(200, 200)

	dc := NewContext(100, 100, WithPixmap(customPixmap))
	if dc == nil {
		t.Fatal("NewContext returned nil")
	}

	// Verify custom pixmap is used
	if dc.pixmap != customPixmap {
		t.Error("pixmap is not the injected custom pixmap")
	}

	// Note: dimensions come from constructor, not pixmap
	if dc.Width() != 100 {
		t.Errorf("Width() = %d, want 100", dc.Width())
	}
}

// TestNewContextMultipleOptions tests combining multiple options.
func TestNewContextMultipleOptions(t *testing.T) {
	mock := &mockRenderer{}
	customPixmap := NewPixmap(200, 200)

	dc := NewContext(100, 100,
		WithRenderer(mock),
		WithPixmap(customPixmap),
	)
	if dc == nil {
		t.Fatal("NewContext returned nil")
	}

	// Verify both options are applied
	if dc.renderer != mock {
		t.Error("renderer is not the injected mock renderer")
	}
	if dc.pixmap != customPixmap {
		t.Error("pixmap is not the injected custom pixmap")
	}
}

// TestNewContextForImageWithRenderer tests DI with NewContextForImage.
func TestNewContextForImageWithRenderer(t *testing.T) {
	mock := &mockRenderer{}
	pm := NewPixmap(100, 100)

	dc := NewContextForImage(pm.ToImage(), WithRenderer(mock))
	if dc == nil {
		t.Fatal("NewContextForImage returned nil")
	}

	// Verify custom renderer is used
	if dc.renderer != mock {
		t.Error("renderer is not the injected mock renderer")
	}
}

// TestRendererInterface verifies that Renderer interface is properly defined.
func TestRendererInterface(t *testing.T) {
	var _ Renderer = (*mockRenderer)(nil)
	var _ Renderer = (*SoftwareRenderer)(nil)
}

// mockAnalyticFiller is a test analytic filler for testing WithAnalyticAA.
type mockAnalyticFiller struct {
	fillCalled  bool
	resetCalled bool
}

func (m *mockAnalyticFiller) Fill(path *Path, fillRule FillRule, callback func(y int, iter func(yield func(x int, alpha uint8) bool))) {
	m.fillCalled = true
}

func (m *mockAnalyticFiller) Reset() {
	m.resetCalled = true
}

// TestNewContextWithAnalyticAA tests dependency injection of analytic filler.
func TestNewContextWithAnalyticAA(t *testing.T) {
	mock := &mockAnalyticFiller{}

	dc := NewContext(100, 100, WithAnalyticAA(mock))
	if dc == nil {
		t.Fatal("NewContext returned nil")
	}

	// Verify renderer is a SoftwareRenderer with analytic filler
	sr, ok := dc.renderer.(*SoftwareRenderer)
	if !ok {
		t.Fatal("renderer is not SoftwareRenderer")
	}

	// Verify analytic mode is enabled
	if sr.RenderMode() != RenderModeAnalytic {
		t.Errorf("RenderMode() = %v, want RenderModeAnalytic", sr.RenderMode())
	}
}

// TestWithAnalyticAAIgnoredWhenCustomRenderer tests that WithAnalyticAA
// is ignored when a custom renderer is provided.
func TestWithAnalyticAAIgnoredWhenCustomRenderer(t *testing.T) {
	mockFiller := &mockAnalyticFiller{}
	mockRend := &mockRenderer{}

	dc := NewContext(100, 100,
		WithRenderer(mockRend),
		WithAnalyticAA(mockFiller),
	)
	if dc == nil {
		t.Fatal("NewContext returned nil")
	}

	// Custom renderer should take precedence
	if dc.renderer != mockRend {
		t.Error("renderer is not the injected mock renderer")
	}
}
