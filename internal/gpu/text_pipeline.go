package gpu

import (
	_ "embed"
	"errors"
	"fmt"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text/msdf"
	"github.com/gogpu/wgpu/core"
)

// Embedded MSDF text shader source.
//
//go:embed shaders/msdf_text.wgsl
var msdfTextShaderSource string

// Text rendering errors.
var (
	// ErrNilTextPipeline is returned when operating on a nil pipeline.
	ErrNilTextPipeline = errors.New("wgpu: text pipeline is nil")

	// ErrTextPipelineNotInitialized is returned when pipeline is not initialized.
	ErrTextPipelineNotInitialized = errors.New("wgpu: text pipeline not initialized")

	// ErrNoQuadsToRender is returned when RenderText is called with empty quads.
	ErrNoQuadsToRender = errors.New("wgpu: no quads to render")

	// ErrQuadBufferOverflow is returned when too many quads are submitted.
	ErrQuadBufferOverflow = errors.New("wgpu: quad buffer overflow")

	// ErrInvalidAtlasIndex is returned when referencing invalid atlas.
	ErrInvalidAtlasIndex = errors.New("wgpu: invalid atlas index")
)

// TextPipeline handles GPU-accelerated MSDF text rendering.
// It manages the render pipeline, bind groups, and vertex buffers
// for rendering text using multi-channel signed distance fields.
//
// TextPipeline is safe for concurrent use after initialization.
type TextPipeline struct {
	mu sync.RWMutex

	// GPU device reference
	device core.DeviceID

	// Shader module for MSDF text rendering
	shaderModule ShaderModuleID

	// Render pipeline for text
	pipeline StubPipelineID

	// Bind group layout for text uniforms, atlas texture, and sampler
	bindGroupLayout StubBindGroupLayoutID

	// Uniform buffer for text parameters
	uniformBuffer StubBufferID

	// Vertex buffer for text quads (dynamic, grows as needed)
	vertexBuffer StubBufferID
	vertexCap    int // Current capacity in quads

	// Index buffer for quads (static, shared pattern)
	indexBuffer StubBufferID

	// Cached bind groups per atlas texture
	atlasBindGroups map[int]StubBindGroupID

	// Default sampler for MSDF textures
	sampler StubSamplerID

	// Configuration
	config TextPipelineConfig

	// State
	initialized bool
}

// TextPipelineConfig holds configuration for the text pipeline.
type TextPipelineConfig struct {
	// InitialQuadCapacity is the initial vertex buffer capacity in quads.
	// Default: 256
	InitialQuadCapacity int

	// MaxQuadCapacity is the maximum number of quads per draw call.
	// Default: 16384
	MaxQuadCapacity int

	// DefaultPxRange is the default MSDF pixel range.
	// Default: 4.0
	DefaultPxRange float32
}

// DefaultTextPipelineConfig returns default configuration.
func DefaultTextPipelineConfig() TextPipelineConfig {
	return TextPipelineConfig{
		InitialQuadCapacity: 256,
		MaxQuadCapacity:     16384,
		DefaultPxRange:      4.0,
	}
}

// StubSamplerID is a placeholder for actual wgpu SamplerID.
type StubSamplerID uint64

// TextQuad represents a single glyph quad for rendering.
// Each glyph is rendered as a textured quad with position and UV coordinates.
type TextQuad struct {
	// Position of quad corners in screen/clip space
	X0, Y0, X1, Y1 float32

	// UV coordinates in MSDF atlas [0, 1]
	U0, V0, U1, V1 float32
}

// TextVertex represents a single vertex for text rendering.
// Matches the VertexInput struct in msdf_text.wgsl.
type TextVertex struct {
	// Position in local/screen space
	X, Y float32

	// UV coordinates in atlas
	U, V float32
}

// TextUniforms represents the uniform buffer for text shaders.
// Matches the TextUniforms struct in msdf_text.wgsl.
type TextUniforms struct {
	// Transform matrix (4x4 for alignment, row-major)
	// Maps local coordinates to clip space [-1, 1]
	Transform [16]float32

	// Text color (RGBA, premultiplied alpha)
	Color [4]float32

	// MSDF parameters:
	// [0]: px_range (distance range in pixels)
	// [1]: atlas_size (texture size)
	// [2]: outline_width (for outline effect)
	// [3]: reserved
	MSDFParams [4]float32
}

// NewTextPipeline creates a new text rendering pipeline.
// The pipeline must be initialized before use.
func NewTextPipeline(device core.DeviceID, config TextPipelineConfig) (*TextPipeline, error) {
	if config.InitialQuadCapacity <= 0 {
		config.InitialQuadCapacity = DefaultTextPipelineConfig().InitialQuadCapacity
	}
	if config.MaxQuadCapacity <= 0 {
		config.MaxQuadCapacity = DefaultTextPipelineConfig().MaxQuadCapacity
	}
	if config.DefaultPxRange <= 0 {
		config.DefaultPxRange = DefaultTextPipelineConfig().DefaultPxRange
	}

	return &TextPipeline{
		device:          device,
		config:          config,
		atlasBindGroups: make(map[int]StubBindGroupID),
	}, nil
}

// NewTextPipelineDefault creates a text pipeline with default configuration.
func NewTextPipelineDefault(device core.DeviceID) (*TextPipeline, error) {
	return NewTextPipeline(device, DefaultTextPipelineConfig())
}

// Init initializes the text pipeline, compiling shaders and creating GPU resources.
func (p *TextPipeline) Init() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return nil
	}

	// Validate shader source
	if msdfTextShaderSource == "" {
		return errors.New("wgpu: MSDF text shader source is empty")
	}

	// Create shader module (stub)
	// TODO: When wgpu shader compilation is available:
	// p.shaderModule, err = core.CreateShaderModule(p.device, msdfTextShaderSource)
	p.shaderModule = ShaderModuleID(100) // Stub ID for text shader

	// Create bind group layout
	// Layout:
	//   Binding 0: TextUniforms (uniform buffer)
	//   Binding 1: MSDF atlas texture (texture_2d)
	//   Binding 2: Sampler
	p.bindGroupLayout = StubBindGroupLayoutID(100)

	// Create render pipeline
	// TODO: When wgpu is ready, create actual pipeline with:
	// - Vertex layout: position (vec2), tex_coord (vec2)
	// - Blend state: premultiplied alpha (One, OneMinusSrcAlpha)
	// - Depth: disabled for 2D text
	p.pipeline = StubPipelineID(200)

	// Create uniform buffer
	// Size: 16 floats (transform) + 4 floats (color) + 4 floats (msdf_params) = 96 bytes
	p.uniformBuffer = StubBufferID(100)

	// Create vertex buffer with initial capacity
	p.vertexCap = p.config.InitialQuadCapacity
	p.vertexBuffer = StubBufferID(101)

	// Create index buffer for quads
	// Each quad uses 6 indices: 0,1,2, 2,3,0
	p.indexBuffer = StubBufferID(102)

	// Create sampler for MSDF textures
	// Uses linear filtering for smooth distance interpolation
	p.sampler = StubSamplerID(1)

	p.initialized = true
	return nil
}

// IsInitialized returns true if the pipeline has been initialized.
func (p *TextPipeline) IsInitialized() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.initialized
}

// RenderText renders text quads using the specified atlas.
// All quads are rendered in a single draw call for efficiency.
//
// Parameters:
//   - pass: The render pass to record commands into
//   - quads: Text quads to render (position and UV for each glyph)
//   - atlasIndex: Index of the MSDF atlas texture to use
//   - color: Text color (RGBA, will be premultiplied)
//   - transform: 2D affine transform matrix (gg.Matrix)
//
// Note: This is a stub implementation that validates inputs and prepares
// vertex data. Actual GPU rendering will be implemented when wgpu is ready.
func (p *TextPipeline) RenderText(
	pass *RenderPass,
	quads []TextQuad,
	atlasIndex int,
	color gg.RGBA,
	transform gg.Matrix,
) error {
	if !p.initialized {
		return ErrTextPipelineNotInitialized
	}

	if len(quads) == 0 {
		return ErrNoQuadsToRender
	}

	if len(quads) > p.config.MaxQuadCapacity {
		return fmt.Errorf("%w: %d quads exceeds max %d",
			ErrQuadBufferOverflow, len(quads), p.config.MaxQuadCapacity)
	}

	if atlasIndex < 0 {
		return ErrInvalidAtlasIndex
	}

	// Prepare vertex data
	vertices := quadsToVertices(quads)

	// Prepare uniforms
	uniforms := p.prepareUniforms(color, transform, p.config.DefaultPxRange)

	// Grow vertex buffer if needed
	if len(quads) > p.vertexCap {
		if err := p.growVertexBuffer(len(quads)); err != nil {
			return err
		}
	}

	// TODO: When wgpu is ready:
	// 1. Update uniform buffer with uniforms
	// 2. Upload vertex data to vertex buffer
	// 3. Get or create bind group for atlas texture
	// 4. Set pipeline
	// 5. Set bind group
	// 6. Set vertex buffer
	// 7. Set index buffer
	// 8. Draw indexed

	// For now, store the data for testing/validation
	_ = vertices
	_ = uniforms
	_ = pass

	return nil
}

// RenderTextBatch renders multiple text batches efficiently.
// Each batch can have different color and transform but shares the same atlas.
func (p *TextPipeline) RenderTextBatch(
	pass *RenderPass,
	batches []TextBatch,
	atlasIndex int,
) error {
	if !p.initialized {
		return ErrTextPipelineNotInitialized
	}

	if atlasIndex < 0 {
		return ErrInvalidAtlasIndex
	}

	for i, batch := range batches {
		if err := p.RenderText(pass, batch.Quads, atlasIndex, batch.Color, batch.Transform); err != nil {
			return fmt.Errorf("batch %d: %w", i, err)
		}
	}

	return nil
}

// TextBatch represents a batch of text quads with shared rendering parameters.
type TextBatch struct {
	Quads     []TextQuad
	Color     gg.RGBA
	Transform gg.Matrix
}

// prepareUniforms creates TextUniforms from rendering parameters.
func (p *TextPipeline) prepareUniforms(color gg.RGBA, transform gg.Matrix, pxRange float32) TextUniforms {
	// Premultiply color
	premul := color.Premultiply()

	uniforms := TextUniforms{
		Color: [4]float32{
			float32(premul.R),
			float32(premul.G),
			float32(premul.B),
			float32(premul.A),
		},
		MSDFParams: [4]float32{pxRange, 0, 0, 0},
	}

	// Convert 2D affine transform (2x3) to 4x4 matrix (row-major)
	// Input: a b c
	//        d e f
	// Output: a b 0 c
	//         d e 0 f
	//         0 0 1 0
	//         0 0 0 1
	uniforms.Transform = [16]float32{
		float32(transform.A), float32(transform.B), 0, float32(transform.C),
		float32(transform.D), float32(transform.E), 0, float32(transform.F),
		0, 0, 1, 0,
		0, 0, 0, 1,
	}

	return uniforms
}

// growVertexBuffer increases vertex buffer capacity.
func (p *TextPipeline) growVertexBuffer(needed int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double capacity until sufficient
	newCap := p.vertexCap
	for newCap < needed {
		newCap *= 2
	}

	// Clamp to max
	if newCap > p.config.MaxQuadCapacity {
		newCap = p.config.MaxQuadCapacity
	}

	// TODO: When wgpu is ready:
	// 1. Create new larger buffer
	// 2. Copy existing data if needed
	// 3. Release old buffer

	p.vertexCap = newCap
	return nil
}

// GetOrCreateAtlasBindGroup gets or creates a bind group for an atlas texture.
func (p *TextPipeline) GetOrCreateAtlasBindGroup(atlasIndex int, atlasTexture *GPUTexture) (StubBindGroupID, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return 0, ErrTextPipelineNotInitialized
	}

	if bg, ok := p.atlasBindGroups[atlasIndex]; ok {
		return bg, nil
	}

	// Create new bind group
	// TODO: When wgpu is ready:
	// entries := []gputypes.BindGroupEntry{
	//     {Binding: 0, Buffer: p.uniformBuffer},
	//     {Binding: 1, TextureView: atlasTexture.ViewID()},
	//     {Binding: 2, Sampler: p.sampler},
	// }
	// bg := core.CreateBindGroup(p.device, p.bindGroupLayout, entries)

	//nolint:gosec // atlasIndex is validated to be non-negative above
	bg := StubBindGroupID(200 + atlasIndex)
	p.atlasBindGroups[atlasIndex] = bg

	return bg, nil
}

// InvalidateAtlasBindGroup removes a cached bind group for an atlas.
// Call this when an atlas texture is updated.
func (p *TextPipeline) InvalidateAtlasBindGroup(atlasIndex int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// TODO: When wgpu is ready, release the bind group
	delete(p.atlasBindGroups, atlasIndex)
}

// InvalidateAllAtlasBindGroups removes all cached bind groups.
func (p *TextPipeline) InvalidateAllAtlasBindGroups() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// TODO: When wgpu is ready, release all bind groups
	p.atlasBindGroups = make(map[int]StubBindGroupID)
}

// Config returns the pipeline configuration.
func (p *TextPipeline) Config() TextPipelineConfig {
	return p.config
}

// Close releases all pipeline resources.
func (p *TextPipeline) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return
	}

	// TODO: When wgpu is ready:
	// - Release shader module
	// - Release pipeline
	// - Release bind group layout
	// - Release uniform buffer
	// - Release vertex buffer
	// - Release index buffer
	// - Release sampler
	// - Release all bind groups

	p.shaderModule = 0
	p.pipeline = 0
	p.bindGroupLayout = 0
	p.uniformBuffer = 0
	p.vertexBuffer = 0
	p.indexBuffer = 0
	p.sampler = 0
	p.atlasBindGroups = nil
	p.initialized = false
}

// quadsToVertices converts TextQuads to vertex array.
// Each quad becomes 4 vertices (for indexed rendering).
func quadsToVertices(quads []TextQuad) []TextVertex {
	vertices := make([]TextVertex, len(quads)*4)

	for i, q := range quads {
		base := i * 4

		// Vertex 0: bottom-left
		vertices[base+0] = TextVertex{X: q.X0, Y: q.Y0, U: q.U0, V: q.V0}
		// Vertex 1: bottom-right
		vertices[base+1] = TextVertex{X: q.X1, Y: q.Y0, U: q.U1, V: q.V0}
		// Vertex 2: top-right
		vertices[base+2] = TextVertex{X: q.X1, Y: q.Y1, U: q.U1, V: q.V1}
		// Vertex 3: top-left
		vertices[base+3] = TextVertex{X: q.X0, Y: q.Y1, U: q.U0, V: q.V1}
	}

	return vertices
}

// generateQuadIndices generates index buffer data for a given number of quads.
// Uses the pattern: 0,1,2, 2,3,0 for each quad (two triangles).
func generateQuadIndices(numQuads int) []uint16 {
	indices := make([]uint16, numQuads*6)

	for i := 0; i < numQuads; i++ {
		base := i * 6
		vertex := uint16(i * 4) //nolint:gosec // numQuads is bounded by MaxQuadCapacity

		// First triangle: 0, 1, 2
		indices[base+0] = vertex + 0
		indices[base+1] = vertex + 1
		indices[base+2] = vertex + 2

		// Second triangle: 2, 3, 0
		indices[base+3] = vertex + 2
		indices[base+4] = vertex + 3
		indices[base+5] = vertex + 0
	}

	return indices
}

// GetMSDFTextShaderSource returns the WGSL source for the MSDF text shader.
func GetMSDFTextShaderSource() string {
	return msdfTextShaderSource
}

// TextRenderer provides a higher-level API for rendering text.
// It combines TextPipeline with AtlasManager for convenient text rendering.
type TextRenderer struct {
	mu sync.RWMutex

	// GPU resources
	backend  *Backend
	pipeline *TextPipeline

	// Atlas management
	atlasManager *msdf.AtlasManager

	// Cached atlas textures
	atlasTextures []*GPUTexture

	// State
	initialized bool
}

// TextRendererConfig holds configuration for TextRenderer.
type TextRendererConfig struct {
	// PipelineConfig for the underlying text pipeline.
	PipelineConfig TextPipelineConfig

	// AtlasConfig for the MSDF atlas manager.
	AtlasConfig msdf.AtlasConfig
}

// DefaultTextRendererConfig returns default configuration.
func DefaultTextRendererConfig() TextRendererConfig {
	return TextRendererConfig{
		PipelineConfig: DefaultTextPipelineConfig(),
		AtlasConfig:    msdf.DefaultAtlasConfig(),
	}
}

// NewTextRenderer creates a new text renderer.
func NewTextRenderer(backend *Backend, config TextRendererConfig) (*TextRenderer, error) {
	if backend == nil {
		return nil, ErrNilBackend
	}

	atlasManager, err := msdf.NewAtlasManager(config.AtlasConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create atlas manager: %w", err)
	}

	pipeline, err := NewTextPipeline(backend.Device(), config.PipelineConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create text pipeline: %w", err)
	}

	return &TextRenderer{
		backend:       backend,
		pipeline:      pipeline,
		atlasManager:  atlasManager,
		atlasTextures: make([]*GPUTexture, 0),
	}, nil
}

// Init initializes the text renderer.
func (r *TextRenderer) Init() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.initialized {
		return nil
	}

	if err := r.pipeline.Init(); err != nil {
		return err
	}

	r.initialized = true
	return nil
}

// SyncAtlases uploads dirty atlases to GPU.
func (r *TextRenderer) SyncAtlases() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		return ErrTextPipelineNotInitialized
	}

	dirtyIndices := r.atlasManager.DirtyAtlases()
	for _, idx := range dirtyIndices {
		atlas := r.atlasManager.GetAtlas(idx)
		if atlas == nil {
			continue
		}

		// Ensure we have enough texture slots
		for len(r.atlasTextures) <= idx {
			r.atlasTextures = append(r.atlasTextures, nil)
		}

		// Create or update texture
		if r.atlasTextures[idx] == nil {
			// Create new texture
			tex, err := CreateTexture(r.backend, TextureConfig{
				Width:  atlas.Size,
				Height: atlas.Size,
				Format: TextureFormatRGBA8, // Note: MSDF uses RGB, but we store as RGBA for compatibility
				Label:  fmt.Sprintf("msdf-atlas-%d", idx),
			})
			if err != nil {
				return fmt.Errorf("failed to create atlas texture %d: %w", idx, err)
			}
			r.atlasTextures[idx] = tex
		}

		// TODO: Upload atlas.Data to texture
		// This requires converting RGB to RGBA format

		// Invalidate bind group since texture was updated
		r.pipeline.InvalidateAtlasBindGroup(idx)

		// Mark atlas as clean
		r.atlasManager.MarkClean(idx)
	}

	return nil
}

// AtlasManager returns the underlying atlas manager.
func (r *TextRenderer) AtlasManager() *msdf.AtlasManager {
	return r.atlasManager
}

// Pipeline returns the underlying text pipeline.
func (r *TextRenderer) Pipeline() *TextPipeline {
	return r.pipeline
}

// Close releases all renderer resources.
func (r *TextRenderer) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		return
	}

	// Close textures
	for _, tex := range r.atlasTextures {
		if tex != nil {
			tex.Close()
		}
	}
	r.atlasTextures = nil

	// Close pipeline
	if r.pipeline != nil {
		r.pipeline.Close()
	}

	// Clear atlas manager
	if r.atlasManager != nil {
		r.atlasManager.Clear()
	}

	r.initialized = false
}

// ErrNilBackend is returned when backend is nil.
var ErrNilBackend = errors.New("wgpu: backend is nil")
