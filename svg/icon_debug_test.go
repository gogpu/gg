package svg

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ToolTerminal icon from JetBrains New UI icon set.
// Two stroke-only paths at default stroke-width=1, fill="none".
const toolTerminalSVG = `<svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
<path d="M3 5L7 8L3 11" stroke="#6C707E" stroke-linecap="round" stroke-linejoin="round"/>
<path d="M9 11H13" stroke="#6C707E" stroke-linecap="round"/>
</svg>`

func TestIconDebug(t *testing.T) {
	sizes := []int{16, 32, 48}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size, size), func(t *testing.T) {
			img, err := Render([]byte(toolTerminalSVG), size, size)
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}

			// Save PNG for visual inspection.
			savePNG(t, img, fmt.Sprintf("icon_debug_%d.png", size))

			// Analyze the rendered image.
			analysis := analyzeImage(img)
			printImageVisualization(t, img, size)
			printAnalysis(t, analysis, size)
			checkStrokeQuality(t, analysis, size)
		})
	}
}

// TestIconDebugCompareFills tests the same icon at native size with fill="none"
// to verify that stroke-only rendering works correctly (no accidental fills).
func TestIconDebugCompareFills(t *testing.T) {
	// Stroke-only (original).
	strokeOnly, err := Render([]byte(toolTerminalSVG), 16, 16)
	if err != nil {
		t.Fatalf("Render stroke-only: %v", err)
	}

	// Same paths but with solid fill added — should produce MORE opaque pixels.
	filledSVG := `<svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
<path d="M3 5L7 8L3 11" fill="#6C707E" stroke="#6C707E" stroke-linecap="round" stroke-linejoin="round"/>
<path d="M9 11H13" fill="#6C707E" stroke="#6C707E" stroke-linecap="round"/>
</svg>`
	filled, err := Render([]byte(filledSVG), 16, 16)
	if err != nil {
		t.Fatalf("Render filled: %v", err)
	}

	savePNG(t, strokeOnly, "icon_debug_stroke_only.png")
	savePNG(t, filled, "icon_debug_filled.png")

	strokeAnalysis := analyzeImage(strokeOnly)
	filledAnalysis := analyzeImage(filled)

	t.Logf("Stroke-only: %d opaque, %d partial, %d transparent",
		strokeAnalysis.fullyOpaque, strokeAnalysis.partialCoverage, strokeAnalysis.fullyTransparent)
	t.Logf("Filled:      %d opaque, %d partial, %d transparent",
		filledAnalysis.fullyOpaque, filledAnalysis.partialCoverage, filledAnalysis.fullyTransparent)

	if filledAnalysis.fullyOpaque <= strokeAnalysis.fullyOpaque {
		t.Logf("WARNING: filled icon has same or fewer opaque pixels — fill may not be working")
	}
}

// imageAnalysis holds per-pixel statistics.
type imageAnalysis struct {
	fullyOpaque      int
	partialCoverage  int
	fullyTransparent int
	totalPixels      int

	// Edge pixel analysis.
	edgeCoverages []float64 // alpha values of partially-covered pixels (0-1)
	isolatedDots  int       // pixels with coverage surrounded by transparent neighbors

	// Stroke width analysis per row.
	maxRunPerRow []int // longest horizontal run of non-transparent pixels per row
}

func analyzeImage(img *image.RGBA) *imageAnalysis {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	a := &imageAnalysis{
		totalPixels:  w * h,
		maxRunPerRow: make([]int, h),
	}

	for y := range h {
		run := 0
		maxRun := 0
		for x := range w {
			_, _, _, alpha := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			alpha8 := alpha >> 8

			switch alpha8 {
			case 255:
				a.fullyOpaque++
				run++
			case 0:
				a.fullyTransparent++
				if run > maxRun {
					maxRun = run
				}
				run = 0
			default:
				a.partialCoverage++
				a.edgeCoverages = append(a.edgeCoverages, float64(alpha8)/255.0)
				run++
			}
		}
		if run > maxRun {
			maxRun = run
		}
		a.maxRunPerRow[y] = maxRun
	}

	// Count isolated dots: a non-transparent pixel with all 4 neighbors transparent.
	for y := range h {
		for x := range w {
			_, _, _, alpha := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			if alpha>>8 == 0 {
				continue
			}
			isolated := true
			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || nx >= w || ny < 0 || ny >= h {
					continue // edge pixels count as transparent neighbors
				}
				_, _, _, na := img.At(nx+bounds.Min.X, ny+bounds.Min.Y).RGBA()
				if na>>8 > 0 {
					isolated = false
					break
				}
			}
			if isolated {
				a.isolatedDots++
			}
		}
	}

	return a
}

func printImageVisualization(t *testing.T, img *image.RGBA, size int) {
	t.Helper()
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	var sb strings.Builder
	fmt.Fprintf(&sb, "\n=== %dx%d Pixel Map (alpha hex) ===\n", size, size)

	// Header: column indices.
	sb.WriteString("    ")
	for x := range w {
		fmt.Fprintf(&sb, " %2d", x)
	}
	sb.WriteString("\n")

	for y := range h {
		fmt.Fprintf(&sb, " %2d:", y)
		for x := range w {
			_, _, _, alpha := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			alpha8 := alpha >> 8
			switch alpha8 {
			case 0:
				sb.WriteString("  .")
			case 255:
				sb.WriteString(" FF")
			default:
				fmt.Fprintf(&sb, " %02X", alpha8)
			}
		}
		sb.WriteString("\n")
	}

	// Also print a visual representation using block characters.
	fmt.Fprintf(&sb, "\n=== %dx%d Visual (coverage blocks) ===\n", size, size)
	sb.WriteString("    Coverage: . = 0%%, ░ = 1-25%%, ▒ = 26-50%%, ▓ = 51-75%%, █ = 76-100%%\n")

	sb.WriteString("    ")
	for x := range w {
		fmt.Fprintf(&sb, "%d", x%10)
	}
	sb.WriteString("\n")

	for y := range h {
		fmt.Fprintf(&sb, " %2d:", y)
		for x := range w {
			_, _, _, alpha := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			coverage := float64(alpha>>8) / 255.0
			switch {
			case coverage == 0:
				sb.WriteString(".")
			case coverage <= 0.25:
				sb.WriteString("░")
			case coverage <= 0.50:
				sb.WriteString("▒")
			case coverage <= 0.75:
				sb.WriteString("▓")
			default:
				sb.WriteString("█")
			}
		}
		sb.WriteString("\n")
	}

	// Print non-zero RGBA pixel dump for the first few non-transparent pixels.
	fmt.Fprintf(&sb, "\n=== %dx%d Non-transparent pixels (first 40) ===\n", size, size)
	count := 0
	for y := 0; y < h && count < 40; y++ {
		for x := 0; x < w && count < 40; x++ {
			r, g, b, a := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			if a == 0 {
				continue
			}
			// Show premultiplied RGBA values (8-bit).
			fmt.Fprintf(&sb, "  (%2d,%2d): R=%3d G=%3d B=%3d A=%3d (coverage=%.1f%%)\n",
				x, y, r>>8, g>>8, b>>8, a>>8, float64(a>>8)/255.0*100)
			count++
		}
	}

	t.Log(sb.String())
}

func printAnalysis(t *testing.T, a *imageAnalysis, size int) {
	t.Helper()
	t.Logf("=== %dx%d Analysis ===", size, size)
	t.Logf("  Total pixels:        %d", a.totalPixels)
	t.Logf("  Fully opaque (FF):   %d (%.1f%%)", a.fullyOpaque, pct(a.fullyOpaque, a.totalPixels))
	t.Logf("  Partial coverage:    %d (%.1f%%)", a.partialCoverage, pct(a.partialCoverage, a.totalPixels))
	t.Logf("  Fully transparent:   %d (%.1f%%)", a.fullyTransparent, pct(a.fullyTransparent, a.totalPixels))
	t.Logf("  Isolated dots:       %d", a.isolatedDots)

	if len(a.edgeCoverages) > 0 {
		avg, lo, hi := avgMinMax(a.edgeCoverages)
		t.Logf("  Edge coverage avg:   %.1f%%", avg*100)
		t.Logf("  Edge coverage range: %.1f%% - %.1f%%", lo*100, hi*100)

		// Histogram of edge coverages.
		buckets := [5]int{} // 0-20%, 20-40%, 40-60%, 60-80%, 80-100%
		for _, c := range a.edgeCoverages {
			idx := int(c * 5)
			if idx >= 5 {
				idx = 4
			}
			buckets[idx]++
		}
		t.Logf("  Edge coverage distribution:")
		labels := []string{"  1-20%%", " 21-40%%", " 41-60%%", " 61-80%%", "81-99%%"}
		for i, label := range labels {
			bar := strings.Repeat("█", buckets[i])
			t.Logf("    %s: %3d %s", label, buckets[i], bar)
		}
	}

	// Stroke width estimate from max run per row.
	t.Logf("  Max non-transparent run per row (stroke width indicator):")
	for y, run := range a.maxRunPerRow {
		if run > 0 {
			t.Logf("    row %2d: %d px", y, run)
		}
	}
}

func checkStrokeQuality(t *testing.T, a *imageAnalysis, size int) {
	t.Helper()

	// Check 1: No isolated dots (stippling artifact).
	if a.isolatedDots > 0 {
		t.Errorf("[%dx%d] STIPPLING: found %d isolated dot(s) — pixels with no non-transparent neighbors",
			size, size, a.isolatedDots)
	}

	// Check 2: Should have SOME non-transparent pixels (rendering worked).
	nonTransparent := a.fullyOpaque + a.partialCoverage
	if nonTransparent == 0 {
		t.Errorf("[%dx%d] EMPTY: no visible pixels rendered", size, size)
		return
	}

	// Check 3: Edge pixel average coverage should be reasonable (25-75% for good AA).
	if len(a.edgeCoverages) > 0 {
		avg, _, _ := avgMinMax(a.edgeCoverages)
		if avg < 0.05 {
			t.Errorf("[%dx%d] THIN AA: average edge coverage %.1f%% is very low (< 5%%) — strokes may be sub-pixel",
				size, size, avg*100)
		}
	}

	// Check 4: For 16x16, strokes should not be wider than 3px (1px stroke + 1px AA on each side).
	if size == 16 {
		for y, run := range a.maxRunPerRow {
			if run > 5 {
				t.Logf("[%dx%d] WARNING: row %d has %d px wide run — may indicate stroke blowup",
					size, size, y, run)
			}
		}
	}

	// Check 5: Ratio of partial to fully opaque pixels.
	// For thin strokes (1px), most pixels should be partially covered (AA edges).
	// For thicker strokes, more fully opaque pixels expected.
	if a.fullyOpaque > 0 && a.partialCoverage > 0 {
		ratio := float64(a.partialCoverage) / float64(a.fullyOpaque)
		t.Logf("[%dx%d] Partial/Opaque ratio: %.2f (>1 = thin strokes, <1 = thick/solid)",
			size, size, ratio)
	} else if a.fullyOpaque == 0 && a.partialCoverage > 0 {
		t.Logf("[%dx%d] ALL edge pixels are partial coverage — very thin strokes, no fully opaque cores",
			size, size)
	}
}

func savePNG(t *testing.T, img *image.RGBA, name string) {
	t.Helper()
	dir := filepath.Join(".", "..", "tmp")
	outPath := filepath.Join(dir, name)

	f, err := os.Create(outPath)
	if err != nil {
		t.Logf("Cannot save %s: %v", outPath, err)
		return
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		t.Logf("Cannot encode PNG %s: %v", outPath, err)
		return
	}
	t.Logf("Saved: %s", outPath)
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

func avgMinMax(vals []float64) (avg, lo, hi float64) {
	if len(vals) == 0 {
		return 0, 0, 0
	}
	lo = math.MaxFloat64
	hi = -math.MaxFloat64
	sum := 0.0
	for _, v := range vals {
		sum += v
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return sum / float64(len(vals)), lo, hi
}
