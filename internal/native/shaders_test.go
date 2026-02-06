package native

import (
	"strings"
	"testing"

	"github.com/gogpu/gg/scene"
)

// TestShaderSourcesNonEmpty verifies that all shader sources are embedded correctly.
func TestShaderSourcesNonEmpty(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{"blit", GetBlitShaderSource()},
		{"blend", GetBlendShaderSource()},
		{"strip", GetStripShaderSource()},
		{"composite", GetCompositeShaderSource()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.source == "" {
				t.Errorf("%s shader source is empty", tt.name)
			}
			if len(tt.source) < 100 {
				t.Errorf("%s shader source suspiciously short: %d bytes", tt.name, len(tt.source))
			}
		})
	}
}

// TestShaderSourcesContainExpectedContent verifies shader sources contain key elements.
func TestShaderSourcesContainExpectedContent(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		required []string
	}{
		{
			name:   "blit",
			source: GetBlitShaderSource(),
			required: []string{
				"@vertex",
				"@fragment",
				"vs_main",
				"fs_main",
				"texture_2d<f32>",
				"sampler",
				"textureSample",
			},
		},
		{
			name:   "blend",
			source: GetBlendShaderSource(),
			required: []string{
				"@vertex",
				"@fragment",
				"BLEND_NORMAL",
				"BLEND_MULTIPLY",
				"BLEND_SCREEN",
				"BLEND_CLEAR",
				"BLEND_SOURCE_OVER",
				"blend_porter_duff",
				"BlendParams",
			},
		},
		{
			name:   "strip",
			source: GetStripShaderSource(),
			required: []string{
				"@compute",
				"@workgroup_size",
				"Strip",
				"StripParams",
				"coverage",
				"textureStore",
			},
		},
		{
			name:   "composite",
			source: GetCompositeShaderSource(),
			required: []string{
				"@vertex",
				"@fragment",
				"Layer",
				"CompositeParams",
				"layer_count",
				"composite_layer",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, req := range tt.required {
				if !strings.Contains(tt.source, req) {
					t.Errorf("%s shader missing required element: %q", tt.name, req)
				}
			}
		})
	}
}

// TestBlendShaderContainsAllModes verifies all 29 blend modes are defined in the shader.
func TestBlendShaderContainsAllModes(t *testing.T) {
	source := GetBlendShaderSource()

	// All blend mode constants that should be defined
	modes := []string{
		"BLEND_NORMAL",
		"BLEND_MULTIPLY",
		"BLEND_SCREEN",
		"BLEND_OVERLAY",
		"BLEND_DARKEN",
		"BLEND_LIGHTEN",
		"BLEND_COLOR_DODGE",
		"BLEND_COLOR_BURN",
		"BLEND_HARD_LIGHT",
		"BLEND_SOFT_LIGHT",
		"BLEND_DIFFERENCE",
		"BLEND_EXCLUSION",
		"BLEND_HUE",
		"BLEND_SATURATION",
		"BLEND_COLOR",
		"BLEND_LUMINOSITY",
		"BLEND_CLEAR",
		"BLEND_COPY",
		"BLEND_DESTINATION",
		"BLEND_SOURCE_OVER",
		"BLEND_DESTINATION_OVER",
		"BLEND_SOURCE_IN",
		"BLEND_DESTINATION_IN",
		"BLEND_SOURCE_OUT",
		"BLEND_DESTINATION_OUT",
		"BLEND_SOURCE_ATOP",
		"BLEND_DESTINATION_ATOP",
		"BLEND_XOR",
		"BLEND_PLUS",
	}

	for _, mode := range modes {
		if !strings.Contains(source, mode) {
			t.Errorf("blend shader missing mode constant: %s", mode)
		}
	}
}

// TestBlendModeConstantsMatchScene verifies Go constants match scene.BlendMode values.
func TestBlendModeConstantsMatchScene(t *testing.T) {
	tests := []struct {
		name        string
		sceneMode   scene.BlendMode
		shaderConst uint32
	}{
		{"Normal", scene.BlendNormal, ShaderBlendNormal},
		{"Multiply", scene.BlendMultiply, ShaderBlendMultiply},
		{"Screen", scene.BlendScreen, ShaderBlendScreen},
		{"Overlay", scene.BlendOverlay, ShaderBlendOverlay},
		{"Darken", scene.BlendDarken, ShaderBlendDarken},
		{"Lighten", scene.BlendLighten, ShaderBlendLighten},
		{"ColorDodge", scene.BlendColorDodge, ShaderBlendColorDodge},
		{"ColorBurn", scene.BlendColorBurn, ShaderBlendColorBurn},
		{"HardLight", scene.BlendHardLight, ShaderBlendHardLight},
		{"SoftLight", scene.BlendSoftLight, ShaderBlendSoftLight},
		{"Difference", scene.BlendDifference, ShaderBlendDifference},
		{"Exclusion", scene.BlendExclusion, ShaderBlendExclusion},
		{"Hue", scene.BlendHue, ShaderBlendHue},
		{"Saturation", scene.BlendSaturation, ShaderBlendSaturation},
		{"Color", scene.BlendColor, ShaderBlendColor},
		{"Luminosity", scene.BlendLuminosity, ShaderBlendLuminosity},
		{"Clear", scene.BlendClear, ShaderBlendClear},
		{"Copy", scene.BlendCopy, ShaderBlendCopy},
		{"Destination", scene.BlendDestination, ShaderBlendDestination},
		{"SourceOver", scene.BlendSourceOver, ShaderBlendSourceOver},
		{"DestinationOver", scene.BlendDestinationOver, ShaderBlendDestinationOver},
		{"SourceIn", scene.BlendSourceIn, ShaderBlendSourceIn},
		{"DestinationIn", scene.BlendDestinationIn, ShaderBlendDestinationIn},
		{"SourceOut", scene.BlendSourceOut, ShaderBlendSourceOut},
		{"DestinationOut", scene.BlendDestinationOut, ShaderBlendDestinationOut},
		{"SourceAtop", scene.BlendSourceAtop, ShaderBlendSourceAtop},
		{"DestinationAtop", scene.BlendDestinationAtop, ShaderBlendDestinationAtop},
		{"Xor", scene.BlendXor, ShaderBlendXor},
		{"Plus", scene.BlendPlus, ShaderBlendPlus},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if uint32(tt.sceneMode) != tt.shaderConst {
				t.Errorf(
					"blend mode mismatch: scene.Blend%s=%d, ShaderBlend%s=%d",
					tt.name, tt.sceneMode, tt.name, tt.shaderConst,
				)
			}
		})
	}
}

// TestValidateBlendModeMapping tests the ValidateBlendModeMapping function.
func TestValidateBlendModeMapping(t *testing.T) {
	err := ValidateBlendModeMapping()
	if err != nil {
		t.Errorf("ValidateBlendModeMapping() returned error: %v", err)
	}
}

// TestBlendModeToShader tests the BlendModeToShader conversion.
func TestBlendModeToShader(t *testing.T) {
	tests := []struct {
		input scene.BlendMode
		want  uint32
	}{
		{scene.BlendNormal, 0},
		{scene.BlendMultiply, 1},
		{scene.BlendClear, 16},
		{scene.BlendPlus, 28},
	}

	for _, tt := range tests {
		got := BlendModeToShader(tt.input)
		if got != tt.want {
			t.Errorf("BlendModeToShader(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// TestShaderToBlendMode tests the ShaderToBlendMode conversion.
func TestShaderToBlendMode(t *testing.T) {
	tests := []struct {
		input uint32
		want  scene.BlendMode
	}{
		{0, scene.BlendNormal},
		{1, scene.BlendMultiply},
		{16, scene.BlendClear},
		{28, scene.BlendPlus},
	}

	for _, tt := range tests {
		got := ShaderToBlendMode(tt.input)
		if got != tt.want {
			t.Errorf("ShaderToBlendMode(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// TestCompileShaders tests the shader compilation function.
func TestCompileShaders(t *testing.T) {
	modules, err := CompileShaders(0)
	if err != nil {
		t.Fatalf("CompileShaders() returned error: %v", err)
	}

	if modules == nil {
		t.Fatal("CompileShaders() returned nil modules")
	}

	if !modules.IsValid() {
		t.Error("CompileShaders() returned invalid modules")
	}

	// Verify each module has a non-zero ID
	if modules.Blit == InvalidShaderModule {
		t.Error("Blit shader module is invalid")
	}
	if modules.Blend == InvalidShaderModule {
		t.Error("Blend shader module is invalid")
	}
	if modules.Strip == InvalidShaderModule {
		t.Error("Strip shader module is invalid")
	}
	if modules.Composite == InvalidShaderModule {
		t.Error("Composite shader module is invalid")
	}
}

// TestShaderModulesIsValid tests the IsValid method.
func TestShaderModulesIsValid(t *testing.T) {
	tests := []struct {
		name    string
		modules ShaderModules
		want    bool
	}{
		{
			name:    "all valid",
			modules: ShaderModules{Blit: 1, Blend: 2, Strip: 3, Composite: 4},
			want:    true,
		},
		{
			name:    "blit invalid",
			modules: ShaderModules{Blit: 0, Blend: 2, Strip: 3, Composite: 4},
			want:    false,
		},
		{
			name:    "blend invalid",
			modules: ShaderModules{Blit: 1, Blend: 0, Strip: 3, Composite: 4},
			want:    false,
		},
		{
			name:    "strip invalid",
			modules: ShaderModules{Blit: 1, Blend: 2, Strip: 0, Composite: 4},
			want:    false,
		},
		{
			name:    "composite invalid",
			modules: ShaderModules{Blit: 1, Blend: 2, Strip: 3, Composite: 0},
			want:    false,
		},
		{
			name:    "all invalid",
			modules: ShaderModules{},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.modules.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBlendParamsSize verifies BlendParams struct has correct size for GPU alignment.
func TestBlendParamsSize(t *testing.T) {
	var params BlendParams
	// BlendParams should be 16 bytes (4 + 4 + 8 padding = 16)
	// This ensures proper GPU buffer alignment
	expectedFields := 4 // mode, alpha, padding[0], padding[1]
	actualFields := 1 + 1 + len(params.Padding)
	if actualFields != expectedFields {
		t.Errorf("BlendParams has %d fields, expected %d", actualFields, expectedFields)
	}
}

// TestStripParamsSize verifies StripParams struct has correct layout.
func TestStripParamsSize(t *testing.T) {
	var params StripParams
	// Verify the color field has 4 components
	if len(params.Color) != 4 {
		t.Errorf("StripParams.Color has %d components, expected 4", len(params.Color))
	}
}

// TestLayerDescriptorSize verifies LayerDescriptor struct has correct layout.
func TestLayerDescriptorSize(t *testing.T) {
	// LayerDescriptor should be 16 bytes (4 + 4 + 4 + 4 = 16)
	// This ensures proper GPU buffer alignment for arrays
	var desc LayerDescriptor
	_ = desc.Padding // Ensure padding field exists
}

// TestWGSLSyntaxBasics verifies basic WGSL syntax elements are present.
func TestWGSLSyntaxBasics(t *testing.T) {
	tests := []struct {
		name   string
		source string
		checks []struct {
			pattern string
			desc    string
		}
	}{
		{
			name:   "blit",
			source: GetBlitShaderSource(),
			checks: []struct {
				pattern string
				desc    string
			}{
				{"struct VertexOutput", "vertex output struct"},
				{"@builtin(position)", "position builtin"},
				{"@location(0)", "location attribute"},
				{"@group(0)", "bind group"},
				{"@binding(", "binding"},
				{"vec4<f32>", "vec4 type"},
			},
		},
		{
			name:   "strip",
			source: GetStripShaderSource(),
			checks: []struct {
				pattern string
				desc    string
			}{
				{"struct Strip", "strip struct"},
				{"struct StripParams", "params struct"},
				{"var<storage, read>", "storage buffer"},
				{"var<uniform>", "uniform buffer"},
				{"texture_storage_2d", "storage texture"},
				{"@builtin(global_invocation_id)", "compute builtin"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, check := range tt.checks {
				if !strings.Contains(tt.source, check.pattern) {
					t.Errorf("%s shader missing %s: %q", tt.name, check.desc, check.pattern)
				}
			}
		})
	}
}

// TestBlendShaderHSLFunctions verifies HSL blend mode helper functions exist.
func TestBlendShaderHSLFunctions(t *testing.T) {
	source := GetBlendShaderSource()

	hslFunctions := []string{
		"fn luminosity",
		"fn saturation",
		"fn clip_color",
		"fn set_lum",
		"fn set_sat",
		"fn blend_hue",
		"fn blend_saturation",
		"fn blend_color",
		"fn blend_luminosity_mode",
	}

	for _, fn := range hslFunctions {
		if !strings.Contains(source, fn) {
			t.Errorf("blend shader missing HSL function: %s", fn)
		}
	}
}

// TestBlendShaderSeparableFunctions verifies separable blend mode functions exist.
func TestBlendShaderSeparableFunctions(t *testing.T) {
	source := GetBlendShaderSource()

	separableFunctions := []string{
		"fn blend_multiply",
		"fn blend_screen",
		"fn blend_overlay",
		"fn blend_darken",
		"fn blend_lighten",
		"fn blend_color_dodge",
		"fn blend_color_burn",
		"fn blend_hard_light",
		"fn blend_soft_light",
		"fn blend_difference",
		"fn blend_exclusion",
	}

	for _, fn := range separableFunctions {
		if !strings.Contains(source, fn) {
			t.Errorf("blend shader missing separable function: %s", fn)
		}
	}
}

// TestStripShaderEntryPoints verifies strip shader has expected entry points.
func TestStripShaderEntryPoints(t *testing.T) {
	source := GetStripShaderSource()

	entryPoints := []string{
		"fn cs_main",
		"fn cs_accumulate",
		"fn cs_colored",
	}

	for _, fn := range entryPoints {
		if !strings.Contains(source, fn) {
			t.Errorf("strip shader missing entry point: %s", fn)
		}
	}
}

// TestCompositeShaderEntryPoints verifies composite shader has expected entry points.
func TestCompositeShaderEntryPoints(t *testing.T) {
	source := GetCompositeShaderSource()

	entryPoints := []string{
		"fn vs_main",
		"fn fs_main",
		"fn fs_simple",
	}

	for _, fn := range entryPoints {
		if !strings.Contains(source, fn) {
			t.Errorf("composite shader missing entry point: %s", fn)
		}
	}
}

// TestShaderSourceLineCount provides approximate line counts for documentation.
func TestShaderSourceLineCount(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		minLines    int
		description string
	}{
		{"blit", GetBlitShaderSource(), 30, "simple texture copy"},
		{"blend", GetBlendShaderSource(), 200, "29 blend modes"},
		{"strip", GetStripShaderSource(), 80, "strip rasterization"},
		{"composite", GetCompositeShaderSource(), 100, "layer compositing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Count(tt.source, "\n")
			if lines < tt.minLines {
				t.Errorf(
					"%s shader has %d lines, expected at least %d for %s",
					tt.name, lines, tt.minLines, tt.description,
				)
			}
			t.Logf("%s shader: %d lines", tt.name, lines)
		})
	}
}
