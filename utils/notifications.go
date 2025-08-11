package utils

import "sync"

// In-memory notification suppression (resets on restart)
var (
	lastLevelAnnounced = make(map[int64]int)
	lastPrestigeReady  = make(map[int64]int)
	notifMutex         sync.Mutex
)

// ShouldAnnounceLevelUp returns true if level is higher than previously announced
func ShouldAnnounceLevelUp(userID int64, newLevel int) bool {
	notifMutex.Lock()
	defer notifMutex.Unlock()
	prev := lastLevelAnnounced[userID]
	if newLevel > prev {
		lastLevelAnnounced[userID] = newLevel
		return true
	}
	return false
}

// ShouldAnnouncePrestigeReady returns true if we haven't announced for this prestige tier
func ShouldAnnouncePrestigeReady(userID int64, prestige int) bool {
	notifMutex.Lock()
	defer notifMutex.Unlock()
	prev, ok := lastPrestigeReady[userID]
	if !ok || prev != prestige {
		lastPrestigeReady[userID] = prestige
		return true
	}
	return false
}

// ResetPrestigeReady clears the prestige-ready flag after user prestiges
func ResetPrestigeReady(userID int64) {
	notifMutex.Lock()
	delete(lastPrestigeReady, userID)
	notifMutex.Unlock()
}
