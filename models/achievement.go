package models

import (
	"fmt"
	"time"
)

// Achievement represents an achievement definition
type Achievement struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Icon        string    `json:"icon"`
	Category    string    `json:"category"`
	Type        string    `json:"achievement_type"`
	TargetValue int       `json:"target_value"`
	ChipsReward int64     `json:"chips_reward"`
	XPReward    int64     `json:"xp_reward"`
	IsHidden    bool      `json:"is_hidden"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
}

// UserAchievement represents a user's progress on or completion of an achievement
type UserAchievement struct {
	UserID        int64     `json:"user_id"`
	AchievementID int       `json:"achievement_id"`
	EarnedAt      time.Time `json:"earned_at"`
	Progress      int       `json:"progress"`

	// Achievement details (from join)
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Category    string `json:"category"`
	Type        string `json:"achievement_type"`
	TargetValue int    `json:"target_value"`
	ChipsReward int64  `json:"chips_reward"`
	XPReward    int64  `json:"xp_reward"`
	IsHidden    bool   `json:"is_hidden"`
}

// IsCompleted checks if the achievement is completed
func (ua *UserAchievement) IsCompleted() bool {
	return ua.Progress >= ua.TargetValue
}

// GetProgressPercentage returns the completion percentage (0-100)
func (ua *UserAchievement) GetProgressPercentage() float64 {
	if ua.TargetValue == 0 {
		return 100.0
	}

	percentage := (float64(ua.Progress) / float64(ua.TargetValue)) * 100
	if percentage > 100 {
		return 100.0
	}

	return percentage
}

// GetProgressText returns a formatted progress text like "5/10"
func (ua *UserAchievement) GetProgressText() string {
	return fmt.Sprintf("%d/%d", ua.Progress, ua.TargetValue)
}

// Achievement types constants
const (
	AchievementTypeWins    = "wins"
	AchievementTypeLosses  = "losses"
	AchievementTypeChips   = "chips"
	AchievementTypeXP      = "xp"
	AchievementTypeBigWin  = "big_win"
	AchievementTypeStreak  = "streak"
	AchievementTypeSpecial = "special"
)
