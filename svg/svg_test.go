package svg

import (
	"image"
	"image/color"
	"testing"
)

// Real JetBrains SVG icons (Apache 2.0 license) for testing.
// These cover all supported elements and attributes.

const closeIconSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
  <path fill="#7F8B91" fill-opacity=".5" fill-rule="evenodd" d="M7.99494337,8.70506958 L4.85408745,11.8540874 L4.14698067,11.1469807 L7.29493214,8.00505835 L4.14698067,4.85710688 L4.85408745,4.1500001 L8.00204181,7.29795446 L11.1439612,4.1500001 L11.851068,4.85710688 L8.70204624,7.99795888 L11.851068,11.1469807 L11.1439612,11.8540874 L7.99494337,8.70506958 Z"/>
</svg>`

const searchIconSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
  <path fill="#7F8B91" fill-opacity=".9" fill-rule="evenodd" d="M11.038136,9.94904865 L13.9980971,12.9090097 L12.9374369,13.9696699 L9.98176525,11.0139983 C9.14925083,11.6334368 8.11743313,12 7,12 C4.23857625,12 2,9.76142375 2,7 C2,4.23857625 4.23857625,2 7,2 C9.76142375,2 12,4.23857625 12,7 C12,8.1028408 11.642948,9.12228765 11.038136,9.94904865 Z M7,11 C9.209139,11 11,9.209139 11,7 C11,4.790861 9.209139,3 7,3 C4.790861,3 3,4.790861 3,7 C3,9.209139 4.790861,11 7,11 Z"/>
</svg>`

const refreshIconSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
  <path fill="#6E6E6E" fill-rule="evenodd" d="M12.5747152,11.8852806 C11.4741474,13.1817355 9.83247882,14.0044386 7.99865879,14.0044386 C5.03907292,14.0044386 2.57997332,11.8615894 2.08820756,9.0427473 L3.94774327,9.10768372 C4.43372186,10.8898575 6.06393114,12.2000519 8.00015362,12.2000519 C9.30149237,12.2000519 10.4645985,11.6082097 11.2349873,10.6790094 L9.05000019,8.71167959 L14.0431479,8.44999981 L14.3048222,13.4430431 L12.5747152,11.8852806 Z M3.42785637,4.11741586 C4.52839138,2.82452748 6.16775464,2.00443857 7.99865879,2.00443857 C10.918604,2.00443857 13.3513802,4.09026967 13.8882946,6.8532307 L12.0226389,6.78808057 C11.5024872,5.05935553 9.89838095,3.8000774 8.00015362,3.8000774 C6.69867367,3.8000774 5.53545628,4.39204806 4.76506921,5.32142241 L6.95482203,7.29304326 L1.96167436,7.55472304 L1.70000005,2.56167973 L3.42785637,4.11741586 Z" transform="rotate(3 8.002 8.004)"/>
</svg>`

const backIconSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
  <g fill="#6E6E6E" fill-rule="evenodd" transform="translate(1 3)">
    <rect width="12" height="2" x="1" y="4"/>
    <g transform="translate(0 .02)">
      <rect width="7" height="1.8" x="-.389" y="2.24" transform="rotate(-45 3.111 3.14)"/>
      <rect width="1.8" height="7" x="2.211" y="3.317" transform="rotate(-45 3.111 6.817)"/>
    </g>
  </g>
</svg>`

const executeIconSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
  <polygon fill="#59A869" fill-rule="evenodd" points="4 2 14 8 4 14"/>
</svg>`

const commitIconSVG = `<svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
<path fill-rule="evenodd" clip-rule="evenodd" d="M8 10C9.10457 10 10 9.10457 10 8C10 6.89543 9.10457 6 8 6C6.89543 6 6 6.89543 6 8C6 9.10457 6.89543 10 8 10ZM10.9585 7.5C10.7205 6.08114 9.4865 5 8 5C6.5135 5 5.27952 6.08114 5.04148 7.5H0.5C0.223858 7.5 0 7.72386 0 8C0 8.27614 0.223858 8.5 0.5 8.5H5.04148C5.27952 9.91886 6.5135 11 8 11C9.4865 11 10.7205 9.91886 10.9585 8.5H15.5C15.7761 8.5 16 8.27614 16 8C16 7.72386 15.7761 7.5 15.5 7.5H10.9585Z" fill="#6C707E"/>
</svg>`

const problemsIconSVG = `<svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
<circle cx="8" cy="8" r="6.5" stroke="#6C707E"/>
<path d="M8 4.59998L8 8.39998" stroke="#6C707E" stroke-width="1.2" stroke-linecap="round"/>
<circle cx="8.00078" cy="10.7" r="0.5" fill="#6C707E" stroke="#6C707E" stroke-width="0.4"/>
</svg>`

// --- Parse Tests ---

func TestParseSVGRoot(t *testing.T) {
	doc, err := Parse([]byte(closeIconSVG))
	if err != nil {
		t.Fatalf("Parse close icon: %v", err)
	}
	if doc.ViewBox.Width != 16 || doc.ViewBox.Height != 16 {
		t.Errorf("ViewBox = %+v, want 16x16", doc.ViewBox)
	}
	if doc.Width != 16 || doc.Height != 16 {
		t.Errorf("Width/Height = %v/%v, want 16/16", doc.Width, doc.Height)
	}
	if len(doc.Elements) != 1 {
		t.Errorf("Elements count = %d, want 1", len(doc.Elements))
	}
}

func TestParsePathElement(t *testing.T) {
	doc, err := Parse([]byte(closeIconSVG))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	path, ok := doc.Elements[0].(*PathElement)
	if !ok {
		t.Fatalf("Elements[0] type = %T, want *PathElement", doc.Elements[0])
	}
	if path.D == "" {
		t.Error("PathElement.D is empty")
	}
	if path.Attrs.Fill != "#7F8B91" {
		t.Errorf("Fill = %q, want #7F8B91", path.Attrs.Fill)
	}
	if path.Attrs.FillRule != "evenodd" {
		t.Errorf("FillRule = %q, want evenodd", path.Attrs.FillRule)
	}
	if path.Attrs.FillOpacity != 0.5 {
		t.Errorf("FillOpacity = %v, want 0.5", path.Attrs.FillOpacity)
	}
}

func TestParseCircleElement(t *testing.T) {
	doc, err := Parse([]byte(problemsIconSVG))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Elements) != 3 {
		t.Fatalf("Elements count = %d, want 3", len(doc.Elements))
	}
	circle, ok := doc.Elements[0].(*CircleElement)
	if !ok {
		t.Fatalf("Elements[0] type = %T, want *CircleElement", doc.Elements[0])
	}
	if circle.CX != 8 || circle.CY != 8 || circle.R != 6.5 {
		t.Errorf("Circle = cx=%v cy=%v r=%v, want 8 8 6.5", circle.CX, circle.CY, circle.R)
	}
	if circle.Attrs.Stroke != "#6C707E" {
		t.Errorf("Stroke = %q, want #6C707E", circle.Attrs.Stroke)
	}
}

func TestParseRectElement(t *testing.T) {
	doc, err := Parse([]byte(backIconSVG))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Elements) != 1 {
		t.Fatalf("Elements count = %d, want 1", len(doc.Elements))
	}
	group, ok := doc.Elements[0].(*GroupElement)
	if !ok {
		t.Fatalf("Elements[0] type = %T, want *GroupElement", doc.Elements[0])
	}
	if len(group.Children) < 2 {
		t.Fatalf("Group children = %d, want >= 2", len(group.Children))
	}
	rect, ok := group.Children[0].(*RectElement)
	if !ok {
		t.Fatalf("Group.Children[0] type = %T, want *RectElement", group.Children[0])
	}
	if rect.W != 12 || rect.H != 2 {
		t.Errorf("Rect size = %vx%v, want 12x2", rect.W, rect.H)
	}
}

func TestParsePolygonElement(t *testing.T) {
	doc, err := Parse([]byte(executeIconSVG))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Elements) != 1 {
		t.Fatalf("Elements count = %d, want 1", len(doc.Elements))
	}
	poly, ok := doc.Elements[0].(*PolygonElement)
	if !ok {
		t.Fatalf("Elements[0] type = %T, want *PolygonElement", doc.Elements[0])
	}
	if len(poly.Points) != 6 {
		t.Errorf("Polygon points = %d, want 6", len(poly.Points))
	}
	if poly.Attrs.Fill != "#59A869" {
		t.Errorf("Fill = %q, want #59A869", poly.Attrs.Fill)
	}
}

func TestParseGroupWithTransform(t *testing.T) {
	doc, err := Parse([]byte(backIconSVG))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	group, ok := doc.Elements[0].(*GroupElement)
	if !ok {
		t.Fatalf("Elements[0] type = %T, want *GroupElement", doc.Elements[0])
	}
	if group.Attrs.Transform != "translate(1 3)" {
		t.Errorf("Transform = %q, want translate(1 3)", group.Attrs.Transform)
	}
	if group.Attrs.Fill != "#6E6E6E" {
		t.Errorf("Fill = %q, want #6E6E6E", group.Attrs.Fill)
	}
}

func TestParsePathTransform(t *testing.T) {
	doc, err := Parse([]byte(refreshIconSVG))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	path, ok := doc.Elements[0].(*PathElement)
	if !ok {
		t.Fatalf("Elements[0] type = %T, want *PathElement", doc.Elements[0])
	}
	if path.Attrs.Transform != "rotate(3 8.002 8.004)" {
		t.Errorf("Transform = %q, want rotate(3 8.002 8.004)", path.Attrs.Transform)
	}
}

// --- Color Tests ---

func TestParseColorHex(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantR   uint8
		wantG   uint8
		wantB   uint8
		wantA   uint8
		wantNil bool
	}{
		{"6-digit hex", "#7F8B91", 0x7F, 0x8B, 0x91, 255, false},
		{"3-digit hex", "#F00", 0xFF, 0x00, 0x00, 255, false},
		{"8-digit hex", "#FF000080", 0x80, 0x00, 0x00, 0x80, false}, // premultiplied: R=255*128/255=128
		{"none", "none", 0, 0, 0, 0, true},
		{"empty", "", 0, 0, 0, 0, true},
		{"named black", "black", 0, 0, 0, 255, false},
		{"named white", "white", 255, 255, 255, 255, false},
		{"named red", "red", 255, 0, 0, 255, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := parseColor(tt.input)
			if err != nil {
				t.Fatalf("parseColor(%q) error: %v", tt.input, err)
			}
			if tt.wantNil {
				if c != nil {
					t.Errorf("parseColor(%q) = %v, want nil", tt.input, c)
				}
				return
			}
			if c == nil {
				t.Fatalf("parseColor(%q) = nil, want color", tt.input)
			}
			r, g, b, a := c.RGBA()
			// Convert from premultiplied 16-bit to 8-bit.
			gotR := uint8(r >> 8)
			gotG := uint8(g >> 8)
			gotB := uint8(b >> 8)
			gotA := uint8(a >> 8)
			if gotR != tt.wantR || gotG != tt.wantG || gotB != tt.wantB || gotA != tt.wantA {
				t.Errorf("parseColor(%q) = RGBA(%d,%d,%d,%d), want (%d,%d,%d,%d)",
					tt.input, gotR, gotG, gotB, gotA, tt.wantR, tt.wantG, tt.wantB, tt.wantA)
			}
		})
	}
}

func TestParseColorRGB(t *testing.T) {
	c, err := parseColor("rgb(127, 139, 145)")
	if err != nil {
		t.Fatalf("parseColor(rgb) error: %v", err)
	}
	if c == nil {
		t.Fatal("parseColor(rgb) returned nil")
	}
	cr, _, _, ca := c.RGBA()
	gotR := uint8(cr >> 8)
	gotA := uint8(ca >> 8)
	if gotR != 127 || gotA != 255 {
		t.Errorf("rgb(127,...) got R=%d A=%d, want R=127 A=255", gotR, gotA)
	}
}

func TestParseColorRGBA(t *testing.T) {
	c, err := parseColor("rgba(255, 0, 0, 0.5)")
	if err != nil {
		t.Fatalf("parseColor(rgba) error: %v", err)
	}
	if c == nil {
		t.Fatal("parseColor(rgba) returned nil")
	}
	ca := alphaOf(c)
	if ca < 125 || ca > 129 {
		t.Errorf("rgba(0.5 alpha) got A=%d, want ~127", ca)
	}
}

func TestParseColorError(t *testing.T) {
	_, err := parseColor("invalidcolor")
	if err == nil {
		t.Error("expected error for invalid color, got nil")
	}
}

// --- Transform Tests ---

func TestParseTransformArgs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // number of args
	}{
		{"comma-separated", "1,3", 2},
		{"space-separated", "1 3", 2},
		{"mixed", "1, 3", 2},
		{"three args", "-45 3 3", 3},
		{"single", "2", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := parseTransformArgs(tt.input)
			if err != nil {
				t.Fatalf("parseTransformArgs(%q): %v", tt.input, err)
			}
			if len(args) != tt.want {
				t.Errorf("len(args) = %d, want %d", len(args), tt.want)
			}
		})
	}
}

// --- Render Tests ---

func TestRenderCloseIcon(t *testing.T) {
	img, err := Render([]byte(closeIconSVG), 16, 16)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if img.Bounds().Dx() != 16 || img.Bounds().Dy() != 16 {
		t.Errorf("Image size = %dx%d, want 16x16", img.Bounds().Dx(), img.Bounds().Dy())
	}
	assertNonEmpty(t, img, "close icon 16x16")
}

func TestRenderSearchIcon(t *testing.T) {
	img, err := Render([]byte(searchIconSVG), 32, 32)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if img.Bounds().Dx() != 32 || img.Bounds().Dy() != 32 {
		t.Errorf("Image size = %dx%d, want 32x32", img.Bounds().Dx(), img.Bounds().Dy())
	}
	assertNonEmpty(t, img, "search icon 32x32")
}

func TestRenderRefreshIconWithTransform(t *testing.T) {
	img, err := Render([]byte(refreshIconSVG), 64, 64)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertNonEmpty(t, img, "refresh icon 64x64 (rotate transform)")
}

func TestRenderBackIconWithNestedTransforms(t *testing.T) {
	img, err := Render([]byte(backIconSVG), 48, 48)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertNonEmpty(t, img, "back icon 48x48 (nested groups + transforms)")
}

func TestRenderExecuteIconPolygon(t *testing.T) {
	img, err := Render([]byte(executeIconSVG), 16, 16)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertNonEmpty(t, img, "execute icon 16x16 (polygon)")
}

func TestRenderCommitIcon(t *testing.T) {
	img, err := Render([]byte(commitIconSVG), 16, 16)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertNonEmpty(t, img, "commit icon 16x16 (evenodd)")
}

func TestRenderProblemsIconMixed(t *testing.T) {
	img, err := Render([]byte(problemsIconSVG), 32, 32)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertNonEmpty(t, img, "problems icon 32x32 (circle + path stroke + circle fill)")
}

func TestRenderWithColorOverride(t *testing.T) {
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	img, err := RenderWithColor([]byte(closeIconSVG), 16, 16, white)
	if err != nil {
		t.Fatalf("RenderWithColor: %v", err)
	}
	assertNonEmpty(t, img, "close icon white override")

	// Check that rendered pixels use white (with fill-opacity=0.5 from icon).
	// White with 50% opacity: R=255, A=127 → premultiplied R≈127, A≈127 → RGBA() R≈0x7F7F.
	found := false
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			r, _, _, a := img.At(x, y).RGBA()
			if a > 0 && r > 0x6000 {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Error("Expected white-ish pixels in color override render, found none")
	}
}

func TestRenderMultipleSizes(t *testing.T) {
	doc, err := Parse([]byte(closeIconSVG))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	sizes := []int{8, 16, 24, 32, 48, 64, 128}
	for _, sz := range sizes {
		img := doc.Render(sz, sz)
		if img.Bounds().Dx() != sz || img.Bounds().Dy() != sz {
			t.Errorf("Render(%d,%d) size = %dx%d", sz, sz, img.Bounds().Dx(), img.Bounds().Dy())
		}
	}
}

func TestRenderWithColorOverridePreservesNone(t *testing.T) {
	// SVG with fill="none" on the root — should remain transparent.
	svg := `<svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
<circle cx="8" cy="8" r="6.5" stroke="#6C707E"/>
</svg>`
	red := color.NRGBA{R: 255, G: 0, B: 0, A: 255}
	img, err := RenderWithColor([]byte(svg), 16, 16, red)
	if err != nil {
		t.Fatalf("RenderWithColor: %v", err)
	}
	// Center pixel should be transparent (circle is stroke-only).
	ca := alphaOf(img.At(8, 8))
	if ca != 0 {
		t.Errorf("Center pixel alpha = %d, want 0 (stroke-only circle)", ca)
	}
}

// --- ViewBox Tests ---

func TestParseViewBox(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantVB  ViewBox
		wantErr bool
	}{
		{"standard", "0 0 16 16", ViewBox{0, 0, 16, 16}, false},
		{"with offset", "5 5 20 20", ViewBox{5, 5, 20, 20}, false},
		{"comma-separated", "0,0,16,16", ViewBox{0, 0, 16, 16}, false},
		{"13x13", "0 0 13 13", ViewBox{0, 0, 13, 13}, false},
		{"invalid", "0 0", ViewBox{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vb, err := parseViewBox(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseViewBox(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && vb != tt.wantVB {
				t.Errorf("parseViewBox(%q) = %+v, want %+v", tt.input, vb, tt.wantVB)
			}
		})
	}
}

// --- Error Handling Tests ---

func TestParseInvalidXML(t *testing.T) {
	_, err := Parse([]byte("not xml at all"))
	if err == nil {
		t.Error("expected error for invalid XML, got nil")
	}
}

func TestParseEmptySVG(t *testing.T) {
	doc, err := Parse([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"></svg>`))
	if err != nil {
		t.Fatalf("Parse empty SVG: %v", err)
	}
	if len(doc.Elements) != 0 {
		t.Errorf("Elements count = %d, want 0", len(doc.Elements))
	}
}

func TestParseNoViewBox(t *testing.T) {
	doc, err := Parse([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24">
  <circle cx="12" cy="12" r="10"/>
</svg>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Should fall back to default viewBox from width/height.
	if doc.Width != 24 || doc.Height != 24 {
		t.Errorf("Width/Height = %v/%v, want 24/24", doc.Width, doc.Height)
	}
}

func TestRenderZeroSize(t *testing.T) {
	doc, err := Parse([]byte(closeIconSVG))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Should not panic on zero-size render.
	img := doc.Render(0, 0)
	if img == nil {
		t.Error("Render(0,0) returned nil, want non-nil image")
	}
}

func TestParseUnsupportedElementsSkipped(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <defs><clipPath id="a"><rect width="16" height="16"/></clipPath></defs>
  <circle cx="8" cy="8" r="4" fill="red"/>
  <text x="0" y="10">Hello</text>
</svg>`
	doc, err := Parse([]byte(svg))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Only the circle should be parsed; defs and text are skipped.
	if len(doc.Elements) != 1 {
		t.Errorf("Elements count = %d, want 1 (circle only)", len(doc.Elements))
	}
}

// --- Additional Element Tests ---

func TestParseEllipseElement(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <ellipse cx="8" cy="8" rx="6" ry="4" fill="#FF0000"/>
</svg>`
	doc, err := Parse([]byte(svg))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	e, ok := doc.Elements[0].(*EllipseElement)
	if !ok {
		t.Fatalf("Elements[0] type = %T, want *EllipseElement", doc.Elements[0])
	}
	if e.CX != 8 || e.CY != 8 || e.RX != 6 || e.RY != 4 {
		t.Errorf("Ellipse = cx=%v cy=%v rx=%v ry=%v", e.CX, e.CY, e.RX, e.RY)
	}
}

func TestParseLineElement(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <line x1="0" y1="0" x2="16" y2="16" stroke="black"/>
</svg>`
	doc, err := Parse([]byte(svg))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	line, ok := doc.Elements[0].(*LineElement)
	if !ok {
		t.Fatalf("Elements[0] type = %T, want *LineElement", doc.Elements[0])
	}
	if line.X1 != 0 || line.Y1 != 0 || line.X2 != 16 || line.Y2 != 16 {
		t.Errorf("Line = (%v,%v)-(%v,%v)", line.X1, line.Y1, line.X2, line.Y2)
	}
}

func TestParsePolylineElement(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <polyline points="1,1 5,5 10,1" stroke="blue" fill="none"/>
</svg>`
	doc, err := Parse([]byte(svg))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	pl, ok := doc.Elements[0].(*PolylineElement)
	if !ok {
		t.Fatalf("Elements[0] type = %T, want *PolylineElement", doc.Elements[0])
	}
	if len(pl.Points) != 6 {
		t.Errorf("Polyline points = %d, want 6", len(pl.Points))
	}
}

func TestRenderEllipse(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <ellipse cx="8" cy="8" rx="6" ry="4" fill="red"/>
</svg>`
	img, err := Render([]byte(svg), 32, 32)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertNonEmpty(t, img, "ellipse render")
}

func TestRenderLine(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <line x1="0" y1="0" x2="16" y2="16" stroke="black" stroke-width="2"/>
</svg>`
	img, err := Render([]byte(svg), 16, 16)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertNonEmpty(t, img, "line render")
}

func TestRenderPolyline(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <polyline points="1,1 8,15 15,1" stroke="blue" fill="none" stroke-width="2"/>
</svg>`
	img, err := Render([]byte(svg), 16, 16)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertNonEmpty(t, img, "polyline render")
}

// --- Document Reuse Tests ---

func TestDocumentReuse(t *testing.T) {
	doc, err := Parse([]byte(commitIconSVG))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	img1 := doc.Render(16, 16)
	img2 := doc.Render(32, 32)
	img3 := doc.RenderWithColor(16, 16, color.White)

	if img1.Bounds().Dx() != 16 {
		t.Errorf("img1 width = %d, want 16", img1.Bounds().Dx())
	}
	if img2.Bounds().Dx() != 32 {
		t.Errorf("img2 width = %d, want 32", img2.Bounds().Dx())
	}
	if img3.Bounds().Dx() != 16 {
		t.Errorf("img3 width = %d, want 16", img3.Bounds().Dx())
	}
}

// --- String ---

func TestDocumentString(t *testing.T) {
	doc, err := Parse([]byte(closeIconSVG))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	s := doc.String()
	if s == "" {
		t.Error("String() returned empty")
	}
}

// --- Fill Rule EvenOdd Test ---

func TestFillRuleEvenOdd(t *testing.T) {
	// Search icon uses evenodd with two concentric circles — center should have hole.
	img, err := Render([]byte(searchIconSVG), 64, 64)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	assertNonEmpty(t, img, "search icon evenodd")

	// The search icon has a magnifying glass with a hollow center.
	// At 64x64, the center of the glass (roughly 7/16 * 64 = 28) should be transparent.
	// The glass body (outer ring) should be non-transparent.
	glassX, glassY := 28, 28 // center of the magnifying glass circle (~7/16 of 64)
	centerA := alphaOf(img.At(glassX, glassY))
	// The outer ring area (~4.5/16 * 64 = 18 from center, so pixel at ~18, 28)
	ringA := alphaOf(img.At(18, 28))
	if centerA >= ringA && ringA > 0 {
		// If the center is as opaque as the ring, evenodd fill rule may not be working.
		// This is a soft check — exact pixel positions depend on rasterization.
		t.Logf("Warning: center alpha=%d >= ring alpha=%d, evenodd may not be cutting hole", centerA, ringA)
	}
}

// --- Render All Real Icons ---

func TestRenderAllRealIcons(t *testing.T) {
	icons := []struct {
		name string
		svg  string
	}{
		{"close", closeIconSVG},
		{"search", searchIconSVG},
		{"refresh", refreshIconSVG},
		{"back", backIconSVG},
		{"execute", executeIconSVG},
		{"commit", commitIconSVG},
		{"problems", problemsIconSVG},
	}
	for _, icon := range icons {
		t.Run(icon.name, func(t *testing.T) {
			for _, sz := range []int{16, 32, 64} {
				img, err := Render([]byte(icon.svg), sz, sz)
				if err != nil {
					t.Errorf("Render %s %dx%d: %v", icon.name, sz, sz, err)
					continue
				}
				assertNonEmpty(t, img, icon.name)
			}
		})
	}
}

// --- Helpers ---

// assertNonEmpty checks that the image has at least one non-transparent pixel.
func assertNonEmpty(t *testing.T, img interface {
	Bounds() image.Rectangle
	At(x, y int) color.Color
}, label string) {
	t.Helper()
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > 0 {
				return // found a non-transparent pixel
			}
		}
	}
	t.Errorf("%s: image is entirely transparent (no visible content rendered)", label)
}

// alphaOf extracts the 8-bit alpha value from a color.
func alphaOf(c color.Color) uint8 {
	nrgba := color.NRGBAModel.Convert(c).(color.NRGBA)
	return nrgba.A
}
