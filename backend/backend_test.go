package backend

import (
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/scene"
)

func TestSoftwareBackendName(t *testing.T) {
	b := NewSoftwareBackend()
	if b.Name() != "software" {
		t.Errorf("Name() = %q, want %q", b.Name(), "software")
	}
}

func TestSoftwareBackendInit(t *testing.T) {
	b := NewSoftwareBackend()
	if err := b.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	b.Close()
}

func TestSoftwareBackendNewRenderer(t *testing.T) {
	b := NewSoftwareBackend()
	if err := b.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer b.Close()

	renderer := b.NewRenderer(100, 100)
	if renderer == nil {
		t.Error("NewRenderer() returned nil")
	}
}

func TestSoftwareBackendRenderScene(t *testing.T) {
	b := NewSoftwareBackend()
	if err := b.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer b.Close()

	// Create a simple scene
	s := scene.NewScene()
	rect := scene.NewRectShape(10, 10, 80, 80)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(),
		scene.SolidBrush(gg.Red), rect)

	// Create target pixmap
	pixmap := gg.NewPixmap(100, 100)

	// Render scene
	if err := b.RenderScene(pixmap, s); err != nil {
		t.Fatalf("RenderScene() error = %v", err)
	}

	// Verify something was rendered (check a pixel inside the rect)
	pixel := pixmap.GetPixel(50, 50)
	if pixel.R == 0 {
		t.Error("RenderScene() did not render any content")
	}
}

func TestSoftwareBackendRenderSceneNil(t *testing.T) {
	b := NewSoftwareBackend()
	if err := b.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer b.Close()

	// Should not error with nil inputs
	if err := b.RenderScene(nil, nil); err != nil {
		t.Errorf("RenderScene(nil, nil) error = %v", err)
	}

	pixmap := gg.NewPixmap(100, 100)
	if err := b.RenderScene(pixmap, nil); err != nil {
		t.Errorf("RenderScene(pixmap, nil) error = %v", err)
	}

	s := scene.NewScene()
	if err := b.RenderScene(nil, s); err != nil {
		t.Errorf("RenderScene(nil, scene) error = %v", err)
	}
}

func TestSoftwareBackendSceneRenderer(t *testing.T) {
	b := NewSoftwareBackend()
	if err := b.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer b.Close()

	// Initially nil
	if b.SceneRenderer() != nil {
		t.Error("SceneRenderer() should be nil before first render")
	}

	// After render, should be set
	s := scene.NewScene()
	pixmap := gg.NewPixmap(100, 100)
	_ = b.RenderScene(pixmap, s)

	if b.SceneRenderer() == nil {
		t.Error("SceneRenderer() should not be nil after render")
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	// Software backend is auto-registered via init()
	if !IsRegistered("software") {
		t.Error("software backend should be auto-registered")
	}

	b := Get("software")
	if b == nil {
		t.Fatal("Get(software) returned nil")
	}
	if b.Name() != "software" {
		t.Errorf("Get(software).Name() = %q, want %q", b.Name(), "software")
	}
}

func TestRegistryGetUnregistered(t *testing.T) {
	b := Get("nonexistent")
	if b != nil {
		t.Error("Get(nonexistent) should return nil")
	}
}

func TestRegistryAvailable(t *testing.T) {
	available := Available()
	found := false
	for _, name := range available {
		if name == "software" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Available() should include 'software'")
	}
}

func TestRegistryDefault(t *testing.T) {
	b := Default()
	if b == nil {
		t.Fatal("Default() returned nil")
	}
	// Software should be the default when no GPU backend is available
	if b.Name() != "software" {
		t.Logf("Default() returned %q (may vary based on available backends)", b.Name())
	}
}

func TestRegistryMustDefault(t *testing.T) {
	// Should not panic when software backend is available
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("MustDefault() panicked: %v", r)
		}
	}()
	b := MustDefault()
	if b == nil {
		t.Error("MustDefault() returned nil")
	}
}

func TestRegistryInitDefault(t *testing.T) {
	b, err := InitDefault()
	if err != nil {
		t.Fatalf("InitDefault() error = %v", err)
	}
	if b == nil {
		t.Fatal("InitDefault() returned nil backend")
	}
	defer b.Close()

	// Verify it's initialized by using it
	renderer := b.NewRenderer(100, 100)
	if renderer == nil {
		t.Error("Backend from InitDefault() should be usable")
	}
}

func TestRegistryUnregister(t *testing.T) {
	// Register a test backend
	testFactory := func() RenderBackend {
		return &SoftwareBackend{}
	}
	Register("test-backend", testFactory)

	if !IsRegistered("test-backend") {
		t.Error("test-backend should be registered")
	}

	Unregister("test-backend")

	if IsRegistered("test-backend") {
		t.Error("test-backend should be unregistered")
	}
}

func TestRegistryIsRegistered(t *testing.T) {
	if !IsRegistered("software") {
		t.Error("software should be registered")
	}
	if IsRegistered("nonexistent") {
		t.Error("nonexistent should not be registered")
	}
}

func TestSoftwareBackendClose(t *testing.T) {
	b := NewSoftwareBackend()
	if err := b.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Render something to create scene renderer
	s := scene.NewScene()
	pixmap := gg.NewPixmap(100, 100)
	_ = b.RenderScene(pixmap, s)

	// Close should not panic
	b.Close()

	// SceneRenderer should be nil after close
	if b.SceneRenderer() != nil {
		t.Error("SceneRenderer() should be nil after Close()")
	}
}

func TestSoftwareBackendResizeSceneRenderer(t *testing.T) {
	b := NewSoftwareBackend()
	if err := b.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer b.Close()

	s := scene.NewScene()

	// First render at 100x100
	pixmap1 := gg.NewPixmap(100, 100)
	_ = b.RenderScene(pixmap1, s)
	r1 := b.SceneRenderer()

	// Second render at same size should reuse renderer
	_ = b.RenderScene(pixmap1, s)
	r2 := b.SceneRenderer()
	if r1 != r2 {
		t.Error("SceneRenderer should be reused for same size")
	}

	// Render at different size should create new renderer
	pixmap2 := gg.NewPixmap(200, 200)
	_ = b.RenderScene(pixmap2, s)
	r3 := b.SceneRenderer()
	if r2 == r3 {
		t.Error("SceneRenderer should be recreated for different size")
	}
}

// Benchmark tests

func BenchmarkSoftwareBackendNewRenderer(b *testing.B) {
	backend := NewSoftwareBackend()
	_ = backend.Init()
	defer backend.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.NewRenderer(800, 600)
	}
}

func BenchmarkSoftwareBackendRenderScene(b *testing.B) {
	backend := NewSoftwareBackend()
	_ = backend.Init()
	defer backend.Close()

	s := scene.NewScene()
	rect := scene.NewRectShape(10, 10, 780, 580)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(),
		scene.SolidBrush(gg.Red), rect)

	pixmap := gg.NewPixmap(800, 600)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.RenderScene(pixmap, s)
	}
}
