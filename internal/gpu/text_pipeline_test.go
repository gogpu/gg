//go:build !nogpu

package gpu

import (
	"strings"
	"testing"

	"github.com/gogpu/gg"
)

// TestMSDFTextShaderSource tests that the shader source is properly embedded.
func TestMSDFTextShaderSource(t *testing.T) {
	source := GetMSDFTextShaderSource()

	if source == "" {
		t.Fatal("MSDF text shader source is empty")
	}

	// Verify expected content
	expectedStrings := []string{
		"TextUniforms",
		"VertexInput",
		"VertexOutput",
		"msdf_atlas",
		"msdf_sampler",
		"median3",
		"vs_main",
		"fs_main",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(source, expected) {
			t.Errorf("shader source missing expected string: %q", expected)
		}
	}

	// Verify it's valid WGSL by checking structure
	if !strings.Contains(source, "@vertex") {
		t.Error("shader missing @vertex entry point")
	}
	if !strings.Contains(source, "@fragment") {
		t.Error("shader missing @fragment entry point")
	}
	if !strings.Contains(source, "@group(0) @binding(0)") {
		t.Error("shader missing bind group 0")
	}
}

// TestTextQuad tests TextQuad struct.
func TestTextQuad(t *testing.T) {
	quad := TextQuad{
		X0: 0, Y0: 0, X1: 100, Y1: 50,
		U0: 0, V0: 0, U1: 0.5, V1: 0.25,
	}

	if quad.X0 != 0 || quad.Y0 != 0 {
		t.Errorf("unexpected top-left: (%f, %f)", quad.X0, quad.Y0)
	}
	if quad.X1 != 100 || quad.Y1 != 50 {
		t.Errorf("unexpected bottom-right: (%f, %f)", quad.X1, quad.Y1)
	}
}

// TestQuadsToVertices tests quad to vertex conversion.
func TestQuadsToVertices(t *testing.T) {
	tests := []struct {
		name  string
		quads []TextQuad
		want  int // expected vertex count
	}{
		{
			name:  "empty",
			quads: nil,
			want:  0,
		},
		{
			name: "single quad",
			quads: []TextQuad{
				{X0: 0, Y0: 0, X1: 10, Y1: 10, U0: 0, V0: 0, U1: 1, V1: 1},
			},
			want: 4,
		},
		{
			name: "multiple quads",
			quads: []TextQuad{
				{X0: 0, Y0: 0, X1: 10, Y1: 10, U0: 0, V0: 0, U1: 0.5, V1: 0.5},
				{X0: 10, Y0: 0, X1: 20, Y1: 10, U0: 0.5, V0: 0, U1: 1, V1: 0.5},
			},
			want: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vertices := quadsToVertices(tt.quads)
			if len(vertices) != tt.want {
				t.Errorf("quadsToVertices() got %d vertices, want %d", len(vertices), tt.want)
			}

			// Verify vertex positions for non-empty quads
			if len(tt.quads) > 0 {
				q := tt.quads[0]
				v := vertices[0:4]

				// Vertex 0: bottom-left
				if v[0].X != q.X0 || v[0].Y != q.Y0 {
					t.Errorf("vertex 0 position: got (%f,%f), want (%f,%f)", v[0].X, v[0].Y, q.X0, q.Y0)
				}
				// Vertex 2: top-right
				if v[2].X != q.X1 || v[2].Y != q.Y1 {
					t.Errorf("vertex 2 position: got (%f,%f), want (%f,%f)", v[2].X, v[2].Y, q.X1, q.Y1)
				}
			}
		})
	}
}

// TestGenerateQuadIndices tests index generation for quads.
func TestGenerateQuadIndices(t *testing.T) {
	tests := []struct {
		name     string
		numQuads int
		want     int // expected index count
	}{
		{"zero quads", 0, 0},
		{"one quad", 1, 6},
		{"two quads", 2, 12},
		{"ten quads", 10, 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indices := generateQuadIndices(tt.numQuads)
			if len(indices) != tt.want {
				t.Errorf("generateQuadIndices(%d) got %d indices, want %d",
					tt.numQuads, len(indices), tt.want)
			}

			// Verify index pattern for first quad
			if tt.numQuads > 0 {
				// First triangle: 0, 1, 2
				if indices[0] != 0 || indices[1] != 1 || indices[2] != 2 {
					t.Errorf("first triangle: got [%d,%d,%d], want [0,1,2]",
						indices[0], indices[1], indices[2])
				}
				// Second triangle: 2, 3, 0
				if indices[3] != 2 || indices[4] != 3 || indices[5] != 0 {
					t.Errorf("second triangle: got [%d,%d,%d], want [2,3,0]",
						indices[3], indices[4], indices[5])
				}
			}
		})
	}
}

// TestTextBatch tests batch rendering struct.
func TestTextBatch(t *testing.T) {
	batch := TextBatch{
		Quads: []TextQuad{
			{X0: 0, Y0: 0, X1: 10, Y1: 10},
			{X0: 10, Y0: 0, X1: 20, Y1: 10},
		},
		Color:     gg.Blue,
		Transform: gg.Translate(50, 50),
	}

	if len(batch.Quads) != 2 {
		t.Errorf("batch quads: got %d, want 2", len(batch.Quads))
	}
}

// TestMSDFTextShaderComments tests shader documentation.
func TestMSDFTextShaderComments(t *testing.T) {
	source := GetMSDFTextShaderSource()

	// Verify shader has proper documentation
	expectedComments := []string{
		"MSDF",
		"Multi-channel Signed Distance Field",
		"median",
		"screen-space",
		"anti-alias",
	}

	for _, expected := range expectedComments {
		if !strings.Contains(strings.ToLower(source), strings.ToLower(expected)) {
			t.Errorf("shader missing documentation for: %q", expected)
		}
	}
}

// TestBuildTextVertexData tests vertex data serialization.
func TestBuildTextVertexData(t *testing.T) {
	t.Run("empty quads", func(t *testing.T) {
		data := buildTextVertexData(nil)
		if data != nil {
			t.Error("expected nil for empty quads")
		}
	})

	t.Run("single quad", func(t *testing.T) {
		quads := []TextQuad{
			{X0: 0, Y0: 0, X1: 10, Y1: 20, U0: 0, V0: 0, U1: 0.5, V1: 1},
		}
		data := buildTextVertexData(quads)
		// 1 quad * 4 vertices * 16 bytes = 64 bytes
		if len(data) != 64 {
			t.Errorf("got %d bytes, want 64", len(data))
		}
	})

	t.Run("multiple quads", func(t *testing.T) {
		quads := []TextQuad{
			{X0: 0, Y0: 0, X1: 10, Y1: 10},
			{X0: 10, Y0: 0, X1: 20, Y1: 10},
		}
		data := buildTextVertexData(quads)
		// 2 quads * 4 vertices * 16 bytes = 128 bytes
		if len(data) != 128 {
			t.Errorf("got %d bytes, want 128", len(data))
		}
	})
}

// TestBuildTextIndexData tests index data serialization.
func TestBuildTextIndexData(t *testing.T) {
	t.Run("single quad", func(t *testing.T) {
		data := buildTextIndexData(1)
		// 1 quad * 6 indices * 2 bytes = 12 bytes
		if len(data) != 12 {
			t.Errorf("got %d bytes, want 12", len(data))
		}
	})

	t.Run("multiple quads", func(t *testing.T) {
		data := buildTextIndexData(5)
		// 5 quads * 6 indices * 2 bytes = 60 bytes
		if len(data) != 60 {
			t.Errorf("got %d bytes, want 60", len(data))
		}
	})
}

// TestMakeTextUniform tests uniform buffer creation.
func TestMakeTextUniform(t *testing.T) {
	buf := makeTextUniform(gg.White, gg.Identity(), 4.0, 256.0)
	if len(buf) != textUniformSize {
		t.Errorf("got %d bytes, want %d", len(buf), textUniformSize)
	}
}

// TestRgbToRGBA tests RGB to RGBA conversion.
func TestRgbToRGBA(t *testing.T) {
	t.Run("single pixel", func(t *testing.T) {
		rgb := []byte{255, 128, 64}
		rgba := rgbToRGBA(rgb, 1, 1)
		if len(rgba) != 4 {
			t.Fatalf("got %d bytes, want 4", len(rgba))
		}
		if rgba[0] != 255 || rgba[1] != 128 || rgba[2] != 64 || rgba[3] != 255 {
			t.Errorf("got [%d,%d,%d,%d], want [255,128,64,255]", rgba[0], rgba[1], rgba[2], rgba[3])
		}
	})

	t.Run("2x2 image", func(t *testing.T) {
		rgb := []byte{
			255, 0, 0, // red
			0, 255, 0, // green
			0, 0, 255, // blue
			128, 128, 128, // gray
		}
		rgba := rgbToRGBA(rgb, 2, 2)
		if len(rgba) != 16 {
			t.Fatalf("got %d bytes, want 16", len(rgba))
		}
		// Check alpha for all pixels
		for i := 0; i < 4; i++ {
			if rgba[i*4+3] != 255 {
				t.Errorf("pixel %d alpha: got %d, want 255", i, rgba[i*4+3])
			}
		}
	})
}

// TestMSDFTextPipelineCreate tests real pipeline creation via noop backend.
func TestMSDFTextPipelineCreate(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewMSDFTextPipeline(device, queue, 4)
	defer p.Destroy()

	if err := p.createPipeline(); err != nil {
		t.Fatalf("createPipeline() error = %v", err)
	}
	if p.shader == nil {
		t.Error("expected non-nil shader")
	}
	if p.uniformLayout == nil {
		t.Error("expected non-nil uniformLayout")
	}
	if p.pipeLayout == nil {
		t.Error("expected non-nil pipeLayout")
	}
	if p.pipeline == nil {
		t.Error("expected non-nil pipeline")
	}
	if p.sampler == nil {
		t.Error("expected non-nil sampler")
	}
}

// TestMSDFTextPipelineWithStencil tests stencil variant creation.
func TestMSDFTextPipelineWithStencil(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewMSDFTextPipeline(device, queue, 4)
	defer p.Destroy()

	if err := p.ensurePipelineWithStencil(); err != nil {
		t.Fatalf("ensurePipelineWithStencil() error = %v", err)
	}
	if p.pipelineWithStencil == nil {
		t.Error("expected non-nil pipelineWithStencil")
	}

	// Second call should be a no-op.
	if err := p.ensurePipelineWithStencil(); err != nil {
		t.Fatalf("second ensurePipelineWithStencil() error = %v", err)
	}
}

// TestMSDFTextPipelineDestroy tests cleanup.
func TestMSDFTextPipelineDestroy(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewMSDFTextPipeline(device, queue, 4)
	_ = p.ensurePipelineWithStencil()
	p.Destroy()

	// All fields should be nil after destroy.
	if p.shader != nil || p.uniformLayout != nil || p.pipeLayout != nil ||
		p.pipeline != nil || p.pipelineWithStencil != nil || p.sampler != nil {
		t.Error("expected all pipeline fields nil after Destroy")
	}

	// Double destroy should be safe.
	p.Destroy()
}

// BenchmarkQuadsToVertices benchmarks quad to vertex conversion.
func BenchmarkQuadsToVertices(b *testing.B) {
	quads := make([]TextQuad, 100)
	for i := range quads {
		quads[i] = TextQuad{
			X0: float32(i * 10), Y0: 0,
			X1: float32(i*10 + 8), Y1: 16,
			U0: 0, V0: 0, U1: 0.1, V1: 0.1,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = quadsToVertices(quads)
	}
}

// BenchmarkGenerateQuadIndices benchmarks index generation.
func BenchmarkGenerateQuadIndices(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateQuadIndices(100)
	}
}

// TestTextVertexLayout tests vertex struct layout.
func TestTextVertexLayout(t *testing.T) {
	// Verify TextVertex is the expected size (4 float32 = 16 bytes)
	v := TextVertex{X: 1, Y: 2, U: 3, V: 4}

	// Access all fields to ensure struct is usable
	if v.X != 1 || v.Y != 2 || v.U != 3 || v.V != 4 {
		t.Error("vertex fields not correctly accessible")
	}
}

// TestTextUniformsLayout tests uniform struct layout.
func TestTextUniformsLayout(t *testing.T) {
	var u TextUniforms

	// Transform should be 16 floats (64 bytes)
	transformLen := len(u.Transform)
	if transformLen != 16 {
		t.Errorf("Transform length: got %d, want 16", transformLen)
	}

	// Color should be 4 floats (16 bytes)
	colorLen := len(u.Color)
	if colorLen != 4 {
		t.Errorf("Color length: got %d, want 4", colorLen)
	}

	// MSDFParams should be 4 floats (16 bytes)
	paramsLen := len(u.MSDFParams)
	if paramsLen != 4 {
		t.Errorf("MSDFParams length: got %d, want 4", paramsLen)
	}

	// Total should be 96 bytes (well-aligned for GPU)
}
