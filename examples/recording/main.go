// Example: Recording System
//
// This example demonstrates the recording system for vector export.
// It creates a drawing, records it, and exports to multiple formats.
//
// To run with PDF/SVG export, you need the backend packages:
//
//	go get github.com/gogpu/gg-pdf
//	go get github.com/gogpu/gg-svg
//
// Then uncomment the relevant import lines below.
package main

import (
	"fmt"
	"log"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/recording"
	_ "github.com/gogpu/gg/recording/backends/raster" // Built-in raster backend
	// _ "github.com/gogpu/gg-pdf" // Uncomment for PDF export
	// _ "github.com/gogpu/gg-svg" // Uncomment for SVG export
)

func main() {
	// Create a recorder with dimensions
	rec := recording.NewRecorder(400, 300)

	// Draw a gradient background
	bgGrad := recording.NewLinearGradientBrush(0, 0, 400, 300).
		AddColorStop(0, gg.RGBA{R: 0.2, G: 0.3, B: 0.5, A: 1}).
		AddColorStop(1, gg.RGBA{R: 0.4, G: 0.5, B: 0.7, A: 1})
	rec.SetFillStyle(bgGrad)
	rec.DrawRectangle(0, 0, 400, 300)
	rec.Fill()

	// Draw a red circle
	rec.SetFillRGBA(1, 0.2, 0.2, 1)
	rec.DrawCircle(150, 150, 60)
	rec.Fill()

	// Draw a stroked rectangle
	rec.SetStrokeRGBA(1, 1, 1, 1)
	rec.SetLineWidth(3)
	rec.DrawRoundedRectangle(220, 80, 140, 140, 10)
	rec.Stroke()

	// Draw some lines with dash pattern
	rec.SetStrokeRGBA(1, 0.8, 0, 1)
	rec.SetLineWidth(2)
	rec.SetDash(10, 5)
	rec.MoveTo(50, 250)
	rec.LineTo(350, 250)
	rec.Stroke()

	// Clear dash for next drawing
	rec.ClearDash()

	// Draw with transform
	rec.Push()
	rec.Translate(290, 150)
	rec.Rotate(0.3)
	rec.SetFillRGBA(0.3, 0.8, 0.3, 0.8)
	rec.DrawRectangle(-30, -30, 60, 60)
	rec.Fill()
	rec.Pop()

	// Finish recording
	r := rec.FinishRecording()

	fmt.Printf("Recording: %d commands, %d paths, %d brushes\n",
		len(r.Commands()), r.Resources().PathCount(), r.Resources().BrushCount())

	// Export to raster (always available)
	exportRaster(r)

	// Export to PDF (if backend is imported)
	if recording.IsRegistered("pdf") {
		exportPDF(r)
	} else {
		fmt.Println("PDF backend not available (import github.com/gogpu/gg-pdf)")
	}

	// Export to SVG (if backend is imported)
	if recording.IsRegistered("svg") {
		exportSVG(r)
	} else {
		fmt.Println("SVG backend not available (import github.com/gogpu/gg-svg)")
	}
}

func exportRaster(r *recording.Recording) {
	backend, err := recording.NewBackend("raster")
	if err != nil {
		log.Printf("Failed to create raster backend: %v", err)
		return
	}

	if err := r.Playback(backend); err != nil {
		log.Printf("Playback failed: %v", err)
		return
	}

	// Save as PNG
	if fb, ok := backend.(recording.FileBackend); ok {
		if err := fb.SaveToFile("output.png"); err != nil {
			log.Printf("Failed to save PNG: %v", err)
			return
		}
		fmt.Println("Saved: output.png")
	}
}

func exportPDF(r *recording.Recording) {
	backend, err := recording.NewBackend("pdf")
	if err != nil {
		log.Printf("Failed to create PDF backend: %v", err)
		return
	}

	if err := r.Playback(backend); err != nil {
		log.Printf("Playback failed: %v", err)
		return
	}

	if fb, ok := backend.(recording.FileBackend); ok {
		if err := fb.SaveToFile("output.pdf"); err != nil {
			log.Printf("Failed to save PDF: %v", err)
			return
		}
		fmt.Println("Saved: output.pdf")
	}
}

func exportSVG(r *recording.Recording) {
	backend, err := recording.NewBackend("svg")
	if err != nil {
		log.Printf("Failed to create SVG backend: %v", err)
		return
	}

	if err := r.Playback(backend); err != nil {
		log.Printf("Playback failed: %v", err)
		return
	}

	if fb, ok := backend.(recording.FileBackend); ok {
		if err := fb.SaveToFile("output.svg"); err != nil {
			log.Printf("Failed to save SVG: %v", err)
			return
		}
		fmt.Println("Saved: output.svg")
	}
}
