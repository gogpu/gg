package text

import (
	"math"
	"sync"
	"testing"
)

func TestNewGlyphRunBuilder(t *testing.T) {
	t.Run("with cache", func(t *testing.T) {
		cache := NewGlyphCache()
		builder := NewGlyphRunBuilder(cache)
		if builder == nil {
			t.Fatal("NewGlyphRunBuilder should not return nil")
		}
		if builder.Cache() != cache {
			t.Error("Builder should use the provided cache")
		}
		if builder.Len() != 0 {
			t.Errorf("New builder should be empty, got len=%d", builder.Len())
		}
	})

	t.Run("with nil cache", func(t *testing.T) {
		builder := NewGlyphRunBuilder(nil)
		if builder == nil {
			t.Fatal("NewGlyphRunBuilder should not return nil")
		}
		if builder.Cache() != GetGlobalGlyphCache() {
			t.Error("Builder should use global cache when nil is provided")
		}
	})
}

func TestGlyphRunBuilder_AddGlyph(t *testing.T) {
	builder := NewGlyphRunBuilder(NewGlyphCache())

	builder.AddGlyph(1, 42, Point{X: 10, Y: 20}, 16.0)
	if builder.Len() != 1 {
		t.Errorf("After AddGlyph, len should be 1, got %d", builder.Len())
	}

	builder.AddGlyph(1, 43, Point{X: 30, Y: 20}, 16.0)
	if builder.Len() != 2 {
		t.Errorf("After second AddGlyph, len should be 2, got %d", builder.Len())
	}

	instances := builder.Instances()
	if len(instances) != 2 {
		t.Fatalf("Instances should return 2 items, got %d", len(instances))
	}

	// Verify first instance
	if instances[0].FontID != 1 {
		t.Errorf("instances[0].FontID = %d, want 1", instances[0].FontID)
	}
	if instances[0].GlyphID != 42 {
		t.Errorf("instances[0].GlyphID = %d, want 42", instances[0].GlyphID)
	}
	if instances[0].Position.X != 10 || instances[0].Position.Y != 20 {
		t.Errorf("instances[0].Position = %v, want {10, 20}", instances[0].Position)
	}
	if instances[0].Size != 16.0 {
		t.Errorf("instances[0].Size = %f, want 16.0", instances[0].Size)
	}

	// Verify second instance
	if instances[1].GlyphID != 43 {
		t.Errorf("instances[1].GlyphID = %d, want 43", instances[1].GlyphID)
	}
}

func TestGlyphRunBuilder_AddShapedGlyph(t *testing.T) {
	builder := NewGlyphRunBuilder(NewGlyphCache())

	glyph := &ShapedGlyph{
		GID: 42,
		X:   10,
		Y:   20,
	}

	builder.AddShapedGlyph(1, glyph, 16.0)
	if builder.Len() != 1 {
		t.Errorf("After AddShapedGlyph, len should be 1, got %d", builder.Len())
	}

	instances := builder.Instances()
	if instances[0].GlyphID != 42 {
		t.Errorf("GlyphID = %d, want 42", instances[0].GlyphID)
	}
	if instances[0].Position.X != 10 || instances[0].Position.Y != 20 {
		t.Errorf("Position = %v, want {10, 20}", instances[0].Position)
	}
}

func TestGlyphRunBuilder_AddShapedGlyph_Nil(t *testing.T) {
	builder := NewGlyphRunBuilder(NewGlyphCache())

	builder.AddShapedGlyph(1, nil, 16.0)
	if builder.Len() != 0 {
		t.Errorf("Adding nil glyph should not increase len, got %d", builder.Len())
	}
}

func TestGlyphRunBuilder_AddShapedGlyphs(t *testing.T) {
	builder := NewGlyphRunBuilder(NewGlyphCache())

	glyphs := []ShapedGlyph{
		{GID: 42, X: 0, Y: 0},
		{GID: 43, X: 10, Y: 0},
		{GID: 44, X: 20, Y: 0},
	}

	origin := Point{X: 100, Y: 200}
	builder.AddShapedGlyphs(1, glyphs, origin, 16.0)

	if builder.Len() != 3 {
		t.Errorf("After AddShapedGlyphs, len should be 3, got %d", builder.Len())
	}

	instances := builder.Instances()

	// Check that origin offset is applied
	if instances[0].Position.X != 100 || instances[0].Position.Y != 200 {
		t.Errorf("instances[0].Position = %v, want {100, 200}", instances[0].Position)
	}
	if instances[1].Position.X != 110 || instances[1].Position.Y != 200 {
		t.Errorf("instances[1].Position = %v, want {110, 200}", instances[1].Position)
	}
	if instances[2].Position.X != 120 || instances[2].Position.Y != 200 {
		t.Errorf("instances[2].Position = %v, want {120, 200}", instances[2].Position)
	}
}

func TestGlyphRunBuilder_Clear(t *testing.T) {
	builder := NewGlyphRunBuilder(NewGlyphCache())

	// Add some glyphs
	for i := 0; i < 10; i++ {
		builder.AddGlyph(1, GlyphID(i), Point{X: float32(i * 10), Y: 0}, 16.0)
	}

	if builder.Len() != 10 {
		t.Errorf("Before Clear, len should be 10, got %d", builder.Len())
	}

	builder.Clear()

	if builder.Len() != 0 {
		t.Errorf("After Clear, len should be 0, got %d", builder.Len())
	}

	// Verify we can add more after clearing
	builder.AddGlyph(1, 42, Point{X: 0, Y: 0}, 16.0)
	if builder.Len() != 1 {
		t.Errorf("After re-adding, len should be 1, got %d", builder.Len())
	}
}

func TestGlyphRunBuilder_Build_Empty(t *testing.T) {
	builder := NewGlyphRunBuilder(NewGlyphCache())

	commands := builder.Build(nil)
	if commands != nil {
		t.Errorf("Build on empty builder should return nil, got %v", commands)
	}
}

func TestGlyphRunBuilder_Build_WithCreate(t *testing.T) {
	cache := NewGlyphCache()
	builder := NewGlyphRunBuilder(cache)

	builder.AddGlyph(1, 42, Point{X: 10, Y: 20}, 16.0)
	builder.AddGlyph(1, 43, Point{X: 30, Y: 20}, 16.0)

	createCalled := 0
	createGlyph := func(fontID uint64, glyphID GlyphID, size float32) *GlyphOutline {
		createCalled++
		return &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
				{Op: OutlineOpLineTo, Points: [3]OutlinePoint{{X: 10, Y: 0}}},
			},
			GID:     glyphID,
			Advance: 10,
		}
	}

	commands := builder.Build(createGlyph)

	if createCalled != 2 {
		t.Errorf("createGlyph should be called 2 times, got %d", createCalled)
	}
	if len(commands) != 2 {
		t.Fatalf("Should have 2 commands, got %d", len(commands))
	}

	// Verify first command
	if commands[0].Outline == nil {
		t.Error("commands[0].Outline should not be nil")
	}
	if commands[0].Transform == nil {
		t.Error("commands[0].Transform should not be nil")
	}
	if commands[0].Instance.GlyphID != 42 {
		t.Errorf("commands[0].Instance.GlyphID = %d, want 42", commands[0].Instance.GlyphID)
	}

	// Verify transform includes position
	if commands[0].Transform.Tx != 10 || commands[0].Transform.Ty != 20 {
		t.Errorf("Transform position = (%f, %f), want (10, 20)",
			commands[0].Transform.Tx, commands[0].Transform.Ty)
	}

	// Verify Y-flip
	if commands[0].Transform.D != -1 {
		t.Errorf("Transform.D = %f, want -1 (Y-flip)", commands[0].Transform.D)
	}
}

func TestGlyphRunBuilder_Build_CacheHit(t *testing.T) {
	cache := NewGlyphCache()
	builder := NewGlyphRunBuilder(cache)

	// Pre-populate cache
	key := OutlineCacheKey{FontID: 1, GID: 42, Size: 16, Hinting: HintingNone}
	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
		},
		GID: 42,
	}
	cache.Set(key, outline)

	builder.AddGlyph(1, 42, Point{X: 0, Y: 0}, 16.0)

	createCalled := false
	commands := builder.Build(func(fontID uint64, glyphID GlyphID, size float32) *GlyphOutline {
		createCalled = true
		return outline
	})

	// createGlyph should NOT be called because we have a cache hit
	if createCalled {
		t.Error("createGlyph should not be called for cache hits")
	}
	if len(commands) != 1 {
		t.Fatalf("Should have 1 command, got %d", len(commands))
	}
}

func TestGlyphRunBuilder_Build_NilCreate(t *testing.T) {
	cache := NewGlyphCache()
	builder := NewGlyphRunBuilder(cache)

	// Pre-populate cache with one outline
	key := OutlineCacheKey{FontID: 1, GID: 42, Size: 16, Hinting: HintingNone}
	outline := &GlyphOutline{
		Segments: []OutlineSegment{
			{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
		},
		GID: 42,
	}
	cache.Set(key, outline)

	builder.AddGlyph(1, 42, Point{X: 0, Y: 0}, 16.0)  // cache hit
	builder.AddGlyph(1, 43, Point{X: 10, Y: 0}, 16.0) // cache miss

	commands := builder.Build(nil)

	// Should only get 1 command (cache hit), miss is skipped
	if len(commands) != 1 {
		t.Errorf("With nil create, should only get cache hits, got %d commands", len(commands))
	}
}

func TestGlyphRunBuilder_Build_SkipEmptyOutlines(t *testing.T) {
	cache := NewGlyphCache()
	builder := NewGlyphRunBuilder(cache)

	builder.AddGlyph(1, 42, Point{X: 0, Y: 0}, 16.0)
	builder.AddGlyph(1, 43, Point{X: 10, Y: 0}, 16.0) // space character

	createGlyph := func(fontID uint64, glyphID GlyphID, size float32) *GlyphOutline {
		if glyphID == 43 {
			// Return empty outline (like a space character)
			return &GlyphOutline{GID: 43}
		}
		return &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
			},
			GID: glyphID,
		}
	}

	commands := builder.Build(createGlyph)

	// Should only have 1 command (empty outline is skipped)
	if len(commands) != 1 {
		t.Errorf("Should skip empty outlines, got %d commands", len(commands))
	}
	if commands[0].Instance.GlyphID != 42 {
		t.Errorf("Command should be for glyph 42, got %d", commands[0].Instance.GlyphID)
	}
}

func TestGlyphRunBuilder_BuildTransformed(t *testing.T) {
	cache := NewGlyphCache()
	builder := NewGlyphRunBuilder(cache)

	builder.AddGlyph(1, 42, Point{X: 10, Y: 20}, 16.0)

	createGlyph := func(fontID uint64, glyphID GlyphID, size float32) *GlyphOutline {
		return &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
			},
			GID: glyphID,
		}
	}

	// Apply a user transform (scale by 2)
	userTransform := ScaleTransform(2, 2)
	commands := builder.BuildTransformed(createGlyph, userTransform)

	if len(commands) != 1 {
		t.Fatalf("Should have 1 command, got %d", len(commands))
	}

	// The final transform should be userTransform * glyphTransform
	// userTransform = [2, 0, 0, 2, 0, 0]
	// glyphTransform = [1, 0, 0, -1, 10, 20]
	// Result: [2, 0, 0, -2, 20, 40]
	transform := commands[0].Transform
	if transform.A != 2 {
		t.Errorf("Transform.A = %f, want 2", transform.A)
	}
	if transform.D != -2 {
		t.Errorf("Transform.D = %f, want -2", transform.D)
	}
	if transform.Tx != 20 {
		t.Errorf("Transform.Tx = %f, want 20", transform.Tx)
	}
	if transform.Ty != 40 {
		t.Errorf("Transform.Ty = %f, want 40", transform.Ty)
	}
}

func TestGlyphRunBuilder_BuildTransformed_NilUserTransform(t *testing.T) {
	cache := NewGlyphCache()
	builder := NewGlyphRunBuilder(cache)

	builder.AddGlyph(1, 42, Point{X: 10, Y: 20}, 16.0)

	createGlyph := func(fontID uint64, glyphID GlyphID, size float32) *GlyphOutline {
		return &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
			},
			GID: glyphID,
		}
	}

	commands := builder.BuildTransformed(createGlyph, nil)

	if len(commands) != 1 {
		t.Fatalf("Should have 1 command, got %d", len(commands))
	}

	// With nil user transform, should just have glyph transform
	transform := commands[0].Transform
	if transform.Tx != 10 || transform.Ty != 20 {
		t.Errorf("Transform position = (%f, %f), want (10, 20)", transform.Tx, transform.Ty)
	}
}

func TestGlyphRunBuilder_SetCache(t *testing.T) {
	builder := NewGlyphRunBuilder(nil)
	originalCache := builder.Cache()

	newCache := NewGlyphCache()
	builder.SetCache(newCache)

	if builder.Cache() != newCache {
		t.Error("SetCache should update the cache")
	}
	if builder.Cache() == originalCache {
		t.Error("Cache should be different from original")
	}

	// Setting nil should use global cache
	builder.SetCache(nil)
	if builder.Cache() != GetGlobalGlyphCache() {
		t.Error("SetCache(nil) should use global cache")
	}
}

func TestGlyphRunBuilder_Instances_Copy(t *testing.T) {
	builder := NewGlyphRunBuilder(NewGlyphCache())

	builder.AddGlyph(1, 42, Point{X: 10, Y: 20}, 16.0)

	instances1 := builder.Instances()
	instances2 := builder.Instances()

	// Should be different slices
	instances1[0].GlyphID = 99
	if instances2[0].GlyphID == 99 {
		t.Error("Instances() should return a copy, not the original")
	}
}

func TestGlyphRunBuilder_Instances_Empty(t *testing.T) {
	builder := NewGlyphRunBuilder(NewGlyphCache())

	instances := builder.Instances()
	if instances != nil {
		t.Errorf("Instances() on empty builder should return nil, got %v", instances)
	}
}

func TestGlyphRunBuilderPool(t *testing.T) {
	cache := NewGlyphCache()
	pool := NewGlyphRunBuilderPool(cache)

	builder := pool.Get()
	if builder == nil {
		t.Fatal("Get should not return nil")
	}
	if builder.Cache() != cache {
		t.Error("Pooled builder should use the pool's cache")
	}

	// Add some glyphs
	builder.AddGlyph(1, 42, Point{X: 0, Y: 0}, 16.0)
	builder.AddGlyph(1, 43, Point{X: 10, Y: 0}, 16.0)

	if builder.Len() != 2 {
		t.Errorf("Builder should have 2 instances, got %d", builder.Len())
	}

	// Return to pool
	pool.Put(builder)

	// Get again - should be cleared
	builder2 := pool.Get()
	if builder2.Len() != 0 {
		t.Errorf("Pooled builder should be cleared, got len=%d", builder2.Len())
	}

	// Put nil should not panic
	pool.Put(nil)
}

func TestGlyphRunBuilderPool_NilCache(t *testing.T) {
	pool := NewGlyphRunBuilderPool(nil)

	builder := pool.Get()
	if builder.Cache() != GetGlobalGlyphCache() {
		t.Error("Pool with nil cache should use global cache")
	}
}

func TestSizeBitsConversion(t *testing.T) {
	sizes := []float32{1.0, 12.0, 16.5, 24.0, 48.0, 72.0, 100.0}

	for _, size := range sizes {
		bits := float32ToSizeBits(size)
		recovered := sizeBitsToFloat32(bits)

		if recovered != size {
			t.Errorf("Size conversion roundtrip failed: %f -> %d -> %f", size, bits, recovered)
		}
	}
}

func TestSizeBitsUniqueness(t *testing.T) {
	// Verify that different float32 sizes produce different bits
	size1 := float32(16.0)
	size2 := float32(16.001)

	bits1 := float32ToSizeBits(size1)
	bits2 := float32ToSizeBits(size2)

	if bits1 == bits2 {
		t.Errorf("Different sizes should produce different bits: %f and %f both -> %d", size1, size2, bits1)
	}
}

func TestPoint(t *testing.T) {
	p := Point{X: 10.5, Y: 20.5}

	if p.X != 10.5 {
		t.Errorf("Point.X = %f, want 10.5", p.X)
	}
	if p.Y != 20.5 {
		t.Errorf("Point.Y = %f, want 20.5", p.Y)
	}
}

func TestGlyphInstance(t *testing.T) {
	inst := GlyphInstance{
		FontID:   12345,
		GlyphID:  42,
		Position: Point{X: 100, Y: 200},
		Size:     16.0,
	}

	if inst.FontID != 12345 {
		t.Errorf("FontID = %d, want 12345", inst.FontID)
	}
	if inst.GlyphID != 42 {
		t.Errorf("GlyphID = %d, want 42", inst.GlyphID)
	}
	if inst.Position.X != 100 || inst.Position.Y != 200 {
		t.Errorf("Position = %v, want {100, 200}", inst.Position)
	}
	if inst.Size != 16.0 {
		t.Errorf("Size = %f, want 16.0", inst.Size)
	}
}

func TestDrawCommand(t *testing.T) {
	outline := &GlyphOutline{GID: 42}
	transform := IdentityTransform()
	instance := GlyphInstance{FontID: 1, GlyphID: 42}

	cmd := DrawCommand{
		Outline:   outline,
		Transform: transform,
		Instance:  instance,
	}

	if cmd.Outline != outline {
		t.Error("DrawCommand.Outline mismatch")
	}
	if cmd.Transform != transform {
		t.Error("DrawCommand.Transform mismatch")
	}
	if cmd.Instance.GlyphID != 42 {
		t.Errorf("DrawCommand.Instance.GlyphID = %d, want 42", cmd.Instance.GlyphID)
	}
}

// Test concurrent usage of pool
func TestGlyphRunBuilderPool_Concurrent(t *testing.T) {
	pool := NewGlyphRunBuilderPool(NewGlyphCache())
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			builder := pool.Get()
			defer pool.Put(builder)

			// Add some glyphs
			for j := 0; j < 10; j++ {
				builder.AddGlyph(uint64(i), GlyphID(j), Point{X: float32(j * 10), Y: 0}, 16.0)
			}

			// Build (without create function, just cache lookups)
			_ = builder.Build(nil)
		}(i)
	}

	wg.Wait()
}

// Test that builder capacity grows efficiently
func TestGlyphRunBuilder_Growth(t *testing.T) {
	builder := NewGlyphRunBuilder(NewGlyphCache())

	// Add many glyphs
	for i := 0; i < 1000; i++ {
		builder.AddGlyph(1, GlyphID(i), Point{X: float32(i), Y: 0}, 16.0)
	}

	if builder.Len() != 1000 {
		t.Errorf("After adding 1000 glyphs, len = %d", builder.Len())
	}

	builder.Clear()
	if builder.Len() != 0 {
		t.Errorf("After Clear, len should be 0, got %d", builder.Len())
	}
}

// Benchmarks

func BenchmarkGlyphRunBuilder_AddGlyph(b *testing.B) {
	builder := NewGlyphRunBuilder(NewGlyphCache())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.AddGlyph(1, GlyphID(i%1000), Point{X: float32(i % 100), Y: 0}, 16.0)
		if builder.Len() > 10000 {
			builder.Clear()
		}
	}
}

func BenchmarkGlyphRunBuilder_Build_CacheHit(b *testing.B) {
	cache := NewGlyphCache()
	builder := NewGlyphRunBuilder(cache)

	// Pre-populate cache
	for i := 0; i < 100; i++ {
		key := OutlineCacheKey{FontID: 1, GID: GlyphID(i), Size: 16, Hinting: HintingNone}
		outline := &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
			},
			GID: GlyphID(i),
		}
		cache.Set(key, outline)
	}

	// Add glyphs
	for i := 0; i < 100; i++ {
		builder.AddGlyph(1, GlyphID(i), Point{X: float32(i * 10), Y: 0}, 16.0)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = builder.Build(nil)
	}
}

func BenchmarkGlyphRunBuilder_Build_CacheMiss(b *testing.B) {
	cache := NewGlyphCache()
	builder := NewGlyphRunBuilder(cache)

	createGlyph := func(fontID uint64, glyphID GlyphID, size float32) *GlyphOutline {
		return &GlyphOutline{
			Segments: []OutlineSegment{
				{Op: OutlineOpMoveTo, Points: [3]OutlinePoint{{X: 0, Y: 0}}},
			},
			GID: glyphID,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Clear()
		for j := 0; j < 100; j++ {
			builder.AddGlyph(1, GlyphID((i*100+j)%10000), Point{X: float32(j * 10), Y: 0}, 16.0)
		}
		_ = builder.Build(createGlyph)
	}
}

func BenchmarkGlyphRunBuilderPool_GetPut(b *testing.B) {
	pool := NewGlyphRunBuilderPool(NewGlyphCache())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder := pool.Get()
		builder.AddGlyph(1, 42, Point{X: 0, Y: 0}, 16.0)
		pool.Put(builder)
	}
}

func BenchmarkGlyphRunBuilderPool_Concurrent(b *testing.B) {
	pool := NewGlyphRunBuilderPool(NewGlyphCache())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			builder := pool.Get()
			for j := 0; j < 10; j++ {
				builder.AddGlyph(1, GlyphID(j), Point{X: float32(j * 10), Y: 0}, 16.0)
			}
			_ = builder.Build(nil)
			pool.Put(builder)
		}
	})
}

func BenchmarkFloat32ToSizeBits(b *testing.B) {
	sizes := []float32{12.0, 14.0, 16.0, 18.0, 24.0, 36.0, 48.0, 72.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		size := sizes[i%len(sizes)]
		_ = math.Float32bits(size)
	}
}
