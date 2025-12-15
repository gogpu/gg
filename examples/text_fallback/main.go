package main

import (
	"log"
	"os"
	"runtime"

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
	ctx := gg.NewContext(800, 500)
	ctx.ClearWithColor(gg.White)

	// Create main face
	mainFace := mainSource.Face(32)

	// Title
	ctx.SetFont(mainFace)
	ctx.SetRGB(0.1, 0.1, 0.1)
	ctx.DrawString("Font Fallback Demo", 50, 60)

	// Draw text with main font only
	ctx.SetFont(mainFace)
	ctx.SetRGB(0.3, 0.3, 0.3)
	ctx.DrawString("Single font: Hello World!", 50, 120)

	// Try to create MultiFace with emoji support
	drawMultiFaceDemo(ctx, mainFace, mainSource, emojiFont)

	// Filtered Face demo
	ctx.SetFont(mainFace)
	ctx.SetRGB(0.1, 0.1, 0.1)
	ctx.DrawString("FilteredFace Demo", 50, 300)

	// Create ASCII-only filtered face
	asciiOnlyFace := text.NewFilteredFace(mainFace, text.RangeBasicLatin)

	ctx.SetFont(asciiOnlyFace)
	ctx.SetRGB(0.4, 0.6, 0.3)
	ctx.DrawString("ASCII only: Hello (extended chars filtered)", 50, 350)

	// Latin Extended demo
	latinExtFace := text.NewFilteredFace(mainFace,
		text.RangeBasicLatin,
		text.RangeLatinExtA,
		text.RangeLatinExtB,
	)

	ctx.SetFont(latinExtFace)
	ctx.SetRGB(0.6, 0.3, 0.5)
	ctx.DrawString("Latin Extended: cafe, naive, resume", 50, 400)

	// Font info
	face14 := mainSource.Face(14)
	ctx.SetFont(face14)
	ctx.SetRGB(0.6, 0.6, 0.6)
	ctx.DrawString("Main font: "+mainSource.Name(), 50, 460)
	if emojiFont != "" {
		ctx.DrawString("Emoji font: found", 50, 480)
	}

	// Save to PNG
	if err := ctx.SavePNG("text_fallback.png"); err != nil {
		log.Fatalf("Failed to save PNG: %v", err)
	}

	log.Println("Created text_fallback.png")
}

// drawMultiFaceDemo demonstrates MultiFace with emoji fallback.
func drawMultiFaceDemo(ctx *gg.Context, mainFace text.Face, mainSource *text.FontSource, emojiFont string) {
	if emojiFont == "" {
		drawNoEmojiFallback(ctx, mainSource)
		return
	}

	emojiSource, err := text.NewFontSourceFromFile(emojiFont)
	if err != nil {
		log.Printf("Failed to load emoji font: %v", err)
		drawNoEmojiFallback(ctx, mainSource)
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
	ctx.SetFont(multiFace)
	ctx.SetRGB(0.2, 0.4, 0.7)
	ctx.DrawString("MultiFace: Hello World! [emoji here]", 50, 180)

	// Explanation
	face16 := mainSource.Face(16)
	ctx.SetFont(face16)
	ctx.SetRGB(0.5, 0.5, 0.5)
	ctx.DrawString("Emoji characters use fallback font automatically", 50, 220)
}

// drawNoEmojiFallback draws fallback message when no emoji font available.
func drawNoEmojiFallback(ctx *gg.Context, mainSource *text.FontSource) {
	face16 := mainSource.Face(16)
	ctx.SetFont(face16)
	ctx.SetRGB(0.8, 0.4, 0.1)
	ctx.DrawString("No emoji font found - fallback not available", 50, 180)
}

// findMainFont returns path to a main system font.
func findMainFont() string {
	var candidates []string

	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			"C:\\Windows\\Fonts\\arial.ttf",
			"C:\\Windows\\Fonts\\calibri.ttf",
			"C:\\Windows\\Fonts\\segoeui.ttf",
		}
	case "darwin":
		candidates = []string{
			"/System/Library/Fonts/Helvetica.ttc",
			"/System/Library/Fonts/SFNSText.ttf",
			"/Library/Fonts/Arial.ttf",
		}
	default: // Linux
		candidates = []string{
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/TTF/DejaVuSans.ttf",
			"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
		}
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// findEmojiFont returns path to an emoji font if available.
func findEmojiFont() string {
	var candidates []string

	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			"C:\\Windows\\Fonts\\seguiemj.ttf", // Segoe UI Emoji
			"C:\\Windows\\Fonts\\seguisym.ttf", // Segoe UI Symbol
		}
	case "darwin":
		candidates = []string{
			"/System/Library/Fonts/Apple Color Emoji.ttc",
			"/System/Library/Fonts/Supplemental/Apple Color Emoji.ttc",
		}
	default: // Linux
		candidates = []string{
			"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
			"/usr/share/fonts/noto-emoji/NotoColorEmoji.ttf",
			"/usr/share/fonts/TTF/NotoEmoji-Regular.ttf",
		}
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}
