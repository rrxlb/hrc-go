package utils

import (
	"log"
	"sync"
	"time"
)

// CacheEntry represents a cached user data entry
type CacheEntry struct {
	User      *User
	ExpiresAt time.Time
	Mutex     sync.RWMutex
}

// UserCache manages cached user data
type UserCache struct {
	data      map[int64]*CacheEntry
	mutex     sync.RWMutex
	ttl       time.Duration
	cleanupTicker *time.Ticker
	done      chan bool
}

// Global cache instance
var Cache *UserCache

// InitializeCache sets up the user cache system
func InitializeCache(ttl time.Duration) {
	Cache = &UserCache{
		data:      make(map[int64]*CacheEntry),
		ttl:       ttl,
		done:      make(chan bool),
	}
	
	// Start cleanup routine every 5 minutes
	Cache.cleanupTicker = time.NewTicker(5 * time.Minute)
	go Cache.cleanupRoutine()
}

// CloseCache stops the cache cleanup routine
func CloseCache() {
	if Cache != nil && Cache.cleanupTicker != nil {
		Cache.cleanupTicker.Stop()
		Cache.done <- true
	}
}

// Get retrieves a user from cache
func (uc *UserCache) Get(userID int64) (*User, bool) {
	uc.mutex.RLock()
	entry, exists := uc.data[userID]
	uc.mutex.RUnlock()
	
	if !exists {
		return nil, false
	}
	
	entry.Mutex.RLock()
	defer entry.Mutex.RUnlock()
	
	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		// Remove expired entry
		uc.mutex.Lock()
		delete(uc.data, userID)
		uc.mutex.Unlock()
		return nil, false
	}
	
	// Return a copy to prevent external modifications
	userCopy := *entry.User
	return &userCopy, true
}

// Set stores a user in cache
func (uc *UserCache) Set(userID int64, user *User) {
	// Create a copy to prevent external modifications
	userCopy := *user
	
	entry := &CacheEntry{
		User:      &userCopy,
		ExpiresAt: time.Now().Add(uc.ttl),
	}
	
	uc.mutex.Lock()
	uc.data[userID] = entry
	uc.mutex.Unlock()
}

// Update modifies a cached user entry
func (uc *UserCache) Update(userID int64, user *User) {
	uc.mutex.RLock()
	entry, exists := uc.data[userID]
	uc.mutex.RUnlock()
	
	if !exists {
		// If not in cache, just set it
		uc.Set(userID, user)
		return
	}
	
	entry.Mutex.Lock()
	defer entry.Mutex.Unlock()
	
	// Update the user data and extend expiration
	*entry.User = *user
	entry.ExpiresAt = time.Now().Add(uc.ttl)
}

// Delete removes a user from cache
func (uc *UserCache) Delete(userID int64) {
	uc.mutex.Lock()
	delete(uc.data, userID)
	uc.mutex.Unlock()
}

// Size returns the number of entries in cache
func (uc *UserCache) Size() int {
	uc.mutex.RLock()
	defer uc.mutex.RUnlock()
	return len(uc.data)
}

// Clear removes all entries from cache
func (uc *UserCache) Clear() {
	uc.mutex.Lock()
	uc.data = make(map[int64]*CacheEntry)
	uc.mutex.Unlock()
}

// cleanupRoutine removes expired entries periodically
func (uc *UserCache) cleanupRoutine() {
	for {
		select {
		case <-uc.cleanupTicker.C:
			uc.cleanup()
		case <-uc.done:
			return
		}
	}
}

// cleanup removes expired entries
func (uc *UserCache) cleanup() {
	now := time.Now()
	expiredKeys := make([]int64, 0)
	
	uc.mutex.RLock()
	for userID, entry := range uc.data {
		entry.Mutex.RLock()
		if now.After(entry.ExpiresAt) {
			expiredKeys = append(expiredKeys, userID)
		}
		entry.Mutex.RUnlock()
	}
	uc.mutex.RUnlock()
	
	if len(expiredKeys) > 0 {
		uc.mutex.Lock()
		for _, userID := range expiredKeys {
			delete(uc.data, userID)
		}
		uc.mutex.Unlock()
		
		if len(expiredKeys) > 0 {
			log.Printf("Cleaned up %d expired cache entries. Cache size: %d", len(expiredKeys), uc.Size())
		}
	}
}

// GetCachedUser retrieves user data from cache or database
func GetCachedUser(userID int64) (*User, error) {
	// Try cache first
	if Cache != nil {
		if user, found := Cache.Get(userID); found {
			return user, nil
		}
	}
	
	// If not in cache, get from database
	user, err := GetUser(userID)
	if err != nil {
		return nil, err
	}
	
	// Store in cache if cache is initialized
	if Cache != nil {
		Cache.Set(userID, user)
	}
	
	return user, nil
}

// UpdateCachedUser updates user data in both cache and database
func UpdateCachedUser(userID int64, updates UserUpdateData) (*User, error) {
	// Update in database first
	user, err := UpdateUser(userID, updates)
	if err != nil {
		return nil, err
	}
	
	// Update cache if cache is initialized
	if Cache != nil {
		Cache.Update(userID, user)
	}
	
	// Check for new achievements if achievement manager is initialized
	if AchievementMgr != nil {
		if newAchievements, err := AchievementMgr.CheckUserAchievements(user); err != nil {
			log.Printf("Failed to check achievements for user %d: %v", userID, err)
		} else if len(newAchievements) > 0 {
			log.Printf("User %d earned %d new achievements", userID, len(newAchievements))
		}
	}
	
	return user, nil
}

// InvalidateUserCache removes a user from cache
func InvalidateUserCache(userID int64) {
	if Cache != nil {
		Cache.Delete(userID)
	}
}

// CacheStats returns cache statistics
type CacheStats struct {
	Size         int           `json:"size"`
	TTL          time.Duration `json:"ttl"`
	LastCleanup  time.Time     `json:"last_cleanup"`
}

// GetCacheStats returns current cache statistics
func GetCacheStats() CacheStats {
	if Cache == nil {
		return CacheStats{}
	}
	
	return CacheStats{
		Size: Cache.Size(),
		TTL:  Cache.ttl,
	}
}