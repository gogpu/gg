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

var fontPath string

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
		"C:/Windows/Fonts/msyh.ttc",        // Microsoft YaHei (Windows, .ttc collection)
		"C:/Windows/Fonts/simsun.ttc",      // SimSun (Windows, .ttc collection)
		"C:/Windows/Fonts/malgun.ttf",      // Malgun Gothic (Windows, Korean)
		"/System/Library/Fonts/PingFang.ttc", // PingFang (macOS, .ttc collection)
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc", // Noto (Linux)
	}

	for _, fp := range fonts {
		err := dc.LoadFontFace(fp, 14)
		if err == nil {
			fontPath = fp
			log.Printf("Font loaded: %s", fp)
			break
		}
		log.Printf("Tried %s: %v", fp, err)
	}

	if fontPath == "" {
		log.Fatal("No CJK font found. Install Noto Sans CJK or Microsoft YaHei.")
	}

	dc.SetRGB(0, 0, 0)
	y := 30.0

	// Section 1: CJK at various body text sizes (Tier 6 bitmap, exact size)
	title(dc, "CJK Body Text — Tier 6 Bitmap (exact size, no bucket quantization)", 10, y)
	y += 25

	bodySizes := []float64{12, 14, 16, 18, 20, 24}
	for _, size := range bodySizes {
		dc.LoadFontFace(fontPath, size)
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
		dc.LoadFontFace(fontPath, size)
		dc.DrawString("中文大标题", 20, y)
		y += size + 10
	}

	// Section 3: Mixed Latin + CJK
	y = 30
	title(dc, "Mixed Script", 500, y)
	y += 25
	dc.LoadFontFace(fontPath, 16)
	dc.DrawString("Hello 世界!", 500, y)
	y += 24
	dc.DrawString("Go言語 is 素晴らしい", 500, y)
	y += 24
	dc.DrawString("1234 가나다라", 500, y)

	// Save.
	outPath := "../../tmp/cjk_text_validation.png"
	if err := dc.SavePNG(outPath); err != nil {
		log.Fatalf("SavePNG: %v", err)
	}
	log.Printf("Saved: %s", outPath)
}

func title(dc *gg.Context, s string, x, y float64) {
	dc.SetRGB(0.2, 0.2, 0.8)
	dc.LoadFontFace(fontPath, 13)
	dc.DrawString(s, x, y)
	dc.SetRGB(0, 0, 0)
}
