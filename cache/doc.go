// Package cache provides generic, high-performance caching primitives.
//
// This package offers two cache implementations optimized for different use cases:
//
// # Cache[K, V]
//
// A simple thread-safe LRU cache suitable for single-threaded or low-contention
// scenarios. Uses a soft limit with 25% eviction when capacity is exceeded.
//
//	cache := cache.New[string, int](100)
//	cache.Set("key", 42)
//	value, ok := cache.Get("key")
//
// # ShardedCache[K, V]
//
// A high-performance sharded cache designed for high-concurrency scenarios.
// Uses 16 shards to reduce lock contention, with proper LRU eviction per shard.
//
//	cache := cache.NewSharded[string, int](256, cache.StringHasher)
//	cache.Set("key", 42)
//	value, ok := cache.Get("key")
//
// # Performance
//
// Benchmarked on Intel i7-1255U:
//   - Cache hit: ~75ns (zero allocations)
//   - Cache miss: ~35ns
//   - Parallel (12 cores): ~170ns/op
//
// # Thread Safety
//
// Both Cache and ShardedCache are safe for concurrent use.
// Neither should be copied after creation (they contain mutexes).
package cache
