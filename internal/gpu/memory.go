package gpu

import (
	"container/list"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Memory management errors.
var (
	// ErrMemoryBudgetExceeded is returned when allocation would exceed budget.
	ErrMemoryBudgetExceeded = errors.New("wgpu: memory budget exceeded")

	// ErrMemoryManagerClosed is returned when operating on a closed manager.
	ErrMemoryManagerClosed = errors.New("wgpu: memory manager closed")

	// ErrTextureNotFound is returned when a texture is not found in the manager.
	ErrTextureNotFound = errors.New("wgpu: texture not found in manager")
)

// Default memory limits.
const (
	// DefaultMaxMemoryMB is the default maximum GPU memory budget (256 MB).
	DefaultMaxMemoryMB = 256

	// DefaultEvictionThreshold is when eviction starts (80% of budget).
	DefaultEvictionThreshold = 0.8

	// MinMemoryMB is the minimum allowed memory budget (16 MB).
	MinMemoryMB = 16
)

// MemoryStats contains GPU memory usage statistics.
type MemoryStats struct {
	// TotalBytes is the total memory budget in bytes.
	TotalBytes uint64

	// UsedBytes is the currently allocated memory in bytes.
	UsedBytes uint64

	// AvailableBytes is the remaining memory budget.
	AvailableBytes uint64

	// TextureCount is the number of allocated textures.
	TextureCount int

	// EvictionCount is the total number of textures evicted.
	EvictionCount uint64

	// Utilization is the percentage of budget used (0.0 to 1.0).
	Utilization float64
}

// String returns a human-readable string of memory stats.
func (s MemoryStats) String() string {
	return fmt.Sprintf("Memory[%.1f%% used, %d/%d MB, %d textures, %d evictions]",
		s.Utilization*100,
		s.UsedBytes/(1024*1024),
		s.TotalBytes/(1024*1024),
		s.TextureCount,
		s.EvictionCount)
}

// textureEntry tracks a texture in the memory manager with LRU information.
type textureEntry struct {
	texture   *GPUTexture
	sizeBytes uint64
	lastUsed  time.Time
	element   *list.Element // Position in LRU list
}

// MemoryManager tracks GPU memory allocations and enforces budget limits.
// It provides LRU eviction when the memory budget is exceeded.
//
// MemoryManager is safe for concurrent use.
type MemoryManager struct {
	mu sync.RWMutex

	// Backend reference for creating textures
	backend *NativeBackend

	// Memory tracking
	budgetBytes uint64 // Total budget in bytes
	usedBytes   uint64 // Currently used bytes

	// Texture tracking
	textures map[*GPUTexture]*textureEntry

	// LRU list (front = most recently used, back = least recently used)
	lruList *list.List

	// Statistics
	evictionCount uint64

	// Configuration
	evictionThreshold float64 // Start evicting when usage exceeds this fraction

	// State
	closed bool
}

// MemoryManagerConfig holds configuration for creating a MemoryManager.
type MemoryManagerConfig struct {
	// MaxMemoryMB is the maximum memory budget in megabytes.
	// Defaults to DefaultMaxMemoryMB if <= 0.
	MaxMemoryMB int

	// EvictionThreshold is the usage fraction at which eviction starts.
	// Defaults to DefaultEvictionThreshold if <= 0.
	EvictionThreshold float64
}

// NewMemoryManager creates a new memory manager for GPU memory tracking.
// The backend parameter is used for texture creation operations.
func NewMemoryManager(backend *NativeBackend, config MemoryManagerConfig) *MemoryManager {
	maxMB := config.MaxMemoryMB
	if maxMB < MinMemoryMB {
		maxMB = DefaultMaxMemoryMB
	}

	threshold := config.EvictionThreshold
	if threshold <= 0 || threshold > 1.0 {
		threshold = DefaultEvictionThreshold
	}

	//nolint:gosec // G115: maxMB is bounded by MinMemoryMB minimum
	return &MemoryManager{
		backend:           backend,
		budgetBytes:       uint64(maxMB) * 1024 * 1024,
		textures:          make(map[*GPUTexture]*textureEntry),
		lruList:           list.New(),
		evictionThreshold: threshold,
	}
}

// AllocTexture allocates a new texture with the given configuration.
// If the allocation would exceed the memory budget, LRU eviction is triggered.
// Returns an error if the allocation cannot be satisfied even after eviction.
func (m *MemoryManager) AllocTexture(config TextureConfig) (*GPUTexture, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, ErrMemoryManagerClosed
	}

	// Calculate required size
	//nolint:gosec // G115: dimensions validated by CreateTexture
	requiredBytes := uint64(config.Width * config.Height * config.Format.BytesPerPixel())

	// Check if single allocation exceeds budget
	if requiredBytes > m.budgetBytes {
		return nil, fmt.Errorf("%w: texture size %d MB exceeds total budget %d MB",
			ErrMemoryBudgetExceeded,
			requiredBytes/(1024*1024),
			m.budgetBytes/(1024*1024))
	}

	// Evict if necessary
	if err := m.evictIfNeeded(requiredBytes); err != nil {
		return nil, err
	}

	// Create the texture
	tex, err := CreateTexture(m.backend, config)
	if err != nil {
		return nil, err
	}

	// Register the texture
	m.registerTextureLocked(tex)

	return tex, nil
}

// FreeTexture releases a texture and returns its memory to the pool.
// The texture is closed and should not be used after this call.
func (m *MemoryManager) FreeTexture(tex *GPUTexture) error {
	if tex == nil {
		return nil
	}

	m.mu.Lock()

	if m.closed {
		m.mu.Unlock()
		return ErrMemoryManagerClosed
	}

	entry, ok := m.textures[tex]
	if !ok {
		m.mu.Unlock()
		// Texture not managed by us, just close it
		tex.Close()
		return nil
	}

	// Remove from tracking (clears tex.manager to prevent double-unregister)
	m.removeTextureLocked(entry)

	// Unlock before Close() to avoid deadlock
	// (Close() might call unregisterTexture which needs the lock)
	m.mu.Unlock()

	// Close the texture (safe now - manager already cleared)
	tex.Close()

	return nil
}

// TouchTexture updates the last-used time of a texture, moving it to
// the front of the LRU list. Call this when a texture is used for rendering.
func (m *MemoryManager) TouchTexture(tex *GPUTexture) {
	if tex == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.textures[tex]
	if !ok {
		return
	}

	entry.lastUsed = time.Now()
	m.lruList.MoveToFront(entry.element)
}

// Stats returns current memory usage statistics.
func (m *MemoryManager) Stats() MemoryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var utilization float64
	if m.budgetBytes > 0 {
		utilization = float64(m.usedBytes) / float64(m.budgetBytes)
	}

	return MemoryStats{
		TotalBytes:     m.budgetBytes,
		UsedBytes:      m.usedBytes,
		AvailableBytes: m.budgetBytes - m.usedBytes,
		TextureCount:   len(m.textures),
		EvictionCount:  m.evictionCount,
		Utilization:    utilization,
	}
}

// SetBudget updates the memory budget.
// If the new budget is lower than current usage, eviction may be triggered.
func (m *MemoryManager) SetBudget(megabytes int) error {
	if megabytes < MinMemoryMB {
		megabytes = MinMemoryMB
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrMemoryManagerClosed
	}

	//nolint:gosec // G115: megabytes bounded by MinMemoryMB minimum
	m.budgetBytes = uint64(megabytes) * 1024 * 1024

	// Evict if now over budget
	return m.evictIfNeeded(0)
}

// Close releases all managed textures and closes the memory manager.
// The manager should not be used after Close is called.
func (m *MemoryManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return
	}

	// Close all managed textures
	for tex, entry := range m.textures {
		m.lruList.Remove(entry.element)
		tex.mu.Lock()
		tex.manager = nil
		tex.mu.Unlock()
		tex.Close()
	}

	m.textures = nil
	m.lruList = nil
	m.usedBytes = 0
	m.closed = true
}

// registerTextureLocked adds a texture to management. Caller must hold mu.
func (m *MemoryManager) registerTextureLocked(tex *GPUTexture) {
	entry := &textureEntry{
		texture:   tex,
		sizeBytes: tex.sizeBytes,
		lastUsed:  time.Now(),
	}

	// Add to LRU list (front = most recently used)
	entry.element = m.lruList.PushFront(entry)

	// Add to map
	m.textures[tex] = entry

	// Update used memory
	m.usedBytes += entry.sizeBytes

	// Set manager reference on texture
	tex.SetMemoryManager(m)
}

// unregisterTexture removes a texture from management.
// Called by GPUTexture.Close() to notify the manager.
func (m *MemoryManager) unregisterTexture(tex *GPUTexture) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.textures[tex]
	if !ok {
		return
	}

	m.removeTextureLocked(entry)
}

// removeTextureLocked removes a texture entry from tracking. Caller must hold mu.
// It also clears the manager reference in the texture to prevent double-unregister.
func (m *MemoryManager) removeTextureLocked(entry *textureEntry) {
	if entry.element != nil {
		m.lruList.Remove(entry.element)
	}

	delete(m.textures, entry.texture)
	m.usedBytes -= entry.sizeBytes

	// Clear manager reference to prevent double-unregister from Close()
	entry.texture.mu.Lock()
	entry.texture.manager = nil
	entry.texture.mu.Unlock()
}

// evictIfNeeded evicts textures until there's room for the requested size.
// Caller must hold mu.
func (m *MemoryManager) evictIfNeeded(requestedBytes uint64) error {
	targetBytes := m.usedBytes + requestedBytes
	thresholdBytes := uint64(float64(m.budgetBytes) * m.evictionThreshold)

	// No eviction needed if under threshold and request fits
	if targetBytes <= m.budgetBytes && m.usedBytes < thresholdBytes {
		return nil
	}

	// Evict from back of LRU list (least recently used)
	for targetBytes > m.budgetBytes && m.lruList.Len() > 0 {
		// Get least recently used
		elem := m.lruList.Back()
		if elem == nil {
			break
		}

		entry, ok := elem.Value.(*textureEntry)
		if !ok {
			m.lruList.Remove(elem)
			continue
		}

		// Remove and close the texture
		tex := entry.texture
		m.removeTextureLocked(entry) // Also clears manager reference

		tex.Close()

		m.evictionCount++
		targetBytes = m.usedBytes + requestedBytes
	}

	// Check if we freed enough
	if targetBytes > m.budgetBytes {
		return fmt.Errorf("%w: need %d bytes, have %d bytes available",
			ErrMemoryBudgetExceeded, requestedBytes, m.budgetBytes-m.usedBytes)
	}

	return nil
}

// Textures returns a slice of all managed textures.
// The returned slice is a copy and can be safely modified.
func (m *MemoryManager) Textures() []*GPUTexture {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*GPUTexture, 0, len(m.textures))
	for tex := range m.textures {
		result = append(result, tex)
	}
	return result
}

// Contains returns true if the texture is managed by this manager.
func (m *MemoryManager) Contains(tex *GPUTexture) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.textures[tex]
	return ok
}
