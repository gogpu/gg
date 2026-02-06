package cache

import (
	"strconv"
	"testing"
)

func BenchmarkCacheGet(b *testing.B) {
	c := New[string, int](1000)
	for i := 0; i < 100; i++ {
		c.Set(strconv.Itoa(i), i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get("50")
	}
}

func BenchmarkCacheSet(b *testing.B) {
	c := New[string, int](1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(strconv.Itoa(i%100), i)
	}
}

func BenchmarkCacheGetOrCreate(b *testing.B) {
	c := New[string, int](1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.GetOrCreate(strconv.Itoa(i%100), func() int {
			return i
		})
	}
}

func BenchmarkShardedCacheGet(b *testing.B) {
	c := NewSharded[string, int](100, StringHasher)
	for i := 0; i < 100; i++ {
		c.Set(strconv.Itoa(i), i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get("50")
	}
}

func BenchmarkShardedCacheSet(b *testing.B) {
	c := NewSharded[string, int](100, StringHasher)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(strconv.Itoa(i%100), i)
	}
}

func BenchmarkShardedCacheGetOrCreate(b *testing.B) {
	c := NewSharded[string, int](100, StringHasher)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.GetOrCreate(strconv.Itoa(i%100), func() int {
			return i
		})
	}
}

func BenchmarkShardedCacheParallel(b *testing.B) {
	c := NewSharded[int, int](100, IntHasher)
	for i := 0; i < 1000; i++ {
		c.Set(i, i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Get(i % 1000)
			i++
		}
	})
}

func BenchmarkShardedCacheParallelMixed(b *testing.B) {
	c := NewSharded[int, int](100, IntHasher)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 == 0 {
				c.Set(i%1000, i)
			} else {
				c.Get(i % 1000)
			}
			i++
		}
	})
}

func BenchmarkIntHasher(b *testing.B) {
	for i := 0; i < b.N; i++ {
		IntHasher(i)
	}
}

func BenchmarkStringHasher(b *testing.B) {
	s := "test_string_key"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		StringHasher(s)
	}
}

func BenchmarkUint64Hasher(b *testing.B) {
	var u uint64 = 12345678901234
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Uint64Hasher(u)
	}
}
