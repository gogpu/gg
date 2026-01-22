// Example: Color Emoji Extraction
//
// This example demonstrates extracting and rendering color emoji from fonts.
// Supports both CBDT/CBLC (bitmap) and COLR/CPAL (vector) color font formats.
//
// Run: go run main.go
//
// Outputs:
//   - color_emoji_bitmap.png   (36 color emoji from CBDT/CBLC font)
//   - color_emoji_palette.png  (COLR/CPAL color palette visualization)
//   - color_emoji_sample.png   (single emoji at high resolution)
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/gogpu/gg/text/emoji"
)

func main() {
	fmt.Println("Color Emoji Extraction Example")
	fmt.Println("===============================")

	// Find fonts
	bitmapFont := findFont([]string{
		"./NotoColorEmoji-Regular.ttf",
		"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
	})

	vectorFont := findFont([]string{
		"C:/Windows/Fonts/seguiemj.ttf",
		"/System/Library/Fonts/Apple Color Emoji.ttc",
	})

	if bitmapFont == nil && vectorFont == nil {
		log.Fatal("No color emoji font found.\n" +
			"Download NotoColorEmoji-Regular.ttf from:\n" +
			"https://github.com/googlefonts/noto-emoji/raw/main/fonts/NotoColorEmoji.ttf")
	}

	generated := 0

	// 1. Generate bitmap emoji grid (CBDT/CBLC)
	if bitmapFont != nil {
		if generateBitmapGrid(bitmapFont) {
			generated++
		}
		if generateSampleEmoji(bitmapFont) {
			generated++
		}
	}

	// 2. Generate color palette (COLR/CPAL)
	if vectorFont != nil {
		if generateColorPalette(vectorFont) {
			generated++
		}
	}

	// Fallback: if no vector font, try bitmap font for palette-like output
	if vectorFont == nil && bitmapFont != nil && generated < 3 {
		fmt.Println("\nNo COLR/CPAL font found, using bitmap font info instead")
	}

	fmt.Printf("\n=== Generated %d files ===\n", generated)
}

func findFont(paths []string) []byte {
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			fmt.Printf("Found font: %s (%d bytes)\n", path, len(data))
			return data
		}
	}
	return nil
}

func getTable(data []byte, tag string) []byte {
	if len(data) < 12 {
		return nil
	}
	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	offset := 12
	for i := 0; i < numTables && offset+16 <= len(data); i++ {
		t := string(data[offset : offset+4])
		tableOffset := binary.BigEndian.Uint32(data[offset+8 : offset+12])
		tableLength := binary.BigEndian.Uint32(data[offset+12 : offset+16])
		if t == tag && tableOffset+tableLength <= uint32(len(data)) {
			return data[tableOffset : tableOffset+tableLength]
		}
		offset += 16
	}
	return nil
}

// generateBitmapGrid creates a 6x6 grid of color emoji
func generateBitmapGrid(fontData []byte) bool {
	cbdt := getTable(fontData, "CBDT")
	cblc := getTable(fontData, "CBLC")
	if cbdt == nil || cblc == nil {
		return false
	}

	extractor, err := emoji.NewCBDTExtractor(cbdt, cblc)
	if err != nil {
		log.Printf("CBDTExtractor error: %v\n", err)
		return false
	}

	ppems := extractor.AvailablePPEMs()
	if len(ppems) == 0 {
		return false
	}

	targetPPEM := ppems[len(ppems)-1]
	fmt.Printf("\nExtracting bitmap emoji (PPEM=%d)...\n", targetPPEM)

	// Create 6x6 grid
	gridSize := 6
	cellSize := 136
	padding := 4
	canvasSize := gridSize*(cellSize+padding) + padding

	canvas := image.NewRGBA(image.Rect(0, 0, canvasSize, canvasSize))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

	found := 0
	for gid := uint16(1); gid < 10000 && found < gridSize*gridSize; gid++ {
		glyph, err := extractor.GetGlyph(gid, targetPPEM)
		if err != nil || glyph.Data == nil || len(glyph.Data) == 0 {
			continue
		}

		img, err := png.Decode(bytes.NewReader(glyph.Data))
		if err != nil {
			continue
		}

		row := found / gridSize
		col := found % gridSize
		x := padding + col*(cellSize+padding)
		y := padding + row*(cellSize+padding)

		bounds := img.Bounds()
		dstRect := image.Rect(x, y, x+bounds.Dx(), y+bounds.Dy())
		draw.Draw(canvas, dstRect, img, bounds.Min, draw.Over)
		found++
	}

	outPath := "color_emoji_bitmap.png"
	f, err := os.Create(outPath)
	if err != nil {
		log.Printf("Failed to create %s: %v\n", outPath, err)
		return false
	}
	defer f.Close()
	png.Encode(f, canvas)

	absPath, _ := filepath.Abs(outPath)
	fmt.Printf("  [1] %s (%d emoji in 6x6 grid)\n", absPath, found)
	return true
}

// generateSampleEmoji extracts a single emoji at full resolution
func generateSampleEmoji(fontData []byte) bool {
	cbdt := getTable(fontData, "CBDT")
	cblc := getTable(fontData, "CBLC")
	if cbdt == nil || cblc == nil {
		return false
	}

	extractor, err := emoji.NewCBDTExtractor(cbdt, cblc)
	if err != nil {
		return false
	}

	ppems := extractor.AvailablePPEMs()
	if len(ppems) == 0 {
		return false
	}

	targetPPEM := ppems[len(ppems)-1]

	// Find first valid glyph
	for gid := uint16(1); gid < 1000; gid++ {
		glyph, err := extractor.GetGlyph(gid, targetPPEM)
		if err != nil || glyph.Data == nil {
			continue
		}

		img, err := png.Decode(bytes.NewReader(glyph.Data))
		if err != nil {
			continue
		}

		outPath := "color_emoji_sample.png"
		f, err := os.Create(outPath)
		if err != nil {
			return false
		}
		defer f.Close()
		png.Encode(f, img)

		absPath, _ := filepath.Abs(outPath)
		bounds := img.Bounds()
		fmt.Printf("  [2] %s (glyph %d, %dx%d)\n", absPath, gid, bounds.Dx(), bounds.Dy())
		return true
	}

	return false
}

// generateColorPalette creates a visualization of COLR/CPAL colors
func generateColorPalette(fontData []byte) bool {
	colr := getTable(fontData, "COLR")
	cpal := getTable(fontData, "CPAL")
	if colr == nil || cpal == nil {
		return false
	}

	parser, err := emoji.NewCOLRParser(colr, cpal)
	if err != nil {
		log.Printf("COLRParser error: %v\n", err)
		return false
	}

	if parser.NumPalettes() == 0 {
		return false
	}

	colors := parser.PaletteColors(0)
	fmt.Printf("\nExtracting COLR palette (%d colors)...\n", len(colors))

	// Create palette grid
	cellSize := 32
	cols := 16
	rows := (len(colors) + cols - 1) / cols
	if rows < 1 {
		rows = 1
	}

	canvas := image.NewRGBA(image.Rect(0, 0, cols*cellSize, rows*cellSize))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

	for i, c := range colors {
		x := (i % cols) * cellSize
		y := (i / cols) * cellSize
		rect := image.Rect(x, y, x+cellSize-1, y+cellSize-1)
		uniform := image.NewUniform(color.RGBA{c.R, c.G, c.B, c.A})
		draw.Draw(canvas, rect, uniform, image.Point{}, draw.Src)
	}

	outPath := "color_emoji_palette.png"
	f, err := os.Create(outPath)
	if err != nil {
		log.Printf("Failed to create %s: %v\n", outPath, err)
		return false
	}
	defer f.Close()
	png.Encode(f, canvas)

	absPath, _ := filepath.Abs(outPath)
	fmt.Printf("  [3] %s (%d colors)\n", absPath, len(colors))
	return true
}
