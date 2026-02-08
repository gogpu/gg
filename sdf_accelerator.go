package gg

import "math"

// SDFAccelerator is a CPU-based SDF accelerator for simple geometric shapes.
// It produces smoother circles and rounded rectangles than the default
// area-based rasterizer by computing per-pixel signed distance fields.
//
// This accelerator only handles circles and rounded rectangles via the
// AccelCircleSDF and AccelRRectSDF operations. All other operations fall
// back to the software renderer.
//
// Usage:
//
//	gg.RegisterAccelerator(&gg.SDFAccelerator{})
type SDFAccelerator struct{}

// Compile-time interface check.
var _ GPUAccelerator = (*SDFAccelerator)(nil)

// Name returns the accelerator name.
func (a *SDFAccelerator) Name() string { return "sdf-cpu" }

// Init initializes the accelerator. No resources are needed.
func (a *SDFAccelerator) Init() error { return nil }

// Close releases resources. No-op for CPU-based SDF.
func (a *SDFAccelerator) Close() {}

// CanAccelerate reports whether the accelerator supports the given operation.
// Only circle and rounded-rect SDF operations are supported.
func (a *SDFAccelerator) CanAccelerate(op AcceleratedOp) bool {
	return op&(AccelCircleSDF|AccelRRectSDF) != 0
}

// FillPath always falls back to CPU for general paths.
func (a *SDFAccelerator) FillPath(_ GPURenderTarget, _ *Path, _ *Paint) error {
	return ErrFallbackToCPU
}

// StrokePath always falls back to CPU for general paths.
func (a *SDFAccelerator) StrokePath(_ GPURenderTarget, _ *Path, _ *Paint) error {
	return ErrFallbackToCPU
}

// FillShape renders a filled shape using SDF.
func (a *SDFAccelerator) FillShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	switch shape.Kind {
	case ShapeCircle:
		return a.fillCircleSDF(target, shape, paint)
	case ShapeEllipse:
		return a.fillEllipseSDF(target, shape, paint)
	case ShapeRect, ShapeRRect:
		return a.fillRRectSDF(target, shape, paint)
	default:
		return ErrFallbackToCPU
	}
}

// StrokeShape renders a stroked shape using SDF.
func (a *SDFAccelerator) StrokeShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	switch shape.Kind {
	case ShapeCircle:
		return a.strokeCircleSDF(target, shape, paint)
	case ShapeEllipse:
		return a.strokeEllipseSDF(target, shape, paint)
	case ShapeRect, ShapeRRect:
		return a.strokeRRectSDF(target, shape, paint)
	default:
		return ErrFallbackToCPU
	}
}

// fillCircleSDF renders a filled circle using SDF coverage.
func (a *SDFAccelerator) fillCircleSDF(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	cx, cy, r := shape.CenterX, shape.CenterY, shape.RadiusX
	color := getColorFromPaint(paint)

	// Bounding box with 1px padding for anti-aliasing.
	minX := int(math.Max(0, math.Floor(cx-r-1)))
	maxX := int(math.Min(float64(target.Width-1), math.Ceil(cx+r+1)))
	minY := int(math.Max(0, math.Floor(cy-r-1)))
	maxY := int(math.Min(float64(target.Height-1), math.Ceil(cy+r+1)))

	for py := minY; py <= maxY; py++ {
		for px := minX; px <= maxX; px++ {
			coverage := SDFFilledCircleCoverage(float64(px)+0.5, float64(py)+0.5, cx, cy, r)
			if coverage > 0 {
				blendPixel(target, px, py, color, coverage)
			}
		}
	}
	return nil
}

// strokeCircleSDF renders a stroked circle using SDF coverage.
func (a *SDFAccelerator) strokeCircleSDF(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	cx, cy, r := shape.CenterX, shape.CenterY, shape.RadiusX
	color := getColorFromPaint(paint)
	halfW := paint.EffectiveLineWidth() / 2

	// Bounding box with stroke width + 1px padding.
	pad := halfW + 1
	minX := int(math.Max(0, math.Floor(cx-r-pad)))
	maxX := int(math.Min(float64(target.Width-1), math.Ceil(cx+r+pad)))
	minY := int(math.Max(0, math.Floor(cy-r-pad)))
	maxY := int(math.Min(float64(target.Height-1), math.Ceil(cy+r+pad)))

	for py := minY; py <= maxY; py++ {
		for px := minX; px <= maxX; px++ {
			coverage := SDFCircleCoverage(float64(px)+0.5, float64(py)+0.5, cx, cy, r, halfW)
			if coverage > 0 {
				blendPixel(target, px, py, color, coverage)
			}
		}
	}
	return nil
}

// fillEllipseSDF renders a filled ellipse by scaling the SDF to a unit circle.
func (a *SDFAccelerator) fillEllipseSDF(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	cx, cy := shape.CenterX, shape.CenterY
	rx, ry := shape.RadiusX, shape.RadiusY
	color := getColorFromPaint(paint)

	// Bounding box with 1px padding.
	minX := int(math.Max(0, math.Floor(cx-rx-1)))
	maxX := int(math.Min(float64(target.Width-1), math.Ceil(cx+rx+1)))
	minY := int(math.Max(0, math.Floor(cy-ry-1)))
	maxY := int(math.Min(float64(target.Height-1), math.Ceil(cy+ry+1)))

	for py := minY; py <= maxY; py++ {
		for px := minX; px <= maxX; px++ {
			// Scale to unit circle space.
			dx := (float64(px) + 0.5 - cx) / rx
			dy := (float64(py) + 0.5 - cy) / ry
			dist := math.Sqrt(dx*dx+dy*dy)*math.Min(rx, ry) - math.Min(rx, ry)
			coverage := smoothstepCoverage(dist)
			if coverage > 0 {
				blendPixel(target, px, py, color, coverage)
			}
		}
	}
	return nil
}

// strokeEllipseSDF renders a stroked ellipse.
func (a *SDFAccelerator) strokeEllipseSDF(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	cx, cy := shape.CenterX, shape.CenterY
	rx, ry := shape.RadiusX, shape.RadiusY
	color := getColorFromPaint(paint)
	halfW := paint.EffectiveLineWidth() / 2

	pad := halfW + 1
	minX := int(math.Max(0, math.Floor(cx-rx-pad)))
	maxX := int(math.Min(float64(target.Width-1), math.Ceil(cx+rx+pad)))
	minY := int(math.Max(0, math.Floor(cy-ry-pad)))
	maxY := int(math.Min(float64(target.Height-1), math.Ceil(cy+ry+pad)))

	for py := minY; py <= maxY; py++ {
		for px := minX; px <= maxX; px++ {
			dx := (float64(px) + 0.5 - cx) / rx
			dy := (float64(py) + 0.5 - cy) / ry
			dist := math.Sqrt(dx*dx+dy*dy)*math.Min(rx, ry) - math.Min(rx, ry)
			sdf := math.Abs(dist) - halfW
			coverage := smoothstepCoverage(sdf)
			if coverage > 0 {
				blendPixel(target, px, py, color, coverage)
			}
		}
	}
	return nil
}

// fillRRectSDF renders a filled rounded rectangle using SDF coverage.
func (a *SDFAccelerator) fillRRectSDF(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	cx, cy := shape.CenterX, shape.CenterY
	halfW, halfH := shape.Width/2, shape.Height/2
	cr := shape.CornerRadius
	color := getColorFromPaint(paint)

	// Bounding box with 1px padding.
	minX := int(math.Max(0, math.Floor(cx-halfW-1)))
	maxX := int(math.Min(float64(target.Width-1), math.Ceil(cx+halfW+1)))
	minY := int(math.Max(0, math.Floor(cy-halfH-1)))
	maxY := int(math.Min(float64(target.Height-1), math.Ceil(cy+halfH+1)))

	for py := minY; py <= maxY; py++ {
		for px := minX; px <= maxX; px++ {
			coverage := SDFFilledRRectCoverage(float64(px)+0.5, float64(py)+0.5, cx, cy, halfW, halfH, cr)
			if coverage > 0 {
				blendPixel(target, px, py, color, coverage)
			}
		}
	}
	return nil
}

// strokeRRectSDF renders a stroked rounded rectangle using SDF coverage.
func (a *SDFAccelerator) strokeRRectSDF(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	cx, cy := shape.CenterX, shape.CenterY
	halfW, halfH := shape.Width/2, shape.Height/2
	cr := shape.CornerRadius
	color := getColorFromPaint(paint)
	halfStroke := paint.EffectiveLineWidth() / 2

	pad := halfStroke + 1
	minX := int(math.Max(0, math.Floor(cx-halfW-pad)))
	maxX := int(math.Min(float64(target.Width-1), math.Ceil(cx+halfW+pad)))
	minY := int(math.Max(0, math.Floor(cy-halfH-pad)))
	maxY := int(math.Min(float64(target.Height-1), math.Ceil(cy+halfH+pad)))

	for py := minY; py <= maxY; py++ {
		for px := minX; px <= maxX; px++ {
			coverage := SDFRRectCoverage(float64(px)+0.5, float64(py)+0.5, cx, cy, halfW, halfH, cr, halfStroke)
			if coverage > 0 {
				blendPixel(target, px, py, color, coverage)
			}
		}
	}
	return nil
}

// getColorFromPaint extracts a solid color from the paint.
// If the paint uses a gradient or pattern, returns the color at (0, 0).
func getColorFromPaint(paint *Paint) RGBA {
	if paint.Brush != nil {
		if sb, ok := paint.Brush.(SolidBrush); ok {
			return sb.Color
		}
		return paint.Brush.ColorAt(0, 0)
	}
	if paint.Pattern != nil {
		return paint.Pattern.ColorAt(0, 0)
	}
	return Black
}

// blendPixel performs premultiplied source-over compositing of a single pixel.
func blendPixel(target GPURenderTarget, x, y int, color RGBA, coverage float64) {
	if x < 0 || x >= target.Width || y < 0 || y >= target.Height {
		return
	}

	idx := y*target.Stride + x*4

	// Source color with coverage applied (premultiplied).
	srcA := color.A * coverage
	srcR := color.R * srcA
	srcG := color.G * srcA
	srcB := color.B * srcA

	invSrcA := 1.0 - srcA

	// Read destination (already premultiplied).
	dstR := float64(target.Data[idx+0]) / 255
	dstG := float64(target.Data[idx+1]) / 255
	dstB := float64(target.Data[idx+2]) / 255
	dstA := float64(target.Data[idx+3]) / 255

	// Source-over: Result = Src + Dst * (1 - SrcA)
	outR := srcR + dstR*invSrcA
	outG := srcG + dstG*invSrcA
	outB := srcB + dstB*invSrcA
	outA := srcA + dstA*invSrcA

	target.Data[idx+0] = uint8(clamp255(outR * 255))
	target.Data[idx+1] = uint8(clamp255(outG * 255))
	target.Data[idx+2] = uint8(clamp255(outB * 255))
	target.Data[idx+3] = uint8(clamp255(outA * 255))
}
