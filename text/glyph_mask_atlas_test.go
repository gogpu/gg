package text

import (
	"testing"
)

func TestGlyphMaskAtlasConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  GlyphMaskAtlasConfig
		wantErr bool
	}{
		{"default is valid", DefaultGlyphMaskAtlasConfig(), false},
		{"size too small", GlyphMaskAtlasConfig{Size: 32, Padding: 1, MaxAtlases: 1, MaxEntries: 100}, true},
		{"size too large", GlyphMaskAtlasConfig{Size: 16384, Padding: 1, MaxAtlases: 1, MaxEntries: 100}, true},
		{"size not power of 2", GlyphMaskAtlasConfig{Size: 500, Padding: 1, MaxAtlases: 1, MaxEntries: 100}, true},
		{"negative padding", GlyphMaskAtlasConfig{Size: 256, Padding: -1, MaxAtlases: 1, MaxEntries: 100}, true},
		{"zero atlases", GlyphMaskAtlasConfig{Size: 256, Padding: 1, MaxAtlases: 0, MaxEntries: 100}, true},
		{"zero entries", GlyphMaskAtlasConfig{Size: 256, Padding: 1, MaxAtlases: 1, MaxEntries: 0}, true},
		{"valid small", GlyphMaskAtlasConfig{Size: 64, Padding: 0, MaxAtlases: 1, MaxEntries: 1}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMakeGlyphMaskKey(t *testing.T) {
	tests := []struct {
		name      string
		fontID    uint64
		glyphID   GlyphID
		size      float64
		subpixelX float64
		subpixelY float64
		wantQ4    int16
		wantSpxQ2 uint8
		wantSpyQ2 uint8
	}{
		{"13px no subpixel", 1, 65, 13.0, 0, 0, 208, 0, 0},
		{"14.5px", 1, 65, 14.5, 0, 0, 232, 0, 0},
		{"13px 0.25 subpixel", 1, 65, 13.0, 0.25, 0, 208, 1, 0},
		{"13px 0.5 subpixel", 1, 65, 13.0, 0.5, 0, 208, 2, 0},
		{"13px 0.75 subpixel", 1, 65, 13.0, 0.75, 0, 208, 3, 0},
		{"tiny size clamped", 1, 65, 0.01, 0, 0, 1, 0, 0},
		{"vertical subpixel", 1, 65, 13.0, 0, 0.5, 208, 0, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := MakeGlyphMaskKey(tt.fontID, tt.glyphID, tt.size, tt.subpixelX, tt.subpixelY)
			if key.SizeQ4 != tt.wantQ4 {
				t.Errorf("SizeQ4 = %d, want %d", key.SizeQ4, tt.wantQ4)
			}
			if key.SubpixelXQ2 != tt.wantSpxQ2 {
				t.Errorf("SubpixelXQ2 = %d, want %d", key.SubpixelXQ2, tt.wantSpxQ2)
			}
			if key.SubpixelYQ2 != tt.wantSpyQ2 {
				t.Errorf("SubpixelYQ2 = %d, want %d", key.SubpixelYQ2, tt.wantSpyQ2)
			}
		})
	}
}

func TestGlyphMaskAtlas_PutGet(t *testing.T) {
	atlas := NewGlyphMaskAtlasDefault()

	// Create a small 8x8 mask with some coverage
	mask := make([]byte, 64)
	for i := range mask {
		mask[i] = uint8(i * 4) //nolint:gosec // test data
	}

	key := MakeGlyphMaskKey(1, 65, 13.0, 0, 0)

	// Put
	region, err := atlas.Put(key, mask, 8, 8, -1.0, 10.0)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if region.Width != 8 || region.Height != 8 {
		t.Errorf("region dimensions = %dx%d, want 8x8", region.Width, region.Height)
	}
	if region.BearingX != -1.0 || region.BearingY != 10.0 {
		t.Errorf("bearings = (%f, %f), want (-1.0, 10.0)", region.BearingX, region.BearingY)
	}

	// Get — should be cached
	got, ok := atlas.Get(key)
	if !ok {
		t.Fatal("Get() returned false for cached glyph")
	}
	if got.Width != region.Width || got.Height != region.Height {
		t.Errorf("Get() returned different region")
	}

	// Different key — should miss
	key2 := MakeGlyphMaskKey(1, 66, 13.0, 0, 0)
	_, ok = atlas.Get(key2)
	if ok {
		t.Error("Get() returned true for uncached glyph")
	}
}

func TestGlyphMaskAtlas_DuplicatePut(t *testing.T) {
	atlas := NewGlyphMaskAtlasDefault()

	mask := make([]byte, 16)
	key := MakeGlyphMaskKey(1, 65, 13.0, 0, 0)

	// Put twice — should return same region
	r1, err := atlas.Put(key, mask, 4, 4, 0, 0)
	if err != nil {
		t.Fatalf("first Put() error = %v", err)
	}

	r2, err := atlas.Put(key, mask, 4, 4, 0, 0)
	if err != nil {
		t.Fatalf("second Put() error = %v", err)
	}

	if r1.X != r2.X || r1.Y != r2.Y {
		t.Errorf("duplicate Put() returned different positions: (%d,%d) vs (%d,%d)", r1.X, r1.Y, r2.X, r2.Y)
	}

	if atlas.EntryCount() != 1 {
		t.Errorf("EntryCount() = %d, want 1", atlas.EntryCount())
	}
}

func TestGlyphMaskAtlas_LRUEviction(t *testing.T) {
	config := GlyphMaskAtlasConfig{
		Size:       256,
		Padding:    1,
		MaxAtlases: 1,
		MaxEntries: 3,
	}
	atlas, err := NewGlyphMaskAtlas(config)
	if err != nil {
		t.Fatalf("NewGlyphMaskAtlas() error = %v", err)
	}

	mask := make([]byte, 4)

	// Fill to capacity
	for i := range 3 {
		key := MakeGlyphMaskKey(1, GlyphID(i), 13.0, 0, 0)
		_, err := atlas.Put(key, mask, 2, 2, 0, 0)
		if err != nil {
			t.Fatalf("Put(%d) error = %v", i, err)
		}
	}

	if atlas.EntryCount() != 3 {
		t.Fatalf("EntryCount() = %d, want 3", atlas.EntryCount())
	}

	// Access glyph 0 to make it recently used
	key0 := MakeGlyphMaskKey(1, 0, 13.0, 0, 0)
	_, ok := atlas.Get(key0)
	if !ok {
		t.Fatal("Get(0) failed")
	}

	// Add a 4th entry — should evict glyph 1 (LRU)
	key3 := MakeGlyphMaskKey(1, 3, 13.0, 0, 0)
	_, err = atlas.Put(key3, mask, 2, 2, 0, 0)
	if err != nil {
		t.Fatalf("Put(3) error = %v", err)
	}

	// Glyph 0 should still be cached (was accessed)
	_, ok = atlas.Get(key0)
	if !ok {
		t.Error("glyph 0 was evicted but shouldn't have been")
	}

	// Glyph 1 should have been evicted (LRU)
	key1 := MakeGlyphMaskKey(1, 1, 13.0, 0, 0)
	_, ok = atlas.Get(key1)
	if ok {
		t.Error("glyph 1 should have been evicted")
	}

	// Glyph 2 should still be cached
	key2 := MakeGlyphMaskKey(1, 2, 13.0, 0, 0)
	_, ok = atlas.Get(key2)
	if !ok {
		t.Error("glyph 2 was evicted but shouldn't have been")
	}
}

func TestGlyphMaskAtlas_Stats(t *testing.T) {
	atlas := NewGlyphMaskAtlasDefault()

	mask := make([]byte, 4)
	key := MakeGlyphMaskKey(1, 65, 13.0, 0, 0)

	// Miss
	_, _ = atlas.Get(key)

	// Put + Hit
	_, _ = atlas.Put(key, mask, 2, 2, 0, 0)
	_, _ = atlas.Get(key)

	hits, misses, entries, pages := atlas.Stats()
	if hits != 1 {
		t.Errorf("hits = %d, want 1", hits)
	}
	if misses != 1 {
		t.Errorf("misses = %d, want 1", misses)
	}
	if entries != 1 {
		t.Errorf("entries = %d, want 1", entries)
	}
	if pages != 1 {
		t.Errorf("pages = %d, want 1", pages)
	}
}

func TestGlyphMaskAtlas_Clear(t *testing.T) {
	atlas := NewGlyphMaskAtlasDefault()

	mask := make([]byte, 4)
	key := MakeGlyphMaskKey(1, 65, 13.0, 0, 0)
	_, _ = atlas.Put(key, mask, 2, 2, 0, 0)

	atlas.Clear()

	if atlas.EntryCount() != 0 {
		t.Errorf("EntryCount() after Clear = %d, want 0", atlas.EntryCount())
	}
	if atlas.PageCount() != 0 {
		t.Errorf("PageCount() after Clear = %d, want 0", atlas.PageCount())
	}
}

func TestGlyphMaskAtlas_DirtyPages(t *testing.T) {
	atlas := NewGlyphMaskAtlasDefault()

	mask := make([]byte, 4)
	key := MakeGlyphMaskKey(1, 65, 13.0, 0, 0)
	_, _ = atlas.Put(key, mask, 2, 2, 0, 0)

	dirty := atlas.DirtyPages()
	if len(dirty) != 1 || dirty[0] != 0 {
		t.Errorf("DirtyPages() = %v, want [0]", dirty)
	}

	atlas.MarkClean(0)

	dirty = atlas.DirtyPages()
	if len(dirty) != 0 {
		t.Errorf("DirtyPages() after MarkClean = %v, want []", dirty)
	}
}

func TestGlyphMaskAtlas_InvalidMask(t *testing.T) {
	atlas := NewGlyphMaskAtlasDefault()

	key := MakeGlyphMaskKey(1, 65, 13.0, 0, 0)

	// Empty mask
	_, err := atlas.Put(key, nil, 0, 0, 0, 0)
	if err == nil {
		t.Error("Put() with empty mask should return error")
	}

	// Mask too small for dimensions
	mask := make([]byte, 2)
	_, err = atlas.Put(key, mask, 4, 4, 0, 0)
	if err == nil {
		t.Error("Put() with undersized mask should return error")
	}
}

func TestGlyphMaskAtlas_GetOrRasterize(t *testing.T) {
	atlas := NewGlyphMaskAtlasDefault()
	key := MakeGlyphMaskKey(1, 65, 13.0, 0, 0)

	callCount := 0
	rasterize := func() ([]byte, int, int, float32, float32, error) {
		callCount++
		mask := make([]byte, 16)
		for i := range mask {
			mask[i] = 128
		}
		return mask, 4, 4, -1.0, 10.0, nil
	}

	// First call — rasterizes
	region, err := atlas.GetOrRasterize(key, rasterize)
	if err != nil {
		t.Fatalf("GetOrRasterize() error = %v", err)
	}
	if region.Width != 4 || region.Height != 4 {
		t.Errorf("region = %dx%d, want 4x4", region.Width, region.Height)
	}
	if callCount != 1 {
		t.Errorf("rasterize called %d times, want 1", callCount)
	}

	// Second call — cached
	region2, err := atlas.GetOrRasterize(key, rasterize)
	if err != nil {
		t.Fatalf("second GetOrRasterize() error = %v", err)
	}
	if callCount != 1 {
		t.Errorf("rasterize called %d times on cache hit, want 1", callCount)
	}
	if region2.X != region.X || region2.Y != region.Y {
		t.Error("cached region differs from original")
	}
}

func TestGlyphMaskAtlas_MemoryUsage(t *testing.T) {
	config := GlyphMaskAtlasConfig{
		Size:       64,
		Padding:    0,
		MaxAtlases: 2,
		MaxEntries: 100,
	}
	atlas, err := NewGlyphMaskAtlas(config)
	if err != nil {
		t.Fatalf("NewGlyphMaskAtlas() error = %v", err)
	}

	if atlas.MemoryUsage() != 0 {
		t.Errorf("MemoryUsage() before any Put = %d, want 0", atlas.MemoryUsage())
	}

	mask := make([]byte, 4)
	key := MakeGlyphMaskKey(1, 65, 13.0, 0, 0)
	_, _ = atlas.Put(key, mask, 2, 2, 0, 0)

	// One 64x64 R8 page = 4096 bytes
	if atlas.MemoryUsage() != 64*64 {
		t.Errorf("MemoryUsage() = %d, want %d", atlas.MemoryUsage(), 64*64)
	}
}

func TestGlyphMaskAtlas_SubpixelVariants(t *testing.T) {
	atlas := NewGlyphMaskAtlasDefault()
	mask := make([]byte, 4)

	// Same glyph, 4 different subpixel X positions should produce 4 distinct cache entries
	for i := range 4 {
		key := MakeGlyphMaskKey(1, 65, 13.0, float64(i)*0.25, 0)
		_, err := atlas.Put(key, mask, 2, 2, 0, 0)
		if err != nil {
			t.Fatalf("Put(subpixel=%d) error = %v", i, err)
		}
	}

	if atlas.EntryCount() != 4 {
		t.Errorf("EntryCount() = %d, want 4 (one per subpixel variant)", atlas.EntryCount())
	}
}

func TestGlyphMaskShelfAllocator(t *testing.T) {
	alloc := newGlyphMaskShelfAllocator(64, 64, 1)

	// Allocate a few items
	x1, y1, ok := alloc.Allocate(10, 12)
	if !ok {
		t.Fatal("first Allocate failed")
	}
	if x1 != 0 || y1 != 0 {
		t.Errorf("first allocation at (%d, %d), want (0, 0)", x1, y1)
	}

	x2, y2, ok := alloc.Allocate(10, 12)
	if !ok {
		t.Fatal("second Allocate failed")
	}
	if x2 != 11 || y2 != 0 {
		t.Errorf("second allocation at (%d, %d), want (11, 0)", x2, y2)
	}

	// Fill the row, then start new shelf
	for i := range 3 {
		_, _, ok = alloc.Allocate(10, 12)
		if !ok {
			t.Fatalf("allocation %d failed", i+3)
		}
	}

	// Should start a new shelf
	x5, y5, ok := alloc.Allocate(10, 8)
	if !ok {
		t.Fatal("new shelf Allocate failed")
	}
	if y5 <= y1 {
		t.Errorf("new shelf at y=%d should be below first shelf at y=%d", y5, y1)
	}
	_ = x5
}

func TestGlyphMaskShelfAllocator_CanFit(t *testing.T) {
	alloc := newGlyphMaskShelfAllocator(64, 64, 1)

	if !alloc.CanFit(10, 10) {
		t.Error("should be able to fit 10x10 in empty 64x64 atlas")
	}

	if alloc.CanFit(65, 10) {
		t.Error("should not fit 65-wide in 64-wide atlas")
	}

	if alloc.CanFit(10, 65) {
		t.Error("should not fit 65-tall in 64-tall atlas")
	}
}
