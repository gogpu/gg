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
	"os"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

var fontSource *text.FontSource

func main() {
	const W, H = 800, 600
	dc := gg.NewContext(W, H)

	dc.SetRGB(1, 1, 1)
	dc.DrawRectangle(0, 0, W, H)
	_ = dc.Fill()

	fonts := []string{
		"C:/Windows/Fonts/msyh.ttc",
		"C:/Windows/Fonts/simsun.ttc",
		"C:/Windows/Fonts/malgun.ttf",
		"/System/Library/Fonts/PingFang.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
	}

	for _, fp := range fonts {
		if _, err := os.Stat(fp); err != nil {
			continue
		}
		src, err := text.NewFontSourceFromFile(fp)
		if err != nil {
			log.Printf("Tried %s: %v", fp, err)
			continue
		}
		fontSource = src
		log.Printf("Font loaded: %s", fp)
		break
	}

	if fontSource == nil {
		log.Fatal("No CJK font found. Install Noto Sans CJK or Microsoft YaHei.")
	}
	defer func() { _ = fontSource.Close() }()

	dc.SetRGB(0, 0, 0)
	y := 30.0

	title(dc, "CJK Body Text — Tier 6 Bitmap (exact size, no bucket quantization)", 10, y)
	y += 25

	bodySizes := []float64{12, 14, 16, 18, 20, 24}
	for _, size := range bodySizes {
		dc.SetFont(fontSource.Face(size))
		label := fmt.Sprintf("%gpx: 中文测试 日本語テスト 한국어 — The quick brown fox", size)
		dc.DrawString(label, 20, y)
		y += size + 8
	}

	y += 15

	title(dc, "CJK Display Text — Tier 4 MSDF (128px reference for CJK)", 10, y)
	y += 25

	displaySizes := []float64{36, 48, 64, 72}
	for _, size := range displaySizes {
		dc.SetFont(fontSource.Face(size))
		dc.DrawString("中文大标题", 20, y)
		y += size + 10
	}

	y = 30
	title(dc, "Mixed Script", 500, y)
	y += 25
	dc.SetFont(fontSource.Face(16))
	dc.DrawString("Hello 世界!", 500, y)
	y += 24
	dc.DrawString("Go言語 is 素晴らしい", 500, y)
	y += 24
	dc.DrawString("1234 가나다라", 500, y)

	outPath := "../../tmp/cjk_text_validation.png"
	if err := dc.SavePNG(outPath); err != nil {
		log.Fatalf("SavePNG: %v", err)
	}
	log.Printf("Saved: %s", outPath)
}

func title(dc *gg.Context, s string, x, y float64) {
	dc.SetRGB(0.2, 0.2, 0.8)
	dc.SetFont(fontSource.Face(13))
	dc.DrawString(s, x, y)
	dc.SetRGB(0, 0, 0)
}
