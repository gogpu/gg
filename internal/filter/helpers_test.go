package filter

import "github.com/gogpu/gg"

// Test helper functions shared across filter tests.

// createTestPixmap creates a pixmap filled with the given color.
func createTestPixmap(w, h int, color gg.RGBA) *gg.Pixmap {
	p := gg.NewPixmap(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p.SetPixel(x, y, color)
		}
	}
	return p
}

// colorApproxEqual compares two colors with tolerance.
func colorApproxEqual(a, b gg.RGBA, tolerance float64) bool {
	return absf(a.R-b.R) < tolerance &&
		absf(a.G-b.G) < tolerance &&
		absf(a.B-b.B) < tolerance &&
		absf(a.A-b.A) < tolerance
}

// absf returns the absolute value of a float64.
func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// absf32 returns the absolute value of a float32.
func absf32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// formatFloat formats a float for benchmark names.
func formatFloat(f float64) string {
	if f == float64(int(f)) {
		return formatInt(int(f))
	}
	intPart := int(f)
	fracPart := int((f - float64(intPart)) * 100)
	if fracPart < 0 {
		fracPart = -fracPart
	}
	return formatInt(intPart) + "." + formatInt(fracPart)
}

// formatInt formats an integer without using fmt.
func formatInt(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
