// Example: Color Emoji Extraction
//
// This example demonstrates extracting and rendering color emoji from fonts.
// Supports both CBDT/CBLC (bitmap) and COLR/CPAL (vector) color font formats.
//
// Run: go run main.go
//
// Outputs:
//   - color_emoji_composite.png (bitmap emoji grid)
//   - colr_palette.png (COLR color palette visualization)
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
	// Try to find a color emoji font
	fontPaths := []string{
		"./NotoColorEmoji-Regular.ttf", // Noto Color Emoji (CBDT/CBLC bitmap)
		"C:/Windows/Fonts/seguiemj.ttf", // Windows Segoe UI Emoji (COLR/CPAL)
		"/System/Library/Fonts/Apple Color Emoji.ttc", // macOS
		"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf", // Linux
	}

	var fontData []byte
	var usedPath string
	var err error

	for _, path := range fontPaths {
		fontData, err = os.ReadFile(path)
		if err == nil {
			usedPath = path
			break
		}
	}

	if fontData == nil {
		log.Fatal("No color emoji font found. Download NotoColorEmoji-Regular.ttf from:\n" +
			"https://github.com/googlefonts/noto-emoji/raw/main/fonts/NotoColorEmoji.ttf")
	}

	fmt.Printf("Using font: %s (%d bytes)\n", usedPath, len(fontData))

	// Analyze font tables
	tables := analyzeFontTables(fontData)
	fmt.Printf("\nFont tables:\n")
	for _, t := range tables {
		fmt.Printf("  %s: %d bytes\n", t.tag, t.length)
	}

	// Try CBDT/CBLC (bitmap emoji like Noto Color Emoji)
	cbdtData := getTable(fontData, "CBDT")
	cblcData := getTable(fontData, "CBLC")

	if cbdtData != nil && cblcData != nil {
		fmt.Printf("\n=== CBDT/CBLC Bitmap Emoji ===\n")
		extractBitmapEmoji(cbdtData, cblcData)
		return
	}

	// Try COLR/CPAL (vector color layers like Segoe UI Emoji)
	colrData := getTable(fontData, "COLR")
	cpalData := getTable(fontData, "CPAL")

	if colrData != nil && cpalData != nil {
		fmt.Printf("\n=== COLR/CPAL Vector Emoji ===\n")
		visualizeCOLREmoji(colrData, cpalData)
		return
	}

	fmt.Println("\nNo color emoji tables found (need CBDT/CBLC or COLR/CPAL)")
}

type tableInfo struct {
	tag    string
	offset uint32
	length uint32
}

func analyzeFontTables(data []byte) []tableInfo {
	if len(data) < 12 {
		return nil
	}

	numTables := int(binary.BigEndian.Uint16(data[4:6]))
	var tables []tableInfo
	offset := 12

	for i := 0; i < numTables && offset+16 <= len(data); i++ {
		tag := string(data[offset : offset+4])
		tableOffset := binary.BigEndian.Uint32(data[offset+8 : offset+12])
		tableLength := binary.BigEndian.Uint32(data[offset+12 : offset+16])
		tables = append(tables, tableInfo{tag, tableOffset, tableLength})
		offset += 16
	}

	return tables
}

func getTable(data []byte, tag string) []byte {
	tables := analyzeFontTables(data)
	for _, t := range tables {
		if t.tag == tag && t.offset+t.length <= uint32(len(data)) {
			return data[t.offset : t.offset+t.length]
		}
	}
	return nil
}

func extractBitmapEmoji(cbdtData, cblcData []byte) {
	extractor, err := emoji.NewCBDTExtractor(cbdtData, cblcData)
	if err != nil {
		log.Printf("CBDTExtractor error: %v\n", err)
		return
	}

	ppems := extractor.AvailablePPEMs()
	fmt.Printf("Available sizes (PPEM): %v\n", ppems)

	if len(ppems) == 0 {
		fmt.Println("No bitmap strikes available")
		return
	}

	targetPPEM := ppems[len(ppems)-1]
	fmt.Printf("Using PPEM: %d\n", targetPPEM)

	// Create 6x6 grid of emoji
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

	fmt.Printf("Extracted %d color emoji\n", found)

	outPath := "color_emoji_composite.png"
	f, err := os.Create(outPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, canvas)

	absPath, _ := filepath.Abs(outPath)
	fmt.Printf("Saved: %s\n", absPath)
}

func visualizeCOLREmoji(colrData, cpalData []byte) {
	parser, err := emoji.NewCOLRParser(colrData, cpalData)
	if err != nil {
		log.Printf("COLRParser error: %v\n", err)
		return
	}

	fmt.Printf("Palettes: %d\n", parser.NumPalettes())

	if parser.NumPalettes() == 0 {
		return
	}

	colors := parser.PaletteColors(0)
	fmt.Printf("Colors in palette 0: %d\n", len(colors))

	// Create color palette visualization
	cellSize := 32
	cols := 16
	rows := (len(colors) + cols - 1) / cols
	canvas := image.NewRGBA(image.Rect(0, 0, cols*cellSize, rows*cellSize))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)

	for i, c := range colors {
		x := (i % cols) * cellSize
		y := (i / cols) * cellSize
		rect := image.Rect(x, y, x+cellSize-1, y+cellSize-1)
		uniform := image.NewUniform(color.RGBA{c.R, c.G, c.B, c.A})
		draw.Draw(canvas, rect, uniform, image.Point{}, draw.Src)
	}

	outPath := "colr_palette.png"
	f, err := os.Create(outPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, canvas)

	absPath, _ := filepath.Abs(outPath)
	fmt.Printf("Saved palette: %s\n", absPath)

	// Show some color glyph info
	fmt.Printf("\nSample color glyphs:\n")
	found := 0
	for gid := uint16(1); gid < 5000 && found < 5; gid++ {
		if parser.HasGlyph(gid) {
			glyph, err := parser.GetGlyph(gid, 0)
			if err == nil && len(glyph.Layers) > 0 {
				fmt.Printf("  Glyph %d: %d color layers\n", gid, len(glyph.Layers))
				found++
			}
		}
	}
}
