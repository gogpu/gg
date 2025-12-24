package msdf

import (
	"errors"
	"sync"
	"testing"

	"github.com/gogpu/gg/text"
)

// --- ShelfAllocator Tests ---

func TestShelfAllocator_Basic(t *testing.T) {
	a := NewShelfAllocator(100, 100, 2)

	x, y, ok := a.Allocate(20, 20)
	if !ok {
		t.Fatal("failed to allocate first cell")
	}
	if x != 0 || y != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", x, y)
	}

	x, y, ok = a.Allocate(20, 20)
	if !ok {
		t.Fatal("failed to allocate second cell")
	}
	if x != 22 || y != 0 { // 20 + 2 padding
		t.Errorf("expected (22,0), got (%d,%d)", x, y)
	}
}

func TestShelfAllocator_NewShelf(t *testing.T) {
	a := NewShelfAllocator(50, 100, 2)

	// First cell
	_, y1, ok := a.Allocate(20, 20)
	if !ok {
		t.Fatal("failed to allocate first cell")
	}

	// Second cell - should fit on same shelf
	_, y2, ok := a.Allocate(20, 20)
	if !ok {
		t.Fatal("failed to allocate second cell")
	}
	if y2 != y1 {
		t.Errorf("expected same shelf, got y1=%d, y2=%d", y1, y2)
	}

	// Third cell - should need new shelf
	x3, y3, ok := a.Allocate(20, 20)
	if !ok {
		t.Fatal("failed to allocate third cell")
	}
	if y3 <= y1 {
		t.Errorf("expected new shelf, got y1=%d, y3=%d", y1, y3)
	}
	if x3 != 0 {
		t.Errorf("expected x=0 for new shelf, got %d", x3)
	}
}

func TestShelfAllocator_Full(t *testing.T) {
	a := NewShelfAllocator(50, 50, 2)

	// Fill up the allocator
	count := 0
	for {
		_, _, ok := a.Allocate(20, 20)
		if !ok {
			break
		}
		count++
		if count > 100 {
			t.Fatal("allocator never filled up")
		}
	}

	if count != 4 { // 2x2 grid of 20+2 in 50x50
		t.Errorf("expected 4 allocations, got %d", count)
	}
}

func TestShelfAllocator_Utilization(t *testing.T) {
	a := NewShelfAllocator(100, 100, 0)

	if a.Utilization() != 0 {
		t.Errorf("expected 0 utilization initially, got %f", a.Utilization())
	}

	a.Allocate(50, 50)
	util := a.Utilization()
	if util != 0.25 {
		t.Errorf("expected 0.25 utilization, got %f", util)
	}
}

func TestShelfAllocator_Reset(t *testing.T) {
	a := NewShelfAllocator(100, 100, 2)

	a.Allocate(20, 20)
	a.Allocate(20, 20)

	if a.ShelfCount() == 0 {
		t.Error("expected shelves before reset")
	}

	a.Reset()

	if a.ShelfCount() != 0 {
		t.Error("expected no shelves after reset")
	}
	if a.Utilization() != 0 {
		t.Error("expected 0 utilization after reset")
	}
}

func TestShelfAllocator_CanFit(t *testing.T) {
	a := NewShelfAllocator(100, 100, 2)

	if !a.CanFit(20, 20) {
		t.Error("should be able to fit 20x20 in empty allocator")
	}

	if a.CanFit(150, 20) {
		t.Error("should not fit item wider than allocator")
	}

	if a.CanFit(20, 150) {
		t.Error("should not fit item taller than allocator")
	}
}

func TestShelfAllocator_VariableHeights(t *testing.T) {
	a := NewShelfAllocator(100, 100, 2)

	// First row with height 20
	a.Allocate(20, 20)

	// Same row, shorter item
	_, y, ok := a.Allocate(20, 10)
	if !ok {
		t.Fatal("failed to allocate shorter item")
	}
	if y != 0 {
		t.Errorf("expected same shelf, got y=%d", y)
	}

	// Fill first row
	a.Allocate(20, 20)
	a.Allocate(20, 20)

	// New row should start at height 20 + padding
	_, y2, ok := a.Allocate(20, 30)
	if !ok {
		t.Fatal("failed to allocate on new shelf")
	}
	if y2 != 22 { // 20 + 2 padding
		t.Errorf("expected y=22 for new shelf, got %d", y2)
	}
}

// --- GridAllocator Tests ---

func TestGridAllocator_Basic(t *testing.T) {
	g := NewGridAllocator(100, 100, 20, 2)

	x, y, ok := g.Allocate()
	if !ok {
		t.Fatal("failed to allocate first cell")
	}
	if x != 0 || y != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", x, y)
	}

	x, y, ok = g.Allocate()
	if !ok {
		t.Fatal("failed to allocate second cell")
	}
	if x != 22 || y != 0 { // cellSize + padding
		t.Errorf("expected (22,0), got (%d,%d)", x, y)
	}
}

func TestGridAllocator_Capacity(t *testing.T) {
	g := NewGridAllocator(100, 100, 20, 2) // 4 cols x 4 rows = 16

	cols, rows := g.GridDimensions()
	expectedCols, expectedRows := 4, 4
	if cols != expectedCols || rows != expectedRows {
		t.Errorf("expected %dx%d grid, got %dx%d", expectedCols, expectedRows, cols, rows)
	}

	if g.Capacity() != 16 {
		t.Errorf("expected capacity 16, got %d", g.Capacity())
	}
}

func TestGridAllocator_Full(t *testing.T) {
	g := NewGridAllocator(50, 50, 20, 2) // 2 cols x 2 rows = 4

	for i := 0; i < 4; i++ {
		_, _, ok := g.Allocate()
		if !ok {
			t.Fatalf("failed to allocate cell %d", i)
		}
	}

	if !g.IsFull() {
		t.Error("grid should be full")
	}

	_, _, ok := g.Allocate()
	if ok {
		t.Error("should not allocate when full")
	}
}

func TestGridAllocator_Utilization(t *testing.T) {
	g := NewGridAllocator(100, 100, 20, 2)

	if g.Utilization() != 0 {
		t.Errorf("expected 0 utilization initially, got %f", g.Utilization())
	}

	g.Allocate()
	g.Allocate()

	expected := 2.0 / float64(g.Capacity())
	if g.Utilization() != expected {
		t.Errorf("expected %f utilization, got %f", expected, g.Utilization())
	}
}

func TestGridAllocator_Reset(t *testing.T) {
	g := NewGridAllocator(100, 100, 20, 2)

	g.Allocate()
	g.Allocate()

	if g.Allocated() != 2 {
		t.Errorf("expected 2 allocated, got %d", g.Allocated())
	}

	g.Reset()

	if g.Allocated() != 0 {
		t.Error("expected 0 allocated after reset")
	}
}

// --- AtlasConfig Tests ---

func TestAtlasConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  AtlasConfig
		wantErr bool
	}{
		{
			name:    "default is valid",
			config:  DefaultAtlasConfig(),
			wantErr: false,
		},
		{
			name:    "size too small",
			config:  AtlasConfig{Size: 32, GlyphSize: 32, Padding: 2, MaxAtlases: 8},
			wantErr: true,
		},
		{
			name:    "size too large",
			config:  AtlasConfig{Size: 16384, GlyphSize: 32, Padding: 2, MaxAtlases: 8},
			wantErr: true,
		},
		{
			name:    "size not power of 2",
			config:  AtlasConfig{Size: 1000, GlyphSize: 32, Padding: 2, MaxAtlases: 8},
			wantErr: true,
		},
		{
			name:    "glyph size too small",
			config:  AtlasConfig{Size: 1024, GlyphSize: 4, Padding: 2, MaxAtlases: 8},
			wantErr: true,
		},
		{
			name:    "glyph size larger than atlas",
			config:  AtlasConfig{Size: 64, GlyphSize: 128, Padding: 2, MaxAtlases: 8},
			wantErr: true,
		},
		{
			name:    "negative padding",
			config:  AtlasConfig{Size: 1024, GlyphSize: 32, Padding: -1, MaxAtlases: 8},
			wantErr: true,
		},
		{
			name:    "padding too large",
			config:  AtlasConfig{Size: 1024, GlyphSize: 32, Padding: 20, MaxAtlases: 8},
			wantErr: true,
		},
		{
			name:    "max atlases too small",
			config:  AtlasConfig{Size: 1024, GlyphSize: 32, Padding: 2, MaxAtlases: 0},
			wantErr: true,
		},
		{
			name:    "max atlases too large",
			config:  AtlasConfig{Size: 1024, GlyphSize: 32, Padding: 2, MaxAtlases: 512},
			wantErr: true,
		},
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

// --- Atlas Tests ---

func TestAtlas_Creation(t *testing.T) {
	atlas := newAtlas(0, 256, 32, 2)

	if atlas.Size != 256 {
		t.Errorf("expected size 256, got %d", atlas.Size)
	}
	if len(atlas.Data) != 256*256*3 {
		t.Errorf("expected data length %d, got %d", 256*256*3, len(atlas.Data))
	}
	if atlas.IsFull() {
		t.Error("new atlas should not be full")
	}
	if atlas.IsDirty() {
		t.Error("new atlas should not be dirty")
	}
}

func TestAtlas_GlyphCount(t *testing.T) {
	atlas := newAtlas(0, 256, 32, 2)

	if atlas.GlyphCount() != 0 {
		t.Errorf("expected 0 glyphs, got %d", atlas.GlyphCount())
	}
}

// --- AtlasManager Tests ---

func TestAtlasManager_Creation(t *testing.T) {
	m, err := NewAtlasManager(DefaultAtlasConfig())
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	if m.AtlasCount() != 0 {
		t.Errorf("expected 0 atlases initially, got %d", m.AtlasCount())
	}

	if m.GlyphCount() != 0 {
		t.Errorf("expected 0 glyphs initially, got %d", m.GlyphCount())
	}
}

func TestAtlasManager_InvalidConfig(t *testing.T) {
	_, err := NewAtlasManager(AtlasConfig{Size: 10, GlyphSize: 8, Padding: 2, MaxAtlases: 1})
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestAtlasManager_Get(t *testing.T) {
	m := NewAtlasManagerDefault()

	// Create a simple outline
	outline := createTestOutline()

	key := GlyphKey{FontID: 1, GlyphID: 65, Size: 32}
	region, err := m.Get(key, outline)
	if err != nil {
		t.Fatalf("failed to get glyph: %v", err)
	}

	if region.AtlasIndex != 0 {
		t.Errorf("expected atlas index 0, got %d", region.AtlasIndex)
	}
	if region.Width != m.Config().GlyphSize {
		t.Errorf("expected width %d, got %d", m.Config().GlyphSize, region.Width)
	}

	// Verify atlas was created
	if m.AtlasCount() != 1 {
		t.Errorf("expected 1 atlas, got %d", m.AtlasCount())
	}

	// Verify glyph was cached
	if m.GlyphCount() != 1 {
		t.Errorf("expected 1 glyph, got %d", m.GlyphCount())
	}
}

func TestAtlasManager_GetCached(t *testing.T) {
	m := NewAtlasManagerDefault()

	outline := createTestOutline()
	key := GlyphKey{FontID: 1, GlyphID: 65, Size: 32}

	// First get - miss
	_, err := m.Get(key, outline)
	if err != nil {
		t.Fatalf("failed to get glyph: %v", err)
	}

	// Second get - hit
	region2, err := m.Get(key, outline)
	if err != nil {
		t.Fatalf("failed to get cached glyph: %v", err)
	}

	hits, misses, _ := m.Stats()
	if hits != 1 {
		t.Errorf("expected 1 hit, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("expected 1 miss, got %d", misses)
	}

	// Verify same region returned (first glyph is at origin)
	if region2.X != 0 || region2.Y != 0 || region2.AtlasIndex != 0 {
		t.Errorf("expected region at (0,0) in atlas 0, got (%d,%d) in atlas %d",
			region2.X, region2.Y, region2.AtlasIndex)
	}
}

func TestAtlasManager_GetBatch(t *testing.T) {
	m := NewAtlasManagerDefault()

	// Create multiple outlines
	keys := make([]GlyphKey, 5)
	outlines := make([]*text.GlyphOutline, 5)
	for i := 0; i < 5; i++ {
		keys[i] = GlyphKey{FontID: 1, GlyphID: uint16(65 + i), Size: 32}
		outlines[i] = createTestOutline()
	}

	regions, err := m.GetBatch(keys, outlines)
	if err != nil {
		t.Fatalf("failed to get batch: %v", err)
	}

	if len(regions) != 5 {
		t.Errorf("expected 5 regions, got %d", len(regions))
	}

	if m.GlyphCount() != 5 {
		t.Errorf("expected 5 glyphs, got %d", m.GlyphCount())
	}
}

func TestAtlasManager_GetBatch_LengthMismatch(t *testing.T) {
	m := NewAtlasManagerDefault()

	keys := make([]GlyphKey, 3)
	outlines := make([]*text.GlyphOutline, 2) // Different length

	_, err := m.GetBatch(keys, outlines)
	if err == nil {
		t.Error("expected error for length mismatch")
	}
}

func TestAtlasManager_HasGlyph(t *testing.T) {
	m := NewAtlasManagerDefault()

	key := GlyphKey{FontID: 1, GlyphID: 65, Size: 32}

	if m.HasGlyph(key) {
		t.Error("should not have glyph before adding")
	}

	_, _ = m.Get(key, createTestOutline())

	if !m.HasGlyph(key) {
		t.Error("should have glyph after adding")
	}
}

func TestAtlasManager_Clear(t *testing.T) {
	m := NewAtlasManagerDefault()

	// Add some glyphs
	for i := 0; i < 10; i++ {
		key := GlyphKey{FontID: 1, GlyphID: uint16(65 + i), Size: 32}
		_, _ = m.Get(key, createTestOutline())
	}

	if m.GlyphCount() != 10 {
		t.Errorf("expected 10 glyphs, got %d", m.GlyphCount())
	}

	m.Clear()

	if m.GlyphCount() != 0 {
		t.Errorf("expected 0 glyphs after clear, got %d", m.GlyphCount())
	}
	if m.AtlasCount() != 0 {
		t.Errorf("expected 0 atlases after clear, got %d", m.AtlasCount())
	}

	hits, misses, _ := m.Stats()
	if hits != 0 || misses != 0 {
		t.Error("stats should be reset after clear")
	}
}

func TestAtlasManager_Remove(t *testing.T) {
	m := NewAtlasManagerDefault()

	key := GlyphKey{FontID: 1, GlyphID: 65, Size: 32}
	_, _ = m.Get(key, createTestOutline())

	if !m.HasGlyph(key) {
		t.Error("should have glyph after adding")
	}

	removed := m.Remove(key)
	if !removed {
		t.Error("should return true when removing existing glyph")
	}

	if m.HasGlyph(key) {
		t.Error("should not have glyph after removing")
	}

	// Remove non-existent
	removed = m.Remove(GlyphKey{FontID: 99, GlyphID: 99, Size: 32})
	if removed {
		t.Error("should return false when removing non-existent glyph")
	}
}

func TestAtlasManager_DirtyAtlases(t *testing.T) {
	m := NewAtlasManagerDefault()

	// Initially no dirty atlases
	dirty := m.DirtyAtlases()
	if len(dirty) != 0 {
		t.Errorf("expected no dirty atlases initially, got %d", len(dirty))
	}

	// Add a glyph - creates dirty atlas
	_, _ = m.Get(GlyphKey{FontID: 1, GlyphID: 65, Size: 32}, createTestOutline())

	dirty = m.DirtyAtlases()
	if len(dirty) != 1 {
		t.Errorf("expected 1 dirty atlas, got %d", len(dirty))
	}
	if dirty[0] != 0 {
		t.Errorf("expected atlas 0 to be dirty, got %d", dirty[0])
	}

	// Mark clean
	m.MarkClean(0)

	dirty = m.DirtyAtlases()
	if len(dirty) != 0 {
		t.Errorf("expected no dirty atlases after marking clean, got %d", len(dirty))
	}
}

func TestAtlasManager_GetAtlas(t *testing.T) {
	m := NewAtlasManagerDefault()

	// No atlases yet
	if m.GetAtlas(0) != nil {
		t.Error("should return nil for non-existent atlas")
	}

	// Add a glyph to create an atlas
	_, _ = m.Get(GlyphKey{FontID: 1, GlyphID: 65, Size: 32}, createTestOutline())

	atlas := m.GetAtlas(0)
	if atlas == nil {
		t.Fatal("should return atlas after creating one")
	}
	if atlas.Size != m.Config().Size {
		t.Errorf("atlas size mismatch: expected %d, got %d", m.Config().Size, atlas.Size)
	}

	// Out of range
	if m.GetAtlas(-1) != nil {
		t.Error("should return nil for negative index")
	}
	if m.GetAtlas(100) != nil {
		t.Error("should return nil for out of range index")
	}
}

func TestAtlasManager_MemoryUsage(t *testing.T) {
	config := AtlasConfig{
		Size:       256,
		GlyphSize:  32,
		Padding:    2,
		MaxAtlases: 8,
	}
	m, _ := NewAtlasManager(config)

	// Initially no memory
	if m.MemoryUsage() != 0 {
		t.Errorf("expected 0 memory initially, got %d", m.MemoryUsage())
	}

	// Add a glyph
	_, _ = m.Get(GlyphKey{FontID: 1, GlyphID: 65, Size: 32}, createTestOutline())

	expected := int64(256 * 256 * 3) // One atlas worth of RGB data
	if m.MemoryUsage() != expected {
		t.Errorf("expected %d bytes, got %d", expected, m.MemoryUsage())
	}
}

func TestAtlasManager_AtlasInfos(t *testing.T) {
	m := NewAtlasManagerDefault()

	// Add some glyphs
	for i := 0; i < 5; i++ {
		key := GlyphKey{FontID: 1, GlyphID: uint16(65 + i), Size: 32}
		_, _ = m.Get(key, createTestOutline())
	}

	infos := m.AtlasInfos()
	if len(infos) != 1 {
		t.Errorf("expected 1 atlas info, got %d", len(infos))
	}

	if infos[0].GlyphCount != 5 {
		t.Errorf("expected 5 glyphs in atlas info, got %d", infos[0].GlyphCount)
	}
	if !infos[0].Dirty {
		t.Error("atlas should be dirty")
	}
}

func TestAtlasManager_MultipleAtlases(t *testing.T) {
	config := AtlasConfig{
		Size:       64, // Small atlas
		GlyphSize:  32,
		Padding:    0,
		MaxAtlases: 8,
	}
	m, _ := NewAtlasManager(config)

	// 64x64 with 32x32 cells = 2x2 = 4 cells per atlas
	// Adding 10 glyphs should create 3 atlases
	for i := 0; i < 10; i++ {
		key := GlyphKey{FontID: 1, GlyphID: uint16(65 + i), Size: 32}
		_, err := m.Get(key, createTestOutline())
		if err != nil {
			t.Fatalf("failed to add glyph %d: %v", i, err)
		}
	}

	if m.AtlasCount() != 3 {
		t.Errorf("expected 3 atlases, got %d", m.AtlasCount())
	}
}

func TestAtlasManager_AtlasFull(t *testing.T) {
	config := AtlasConfig{
		Size:       64, // Small atlas
		GlyphSize:  32,
		Padding:    0,
		MaxAtlases: 1, // Only 1 atlas allowed
	}
	m, _ := NewAtlasManager(config)

	// 64x64 with 32x32 cells = 2x2 = 4 cells
	// Fifth glyph should fail
	for i := 0; i < 4; i++ {
		key := GlyphKey{FontID: 1, GlyphID: uint16(65 + i), Size: 32}
		_, err := m.Get(key, createTestOutline())
		if err != nil {
			t.Fatalf("failed to add glyph %d: %v", i, err)
		}
	}

	key := GlyphKey{FontID: 1, GlyphID: 100, Size: 32}
	_, err := m.Get(key, createTestOutline())
	if err == nil {
		t.Error("expected error when atlas is full")
	}

	var fullErr *AtlasFullError
	if errors.As(err, &fullErr) {
		if fullErr.MaxAtlases != 1 {
			t.Errorf("expected MaxAtlases=1 in error, got %d", fullErr.MaxAtlases)
		}
	} else {
		t.Errorf("expected AtlasFullError, got %T: %v", err, err)
	}
}

func TestAtlasManager_Concurrent(t *testing.T) {
	m := NewAtlasManagerDefault()

	var wg sync.WaitGroup
	numGoroutines := 10
	numGlyphs := 20

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < numGlyphs; i++ {
				key := GlyphKey{FontID: uint64(gid), GlyphID: uint16(i), Size: 32}
				_, err := m.Get(key, createTestOutline())
				if err != nil {
					t.Errorf("goroutine %d: failed to get glyph %d: %v", gid, i, err)
				}
			}
		}(g)
	}

	wg.Wait()

	expected := numGoroutines * numGlyphs
	if m.GlyphCount() != expected {
		t.Errorf("expected %d glyphs, got %d", expected, m.GlyphCount())
	}
}

func TestAtlasManager_ConcurrentReadWrite(t *testing.T) {
	m := NewAtlasManagerDefault()

	// Pre-populate with some glyphs
	for i := 0; i < 10; i++ {
		key := GlyphKey{FontID: 1, GlyphID: uint16(i), Size: 32}
		_, _ = m.Get(key, createTestOutline())
	}

	var wg sync.WaitGroup

	// Writers
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				key := GlyphKey{FontID: uint64(wid + 10), GlyphID: uint16(i), Size: 32}
				_, _ = m.Get(key, createTestOutline())
			}
		}(w)
	}

	// Readers
	for r := 0; r < 10; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				key := GlyphKey{FontID: 1, GlyphID: uint16(i % 10), Size: 32}
				m.HasGlyph(key)
				_ = m.GlyphCount()
				_ = m.AtlasCount()
			}
		}()
	}

	wg.Wait()
}

// --- ConcurrentAtlasManager Tests ---

func TestConcurrentAtlasManager_Creation(t *testing.T) {
	c, err := NewConcurrentAtlasManager(DefaultAtlasConfig(), 4)
	if err != nil {
		t.Fatalf("failed to create concurrent manager: %v", err)
	}

	if c.GlyphCount() != 0 {
		t.Errorf("expected 0 glyphs initially, got %d", c.GlyphCount())
	}
}

func TestConcurrentAtlasManager_Get(t *testing.T) {
	c, _ := NewConcurrentAtlasManager(DefaultAtlasConfig(), 4)

	key := GlyphKey{FontID: 1, GlyphID: 65, Size: 32}
	outline := createTestOutline()

	region, err := c.Get(key, outline)
	if err != nil {
		t.Fatalf("failed to get glyph: %v", err)
	}

	if region.Width != DefaultAtlasConfig().GlyphSize {
		t.Errorf("expected width %d, got %d", DefaultAtlasConfig().GlyphSize, region.Width)
	}

	if !c.HasGlyph(key) {
		t.Error("should have glyph after adding")
	}
}

func TestConcurrentAtlasManager_Concurrent(t *testing.T) {
	c, _ := NewConcurrentAtlasManager(DefaultAtlasConfig(), 8)

	var wg sync.WaitGroup
	numGoroutines := 20
	numGlyphs := 50

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < numGlyphs; i++ {
				key := GlyphKey{FontID: uint64(gid), GlyphID: uint16(i), Size: 32}
				_, err := c.Get(key, createTestOutline())
				if err != nil {
					t.Errorf("goroutine %d: failed to get glyph %d: %v", gid, i, err)
				}
			}
		}(g)
	}

	wg.Wait()

	expected := numGoroutines * numGlyphs
	if c.GlyphCount() != expected {
		t.Errorf("expected %d glyphs, got %d", expected, c.GlyphCount())
	}
}

func TestConcurrentAtlasManager_Stats(t *testing.T) {
	c, _ := NewConcurrentAtlasManager(DefaultAtlasConfig(), 4)

	// Add and access glyphs
	key := GlyphKey{FontID: 1, GlyphID: 65, Size: 32}
	_, _ = c.Get(key, createTestOutline())
	_, _ = c.Get(key, createTestOutline()) // Hit

	hits, misses, atlasCount := c.Stats()
	if hits != 1 {
		t.Errorf("expected 1 hit, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("expected 1 miss, got %d", misses)
	}
	if atlasCount != 1 {
		t.Errorf("expected 1 atlas, got %d", atlasCount)
	}
}

func TestConcurrentAtlasManager_Clear(t *testing.T) {
	c, _ := NewConcurrentAtlasManager(DefaultAtlasConfig(), 4)

	// Add glyphs to different shards
	for i := 0; i < 10; i++ {
		key := GlyphKey{FontID: uint64(i), GlyphID: 65, Size: 32}
		_, _ = c.Get(key, createTestOutline())
	}

	if c.GlyphCount() != 10 {
		t.Errorf("expected 10 glyphs, got %d", c.GlyphCount())
	}

	c.Clear()

	if c.GlyphCount() != 0 {
		t.Errorf("expected 0 glyphs after clear, got %d", c.GlyphCount())
	}
}

func TestConcurrentAtlasManager_MemoryUsage(t *testing.T) {
	c, _ := NewConcurrentAtlasManager(DefaultAtlasConfig(), 4)

	if c.MemoryUsage() != 0 {
		t.Errorf("expected 0 memory initially, got %d", c.MemoryUsage())
	}

	// Add glyph (creates atlas in one shard)
	key := GlyphKey{FontID: 1, GlyphID: 65, Size: 32}
	_, _ = c.Get(key, createTestOutline())

	if c.MemoryUsage() == 0 {
		t.Error("expected non-zero memory after adding glyph")
	}
}

// --- Benchmarks ---

func BenchmarkShelfAllocator_Allocate(b *testing.B) {
	a := NewShelfAllocator(1024, 1024, 2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%100 == 0 {
			a.Reset()
		}
		a.Allocate(32, 32)
	}
}

func BenchmarkGridAllocator_Allocate(b *testing.B) {
	g := NewGridAllocator(1024, 1024, 32, 2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%100 == 0 {
			g.Reset()
		}
		g.Allocate()
	}
}

func BenchmarkAtlasManager_Get_Miss(b *testing.B) {
	m := NewAtlasManagerDefault()
	outline := createTestOutline()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := GlyphKey{FontID: 1, GlyphID: uint16(i % 65535), Size: 32}
		_, _ = m.Get(key, outline)
	}
}

func BenchmarkAtlasManager_Get_Hit(b *testing.B) {
	m := NewAtlasManagerDefault()
	outline := createTestOutline()

	// Pre-populate
	key := GlyphKey{FontID: 1, GlyphID: 65, Size: 32}
	_, _ = m.Get(key, outline)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.Get(key, outline)
	}
}

func BenchmarkAtlasManager_HasGlyph(b *testing.B) {
	m := NewAtlasManagerDefault()
	outline := createTestOutline()

	// Pre-populate
	key := GlyphKey{FontID: 1, GlyphID: 65, Size: 32}
	_, _ = m.Get(key, outline)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.HasGlyph(key)
	}
}

func BenchmarkAtlasManager_Concurrent(b *testing.B) {
	m := NewAtlasManagerDefault()
	outline := createTestOutline()

	// Pre-populate
	for i := 0; i < 100; i++ {
		key := GlyphKey{FontID: 1, GlyphID: uint16(i), Size: 32}
		_, _ = m.Get(key, outline)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := GlyphKey{FontID: 1, GlyphID: uint16(i % 100), Size: 32}
			_, _ = m.Get(key, outline)
			i++
		}
	})
}

func BenchmarkConcurrentAtlasManager_Get(b *testing.B) {
	c, _ := NewConcurrentAtlasManager(DefaultAtlasConfig(), 8)
	outline := createTestOutline()

	// Pre-populate each shard
	for i := 0; i < 100; i++ {
		key := GlyphKey{FontID: uint64(i), GlyphID: 65, Size: 32}
		_, _ = c.Get(key, outline)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := GlyphKey{FontID: uint64(i % 100), GlyphID: 65, Size: 32}
			_, _ = c.Get(key, outline)
			i++
		}
	})
}

// --- Test Helpers ---

// createTestOutline creates a simple square outline for testing.
func createTestOutline() *text.GlyphOutline {
	return &text.GlyphOutline{
		Segments: []text.OutlineSegment{
			{Op: text.OutlineOpMoveTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 100, Y: 0}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 100, Y: 100}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 100}}},
			{Op: text.OutlineOpLineTo, Points: [3]text.OutlinePoint{{X: 0, Y: 0}}},
		},
		Bounds: text.Rect{
			MinX: 0, MinY: 0,
			MaxX: 100, MaxY: 100,
		},
		Advance: 110,
		LSB:     5,
		GID:     65,
		Type:    text.GlyphTypeOutline,
	}
}
