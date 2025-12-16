package blend

import (
	"testing"

	"github.com/gogpu/gg/internal/image"
)

// TestNewLayer tests layer creation.
func TestNewLayer(t *testing.T) {
	pool := image.NewPool(4)
	bounds := Bounds{X: 0, Y: 0, Width: 100, Height: 100}

	tests := []struct {
		name      string
		blendMode BlendMode
		opacity   float64
		bounds    Bounds
		wantErr   bool
	}{
		{
			name:      "valid layer",
			blendMode: BlendSourceOver,
			opacity:   1.0,
			bounds:    bounds,
			wantErr:   false,
		},
		{
			name:      "opacity clamped to 0",
			blendMode: BlendSourceOver,
			opacity:   -0.5,
			bounds:    bounds,
			wantErr:   false,
		},
		{
			name:      "opacity clamped to 1",
			blendMode: BlendSourceOver,
			opacity:   1.5,
			bounds:    bounds,
			wantErr:   false,
		},
		{
			name:      "invalid dimensions",
			blendMode: BlendSourceOver,
			opacity:   1.0,
			bounds:    Bounds{X: 0, Y: 0, Width: 0, Height: 0},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layer, err := NewLayer(tt.blendMode, tt.opacity, tt.bounds, pool)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLayer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			// Verify layer properties
			if layer.BlendMode() != tt.blendMode {
				t.Errorf("BlendMode() = %v, want %v", layer.BlendMode(), tt.blendMode)
			}

			// Check opacity clamping
			expectedOpacity := tt.opacity
			if expectedOpacity < 0 {
				expectedOpacity = 0
			}
			if expectedOpacity > 1 {
				expectedOpacity = 1
			}
			if layer.Opacity() != expectedOpacity {
				t.Errorf("Opacity() = %v, want %v", layer.Opacity(), expectedOpacity)
			}

			if layer.Buffer() == nil {
				t.Error("Buffer() returned nil")
			}

			b := layer.Bounds()
			if b.Width != tt.bounds.Width || b.Height != tt.bounds.Height {
				t.Errorf("Bounds() = %v, want %v", b, tt.bounds)
			}

			// Return buffer to pool
			pool.Put(layer.buffer)
		})
	}
}

// TestLayerSetOpacity tests setting layer opacity.
func TestLayerSetOpacity(t *testing.T) {
	pool := image.NewPool(4)
	bounds := Bounds{X: 0, Y: 0, Width: 100, Height: 100}
	layer, err := NewLayer(BlendSourceOver, 1.0, bounds, pool)
	if err != nil {
		t.Fatalf("NewLayer() error = %v", err)
	}
	defer pool.Put(layer.buffer)

	tests := []struct {
		name     string
		opacity  float64
		expected float64
	}{
		{"set 0.5", 0.5, 0.5},
		{"set negative (clamped)", -0.5, 0.0},
		{"set > 1 (clamped)", 1.5, 1.0},
		{"set 0", 0.0, 0.0},
		{"set 1", 1.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layer.SetOpacity(tt.opacity)
			if layer.Opacity() != tt.expected {
				t.Errorf("SetOpacity(%v) resulted in %v, want %v", tt.opacity, layer.Opacity(), tt.expected)
			}
		})
	}
}

// TestNewLayerStack tests layer stack creation.
func TestNewLayerStack(t *testing.T) {
	base, err := image.NewImageBuf(200, 200, image.FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf() error = %v", err)
	}

	t.Run("with custom pool", func(t *testing.T) {
		pool := image.NewPool(4)
		stack := NewLayerStack(base, pool)
		if stack == nil {
			t.Fatal("NewLayerStack() returned nil")
		}
		if stack.Depth() != 0 {
			t.Errorf("Depth() = %v, want 0", stack.Depth())
		}
		if stack.Current() != base {
			t.Error("Current() should return base when stack is empty")
		}
	})

	t.Run("with nil pool", func(t *testing.T) {
		stack := NewLayerStack(base, nil)
		if stack == nil {
			t.Fatal("NewLayerStack() returned nil")
		}
	})
}

// TestLayerStackPushPop tests push and pop operations.
func TestLayerStackPushPop(t *testing.T) {
	base, err := image.NewImageBuf(200, 200, image.FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf() error = %v", err)
	}

	pool := image.NewPool(4)
	stack := NewLayerStack(base, pool)

	// Push first layer
	layer1, err := stack.Push(BlendSourceOver, 1.0, Bounds{0, 0, 100, 100})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if stack.Depth() != 1 {
		t.Errorf("Depth() = %v, want 1", stack.Depth())
	}
	if stack.Current() != layer1.Buffer() {
		t.Error("Current() should return top layer buffer")
	}
	if stack.CurrentBlendMode() != BlendSourceOver {
		t.Errorf("CurrentBlendMode() = %v, want BlendSourceOver", stack.CurrentBlendMode())
	}

	// Push second layer
	layer2, err := stack.Push(BlendMultiply, 0.5, Bounds{0, 0, 100, 100})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if stack.Depth() != 2 {
		t.Errorf("Depth() = %v, want 2", stack.Depth())
	}
	if stack.Current() != layer2.Buffer() {
		t.Error("Current() should return new top layer buffer")
	}
	if stack.CurrentBlendMode() != BlendMultiply {
		t.Errorf("CurrentBlendMode() = %v, want BlendMultiply", stack.CurrentBlendMode())
	}

	// Pop second layer
	result := stack.Pop()
	if result != layer1.Buffer() {
		t.Error("Pop() should return parent layer buffer")
	}
	if stack.Depth() != 1 {
		t.Errorf("Depth() = %v, want 1", stack.Depth())
	}

	// Pop first layer
	result = stack.Pop()
	if result != base {
		t.Error("Pop() should return base when popping last layer")
	}
	if stack.Depth() != 0 {
		t.Errorf("Depth() = %v, want 0", stack.Depth())
	}

	// Pop empty stack
	result = stack.Pop()
	if result != nil {
		t.Error("Pop() on empty stack should return nil")
	}
}

// TestLayerStackPushWithInvalidBounds tests pushing layers with invalid bounds.
func TestLayerStackPushWithInvalidBounds(t *testing.T) {
	base, err := image.NewImageBuf(200, 200, image.FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf() error = %v", err)
	}

	pool := image.NewPool(4)
	stack := NewLayerStack(base, pool)

	// Push with zero width/height should use base dimensions
	layer, err := stack.Push(BlendSourceOver, 1.0, Bounds{0, 0, 0, 0})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	bounds := layer.Bounds()
	if bounds.Width != 200 || bounds.Height != 200 {
		t.Errorf("Layer bounds = %v, want {0 0 200 200}", bounds)
	}

	stack.Clear()
}

// TestLayerStackClear tests clearing the layer stack.
func TestLayerStackClear(t *testing.T) {
	base, err := image.NewImageBuf(200, 200, image.FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf() error = %v", err)
	}

	pool := image.NewPool(4)
	stack := NewLayerStack(base, pool)

	// Push multiple layers
	_, _ = stack.Push(BlendSourceOver, 1.0, Bounds{0, 0, 100, 100})
	_, _ = stack.Push(BlendMultiply, 0.5, Bounds{0, 0, 100, 100})
	_, _ = stack.Push(BlendScreen, 0.8, Bounds{0, 0, 100, 100})

	if stack.Depth() != 3 {
		t.Errorf("Depth() = %v, want 3", stack.Depth())
	}

	// Clear all layers
	stack.Clear()

	if stack.Depth() != 0 {
		t.Errorf("Depth() after Clear() = %v, want 0", stack.Depth())
	}
	if stack.Current() != base {
		t.Error("Current() should return base after Clear()")
	}
}

// TestCompositeLayer tests layer compositing.
func TestCompositeLayer(t *testing.T) {
	pool := image.NewPool(4)

	// Create destination buffer (100x100, white background)
	dst, err := image.NewImageBuf(100, 100, image.FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf() error = %v", err)
	}
	dst.Fill(255, 255, 255, 255) // White

	// Create layer (50x50, red with 50% opacity)
	layer, err := NewLayer(BlendSourceOver, 0.5, Bounds{25, 25, 50, 50}, pool)
	if err != nil {
		t.Fatalf("NewLayer() error = %v", err)
	}
	layer.Buffer().Fill(255, 0, 0, 255) // Red

	// Composite layer onto destination
	compositeLayer(layer, dst)

	// Check a pixel in the center (should be blend of red and white)
	r, g, b, a := dst.GetRGBA(50, 50)
	if a != 255 {
		t.Errorf("Center pixel alpha = %v, want 255", a)
	}
	// With 50% opacity, red (255,0,0) over white (255,255,255) should give approximately (255, 127, 127)
	// Due to premultiplied alpha blending, exact values may vary slightly
	if r < 250 {
		t.Errorf("Center pixel red = %v, expected high red value", r)
	}
	if g < 100 || g > 150 {
		t.Errorf("Center pixel green = %v, expected around 127", g)
	}
	if b < 100 || b > 150 {
		t.Errorf("Center pixel blue = %v, expected around 127", b)
	}

	// Check a pixel outside the layer bounds (should be unchanged white)
	r, g, b, a = dst.GetRGBA(10, 10)
	if r != 255 || g != 255 || b != 255 || a != 255 {
		t.Errorf("Outside pixel = (%v,%v,%v,%v), want (255,255,255,255)", r, g, b, a)
	}

	pool.Put(layer.buffer)
}

// TestCompositeLayerWithBlendModes tests compositing with different blend modes.
func TestCompositeLayerWithBlendModes(t *testing.T) {
	pool := image.NewPool(4)

	tests := []struct {
		name      string
		blendMode BlendMode
		opacity   float64
		srcColor  [4]byte // RGBA
		dstColor  [4]byte // RGBA
	}{
		{
			name:      "source over",
			blendMode: BlendSourceOver,
			opacity:   1.0,
			srcColor:  [4]byte{255, 0, 0, 255},
			dstColor:  [4]byte{255, 255, 255, 255},
		},
		{
			name:      "multiply",
			blendMode: BlendMultiply,
			opacity:   1.0,
			srcColor:  [4]byte{128, 128, 128, 255},
			dstColor:  [4]byte{255, 255, 255, 255},
		},
		{
			name:      "screen",
			blendMode: BlendScreen,
			opacity:   1.0,
			srcColor:  [4]byte{128, 128, 128, 255},
			dstColor:  [4]byte{128, 128, 128, 255},
		},
		{
			name:      "source over with opacity",
			blendMode: BlendSourceOver,
			opacity:   0.5,
			srcColor:  [4]byte{255, 0, 0, 255},
			dstColor:  [4]byte{0, 255, 0, 255},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create destination buffer
			dst, err := image.NewImageBuf(10, 10, image.FormatRGBA8)
			if err != nil {
				t.Fatalf("NewImageBuf() error = %v", err)
			}
			dst.Fill(tt.dstColor[0], tt.dstColor[1], tt.dstColor[2], tt.dstColor[3])

			// Create layer
			layer, err := NewLayer(tt.blendMode, tt.opacity, Bounds{0, 0, 10, 10}, pool)
			if err != nil {
				t.Fatalf("NewLayer() error = %v", err)
			}
			layer.Buffer().Fill(tt.srcColor[0], tt.srcColor[1], tt.srcColor[2], tt.srcColor[3])

			// Composite
			compositeLayer(layer, dst)

			// Verify compositing happened (result should be different from destination)
			r, g, b, a := dst.GetRGBA(5, 5)
			_ = r
			_ = g
			_ = b

			// Alpha should always be >= original destination alpha
			if a < tt.dstColor[3] {
				t.Errorf("Result alpha %v < dest alpha %v", a, tt.dstColor[3])
			}

			pool.Put(layer.buffer)
		})
	}
}

// TestCompositeLayerOutOfBounds tests compositing with layer bounds outside destination.
func TestCompositeLayerOutOfBounds(t *testing.T) {
	pool := image.NewPool(4)

	// Create destination buffer (100x100, white)
	dst, err := image.NewImageBuf(100, 100, image.FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf() error = %v", err)
	}
	dst.Fill(255, 255, 255, 255)

	tests := []struct {
		name   string
		bounds Bounds
	}{
		{
			name:   "layer partially outside left",
			bounds: Bounds{-25, 25, 50, 50},
		},
		{
			name:   "layer partially outside top",
			bounds: Bounds{25, -25, 50, 50},
		},
		{
			name:   "layer partially outside right",
			bounds: Bounds{75, 25, 50, 50},
		},
		{
			name:   "layer partially outside bottom",
			bounds: Bounds{25, 75, 50, 50},
		},
		{
			name:   "layer completely outside",
			bounds: Bounds{150, 150, 50, 50},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create layer with red color
			layer, err := NewLayer(BlendSourceOver, 1.0, tt.bounds, pool)
			if err != nil {
				t.Fatalf("NewLayer() error = %v", err)
			}
			layer.Buffer().Fill(255, 0, 0, 255) // Red

			// Composite should not panic
			compositeLayer(layer, dst)

			// Verify destination is still valid
			r, g, b, a := dst.GetRGBA(50, 50)
			_ = r
			_ = g
			_ = b
			if a == 0 {
				t.Error("Destination was corrupted (alpha = 0)")
			}

			pool.Put(layer.buffer)
		})
	}
}

// TestLayerStackIntegration tests a complete workflow with multiple layers.
func TestLayerStackIntegration(t *testing.T) {
	// Create base buffer (100x100, white)
	base, err := image.NewImageBuf(100, 100, image.FormatRGBA8)
	if err != nil {
		t.Fatalf("NewImageBuf() error = %v", err)
	}
	base.Fill(255, 255, 255, 255)

	pool := image.NewPool(8)
	stack := NewLayerStack(base, pool)

	// Push layer 1: red, 50% opacity
	layer1, err := stack.Push(BlendSourceOver, 0.5, Bounds{10, 10, 30, 30})
	if err != nil {
		t.Fatalf("Push() layer1 error = %v", err)
	}
	layer1.Buffer().Fill(255, 0, 0, 255)

	// Push layer 2: green, multiply blend
	layer2, err := stack.Push(BlendMultiply, 1.0, Bounds{20, 20, 30, 30})
	if err != nil {
		t.Fatalf("Push() layer2 error = %v", err)
	}
	layer2.Buffer().Fill(0, 255, 0, 255)

	// Verify stack state
	if stack.Depth() != 2 {
		t.Errorf("Depth() = %v, want 2", stack.Depth())
	}
	if stack.Current() != layer2.Buffer() {
		t.Error("Current() should return layer2 buffer")
	}

	// Pop layer 2 (composites onto layer 1)
	result := stack.Pop()
	if result != layer1.Buffer() {
		t.Error("Pop() should return layer1 buffer")
	}
	if stack.Depth() != 1 {
		t.Errorf("Depth() = %v, want 1", stack.Depth())
	}

	// Pop layer 1 (composites onto base)
	result = stack.Pop()
	if result != base {
		t.Error("Pop() should return base")
	}
	if stack.Depth() != 0 {
		t.Errorf("Depth() = %v, want 0", stack.Depth())
	}

	// Verify base has been modified
	r, _, _, a := base.GetRGBA(15, 15)
	if a == 0 {
		t.Error("Base alpha should not be 0")
	}
	// Pixel (15,15) is in layer1 bounds, should have some red
	if r < 200 {
		t.Errorf("Expected significant red component, got %v", r)
	}
	// Verify pixel outside all layers is still white
	r, g, b, a := base.GetRGBA(5, 5)
	if r != 255 || g != 255 || b != 255 || a != 255 {
		t.Errorf("Outside pixel = (%v,%v,%v,%v), want (255,255,255,255)", r, g, b, a)
	}
}

// BenchmarkLayerPush benchmarks pushing a layer onto the stack.
func BenchmarkLayerPush(b *testing.B) {
	base, _ := image.NewImageBuf(1000, 1000, image.FormatRGBA8)
	pool := image.NewPool(16)
	stack := NewLayerStack(base, pool)
	bounds := Bounds{0, 0, 500, 500}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = stack.Push(BlendSourceOver, 1.0, bounds)
		stack.Clear()
	}
}

// BenchmarkLayerPop benchmarks popping and compositing a layer.
func BenchmarkLayerPop(b *testing.B) {
	base, _ := image.NewImageBuf(1000, 1000, image.FormatRGBA8)
	pool := image.NewPool(16)
	bounds := Bounds{0, 0, 500, 500}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stack := NewLayerStack(base, pool)
		_, _ = stack.Push(BlendSourceOver, 1.0, bounds)
		_ = stack.Pop()
	}
}

// BenchmarkCompositeLayer benchmarks layer compositing.
func BenchmarkCompositeLayer(b *testing.B) {
	pool := image.NewPool(16)
	dst, _ := image.NewImageBuf(1000, 1000, image.FormatRGBA8)
	dst.Fill(255, 255, 255, 255)

	layer, _ := NewLayer(BlendSourceOver, 0.5, Bounds{0, 0, 1000, 1000}, pool)
	layer.Buffer().Fill(255, 0, 0, 255)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compositeLayer(layer, dst)
	}

	pool.Put(layer.buffer)
}
