package text

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"math"
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func TestGlyphMaskCacheBudgetAndLRU(t *testing.T) {
	var c glyphMaskCache
	mask := &GlyphMaskResult{Mask: make([]byte, glyphMaskMaxBytes)}
	create := func() (*GlyphMaskResult, error) { return mask, nil }
	count := glyphMaskCacheBytes / glyphMaskMaxBytes
	for i := 0; i < count; i++ {
		_, _ = c.get(glyphMaskKey{gid: GlyphID(i)}, create)
	}
	_, _ = c.get(glyphMaskKey{}, create) // Keep oldest entry hot.
	_, _ = c.get(glyphMaskKey{gid: GlyphID(count)}, create)
	if c.bytes > glyphMaskCacheBytes || len(c.entries) != count {
		t.Fatalf("budget exceeded: %d bytes, %d entries", c.bytes, len(c.entries))
	}
	if _, ok := c.entries[glyphMaskKey{}]; !ok {
		t.Fatal("recently used entry was evicted")
	}
	if _, ok := c.entries[glyphMaskKey{gid: 1}]; ok {
		t.Fatal("least recently used entry was retained")
	}
	large := &GlyphMaskResult{Mask: make([]byte, glyphMaskMaxBytes+1)}
	got, err := c.get(glyphMaskKey{gid: 999}, func() (*GlyphMaskResult, error) { return large, nil })
	if err != nil || got != large || len(c.entries) != count || c.bytes != glyphMaskCacheBytes {
		t.Fatal("oversized mask must render without entering or evicting the cache")
	}
}

//nolint:nilnil // A glyph without an outline legitimately rasterizes to nil, nil.
func TestGlyphMaskCacheNilAndError(t *testing.T) {
	var c glyphMaskCache
	calls := 0
	empty := func() (*GlyphMaskResult, error) { calls++; return nil, nil }
	for i := 0; i < glyphMaskCacheLimit+1; i++ {
		_, _ = c.get(glyphMaskKey{gid: GlyphID(i)}, empty)
	}
	if len(c.entries) != glyphMaskCacheLimit || c.bytes != 0 {
		t.Fatal("empty outlines must still respect the entry limit")
	}
	key := glyphMaskKey{gid: GlyphID(glyphMaskCacheLimit)}
	before := calls
	_, _ = c.get(key, empty)
	if calls != before {
		t.Fatal("nil outline was not cached")
	}
	wantErr := errors.New("rasterization failed")
	for i := 0; i < 2; i++ {
		_, err := c.get(glyphMaskKey{gid: 9999}, func() (*GlyphMaskResult, error) { calls++; return nil, wantErr })
		if !errors.Is(err, wantErr) {
			t.Fatal("rasterization error was lost")
		}
	}
	if calls != before+2 {
		t.Fatal("errors were cached")
	}
}

//nolint:nilnil // Exercise a non-rendering glyph with an invalid cache key.
func TestGlyphMaskCacheNaNBypass(t *testing.T) {
	var c glyphMaskCache
	for _, key := range []glyphMaskKey{{ppem: math.NaN()}, {subpixelX: math.NaN()}, {subpixelY: math.NaN()}} {
		for i := 0; i <= glyphMaskCacheLimit; i++ {
			_, _ = c.get(key, func() (*GlyphMaskResult, error) { return nil, nil })
		}
	}
	if len(c.entries) != 0 {
		t.Fatal("NaN keys cannot be retained because map eviction cannot delete them")
	}
}

func TestGlyphMaskCacheConcurrentMissAndClose(t *testing.T) {
	for _, closeDuringMiss := range []bool{false, true} {
		var c glyphMaskCache
		var started, done sync.WaitGroup
		started.Add(8)
		done.Add(8)
		release := make(chan struct{})
		for i := 0; i < 8; i++ {
			go func() {
				defer done.Done()
				_, _ = c.get(glyphMaskKey{}, func() (*GlyphMaskResult, error) {
					started.Done()
					<-release
					return &GlyphMaskResult{Mask: make([]byte, 16)}, nil
				})
			}()
		}
		started.Wait()
		if closeDuringMiss {
			c.close()
		}
		close(release)
		done.Wait()
		wantEntries, wantBytes := 1, 16
		if closeDuringMiss {
			wantEntries, wantBytes = 0, 0
		}
		if len(c.entries) != wantEntries || c.bytes != wantBytes {
			t.Fatalf("close=%v: %d entries, %d bytes", closeDuringMiss, len(c.entries), c.bytes)
		}
	}
}

func TestDrawCacheMatchesUncachedAndCloses(t *testing.T) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	font := source.Parsed().(*ownParsedFont)
	face := source.Face(14)
	uncachedSource, err := NewFontSource(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = uncachedSource.Close() })
	uncachedSource.Parsed().(*ownParsedFont).glyphMasks.close()
	uncachedFace := uncachedSource.Face(14)
	for _, mode := range []glyphRasterMode{rasterModeAA, rasterModeAliased} {
		for _, x := range []float64{10, 10.125, 10.625} {
			cold := image.NewRGBA(image.Rect(0, 0, 160, 40))
			warm := image.NewRGBA(cold.Bounds())
			if mode == rasterModeAA {
				Draw(cold, "Cache 123", uncachedFace, x, 25.25, color.Black)
				Draw(warm, "Cache 123", face, x, 25.25, color.Black)
				clear(warm.Pix)
				Draw(warm, "Cache 123", face, x, 25.25, color.Black)
			} else {
				DrawAliased(cold, "Cache 123", uncachedFace, x, 25.25, color.Black)
				DrawAliased(warm, "Cache 123", face, x, 25.25, color.Black)
				clear(warm.Pix)
				DrawAliased(warm, "Cache 123", face, x, 25.25, color.Black)
			}
			if !bytes.Equal(cold.Pix, warm.Pix) {
				t.Fatalf("cache hit changed pixels: mode=%v, x=%v", mode, x)
			}
		}
	}
	if len(font.glyphMasks.entries) == 0 || font.glyphMasks.bytes == 0 {
		t.Fatal("drawing did not populate the font's cache")
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if len(font.glyphMasks.entries) != 0 || font.glyphMasks.bytes != 0 || !font.glyphMasks.closed {
		t.Fatal("Close retained glyph masks")
	}
}

func BenchmarkDrawCachedLabels(b *testing.B) {
	source, err := NewFontSource(goregular.TTF)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = source.Close() })
	face := source.Face(14)
	dst := image.NewRGBA(image.Rect(0, 0, 600, 40))
	b.ReportAllocs()
	for b.Loop() {
		Draw(dst, "2026-09-22 12:34:56 Temperature 123.45", face, 10, 25, color.Black)
	}
}
