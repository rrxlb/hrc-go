package utils

import (
	"testing"
	"time"
)

// BenchmarkObjectPools tests the performance of object pooling
func BenchmarkObjectPools(b *testing.B) {
	b.Run("UserPool", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			user := GetUserFromPool()
			user.UserID = int64(i)
			user.Chips = 1000
			PutUserToPool(user)
		}
	})

	b.Run("EmbedPool", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			embed := GetEmbedFromPool()
			embed.Title = "Test"
			embed.Description = "Test Description"
			PutEmbedToPool(embed)
		}
	})

	b.Run("StringSlicePool", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			slice := GetStringSliceFromPool()
			slice = append(slice, "test1", "test2", "test3")
			PutStringSliceToPool(slice)
		}
	})
}

// BenchmarkCacheOperations tests cache performance
func BenchmarkCacheOperations(b *testing.B) {
	// Initialize cache for testing
	InitializeCache(5 * time.Minute)
	defer CloseCache()

	testUser := &User{
		UserID: 12345,
		Chips:  1000,
		TotalXP: 5000,
	}

	b.Run("CacheSet", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			Cache.Set(int64(i), testUser)
		}
	})

	b.Run("CacheGet", func(b *testing.B) {
		// Pre-populate cache
		for i := 0; i < 1000; i++ {
			Cache.Set(int64(i), testUser)
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			Cache.Get(int64(i % 1000))
		}
	})
}

// BenchmarkFormatting tests number formatting performance
func BenchmarkFormatting(b *testing.B) {
	testNumbers := []int64{123, 1234, 12345, 123456, 1234567, 12345678}
	
	b.Run("FormatChips", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			FormatChips(testNumbers[i%len(testNumbers)])
		}
	})
}