//go:build !nogpu

package gpu

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/wgpu/hal"
)

func TestConvexRendererCreation(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	cr := NewConvexRenderer(device, queue)
	defer cr.Destroy()

	if cr.device == nil {
		t.Error("expected non-nil device")
	}
	if cr.queue == nil {
		t.Error("expected non-nil queue")
	}

	// Before pipeline creation, all GPU objects should be nil.
	if cr.shader != nil {
		t.Error("expected nil shader before pipeline creation")
	}
	if cr.pipeline != nil {
		t.Error("expected nil pipeline before pipeline creation")
	}
}

func TestConvexRendererPipeline(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	cr := NewConvexRenderer(device, queue)
	defer cr.Destroy()

	err := cr.ensurePipeline()
	if err != nil {
		t.Fatalf("ensurePipeline failed: %v", err)
	}

	if cr.shader == nil {
		t.Error("expected non-nil shader")
	}
	if cr.uniformLayout == nil {
		t.Error("expected non-nil uniformLayout")
	}
	if cr.pipeLayout == nil {
		t.Error("expected non-nil pipeLayout")
	}
	if cr.pipeline == nil {
		t.Error("expected non-nil pipeline")
	}

	// Idempotent: calling again should not re-create.
	origPipeline := cr.pipeline
	err = cr.ensurePipeline()
	if err != nil {
		t.Fatalf("second ensurePipeline failed: %v", err)
	}
	if cr.pipeline != origPipeline {
		t.Error("pipeline was recreated unnecessarily")
	}
}

func TestConvexRendererPipelineWithStencil(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	cr := NewConvexRenderer(device, queue)
	defer cr.Destroy()

	err := cr.ensurePipelineWithStencil()
	if err != nil {
		t.Fatalf("ensurePipelineWithStencil failed: %v", err)
	}

	// Both pipeline variants should exist.
	if cr.pipeline == nil {
		t.Error("expected non-nil base pipeline")
	}
	if cr.pipelineWithStencil == nil {
		t.Error("expected non-nil pipelineWithStencil")
	}

	// Idempotent.
	origPipeline := cr.pipelineWithStencil
	err = cr.ensurePipelineWithStencil()
	if err != nil {
		t.Fatalf("second ensurePipelineWithStencil failed: %v", err)
	}
	if cr.pipelineWithStencil != origPipeline {
		t.Error("pipelineWithStencil was recreated unnecessarily")
	}
}

func TestConvexRendererDestroy(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	cr := NewConvexRenderer(device, queue)

	err := cr.ensurePipelineWithStencil()
	if err != nil {
		t.Fatalf("ensurePipelineWithStencil failed: %v", err)
	}

	cr.Destroy()

	if cr.shader != nil {
		t.Error("expected nil shader after Destroy")
	}
	if cr.uniformLayout != nil {
		t.Error("expected nil uniformLayout after Destroy")
	}
	if cr.pipeLayout != nil {
		t.Error("expected nil pipeLayout after Destroy")
	}
	if cr.pipeline != nil {
		t.Error("expected nil pipeline after Destroy")
	}
	if cr.pipelineWithStencil != nil {
		t.Error("expected nil pipelineWithStencil after Destroy")
	}

	// Double-destroy should be safe.
	cr.Destroy()
}

func TestConvexRendererDestroyBeforeCreate(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	cr := NewConvexRenderer(device, queue)

	// Destroying without creating should not panic.
	cr.Destroy()
}

func TestConvexRendererShaderCompilation(t *testing.T) {
	if convexShaderSource == "" {
		t.Fatal("convex shader source is empty")
	}

	device, _, cleanup := createNoopDevice(t)
	defer cleanup()

	module, err := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "test_convex_shader",
		Source: hal.ShaderSource{WGSL: convexShaderSource},
	})
	if err != nil {
		t.Fatalf("shader compilation failed: %v", err)
	}
	if module == nil {
		t.Error("expected non-nil shader module")
	}
}

func TestConvexRendererRecordDrawsEmpty(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	cr := NewConvexRenderer(device, queue)
	defer cr.Destroy()

	err := cr.ensurePipelineWithStencil()
	if err != nil {
		t.Fatalf("ensurePipelineWithStencil failed: %v", err)
	}

	// RecordDraws with nil resources should be a no-op (not panic).
	// We cannot call it without a real render pass, but we can verify the nil guard.
	cr.RecordDraws(nil, nil)

	// RecordDraws with zero vertex count should be a no-op.
	cr.RecordDraws(nil, &convexFrameResources{vertCount: 0})
}

func TestConvexVertexLayout(t *testing.T) {
	layout := convexVertexLayout()
	if len(layout) != 1 {
		t.Fatalf("expected 1 buffer layout, got %d", len(layout))
	}

	vbl := layout[0]
	if vbl.ArrayStride != convexVertexStride {
		t.Errorf("expected stride %d, got %d", convexVertexStride, vbl.ArrayStride)
	}

	// 3 attributes: position, coverage, color.
	if len(vbl.Attributes) != 3 {
		t.Fatalf("expected 3 attributes, got %d", len(vbl.Attributes))
	}

	// Verify position at offset 0, location 0.
	if vbl.Attributes[0].Offset != 0 || vbl.Attributes[0].ShaderLocation != 0 {
		t.Errorf("position attribute: offset=%d location=%d, expected offset=0 location=0",
			vbl.Attributes[0].Offset, vbl.Attributes[0].ShaderLocation)
	}

	// Verify coverage at offset 8, location 1.
	if vbl.Attributes[1].Offset != 8 || vbl.Attributes[1].ShaderLocation != 1 {
		t.Errorf("coverage attribute: offset=%d location=%d, expected offset=8 location=1",
			vbl.Attributes[1].Offset, vbl.Attributes[1].ShaderLocation)
	}

	// Verify color at offset 12, location 2.
	if vbl.Attributes[2].Offset != 12 || vbl.Attributes[2].ShaderLocation != 2 {
		t.Errorf("color attribute: offset=%d location=%d, expected offset=12 location=2",
			vbl.Attributes[2].Offset, vbl.Attributes[2].ShaderLocation)
	}
}

func TestBuildConvexVerticesTriangle(t *testing.T) {
	commands := []ConvexDrawCommand{
		{
			Points: []gg.Point{
				gg.Pt(0, 0),
				gg.Pt(100, 0),
				gg.Pt(50, 100),
			},
			Color: [4]float32{1, 0, 0, 1},
		},
	}

	data := BuildConvexVertices(commands)

	// 3 edges * 9 vertices = 27 vertices * 28 bytes = 756 bytes.
	expectedVerts := 3 * 9
	expectedLen := expectedVerts * convexVertexStride
	if len(data) != expectedLen {
		t.Fatalf("expected %d bytes, got %d", expectedLen, len(data))
	}

	// Read first vertex (should be centroid with coverage=1.0).
	px := math.Float32frombits(binary.LittleEndian.Uint32(data[0:4]))
	py := math.Float32frombits(binary.LittleEndian.Uint32(data[4:8]))
	cov := math.Float32frombits(binary.LittleEndian.Uint32(data[8:12]))

	expectedCX := float32(50.0)
	expectedCY := float32(100.0 / 3.0)

	if math.Abs(float64(px-expectedCX)) > 0.01 {
		t.Errorf("centroid px = %f, expected %f", px, expectedCX)
	}
	if math.Abs(float64(py-expectedCY)) > 0.01 {
		t.Errorf("centroid py = %f, expected %f", py, expectedCY)
	}
	if cov != 1.0 {
		t.Errorf("centroid coverage = %f, expected 1.0", cov)
	}

	// Check color on first vertex.
	colorR := math.Float32frombits(binary.LittleEndian.Uint32(data[12:16]))
	colorA := math.Float32frombits(binary.LittleEndian.Uint32(data[24:28]))
	if colorR != 1.0 {
		t.Errorf("first vertex colorR = %f, expected 1.0", colorR)
	}
	if colorA != 1.0 {
		t.Errorf("first vertex colorA = %f, expected 1.0", colorA)
	}
}

func TestBuildConvexVerticesSquare(t *testing.T) {
	commands := []ConvexDrawCommand{
		{
			Points: []gg.Point{
				gg.Pt(0, 0),
				gg.Pt(100, 0),
				gg.Pt(100, 100),
				gg.Pt(0, 100),
			},
			Color: [4]float32{0, 1, 0, 1},
		},
	}

	data := BuildConvexVertices(commands)

	// 4 edges * 9 vertices = 36 vertices * 28 bytes = 1008 bytes.
	expectedVerts := 4 * 9
	expectedLen := expectedVerts * convexVertexStride
	if len(data) != expectedLen {
		t.Fatalf("expected %d bytes, got %d", expectedLen, len(data))
	}

	// Verify centroid is at (50, 50).
	px := math.Float32frombits(binary.LittleEndian.Uint32(data[0:4]))
	py := math.Float32frombits(binary.LittleEndian.Uint32(data[4:8]))
	if math.Abs(float64(px-50)) > 0.01 || math.Abs(float64(py-50)) > 0.01 {
		t.Errorf("centroid = (%f, %f), expected (50, 50)", px, py)
	}
}

func TestBuildConvexVerticesWithAA(t *testing.T) {
	// Triangle.
	commands := []ConvexDrawCommand{
		{
			Points: []gg.Point{
				gg.Pt(100, 0),
				gg.Pt(200, 100),
				gg.Pt(0, 100),
			},
			Color: [4]float32{0, 0, 1, 1},
		},
	}

	data := BuildConvexVertices(commands)
	if len(data) == 0 {
		t.Fatal("expected non-empty vertex data")
	}

	// For the first edge, vertices 0-2 are interior (coverage=1.0),
	// vertices 3-8 are fringe (mix of 1.0 and 0.0).
	// Check that the AA fringe vertices exist with coverage=0.0.
	foundZeroCoverage := false
	vertCount := len(data) / convexVertexStride
	for v := 0; v < vertCount; v++ {
		off := v * convexVertexStride
		cov := math.Float32frombits(binary.LittleEndian.Uint32(data[off+8 : off+12]))
		if cov == 0.0 {
			foundZeroCoverage = true
			break
		}
	}
	if !foundZeroCoverage {
		t.Error("expected at least one vertex with coverage=0.0 in AA fringe")
	}

	// Verify fringe vertices are expanded outward.
	// The interior vertex at index 3 should have coverage=1.0,
	// and the fringe vertex at index 5 should have coverage=0.0.
	interiorOff := 3 * convexVertexStride
	fringeOff := 5 * convexVertexStride
	interiorCov := math.Float32frombits(binary.LittleEndian.Uint32(data[interiorOff+8 : interiorOff+12]))
	fringeCov := math.Float32frombits(binary.LittleEndian.Uint32(data[fringeOff+8 : fringeOff+12]))
	if interiorCov != 1.0 {
		t.Errorf("interior fringe vertex coverage = %f, expected 1.0", interiorCov)
	}
	if fringeCov != 0.0 {
		t.Errorf("outer fringe vertex coverage = %f, expected 0.0", fringeCov)
	}
}

func TestBuildConvexVerticesEmpty(t *testing.T) {
	// No commands.
	data := BuildConvexVertices(nil)
	if len(data) != 0 {
		t.Errorf("expected 0 bytes for nil commands, got %d", len(data))
	}

	data = BuildConvexVertices([]ConvexDrawCommand{})
	if len(data) != 0 {
		t.Errorf("expected 0 bytes for empty commands, got %d", len(data))
	}
}

func TestBuildConvexVerticesDegeneratePolygon(t *testing.T) {
	// Polygon with fewer than 3 points should produce no vertices.
	commands := []ConvexDrawCommand{
		{
			Points: []gg.Point{gg.Pt(0, 0), gg.Pt(100, 100)},
			Color:  [4]float32{1, 0, 0, 1},
		},
	}
	data := BuildConvexVertices(commands)
	if len(data) != 0 {
		t.Errorf("expected 0 bytes for 2-point polygon, got %d", len(data))
	}

	// Single point.
	commands = []ConvexDrawCommand{
		{
			Points: []gg.Point{gg.Pt(50, 50)},
			Color:  [4]float32{1, 0, 0, 1},
		},
	}
	data = BuildConvexVertices(commands)
	if len(data) != 0 {
		t.Errorf("expected 0 bytes for 1-point polygon, got %d", len(data))
	}
}

func TestBuildConvexVerticesMultipleCommands(t *testing.T) {
	commands := []ConvexDrawCommand{
		{
			Points: []gg.Point{
				gg.Pt(0, 0), gg.Pt(100, 0), gg.Pt(50, 100),
			},
			Color: [4]float32{1, 0, 0, 1},
		},
		{
			Points: []gg.Point{
				gg.Pt(200, 200), gg.Pt(300, 200),
				gg.Pt(300, 300), gg.Pt(200, 300),
			},
			Color: [4]float32{0, 1, 0, 1},
		},
	}

	data := BuildConvexVertices(commands)

	// Triangle: 3*9 = 27 vertices. Square: 4*9 = 36 vertices. Total: 63.
	expectedVerts := 27 + 36
	expectedLen := expectedVerts * convexVertexStride
	if len(data) != expectedLen {
		t.Fatalf("expected %d bytes, got %d", expectedLen, len(data))
	}

	// Verify the second command's color appears in the second polygon's vertices.
	// First polygon ends at vertex 27. Second polygon starts at vertex 27.
	secondStart := 27 * convexVertexStride
	colorG := math.Float32frombits(binary.LittleEndian.Uint32(data[secondStart+16 : secondStart+20]))
	if colorG != 1.0 {
		t.Errorf("second polygon colorG = %f, expected 1.0", colorG)
	}
}

func TestConvexDrawCommandFields(t *testing.T) {
	cmd := ConvexDrawCommand{
		Points: []gg.Point{
			gg.Pt(0, 0), gg.Pt(100, 0), gg.Pt(100, 100),
		},
		Color: [4]float32{0.5, 0.25, 0.125, 0.75},
	}

	if len(cmd.Points) != 3 {
		t.Errorf("expected 3 points, got %d", len(cmd.Points))
	}
	if cmd.Color[0] != 0.5 || cmd.Color[3] != 0.75 {
		t.Errorf("unexpected color: %v", cmd.Color)
	}
}

func TestConvexVertexCount(t *testing.T) {
	tests := []struct {
		name     string
		commands []ConvexDrawCommand
		want     uint32
	}{
		{
			name:     "nil commands",
			commands: nil,
			want:     0,
		},
		{
			name:     "empty commands",
			commands: []ConvexDrawCommand{},
			want:     0,
		},
		{
			name: "triangle",
			commands: []ConvexDrawCommand{
				{Points: []gg.Point{gg.Pt(0, 0), gg.Pt(1, 0), gg.Pt(0, 1)}},
			},
			want: 27, // 3 * 9
		},
		{
			name: "square",
			commands: []ConvexDrawCommand{
				{Points: []gg.Point{gg.Pt(0, 0), gg.Pt(1, 0), gg.Pt(1, 1), gg.Pt(0, 1)}},
			},
			want: 36, // 4 * 9
		},
		{
			name: "pentagon",
			commands: []ConvexDrawCommand{
				{Points: makeRegularPolygon(0, 0, 50, 5)},
			},
			want: 45, // 5 * 9
		},
		{
			name: "degenerate two points",
			commands: []ConvexDrawCommand{
				{Points: []gg.Point{gg.Pt(0, 0), gg.Pt(1, 0)}},
			},
			want: 0,
		},
		{
			name: "mixed valid and degenerate",
			commands: []ConvexDrawCommand{
				{Points: []gg.Point{gg.Pt(0, 0), gg.Pt(1, 0), gg.Pt(0, 1)}},              // 27
				{Points: []gg.Point{gg.Pt(0, 0)}},                                        // 0
				{Points: []gg.Point{gg.Pt(0, 0), gg.Pt(1, 0), gg.Pt(1, 1), gg.Pt(0, 1)}}, // 36
			},
			want: 63,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convexVertexCount(tt.commands)
			if got != tt.want {
				t.Errorf("convexVertexCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWriteConvexVertex(t *testing.T) {
	buf := make([]byte, convexVertexStride)
	color := [4]float32{0.5, 0.25, 0.125, 0.75}
	writeConvexVertex(buf, 100, 200, 0.8, color)

	// Verify position.
	px := math.Float32frombits(binary.LittleEndian.Uint32(buf[0:4]))
	py := math.Float32frombits(binary.LittleEndian.Uint32(buf[4:8]))
	if px != 100 || py != 200 {
		t.Errorf("position = (%f, %f), expected (100, 200)", px, py)
	}

	// Verify coverage.
	cov := math.Float32frombits(binary.LittleEndian.Uint32(buf[8:12]))
	if cov != 0.8 {
		t.Errorf("coverage = %f, expected 0.8", cov)
	}

	// Verify color.
	cr := math.Float32frombits(binary.LittleEndian.Uint32(buf[12:16]))
	cg := math.Float32frombits(binary.LittleEndian.Uint32(buf[16:20]))
	cb := math.Float32frombits(binary.LittleEndian.Uint32(buf[20:24]))
	ca := math.Float32frombits(binary.LittleEndian.Uint32(buf[24:28]))
	if cr != 0.5 || cg != 0.25 || cb != 0.125 || ca != 0.75 {
		t.Errorf("color = (%f, %f, %f, %f), expected (0.5, 0.25, 0.125, 0.75)", cr, cg, cb, ca)
	}
}

func TestBuildConvexVerticesNormalDirection(t *testing.T) {
	// For a CCW square, the AA fringe normals should point outward.
	// Test that fringe vertices are outside the polygon bounds.
	commands := []ConvexDrawCommand{
		{
			Points: []gg.Point{
				gg.Pt(10, 10),
				gg.Pt(90, 10),
				gg.Pt(90, 90),
				gg.Pt(10, 90),
			},
			Color: [4]float32{1, 1, 1, 1},
		},
	}

	data := BuildConvexVertices(commands)
	vertCount := len(data) / convexVertexStride

	// Find all vertices with coverage=0.0 (fringe outer vertices).
	for v := 0; v < vertCount; v++ {
		off := v * convexVertexStride
		cov := math.Float32frombits(binary.LittleEndian.Uint32(data[off+8 : off+12]))
		if cov == 0.0 {
			px := math.Float32frombits(binary.LittleEndian.Uint32(data[off : off+4]))
			py := math.Float32frombits(binary.LittleEndian.Uint32(data[off+4 : off+8]))
			// Fringe vertices should be outside the 10..90 range.
			if px > 10.1 && px < 89.9 && py > 10.1 && py < 89.9 {
				t.Errorf("fringe vertex (%f, %f) is inside polygon bounds [10,90]", px, py)
			}
		}
	}
}

func TestBuildConvexVerticesRegularPolygons(t *testing.T) {
	// Test that regular polygons with 3 to 8 sides produce valid vertex data.
	for n := 3; n <= 8; n++ {
		points := makeRegularPolygon(100, 100, 50, n)
		commands := []ConvexDrawCommand{
			{
				Points: points,
				Color:  [4]float32{1, 0, 0, 1},
			},
		}
		data := BuildConvexVertices(commands)
		expectedLen := n * 9 * convexVertexStride
		if len(data) != expectedLen {
			t.Errorf("%d-gon: expected %d bytes, got %d", n, expectedLen, len(data))
		}
	}
}

func TestConvexFrameResourcesDestroy(t *testing.T) {
	device, _, cleanup := createNoopDevice(t)
	defer cleanup()

	// Destroying nil resources should not panic.
	r := &convexFrameResources{}
	r.destroy(device)

	// Destroying with nil fields should be safe.
	r2 := &convexFrameResources{
		vertBuf:    nil,
		uniformBuf: nil,
		bindGroup:  nil,
	}
	r2.destroy(device)
}

// --- extractConvexPolygon tests ---

func TestExtractConvexPolygonTriangle(t *testing.T) {
	p := gg.NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(50, 100)
	p.Close()

	points, ok := extractConvexPolygon(p)
	if !ok {
		t.Fatal("expected convex polygon for triangle")
	}
	if len(points) != 3 {
		t.Errorf("expected 3 points, got %d", len(points))
	}
}

func TestExtractConvexPolygonSquare(t *testing.T) {
	p := gg.NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(100, 100)
	p.LineTo(0, 100)
	p.Close()

	points, ok := extractConvexPolygon(p)
	if !ok {
		t.Fatal("expected convex polygon for square")
	}
	if len(points) != 4 {
		t.Errorf("expected 4 points, got %d", len(points))
	}
}

func TestExtractConvexPolygonNotClosed(t *testing.T) {
	p := gg.NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(50, 100)
	// No Close()

	_, ok := extractConvexPolygon(p)
	if ok {
		t.Error("expected false for unclosed path")
	}
}

func TestExtractConvexPolygonWithCurves(t *testing.T) {
	p := gg.NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.CubicTo(100, 50, 100, 50, 50, 100)
	p.Close()

	_, ok := extractConvexPolygon(p)
	if ok {
		t.Error("expected false for path with cubic curves")
	}
}

func TestExtractConvexPolygonConcave(t *testing.T) {
	// L-shaped concave polygon.
	p := gg.NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.LineTo(100, 50)
	p.LineTo(50, 50)
	p.LineTo(50, 100)
	p.LineTo(0, 100)
	p.Close()

	_, ok := extractConvexPolygon(p)
	if ok {
		t.Error("expected false for concave L-shape")
	}
}

func TestExtractConvexPolygonMultipleSubpaths(t *testing.T) {
	p := gg.NewPath()
	p.MoveTo(0, 0)
	p.LineTo(50, 0)
	p.LineTo(25, 50)
	p.Close()
	// Second subpath.
	p.MoveTo(100, 100)
	p.LineTo(150, 100)
	p.LineTo(125, 150)
	p.Close()

	_, ok := extractConvexPolygon(p)
	if ok {
		t.Error("expected false for multiple subpaths")
	}
}

func TestExtractConvexPolygonTooFewPoints(t *testing.T) {
	p := gg.NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.Close()

	_, ok := extractConvexPolygon(p)
	if ok {
		t.Error("expected false for 2-point path")
	}
}

func TestExtractConvexPolygonEmptyPath(t *testing.T) {
	p := gg.NewPath()

	_, ok := extractConvexPolygon(p)
	if ok {
		t.Error("expected false for empty path")
	}
}

func TestExtractConvexPolygonPentagon(t *testing.T) {
	// Regular pentagon is convex.
	pts := makeRegularPolygon(100, 100, 50, 5)
	p := gg.NewPath()
	p.MoveTo(pts[0].X, pts[0].Y)
	for i := 1; i < len(pts); i++ {
		p.LineTo(pts[i].X, pts[i].Y)
	}
	p.Close()

	points, ok := extractConvexPolygon(p)
	if !ok {
		t.Fatal("expected convex polygon for regular pentagon")
	}
	if len(points) != 5 {
		t.Errorf("expected 5 points, got %d", len(points))
	}
}

func TestExtractConvexPolygonWithQuadCurve(t *testing.T) {
	p := gg.NewPath()
	p.MoveTo(0, 0)
	p.LineTo(100, 0)
	p.QuadraticTo(100, 100, 0, 100)
	p.Close()

	_, ok := extractConvexPolygon(p)
	if ok {
		t.Error("expected false for path with quadratic curves")
	}
}

// --- Render session convex integration tests ---

func TestRenderSessionConvexOnly(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	target := gg.GPURenderTarget{
		Width:  200,
		Height: 200,
		Data:   make([]uint8, 200*200*4),
		Stride: 200 * 4,
	}

	commands := []ConvexDrawCommand{
		{
			Points: []gg.Point{
				gg.Pt(50, 50), gg.Pt(150, 50), gg.Pt(100, 150),
			},
			Color: [4]float32{1, 0, 0, 1},
		},
	}

	err := s.RenderFrame(target, nil, commands, nil)
	if err != nil {
		t.Fatalf("RenderFrame with convex commands failed: %v", err)
	}

	// Convex renderer should have been created.
	if s.convexRenderer == nil {
		t.Error("expected non-nil convex renderer after render")
	}
	if s.convexRenderer.pipelineWithStencil == nil {
		t.Error("expected non-nil pipelineWithStencil after render")
	}
}

func TestRenderSessionMixedWithConvex(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	target := gg.GPURenderTarget{
		Width:  400,
		Height: 300,
		Data:   make([]uint8, 400*300*4),
		Stride: 400 * 4,
	}

	// All three tiers at once.
	shapes := []SDFRenderShape{
		{Kind: 0, CenterX: 100, CenterY: 100, Param1: 40, Param2: 40, ColorA: 1},
	}
	convexCmds := []ConvexDrawCommand{
		{
			Points: []gg.Point{gg.Pt(200, 50), gg.Pt(280, 50), gg.Pt(240, 130)},
			Color:  [4]float32{0, 1, 0, 1},
		},
	}
	paths := []StencilPathCommand{
		{
			Vertices:  []float32{300, 200, 350, 200, 350, 250},
			CoverQuad: [12]float32{299, 199, 351, 199, 351, 251, 299, 199, 351, 251, 299, 251},
			Color:     [4]float32{0, 0, 1, 1},
			FillRule:  gg.FillRuleNonZero,
		},
	}

	err := s.RenderFrame(target, shapes, convexCmds, paths)
	if err != nil {
		t.Fatalf("RenderFrame with all three tiers failed: %v", err)
	}

	// All renderer types should be initialized.
	if s.sdfPipeline == nil || s.sdfPipeline.pipelineWithStencil == nil {
		t.Error("expected SDF pipelines")
	}
	if s.convexRenderer == nil || s.convexRenderer.pipelineWithStencil == nil {
		t.Error("expected convex pipelines")
	}
	if s.stencilRenderer == nil || s.stencilRenderer.nonZeroStencilPipeline == nil {
		t.Error("expected stencil pipelines")
	}
}

func TestRenderSessionConvexRendererSetter(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	s := NewGPURenderSession(device, queue)
	defer s.Destroy()

	if s.ConvexRendererRef() != nil {
		t.Error("expected nil convex renderer initially")
	}

	cr := NewConvexRenderer(device, queue)
	defer cr.Destroy()

	s.SetConvexRenderer(cr)

	if s.ConvexRendererRef() != cr {
		t.Error("SetConvexRenderer did not set correctly")
	}
}

// --- Benchmarks ---

func BenchmarkBuildConvexVerticesTriangle(b *testing.B) {
	commands := []ConvexDrawCommand{
		{
			Points: []gg.Point{gg.Pt(0, 0), gg.Pt(100, 0), gg.Pt(50, 100)},
			Color:  [4]float32{1, 0, 0, 1},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildConvexVertices(commands)
	}
}

func BenchmarkBuildConvexVerticesHexagon(b *testing.B) {
	commands := []ConvexDrawCommand{
		{
			Points: makeRegularPolygon(100, 100, 50, 6),
			Color:  [4]float32{1, 0, 0, 1},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildConvexVertices(commands)
	}
}

func BenchmarkBuildConvexVertices100Gon(b *testing.B) {
	commands := []ConvexDrawCommand{
		{
			Points: makeRegularPolygon(100, 100, 50, 100),
			Color:  [4]float32{1, 0, 0, 1},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildConvexVertices(commands)
	}
}
