package utils

import (
	"log"
	"sync"
	"time"
)

// GameState represents common game state information
type GameState interface {
	GetUserID() int64
	GetGameType() string
	GetCreatedAt() time.Time
	GetExpiresAt() time.Time
	IsExpired() bool
	Cleanup()
}

// GameStateManager provides centralized management for all game states
type GameStateManager struct {
	// Organize games by type for efficient cleanup
	gamesByType map[string]map[interface{}]GameState
	// Index by user for quick user-specific queries
	gamesByUser   map[int64][]interface{}
	mutex         sync.RWMutex
	cleanupTicker *time.Ticker
	done          chan bool
}

// Global game state manager
var GameStateMgr *GameStateManager

// InitializeGameManager sets up centralized game state management
func InitializeGameManager() {
	GameStateMgr = &GameStateManager{
		gamesByType: make(map[string]map[interface{}]GameState),
		gamesByUser: make(map[int64][]interface{}),
		done:        make(chan bool),
	}

	// Start coordinated cleanup routine every 90 seconds
	GameStateMgr.cleanupTicker = time.NewTicker(90 * time.Second)
	go GameStateMgr.cleanupRoutine()
}

// CloseGameManager stops the centralized game manager
func CloseGameManager() {
	if GameStateMgr != nil && GameStateMgr.cleanupTicker != nil {
		GameStateMgr.cleanupTicker.Stop()
		GameStateMgr.done <- true
	}
}

// RegisterGame adds a game to centralized tracking
func (gsm *GameStateManager) RegisterGame(gameType string, gameID interface{}, game GameState) {
	gsm.mutex.Lock()
	defer gsm.mutex.Unlock()

	// Initialize game type map if it doesn't exist
	if gsm.gamesByType[gameType] == nil {
		gsm.gamesByType[gameType] = make(map[interface{}]GameState)
	}

	// Add to type index
	gsm.gamesByType[gameType][gameID] = game

	// Add to user index
	userID := game.GetUserID()
	gsm.gamesByUser[userID] = append(gsm.gamesByUser[userID], gameID)
}

// UnregisterGame removes a game from centralized tracking
func (gsm *GameStateManager) UnregisterGame(gameType string, gameID interface{}) {
	gsm.mutex.Lock()
	defer gsm.mutex.Unlock()

	// Get the game to find user ID for cleanup
	if gameMap, exists := gsm.gamesByType[gameType]; exists {
		if game, exists := gameMap[gameID]; exists {
			userID := game.GetUserID()

			// Remove from type index
			delete(gameMap, gameID)

			// Remove from user index
			userGames := gsm.gamesByUser[userID]
			for i, id := range userGames {
				if id == gameID {
					gsm.gamesByUser[userID] = append(userGames[:i], userGames[i+1:]...)
					break
				}
			}

			// Clean up empty slices
			if len(gsm.gamesByUser[userID]) == 0 {
				delete(gsm.gamesByUser, userID)
			}
		}
	}
}

// GetUserGames returns all active games for a specific user
func (gsm *GameStateManager) GetUserGames(userID int64) map[string][]interface{} {
	gsm.mutex.RLock()
	defer gsm.mutex.RUnlock()

	result := make(map[string][]interface{})
	userGameIDs := gsm.gamesByUser[userID]

	for _, gameID := range userGameIDs {
		for gameType, gameMap := range gsm.gamesByType {
			if _, exists := gameMap[gameID]; exists {
				result[gameType] = append(result[gameType], gameID)
			}
		}
	}

	return result
}

// GetGameStats returns statistics about active games
func (gsm *GameStateManager) GetGameStats() map[string]int {
	gsm.mutex.RLock()
	defer gsm.mutex.RUnlock()

	stats := make(map[string]int)
	totalGames := 0

	for gameType, gameMap := range gsm.gamesByType {
		count := len(gameMap)
		stats[gameType] = count
		totalGames += count
	}

	stats["total"] = totalGames
	stats["unique_users"] = len(gsm.gamesByUser)

	return stats
}

// cleanupRoutine runs centralized cleanup for all game types
func (gsm *GameStateManager) cleanupRoutine() {
	for {
		select {
		case <-gsm.cleanupTicker.C:
			gsm.cleanupExpiredGames()
		case <-gsm.done:
			return
		}
	}
}

// cleanupExpiredGames removes expired games across all types in a single pass
func (gsm *GameStateManager) cleanupExpiredGames() {
	gsm.mutex.Lock()
	defer gsm.mutex.Unlock()

	now := time.Now()
	totalCleaned := 0
	cleanedByType := make(map[string]int)

	// Single pass through all games to find expired ones
	expiredGames := make(map[string][]interface{})

	for gameType, gameMap := range gsm.gamesByType {
		for gameID, game := range gameMap {
			if game.IsExpired() || now.After(game.GetExpiresAt()) {
				expiredGames[gameType] = append(expiredGames[gameType], gameID)
			}
		}
	}

	// Remove expired games and update indices
	for gameType, gameIDs := range expiredGames {
		gameMap := gsm.gamesByType[gameType]

		for _, gameID := range gameIDs {
			if game, exists := gameMap[gameID]; exists {
				// Call game-specific cleanup
				game.Cleanup()

				// Remove from type index
				delete(gameMap, gameID)

				// Remove from user index
				userID := game.GetUserID()
				userGames := gsm.gamesByUser[userID]
				for i, id := range userGames {
					if id == gameID {
						gsm.gamesByUser[userID] = append(userGames[:i], userGames[i+1:]...)
						break
					}
				}

				// Clean up empty user entry
				if len(gsm.gamesByUser[userID]) == 0 {
					delete(gsm.gamesByUser, userID)
				}

				totalCleaned++
				cleanedByType[gameType]++
			}
		}
	}

	// Cleanup completed silently for performance
	// Only log if significant cleanup occurred
	if totalCleaned > 10 {
	}
	_ = cleanedByType // Mark as used for build
}

// ForceCleanupUserGames removes all games for a specific user (useful for disconnections)
func (gsm *GameStateManager) ForceCleanupUserGames(userID int64) int {
	gsm.mutex.Lock()
	defer gsm.mutex.Unlock()

	userGameIDs := gsm.gamesByUser[userID]
	if len(userGameIDs) == 0 {
		return 0
	}

	cleaned := 0
	for _, gameID := range userGameIDs {
		for _, gameMap := range gsm.gamesByType {
			if game, exists := gameMap[gameID]; exists {
				game.Cleanup()
				delete(gameMap, gameID)
				cleaned++
			}
		}
	}

	// Remove user entry
	delete(gsm.gamesByUser, userID)

	return cleaned
}
