package utils

import (
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// CacheEntry represents a cached user data entry with optimization flags
type CacheEntry struct {
	User         *User
	ExpiresAt    time.Time
	Hot          bool      // Frequently accessed users
	AccessCount  int64     // Track access frequency for hot/cold decisions
	LastAccessed time.Time // Track last access time
	Mutex        sync.RWMutex
}

// UserCache manages cached user data with performance optimizations
type UserCache struct {
	data          map[int64]*CacheEntry
	mutex         sync.RWMutex
	hotTTL        time.Duration // Short TTL for frequently accessed users
	coldTTL       time.Duration // Longer TTL for infrequently accessed users
	cleanupTicker *time.Ticker
	done          chan bool
	hotThreshold  int64 // Access count threshold for "hot" classification
}

// Global cache instance
var Cache *UserCache

// InitializeCache sets up the user cache system with dynamic TTL
func InitializeCache(ttl time.Duration) {
	Cache = &UserCache{
		data:         make(map[int64]*CacheEntry),
		hotTTL:       2 * time.Minute, // Hot users: 2 minute TTL for fresh data
		coldTTL:      ttl,             // Cold users: original TTL (typically 5 minutes)
		done:         make(chan bool),
		hotThreshold: 5, // Consider user "hot" after 5 accesses
	}

	// Start cleanup routine every 90 seconds for better performance
	Cache.cleanupTicker = time.NewTicker(90 * time.Second)
	go Cache.cleanupRoutine()
}

// CloseCache stops the cache cleanup routine
func CloseCache() {
	if Cache != nil && Cache.cleanupTicker != nil {
		Cache.cleanupTicker.Stop()
		Cache.done <- true
	}
}

// Get retrieves a user from cache with zero-copy optimization for read-only access
func (uc *UserCache) Get(userID int64) (*User, bool) {
	uc.mutex.RLock()
	entry, exists := uc.data[userID]
	uc.mutex.RUnlock()

	if !exists {
		return nil, false
	}

	entry.Mutex.Lock()
	defer entry.Mutex.Unlock()

	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		// Remove expired entry (defer to avoid deadlock)
		go func() {
			uc.mutex.Lock()
			delete(uc.data, userID)
			uc.mutex.Unlock()
		}()
		return nil, false
	}

	// Update access tracking for hot/cold classification
	entry.AccessCount++
	entry.LastAccessed = time.Now()

	// Update hot status based on access patterns
	if entry.AccessCount >= uc.hotThreshold {
		entry.Hot = true
	}

	// Return direct pointer for read-only access (zero-copy)
	// Caller must not modify the returned user data
	return entry.User, true
}

// GetCopy retrieves a copy of user from cache (for modifications)
func (uc *UserCache) GetCopy(userID int64) (*User, bool) {
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

	// Return a copy for safe modifications
	userCopy := *entry.User
	return &userCopy, true
}

// Set stores a user in cache with dynamic TTL
func (uc *UserCache) Set(userID int64, user *User) {
	// Create a copy to prevent external modifications
	userCopy := *user
	now := time.Now()

	// Check if this is an existing entry to preserve hot status
	uc.mutex.RLock()
	existingEntry, exists := uc.data[userID]
	uc.mutex.RUnlock()

	var hot bool
	var accessCount int64
	if exists && existingEntry != nil {
		existingEntry.Mutex.RLock()
		hot = existingEntry.Hot
		accessCount = existingEntry.AccessCount
		existingEntry.Mutex.RUnlock()
	}

	// Use appropriate TTL based on hot/cold status
	ttl := uc.coldTTL
	if hot {
		ttl = uc.hotTTL
	}

	entry := &CacheEntry{
		User:         &userCopy,
		ExpiresAt:    now.Add(ttl),
		Hot:          hot,
		AccessCount:  accessCount,
		LastAccessed: now,
	}

	uc.mutex.Lock()
	uc.data[userID] = entry
	uc.mutex.Unlock()
}

// Update modifies a cached user entry with dynamic TTL
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

	// Update the user data and extend expiration with appropriate TTL
	*entry.User = *user

	ttl := uc.coldTTL
	if entry.Hot {
		ttl = uc.hotTTL
	}

	entry.ExpiresAt = time.Now().Add(ttl)
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

		// Cleanup completed silently for performance
	}
}

// GetCachedUser retrieves user data from cache or database (zero-copy for read-only access)
func GetCachedUser(userID int64) (*User, error) {
	// Try cache first with zero-copy optimization
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
	return UpdateCachedUserWithNotification(userID, updates, nil, nil)
}

// UpdateCachedUserWithNotification updates user data in both cache and database,
// and sends achievement notifications if session and interaction are provided
func UpdateCachedUserWithNotification(userID int64, updates UserUpdateData, session *discordgo.Session, interaction *discordgo.InteractionCreate) (*User, error) {
	// Update in database first
	user, err := UpdateUser(userID, updates)
	if err != nil {
		return nil, err
	}

	// Update cache if cache is initialized
	if Cache != nil {
		Cache.Update(userID, user)
	}

	// Check for new achievements
	if AchievementMgr != nil {
		if session != nil && interaction != nil {
			// Synchronous check with notification when context is available
			newlyAwarded, err := AchievementMgr.CheckUserAchievements(user)
			if err == nil && len(newlyAwarded) > 0 {
				SendAchievementNotification(session, interaction, newlyAwarded)
			}
		} else {
			// Asynchronous check without notification when no context
			go func(u *User, uid int64) {
				AchievementMgr.CheckUserAchievements(u)
			}(user, userID)
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
	Size        int           `json:"size"`
	HotTTL      time.Duration `json:"hot_ttl"`
	ColdTTL     time.Duration `json:"cold_ttl"`
	HotEntries  int           `json:"hot_entries"`
	ColdEntries int           `json:"cold_entries"`
	LastCleanup time.Time     `json:"last_cleanup"`
}

// GetCacheStats returns current cache statistics with hot/cold breakdown
func GetCacheStats() CacheStats {
	if Cache == nil {
		return CacheStats{}
	}

	Cache.mutex.RLock()
	defer Cache.mutex.RUnlock()

	hot, cold := 0, 0
	for _, entry := range Cache.data {
		entry.Mutex.RLock()
		if entry.Hot {
			hot++
		} else {
			cold++
		}
		entry.Mutex.RUnlock()
	}

	return CacheStats{
		Size:        len(Cache.data),
		HotTTL:      Cache.hotTTL,
		ColdTTL:     Cache.coldTTL,
		HotEntries:  hot,
		ColdEntries: cold,
	}
}
