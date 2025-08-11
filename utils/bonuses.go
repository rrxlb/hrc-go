package utils

import (
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

// BonusType represents different types of bonuses
type BonusType string

const (
	BonusHourly  BonusType = "hourly"
	BonusDaily   BonusType = "daily"
	BonusWeekly  BonusType = "weekly"
	BonusSpecial BonusType = "special"
	BonusVote    BonusType = "vote"
)

// BonusInfo contains information about a bonus
type BonusInfo struct {
	Type          BonusType `json:"type"`
	BaseAmount    int64     `json:"base_amount"`
	ActualAmount  int64     `json:"actual_amount"`
	XPAmount      int64     `json:"xp_amount"`
	Cooldown      time.Duration `json:"cooldown"`
	NextAvailable time.Time `json:"next_available"`
	Multiplier    float64   `json:"multiplier"`
	StreakBonus   int64     `json:"streak_bonus"`
}

// BonusResult represents the result of claiming a bonus
type BonusResult struct {
	Success       bool      `json:"success"`
	BonusInfo     *BonusInfo `json:"bonus_info,omitempty"`
	Error         string    `json:"error,omitempty"`
	TimeRemaining time.Duration `json:"time_remaining"`
}

// Bonus configuration constants
const (
	// Base bonus amounts
	BaseHourlyBonus  = 100   // 100 chips per hour
	BaseDailyBonus   = 500   // 500 chips per day
	BaseWeeklyBonus  = 2500  // 2500 chips per week
	BaseVoteBonus    = 1000  // 1000 chips per vote

	// XP rewards
	HourlyXP  = 50   // 50 XP per hourly
	DailyXP   = 250  // 250 XP per daily
	WeeklyXP  = 1000 // 1000 XP per weekly
	VoteXP    = 500  // 500 XP per vote

	// Cooldown periods
	HourlyCooldown = time.Hour        // 1 hour
	DailyCooldown  = 24 * time.Hour   // 24 hours
	WeeklyCooldown = 7 * 24 * time.Hour // 7 days
	VoteCooldown   = 12 * time.Hour   // 12 hours (top.gg voting cooldown)

	// Bonus multipliers
	MaxRankMultiplier     = 1.5  // 50% bonus for max rank
	PrestigeMultiplier    = 0.1  // 10% per prestige level
	MaxPrestigeMultiplier = 2.0  // 200% max bonus from prestige
)

// BonusManager handles all bonus-related operations
type BonusManager struct{}

// Global bonus manager
var BonusMgr = &BonusManager{}

// CanClaimBonus checks if a user can claim a specific bonus type
func (bm *BonusManager) CanClaimBonus(user *User, bonusType BonusType) *BonusResult {
	var lastClaimed *time.Time
	var cooldown time.Duration

	switch bonusType {
	case BonusHourly:
		lastClaimed = user.LastHourly
		cooldown = HourlyCooldown
	case BonusDaily:
		lastClaimed = user.LastDaily
		cooldown = DailyCooldown
	case BonusWeekly:
		lastClaimed = user.LastWeekly
		cooldown = WeeklyCooldown
	case BonusVote:
		lastClaimed = user.LastVote
		cooldown = VoteCooldown
	default:
		return &BonusResult{
			Success: false,
			Error:   "Invalid bonus type",
		}
	}

	// Check if enough time has passed
	if lastClaimed != nil {
		nextAvailable := lastClaimed.Add(cooldown)
		if time.Now().Before(nextAvailable) {
			return &BonusResult{
				Success:       false,
				Error:         "Bonus not yet available",
				TimeRemaining: time.Until(nextAvailable),
			}
		}
	}

	return &BonusResult{Success: true}
}

// ClaimBonus processes a bonus claim for a user
func (bm *BonusManager) ClaimBonus(user *User, bonusType BonusType) (*BonusResult, error) {
	// Check if bonus can be claimed
	canClaim := bm.CanClaimBonus(user, bonusType)
	if !canClaim.Success {
		return canClaim, nil
	}

	// Calculate bonus amounts
	bonusInfo := bm.calculateBonusAmount(user, bonusType)
	
	// Update user data
	now := time.Now()
	updates := UserUpdateData{
		ChipsIncrement:   bonusInfo.ActualAmount,
		TotalXPIncrement: bonusInfo.XPAmount,
	}

	// Update the appropriate timestamp
	switch bonusType {
	case BonusHourly:
		updates.LastHourly = &now
	case BonusDaily:
		updates.LastDaily = &now
		updates.DailyBonusesClaimedIncrement = 1
	case BonusWeekly:
		updates.LastWeekly = &now
	case BonusVote:
		updates.LastVote = &now
		updates.VotesCountIncrement = 1
	}

	// Apply updates to database and cache
	updatedUser, err := UpdateCachedUser(user.UserID, updates)
	if err != nil {
		return &BonusResult{
			Success: false,
			Error:   "Failed to update user data",
		}, err
	}

	log.Printf("User %d claimed %s bonus: %d chips, %d XP", 
		user.UserID, bonusType, bonusInfo.ActualAmount, bonusInfo.XPAmount)

	// Return success result
	bonusInfo.NextAvailable = now.Add(bonusInfo.Cooldown)
	return &BonusResult{
		Success:   true,
		BonusInfo: bonusInfo,
	}, nil
}

// calculateBonusAmount calculates the actual bonus amount based on user stats
func (bm *BonusManager) calculateBonusAmount(user *User, bonusType BonusType) *BonusInfo {
	var baseAmount, xpAmount int64
	var cooldown time.Duration

	// Get base amounts
	switch bonusType {
	case BonusHourly:
		baseAmount = BaseHourlyBonus
		xpAmount = HourlyXP
		cooldown = HourlyCooldown
	case BonusDaily:
		baseAmount = BaseDailyBonus
		xpAmount = DailyXP
		cooldown = DailyCooldown
	case BonusWeekly:
		baseAmount = BaseWeeklyBonus
		xpAmount = WeeklyXP
		cooldown = WeeklyCooldown
	case BonusVote:
		baseAmount = BaseVoteBonus
		xpAmount = VoteXP
		cooldown = VoteCooldown
	}

	// Calculate multipliers
	multiplier := 1.0

	// Rank-based multiplier
	_, _, _, nextRankXP := GetRank(user.TotalXP)
	if nextRankXP == user.TotalXP { // Max rank reached
		multiplier += MaxRankMultiplier - 1.0 // Add 50% bonus
	} else {
		// Scale bonus based on rank progress
		rankProgress := float64(user.TotalXP) / float64(nextRankXP)
		multiplier += rankProgress * (MaxRankMultiplier - 1.0)
	}

	// Prestige-based multiplier
	if user.Prestige > 0 {
		prestigeBonus := float64(user.Prestige) * PrestigeMultiplier
		if prestigeBonus > MaxPrestigeMultiplier - 1.0 {
			prestigeBonus = MaxPrestigeMultiplier - 1.0
		}
		multiplier += prestigeBonus
	}

	// Apply multiplier
	actualAmount := int64(float64(baseAmount) * multiplier)
	
	// Calculate streak bonus for daily bonuses
	var streakBonus int64
	if bonusType == BonusDaily {
		streakBonus = bm.calculateDailyStreakBonus(user)
		actualAmount += streakBonus
	}

	return &BonusInfo{
		Type:         bonusType,
		BaseAmount:   baseAmount,
		ActualAmount: actualAmount,
		XPAmount:     xpAmount,
		Cooldown:     cooldown,
		Multiplier:   multiplier,
		StreakBonus:  streakBonus,
	}
}

// calculateDailyStreakBonus calculates bonus for consecutive daily claims
func (bm *BonusManager) calculateDailyStreakBonus(user *User) int64 {
	// This is a simplified streak calculation
	// In a full implementation, you'd track daily streak in the database
	dailyCount := user.DailyBonusesClaimed
	
	// Every 7 days gives a streak bonus
	streakWeeks := dailyCount / 7
	return int64(streakWeeks) * 100 // 100 chips per week of daily bonuses
}

// GetAllCooldowns returns cooldown information for all bonus types
func (bm *BonusManager) GetAllCooldowns(user *User) map[BonusType]*BonusResult {
	cooldowns := make(map[BonusType]*BonusResult)
	
	bonusTypes := []BonusType{BonusHourly, BonusDaily, BonusWeekly, BonusVote}
	
	for _, bonusType := range bonusTypes {
		result := bm.CanClaimBonus(user, bonusType)
		if result.Success {
			// If can claim, calculate the bonus info
			bonusInfo := bm.calculateBonusAmount(user, bonusType)
			result.BonusInfo = bonusInfo
		}
		cooldowns[bonusType] = result
	}
	
	return cooldowns
}

// ClaimAllAvailableBonuses claims all available bonuses for a user
func (bm *BonusManager) ClaimAllAvailableBonuses(user *User) ([]*BonusResult, error) {
	bonusTypes := []BonusType{BonusHourly, BonusDaily, BonusWeekly, BonusVote}
	var claimedBonuses []*BonusResult
	var totalChips, totalXP int64

	// First, check which bonuses can be claimed
	var availableBonuses []BonusType
	for _, bonusType := range bonusTypes {
		if result := bm.CanClaimBonus(user, bonusType); result.Success {
			availableBonuses = append(availableBonuses, bonusType)
		}
	}

	if len(availableBonuses) == 0 {
		return nil, nil // No bonuses available
	}

	// Calculate total amounts for all available bonuses
	now := time.Now()
	updates := UserUpdateData{}

	for _, bonusType := range availableBonuses {
		bonusInfo := bm.calculateBonusAmount(user, bonusType)
		totalChips += bonusInfo.ActualAmount
		totalXP += bonusInfo.XPAmount

		// Set timestamps
		switch bonusType {
		case BonusHourly:
			updates.LastHourly = &now
		case BonusDaily:
			updates.LastDaily = &now
			updates.DailyBonusesClaimedIncrement = 1
		case BonusWeekly:
			updates.LastWeekly = &now
		case BonusVote:
			updates.LastVote = &now
			updates.VotesCountIncrement = 1
		}

		// Add to results
		bonusInfo.NextAvailable = now.Add(bonusInfo.Cooldown)
		claimedBonuses = append(claimedBonuses, &BonusResult{
			Success:   true,
			BonusInfo: bonusInfo,
		})
	}

	// Apply all updates at once
	updates.ChipsIncrement = totalChips
	updates.TotalXPIncrement = totalXP

	_, err := UpdateCachedUser(user.UserID, updates)
	if err != nil {
		return nil, fmt.Errorf("failed to update user data: %w", err)
	}

	log.Printf("User %d claimed %d bonuses: %d chips, %d XP total", 
		user.UserID, len(claimedBonuses), totalChips, totalXP)

	return claimedBonuses, nil
}

// CreateBonusEmbed creates a Discord embed for bonus information
func (bm *BonusManager) CreateBonusEmbed(user *User, bonusResult *BonusResult, title string) *discordgo.MessageEmbed {
	if !bonusResult.Success {
		// Error embed
		return &discordgo.MessageEmbed{
			Title:       title,
			Description: fmt.Sprintf("❌ %s", bonusResult.Error),
			Color:       0xff0000, // Red
			Footer: &discordgo.MessageEmbedFooter{
				Text: "Bonus System",
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}
	}

	bonusInfo := bonusResult.BonusInfo
	embed := &discordgo.MessageEmbed{
		Title: title,
		Color: 0x00ff00, // Green
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "💰 Chips Earned",
				Value:  fmt.Sprintf("%d %s", bonusInfo.ActualAmount, ChipsEmoji),
				Inline: true,
			},
			{
				Name:   "⭐ XP Earned", 
				Value:  fmt.Sprintf("%d XP", bonusInfo.XPAmount),
				Inline: true,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Bonus System",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// Add bonus details
	if bonusInfo.Multiplier > 1.0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "🎯 Bonus Multiplier",
			Value:  fmt.Sprintf("%.1fx", bonusInfo.Multiplier),
			Inline: true,
		})
	}

	if bonusInfo.StreakBonus > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "🔥 Streak Bonus",
			Value:  fmt.Sprintf("%d %s", bonusInfo.StreakBonus, ChipsEmoji),
			Inline: true,
		})
	}

	// Add next available time
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "⏰ Next Available",
		Value:  fmt.Sprintf("<t:%d:R>", bonusInfo.NextAvailable.Unix()),
		Inline: false,
	})

	return embed
}

// CreateCooldownEmbed creates a Discord embed showing all cooldowns
func (bm *BonusManager) CreateCooldownEmbed(user *User) *discordgo.MessageEmbed {
	cooldowns := bm.GetAllCooldowns(user)
	
	embed := &discordgo.MessageEmbed{
		Title:       "🕒 Bonus Cooldowns",
		Description: fmt.Sprintf("Bonus status for %s", user.UserID),
		Color:       BotColor,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Bonus System",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	bonusNames := map[BonusType]string{
		BonusHourly: "⏰ Hourly Bonus",
		BonusDaily:  "📅 Daily Bonus",
		BonusWeekly: "🗓️ Weekly Bonus",
		BonusVote:   "🗳️ Vote Bonus",
	}

	for bonusType, result := range cooldowns {
		name := bonusNames[bonusType]
		var value string

		if result.Success {
			// Bonus is available
			value = fmt.Sprintf("✅ Ready! (%d %s)", 
				result.BonusInfo.ActualAmount, ChipsEmoji)
		} else {
			// Show time remaining
			if result.TimeRemaining > 0 {
				hours := int(result.TimeRemaining.Hours())
				minutes := int(result.TimeRemaining.Minutes()) % 60
				
				if hours > 24 {
					days := hours / 24
					hours = hours % 24
					value = fmt.Sprintf("⏳ %dd %dh %dm", days, hours, minutes)
				} else if hours > 0 {
					value = fmt.Sprintf("⏳ %dh %dm", hours, minutes)
				} else {
					value = fmt.Sprintf("⏳ %dm", minutes)
				}
			} else {
				value = "✅ Ready!"
			}
		}

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   name,
			Value:  value,
			Inline: true,
		})
	}

	return embed
}

// FormatDuration formats a duration into a human-readable string
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "Ready!"
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 24 {
		days := hours / 24
		hours = hours % 24
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	} else {
		return fmt.Sprintf("%ds", seconds)
	}
}