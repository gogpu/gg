//go:build !nogpu

package gpu

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gogpu/gg"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
)

// Texture-related errors.
var (
	// ErrTextureReleased is returned when operating on a released texture.
	ErrTextureReleased = errors.New("wgpu: texture has been released")

	// ErrTextureSizeMismatch is returned when pixmap size doesn't match texture.
	ErrTextureSizeMismatch = errors.New("wgpu: pixmap size does not match texture")

	// ErrNilPixmap is returned when pixmap is nil.
	ErrNilPixmap = errors.New("wgpu: pixmap is nil")

	// ErrTextureReadbackNotSupported is returned when readback is not available.
	ErrTextureReadbackNotSupported = errors.New("wgpu: texture readback not supported (stub)")
)

// TextureFormat represents the pixel format of a GPU texture.
type TextureFormat uint8

const (
	// TextureFormatRGBA8 is the standard RGBA format with 8 bits per channel.
	TextureFormatRGBA8 TextureFormat = iota

	// TextureFormatBGRA8 is BGRA format, often used for surface presentation.
	TextureFormatBGRA8

	// TextureFormatR8 is single-channel 8-bit format, used for masks.
	TextureFormatR8
)

// String returns a human-readable name for the format.
func (f TextureFormat) String() string {
	switch f {
	case TextureFormatRGBA8:
		return "RGBA8"
	case TextureFormatBGRA8:
		return "BGRA8"
	case TextureFormatR8:
		return "R8"
	default:
		return fmt.Sprintf("Unknown(%d)", f)
	}
}

// BytesPerPixel returns the number of bytes per pixel for the format.
func (f TextureFormat) BytesPerPixel() int {
	switch f {
	case TextureFormatRGBA8, TextureFormatBGRA8:
		return 4
	case TextureFormatR8:
		return 1
	default:
		return 4
	}
}

// ToWGPUFormat converts to wgpu gputypes.TextureFormat.
// This will be used when actual GPU texture creation is implemented.
func (f TextureFormat) ToWGPUFormat() gputypes.TextureFormat {
	switch f {
	case TextureFormatRGBA8:
		return gputypes.TextureFormatRGBA8Unorm
	case TextureFormatBGRA8:
		return gputypes.TextureFormatBGRA8Unorm
	case TextureFormatR8:
		return gputypes.TextureFormatR8Unorm
	default:
		return gputypes.TextureFormatRGBA8Unorm
	}
}

// GPUTexture represents a GPU texture resource.
// It wraps the underlying wgpu texture and provides a high-level interface
// for texture operations including upload and download.
//
// GPUTexture is safe for concurrent read access. Write operations
// (Upload, Close) should be synchronized externally.
type GPUTexture struct {
	mu sync.RWMutex

	// GPU resource IDs (stub - will be real wgpu handles when available)
	textureID core.TextureID
	viewID    core.TextureViewID

	// Texture properties
	width  int
	height int
	format TextureFormat

	// Memory tracking
	sizeBytes uint64
	manager   *MemoryManager // optional, for memory tracking

	// State
	released atomic.Bool
	label    string
}

// TextureConfig holds configuration for creating a new texture.
type TextureConfig struct {
	// Width is the texture width in pixels.
	Width int

	// Height is the texture height in pixels.
	Height int

	// Format is the pixel format.
	Format TextureFormat

	// Label is an optional debug label.
	Label string

	// Usage flags (default: CopySrc | CopyDst | TextureBinding)
	Usage gputypes.TextureUsage
}

// DefaultTextureUsage is the default usage for textures created without specific flags.
const DefaultTextureUsage = gputypes.TextureUsageCopySrc | gputypes.TextureUsageCopyDst | gputypes.TextureUsageTextureBinding

// CreateTexture creates a new GPU texture with the given configuration.
// The texture is uninitialized and should be filled with UploadPixmap.
//
// Note: This is a stub implementation. The actual GPU texture creation
// will be implemented when wgpu texture support is complete.
func CreateTexture(backend *Backend, config TextureConfig) (*GPUTexture, error) {
	if config.Width <= 0 || config.Height <= 0 {
		return nil, ErrInvalidDimensions
	}

	// Allow nil backend for stub/testing mode
	// When backend is nil, we create a logical texture without GPU resources
	if backend != nil && !backend.IsInitialized() {
		return nil, ErrNotInitialized
	}

	// Calculate memory size
	//nolint:gosec // G115: dimensions are validated positive, overflow is acceptable for this use case
	sizeBytes := uint64(config.Width * config.Height * config.Format.BytesPerPixel())

	// Set default usage if not specified
	// Note: usage will be used when actual GPU texture creation is implemented
	_ = config.Usage // Acknowledge usage for future GPU texture creation

	// TODO: Actual wgpu texture creation when available
	// For now, create stub IDs to track the logical texture
	//
	// desc := &gputypes.TextureDescriptor{
	//     Label: config.Label,
	//     Size: gputypes.Extent3D{
	//         Width:              uint32(config.Width),
	//         Height:             uint32(config.Height),
	//         DepthOrArrayLayers: 1,
	//     },
	//     MipLevelCount: 1,
	//     SampleCount:   1,
	//     Dimension:     gputypes.TextureDimension2D,
	//     Format:        config.Format.toWGPUFormat(),
	//     Usage:         usage,
	// }
	// textureID, err := core.CreateTexture(backend.Device(), desc)

	tex := &GPUTexture{
		width:     config.Width,
		height:    config.Height,
		format:    config.Format,
		sizeBytes: sizeBytes,
		label:     config.Label,
		// textureID and viewID are zero (stub)
	}

	return tex, nil
}

// CreateTextureFromPixmap creates a GPU texture from a pixmap, uploading
// the pixel data immediately.
func CreateTextureFromPixmap(backend *Backend, pixmap *gg.Pixmap, label string) (*GPUTexture, error) {
	if pixmap == nil {
		return nil, ErrNilPixmap
	}

	tex, err := CreateTexture(backend, TextureConfig{
		Width:  pixmap.Width(),
		Height: pixmap.Height(),
		Format: TextureFormatRGBA8,
		Label:  label,
	})
	if err != nil {
		return nil, err
	}

	if err := tex.UploadPixmap(pixmap); err != nil {
		tex.Close()
		return nil, err
	}

	return tex, nil
}

// Width returns the texture width in pixels.
func (t *GPUTexture) Width() int {
	return t.width
}

// Height returns the texture height in pixels.
func (t *GPUTexture) Height() int {
	return t.height
}

// Format returns the texture format.
func (t *GPUTexture) Format() TextureFormat {
	return t.format
}

// SizeBytes returns the texture size in bytes.
func (t *GPUTexture) SizeBytes() uint64 {
	return t.sizeBytes
}

// Label returns the debug label.
func (t *GPUTexture) Label() string {
	return t.label
}

// IsReleased returns true if the texture has been released.
func (t *GPUTexture) IsReleased() bool {
	return t.released.Load()
}

// TextureID returns the underlying wgpu texture ID.
// Returns a zero ID for stub textures.
func (t *GPUTexture) TextureID() core.TextureID {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.textureID
}

// ViewID returns the texture view ID.
// Returns a zero ID for stub textures.
func (t *GPUTexture) ViewID() core.TextureViewID {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.viewID
}

// UploadPixmap uploads pixel data from a Pixmap to the GPU texture.
// The pixmap dimensions must match the texture dimensions.
//
// Note: This is a stub implementation. The actual GPU upload will be
// implemented when wgpu queue.WriteTexture is available.
func (t *GPUTexture) UploadPixmap(pixmap *gg.Pixmap) error {
	if t.released.Load() {
		return ErrTextureReleased
	}

	if pixmap == nil {
		return ErrNilPixmap
	}

	if pixmap.Width() != t.width || pixmap.Height() != t.height {
		return fmt.Errorf("%w: expected %dx%d, got %dx%d",
			ErrTextureSizeMismatch, t.width, t.height, pixmap.Width(), pixmap.Height())
	}

	// TODO: Actual GPU upload when wgpu queue.WriteTexture is available
	//
	// data := pixmap.Data()
	// queue := backend.Queue()
	//
	// core.QueueWriteTexture(queue, &gputypes.ImageCopyTexture{
	//     Texture:  uintptr(t.textureID.Raw()),
	//     MipLevel: 0,
	//     Origin:   gputypes.Origin3D{X: 0, Y: 0, Z: 0},
	//     Aspect:   gputypes.TextureAspectAll,
	// }, data, &gputypes.TextureDataLayout{
	//     Offset:       0,
	//     BytesPerRow:  uint32(t.width * t.format.BytesPerPixel()),
	//     RowsPerImage: uint32(t.height),
	// }, &gputypes.Extent3D{
	//     Width:              uint32(t.width),
	//     Height:             uint32(t.height),
	//     DepthOrArrayLayers: 1,
	// })

	return nil
}

// UploadRegion uploads pixel data to a region of the texture.
// This is useful for texture atlas updates.
//
// Note: This is a stub implementation.
func (t *GPUTexture) UploadRegion(x, y int, pixmap *gg.Pixmap) error {
	if t.released.Load() {
		return ErrTextureReleased
	}

	if pixmap == nil {
		return ErrNilPixmap
	}

	// Bounds check
	if x < 0 || y < 0 || x+pixmap.Width() > t.width || y+pixmap.Height() > t.height {
		return fmt.Errorf("%w: region (%d,%d)+(%dx%d) exceeds texture bounds (%dx%d)",
			ErrInvalidDimensions, x, y, pixmap.Width(), pixmap.Height(), t.width, t.height)
	}

	// TODO: Actual GPU upload with offset when wgpu is available
	// Similar to UploadPixmap but with Origin3D{X: uint32(x), Y: uint32(y), Z: 0}

	return nil
}

// DownloadPixmap downloads pixel data from GPU to a new Pixmap.
// This operation requires the texture to have CopySrc usage.
//
// Note: This is a stub implementation that returns an error.
// GPU readback requires staging buffers and synchronization.
func (t *GPUTexture) DownloadPixmap() (*gg.Pixmap, error) {
	if t.released.Load() {
		return nil, ErrTextureReleased
	}

	// TODO: Implement GPU readback when wgpu supports it
	// This requires:
	// 1. Create staging buffer with MapRead usage
	// 2. Copy texture to buffer
	// 3. Map buffer
	// 4. Read data
	// 5. Unmap buffer
	// 6. Destroy staging buffer

	return nil, ErrTextureReadbackNotSupported
}

// SetMemoryManager sets the memory manager for tracking.
// This is called internally when allocating through MemoryManager.
func (t *GPUTexture) SetMemoryManager(m *MemoryManager) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.manager = m
}

// Close releases the GPU texture resources.
// The texture should not be used after Close is called.
func (t *GPUTexture) Close() {
	if t.released.Swap(true) {
		return // Already released
	}

	t.mu.Lock()
	manager := t.manager
	t.mu.Unlock()

	// Notify memory manager if present
	if manager != nil {
		manager.unregisterTexture(t)
	}

	// TODO: Release actual GPU resources when wgpu supports it
	//
	// if !t.viewID.IsZero() {
	//     core.TextureViewDrop(t.viewID)
	// }
	// if !t.textureID.IsZero() {
	//     core.TextureDrop(t.textureID)
	// }

	t.mu.Lock()
	t.textureID = core.TextureID{}
	t.viewID = core.TextureViewID{}
	t.manager = nil
	t.mu.Unlock()
}

// String returns a string representation of the texture.
func (t *GPUTexture) String() string {
	status := "active"
	if t.released.Load() {
		status = "released"
	}
	return fmt.Sprintf("GPUTexture[%s %dx%d %s %d bytes %s]",
		t.label, t.width, t.height, t.format, t.sizeBytes, status)
}
