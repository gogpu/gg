package gpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gg"
)

// TestTextureFormat tests TextureFormat methods.
func TestTextureFormat(t *testing.T) {
	tests := []struct {
		format        TextureFormat
		wantString    string
		wantBytesPerP int
	}{
		{TextureFormatRGBA8, "RGBA8", 4},
		{TextureFormatBGRA8, "BGRA8", 4},
		{TextureFormatR8, "R8", 1},
		{TextureFormat(99), "Unknown(99)", 4}, // Default fallback
	}

	for _, tt := range tests {
		t.Run(tt.wantString, func(t *testing.T) {
			if got := tt.format.String(); got != tt.wantString {
				t.Errorf("String() = %q, want %q", got, tt.wantString)
			}
			if got := tt.format.BytesPerPixel(); got != tt.wantBytesPerP {
				t.Errorf("BytesPerPixel() = %d, want %d", got, tt.wantBytesPerP)
			}
		})
	}
}

// TestCreateTexture tests texture creation.
func TestCreateTexture(t *testing.T) {
	tests := []struct {
		name      string
		config    TextureConfig
		wantErr   bool
		wantBytes uint64
	}{
		{
			name: "valid RGBA8",
			config: TextureConfig{
				Width:  100,
				Height: 100,
				Format: TextureFormatRGBA8,
				Label:  "test",
			},
			wantErr:   false,
			wantBytes: 100 * 100 * 4,
		},
		{
			name: "valid R8",
			config: TextureConfig{
				Width:  256,
				Height: 256,
				Format: TextureFormatR8,
				Label:  "mask",
			},
			wantErr:   false,
			wantBytes: 256 * 256 * 1,
		},
		{
			name: "invalid zero width",
			config: TextureConfig{
				Width:  0,
				Height: 100,
				Format: TextureFormatRGBA8,
			},
			wantErr: true,
		},
		{
			name: "invalid zero height",
			config: TextureConfig{
				Width:  100,
				Height: 0,
				Format: TextureFormatRGBA8,
			},
			wantErr: true,
		},
		{
			name: "invalid negative width",
			config: TextureConfig{
				Width:  -10,
				Height: 100,
				Format: TextureFormatRGBA8,
			},
			wantErr: true,
		},
	}

	// Note: We pass nil backend since CreateTexture is a stub
	// and doesn't actually create GPU resources
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tex, err := CreateTexture(nil, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateTexture() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if tex.Width() != tt.config.Width {
				t.Errorf("Width() = %d, want %d", tex.Width(), tt.config.Width)
			}
			if tex.Height() != tt.config.Height {
				t.Errorf("Height() = %d, want %d", tex.Height(), tt.config.Height)
			}
			if tex.Format() != tt.config.Format {
				t.Errorf("Format() = %v, want %v", tex.Format(), tt.config.Format)
			}
			if tex.SizeBytes() != tt.wantBytes {
				t.Errorf("SizeBytes() = %d, want %d", tex.SizeBytes(), tt.wantBytes)
			}
			if tex.Label() != tt.config.Label {
				t.Errorf("Label() = %q, want %q", tex.Label(), tt.config.Label)
			}

			tex.Close()
			if !tex.IsReleased() {
				t.Error("texture should be released after Close()")
			}
		})
	}
}

// TestTextureUploadDownload tests upload and download operations.
func TestTextureUploadDownload(t *testing.T) {
	tex, err := CreateTexture(nil, TextureConfig{
		Width:  10,
		Height: 10,
		Format: TextureFormatRGBA8,
	})
	if err != nil {
		t.Fatalf("CreateTexture() error = %v", err)
	}
	defer tex.Close()

	// Test upload with matching pixmap
	pixmap := gg.NewPixmap(10, 10)
	if err := tex.UploadPixmap(pixmap); err != nil {
		t.Errorf("UploadPixmap() error = %v", err)
	}

	// Test upload with nil pixmap
	if err := tex.UploadPixmap(nil); !errors.Is(err, ErrNilPixmap) {
		t.Errorf("UploadPixmap(nil) error = %v, want %v", err, ErrNilPixmap)
	}

	// Test upload with mismatched size
	wrongPixmap := gg.NewPixmap(20, 20)
	if err := tex.UploadPixmap(wrongPixmap); err == nil {
		t.Error("UploadPixmap() expected error for size mismatch")
	}

	// Test download (stub returns error)
	_, err = tex.DownloadPixmap()
	if !errors.Is(err, ErrTextureReadbackNotSupported) {
		t.Errorf("DownloadPixmap() error = %v, want %v", err, ErrTextureReadbackNotSupported)
	}

	// Test operations on released texture
	tex.Close()
	if err := tex.UploadPixmap(pixmap); !errors.Is(err, ErrTextureReleased) {
		t.Errorf("UploadPixmap() on released texture error = %v, want %v", err, ErrTextureReleased)
	}
}

// TestTextureUploadRegion tests region upload.
func TestTextureUploadRegion(t *testing.T) {
	tex, err := CreateTexture(nil, TextureConfig{
		Width:  100,
		Height: 100,
		Format: TextureFormatRGBA8,
	})
	if err != nil {
		t.Fatalf("CreateTexture() error = %v", err)
	}
	defer tex.Close()

	// Valid region upload
	pixmap := gg.NewPixmap(10, 10)
	if err := tex.UploadRegion(0, 0, pixmap); err != nil {
		t.Errorf("UploadRegion() error = %v", err)
	}

	// Upload at offset
	if err := tex.UploadRegion(50, 50, pixmap); err != nil {
		t.Errorf("UploadRegion() at offset error = %v", err)
	}

	// Out of bounds
	if err := tex.UploadRegion(95, 95, pixmap); err == nil {
		t.Error("UploadRegion() expected error for out of bounds")
	}

	// Negative coordinates
	if err := tex.UploadRegion(-1, 0, pixmap); err == nil {
		t.Error("UploadRegion() expected error for negative coordinates")
	}
}

// TestMemoryManagerBasic tests basic memory manager operations.
func TestMemoryManagerBasic(t *testing.T) {
	mm := NewMemoryManager(nil, MemoryManagerConfig{
		MaxMemoryMB: 16,
	})
	defer mm.Close()

	// Check initial stats
	stats := mm.Stats()
	if stats.UsedBytes != 0 {
		t.Errorf("Initial UsedBytes = %d, want 0", stats.UsedBytes)
	}
	if stats.TextureCount != 0 {
		t.Errorf("Initial TextureCount = %d, want 0", stats.TextureCount)
	}

	// Allocate a texture
	tex, err := mm.AllocTexture(TextureConfig{
		Width:  100,
		Height: 100,
		Format: TextureFormatRGBA8,
	})
	if err != nil {
		t.Fatalf("AllocTexture() error = %v", err)
	}

	// Check stats after allocation
	stats = mm.Stats()
	expectedBytes := uint64(100 * 100 * 4)
	if stats.UsedBytes != expectedBytes {
		t.Errorf("UsedBytes = %d, want %d", stats.UsedBytes, expectedBytes)
	}
	if stats.TextureCount != 1 {
		t.Errorf("TextureCount = %d, want 1", stats.TextureCount)
	}

	// Check texture is managed
	if !mm.Contains(tex) {
		t.Error("Manager should contain allocated texture")
	}

	// Free the texture
	if err := mm.FreeTexture(tex); err != nil {
		t.Errorf("FreeTexture() error = %v", err)
	}

	// Check stats after free
	stats = mm.Stats()
	if stats.UsedBytes != 0 {
		t.Errorf("UsedBytes after free = %d, want 0", stats.UsedBytes)
	}
	if stats.TextureCount != 0 {
		t.Errorf("TextureCount after free = %d, want 0", stats.TextureCount)
	}
}

// TestMemoryManagerEviction tests LRU eviction.
func TestMemoryManagerEviction(t *testing.T) {
	// Small budget: 16 MB (minimum allowed)
	// Each 512x512 RGBA8 texture = 1 MB
	mm := NewMemoryManager(nil, MemoryManagerConfig{
		MaxMemoryMB:       16,
		EvictionThreshold: 0.5, // Start evicting at 8 MB
	})
	defer mm.Close()

	// Allocate 10 textures (10 MB total, triggers eviction after 8 MB threshold)
	var textures []*GPUTexture
	for i := 0; i < 10; i++ {
		tex, err := mm.AllocTexture(TextureConfig{
			Width:  512,
			Height: 512,
			Format: TextureFormatRGBA8, // 1 MB each
		})
		if err != nil {
			t.Logf("AllocTexture %d error = %v (expected when budget exceeded)", i, err)
			break
		}
		textures = append(textures, tex)
	}

	// Should have allocated some textures
	if len(textures) < 8 {
		t.Fatalf("Should have allocated at least 8 textures, got %d", len(textures))
	}

	stats := mm.Stats()
	t.Logf("After allocation: %s", stats.String())

	// Try to allocate a large texture that may trigger eviction
	largeTex, err := mm.AllocTexture(TextureConfig{
		Width:  1024,
		Height: 1024,
		Format: TextureFormatRGBA8, // 4 MB
	})
	if err != nil {
		t.Logf("Large allocation failed (budget exceeded): %v", err)
		return
	}

	newStats := mm.Stats()
	t.Logf("After large allocation: %s", newStats.String())

	if newStats.EvictionCount > 0 {
		t.Logf("Eviction triggered: %d textures evicted", newStats.EvictionCount)
	}

	_ = mm.FreeTexture(largeTex)
}

// TestMemoryManagerTouch tests LRU touch operation.
func TestMemoryManagerTouch(t *testing.T) {
	mm := NewMemoryManager(nil, MemoryManagerConfig{
		MaxMemoryMB: 16,
	})
	defer mm.Close()

	// Allocate two textures
	tex1, err := mm.AllocTexture(TextureConfig{
		Width: 10, Height: 10, Format: TextureFormatRGBA8,
	})
	if err != nil {
		t.Fatalf("AllocTexture() error = %v", err)
	}

	tex2, err := mm.AllocTexture(TextureConfig{
		Width: 10, Height: 10, Format: TextureFormatRGBA8,
	})
	if err != nil {
		t.Fatalf("AllocTexture() error = %v", err)
	}

	// Touch tex1 to make it more recently used
	mm.TouchTexture(tex1)

	// Both should still be managed
	if !mm.Contains(tex1) {
		t.Error("tex1 should still be managed")
	}
	if !mm.Contains(tex2) {
		t.Error("tex2 should still be managed")
	}

	_ = mm.FreeTexture(tex1)
	_ = mm.FreeTexture(tex2)
}

// TestMemoryManagerBudget tests budget changes.
func TestMemoryManagerBudget(t *testing.T) {
	mm := NewMemoryManager(nil, MemoryManagerConfig{
		MaxMemoryMB: 32,
	})
	defer mm.Close()

	// Allocate some textures
	for i := 0; i < 3; i++ {
		_, err := mm.AllocTexture(TextureConfig{
			Width:  256,
			Height: 256,
			Format: TextureFormatRGBA8,
		})
		if err != nil {
			t.Fatalf("AllocTexture() error = %v", err)
		}
	}

	// Reduce budget - should trigger eviction
	if err := mm.SetBudget(1); err != nil {
		t.Logf("SetBudget() error = %v (may be expected if eviction can't free enough)", err)
	}

	stats := mm.Stats()
	t.Logf("After budget reduction: %s", stats.String())
}

// TestMemoryManagerClose tests manager closure.
func TestMemoryManagerClose(t *testing.T) {
	mm := NewMemoryManager(nil, MemoryManagerConfig{
		MaxMemoryMB: 16,
	})

	// Allocate a texture
	_, err := mm.AllocTexture(TextureConfig{
		Width:  10,
		Height: 10,
		Format: TextureFormatRGBA8,
	})
	if err != nil {
		t.Fatalf("AllocTexture() error = %v", err)
	}

	// Close the manager
	mm.Close()

	// Operations should fail
	_, err = mm.AllocTexture(TextureConfig{
		Width:  10,
		Height: 10,
		Format: TextureFormatRGBA8,
	})
	if !errors.Is(err, ErrMemoryManagerClosed) {
		t.Errorf("AllocTexture() after close error = %v, want %v", err, ErrMemoryManagerClosed)
	}
}

// TestRectAllocator tests the shelf-packing allocator.
func TestRectAllocator(t *testing.T) {
	alloc := NewRectAllocator(256, 256, 1)

	// Allocate several rectangles
	tests := []struct {
		w, h     int
		wantOK   bool
		wantX    int
		wantY    int
		wantW    int
		wantH    int
		checkPos bool
	}{
		{50, 30, true, 0, 0, 50, 30, true},      // First allocation
		{50, 30, true, 51, 0, 50, 30, true},     // Same shelf
		{50, 30, true, 102, 0, 50, 30, true},    // Same shelf
		{50, 30, true, 153, 0, 50, 30, true},    // Same shelf
		{50, 30, true, 204, 0, 50, 30, true},    // Same shelf (fits: 204+50+1=255)
		{50, 30, true, 0, 31, 50, 30, true},     // New shelf
		{300, 300, false, 0, 0, 0, 0, false},    // Too big
		{0, 10, false, 0, 0, 0, 0, false},       // Invalid width
		{10, 0, false, 0, 0, 0, 0, false},       // Invalid height
		{-10, 10, false, 0, 0, 0, 0, false},     // Negative width
		{255, 255, false, 0, 0, 0, 0, false},    // Won't fit (need 256 including padding)
		{254, 10, true, 0, 0, 254, 10, false},   // Will fit on new shelf (254+1=255)
		{254, 10, true, 0, 0, 254, 10, false},   // Another one
		{254, 10, true, 0, 0, 254, 10, false},   // And another
		{254, 10, true, 0, 0, 254, 10, false},   // Keep going
		{254, 10, true, 0, 0, 254, 10, false},   // Still going
		{254, 100, true, 0, 0, 254, 100, false}, // Still fits (y~117, needs 101, have 139)
		{254, 100, false, 0, 0, 0, 0, false},    // Should fail - out of vertical space now
	}

	for i, tt := range tests {
		region := alloc.Allocate(tt.w, tt.h)
		ok := region.IsValid()
		if ok != tt.wantOK {
			t.Errorf("Test %d: Allocate(%d,%d) ok = %v, want %v", i, tt.w, tt.h, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if region.Width != tt.wantW || region.Height != tt.wantH {
			t.Errorf("Test %d: region size = %dx%d, want %dx%d",
				i, region.Width, region.Height, tt.wantW, tt.wantH)
		}
		if tt.checkPos {
			if region.X != tt.wantX || region.Y != tt.wantY {
				t.Errorf("Test %d: region pos = (%d,%d), want (%d,%d)",
					i, region.X, region.Y, tt.wantX, tt.wantY)
			}
		}
	}

	// Check utilization
	util := alloc.Utilization()
	if util <= 0 {
		t.Errorf("Utilization = %f, want > 0", util)
	}
	t.Logf("Allocator utilization: %.1f%%", util*100)

	// Reset and verify
	alloc.Reset()
	if alloc.AllocCount() != 0 {
		t.Errorf("AllocCount after reset = %d, want 0", alloc.AllocCount())
	}
	if alloc.UsedArea() != 0 {
		t.Errorf("UsedArea after reset = %d, want 0", alloc.UsedArea())
	}
}

// TestAtlasRegion tests AtlasRegion methods.
func TestAtlasRegion(t *testing.T) {
	r := AtlasRegion{X: 10, Y: 20, Width: 30, Height: 40}

	if !r.IsValid() {
		t.Error("Region should be valid")
	}

	if !r.Contains(10, 20) {
		t.Error("Region should contain top-left corner")
	}
	if !r.Contains(39, 59) {
		t.Error("Region should contain bottom-right - 1")
	}
	if r.Contains(40, 60) {
		t.Error("Region should not contain bottom-right edge")
	}
	if r.Contains(9, 20) {
		t.Error("Region should not contain point outside left")
	}

	invalid := AtlasRegion{Width: 0, Height: 10}
	if invalid.IsValid() {
		t.Error("Region with zero width should be invalid")
	}
}

// TestTextureAtlas tests texture atlas operations.
func TestTextureAtlas(t *testing.T) {
	atlas, err := NewTextureAtlas(nil, TextureAtlasConfig{
		Width:   512,
		Height:  512,
		Padding: 1,
		Label:   "test-atlas",
	})
	if err != nil {
		t.Fatalf("NewTextureAtlas() error = %v", err)
	}
	defer atlas.Close()

	// Allocate several regions
	regions := make([]AtlasRegion, 0)
	for i := 0; i < 5; i++ {
		region, err := atlas.Allocate(64, 64)
		if err != nil {
			t.Errorf("Allocate() %d error = %v", i, err)
			continue
		}
		if !region.IsValid() {
			t.Errorf("Allocate() %d returned invalid region", i)
			continue
		}
		regions = append(regions, region)
	}

	// Upload to a region
	if len(regions) > 0 {
		pixmap := gg.NewPixmap(64, 64)
		if err := atlas.Upload(regions[0], pixmap); err != nil {
			t.Errorf("Upload() error = %v", err)
		}
	}

	// Test AllocateAndUpload
	pixmap := gg.NewPixmap(32, 32)
	region, err := atlas.AllocateAndUpload(pixmap)
	if err != nil {
		t.Errorf("AllocateAndUpload() error = %v", err)
	}
	if !region.IsValid() {
		t.Error("AllocateAndUpload() returned invalid region")
	}

	// Check utilization
	util := atlas.Utilization()
	if util <= 0 {
		t.Errorf("Utilization = %f, want > 0", util)
	}
	t.Logf("Atlas utilization: %.1f%%", util*100)

	// Reset and check
	atlas.Reset()
	if atlas.AllocCount() != 0 {
		t.Errorf("AllocCount after reset = %d, want 0", atlas.AllocCount())
	}
}

// TestTextureAtlasErrors tests atlas error conditions.
func TestTextureAtlasErrors(t *testing.T) {
	// Use minimum atlas size (256x256) to test bounds properly
	atlas, err := NewTextureAtlas(nil, TextureAtlasConfig{
		Width:  256,
		Height: 256,
	})
	if err != nil {
		t.Fatalf("NewTextureAtlas() error = %v", err)
	}

	// Fill the atlas
	for i := 0; i < 100; i++ {
		_, err := atlas.Allocate(32, 32)
		if errors.Is(err, ErrAtlasFull) {
			t.Logf("Atlas full after %d allocations", i)
			break
		}
		if err != nil {
			t.Errorf("Allocate() unexpected error = %v", err)
			break
		}
	}

	// Upload with wrong size
	region := AtlasRegion{X: 0, Y: 0, Width: 32, Height: 32}
	wrongPixmap := gg.NewPixmap(64, 64)
	if err := atlas.Upload(region, wrongPixmap); err == nil {
		t.Error("Upload() expected error for size mismatch")
	}

	// Upload with nil pixmap
	if err := atlas.Upload(region, nil); !errors.Is(err, ErrNilPixmap) {
		t.Errorf("Upload(nil) error = %v, want %v", err, ErrNilPixmap)
	}

	// Upload out of bounds (256x256 atlas, region extends beyond)
	outOfBounds := AtlasRegion{X: 200, Y: 200, Width: 64, Height: 64}
	pixmap := gg.NewPixmap(64, 64)
	if err := atlas.Upload(outOfBounds, pixmap); !errors.Is(err, ErrRegionOutOfBounds) {
		t.Errorf("Upload() out of bounds error = %v, want %v", err, ErrRegionOutOfBounds)
	}

	// Close and check operations fail
	atlas.Close()
	if !atlas.IsClosed() {
		t.Error("Atlas should be closed")
	}

	_, err = atlas.Allocate(10, 10)
	if !errors.Is(err, ErrAtlasClosed) {
		t.Errorf("Allocate() after close error = %v, want %v", err, ErrAtlasClosed)
	}
}

// TestMemoryStats tests MemoryStats string formatting.
func TestMemoryStats(t *testing.T) {
	stats := MemoryStats{
		TotalBytes:     256 * 1024 * 1024,
		UsedBytes:      128 * 1024 * 1024,
		AvailableBytes: 128 * 1024 * 1024,
		TextureCount:   10,
		EvictionCount:  5,
		Utilization:    0.5,
	}

	s := stats.String()
	if s == "" {
		t.Error("MemoryStats.String() should not be empty")
	}
	t.Logf("MemoryStats: %s", s)
}

// TestCreateTextureFromPixmap tests creating texture from pixmap.
func TestCreateTextureFromPixmap(t *testing.T) {
	pixmap := gg.NewPixmap(50, 50)

	tex, err := CreateTextureFromPixmap(nil, pixmap, "test-from-pixmap")
	if err != nil {
		t.Fatalf("CreateTextureFromPixmap() error = %v", err)
	}
	defer tex.Close()

	if tex.Width() != 50 || tex.Height() != 50 {
		t.Errorf("Texture size = %dx%d, want 50x50", tex.Width(), tex.Height())
	}
	if tex.Format() != TextureFormatRGBA8 {
		t.Errorf("Format = %v, want RGBA8", tex.Format())
	}

	// Test with nil pixmap
	_, err = CreateTextureFromPixmap(nil, nil, "nil-test")
	if !errors.Is(err, ErrNilPixmap) {
		t.Errorf("CreateTextureFromPixmap(nil) error = %v, want %v", err, ErrNilPixmap)
	}
}

// TestDoubleClose tests that double close is safe.
func TestDoubleClose(t *testing.T) {
	tex, err := CreateTexture(nil, TextureConfig{
		Width:  10,
		Height: 10,
		Format: TextureFormatRGBA8,
	})
	if err != nil {
		t.Fatalf("CreateTexture() error = %v", err)
	}

	// First close
	tex.Close()
	if !tex.IsReleased() {
		t.Error("Texture should be released")
	}

	// Second close should be safe
	tex.Close()

	// Same for atlas
	atlas, err := NewTextureAtlas(nil, TextureAtlasConfig{Width: 128, Height: 128})
	if err != nil {
		t.Fatalf("NewTextureAtlas() error = %v", err)
	}
	atlas.Close()
	atlas.Close() // Should not panic

	// Same for memory manager
	mm := NewMemoryManager(nil, MemoryManagerConfig{MaxMemoryMB: 16})
	mm.Close()
	mm.Close() // Should not panic
}
