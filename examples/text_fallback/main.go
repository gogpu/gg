package main

import (
	"log"
	"os"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

func main() {
	// Find system fonts
	mainFont := findMainFont()
	emojiFont := findEmojiFont()

	if mainFont == "" {
		log.Println("No main font found. Skipping text fallback example.")
		return
	}

	// Load main font source
	mainSource, err := text.NewFontSourceFromFile(mainFont)
	if err != nil {
		log.Fatalf("Failed to load main font: %v", err)
	}
	defer func() { _ = mainSource.Close() }()

	// Create context
	dc := gg.NewContext(800, 500)
	dc.ClearWithColor(gg.White)

	// Create main face
	mainFace := mainSource.Face(32)

	// Title
	dc.SetFont(mainFace)
	dc.SetRGB(0.1, 0.1, 0.1)
	dc.DrawString("Font Fallback Demo", 50, 60)

	// Draw text with main font only
	dc.SetFont(mainFace)
	dc.SetRGB(0.3, 0.3, 0.3)
	dc.DrawString("Single font: Hello World!", 50, 120)

	// Try to create MultiFace with emoji support
	drawMultiFaceDemo(dc, mainFace, mainSource, emojiFont)

	// Filtered Face demo
	dc.SetFont(mainFace)
	dc.SetRGB(0.1, 0.1, 0.1)
	dc.DrawString("FilteredFace Demo", 50, 300)

	// Create ASCII-only filtered face
	asciiOnlyFace := text.NewFilteredFace(mainFace, text.RangeBasicLatin)

	dc.SetFont(asciiOnlyFace)
	dc.SetRGB(0.4, 0.6, 0.3)
	dc.DrawString("ASCII only: Hello (extended chars filtered)", 50, 350)

	// Latin Extended demo
	latinExtFace := text.NewFilteredFace(mainFace,
		text.RangeBasicLatin,
		text.RangeLatinExtA,
		text.RangeLatinExtB,
	)

	dc.SetFont(latinExtFace)
	dc.SetRGB(0.6, 0.3, 0.5)
	dc.DrawString("Latin Extended: cafe, naive, resume", 50, 400)

	// Font info
	face14 := mainSource.Face(14)
	dc.SetFont(face14)
	dc.SetRGB(0.6, 0.6, 0.6)
	dc.DrawString("Main font: "+mainSource.Name(), 50, 460)
	if emojiFont != "" {
		dc.DrawString("Emoji font: found", 50, 480)
	}

	// Save to PNG
	if err := dc.SavePNG("text_fallback.png"); err != nil {
		log.Fatalf("Failed to save PNG: %v", err)
	}

	log.Println("Created text_fallback.png")
}

// drawMultiFaceDemo demonstrates MultiFace with emoji fallback.
func drawMultiFaceDemo(dc *gg.Context, mainFace text.Face, mainSource *text.FontSource, emojiFont string) {
	if emojiFont == "" {
		drawNoEmojiFallback(dc, mainSource)
		return
	}

	emojiSource, err := text.NewFontSourceFromFile(emojiFont)
	if err != nil {
		log.Printf("Failed to load emoji font: %v", err)
		drawNoEmojiFallback(dc, mainSource)
		return
	}
	defer func() { _ = emojiSource.Close() }()

	// Create filtered emoji face
	emojiFace := text.NewFilteredFace(
		emojiSource.Face(32),
		text.RangeEmoji,
		text.RangeEmojiMisc,
		text.RangeEmojiSymbols,
	)

	// Create MultiFace: main font first, emoji fallback
	multiFace, err := text.NewMultiFace(mainFace, emojiFace)
	if err != nil {
		log.Printf("Failed to create MultiFace: %v", err)
		return
	}

	// Draw with MultiFace (emoji should use fallback font)
	dc.SetFont(multiFace)
	dc.SetRGB(0.2, 0.4, 0.7)
	dc.DrawString("MultiFace: Hello World! [emoji here]", 50, 180)

	// Explanation
	face16 := mainSource.Face(16)
	dc.SetFont(face16)
	dc.SetRGB(0.5, 0.5, 0.5)
	dc.DrawString("Emoji characters use fallback font automatically", 50, 220)
}

// drawNoEmojiFallback draws fallback message when no emoji font available.
func drawNoEmojiFallback(dc *gg.Context, mainSource *text.FontSource) {
	face16 := mainSource.Face(16)
	dc.SetFont(face16)
	dc.SetRGB(0.8, 0.4, 0.1)
	dc.DrawString("No emoji font found - fallback not available", 50, 180)
}

// findMainFont returns path to a TTF font (TTC collections not supported).
func findMainFont() string {
	// Only TTF files are supported (not TTC font collections)
	candidates := []string{
		// Windows
		"C:\\Windows\\Fonts\\arial.ttf",
		"C:\\Windows\\Fonts\\calibri.ttf",
		"C:\\Windows\\Fonts\\segoeui.ttf",
		// macOS - Supplemental fonts are TTF
		"/Library/Fonts/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Courier New.ttf",
		"/System/Library/Fonts/Supplemental/Times New Roman.ttf",
		"/System/Library/Fonts/Monaco.ttf",
		// Linux
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// findEmojiFont returns path to an emoji TTF font if available.
func findEmojiFont() string {
	// Only TTF files are supported (TTC like Apple Color Emoji not supported)
	candidates := []string{
		// Windows
		"C:\\Windows\\Fonts\\seguiemj.ttf", // Segoe UI Emoji
		"C:\\Windows\\Fonts\\seguisym.ttf", // Segoe UI Symbol
		// Linux
		"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
		"/usr/share/fonts/noto-emoji/NotoColorEmoji.ttf",
		"/usr/share/fonts/TTF/NotoEmoji-Regular.ttf",
	}
	// Note: macOS Apple Color Emoji is TTC, not supported

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}
