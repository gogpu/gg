package text

import (
	"sync"
	"testing"
)

func TestSubpixelMode_String(t *testing.T) {
	tests := []struct {
		mode SubpixelMode
		want string
	}{
		{SubpixelNone, "None"},
		{Subpixel4, "Subpixel4"},
		{Subpixel10, "Subpixel10"},
		{SubpixelMode(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("SubpixelMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestSubpixelMode_IsEnabled(t *testing.T) {
	tests := []struct {
		mode SubpixelMode
		want bool
	}{
		{SubpixelNone, false},
		{Subpixel4, true},
		{Subpixel10, true},
		{SubpixelMode(-1), false},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			if got := tt.mode.IsEnabled(); got != tt.want {
				t.Errorf("SubpixelMode(%d).IsEnabled() = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestSubpixelMode_Divisions(t *testing.T) {
	tests := []struct {
		mode SubpixelMode
		want int
	}{
		{SubpixelNone, 1},
		{Subpixel4, 4},
		{Subpixel10, 10},
		{SubpixelMode(-1), 1},
	}

	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			if got := tt.mode.Divisions(); got != tt.want {
				t.Errorf("SubpixelMode(%d).Divisions() = %d, want %d", tt.mode, got, tt.want)
			}
		})
	}
}

func TestDefaultSubpixelConfig(t *testing.T) {
	config := DefaultSubpixelConfig()

	if config.Mode != Subpixel4 {
		t.Errorf("Mode = %v, want Subpixel4", config.Mode)
	}
	if !config.Horizontal {
		t.Error("Horizontal should be true")
	}
	if config.Vertical {
		t.Error("Vertical should be false")
	}
}

func TestNoSubpixelConfig(t *testing.T) {
	config := NoSubpixelConfig()

	if config.Mode != SubpixelNone {
		t.Errorf("Mode = %v, want SubpixelNone", config.Mode)
	}
	if config.Horizontal {
		t.Error("Horizontal should be false")
	}
	if config.Vertical {
		t.Error("Vertical should be false")
	}
}

func TestHighQualitySubpixelConfig(t *testing.T) {
	config := HighQualitySubpixelConfig()

	if config.Mode != Subpixel10 {
		t.Errorf("Mode = %v, want Subpixel10", config.Mode)
	}
	if !config.Horizontal {
		t.Error("Horizontal should be true")
	}
	if config.Vertical {
		t.Error("Vertical should be false")
	}
}

func TestSubpixelConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config SubpixelConfig
		want   bool
	}{
		{"default enabled", DefaultSubpixelConfig(), true},
		{"none disabled", NoSubpixelConfig(), false},
		{"mode enabled but no axis", SubpixelConfig{Mode: Subpixel4, Horizontal: false, Vertical: false}, false},
		{"vertical only", SubpixelConfig{Mode: Subpixel4, Horizontal: false, Vertical: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubpixelConfig_CacheMultiplier(t *testing.T) {
	tests := []struct {
		name   string
		config SubpixelConfig
		want   int
	}{
		{"none", NoSubpixelConfig(), 1},
		{"horizontal only 4", SubpixelConfig{Mode: Subpixel4, Horizontal: true, Vertical: false}, 4},
		{"horizontal only 10", SubpixelConfig{Mode: Subpixel10, Horizontal: true, Vertical: false}, 10},
		{"vertical only 4", SubpixelConfig{Mode: Subpixel4, Horizontal: false, Vertical: true}, 4},
		{"both 4", SubpixelConfig{Mode: Subpixel4, Horizontal: true, Vertical: true}, 16},
		{"both 10", SubpixelConfig{Mode: Subpixel10, Horizontal: true, Vertical: true}, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.CacheMultiplier(); got != tt.want {
				t.Errorf("CacheMultiplier() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestQuantize_Subpixel4(t *testing.T) {
	tests := []struct {
		pos     float64
		wantInt int
		wantSub uint8
	}{
		{0.0, 0, 0},
		{0.1, 0, 0},   // 0.1 * 4 = 0.4 -> 0
		{0.25, 0, 1},  // 0.25 * 4 = 1.0 -> 1
		{0.5, 0, 2},   // 0.5 * 4 = 2.0 -> 2
		{0.75, 0, 3},  // 0.75 * 4 = 3.0 -> 3
		{0.99, 0, 3},  // 0.99 * 4 = 3.96 -> 3
		{1.0, 1, 0},   // next integer
		{1.25, 1, 1},  // 1.25 -> int=1, frac=0.25 -> sub=1
		{10.3, 10, 1}, // 10.3 -> int=10, frac=0.3 -> 0.3*4=1.2 -> 1
		{10.5, 10, 2},
		{10.7, 10, 2}, // 0.7 * 4 = 2.8 -> 2
		{10.8, 10, 3}, // 0.8 * 4 = 3.2 -> 3
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			gotInt, gotSub := Quantize(tt.pos, Subpixel4)
			if gotInt != tt.wantInt || gotSub != tt.wantSub {
				t.Errorf("Quantize(%v, Subpixel4) = (%d, %d), want (%d, %d)",
					tt.pos, gotInt, gotSub, tt.wantInt, tt.wantSub)
			}
		})
	}
}

func TestQuantize_Subpixel10(t *testing.T) {
	tests := []struct {
		pos     float64
		wantInt int
		wantSub uint8
	}{
		{0.0, 0, 0},
		{0.1, 0, 1},
		{0.2, 0, 2},
		{0.5, 0, 5},
		{0.9, 0, 9},
		{0.99, 0, 9},
		{1.0, 1, 0},
		{5.35, 5, 3}, // 0.35 * 10 = 3.5 -> 3
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			gotInt, gotSub := Quantize(tt.pos, Subpixel10)
			if gotInt != tt.wantInt || gotSub != tt.wantSub {
				t.Errorf("Quantize(%v, Subpixel10) = (%d, %d), want (%d, %d)",
					tt.pos, gotInt, gotSub, tt.wantInt, tt.wantSub)
			}
		})
	}
}

func TestQuantize_SubpixelNone(t *testing.T) {
	tests := []struct {
		pos     float64
		wantInt int
	}{
		{0.0, 0},
		{0.3, 0}, // rounds to 0
		{0.5, 1}, // rounds to 1
		{0.7, 1}, // rounds to 1
		{10.3, 10},
		{10.7, 11},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			gotInt, gotSub := Quantize(tt.pos, SubpixelNone)
			if gotInt != tt.wantInt || gotSub != 0 {
				t.Errorf("Quantize(%v, SubpixelNone) = (%d, %d), want (%d, 0)",
					tt.pos, gotInt, gotSub, tt.wantInt)
			}
		})
	}
}

func TestQuantize_NegativePositions(t *testing.T) {
	// Negative positions should also work correctly.
	// The quantization ensures consistent behavior:
	// intPart is the floor, and frac is always in [0, 1)
	tests := []struct {
		pos     float64
		wantInt int
		wantSub uint8
	}{
		{-0.25, -1, 3}, // -0.25 -> floor=-1, frac=0.75 -> 0.75*4=3 -> sub=3
		{-0.5, -1, 2},  // -0.5 -> floor=-1, frac=0.5 -> 0.5*4=2 -> sub=2
		{-1.0, -1, 0},  // -1.0 -> floor=-1 (exact integer), frac=0 -> sub=0
		{-1.25, -2, 3}, // -1.25 -> floor=-2, frac=0.75 -> sub=3
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			gotInt, gotSub := Quantize(tt.pos, Subpixel4)
			if gotInt != tt.wantInt || gotSub != tt.wantSub {
				t.Errorf("Quantize(%v, Subpixel4) = (%d, %d), want (%d, %d)",
					tt.pos, gotInt, gotSub, tt.wantInt, tt.wantSub)
			}
		})
	}
}

func TestQuantizePoint(t *testing.T) {
	config := SubpixelConfig{Mode: Subpixel4, Horizontal: true, Vertical: true}

	intX, intY, subX, subY := QuantizePoint(10.25, 20.5, config)

	if intX != 10 || subX != 1 {
		t.Errorf("X: got (%d, %d), want (10, 1)", intX, subX)
	}
	if intY != 20 || subY != 2 {
		t.Errorf("Y: got (%d, %d), want (20, 2)", intY, subY)
	}
}

func TestQuantizePoint_HorizontalOnly(t *testing.T) {
	config := SubpixelConfig{Mode: Subpixel4, Horizontal: true, Vertical: false}

	intX, intY, subX, subY := QuantizePoint(10.25, 20.7, config)

	if intX != 10 || subX != 1 {
		t.Errorf("X: got (%d, %d), want (10, 1)", intX, subX)
	}
	// Y should be rounded to nearest integer, no subpixel
	if intY != 21 || subY != 0 {
		t.Errorf("Y: got (%d, %d), want (21, 0)", intY, subY)
	}
}

func TestSubpixelOffset(t *testing.T) {
	tests := []struct {
		subPos uint8
		mode   SubpixelMode
		want   float64
	}{
		{0, Subpixel4, 0.0},
		{1, Subpixel4, 0.25},
		{2, Subpixel4, 0.5},
		{3, Subpixel4, 0.75},
		{0, Subpixel10, 0.0},
		{1, Subpixel10, 0.1},
		{5, Subpixel10, 0.5},
		{9, Subpixel10, 0.9},
		{0, SubpixelNone, 0.0},
		{5, SubpixelNone, 0.0}, // No subpixel, always 0
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := SubpixelOffset(tt.subPos, tt.mode)
			if got != tt.want {
				t.Errorf("SubpixelOffset(%d, %v) = %f, want %f",
					tt.subPos, tt.mode, got, tt.want)
			}
		})
	}
}

func TestSubpixelOffsets(t *testing.T) {
	config := SubpixelConfig{Mode: Subpixel4, Horizontal: true, Vertical: true}

	offsetX, offsetY := SubpixelOffsets(1, 2, config)

	if offsetX != 0.25 {
		t.Errorf("offsetX = %f, want 0.25", offsetX)
	}
	if offsetY != 0.5 {
		t.Errorf("offsetY = %f, want 0.5", offsetY)
	}
}

func TestSubpixelOffsets_Disabled(t *testing.T) {
	config := SubpixelConfig{Mode: Subpixel4, Horizontal: false, Vertical: false}

	offsetX, offsetY := SubpixelOffsets(1, 2, config)

	if offsetX != 0 {
		t.Errorf("offsetX = %f, want 0", offsetX)
	}
	if offsetY != 0 {
		t.Errorf("offsetY = %f, want 0", offsetY)
	}
}

func TestNewSubpixelCache(t *testing.T) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())

	if cache == nil {
		t.Fatal("NewSubpixelCache should not return nil")
	}
	if cache.Len() != 0 {
		t.Errorf("New cache should be empty, got len=%d", cache.Len())
	}
	if !cache.Config().IsEnabled() {
		t.Error("Default config should be enabled")
	}
}

func TestNewSubpixelCacheWithConfig(t *testing.T) {
	subConfig := DefaultSubpixelConfig()
	glyphConfig := GlyphCacheConfig{MaxEntries: 1000, FrameLifetime: 32}

	cache := NewSubpixelCacheWithConfig(subConfig, glyphConfig)

	if cache == nil {
		t.Fatal("NewSubpixelCacheWithConfig should not return nil")
	}

	// Cache size should be multiplied by subpixel factor
	// For Subpixel4 horizontal only: 4x
	if cache.cache.config.MaxEntries != 4000 {
		t.Errorf("MaxEntries = %d, want 4000", cache.cache.config.MaxEntries)
	}
}

func TestSubpixelCache_SetGet(t *testing.T) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())

	key := SubpixelKey{
		OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: 42, Size: 16},
		SubX:            1,
		SubY:            0,
	}
	outline := &GlyphOutline{GID: 42, Advance: 10}

	// Get before set should return nil
	got := cache.Get(key)
	if got != nil {
		t.Error("Get before Set should return nil")
	}

	// Set and get
	cache.Set(key, outline)
	got = cache.Get(key)
	if got == nil {
		t.Fatal("Get after Set should not return nil")
	}
	if got.GID != 42 {
		t.Errorf("Got GID = %d, want 42", got.GID)
	}
}

func TestSubpixelCache_DifferentSubpixelPositions(t *testing.T) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())

	baseKey := OutlineCacheKey{FontID: 1, GID: 42, Size: 16}

	// Store same glyph at different subpixel positions
	for subX := uint8(0); subX < 4; subX++ {
		key := SubpixelKey{OutlineCacheKey: baseKey, SubX: subX, SubY: 0}
		cache.Set(key, &GlyphOutline{GID: 42, Advance: float32(subX)})
	}

	// All should be separate entries
	if cache.Len() != 4 {
		t.Errorf("Cache should have 4 entries, got %d", cache.Len())
	}

	// Retrieve and verify each
	for subX := uint8(0); subX < 4; subX++ {
		key := SubpixelKey{OutlineCacheKey: baseKey, SubX: subX, SubY: 0}
		got := cache.Get(key)
		if got == nil {
			t.Errorf("Entry for subX=%d should exist", subX)
			continue
		}
		if got.Advance != float32(subX) {
			t.Errorf("subX=%d: Advance=%f, want %d", subX, got.Advance, subX)
		}
	}
}

func TestSubpixelCache_GetOrCreate(t *testing.T) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())

	key := SubpixelKey{
		OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: 42, Size: 16},
		SubX:            1,
		SubY:            0,
	}

	createCalled := false
	var receivedOffsetX, receivedOffsetY float64

	// First call should invoke create with correct offsets
	outline := cache.GetOrCreate(key, func(offsetX, offsetY float64) *GlyphOutline {
		createCalled = true
		receivedOffsetX = offsetX
		receivedOffsetY = offsetY
		return &GlyphOutline{GID: 42}
	})

	if !createCalled {
		t.Error("Create should be called on first access")
	}
	if receivedOffsetX != 0.25 {
		t.Errorf("offsetX = %f, want 0.25", receivedOffsetX)
	}
	if receivedOffsetY != 0 {
		t.Errorf("offsetY = %f, want 0", receivedOffsetY)
	}
	if outline == nil {
		t.Error("Should return created outline")
	}

	// Second call should not invoke create
	createCalled = false
	outline = cache.GetOrCreate(key, func(offsetX, offsetY float64) *GlyphOutline {
		createCalled = true
		return &GlyphOutline{GID: 99}
	})

	if createCalled {
		t.Error("Create should not be called on second access")
	}
	if outline.GID != 42 {
		t.Errorf("Should return cached outline, got GID=%d", outline.GID)
	}
}

func TestSubpixelCache_GetOrCreate_NilCreate(t *testing.T) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())

	key := SubpixelKey{
		OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: 42, Size: 16},
		SubX:            0,
		SubY:            0,
	}

	outline := cache.GetOrCreate(key, nil)
	if outline != nil {
		t.Error("GetOrCreate with nil create should return nil")
	}
}

func TestSubpixelCache_Delete(t *testing.T) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())

	key := SubpixelKey{
		OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: 42, Size: 16},
		SubX:            1,
		SubY:            0,
	}

	cache.Set(key, &GlyphOutline{GID: 42})
	cache.Delete(key)

	if cache.Get(key) != nil {
		t.Error("Entry should be deleted")
	}
}

func TestSubpixelCache_Clear(t *testing.T) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())

	// Add entries
	for i := 0; i < 10; i++ {
		key := SubpixelKey{
			OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: GlyphID(i), Size: 16},
			SubX:            uint8(i % 4),
			SubY:            0,
		}
		cache.Set(key, &GlyphOutline{GID: GlyphID(i)})
	}

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("After Clear, len should be 0, got %d", cache.Len())
	}
}

func TestSubpixelCache_Maintain(t *testing.T) {
	subConfig := DefaultSubpixelConfig()
	glyphConfig := GlyphCacheConfig{MaxEntries: 100, FrameLifetime: 2}
	cache := NewSubpixelCacheWithConfig(subConfig, glyphConfig)

	key := SubpixelKey{
		OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: 42, Size: 16},
		SubX:            0,
		SubY:            0,
	}
	cache.Set(key, &GlyphOutline{GID: 42})

	// Advance past lifetime
	cache.Maintain()
	cache.Maintain()
	cache.Maintain()

	if cache.Len() != 0 {
		t.Errorf("Entry should be evicted after lifetime, got len=%d", cache.Len())
	}
}

func TestSubpixelCache_SetConfig(t *testing.T) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())

	// Add entry
	key := SubpixelKey{
		OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: 42, Size: 16},
		SubX:            1,
		SubY:            0,
	}
	cache.Set(key, &GlyphOutline{GID: 42})

	// Change config (should clear cache)
	cache.SetConfig(NoSubpixelConfig())

	if cache.Len() != 0 {
		t.Error("SetConfig should clear the cache")
	}
	if cache.Config().IsEnabled() {
		t.Error("Config should be updated")
	}
}

func TestSubpixelCache_Stats(t *testing.T) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())

	key := SubpixelKey{
		OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: 42, Size: 16},
		SubX:            0,
		SubY:            0,
	}

	// Miss
	_ = cache.Get(key)
	stats := cache.Stats()
	if stats.Misses != 1 {
		t.Errorf("Misses = %d, want 1", stats.Misses)
	}

	// Set and hit
	cache.Set(key, &GlyphOutline{GID: 42})
	_ = cache.Get(key)
	stats = cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("Hits = %d, want 1", stats.Hits)
	}
	if stats.SubpixelHits != 1 {
		t.Errorf("SubpixelHits = %d, want 1", stats.SubpixelHits)
	}
}

func TestSubpixelCache_HitRate(t *testing.T) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())

	// No accesses
	if rate := cache.HitRate(); rate != 0 {
		t.Errorf("HitRate with no accesses should be 0, got %f", rate)
	}

	key := SubpixelKey{
		OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: 42, Size: 16},
		SubX:            0,
		SubY:            0,
	}

	cache.Set(key, &GlyphOutline{GID: 42})
	_ = cache.Get(SubpixelKey{OutlineCacheKey: OutlineCacheKey{FontID: 999}}) // miss
	_ = cache.Get(key)                                                        // hit

	rate := cache.HitRate()
	if rate != 50.0 {
		t.Errorf("HitRate should be 50%%, got %f", rate)
	}
}

func TestSubpixelCache_ResetStats(t *testing.T) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())

	key := SubpixelKey{
		OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: 42, Size: 16},
		SubX:            0,
		SubY:            0,
	}
	cache.Set(key, &GlyphOutline{GID: 42})
	_ = cache.Get(key)

	cache.ResetStats()

	stats := cache.Stats()
	if stats.Hits != 0 || stats.Misses != 0 || stats.SubpixelHits != 0 || stats.SubpixelCreates != 0 {
		t.Error("All stats should be 0 after reset")
	}
}

func TestMakeSubpixelKey(t *testing.T) {
	baseKey := OutlineCacheKey{FontID: 1, GID: 42, Size: 16, Hinting: HintingNone}
	config := SubpixelConfig{Mode: Subpixel4, Horizontal: true, Vertical: false}

	key := MakeSubpixelKey(baseKey, 10.25, 20.7, config)

	if key.FontID != 1 || key.GID != 42 || key.Size != 16 {
		t.Error("Base key fields should be preserved")
	}
	if key.SubX != 1 {
		t.Errorf("SubX = %d, want 1", key.SubX)
	}
	if key.SubY != 0 {
		t.Errorf("SubY = %d, want 0 (vertical disabled)", key.SubY)
	}
}

func TestGlobalSubpixelCache(t *testing.T) {
	cache := GetGlobalSubpixelCache()
	if cache == nil {
		t.Fatal("GetGlobalSubpixelCache should not return nil")
	}

	// Test setting a new global cache
	newCache := NewSubpixelCache(HighQualitySubpixelConfig())
	old := SetGlobalSubpixelCache(newCache)
	if old == nil {
		t.Error("SetGlobalSubpixelCache should return old cache")
	}

	current := GetGlobalSubpixelCache()
	if current != newCache {
		t.Error("GetGlobalSubpixelCache should return new cache")
	}

	// Restore old cache
	_ = SetGlobalSubpixelCache(old)
}

func TestSetGlobalSubpixelCache_Nil(t *testing.T) {
	old := GetGlobalSubpixelCache()
	_ = SetGlobalSubpixelCache(nil)
	defer func() {
		_ = SetGlobalSubpixelCache(old)
	}()

	current := GetGlobalSubpixelCache()
	if current == nil {
		t.Error("SetGlobalSubpixelCache(nil) should create a default cache")
	}
}

func TestSubpixelCache_Concurrent(t *testing.T) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())
	var wg sync.WaitGroup

	// Concurrent writes with different subpixel positions
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := SubpixelKey{
				OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: GlyphID(i / 4), Size: 16},
				SubX:            uint8(i % 4),
				SubY:            0,
			}
			cache.Set(key, &GlyphOutline{GID: GlyphID(i / 4)})
		}(i)
	}
	wg.Wait()

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := SubpixelKey{
				OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: GlyphID(i / 4), Size: 16},
				SubX:            uint8(i % 4),
				SubY:            0,
			}
			_ = cache.Get(key)
		}(i)
	}
	wg.Wait()

	// Concurrent GetOrCreate
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := SubpixelKey{
				OutlineCacheKey: OutlineCacheKey{FontID: 2, GID: GlyphID(i / 4), Size: 16},
				SubX:            uint8(i % 4),
				SubY:            0,
			}
			_ = cache.GetOrCreate(key, func(offsetX, offsetY float64) *GlyphOutline {
				return &GlyphOutline{GID: GlyphID(i / 4)}
			})
		}(i)
	}
	wg.Wait()
}

// Benchmarks

func BenchmarkQuantize_Subpixel4(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = Quantize(10.3, Subpixel4)
	}
}

func BenchmarkQuantize_Subpixel10(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = Quantize(10.3, Subpixel10)
	}
}

func BenchmarkQuantize_None(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = Quantize(10.3, SubpixelNone)
	}
}

func BenchmarkQuantizePoint(b *testing.B) {
	config := DefaultSubpixelConfig()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = QuantizePoint(10.3, 20.7, config)
	}
}

func BenchmarkSubpixelCache_Get_Hit(b *testing.B) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())
	key := SubpixelKey{
		OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: 42, Size: 16},
		SubX:            1,
		SubY:            0,
	}
	cache.Set(key, &GlyphOutline{GID: 42})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Get(key)
	}
}

func BenchmarkSubpixelCache_Get_Miss(b *testing.B) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())
	key := SubpixelKey{
		OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: 42, Size: 16},
		SubX:            1,
		SubY:            0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Get(key)
	}
}

func BenchmarkSubpixelCache_Set(b *testing.B) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())
	outline := &GlyphOutline{GID: 42}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := SubpixelKey{
			OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: GlyphID(i % 1000), Size: 16},
			SubX:            uint8(i % 4),
			SubY:            0,
		}
		cache.Set(key, outline)
	}
}

func BenchmarkSubpixelCache_GetOrCreate_Hit(b *testing.B) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())
	key := SubpixelKey{
		OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: 42, Size: 16},
		SubX:            1,
		SubY:            0,
	}
	cache.Set(key, &GlyphOutline{GID: 42})

	create := func(offsetX, offsetY float64) *GlyphOutline {
		return &GlyphOutline{GID: 42}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.GetOrCreate(key, create)
	}
}

func BenchmarkSubpixelCache_Concurrent_Get(b *testing.B) {
	cache := NewSubpixelCache(DefaultSubpixelConfig())

	// Prepopulate with different subpixel positions
	for i := 0; i < 1000; i++ {
		key := SubpixelKey{
			OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: GlyphID(i / 4), Size: 16},
			SubX:            uint8(i % 4),
			SubY:            0,
		}
		cache.Set(key, &GlyphOutline{GID: GlyphID(i / 4)})
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := SubpixelKey{
				OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: GlyphID((i / 4) % 250), Size: 16},
				SubX:            uint8(i % 4),
				SubY:            0,
			}
			_ = cache.Get(key)
			i++
		}
	})
}

func BenchmarkSubpixelCache_vs_GlyphCache(b *testing.B) {
	// Compare performance of subpixel cache vs regular glyph cache

	b.Run("GlyphCache", func(b *testing.B) {
		cache := NewGlyphCache()
		key := OutlineCacheKey{FontID: 1, GID: 42, Size: 16}
		cache.Set(key, &GlyphOutline{GID: 42})

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = cache.Get(key)
		}
	})

	b.Run("SubpixelCache", func(b *testing.B) {
		cache := NewSubpixelCache(DefaultSubpixelConfig())
		key := SubpixelKey{
			OutlineCacheKey: OutlineCacheKey{FontID: 1, GID: 42, Size: 16},
			SubX:            1,
			SubY:            0,
		}
		cache.Set(key, &GlyphOutline{GID: 42})

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = cache.Get(key)
		}
	})
}
