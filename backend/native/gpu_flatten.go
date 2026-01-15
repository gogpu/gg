//go:build !nogpu

// Package wgpu provides GPU-accelerated rendering using WebGPU.
package native

import (
	_ "embed"
	"fmt"
	"math"
	"sync"

	"github.com/gogpu/gg/scene"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/types"
)

//go:embed shaders/flatten.wgsl
var flattenShaderWGSL string

// FlattenMaxSegmentsPerCurve is the maximum segments generated per curve.
const FlattenMaxSegmentsPerCurve = 64

// GPUPathElement represents a path element for GPU processing.
// Must match PathElement in flatten.wgsl.
type GPUPathElement struct {
	Verb       uint32 // Path verb type (0=MoveTo, 1=LineTo, 2=QuadTo, 3=CubicTo, 4=Close)
	PointStart uint32 // Start index in points array
	PointCount uint32 // Number of points for this element
	Padding    uint32
}

// GPUAffineTransform represents an affine transform for GPU.
// Must match AffineTransform in flatten.wgsl.
// Matrix layout (column-major):
// | a c e |
// | b d f |
// | 0 0 1 |
type GPUAffineTransform struct {
	A        float32 // Scale X
	B        float32 // Shear Y
	C        float32 // Shear X
	D        float32 // Scale Y
	E        float32 // Translate X
	F        float32 // Translate Y
	Padding1 float32
	Padding2 float32
}

// GPUFlattenConfig contains GPU flattening configuration.
// Must match FlattenConfig in flatten.wgsl.
type GPUFlattenConfig struct {
	ElementCount   uint32  // Number of path elements
	Tolerance      float32 // Flattening tolerance
	MaxSegments    uint32  // Maximum total segments
	TileSize       uint32  // Tile size in pixels
	ViewportWidth  uint32  // Viewport width
	ViewportHeight uint32  // Viewport height
	Padding1       uint32
	Padding2       uint32
}

// GPUSegmentCount holds segment count per path element.
// Must match SegmentCount in flatten.wgsl.
type GPUSegmentCount struct {
	Count    uint32 // Number of segments for this element
	Offset   uint32 // Prefix sum offset
	Padding1 uint32
	Padding2 uint32
}

// GPUCursorState tracks the cursor position per path element.
// Must match CursorState in flatten.wgsl.
type GPUCursorState struct {
	CurX   float32 // Current cursor X
	CurY   float32 // Current cursor Y
	StartX float32 // Subpath start X (for Close)
	StartY float32 // Subpath start Y (for Close)
}

// GPUFlattenRasterizer performs curve flattening on the GPU.
// It converts Bezier curves to monotonic line segments using Wang's formula.
//
// Note: Phase 6.3 implementation. Full GPU dispatch requires additional
// cursor tracking infrastructure. Currently provides CPU fallback using
// the existing flatten.go algorithms.
type GPUFlattenRasterizer struct {
	mu sync.Mutex

	device hal.Device
	queue  hal.Queue

	// Compute pipelines
	preparePipeline      hal.ComputePipeline
	flattenPipeline      hal.ComputePipeline
	clearCounterPipeline hal.ComputePipeline

	// Shader module (cached)
	shaderModule hal.ShaderModule

	// Pipeline layout and bind group layouts
	pipelineLayout   hal.PipelineLayout
	inputBindLayout  hal.BindGroupLayout
	outputBindLayout hal.BindGroupLayout

	// Compiled SPIR-V (cached for verification)
	spirvCode []uint32

	// Configuration
	maxPaths    int
	maxSegments int
	tolerance   float32

	// State
	initialized bool
	shaderReady bool

	// Reusable flatten context for CPU fallback
	flattenCtx *FlattenContext
}

// NewGPUFlattenRasterizer creates a new GPU flatten rasterizer.
// maxPaths: Maximum number of path elements to process
// maxSegments: Maximum number of output segments
func NewGPUFlattenRasterizer(device hal.Device, queue hal.Queue, maxPaths, maxSegments int) (*GPUFlattenRasterizer, error) {
	if device == nil || queue == nil {
		return nil, fmt.Errorf("gpu_flatten: device and queue are required")
	}

	if maxPaths <= 0 {
		maxPaths = 1024
	}
	if maxSegments <= 0 {
		maxSegments = maxPaths * FlattenMaxSegmentsPerCurve
	}

	r := &GPUFlattenRasterizer{
		device:      device,
		queue:       queue,
		maxPaths:    maxPaths,
		maxSegments: maxSegments,
		tolerance:   FlattenTolerance,
		flattenCtx:  NewFlattenContext(),
	}

	if err := r.init(); err != nil {
		r.Destroy()
		return nil, err
	}

	return r, nil
}

// init initializes GPU resources (pipelines, layouts).
func (r *GPUFlattenRasterizer) init() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Compile WGSL to SPIR-V using shared helper
	spirvCode, err := CompileShaderToSPIRV(flattenShaderWGSL)
	if err != nil {
		return fmt.Errorf("gpu_flatten: %w", err)
	}
	r.spirvCode = spirvCode
	r.shaderReady = true

	// Create shader module using shared helper
	shaderModule, err := CreateShaderModule(r.device, "flatten_shader", r.spirvCode)
	if err != nil {
		return fmt.Errorf("gpu_flatten: failed to create shader module: %w", err)
	}
	r.shaderModule = shaderModule

	// Create bind group layouts
	if err := r.createBindGroupLayouts(); err != nil {
		return err
	}

	// Create pipeline layout
	if err := r.createPipelineLayout(); err != nil {
		return err
	}

	// Create compute pipelines
	if err := r.createPipelines(); err != nil {
		return err
	}

	r.initialized = true
	return nil
}

// createBindGroupLayouts creates the bind group layouts for the pipeline.
func (r *GPUFlattenRasterizer) createBindGroupLayouts() error {
	// Input bind group layout (group 0)
	inputLayout, err := r.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "flatten_input_layout",
		Entries: []types.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: types.ShaderStageCompute,
				Buffer: &types.BufferBindingLayout{
					Type:           types.BufferBindingTypeUniform,
					MinBindingSize: 32, // sizeof(FlattenConfig)
				},
			},
			{
				Binding:    1,
				Visibility: types.ShaderStageCompute,
				Buffer: &types.BufferBindingLayout{
					Type:           types.BufferBindingTypeUniform,
					MinBindingSize: 32, // sizeof(AffineTransform)
				},
			},
			{
				Binding:    2,
				Visibility: types.ShaderStageCompute,
				Buffer: &types.BufferBindingLayout{
					Type: types.BufferBindingTypeReadOnlyStorage,
				},
			},
			{
				Binding:    3,
				Visibility: types.ShaderStageCompute,
				Buffer: &types.BufferBindingLayout{
					Type: types.BufferBindingTypeReadOnlyStorage,
				},
			},
			{
				Binding:    4,
				Visibility: types.ShaderStageCompute,
				Buffer: &types.BufferBindingLayout{
					Type: types.BufferBindingTypeReadOnlyStorage,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_flatten: failed to create input bind group layout: %w", err)
	}
	r.inputBindLayout = inputLayout

	// Output bind group layout (group 1)
	outputLayout, err := r.device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "flatten_output_layout",
		Entries: []types.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: types.ShaderStageCompute,
				Buffer: &types.BufferBindingLayout{
					Type: types.BufferBindingTypeStorage,
				},
			},
			{
				Binding:    1,
				Visibility: types.ShaderStageCompute,
				Buffer: &types.BufferBindingLayout{
					Type: types.BufferBindingTypeStorage,
				},
			},
			{
				Binding:    2,
				Visibility: types.ShaderStageCompute,
				Buffer: &types.BufferBindingLayout{
					Type: types.BufferBindingTypeStorage,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_flatten: failed to create output bind group layout: %w", err)
	}
	r.outputBindLayout = outputLayout

	return nil
}

// createPipelineLayout creates the pipeline layout.
func (r *GPUFlattenRasterizer) createPipelineLayout() error {
	layout, err := r.device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "flatten_pipeline_layout",
		BindGroupLayouts: []hal.BindGroupLayout{r.inputBindLayout, r.outputBindLayout},
	})
	if err != nil {
		return fmt.Errorf("gpu_flatten: failed to create pipeline layout: %w", err)
	}
	r.pipelineLayout = layout
	return nil
}

// createPipelines creates the compute pipelines.
func (r *GPUFlattenRasterizer) createPipelines() error {
	// Prepare pipeline (count segments)
	preparePipeline, err := r.device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  "flatten_prepare_pipeline",
		Layout: r.pipelineLayout,
		Compute: hal.ComputeState{
			Module:     r.shaderModule,
			EntryPoint: "cs_flatten_prepare",
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_flatten: failed to create prepare pipeline: %w", err)
	}
	r.preparePipeline = preparePipeline

	// Flatten pipeline (generate segments)
	flattenPipeline, err := r.device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  "flatten_pipeline",
		Layout: r.pipelineLayout,
		Compute: hal.ComputeState{
			Module:     r.shaderModule,
			EntryPoint: "cs_flatten",
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_flatten: failed to create flatten pipeline: %w", err)
	}
	r.flattenPipeline = flattenPipeline

	// Clear counter pipeline
	clearPipeline, err := r.device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  "flatten_clear_pipeline",
		Layout: r.pipelineLayout,
		Compute: hal.ComputeState{
			Module:     r.shaderModule,
			EntryPoint: "cs_clear_counter",
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_flatten: failed to create clear pipeline: %w", err)
	}
	r.clearCounterPipeline = clearPipeline

	return nil
}

// SetTolerance sets the flattening tolerance.
func (r *GPUFlattenRasterizer) SetTolerance(tolerance float32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if tolerance > 0 {
		r.tolerance = tolerance
	}
}

// Tolerance returns the current flattening tolerance.
func (r *GPUFlattenRasterizer) Tolerance() float32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tolerance
}

// Flatten flattens a path to monotonic line segments.
// Uses CPU fallback (matching existing flatten.go algorithm).
//
// Parameters:
//   - path: The input path to flatten
//   - transform: Affine transformation to apply to all points
//   - tolerance: Flattening tolerance (use 0 for default)
//
// Returns a SegmentList containing all flattened line segments.
func (r *GPUFlattenRasterizer) Flatten(path *scene.Path, transform scene.Affine, tolerance float32) (*SegmentList, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		return nil, fmt.Errorf("gpu_flatten: rasterizer not initialized")
	}

	if path == nil || path.IsEmpty() {
		return NewSegmentList(), nil
	}

	if tolerance <= 0 {
		tolerance = r.tolerance
	}

	// Phase 6.3: GPU infrastructure is ready, but cursor tracking needs
	// additional buffer management. For now, use CPU fallback.
	return r.flattenCPU(path, transform, tolerance), nil
}

// flattenCPU performs curve flattening using CPU (mirrors GPU algorithm).
// This is the reference implementation and fallback.
func (r *GPUFlattenRasterizer) flattenCPU(path *scene.Path, transform scene.Affine, tolerance float32) *SegmentList {
	// Use existing FlattenPath function which implements the same algorithm
	return FlattenPath(path, transform, tolerance)
}

// FlattenWithContext flattens a path using a provided context for efficiency.
// This avoids allocating a new SegmentList for each path.
func (r *GPUFlattenRasterizer) FlattenWithContext(
	path *scene.Path,
	transform scene.Affine,
	tolerance float32,
) *SegmentList {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		return NewSegmentList()
	}

	if path == nil || path.IsEmpty() {
		return r.flattenCtx.Segments()
	}

	if tolerance <= 0 {
		tolerance = r.tolerance
	}

	// Reset and reuse context
	r.flattenCtx.Reset()
	r.flattenCtx.FlattenPathTo(path, transform, tolerance)
	return r.flattenCtx.Segments()
}

// EstimateSegmentCount estimates the number of segments for a path.
// Uses Wang's formula to estimate without actually flattening.
func (r *GPUFlattenRasterizer) EstimateSegmentCount(path *scene.Path, transform scene.Affine, tolerance float32) int {
	if path == nil || path.IsEmpty() {
		return 0
	}

	if tolerance <= 0 {
		tolerance = r.tolerance
	}

	count := 0
	pointIdx := 0
	points := path.Points()
	verbs := path.Verbs()

	var curX, curY float32

	for _, verb := range verbs {
		switch verb {
		case scene.VerbMoveTo:
			x, y := points[pointIdx], points[pointIdx+1]
			curX, curY = transform.TransformPoint(x, y)
			pointIdx += 2

		case scene.VerbLineTo:
			count++
			x, y := points[pointIdx], points[pointIdx+1]
			curX, curY = transform.TransformPoint(x, y)
			pointIdx += 2

		case scene.VerbQuadTo:
			cx, cy := points[pointIdx], points[pointIdx+1]
			x, y := points[pointIdx+2], points[pointIdx+3]
			tcx, tcy := transform.TransformPoint(cx, cy)
			tx, ty := transform.TransformPoint(x, y)
			count += wangQuadratic(curX, curY, tcx, tcy, tx, ty, tolerance)
			curX, curY = tx, ty
			pointIdx += 4

		case scene.VerbCubicTo:
			c1x, c1y := points[pointIdx], points[pointIdx+1]
			c2x, c2y := points[pointIdx+2], points[pointIdx+3]
			x, y := points[pointIdx+4], points[pointIdx+5]
			tc1x, tc1y := transform.TransformPoint(c1x, c1y)
			tc2x, tc2y := transform.TransformPoint(c2x, c2y)
			tx, ty := transform.TransformPoint(x, y)
			count += wangCubic(curX, curY, tc1x, tc1y, tc2x, tc2y, tx, ty, tolerance)
			curX, curY = tx, ty
			pointIdx += 6

		case scene.VerbClose:
			count++ // Close line
		}
	}

	return count
}

// wangQuadratic estimates segment count for quadratic Bezier using Wang's formula.
func wangQuadratic(x0, y0, cx, cy, x1, y1 float32, tolerance float32) int {
	// Second derivative (constant for quadratic)
	d1x := x0 - 2*cx + x1
	d1y := y0 - 2*cy + y1

	// Maximum deviation
	maxD := float32(math.Sqrt(float64(d1x*d1x + d1y*d1y)))

	if maxD <= flattenEpsilon {
		return 1
	}

	// Wang's formula: n = sqrt(max_d / (8 * tolerance))
	n := float32(math.Sqrt(float64(maxD / (8.0 * tolerance))))

	count := int(math.Ceil(float64(n)))
	if count < 1 {
		return 1
	}
	if count > FlattenMaxSegmentsPerCurve {
		return FlattenMaxSegmentsPerCurve
	}
	return count
}

// wangCubic estimates segment count for cubic Bezier using Wang's formula.
func wangCubic(x0, y0, c1x, c1y, c2x, c2y, x1, y1 float32, tolerance float32) int {
	// Approximate second derivative maximum
	d1x := 3.0 * (c1x - 2.0*c2x + x1)
	d1y := 3.0 * (c1y - 2.0*c2y + y1)
	d2x := 3.0 * (x0 - 2.0*c1x + c2x)
	d2y := 3.0 * (y0 - 2.0*c1y + c2y)

	max1 := float32(math.Sqrt(float64(d1x*d1x + d1y*d1y)))
	max2 := float32(math.Sqrt(float64(d2x*d2x + d2y*d2y)))

	maxD := max1
	if max2 > maxD {
		maxD = max2
	}

	if maxD <= flattenEpsilon {
		return 1
	}

	// Wang's formula for cubic: n = (3/4) * sqrt(sqrt(max_d / tolerance))
	n := 0.75 * float32(math.Sqrt(math.Sqrt(float64(maxD/tolerance))))

	count := int(math.Ceil(float64(n)))
	if count < 1 {
		return 1
	}
	if count > FlattenMaxSegmentsPerCurve {
		return FlattenMaxSegmentsPerCurve
	}
	return count
}

// ConvertPathToGPU converts a scene.Path to GPU buffer format.
// Returns elements and points arrays suitable for GPU upload.
func (r *GPUFlattenRasterizer) ConvertPathToGPU(path *scene.Path) ([]GPUPathElement, []float32) {
	if path == nil || path.IsEmpty() {
		return nil, nil
	}

	verbs := path.Verbs()
	points := path.Points()

	elements := make([]GPUPathElement, len(verbs))
	pointIdx := uint32(0)

	for i, verb := range verbs {
		elem := GPUPathElement{
			Verb:       uint32(verb),
			PointStart: pointIdx,
		}

		switch verb {
		case scene.VerbMoveTo, scene.VerbLineTo:
			elem.PointCount = 2
			pointIdx += 2
		case scene.VerbQuadTo:
			elem.PointCount = 4
			pointIdx += 4
		case scene.VerbCubicTo:
			elem.PointCount = 6
			pointIdx += 6
		case scene.VerbClose:
			elem.PointCount = 0
		}

		elements[i] = elem
	}

	// Copy points (already in correct format)
	gpuPoints := make([]float32, len(points))
	copy(gpuPoints, points)

	return elements, gpuPoints
}

// ConvertAffineToGPU converts a scene.Affine to GPU format.
func ConvertAffineToGPU(t scene.Affine) GPUAffineTransform {
	return GPUAffineTransform{
		A: t.A,
		B: t.B,
		C: t.C,
		D: t.D,
		E: t.E,
		F: t.F,
	}
}

// IsInitialized returns whether the rasterizer is initialized.
func (r *GPUFlattenRasterizer) IsInitialized() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.initialized
}

// IsShaderReady returns whether the shader compiled successfully.
func (r *GPUFlattenRasterizer) IsShaderReady() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shaderReady
}

// SPIRVCode returns the compiled SPIR-V code (for debugging/verification).
func (r *GPUFlattenRasterizer) SPIRVCode() []uint32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.spirvCode
}

// MaxPaths returns the maximum number of path elements.
func (r *GPUFlattenRasterizer) MaxPaths() int {
	return r.maxPaths
}

// MaxSegments returns the maximum number of output segments.
func (r *GPUFlattenRasterizer) MaxSegments() int {
	return r.maxSegments
}

// Destroy releases all GPU resources.
func (r *GPUFlattenRasterizer) Destroy() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.device == nil {
		return
	}

	// Destroy pipelines
	if r.preparePipeline != nil {
		r.device.DestroyComputePipeline(r.preparePipeline)
		r.preparePipeline = nil
	}
	if r.flattenPipeline != nil {
		r.device.DestroyComputePipeline(r.flattenPipeline)
		r.flattenPipeline = nil
	}
	if r.clearCounterPipeline != nil {
		r.device.DestroyComputePipeline(r.clearCounterPipeline)
		r.clearCounterPipeline = nil
	}

	// Destroy pipeline layout
	if r.pipelineLayout != nil {
		r.device.DestroyPipelineLayout(r.pipelineLayout)
		r.pipelineLayout = nil
	}

	// Destroy bind group layouts
	if r.inputBindLayout != nil {
		r.device.DestroyBindGroupLayout(r.inputBindLayout)
		r.inputBindLayout = nil
	}
	if r.outputBindLayout != nil {
		r.device.DestroyBindGroupLayout(r.outputBindLayout)
		r.outputBindLayout = nil
	}

	// Destroy shader module
	if r.shaderModule != nil {
		r.device.DestroyShaderModule(r.shaderModule)
		r.shaderModule = nil
	}

	r.initialized = false
}

// Byte serialization helpers for GPU buffer upload

func flattenConfigToBytes(cfg GPUFlattenConfig) []byte {
	buf := make([]byte, 32)
	writeUint32(buf, 0, cfg.ElementCount)
	writeFloat32(buf, 4, cfg.Tolerance)
	writeUint32(buf, 8, cfg.MaxSegments)
	writeUint32(buf, 12, cfg.TileSize)
	writeUint32(buf, 16, cfg.ViewportWidth)
	writeUint32(buf, 20, cfg.ViewportHeight)
	writeUint32(buf, 24, cfg.Padding1)
	writeUint32(buf, 28, cfg.Padding2)
	return buf
}

func affineTransformToBytes(t GPUAffineTransform) []byte {
	buf := make([]byte, 32)
	writeFloat32(buf, 0, t.A)
	writeFloat32(buf, 4, t.B)
	writeFloat32(buf, 8, t.C)
	writeFloat32(buf, 12, t.D)
	writeFloat32(buf, 16, t.E)
	writeFloat32(buf, 20, t.F)
	writeFloat32(buf, 24, t.Padding1)
	writeFloat32(buf, 28, t.Padding2)
	return buf
}

func pathElementsToBytes(elements []GPUPathElement) []byte {
	buf := make([]byte, len(elements)*16)
	for i, elem := range elements {
		off := i * 16
		writeUint32(buf, off+0, elem.Verb)
		writeUint32(buf, off+4, elem.PointStart)
		writeUint32(buf, off+8, elem.PointCount)
		writeUint32(buf, off+12, elem.Padding)
	}
	return buf
}

func pointsToBytes(points []float32) []byte {
	buf := make([]byte, len(points)*4)
	for i, p := range points {
		writeFloat32(buf, i*4, p)
	}
	return buf
}

func segmentCountsToBytes(counts []GPUSegmentCount) []byte {
	buf := make([]byte, len(counts)*16)
	for i, c := range counts {
		off := i * 16
		writeUint32(buf, off+0, c.Count)
		writeUint32(buf, off+4, c.Offset)
		writeUint32(buf, off+8, c.Padding1)
		writeUint32(buf, off+12, c.Padding2)
	}
	return buf
}

func cursorStatesToBytes(states []GPUCursorState) []byte {
	buf := make([]byte, len(states)*16)
	for i, s := range states {
		off := i * 16
		writeFloat32(buf, off+0, s.CurX)
		writeFloat32(buf, off+4, s.CurY)
		writeFloat32(buf, off+8, s.StartX)
		writeFloat32(buf, off+12, s.StartY)
	}
	return buf
}

// ComputeCursorStates computes cursor states for each path element.
// This tracks the cursor position (curX, curY) and subpath start (startX, startY)
// for each element, which is needed by the GPU shader.
func (r *GPUFlattenRasterizer) ComputeCursorStates(path *scene.Path) []GPUCursorState {
	if path == nil || path.IsEmpty() {
		return nil
	}

	verbs := path.Verbs()
	points := path.Points()

	states := make([]GPUCursorState, len(verbs))
	var curX, curY float32
	var startX, startY float32
	pointIdx := 0

	for i, verb := range verbs {
		// Record current state BEFORE this verb executes
		states[i] = GPUCursorState{
			CurX:   curX,
			CurY:   curY,
			StartX: startX,
			StartY: startY,
		}

		// Update cursor based on verb
		switch verb {
		case scene.VerbMoveTo:
			x, y := points[pointIdx], points[pointIdx+1]
			curX, curY = x, y
			startX, startY = x, y
			pointIdx += 2

		case scene.VerbLineTo:
			curX, curY = points[pointIdx], points[pointIdx+1]
			pointIdx += 2

		case scene.VerbQuadTo:
			curX, curY = points[pointIdx+2], points[pointIdx+3]
			pointIdx += 4

		case scene.VerbCubicTo:
			curX, curY = points[pointIdx+4], points[pointIdx+5]
			pointIdx += 6

		case scene.VerbClose:
			curX, curY = startX, startY
		}
	}

	return states
}
