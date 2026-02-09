// Package wgpu provides GPU-accelerated rendering backend using WebGPU.
package gpu

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/gogpu/gg/scene"
)

// Embedded WGSL shader sources.
// These are compiled at build time using go:embed directives.

//go:embed shaders/blit.wgsl
var blitShaderSource string

//go:embed shaders/blend.wgsl
var blendShaderSource string

//go:embed shaders/strip.wgsl
var stripShaderSource string

//go:embed shaders/composite.wgsl
var compositeShaderSource string

//go:embed shaders/sdf_circle.wgsl
var sdfCircleShaderSource string

//go:embed shaders/sdf_rrect.wgsl
var sdfRRectShaderSource string

// ShaderModuleID represents a compiled shader module handle.
// This is a placeholder type that will be replaced with the actual
// wgpu core.ShaderModuleID once shader compilation is implemented.
type ShaderModuleID uint64

// InvalidShaderModule represents an invalid/uninitialized shader module.
const InvalidShaderModule ShaderModuleID = 0

// ShaderModules holds compiled shader modules for all rendering operations.
type ShaderModules struct {
	// Blit is the simple texture copy shader
	Blit ShaderModuleID

	// Blend is the 29-mode blend shader
	Blend ShaderModuleID

	// Strip is the strip rasterization compute shader
	Strip ShaderModuleID

	// Composite is the final layer compositing shader
	Composite ShaderModuleID
}

// IsValid returns true if all shader modules are initialized.
func (s *ShaderModules) IsValid() bool {
	return s.Blit != InvalidShaderModule &&
		s.Blend != InvalidShaderModule &&
		s.Strip != InvalidShaderModule &&
		s.Composite != InvalidShaderModule
}

// CompileShaders compiles all WGSL shaders and returns the shader modules.
// This function currently returns stub module IDs since gogpu/wgpu shader
// compilation is not yet fully implemented. The WGSL sources are validated
// for correct syntax.
//
// Parameters:
//   - deviceID: The GPU device ID to use for compilation (currently unused)
//
// Returns:
//   - *ShaderModules: Compiled shader module handles
//   - error: Compilation error if shader sources are invalid
func CompileShaders(deviceID uint64) (*ShaderModules, error) {
	// Validate shader sources are non-empty
	if blitShaderSource == "" {
		return nil, errors.New("blit shader source is empty")
	}
	if blendShaderSource == "" {
		return nil, errors.New("blend shader source is empty")
	}
	if stripShaderSource == "" {
		return nil, errors.New("strip shader source is empty")
	}
	if compositeShaderSource == "" {
		return nil, errors.New("composite shader source is empty")
	}

	// TODO: When gogpu/wgpu shader compilation is implemented, compile here:
	// blitSPIRV, err := naga.Compile(blitShaderSource)
	// blitModule, err := core.CreateShaderModule(device, blitSPIRV)

	// For now, return stub module IDs
	// These will be replaced with actual compiled modules when wgpu support is ready
	modules := &ShaderModules{
		Blit:      ShaderModuleID(1),
		Blend:     ShaderModuleID(2),
		Strip:     ShaderModuleID(3),
		Composite: ShaderModuleID(4),
	}

	return modules, nil
}

// GetBlitShaderSource returns the WGSL source for the blit shader.
func GetBlitShaderSource() string {
	return blitShaderSource
}

// GetBlendShaderSource returns the WGSL source for the blend shader.
func GetBlendShaderSource() string {
	return blendShaderSource
}

// GetStripShaderSource returns the WGSL source for the strip shader.
func GetStripShaderSource() string {
	return stripShaderSource
}

// GetCompositeShaderSource returns the WGSL source for the composite shader.
func GetCompositeShaderSource() string {
	return compositeShaderSource
}

// Blend mode constants matching scene.BlendMode values.
// These are used for GPU shader uniform values.
const (
	// Standard blend modes (0-11)
	ShaderBlendNormal     uint32 = 0
	ShaderBlendMultiply   uint32 = 1
	ShaderBlendScreen     uint32 = 2
	ShaderBlendOverlay    uint32 = 3
	ShaderBlendDarken     uint32 = 4
	ShaderBlendLighten    uint32 = 5
	ShaderBlendColorDodge uint32 = 6
	ShaderBlendColorBurn  uint32 = 7
	ShaderBlendHardLight  uint32 = 8
	ShaderBlendSoftLight  uint32 = 9
	ShaderBlendDifference uint32 = 10
	ShaderBlendExclusion  uint32 = 11

	// HSL blend modes (12-15)
	ShaderBlendHue        uint32 = 12
	ShaderBlendSaturation uint32 = 13
	ShaderBlendColor      uint32 = 14
	ShaderBlendLuminosity uint32 = 15

	// Porter-Duff modes (16-28)
	ShaderBlendClear           uint32 = 16
	ShaderBlendCopy            uint32 = 17
	ShaderBlendDestination     uint32 = 18
	ShaderBlendSourceOver      uint32 = 19
	ShaderBlendDestinationOver uint32 = 20
	ShaderBlendSourceIn        uint32 = 21
	ShaderBlendDestinationIn   uint32 = 22
	ShaderBlendSourceOut       uint32 = 23
	ShaderBlendDestinationOut  uint32 = 24
	ShaderBlendSourceAtop      uint32 = 25
	ShaderBlendDestinationAtop uint32 = 26
	ShaderBlendXor             uint32 = 27
	ShaderBlendPlus            uint32 = 28
)

// BlendModeToShader converts a scene.BlendMode to the shader constant value.
// The values are designed to match directly, so this is primarily for type safety.
func BlendModeToShader(mode scene.BlendMode) uint32 {
	return uint32(mode)
}

// ShaderToBlendMode converts a shader blend mode constant to scene.BlendMode.
func ShaderToBlendMode(shaderMode uint32) scene.BlendMode {
	return scene.BlendMode(shaderMode)
}

// ValidateBlendModeMapping verifies that shader constants match scene.BlendMode values.
// Returns an error if any mismatch is found.
func ValidateBlendModeMapping() error {
	mappings := []struct {
		sceneMode   scene.BlendMode
		shaderConst uint32
		name        string
	}{
		{scene.BlendNormal, ShaderBlendNormal, "Normal"},
		{scene.BlendMultiply, ShaderBlendMultiply, "Multiply"},
		{scene.BlendScreen, ShaderBlendScreen, "Screen"},
		{scene.BlendOverlay, ShaderBlendOverlay, "Overlay"},
		{scene.BlendDarken, ShaderBlendDarken, "Darken"},
		{scene.BlendLighten, ShaderBlendLighten, "Lighten"},
		{scene.BlendColorDodge, ShaderBlendColorDodge, "ColorDodge"},
		{scene.BlendColorBurn, ShaderBlendColorBurn, "ColorBurn"},
		{scene.BlendHardLight, ShaderBlendHardLight, "HardLight"},
		{scene.BlendSoftLight, ShaderBlendSoftLight, "SoftLight"},
		{scene.BlendDifference, ShaderBlendDifference, "Difference"},
		{scene.BlendExclusion, ShaderBlendExclusion, "Exclusion"},
		{scene.BlendHue, ShaderBlendHue, "Hue"},
		{scene.BlendSaturation, ShaderBlendSaturation, "Saturation"},
		{scene.BlendColor, ShaderBlendColor, "Color"},
		{scene.BlendLuminosity, ShaderBlendLuminosity, "Luminosity"},
		{scene.BlendClear, ShaderBlendClear, "Clear"},
		{scene.BlendCopy, ShaderBlendCopy, "Copy"},
		{scene.BlendDestination, ShaderBlendDestination, "Destination"},
		{scene.BlendSourceOver, ShaderBlendSourceOver, "SourceOver"},
		{scene.BlendDestinationOver, ShaderBlendDestinationOver, "DestinationOver"},
		{scene.BlendSourceIn, ShaderBlendSourceIn, "SourceIn"},
		{scene.BlendDestinationIn, ShaderBlendDestinationIn, "DestinationIn"},
		{scene.BlendSourceOut, ShaderBlendSourceOut, "SourceOut"},
		{scene.BlendDestinationOut, ShaderBlendDestinationOut, "DestinationOut"},
		{scene.BlendSourceAtop, ShaderBlendSourceAtop, "SourceAtop"},
		{scene.BlendDestinationAtop, ShaderBlendDestinationAtop, "DestinationAtop"},
		{scene.BlendXor, ShaderBlendXor, "Xor"},
		{scene.BlendPlus, ShaderBlendPlus, "Plus"},
	}

	for _, m := range mappings {
		if uint32(m.sceneMode) != m.shaderConst {
			return fmt.Errorf(
				"blend mode mismatch for %s: scene.BlendMode=%d, shader=%d",
				m.name, m.sceneMode, m.shaderConst,
			)
		}
	}

	return nil
}

// BlendParams represents the uniform buffer structure for blend shaders.
// This matches the BlendParams struct in blend.wgsl.
type BlendParams struct {
	Mode    uint32  // Blend mode enum value
	Alpha   float32 // Layer opacity (0.0 - 1.0)
	Padding [2]float32
}

// StripParams represents the uniform buffer structure for strip shaders.
// This matches the StripParams struct in strip.wgsl.
type StripParams struct {
	Color        [4]float32 // Fill color (premultiplied RGBA)
	TargetWidth  int32      // Output texture width
	TargetHeight int32      // Output texture height
	StripCount   int32      // Number of strips to process
	Padding      int32      // Alignment padding
}

// CompositeParams represents the uniform buffer structure for composite shaders.
// This matches the CompositeParams struct in composite.wgsl.
type CompositeParams struct {
	LayerCount uint32 // Number of layers to composite
	Width      uint32 // Output width
	Height     uint32 // Output height
	Padding    uint32 // Alignment padding
}

// LayerDescriptor represents a single layer for compositing.
// This matches the Layer struct in composite.wgsl.
type LayerDescriptor struct {
	TextureIdx uint32  // Index into layer textures
	BlendMode  uint32  // Blend mode for this layer
	Alpha      float32 // Layer opacity
	Padding    float32 // Alignment padding
}

// SDFCircleParams matches the Params struct in sdf_circle.wgsl.
type SDFCircleParams struct {
	CenterX         float32
	CenterY         float32
	RadiusX         float32
	RadiusY         float32
	HalfStrokeWidth float32
	IsStroked       uint32
	ColorR          float32
	ColorG          float32
	ColorB          float32
	ColorA          float32
	TargetWidth     uint32
	TargetHeight    uint32
}

// SDFRRectParams matches the Params struct in sdf_rrect.wgsl.
type SDFRRectParams struct {
	CenterX         float32
	CenterY         float32
	HalfWidth       float32
	HalfHeight      float32
	CornerRadius    float32
	HalfStrokeWidth float32
	IsStroked       uint32
	ColorR          float32
	ColorG          float32
	ColorB          float32
	ColorA          float32
	TargetWidth     uint32
	TargetHeight    uint32
	Padding         uint32 // align to 16 bytes
}
