//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
	"github.com/gogpu/wgpu/core"
)

// testDeviceID returns a DeviceID suitable for testing (zero value).
// In real usage, this would be obtained from backend initialization.
var testDeviceID core.DeviceID

// TestPipelineCacheCreation tests PipelineCache creation.
func TestPipelineCacheCreation(t *testing.T) {
	// Create stub shader modules
	shaders := &ShaderModules{
		Blit:      ShaderModuleID(1),
		Blend:     ShaderModuleID(2),
		Strip:     ShaderModuleID(3),
		Composite: ShaderModuleID(4),
	}

	// Test creation
	pc, err := NewPipelineCache(testDeviceID, shaders)
	if err != nil {
		t.Fatalf("NewPipelineCache failed: %v", err)
	}
	defer pc.Close()

	// Verify initialization
	if !pc.IsInitialized() {
		t.Error("PipelineCache should be initialized")
	}

	// Verify blit pipeline exists
	if pc.GetBlitPipeline() == InvalidPipelineID {
		t.Error("Blit pipeline should be valid")
	}

	// Verify strip pipeline exists
	if pc.GetStripPipeline() == 0 {
		t.Error("Strip pipeline should be valid")
	}

	// Verify composite pipeline exists
	if pc.GetCompositePipeline() == InvalidPipelineID {
		t.Error("Composite pipeline should be valid")
	}
}

// TestPipelineCacheNilShaders tests that nil shaders returns error.
func TestPipelineCacheNilShaders(t *testing.T) {
	pc, err := NewPipelineCache(testDeviceID, nil)
	if err == nil {
		t.Error("Expected error for nil shaders")
		if pc != nil {
			pc.Close()
		}
	}
}

// TestPipelineCacheBlendPipelines tests blend pipeline caching.
func TestPipelineCacheBlendPipelines(t *testing.T) {
	shaders := &ShaderModules{
		Blit:      ShaderModuleID(1),
		Blend:     ShaderModuleID(2),
		Strip:     ShaderModuleID(3),
		Composite: ShaderModuleID(4),
	}

	pc, err := NewPipelineCache(testDeviceID, shaders)
	if err != nil {
		t.Fatalf("NewPipelineCache failed: %v", err)
	}
	defer pc.Close()

	// Get blend pipeline for Normal mode
	p1 := pc.GetBlendPipeline(scene.BlendNormal)
	if p1 == InvalidPipelineID {
		t.Error("Normal blend pipeline should be valid")
	}

	// Get same pipeline again (should be cached)
	p2 := pc.GetBlendPipeline(scene.BlendNormal)
	if p1 != p2 {
		t.Error("Same blend mode should return cached pipeline")
	}

	// Get different blend mode
	p3 := pc.GetBlendPipeline(scene.BlendMultiply)
	if p3 == InvalidPipelineID {
		t.Error("Multiply blend pipeline should be valid")
	}

	// Verify count
	if pc.BlendPipelineCount() != 2 {
		t.Errorf("Expected 2 blend pipelines, got %d", pc.BlendPipelineCount())
	}
}

// TestPipelineCacheWarmup tests pipeline warmup.
func TestPipelineCacheWarmup(t *testing.T) {
	shaders := &ShaderModules{
		Blit:      ShaderModuleID(1),
		Blend:     ShaderModuleID(2),
		Strip:     ShaderModuleID(3),
		Composite: ShaderModuleID(4),
	}

	pc, err := NewPipelineCache(testDeviceID, shaders)
	if err != nil {
		t.Fatalf("NewPipelineCache failed: %v", err)
	}
	defer pc.Close()

	// Warmup should create multiple pipelines
	pc.WarmupBlendPipelines()

	// Should have at least 5 pipelines (Normal, Multiply, Screen, Overlay, SourceOver)
	if pc.BlendPipelineCount() < 5 {
		t.Errorf("Expected at least 5 blend pipelines after warmup, got %d", pc.BlendPipelineCount())
	}
}

// TestPipelineCacheClose tests pipeline cache cleanup.
func TestPipelineCacheClose(t *testing.T) {
	shaders := &ShaderModules{
		Blit:      ShaderModuleID(1),
		Blend:     ShaderModuleID(2),
		Strip:     ShaderModuleID(3),
		Composite: ShaderModuleID(4),
	}

	pc, err := NewPipelineCache(testDeviceID, shaders)
	if err != nil {
		t.Fatalf("NewPipelineCache failed: %v", err)
	}

	// Create some blend pipelines
	_ = pc.GetBlendPipeline(scene.BlendNormal)
	_ = pc.GetBlendPipeline(scene.BlendMultiply)

	// Close
	pc.Close()

	// Verify cleanup
	if pc.IsInitialized() {
		t.Error("PipelineCache should not be initialized after Close")
	}

	if pc.GetBlitPipeline() != InvalidPipelineID {
		t.Error("Blit pipeline should be invalid after Close")
	}

	if pc.BlendPipelineCount() != 0 {
		t.Error("Blend pipelines should be cleared after Close")
	}
}

// TestCommandEncoderCreation tests CommandEncoder creation.
func TestCommandEncoderCreation(t *testing.T) {
	enc := NewCommandEncoder(testDeviceID)

	if enc == nil {
		t.Fatal("NewCommandEncoder returned nil")
	}

	if enc.PassCount() != 0 {
		t.Errorf("New encoder should have 0 passes, got %d", enc.PassCount())
	}
}

// TestCommandEncoderRenderPass tests render pass creation.
func TestCommandEncoderRenderPass(t *testing.T) {
	enc := NewCommandEncoder(testDeviceID)

	// Create stub texture
	tex := &GPUTexture{
		width:  100,
		height: 100,
		format: TextureFormatRGBA8,
	}

	// Begin render pass
	pass := enc.BeginRenderPass(tex, true)
	if pass == nil {
		t.Fatal("BeginRenderPass returned nil")
	}

	// Verify target
	if pass.Target() != tex {
		t.Error("Pass target should match input texture")
	}

	// Cannot begin another pass while one is active
	pass2 := enc.BeginRenderPass(tex, false)
	if pass2 != nil {
		t.Error("Should not be able to begin pass while one is active")
	}

	// End pass
	pass.End()

	// Now we can begin another pass
	pass3 := enc.BeginRenderPass(tex, false)
	if pass3 == nil {
		t.Error("Should be able to begin pass after ending previous")
	}
	pass3.End()

	// Check pass count
	if enc.PassCount() != 2 {
		t.Errorf("Expected 2 passes, got %d", enc.PassCount())
	}
}

// TestCommandEncoderComputePass tests compute pass creation.
func TestCommandEncoderComputePass(t *testing.T) {
	enc := NewCommandEncoder(testDeviceID)

	// Begin compute pass
	pass := enc.BeginComputePass()
	if pass == nil {
		t.Fatal("BeginComputePass returned nil")
	}

	// Cannot begin another pass while one is active
	pass2 := enc.BeginComputePass()
	if pass2 != nil {
		t.Error("Should not be able to begin pass while one is active")
	}

	// End pass
	pass.End()

	// Now we can begin another pass
	pass3 := enc.BeginComputePass()
	if pass3 == nil {
		t.Error("Should be able to begin pass after ending previous")
	}
	pass3.End()
}

// TestCommandEncoderFinish tests command buffer creation.
func TestCommandEncoderFinish(t *testing.T) {
	enc := NewCommandEncoder(testDeviceID)

	// Create and end a pass
	tex := &GPUTexture{width: 100, height: 100, format: TextureFormatRGBA8}
	pass := enc.BeginRenderPass(tex, true)
	pass.End()

	// Finish encoder
	cmdBuf := enc.Finish()
	if cmdBuf == 0 {
		t.Error("Finish should return valid command buffer")
	}
}

// TestRenderPassOperations tests render pass draw operations.
func TestRenderPassOperations(t *testing.T) {
	enc := NewCommandEncoder(testDeviceID)
	tex := &GPUTexture{width: 100, height: 100, format: TextureFormatRGBA8}
	pass := enc.BeginRenderPass(tex, true)

	// Draw without pipeline (should be no-op)
	pass.Draw(3, 1, 0, 0) // Should not panic

	// Set pipeline
	pass.SetPipeline(StubPipelineID(1))

	// Now draw should work
	pass.Draw(3, 1, 0, 0)
	pass.DrawFullScreenTriangle()

	// Set bind group
	pass.SetBindGroup(0, StubBindGroupID(1))

	pass.End()
}

// TestComputePassOperations tests compute pass dispatch operations.
func TestComputePassOperations(t *testing.T) {
	enc := NewCommandEncoder(testDeviceID)
	pass := enc.BeginComputePass()

	// Dispatch without pipeline (should be no-op)
	pass.DispatchWorkgroups(1, 1, 1)

	// Set pipeline
	pass.SetPipeline(StubComputePipelineID(1))

	// Now dispatch should work
	pass.DispatchWorkgroups(4, 1, 1)
	pass.DispatchWorkgroupsForSize(256, 64)

	// Set bind group
	pass.SetBindGroup(0, StubBindGroupID(1))

	pass.End()
}

// TestRenderCommandBuilder tests the fluent builder API.
func TestRenderCommandBuilder(t *testing.T) {
	tex := &GPUTexture{width: 100, height: 100, format: TextureFormatRGBA8}

	cmdBuf := NewRenderCommandBuilder(testDeviceID, tex, true).
		SetPipeline(StubPipelineID(1)).
		SetBindGroup(0, StubBindGroupID(1)).
		DrawFullScreen().
		Finish()

	if cmdBuf == 0 {
		t.Error("Builder should produce valid command buffer")
	}
}

// TestComputeCommandBuilder tests the compute builder API.
func TestComputeCommandBuilder(t *testing.T) {
	cmdBuf := NewComputeCommandBuilder(testDeviceID).
		SetPipeline(StubComputePipelineID(1)).
		SetBindGroup(0, StubBindGroupID(1)).
		DispatchForSize(1024, 64).
		Finish()

	if cmdBuf == 0 {
		t.Error("Builder should produce valid command buffer")
	}
}

// TestGPUSceneRendererCreation tests renderer creation without actual GPU.
// This tests the initialization logic with nil backend (which is allowed for testing).
func TestGPUSceneRendererCreation(t *testing.T) {
	// With nil backend, creation should fail
	_, err := NewGPUSceneRenderer(nil, GPUSceneRendererConfig{
		Width:  800,
		Height: 600,
	})
	if err == nil {
		t.Error("Expected error with nil backend")
	}
}

// TestGPUSceneRendererInvalidDimensions tests that invalid dimensions are rejected.
func TestGPUSceneRendererInvalidDimensions(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"zero width", 0, 600},
		{"zero height", 800, 0},
		{"negative width", -100, 600},
		{"negative height", 800, -100},
		{"both zero", 0, 0},
		{"both negative", -100, -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGPUSceneRenderer(nil, GPUSceneRendererConfig{
				Width:  tt.width,
				Height: tt.height,
			})
			// Expect error due to nil backend or invalid dimensions
			if err == nil {
				t.Error("Expected error for invalid dimensions")
			}
		})
	}
}

// TestBlendModeMapping tests blend mode shader constant mapping.
func TestBlendModeMapping(t *testing.T) {
	tests := []struct {
		mode      scene.BlendMode
		shaderVal uint32
	}{
		{scene.BlendNormal, ShaderBlendNormal},
		{scene.BlendMultiply, ShaderBlendMultiply},
		{scene.BlendScreen, ShaderBlendScreen},
		{scene.BlendOverlay, ShaderBlendOverlay},
		{scene.BlendSourceOver, ShaderBlendSourceOver},
		{scene.BlendXor, ShaderBlendXor},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			got := BlendModeToShader(tt.mode)
			if got != tt.shaderVal {
				t.Errorf("BlendModeToShader(%v) = %d, want %d", tt.mode, got, tt.shaderVal)
			}

			// Test reverse mapping
			back := ShaderToBlendMode(tt.shaderVal)
			if back != tt.mode {
				t.Errorf("ShaderToBlendMode(%d) = %v, want %v", tt.shaderVal, back, tt.mode)
			}
		})
	}
}

// TestBlendParams tests BlendParams struct layout.
func TestBlendParams(t *testing.T) {
	params := BlendParams{
		Mode:    ShaderBlendMultiply,
		Alpha:   0.75,
		Padding: [2]float32{0, 0},
	}

	if params.Mode != 1 {
		t.Errorf("Mode should be 1, got %d", params.Mode)
	}

	if params.Alpha != 0.75 {
		t.Errorf("Alpha should be 0.75, got %f", params.Alpha)
	}
}

// TestStripParams tests StripParams struct layout.
func TestStripParams(t *testing.T) {
	params := StripParams{
		Color:        [4]float32{1.0, 0.5, 0.25, 1.0},
		TargetWidth:  800,
		TargetHeight: 600,
		StripCount:   100,
	}

	if params.Color[0] != 1.0 {
		t.Errorf("Color[0] should be 1.0, got %f", params.Color[0])
	}

	if params.TargetWidth != 800 {
		t.Errorf("TargetWidth should be 800, got %d", params.TargetWidth)
	}

	if params.StripCount != 100 {
		t.Errorf("StripCount should be 100, got %d", params.StripCount)
	}
}

// TestLayerStackOperations tests layer stack push/pop logic.
func TestLayerStackOperations(t *testing.T) {
	// Create a minimal test scene
	s := scene.NewScene()

	// Add some content
	rect := scene.NewRectShape(10, 10, 100, 100)
	brush := scene.SolidBrush(gg.Red)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(), brush, rect)

	// Push a layer
	s.PushLayer(scene.BlendMultiply, 0.5, nil)

	// Add content to layer
	circle := scene.NewCircleShape(60, 60, 40)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Blue), circle)

	// Pop layer
	s.PopLayer()

	// Verify scene is not empty
	if s.IsEmpty() {
		t.Error("Scene should not be empty")
	}

	// Get encoding
	enc := s.Encoding()
	if enc.IsEmpty() {
		t.Error("Encoding should not be empty")
	}
}

// TestQueueSubmitter tests queue submission helper.
func TestQueueSubmitter(t *testing.T) {
	var testQueueID core.QueueID
	submitter := NewQueueSubmitter(testQueueID)

	// Submit should not panic with nil buffers
	submitter.Submit()

	// Submit with buffer
	cmd := NewCommandBuffer(StubCommandBufferID(1))
	submitter.Submit(cmd)

	// WriteBuffer should not panic
	submitter.WriteBuffer(StubBufferID(1), 0, []byte{1, 2, 3, 4})

	// WriteTexture should not panic
	tex := &GPUTexture{width: 100, height: 100, format: TextureFormatRGBA8}
	submitter.WriteTexture(tex, make([]byte, 100*100*4))
}

// TestBindGroupBuilder tests bind group builder.
func TestBindGroupBuilder(t *testing.T) {
	builder := NewBindGroupBuilder(testDeviceID, StubBindGroupLayoutID(1))
	bg := builder.Build()

	if bg == 0 {
		t.Error("Build should return valid bind group")
	}
}

// TestSceneDecoderIntegration tests scene decoding integration.
func TestSceneDecoderIntegration(t *testing.T) {
	// Create scene with various commands
	s := scene.NewScene()

	// Transform
	s.PushTransform(scene.TranslateAffine(100, 100))

	// Fill
	rect := scene.NewRectShape(0, 0, 50, 50)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), rect)

	// Stroke
	line := scene.NewLineShape(0, 0, 100, 100)
	s.Stroke(scene.DefaultStrokeStyle(), scene.IdentityAffine(), scene.SolidBrush(gg.Blue), line)

	// Pop transform
	s.PopTransform()

	// Layer
	s.PushLayer(scene.BlendScreen, 0.8, nil)
	circle := scene.NewCircleShape(75, 75, 30)
	s.Fill(scene.FillEvenOdd, scene.IdentityAffine(), scene.SolidBrush(gg.Green), circle)
	s.PopLayer()

	// Get encoding and verify
	enc := s.Encoding()

	// Count tags
	tagCounts := make(map[scene.Tag]int)
	for _, tag := range enc.Tags() {
		tagCounts[tag]++
	}

	// Should have various commands
	if tagCounts[scene.TagFill] == 0 {
		t.Error("Expected Fill commands")
	}
	if tagCounts[scene.TagStroke] == 0 {
		t.Error("Expected Stroke commands")
	}
	if tagCounts[scene.TagPushLayer] == 0 {
		t.Error("Expected PushLayer commands")
	}
	if tagCounts[scene.TagPopLayer] == 0 {
		t.Error("Expected PopLayer commands")
	}
}

// TestIndexFormat tests index format constants.
func TestIndexFormat(t *testing.T) {
	if IndexFormatUint16 != 0 {
		t.Errorf("IndexFormatUint16 should be 0, got %d", IndexFormatUint16)
	}

	if IndexFormatUint32 != 1 {
		t.Errorf("IndexFormatUint32 should be 1, got %d", IndexFormatUint32)
	}
}
