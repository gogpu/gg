package msdf

import (
	"math"
	"sync"

	"github.com/gogpu/gg/text"
)

// Generator creates MSDF textures from glyph outlines.
type Generator struct {
	config Config
}

// NewGenerator creates a new MSDF generator with the given configuration.
func NewGenerator(config Config) *Generator {
	return &Generator{
		config: config,
	}
}

// DefaultGenerator creates a new MSDF generator with default configuration.
func DefaultGenerator() *Generator {
	return NewGenerator(DefaultConfig())
}

// Config returns the generator's configuration.
func (g *Generator) Config() Config {
	return g.config
}

// SetConfig updates the generator's configuration.
func (g *Generator) SetConfig(config Config) {
	g.config = config
}

// Generate creates an MSDF texture from a glyph outline.
// Returns nil if the outline is empty.
func (g *Generator) Generate(outline *text.GlyphOutline) (*MSDF, error) {
	if err := g.config.Validate(); err != nil {
		return nil, err
	}

	if outline == nil || outline.IsEmpty() {
		// Return a simple transparent MSDF for empty glyphs
		return g.generateEmpty(), nil
	}

	// Convert outline to shape with edges
	shape := FromOutline(outline)
	if shape.EdgeCount() == 0 {
		return g.generateEmpty(), nil
	}

	// Assign colors to edges based on corners
	AssignColors(shape, g.config.AngleThreshold)

	// Calculate scaling and translation
	shapeBounds := shape.Bounds
	if shapeBounds.IsEmpty() {
		return g.generateEmpty(), nil
	}

	// Add padding for the distance range
	padding := g.config.Range
	bounds := shapeBounds.Expand(padding)

	// Calculate scale to fit in texture size
	scale := calculateScale(bounds, g.config.Size, padding)

	// Center the expanded bounds within the MSDF cell.
	// With uniform scaling (min of scaleX, scaleY), the non-limiting axis
	// doesn't fill the available space. Centering ensures the glyph content
	// is at the center of the cell, which allows symmetric padding in the
	// screen quad computation (gpu_text.go).
	occupiedW := bounds.Width() * scale
	occupiedH := bounds.Height() * scale
	translateX := (float64(g.config.Size) - occupiedW) / 2
	translateY := (float64(g.config.Size) - occupiedH) / 2

	// Create the MSDF texture
	msdf := &MSDF{
		Data:       make([]byte, g.config.Size*g.config.Size*3),
		Width:      g.config.Size,
		Height:     g.config.Size,
		Bounds:     bounds,
		Scale:      scale,
		TranslateX: translateX,
		TranslateY: translateY,
	}

	// Generate the distance field
	g.generateDistanceField(msdf, shape)

	return msdf, nil
}

// generateEmpty creates an MSDF for an empty glyph (e.g., space).
func (g *Generator) generateEmpty() *MSDF {
	size := g.config.Size
	data := make([]byte, size*size*3)

	// Fill with "outside" distance (< 0.5 = outside)
	// We use 0 (far outside) for empty glyphs
	for i := range data {
		data[i] = 0
	}

	return &MSDF{
		Data:   data,
		Width:  size,
		Height: size,
		Bounds: Rect{},
		Scale:  1.0,
	}
}

// generateDistanceField fills the MSDF data with distance values.
func (g *Generator) generateDistanceField(msdf *MSDF, shape *Shape) {
	size := g.config.Size
	pixelRange := g.config.Range

	// Process rows in parallel for performance
	var wg sync.WaitGroup
	numWorkers := 4 // Reasonable default for most systems

	rowsPerWorker := (size + numWorkers - 1) / numWorkers

	for w := 0; w < numWorkers; w++ {
		startRow := w * rowsPerWorker
		endRow := startRow + rowsPerWorker
		if endRow > size {
			endRow = size
		}
		if startRow >= endRow {
			continue
		}

		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			g.processRows(msdf, shape, start, end, pixelRange)
		}(startRow, endRow)
	}

	wg.Wait()
}

// processRows processes a range of rows in the MSDF.
func (g *Generator) processRows(msdf *MSDF, shape *Shape, startRow, endRow int, pixelRange float64) {
	size := msdf.Width

	for y := startRow; y < endRow; y++ {
		for x := 0; x < size; x++ {
			// Convert pixel coordinates to outline coordinates
			px := float64(x) + 0.5 // Center of pixel
			py := float64(y) + 0.5

			// Unproject to outline space
			ox := (px-msdf.TranslateX)/msdf.Scale + msdf.Bounds.MinX
			oy := (py-msdf.TranslateY)/msdf.Scale + msdf.Bounds.MinY

			point := Point{X: ox, Y: oy}

			// Calculate signed distance for each channel
			r := g.channelDistance(shape, point, SelectRed)
			gDist := g.channelDistance(shape, point, SelectGreen)
			b := g.channelDistance(shape, point, SelectBlue)

			// Normalize distances to [0, 255]
			rVal := distanceToPixel(r.Distance, pixelRange, msdf.Scale)
			gVal := distanceToPixel(gDist.Distance, pixelRange, msdf.Scale)
			bVal := distanceToPixel(b.Distance, pixelRange, msdf.Scale)

			msdf.SetPixel(x, y, rVal, gVal, bVal)
		}
	}
}

// channelDistance calculates the minimum signed distance for a specific channel.
func (g *Generator) channelDistance(shape *Shape, p Point, selector EdgeSelectorFunc) SignedDistance {
	minDist := Infinite()

	for _, contour := range shape.Contours {
		for _, edge := range contour.Edges {
			if !selector(edge.Color) {
				continue
			}

			sd := edge.SignedDistance(p)
			minDist = minDist.Combine(sd)
		}
	}

	// If no edges selected, fall back to all edges (shouldn't happen with proper coloring)
	if minDist.Distance == math.MaxFloat64 {
		for _, contour := range shape.Contours {
			for _, edge := range contour.Edges {
				sd := edge.SignedDistance(p)
				minDist = minDist.Combine(sd)
			}
		}
	}

	return minDist
}

// distanceToPixel converts a signed distance to a pixel value [0, 255].
// 0.5 (128) represents the edge, < 0.5 is outside, > 0.5 is inside.
func distanceToPixel(distance, pixelRange, scale float64) byte {
	// Convert distance to pixel units
	distPx := distance * scale

	// Map to [0, 1] range centered at 0.5
	normalized := 0.5 + distPx/(2*pixelRange)

	// Clamp to [0, 1]
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}

	// Convert to byte
	// Use math.Round for proper rounding
	return byte(math.Round(normalized * 255))
}

// calculateScale determines the scale factor to fit the shape in the texture.
func calculateScale(bounds Rect, size int, padding float64) float64 {
	available := float64(size) - 2*padding
	if available <= 0 {
		available = float64(size)
	}

	w := bounds.Width()
	h := bounds.Height()

	if w <= 0 && h <= 0 {
		return 1.0
	}

	scaleX := available / w
	scaleY := available / h

	// Use the smaller scale to ensure the shape fits
	if w > 0 && h > 0 {
		return min(scaleX, scaleY)
	}
	if w > 0 {
		return scaleX
	}
	return scaleY
}

// GenerateBatch generates MSDF textures for multiple outlines.
// This is more efficient than generating them one by one.
func (g *Generator) GenerateBatch(outlines []*text.GlyphOutline) ([]*MSDF, error) {
	if err := g.config.Validate(); err != nil {
		return nil, err
	}

	results := make([]*MSDF, len(outlines))
	var wg sync.WaitGroup
	var firstError error
	var errMu sync.Mutex

	for i, outline := range outlines {
		wg.Add(1)
		go func(idx int, o *text.GlyphOutline) {
			defer wg.Done()

			msdf, err := g.Generate(o)
			if err != nil {
				errMu.Lock()
				if firstError == nil {
					firstError = err
				}
				errMu.Unlock()
				return
			}
			results[idx] = msdf
		}(i, outline)
	}

	wg.Wait()

	if firstError != nil {
		return nil, firstError
	}
	return results, nil
}

// Metrics returns statistics about the generated MSDF.
type Metrics struct {
	// Width and Height of the texture.
	Width, Height int

	// Scale factor used.
	Scale float64

	// Bounds in outline space.
	Bounds Rect

	// NumContours is the number of contours in the source shape.
	NumContours int

	// NumEdges is the total number of edges.
	NumEdges int
}

// GenerateWithMetrics generates an MSDF and returns metrics.
func (g *Generator) GenerateWithMetrics(outline *text.GlyphOutline) (*MSDF, *Metrics, error) {
	if err := g.config.Validate(); err != nil {
		return nil, nil, err
	}

	if outline == nil || outline.IsEmpty() {
		msdf := g.generateEmpty()
		metrics := &Metrics{
			Width:       msdf.Width,
			Height:      msdf.Height,
			Scale:       msdf.Scale,
			Bounds:      msdf.Bounds,
			NumContours: 0,
			NumEdges:    0,
		}
		return msdf, metrics, nil
	}

	// Convert outline to shape
	shape := FromOutline(outline)

	// Collect metrics
	metrics := &Metrics{
		Width:       g.config.Size,
		Height:      g.config.Size,
		NumContours: len(shape.Contours),
		NumEdges:    shape.EdgeCount(),
		Bounds:      shape.Bounds,
	}

	// Generate the MSDF
	msdf, err := g.Generate(outline)
	if err != nil {
		return nil, nil, err
	}

	metrics.Scale = msdf.Scale

	return msdf, metrics, nil
}

// GeneratorPool manages a pool of generators for concurrent use.
type GeneratorPool struct {
	pool   sync.Pool
	config Config
}

// NewGeneratorPool creates a new generator pool with the given configuration.
func NewGeneratorPool(config Config) *GeneratorPool {
	return &GeneratorPool{
		config: config,
		pool: sync.Pool{
			New: func() interface{} {
				return NewGenerator(config)
			},
		},
	}
}

// Get retrieves a generator from the pool.
func (p *GeneratorPool) Get() *Generator {
	return p.pool.Get().(*Generator)
}

// Put returns a generator to the pool.
func (p *GeneratorPool) Put(g *Generator) {
	// Reset config in case it was modified
	g.config = p.config
	p.pool.Put(g)
}

// Generate generates an MSDF using a pooled generator.
func (p *GeneratorPool) Generate(outline *text.GlyphOutline) (*MSDF, error) {
	gen := p.Get()
	defer p.Put(gen)
	return gen.Generate(outline)
}

// MedianFilter applies a median filter to clean up noise.
// This is a post-processing step that can improve quality.
func MedianFilter(msdf *MSDF) *MSDF {
	if msdf == nil {
		return nil
	}

	result := &MSDF{
		Data:       make([]byte, len(msdf.Data)),
		Width:      msdf.Width,
		Height:     msdf.Height,
		Bounds:     msdf.Bounds,
		Scale:      msdf.Scale,
		TranslateX: msdf.TranslateX,
		TranslateY: msdf.TranslateY,
	}

	w, h := msdf.Width, msdf.Height

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rVals, gVals, bVals := collectNeighborhood(msdf, x, y, w, h)
			rMed := median9(rVals)
			gMed := median9(gVals)
			bMed := median9(bVals)

			result.SetPixel(x, y, rMed, gMed, bMed)
		}
	}

	return result
}

// collectNeighborhood collects 3x3 neighborhood pixel values with clamping.
func collectNeighborhood(msdf *MSDF, x, y, w, h int) (rVals, gVals, bVals [9]byte) {
	idx := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			nx := clampInt(x+dx, 0, w-1)
			ny := clampInt(y+dy, 0, h-1)

			r, g, b := msdf.GetPixel(nx, ny)
			rVals[idx] = r
			gVals[idx] = g
			bVals[idx] = b
			idx++
		}
	}
	return
}

// clampInt clamps value to [lo, hi].
func clampInt(val, lo, hi int) int {
	if val < lo {
		return lo
	}
	if val > hi {
		return hi
	}
	return val
}

// median9 finds the median of 9 byte values.
// Uses a sorting network for efficiency.
func median9(vals [9]byte) byte {
	// Sorting network for 9 elements (only need to find median)
	// This is faster than sorting the entire array
	swap := func(i, j int) {
		if vals[i] > vals[j] {
			vals[i], vals[j] = vals[j], vals[i]
		}
	}

	// Batcher's odd-even merge sort network (partial)
	swap(0, 1)
	swap(3, 4)
	swap(6, 7)
	swap(1, 2)
	swap(4, 5)
	swap(7, 8)
	swap(0, 1)
	swap(3, 4)
	swap(6, 7)
	swap(0, 3)
	swap(3, 6)
	swap(0, 3)
	swap(1, 4)
	swap(4, 7)
	swap(1, 4)
	swap(2, 5)
	swap(5, 8)
	swap(2, 5)
	swap(1, 3)
	swap(5, 7)
	swap(2, 6)
	swap(4, 6)
	swap(2, 4)
	swap(2, 3)
	swap(5, 6)

	return vals[4] // Middle element after partial sort
}

// ErrorCorrection applies error correction to fix artifacts.
// This handles cases where the median produces incorrect results.
func ErrorCorrection(msdf *MSDF, threshold float64) {
	if msdf == nil {
		return
	}

	w, h := msdf.Width, msdf.Height

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b := msdf.GetPixel(x, y)

			// Get the median
			med := median3Byte(r, g, b)

			// Calculate error for each channel
			rErr := math.Abs(float64(r) - float64(med))
			gErr := math.Abs(float64(g) - float64(med))
			bErr := math.Abs(float64(b) - float64(med))

			maxErr := max(rErr, gErr, bErr)
			thresholdVal := threshold * 255

			// If error is too high, correct by moving toward median
			if maxErr > thresholdVal {
				if rErr > thresholdVal {
					r = correctChannel(r, med, thresholdVal)
				}
				if gErr > thresholdVal {
					g = correctChannel(g, med, thresholdVal)
				}
				if bErr > thresholdVal {
					b = correctChannel(b, med, thresholdVal)
				}
				msdf.SetPixel(x, y, r, g, b)
			}
		}
	}
}

// median3Byte returns the median of three byte values.
func median3Byte(a, b, c byte) byte {
	if a > b {
		a, b = b, a
	}
	if b > c {
		b = c
	}
	if a > b {
		b = a
	}
	return b
}

// correctChannel moves a channel value toward the median.
func correctChannel(val, med byte, threshold float64) byte {
	diff := float64(val) - float64(med)
	if diff > 0 {
		return byte(float64(med) + threshold)
	}
	return byte(float64(med) - threshold)
}
