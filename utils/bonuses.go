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
	BonusServer  BonusType = "server"
)

// BonusInfo contains information about a bonus
type BonusInfo struct {
	Type          BonusType     `json:"type"`
	BaseAmount    int64         `json:"base_amount"`
	ActualAmount  int64         `json:"actual_amount"`
	XPAmount      int64         `json:"xp_amount"`
	Cooldown      time.Duration `json:"cooldown"`
	NextAvailable time.Time     `json:"next_available"`
	Multiplier    float64       `json:"multiplier"`
	StreakBonus   int64         `json:"streak_bonus"`
}

// BonusResult represents the result of claiming a bonus
type BonusResult struct {
	Success       bool          `json:"success"`
	BonusInfo     *BonusInfo    `json:"bonus_info,omitempty"`
	Error         string        `json:"error,omitempty"`
	TimeRemaining time.Duration `json:"time_remaining"`
}

// Bonus configuration constants
const (
	// Base bonus amounts
	BaseHourlyBonus = 25  // 25 chips per hour (matches Python)
	BaseDailyBonus  = 150 // 150 chips per day (matches Python)
	BaseWeeklyBonus = 600 // 600 chips per week (matches Python)
	BaseVoteBonus   = 250 // 250 chips per vote (matches Python)
	BaseServerBonus = 500 // 500 chips per server bonus

	// Prestige bonus amounts (per prestige level) - reduced by ~30%
	PrestigeBonusHourly = 35   // 35 chips per prestige level for hourly (was 50)
	PrestigeBonusDaily  = 250  // 250 chips per prestige level for daily (was 350)
	PrestigeBonusWeekly = 1100 // 1100 chips per prestige level for weekly (was 1500)
	PrestigeBonusVote   = 450  // 450 chips per prestige level for vote (was 625)
	PrestigeBonusServer = 900  // 900 chips per prestige level for server (was 1250)

	// Level bonus amounts (per level) - reduced by ~30%
	LevelBonusHourly = 10  // 10 chips per level for hourly (was 15)
	LevelBonusDaily  = 50  // 50 chips per level for daily (was 75)
	LevelBonusWeekly = 200 // 200 chips per level for weekly (was 300)
	LevelBonusVote   = 85  // 85 chips per level for vote (was 125)
	LevelBonusServer = 175 // 175 chips per level for server (was 250)

	// XP rewards
	HourlyXP = 50   // 50 XP per hourly
	DailyXP  = 250  // 250 XP per daily
	WeeklyXP = 1000 // 1000 XP per weekly
	VoteXP   = 500  // 500 XP per vote
	ServerXP = 750  // 750 XP per server bonus

	// Cooldown periods
	HourlyCooldown = time.Hour          // 1 hour
	DailyCooldown  = 24 * time.Hour     // 24 hours
	WeeklyCooldown = 7 * 24 * time.Hour // 7 days
	VoteCooldown   = 12 * time.Hour     // 12 hours (top.gg voting cooldown)
	ServerCooldown = 24 * time.Hour     // 24 hours (daily server bonus)

	// Bonus multipliers - reduced for better balance
	MaxRankMultiplier     = 1.3  // 30% bonus for max rank (was 50%)
	PrestigeMultiplier    = 0.08 // 8% per prestige level (was 10%)
	MaxPrestigeMultiplier = 1.75 // 175% max bonus from prestige (was 200%)
)

// Main support server configuration
const MainSupportServerID = "1396567190102347776" // Discord server where server bonus can be claimed

// BonusManager handles all bonus-related operations
type BonusManager struct{}

// Global bonus manager
var BonusMgr = &BonusManager{}

// IsUserInMainSupportServer checks if a user is a member of the main support server
func IsUserInMainSupportServer(session *discordgo.Session, userID string) bool {
	if session == nil {
		return false
	}

	// Check if user is in the main support server
	_, err := session.GuildMember(MainSupportServerID, userID)
	return err == nil
}

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
	case BonusServer:
		lastClaimed = user.LastBonus
		cooldown = ServerCooldown
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
	return bm.ClaimBonusWithNotification(user, bonusType, nil, nil)
}

// ClaimBonusWithNotification processes a bonus claim for a user and sends achievement notifications if context is provided
func (bm *BonusManager) ClaimBonusWithNotification(user *User, bonusType BonusType, session *discordgo.Session, interaction *discordgo.InteractionCreate) (*BonusResult, error) {
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
	case BonusServer:
		updates.LastBonus = &now
	}

	// Apply updates to database and cache
	_, err := UpdateCachedUserWithNotification(user.UserID, updates, session, interaction)
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
// Hybrid system: Python base formula + Go multiplier system
func (bm *BonusManager) calculateBonusAmount(user *User, bonusType BonusType) *BonusInfo {
	var baseAmount, prestigeBonus, levelBonus, xpAmount int64
	var cooldown time.Duration

	// Get user level based on current XP and prestige
	level := GetUserLevel(user.CurrentXP, user.Prestige)

	// Get base amounts and bonuses per type (Python formula)
	switch bonusType {
	case BonusHourly:
		baseAmount = BaseHourlyBonus
		prestigeBonus = int64(user.Prestige) * PrestigeBonusHourly
		levelBonus = int64(level) * LevelBonusHourly
		xpAmount = HourlyXP
		cooldown = HourlyCooldown
	case BonusDaily:
		baseAmount = BaseDailyBonus
		prestigeBonus = int64(user.Prestige) * PrestigeBonusDaily
		levelBonus = int64(level) * LevelBonusDaily
		xpAmount = DailyXP
		cooldown = DailyCooldown
	case BonusWeekly:
		baseAmount = BaseWeeklyBonus
		prestigeBonus = int64(user.Prestige) * PrestigeBonusWeekly
		levelBonus = int64(level) * LevelBonusWeekly
		xpAmount = WeeklyXP
		cooldown = WeeklyCooldown
	case BonusVote:
		baseAmount = BaseVoteBonus
		prestigeBonus = int64(user.Prestige) * PrestigeBonusVote
		levelBonus = int64(level) * LevelBonusVote
		xpAmount = VoteXP
		cooldown = VoteCooldown
	case BonusServer:
		baseAmount = BaseServerBonus
		prestigeBonus = int64(user.Prestige) * PrestigeBonusServer
		levelBonus = int64(level) * LevelBonusServer
		xpAmount = ServerXP
		cooldown = ServerCooldown
	}

	// Calculate total amount from Python formula: base + prestige_amount + level_amount
	pythonAmount := baseAmount + prestigeBonus + levelBonus

	// Apply Go multiplier system
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

	// Prestige-based multiplier (additional to Python prestige bonus)
	if user.Prestige > 0 {
		prestigeMultiplierBonus := float64(user.Prestige) * PrestigeMultiplier
		if prestigeMultiplierBonus > MaxPrestigeMultiplier-1.0 {
			prestigeMultiplierBonus = MaxPrestigeMultiplier - 1.0
		}
		multiplier += prestigeMultiplierBonus
	}

	// Apply multiplier to the Python formula result
	actualAmount := int64(float64(pythonAmount) * multiplier)

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

	bonusTypes := []BonusType{BonusHourly, BonusDaily, BonusWeekly, BonusVote, BonusServer}

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

// ClaimAllAvailableBonusesWithGuildCheck claims all available bonuses for a user including server bonus if in main guild
func (bm *BonusManager) ClaimAllAvailableBonusesWithGuildCheck(user *User, session *discordgo.Session, userID string) ([]*BonusResult, error) {
	// Exclude vote bonus - it requires separate Top.gg verification via /vote command
	bonusTypes := []BonusType{BonusHourly, BonusDaily, BonusWeekly}

	// Include server bonus only if user is in main support server
	if IsUserInMainSupportServer(session, userID) {
		bonusTypes = append(bonusTypes, BonusServer)
	}

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
		case BonusServer:
			updates.LastBonus = &now
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

// ClaimAllAvailableBonuses claims all available bonuses for a user
// Note: Vote bonus is excluded from claimall as it requires manual Top.gg verification
func (bm *BonusManager) ClaimAllAvailableBonuses(user *User) ([]*BonusResult, error) {
	// Exclude vote bonus - it requires separate Top.gg verification via /vote command
	bonusTypes := []BonusType{BonusHourly, BonusDaily, BonusWeekly}
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

	embed := CreateBrandedEmbed("🕒 Bonus Cooldowns", "Check your bonus availability and cooldown timers", BotColor)
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
		URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1754610906/TU_khaw12.png",
	}

	bonusNames := map[BonusType]string{
		BonusHourly: "⏰ Hourly Bonus",
		BonusDaily:  "📅 Daily Bonus",
		BonusWeekly: "🗓️ Weekly Bonus",
		BonusVote:   "🗳️ Vote Bonus",
		BonusServer: "🏠 High Roller Club",
	}

	for bonusType, result := range cooldowns {
		name := bonusNames[bonusType]
		var value string

		if result.Success {
			// Bonus is available - clean format without chip amounts
			if bonusType == BonusVote {
				value = "✅ Ready! (Use /vote to claim)"
			} else if bonusType == BonusServer {
				value = "✅ Ready! (Use /bonus to claim)"
			} else {
				value = "✅ Ready!"
			}
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
