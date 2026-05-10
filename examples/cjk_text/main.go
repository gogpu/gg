// CJK text rendering validation example (ADR-027).
// Renders CJK text at various sizes to verify:
// - No blurry text (exact-size rasterization, no bucket quantization)
// - Correct hinting (HintingVertical for CJK vs HintingFull for Latin)
// - MSDF quality for display text (128px reference)
//
// Usage: go run ./examples/cjk_text/
// Output: tmp/cjk_text_validation.png

package main

import (
	"fmt"
	"log"

	"github.com/gogpu/gg"
)

func main() {
	const W, H = 800, 600
	dc := gg.NewContext(W, H)

	// White background.
	dc.SetRGB(1, 1, 1)
	dc.DrawRectangle(0, 0, W, H)
	_ = dc.Fill()

	// Use system font (or embedded font if available).
	// On Windows: Microsoft YaHei, SimHei, or NSimSun
	// On macOS: PingFang SC, Hiragino Sans
	// On Linux: Noto Sans CJK, WenQuanYi
	fonts := []string{
		"C:/Windows/Fonts/msyh.ttc",       // Microsoft YaHei (Windows)
		"C:/Windows/Fonts/simsun.ttc",      // SimSun (Windows)
		"/System/Library/Fonts/PingFang.ttc", // PingFang (macOS)
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc", // Noto (Linux)
	}

	var fontLoaded bool
	for _, fontPath := range fonts {
		if err := dc.LoadFontFace(fontPath, 14); err == nil {
			fontLoaded = true
			log.Printf("Font loaded: %s", fontPath)
			break
		}
	}

	if !fontLoaded {
		log.Println("No CJK font found — using default font. CJK characters may show as boxes.")
		dc.LoadFontFace("", 14)
	}

	dc.SetRGB(0, 0, 0)
	y := 30.0

	// Section 1: CJK at various body text sizes (Tier 6 bitmap, exact size)
	title(dc, "CJK Body Text — Tier 6 Bitmap (exact size, no bucket quantization)", 10, y)
	y += 25

	bodySizes := []float64{12, 14, 16, 18, 20, 24}
	for _, size := range bodySizes {
		dc.LoadFontFace(currentFont(dc), size)
		label := fmt.Sprintf("%gpx: 中文测试 日本語テスト 한국어 — The quick brown fox", size)
		dc.DrawString(label, 20, y)
		y += size + 8
	}

	y += 15

	// Section 2: CJK at display sizes (Tier 4 MSDF, 128px reference)
	title(dc, "CJK Display Text — Tier 4 MSDF (128px reference for CJK)", 10, y)
	y += 25

	displaySizes := []float64{36, 48, 64, 72}
	for _, size := range displaySizes {
		dc.LoadFontFace(currentFont(dc), size)
		dc.DrawString("中文大标题", 20, y)
		y += size + 10
	}

	// Section 3: Mixed Latin + CJK
	y = 30
	title(dc, "Mixed Script", 500, y)
	y += 25
	dc.LoadFontFace(currentFont(dc), 16)
	dc.DrawString("Hello 世界!", 500, y)
	y += 24
	dc.DrawString("Go言語 is 素晴らしい", 500, y)
	y += 24
	dc.DrawString("1234 가나다라", 500, y)

	// Save.
	if err := dc.SavePNG("tmp/cjk_text_validation.png"); err != nil {
		log.Fatalf("SavePNG: %v", err)
	}
	log.Println("Saved: tmp/cjk_text_validation.png")
}

func title(dc *gg.Context, s string, x, y float64) {
	dc.SetRGB(0.2, 0.2, 0.8)
	dc.LoadFontFace(currentFont(dc), 13)
	dc.DrawString(s, x, y)
	dc.SetRGB(0, 0, 0)
}

func currentFont(dc *gg.Context) string {
	// Return empty to keep current font.
	return ""
}
