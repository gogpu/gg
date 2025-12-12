// Package blend provides color blending operations.
package blend

import "github.com/gogpu/gg"

// Mode represents a blending mode.
type Mode int

const (
	// ModeSourceOver is the default alpha blending mode.
	ModeSourceOver Mode = iota
	// ModeSourceCopy replaces the destination with the source.
	ModeSourceCopy
	// ModeDestinationOver draws destination over source.
	ModeDestinationOver
	// ModeDestinationIn keeps destination where source is opaque.
	ModeDestinationIn
	// ModeDestinationOut keeps destination where source is transparent.
	ModeDestinationOut
)

// Blend blends two colors using the specified mode.
func Blend(src, dst gg.RGBA, mode Mode) gg.RGBA {
	switch mode {
	case ModeSourceOver:
		return sourceOver(src, dst)
	case ModeSourceCopy:
		return src
	case ModeDestinationOver:
		return sourceOver(dst, src)
	case ModeDestinationIn:
		return destinationIn(src, dst)
	case ModeDestinationOut:
		return destinationOut(src, dst)
	default:
		return sourceOver(src, dst)
	}
}

// sourceOver blends source over destination using alpha compositing.
func sourceOver(src, dst gg.RGBA) gg.RGBA {
	srcA := src.A
	dstA := dst.A
	invSrcA := 1.0 - srcA

	outA := srcA + dstA*invSrcA
	if outA == 0 {
		return gg.RGBA{R: 0, G: 0, B: 0, A: 0}
	}

	outR := (src.R*srcA + dst.R*dstA*invSrcA) / outA
	outG := (src.G*srcA + dst.G*dstA*invSrcA) / outA
	outB := (src.B*srcA + dst.B*dstA*invSrcA) / outA

	return gg.RGBA{
		R: outR,
		G: outG,
		B: outB,
		A: outA,
	}
}

// destinationIn keeps destination where source is opaque.
func destinationIn(src, dst gg.RGBA) gg.RGBA {
	return gg.RGBA{
		R: dst.R,
		G: dst.G,
		B: dst.B,
		A: dst.A * src.A,
	}
}

// destinationOut keeps destination where source is transparent.
func destinationOut(src, dst gg.RGBA) gg.RGBA {
	return gg.RGBA{
		R: dst.R,
		G: dst.G,
		B: dst.B,
		A: dst.A * (1 - src.A),
	}
}
