//go:build !nogpu

package gpu

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// defaultImageCacheBudget is the maximum number of cached image textures.
// LRU eviction removes the least recently used entry when exceeded.
const defaultImageCacheBudget = 64

// imageCacheEntry holds a GPU texture and view for a cached image.
type imageCacheEntry struct {
	texture *wgpu.Texture
	view    *wgpu.TextureView
	width   int
	height  int
	gen     uint64 // generation counter for LRU tracking
}

// ImageCache manages GPU textures for image patterns. Images are uploaded
// on first use and reused on subsequent frames. The cache is keyed by the
// pixel data slice pointer (identity-based: same underlying array = same entry).
//
// LRU eviction removes the oldest entry when the cache exceeds its budget.
// The cache is NOT thread-safe — it is accessed only from the render path
// which is serialized per GPURenderContext.
//
// This follows the enterprise pattern:
//   - Skia: GrTextureProxy cache (keyed by UniqueID)
//   - Vello: image atlas (uploaded once, referenced by scene commands)
//   - Qt Quick: QSGTexture cache (per-window, LRU evicted)
type ImageCache struct {
	device *wgpu.Device
	queue  *wgpu.Queue

	entries map[uintptr]*imageCacheEntry // keyed by pixel data pointer
	budget  int
	gen     uint64 // global generation counter
}

// NewImageCache creates a new image texture cache with the given device and queue.
func NewImageCache(device *wgpu.Device, queue *wgpu.Queue) *ImageCache {
	return &ImageCache{
		device:  device,
		queue:   queue,
		entries: make(map[uintptr]*imageCacheEntry),
		budget:  defaultImageCacheBudget,
	}
}

// GetOrUpload returns the cached GPU texture view for the given image data,
// uploading it if not already cached. The pixel data must be premultiplied RGBA.
// Returns the texture view for use in bind groups.
func (c *ImageCache) GetOrUpload(cmd *ImageDrawCommand) (*wgpu.TextureView, error) {
	if len(cmd.PixelData) == 0 {
		return nil, fmt.Errorf("empty pixel data")
	}

	key := pixelDataKey(cmd.PixelData)
	if entry, ok := c.entries[key]; ok {
		c.gen++
		entry.gen = c.gen
		return entry.view, nil
	}

	// Evict if over budget.
	if len(c.entries) >= c.budget {
		c.evictOldest()
	}

	// Upload new texture.
	entry, err := c.uploadImage(cmd)
	if err != nil {
		return nil, err
	}

	c.gen++
	entry.gen = c.gen
	c.entries[key] = entry

	return entry.view, nil
}

// Destroy releases all cached GPU textures and views.
func (c *ImageCache) Destroy() {
	for key, entry := range c.entries {
		entry.view.Release()
		entry.texture.Release()
		delete(c.entries, key)
	}
}

// uploadImage creates a GPU texture and uploads pixel data from an ImageDrawCommand.
func (c *ImageCache) uploadImage(cmd *ImageDrawCommand) (*imageCacheEntry, error) {
	w := cmd.ImgWidth
	h := cmd.ImgHeight
	if w == 0 || h == 0 {
		return nil, fmt.Errorf("empty image (%dx%d)", w, h)
	}

	tex, err := c.device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "image_cache_tex",
		Size:          wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1}, //nolint:gosec // image dimensions fit uint32
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding | gputypes.TextureUsageCopyDst,
	})
	if err != nil {
		return nil, fmt.Errorf("create image texture: %w", err)
	}

	view, err := c.device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
		Label:         "image_cache_view",
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Dimension:     gputypes.TextureViewDimension2D,
		Aspect:        gputypes.TextureAspectAll,
		MipLevelCount: 1,
	})
	if err != nil {
		tex.Release()
		return nil, fmt.Errorf("create image texture view: %w", err)
	}

	// Upload pixel data. The source may have stride != w*4.
	// Repack to tight rows if needed.
	bytesPerRow := uint32(w * 4) //nolint:gosec // image width fits uint32
	var pixelData []byte
	stride := cmd.ImgStride
	if stride == 0 {
		stride = w * 4
	}
	if stride == w*4 {
		pixelData = cmd.PixelData[:w*h*4]
	} else {
		pixelData = make([]byte, w*h*4)
		for row := 0; row < h; row++ {
			srcOff := row * stride
			dstOff := row * w * 4
			copy(pixelData[dstOff:dstOff+w*4], cmd.PixelData[srcOff:srcOff+w*4])
		}
	}

	if err := c.queue.WriteTexture(
		&wgpu.ImageCopyTexture{Texture: tex, MipLevel: 0},
		pixelData,
		&wgpu.ImageDataLayout{
			Offset:       0,
			BytesPerRow:  bytesPerRow,
			RowsPerImage: uint32(h), //nolint:gosec // image height fits uint32
		},
		&wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1}, //nolint:gosec // image dimensions fit uint32
	); err != nil {
		view.Release()
		tex.Release()
		return nil, fmt.Errorf("upload image pixels: %w", err)
	}

	return &imageCacheEntry{
		texture: tex,
		view:    view,
		width:   w,
		height:  h,
	}, nil
}

// evictOldest removes the least recently used cache entry.
func (c *ImageCache) evictOldest() {
	var oldestKey uintptr
	oldestGen := ^uint64(0)
	for key, entry := range c.entries {
		if entry.gen < oldestGen {
			oldestGen = entry.gen
			oldestKey = key
		}
	}
	if entry, ok := c.entries[oldestKey]; ok {
		entry.view.Release()
		entry.texture.Release()
		delete(c.entries, oldestKey)
	}
}

// pixelDataKey returns a cache key based on the pixel data slice's underlying
// array pointer. Two slices backed by the same array produce the same key.
// This is identity-based: the same ImageBuf's Data() returns the same pointer.
func pixelDataKey(data []byte) uintptr {
	if len(data) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&data[0]))
}
