//go:build !nogpu

package gpu

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/wgpu/hal"
)

func TestSDFRenderPipelineCreation(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewSDFRenderPipeline(device, queue)
	defer p.Destroy()

	err := p.createPipeline()
	if err != nil {
		t.Fatalf("createPipeline failed: %v", err)
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
}

func TestSDFRenderPipelineDestroy(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewSDFRenderPipeline(device, queue)

	err := p.createPipeline()
	if err != nil {
		t.Fatalf("createPipeline failed: %v", err)
	}

	if p.pipeline == nil {
		t.Fatal("expected non-nil pipeline before destroy")
	}

	p.destroyPipeline()

	if p.shader != nil {
		t.Error("expected nil shader after destroy")
	}
	if p.uniformLayout != nil {
		t.Error("expected nil uniformLayout after destroy")
	}
	if p.pipeLayout != nil {
		t.Error("expected nil pipeLayout after destroy")
	}
	if p.pipeline != nil {
		t.Error("expected nil pipeline after destroy")
	}

	// Double-destroy should be safe.
	p.destroyPipeline()
}

func TestSDFRenderPipelineDestroyBeforeCreate(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewSDFRenderPipeline(device, queue)

	// Destroying a pipeline that was never created should not panic.
	p.destroyPipeline()
}

func TestSDFRenderPipelineTextures(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewSDFRenderPipeline(device, queue)
	defer p.Destroy()

	err := p.ensureTextures(800, 600)
	if err != nil {
		t.Fatalf("ensureTextures failed: %v", err)
	}

	if p.msaaTex == nil {
		t.Error("expected non-nil msaaTex")
	}
	if p.msaaView == nil {
		t.Error("expected non-nil msaaView")
	}
	if p.resolveTex == nil {
		t.Error("expected non-nil resolveTex")
	}
	if p.resolveView == nil {
		t.Error("expected non-nil resolveView")
	}

	w, h := p.Size()
	if w != 800 || h != 600 {
		t.Errorf("expected size (800, 600), got (%d, %d)", w, h)
	}
}

func TestSDFRenderPipelineTexturesIdempotent(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewSDFRenderPipeline(device, queue)
	defer p.Destroy()

	err := p.ensureTextures(640, 480)
	if err != nil {
		t.Fatalf("first ensureTextures failed: %v", err)
	}

	origMSAA := p.msaaTex
	origResolve := p.resolveTex

	// Same dimensions should be a no-op.
	err = p.ensureTextures(640, 480)
	if err != nil {
		t.Fatalf("second ensureTextures failed: %v", err)
	}

	if p.msaaTex != origMSAA {
		t.Error("MSAA texture was recreated unnecessarily")
	}
	if p.resolveTex != origResolve {
		t.Error("resolve texture was recreated unnecessarily")
	}
}

func TestSDFRenderPipelineTexturesResize(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewSDFRenderPipeline(device, queue)
	defer p.Destroy()

	err := p.ensureTextures(800, 600)
	if err != nil {
		t.Fatalf("initial ensureTextures failed: %v", err)
	}

	w, h := p.Size()
	if w != 800 || h != 600 {
		t.Errorf("expected (800, 600), got (%d, %d)", w, h)
	}

	// Resize triggers recreation.
	err = p.ensureTextures(1920, 1080)
	if err != nil {
		t.Fatalf("resize ensureTextures failed: %v", err)
	}

	w, h = p.Size()
	if w != 1920 || h != 1080 {
		t.Errorf("expected (1920, 1080), got (%d, %d)", w, h)
	}

	if p.msaaTex == nil || p.resolveTex == nil {
		t.Error("expected non-nil textures after resize")
	}
}

func TestSDFRenderPipelineFullDestroy(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewSDFRenderPipeline(device, queue)

	err := p.ensureTextures(512, 512)
	if err != nil {
		t.Fatalf("ensureTextures failed: %v", err)
	}
	err = p.createPipeline()
	if err != nil {
		t.Fatalf("createPipeline failed: %v", err)
	}

	p.Destroy()

	if p.pipeline != nil {
		t.Error("expected nil pipeline after Destroy")
	}
	if p.msaaTex != nil {
		t.Error("expected nil msaaTex after Destroy")
	}
	if p.resolveTex != nil {
		t.Error("expected nil resolveTex after Destroy")
	}

	w, h := p.Size()
	if w != 0 || h != 0 {
		t.Errorf("expected (0, 0) after Destroy, got (%d, %d)", w, h)
	}

	// Double-destroy should be safe.
	p.Destroy()
}

func TestSDFRenderPipelineRecreate(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewSDFRenderPipeline(device, queue)
	defer p.Destroy()

	// Create, destroy, recreate.
	err := p.createPipeline()
	if err != nil {
		t.Fatalf("first createPipeline failed: %v", err)
	}
	p.destroyPipeline()

	err = p.createPipeline()
	if err != nil {
		t.Fatalf("second createPipeline failed: %v", err)
	}

	if p.pipeline == nil {
		t.Error("expected non-nil pipeline after recreate")
	}
}

func TestSDFRenderShaderCompilation(t *testing.T) {
	if sdfRenderShaderSource == "" {
		t.Fatal("sdf_render shader source is empty")
	}

	device, _, cleanup := createNoopDevice(t)
	defer cleanup()

	module, err := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "test_sdf_render",
		Source: hal.ShaderSource{WGSL: sdfRenderShaderSource},
	})
	if err != nil {
		t.Fatalf("shader compilation failed: %v", err)
	}
	if module == nil {
		t.Error("expected non-nil shader module")
	}
}

func TestSDFRenderVertexLayout(t *testing.T) {
	layout := sdfRenderVertexLayout()
	if len(layout) != 1 {
		t.Fatalf("expected 1 buffer layout, got %d", len(layout))
	}

	vbl := layout[0]
	if vbl.ArrayStride != sdfRenderVertexStride {
		t.Errorf("expected stride %d, got %d", sdfRenderVertexStride, vbl.ArrayStride)
	}

	// 9 attributes: position, local, shape_kind, param1..4, half_stroke, is_stroked, color
	if len(vbl.Attributes) != 9 {
		t.Errorf("expected 9 attributes, got %d", len(vbl.Attributes))
	}

	// Verify first attribute is position at offset 0.
	if vbl.Attributes[0].Offset != 0 || vbl.Attributes[0].ShaderLocation != 0 {
		t.Errorf("first attribute: offset=%d location=%d, expected offset=0 location=0",
			vbl.Attributes[0].Offset, vbl.Attributes[0].ShaderLocation)
	}

	// Verify last attribute (color) at offset 40, location 8.
	last := vbl.Attributes[len(vbl.Attributes)-1]
	if last.Offset != 40 || last.ShaderLocation != 8 {
		t.Errorf("last attribute: offset=%d location=%d, expected offset=40 location=8",
			last.Offset, last.ShaderLocation)
	}
}

func TestBuildSDFRenderVerticesCircle(t *testing.T) {
	shapes := []SDFRenderShape{
		{
			Kind: 0, CenterX: 100, CenterY: 100,
			Param1: 50, Param2: 50, Param3: 0,
			HalfStroke: 0, IsStroked: 0,
			ColorR: 1, ColorG: 0, ColorB: 0, ColorA: 1,
		},
	}

	data := buildSDFRenderVertices(shapes, 200, 200)

	// 1 shape * 6 vertices * 56 bytes = 336 bytes.
	expectedLen := 1 * 6 * sdfRenderVertexStride
	if len(data) != expectedLen {
		t.Fatalf("expected %d bytes, got %d", expectedLen, len(data))
	}

	// Read first vertex (top-left corner).
	px := math.Float32frombits(binary.LittleEndian.Uint32(data[0:4]))
	py := math.Float32frombits(binary.LittleEndian.Uint32(data[4:8]))
	lx := math.Float32frombits(binary.LittleEndian.Uint32(data[8:12]))
	ly := math.Float32frombits(binary.LittleEndian.Uint32(data[12:16]))

	// halfW = 50 + 0 + 1.5 = 51.5
	expectedHalf := float32(50 + sdfRenderAAMargin)
	expectedPX := float32(100) - expectedHalf
	expectedPY := float32(100) - expectedHalf
	expectedLX := -expectedHalf
	expectedLY := -expectedHalf

	if math.Abs(float64(px-expectedPX)) > 0.01 {
		t.Errorf("first vertex px = %f, expected %f", px, expectedPX)
	}
	if math.Abs(float64(py-expectedPY)) > 0.01 {
		t.Errorf("first vertex py = %f, expected %f", py, expectedPY)
	}
	if math.Abs(float64(lx-expectedLX)) > 0.01 {
		t.Errorf("first vertex lx = %f, expected %f", lx, expectedLX)
	}
	if math.Abs(float64(ly-expectedLY)) > 0.01 {
		t.Errorf("first vertex ly = %f, expected %f", ly, expectedLY)
	}

	// Check color at first vertex (offset 40).
	colorR := math.Float32frombits(binary.LittleEndian.Uint32(data[40:44]))
	colorA := math.Float32frombits(binary.LittleEndian.Uint32(data[52:56]))
	if colorR != 1.0 {
		t.Errorf("first vertex colorR = %f, expected 1.0", colorR)
	}
	if colorA != 1.0 {
		t.Errorf("first vertex colorA = %f, expected 1.0", colorA)
	}
}

func TestBuildSDFRenderVerticesRRect(t *testing.T) {
	shapes := []SDFRenderShape{
		{
			Kind: 1, CenterX: 200, CenterY: 150,
			Param1: 80, Param2: 60, Param3: 10,
			HalfStroke: 0, IsStroked: 0,
			ColorR: 0, ColorG: 0.5, ColorB: 1, ColorA: 1,
		},
	}

	data := buildSDFRenderVertices(shapes, 400, 300)
	expectedLen := 6 * sdfRenderVertexStride
	if len(data) != expectedLen {
		t.Fatalf("expected %d bytes, got %d", expectedLen, len(data))
	}

	// Read shape_kind from first vertex.
	kind := math.Float32frombits(binary.LittleEndian.Uint32(data[16:20]))
	if kind != 1.0 {
		t.Errorf("shape_kind = %f, expected 1.0", kind)
	}

	// Check param1 (half_width = 80).
	p1 := math.Float32frombits(binary.LittleEndian.Uint32(data[20:24]))
	if p1 != 80.0 {
		t.Errorf("param1 = %f, expected 80.0", p1)
	}

	// Check param3 (corner_radius = 10).
	p3 := math.Float32frombits(binary.LittleEndian.Uint32(data[28:32]))
	if p3 != 10.0 {
		t.Errorf("param3 = %f, expected 10.0", p3)
	}
}

func TestBuildSDFRenderVerticesMultipleShapes(t *testing.T) {
	shapes := []SDFRenderShape{
		{Kind: 0, CenterX: 50, CenterY: 50, Param1: 30, Param2: 30, ColorA: 1},
		{Kind: 1, CenterX: 150, CenterY: 50, Param1: 40, Param2: 30, Param3: 5, ColorA: 1},
		{Kind: 0, CenterX: 250, CenterY: 50, Param1: 20, Param2: 20, ColorA: 1},
	}

	data := buildSDFRenderVertices(shapes, 300, 100)

	// 3 shapes * 6 vertices * 56 bytes.
	expectedLen := 3 * 6 * sdfRenderVertexStride
	if len(data) != expectedLen {
		t.Fatalf("expected %d bytes, got %d", expectedLen, len(data))
	}

	// Check center of each shape's first vertex has correct kind.
	for i, s := range shapes {
		vertOffset := i * 6 * sdfRenderVertexStride
		kind := math.Float32frombits(binary.LittleEndian.Uint32(data[vertOffset+16 : vertOffset+20]))
		expectedKind := float32(s.Kind)
		if kind != expectedKind {
			t.Errorf("shape %d: kind = %f, expected %f", i, kind, expectedKind)
		}
	}
}

func TestBuildSDFRenderVerticesStrokedShape(t *testing.T) {
	shapes := []SDFRenderShape{
		{
			Kind: 0, CenterX: 100, CenterY: 100,
			Param1: 40, Param2: 40,
			HalfStroke: 2.5, IsStroked: 1.0,
			ColorR: 0, ColorG: 1, ColorB: 0, ColorA: 1,
		},
	}

	data := buildSDFRenderVertices(shapes, 200, 200)

	// First vertex: check half_stroke and is_stroked values.
	hs := math.Float32frombits(binary.LittleEndian.Uint32(data[32:36]))
	is := math.Float32frombits(binary.LittleEndian.Uint32(data[36:40]))
	if hs != 2.5 {
		t.Errorf("half_stroke = %f, expected 2.5", hs)
	}
	if is != 1.0 {
		t.Errorf("is_stroked = %f, expected 1.0", is)
	}

	// The bounding quad should be expanded by stroke + AA margin.
	px := math.Float32frombits(binary.LittleEndian.Uint32(data[0:4]))
	// halfW = 40 + 2.5 + 1.5 = 44
	expectedPX := float32(100 - 44)
	if math.Abs(float64(px-expectedPX)) > 0.01 {
		t.Errorf("stroked vertex px = %f, expected %f", px, expectedPX)
	}
}

func TestBuildSDFRenderVerticesEmpty(t *testing.T) {
	data := buildSDFRenderVertices(nil, 100, 100)
	if len(data) != 0 {
		t.Errorf("expected 0 bytes for empty shapes, got %d", len(data))
	}
}

func TestMakeSDFRenderUniform(t *testing.T) {
	data := makeSDFRenderUniform(800, 600)
	if len(data) != sdfRenderUniformSize {
		t.Fatalf("expected %d bytes, got %d", sdfRenderUniformSize, len(data))
	}

	w := math.Float32frombits(binary.LittleEndian.Uint32(data[0:4]))
	h := math.Float32frombits(binary.LittleEndian.Uint32(data[4:8]))
	if w != 800.0 {
		t.Errorf("viewport width = %f, expected 800.0", w)
	}
	if h != 600.0 {
		t.Errorf("viewport height = %f, expected 600.0", h)
	}

	// Padding should be zero.
	pad1 := binary.LittleEndian.Uint32(data[8:12])
	pad2 := binary.LittleEndian.Uint32(data[12:16])
	if pad1 != 0 || pad2 != 0 {
		t.Errorf("expected zero padding, got %d, %d", pad1, pad2)
	}
}

func TestDetectedShapeToRenderShapeCircle(t *testing.T) {
	shape := gg.DetectedShape{
		Kind:    gg.ShapeCircle,
		CenterX: 100, CenterY: 200,
		RadiusX: 50, RadiusY: 50,
	}
	paint := &gg.Paint{Brush: gg.SolidBrush{Color: gg.RGBA{R: 1, G: 0, B: 0, A: 1}}}

	rs, ok := DetectedShapeToRenderShape(shape, paint, false)
	if !ok {
		t.Fatal("expected ok=true for circle")
	}
	if rs.Kind != 0 {
		t.Errorf("kind = %d, expected 0", rs.Kind)
	}
	if rs.CenterX != 100 {
		t.Errorf("CenterX = %f, expected 100", rs.CenterX)
	}
	if rs.Param1 != 50 {
		t.Errorf("Param1 = %f, expected 50", rs.Param1)
	}
	if rs.IsStroked != 0 {
		t.Errorf("IsStroked = %f, expected 0", rs.IsStroked)
	}
	// Premultiplied red: R=1*1=1, A=1.
	if rs.ColorR != 1.0 || rs.ColorA != 1.0 {
		t.Errorf("color: R=%f A=%f, expected R=1 A=1", rs.ColorR, rs.ColorA)
	}
}

func TestDetectedShapeToRenderShapeEllipse(t *testing.T) {
	shape := gg.DetectedShape{
		Kind:    gg.ShapeEllipse,
		CenterX: 50, CenterY: 50,
		RadiusX: 40, RadiusY: 20,
	}
	paint := &gg.Paint{Brush: gg.SolidBrush{Color: gg.RGBA{R: 0, G: 1, B: 0, A: 0.5}}}

	rs, ok := DetectedShapeToRenderShape(shape, paint, false)
	if !ok {
		t.Fatal("expected ok=true for ellipse")
	}
	if rs.Kind != 0 {
		t.Errorf("kind = %d, expected 0", rs.Kind)
	}
	if rs.Param1 != 40 || rs.Param2 != 20 {
		t.Errorf("params = (%f, %f), expected (40, 20)", rs.Param1, rs.Param2)
	}
	// Premultiplied: G=1*0.5=0.5, A=0.5.
	if math.Abs(float64(rs.ColorG-0.5)) > 0.001 {
		t.Errorf("ColorG = %f, expected 0.5", rs.ColorG)
	}
	if math.Abs(float64(rs.ColorA-0.5)) > 0.001 {
		t.Errorf("ColorA = %f, expected 0.5", rs.ColorA)
	}
}

func TestDetectedShapeToRenderShapeRRect(t *testing.T) {
	shape := gg.DetectedShape{
		Kind:    gg.ShapeRRect,
		CenterX: 100, CenterY: 100,
		Width: 200, Height: 100,
		CornerRadius: 15,
	}
	paint := &gg.Paint{Brush: gg.SolidBrush{Color: gg.RGBA{R: 0, G: 0, B: 1, A: 1}}}

	rs, ok := DetectedShapeToRenderShape(shape, paint, false)
	if !ok {
		t.Fatal("expected ok=true for rrect")
	}
	if rs.Kind != 1 {
		t.Errorf("kind = %d, expected 1", rs.Kind)
	}
	// half_width = 200/2 = 100, half_height = 100/2 = 50.
	if rs.Param1 != 100 || rs.Param2 != 50 {
		t.Errorf("params = (%f, %f), expected (100, 50)", rs.Param1, rs.Param2)
	}
	if rs.Param3 != 15 {
		t.Errorf("Param3 = %f, expected 15", rs.Param3)
	}
}

func TestDetectedShapeToRenderShapeRect(t *testing.T) {
	shape := gg.DetectedShape{
		Kind:    gg.ShapeRect,
		CenterX: 50, CenterY: 50,
		Width: 80, Height: 60,
	}
	paint := &gg.Paint{Brush: gg.SolidBrush{Color: gg.RGBA{R: 1, G: 1, B: 1, A: 1}}}

	rs, ok := DetectedShapeToRenderShape(shape, paint, false)
	if !ok {
		t.Fatal("expected ok=true for rect")
	}
	if rs.Kind != 1 {
		t.Errorf("kind = %d, expected 1 (rects use rrect SDF with corner_radius=0)", rs.Kind)
	}
	if rs.Param3 != 0 {
		t.Errorf("Param3 = %f, expected 0 (no corner radius)", rs.Param3)
	}
}

func TestDetectedShapeToRenderShapeStroked(t *testing.T) {
	shape := gg.DetectedShape{
		Kind:    gg.ShapeCircle,
		CenterX: 100, CenterY: 100,
		RadiusX: 50, RadiusY: 50,
	}
	paint := &gg.Paint{
		Brush:     gg.SolidBrush{Color: gg.RGBA{R: 1, G: 0, B: 0, A: 1}},
		LineWidth: 4,
	}

	rs, ok := DetectedShapeToRenderShape(shape, paint, true)
	if !ok {
		t.Fatal("expected ok=true for stroked circle")
	}
	if rs.IsStroked != 1.0 {
		t.Errorf("IsStroked = %f, expected 1.0", rs.IsStroked)
	}
	if rs.HalfStroke != 2.0 {
		t.Errorf("HalfStroke = %f, expected 2.0", rs.HalfStroke)
	}
}

func TestDetectedShapeToRenderShapeUnknown(t *testing.T) {
	shape := gg.DetectedShape{Kind: gg.ShapeUnknown}
	paint := &gg.Paint{Brush: gg.SolidBrush{Color: gg.RGBA{R: 1, A: 1}}}

	_, ok := DetectedShapeToRenderShape(shape, paint, false)
	if ok {
		t.Error("expected ok=false for ShapeUnknown")
	}
}

func TestSDFRenderPipelineRenderShapesEmpty(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewSDFRenderPipeline(device, queue)
	defer p.Destroy()

	target := gg.GPURenderTarget{
		Width:  100,
		Height: 100,
		Data:   make([]uint8, 100*100*4),
		Stride: 100 * 4,
	}

	// Empty shapes should return nil without creating resources.
	err := p.RenderShapes(target, nil)
	if err != nil {
		t.Fatalf("RenderShapes(nil) failed: %v", err)
	}

	err = p.RenderShapes(target, []SDFRenderShape{})
	if err != nil {
		t.Fatalf("RenderShapes([]) failed: %v", err)
	}

	// Pipeline should not have been created for empty shapes.
	if p.pipeline != nil {
		t.Error("expected nil pipeline after empty RenderShapes")
	}
}

func TestSDFRenderPipelineEnsureReady(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewSDFRenderPipeline(device, queue)
	defer p.Destroy()

	// Before ensureReady, nothing is allocated.
	if p.pipeline != nil || p.msaaTex != nil {
		t.Fatal("expected nil resources before ensureReady")
	}

	err := p.ensureReady(800, 600)
	if err != nil {
		t.Fatalf("ensureReady failed: %v", err)
	}

	if p.pipeline == nil {
		t.Error("expected non-nil pipeline after ensureReady")
	}
	if p.msaaTex == nil {
		t.Error("expected non-nil msaaTex after ensureReady")
	}

	w, h := p.Size()
	if w != 800 || h != 600 {
		t.Errorf("expected (800, 600), got (%d, %d)", w, h)
	}
}

func TestSDFRenderPipelineTexturesAfterDestroy(t *testing.T) {
	device, queue, cleanup := createNoopDevice(t)
	defer cleanup()

	p := NewSDFRenderPipeline(device, queue)

	// Create, destroy, recreate.
	err := p.ensureTextures(256, 256)
	if err != nil {
		t.Fatalf("first ensureTextures failed: %v", err)
	}
	p.Destroy()

	w, h := p.Size()
	if w != 0 || h != 0 {
		t.Errorf("expected (0, 0) after Destroy, got (%d, %d)", w, h)
	}

	// Re-create with different dimensions.
	err = p.ensureTextures(512, 512)
	if err != nil {
		t.Fatalf("ensureTextures after Destroy failed: %v", err)
	}
	defer p.Destroy()

	w, h = p.Size()
	if w != 512 || h != 512 {
		t.Errorf("expected (512, 512), got (%d, %d)", w, h)
	}
}

func TestWriteSDFRenderVertex(t *testing.T) {
	buf := make([]byte, sdfRenderVertexStride)
	s := &SDFRenderShape{
		Kind: 1, Param1: 80, Param2: 60, Param3: 10,
		HalfStroke: 2.5, IsStroked: 1.0,
		ColorR: 0.5, ColorG: 0.25, ColorB: 0.125, ColorA: 0.75,
	}

	writeSDFRenderVertex(buf, 100, 200, -50, 30, s)

	// Verify position.
	px := math.Float32frombits(binary.LittleEndian.Uint32(buf[0:4]))
	py := math.Float32frombits(binary.LittleEndian.Uint32(buf[4:8]))
	if px != 100 || py != 200 {
		t.Errorf("position = (%f, %f), expected (100, 200)", px, py)
	}

	// Verify local.
	lx := math.Float32frombits(binary.LittleEndian.Uint32(buf[8:12]))
	ly := math.Float32frombits(binary.LittleEndian.Uint32(buf[12:16]))
	if lx != -50 || ly != 30 {
		t.Errorf("local = (%f, %f), expected (-50, 30)", lx, ly)
	}

	// Verify shape_kind.
	kind := math.Float32frombits(binary.LittleEndian.Uint32(buf[16:20]))
	if kind != 1.0 {
		t.Errorf("shape_kind = %f, expected 1.0", kind)
	}

	// Verify color.
	cr := math.Float32frombits(binary.LittleEndian.Uint32(buf[40:44]))
	cg := math.Float32frombits(binary.LittleEndian.Uint32(buf[44:48]))
	cb := math.Float32frombits(binary.LittleEndian.Uint32(buf[48:52]))
	ca := math.Float32frombits(binary.LittleEndian.Uint32(buf[52:56]))
	if cr != 0.5 || cg != 0.25 || cb != 0.125 || ca != 0.75 {
		t.Errorf("color = (%f, %f, %f, %f), expected (0.5, 0.25, 0.125, 0.75)", cr, cg, cb, ca)
	}
}
