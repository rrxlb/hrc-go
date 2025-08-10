package models

import (
	"time"
)

// User represents a Discord user in the casino bot system
type User struct {
	UserID        int64           `json:"user_id"`
	Username      string          `json:"username"`
	Chips         int64           `json:"chips"`
	Wins          int             `json:"wins"`
	Losses        int             `json:"losses"`
	TotalXP       int64           `json:"total_xp"`
	CurrentXP     int64           `json:"current_xp"`
	PrestigeLevel int             `json:"prestige_level"`
	PremiumData   map[string]interface{} `json:"premium_data"`
	LastDaily     *time.Time      `json:"last_daily"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// UserUpdateData represents the data that can be updated for a user
type UserUpdateData struct {
	Username           string `json:"username,omitempty"`
	ChipsIncrement     int64  `json:"chips_increment,omitempty"`
	WinsIncrement      int    `json:"wins_increment,omitempty"`
	LossesIncrement    int    `json:"losses_increment,omitempty"`
	TotalXPIncrement   int64  `json:"total_xp_increment,omitempty"`
	CurrentXPIncrement int64  `json:"current_xp_increment,omitempty"`
	PrestigeLevel      int    `json:"prestige_level,omitempty"`
	LastDaily          *time.Time `json:"last_daily,omitempty"`
}

// Rank represents a user rank
type Rank struct {
	Name       string
	Icon       string
	XPRequired int
	Color      int
}

// GetRank returns the user's current rank based on total XP
func (u *User) GetRank() Rank {
	ranks := map[int]Rank{
		0: {"Novice", "🥉", 0, 0xcd7f32},
		1: {"Apprentice", "🥈", 10000, 0xc0c0c0},
		2: {"Gambler", "🥇", 40000, 0xffd700},
		3: {"High Roller", "💰", 125000, 0x22a7f0},
		4: {"Card Shark", "🦈", 350000, 0x1f3a93},
		5: {"Pit Boss", "👑", 650000, 0x9b59b6},
		6: {"Legend", "🌟", 2000000, 0xf1c40f},
		7: {"Casino Elite", "💎", 4500000, 0x1abc9c},
	}
	
	currentRank := ranks[0] // Default to first rank
	
	for level := len(ranks) - 1; level >= 0; level-- {
		rank, exists := ranks[level]
		if exists && u.TotalXP >= int64(rank.XPRequired) {
			return rank
		}
	}
	
	return currentRank
}

// GetRankLevel returns the numerical rank level (0-7)
func (u *User) GetRankLevel() int {
	ranks := map[int]Rank{
		0: {"Novice", "🥉", 0, 0xcd7f32},
		1: {"Apprentice", "🥈", 10000, 0xc0c0c0},
		2: {"Gambler", "🥇", 40000, 0xffd700},
		3: {"High Roller", "💰", 125000, 0x22a7f0},
		4: {"Card Shark", "🦈", 350000, 0x1f3a93},
		5: {"Pit Boss", "👑", 650000, 0x9b59b6},
		6: {"Legend", "🌟", 2000000, 0xf1c40f},
		7: {"Casino Elite", "💎", 4500000, 0x1abc9c},
	}
	
	for level := len(ranks) - 1; level >= 0; level-- {
		rank, exists := ranks[level]
		if exists && u.TotalXP >= int64(rank.XPRequired) {
			return level
		}
	}
	return 0
}

// GetNextRank returns the next rank the user can achieve, or nil if at max rank
func (u *User) GetNextRank() *Rank {
	ranks := map[int]Rank{
		0: {"Novice", "🥉", 0, 0xcd7f32},
		1: {"Apprentice", "🥈", 10000, 0xc0c0c0},
		2: {"Gambler", "🥇", 40000, 0xffd700},
		3: {"High Roller", "💰", 125000, 0x22a7f0},
		4: {"Card Shark", "🦈", 350000, 0x1f3a93},
		5: {"Pit Boss", "👑", 650000, 0x9b59b6},
		6: {"Legend", "🌟", 2000000, 0xf1c40f},
		7: {"Casino Elite", "💎", 4500000, 0x1abc9c},
	}
	
	currentLevel := u.GetRankLevel()
	nextLevel := currentLevel + 1
	
	if nextRank, exists := ranks[nextLevel]; exists {
		return &nextRank
	}
	
	return nil // Already at max rank
}

// GetXPToNextRank returns the XP needed to reach the next rank
func (u *User) GetXPToNextRank() int64 {
	nextRank := u.GetNextRank()
	if nextRank == nil {
		return 0 // Already at max rank
	}
	
	return int64(nextRank.XPRequired) - u.TotalXP
}

// HasPremium checks if the user has premium status
func (u *User) HasPremium() bool {
	if u.PremiumData == nil {
		return false
	}
	
	premium, exists := u.PremiumData["active"]
	if !exists {
		return false
	}
	
	active, ok := premium.(bool)
	return ok && active
}

// GetWinRate calculates the user's win rate as a percentage
func (u *User) GetWinRate() float64 {
	totalGames := u.Wins + u.Losses
	if totalGames == 0 {
		return 0.0
	}
	
	return (float64(u.Wins) / float64(totalGames)) * 100
}

// CanAffordBet checks if the user can afford a specific bet amount
func (u *User) CanAffordBet(amount int64) bool {
	return u.Chips >= amount
}

// NetProfit calculates the user's lifetime net profit/loss
func (u *User) NetProfit() int64 {
	const StartingChips = 1000
	return u.Chips - StartingChips
}

// IsNewUser checks if this is a new user (less than 24 hours old)
func (u *User) IsNewUser() bool {
	return time.Since(u.CreatedAt) < 24*time.Hour
}

// CanClaimDaily checks if the user can claim their daily reward
func (u *User) CanClaimDaily() bool {
	if u.LastDaily == nil {
		return true
	}
	
	// Check if it's been at least 24 hours since last daily claim
	return time.Since(*u.LastDaily) >= 24*time.Hour
}

// GetTimeUntilNextDaily returns the duration until the next daily reward can be claimed
func (u *User) GetTimeUntilNextDaily() time.Duration {
	if u.CanClaimDaily() {
		return 0
	}
	
	nextDaily := u.LastDaily.Add(24 * time.Hour)
	return time.Until(nextDaily)
}