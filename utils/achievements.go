package utils

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v5"
)

// AchievementCategory represents different categories of achievements
type AchievementCategory string

const (
	CategoryFirstSteps AchievementCategory = "First Steps"
	CategoryWins       AchievementCategory = "Wins"
	CategoryWealth     AchievementCategory = "Wealth"
	CategoryExperience AchievementCategory = "Experience"
	CategoryPrestige   AchievementCategory = "Prestige"
	CategoryGaming     AchievementCategory = "Gaming"
	CategorySpecial    AchievementCategory = "Special"
	CategoryLoyalty    AchievementCategory = "Loyalty"
)

// RequirementType defines different types of achievement requirements
type RequirementType string

const (
	RequirementChips        RequirementType = "chips"
	RequirementWins         RequirementType = "wins"
	RequirementTotalXP      RequirementType = "total_xp"
	RequirementPrestige     RequirementType = "prestige"
	RequirementGamesPlayed  RequirementType = "games_played"
	RequirementDailyBonuses RequirementType = "daily_bonuses"
	RequirementVotes        RequirementType = "votes"
	RequirementSpecial      RequirementType = "special"
)

// AchievementChecker interface for checking if achievements are earned
type AchievementChecker interface {
	Check(user *User, achievement *Achievement) bool
}

// DefaultAchievementChecker implements standard achievement checking
type DefaultAchievementChecker struct{}

func (c *DefaultAchievementChecker) Check(user *User, achievement *Achievement) bool {
	switch RequirementType(achievement.RequirementType) {
	case RequirementChips:
		return user.Chips >= achievement.RequirementValue
	case RequirementWins:
		return int64(user.Wins) >= achievement.RequirementValue
	case RequirementTotalXP:
		return user.TotalXP >= achievement.RequirementValue
	case RequirementPrestige:
		return int64(user.Prestige) >= achievement.RequirementValue
	case RequirementGamesPlayed:
		return int64(user.Wins+user.Losses) >= achievement.RequirementValue
	case RequirementDailyBonuses:
		return int64(user.DailyBonusesClaimed) >= achievement.RequirementValue
	case RequirementVotes:
		return int64(user.VotesCount) >= achievement.RequirementValue
	default:
		return false
	}
}

// AchievementManager manages achievements and checking
type AchievementManager struct {
	achievements map[int]*Achievement
	checker      AchievementChecker
	mutex        sync.RWMutex
}

// Global achievement manager
var AchievementMgr *AchievementManager

// InitializeAchievementManager sets up the achievement system
func InitializeAchievementManager() error {
	AchievementMgr = &AchievementManager{
		achievements: make(map[int]*Achievement),
		checker:      &DefaultAchievementChecker{},
	}

	// Load achievements from database
	return AchievementMgr.LoadAchievements()
}

// LoadAchievements loads all achievements from the database
func (am *AchievementManager) LoadAchievements() error {
	if DB == nil {
		// Load default achievements for offline mode
		am.loadDefaultAchievements()
		return nil
	}

	ctx := context.Background()
	query := `
		SELECT id, name, description, icon, category, requirement_type, 
		       requirement_value, chips_reward, xp_reward, hidden, created_at
		FROM achievements ORDER BY id`

	rows, err := DB.Query(ctx, query)
	if err != nil {
		// If table doesn't exist, create it and load defaults
		if err := am.createAchievementsTable(); err != nil {
			return fmt.Errorf("failed to create achievements table: %w", err)
		}
		am.loadDefaultAchievements()
		return am.saveDefaultAchievements()
	}
	defer rows.Close()

	am.mutex.Lock()
	defer am.mutex.Unlock()

	for rows.Next() {
		var achievement Achievement
		err := rows.Scan(
			&achievement.ID,
			&achievement.Name,
			&achievement.Description,
			&achievement.Icon,
			&achievement.Category,
			&achievement.RequirementType,
			&achievement.RequirementValue,
			&achievement.ChipsReward,
			&achievement.XPReward,
			&achievement.Hidden,
			&achievement.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to scan achievement: %w", err)
		}
		am.achievements[achievement.ID] = &achievement
	}

	return nil
}

// createAchievementsTable creates the achievements table if it doesn't exist
func (am *AchievementManager) createAchievementsTable() error {
	if DB == nil {
		return fmt.Errorf("database not connected")
	}

	ctx := context.Background()
	query := `
		CREATE TABLE IF NOT EXISTS achievements (
			id SERIAL PRIMARY KEY,
			name VARCHAR(100) NOT NULL UNIQUE,
			description TEXT NOT NULL,
			icon VARCHAR(50) NOT NULL,
			category VARCHAR(50) NOT NULL,
			requirement_type VARCHAR(50) NOT NULL,
			requirement_value BIGINT NOT NULL,
			chips_reward BIGINT NOT NULL DEFAULT 0,
			xp_reward BIGINT NOT NULL DEFAULT 0,
			hidden BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`

	_, err := DB.Exec(ctx, query)
	return err
}

// loadDefaultAchievements loads the standard set of achievements
func (am *AchievementManager) loadDefaultAchievements() {
	defaultAchievements := []*Achievement{
		// First Steps (1-5)
		{ID: 1, Name: "First Blood", Description: "Win your first game", Icon: "🎯", Category: string(CategoryFirstSteps), RequirementType: string(RequirementWins), RequirementValue: 1, ChipsReward: 500, XPReward: 100, Hidden: false},
		{ID: 2, Name: "Getting Started", Description: "Reach 5,000 chips", Icon: "💰", Category: string(CategoryFirstSteps), RequirementType: string(RequirementChips), RequirementValue: 5000, ChipsReward: 1000, XPReward: 250, Hidden: false},
		{ID: 3, Name: "Beginner's Luck", Description: "Win 10 games", Icon: "🍀", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 10, ChipsReward: 1500, XPReward: 500, Hidden: false},
		{ID: 4, Name: "Lucky Streak", Description: "Win 50 games", Icon: "🏅", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 50, ChipsReward: 5000, XPReward: 1500, Hidden: false},
		{ID: 5, Name: "Seasoned Player", Description: "Win 100 games", Icon: "🎲", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 100, ChipsReward: 10000, XPReward: 3000, Hidden: false},
		{ID: 6, Name: "Gambling Master", Description: "Win 500 games", Icon: "👑", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 500, ChipsReward: 25000, XPReward: 7500, Hidden: false},
		{ID: 7, Name: "Small Fortune", Description: "Accumulate 25,000 chips", Icon: "💎", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 25000, ChipsReward: 5000, XPReward: 1000, Hidden: false},
		{ID: 8, Name: "Big Money", Description: "Accumulate 100,000 chips", Icon: "💸", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 100000, ChipsReward: 15000, XPReward: 2500, Hidden: false},
		{ID: 9, Name: "Millionaire", Description: "Accumulate 1,000,000 chips", Icon: "🏰", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 1000000, ChipsReward: 100000, XPReward: 10000, Hidden: false},
		{ID: 10, Name: "Rising Star", Description: "Reach 10,000 total XP", Icon: "⭐", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 10000, ChipsReward: 2500, XPReward: 500, Hidden: false},
		{ID: 11, Name: "Veteran", Description: "Reach 100,000 total XP", Icon: "🏅", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 100000, ChipsReward: 15000, XPReward: 2500, Hidden: false},
		{ID: 12, Name: "Legend", Description: "Reach 500,000 total XP", Icon: "🌟", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 500000, ChipsReward: 50000, XPReward: 7500, Hidden: false},
		{ID: 13, Name: "Fresh Start", Description: "Reach Prestige Level 1", Icon: "🔄", Category: string(CategoryPrestige), RequirementType: string(RequirementPrestige), RequirementValue: 1, ChipsReward: 10000, XPReward: 5000, Hidden: false},
		{ID: 14, Name: "Second Wind", Description: "Reach Prestige Level 3", Icon: "🌪️", Category: string(CategoryPrestige), RequirementType: string(RequirementPrestige), RequirementValue: 3, ChipsReward: 25000, XPReward: 15000, Hidden: false},
		{ID: 15, Name: "Prestige Master", Description: "Reach Prestige Level 5", Icon: "👑", Category: string(CategoryPrestige), RequirementType: string(RequirementPrestige), RequirementValue: 5, ChipsReward: 100000, XPReward: 50000, Hidden: true},
		{ID: 16, Name: "Century Club", Description: "Play 100 total games", Icon: "💯", Category: string(CategoryGaming), RequirementType: string(RequirementGamesPlayed), RequirementValue: 100, ChipsReward: 7500, XPReward: 2000, Hidden: false},
		{ID: 17, Name: "Dedication", Description: "Play 500 total games", Icon: "🎮", Category: string(CategoryGaming), RequirementType: string(RequirementGamesPlayed), RequirementValue: 500, ChipsReward: 20000, XPReward: 5000, Hidden: false},
		{ID: 18, Name: "Addiction", Description: "Play 1,000 total games", Icon: "🎪", Category: string(CategoryGaming), RequirementType: string(RequirementGamesPlayed), RequirementValue: 1000, ChipsReward: 50000, XPReward: 15000, Hidden: false},
		{ID: 19, Name: "Big Winner", Description: "Win 50,000 chips in a single game", Icon: "💸", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 50000, ChipsReward: 25000, XPReward: 10000, Hidden: false},
		{ID: 20, Name: "Whale", Description: "Win 100,000 chips in a single game", Icon: "🐋", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 100000, ChipsReward: 100000, XPReward: 25000, Hidden: false},
		{ID: 21, Name: "Regular Visitor", Description: "Claim 50 daily bonuses", Icon: "📅", Category: string(CategoryLoyalty), RequirementType: string(RequirementDailyBonuses), RequirementValue: 50, ChipsReward: 15000, XPReward: 5000, Hidden: false},
		{ID: 22, Name: "Supporter", Description: "Vote for the bot 25 times", Icon: "💝", Category: string(CategoryLoyalty), RequirementType: string(RequirementVotes), RequirementValue: 25, ChipsReward: 10000, XPReward: 3000, Hidden: false},

		// Additional achievements from CSV to reach the full set
		{ID: 274, Name: "Novice", Description: "Reach 1,000 total XP", Icon: "🥉", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 1000, ChipsReward: 500, XPReward: 100, Hidden: false},
		{ID: 275, Name: "Learner", Description: "Reach 5,000 total XP", Icon: "📚", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 5000, ChipsReward: 1500, XPReward: 300, Hidden: false},
		{ID: 277, Name: "Adept", Description: "Reach 25,000 total XP", Icon: "⚡", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 25000, ChipsReward: 4000, XPReward: 800, Hidden: false},
		{ID: 278, Name: "Skilled", Description: "Reach 50,000 total XP", Icon: "🧠", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 50000, ChipsReward: 6000, XPReward: 1500, Hidden: false},
		{ID: 280, Name: "Elite", Description: "Reach 250,000 total XP", Icon: "💠", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 250000, ChipsReward: 20000, XPReward: 4000, Hidden: false},
		{ID: 282, Name: "Mythic", Description: "Reach 1,000,000 total XP", Icon: "🔮", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 1000000, ChipsReward: 80000, XPReward: 15000, Hidden: false},
		{ID: 283, Name: "Ascendant", Description: "Reach 2,500,000 total XP", Icon: "🚀", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 2500000, ChipsReward: 150000, XPReward: 40000, Hidden: false},
		{ID: 284, Name: "Transcendent", Description: "Reach 5,000,000 total XP", Icon: "🌌", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 5000000, ChipsReward: 300000, XPReward: 80000, Hidden: true},
	}

	am.mutex.Lock()
	defer am.mutex.Unlock()

	for _, achievement := range defaultAchievements {
		achievement.CreatedAt = time.Now()
		am.achievements[achievement.ID] = achievement
	}

}

// saveDefaultAchievements saves default achievements to database
func (am *AchievementManager) saveDefaultAchievements() error {
	if DB == nil {
		return nil
	}

	ctx := context.Background()
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	for _, achievement := range am.achievements {
		query := `
			INSERT INTO achievements (id, name, description, icon, category, requirement_type, 
			                        requirement_value, chips_reward, xp_reward, hidden, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (name) DO NOTHING`

		_, err := DB.Exec(ctx, query,
			achievement.ID,
			achievement.Name,
			achievement.Description,
			achievement.Icon,
			achievement.Category,
			achievement.RequirementType,
			achievement.RequirementValue,
			achievement.ChipsReward,
			achievement.XPReward,
			achievement.Hidden,
			achievement.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert achievement %s: %w", achievement.Name, err)
		}
	}

	return nil
}

// CheckUserAchievements checks if user has earned any new achievements
func (am *AchievementManager) CheckUserAchievements(user *User) ([]*Achievement, error) {
	if DB == nil {
		return nil, nil // Skip achievement checking in offline mode
	}

	// Get user's current achievements
	earnedAchievements, err := am.GetUserAchievements(user.UserID)
	if err != nil {
		return nil, err
	}

	// Create map for quick lookup
	earnedMap := make(map[int]bool)
	for _, ua := range earnedAchievements {
		earnedMap[ua.AchievementID] = true
	}

	var newAchievements []*Achievement

	am.mutex.RLock()
	defer am.mutex.RUnlock()

	// Check each achievement
	for _, achievement := range am.achievements {
		if earnedMap[achievement.ID] {
			continue // Already earned
		}

		if am.checker.Check(user, achievement) {
			// Award the achievement
			if err := am.AwardAchievement(user.UserID, achievement.ID); err != nil {
				continue
			}
			newAchievements = append(newAchievements, achievement)

			// Apply rewards
			if achievement.ChipsReward > 0 || achievement.XPReward > 0 {
				updates := UserUpdateData{
					ChipsIncrement:   achievement.ChipsReward,
					TotalXPIncrement: achievement.XPReward,
				}
				UpdateCachedUser(user.UserID, updates)
			}
		}
	}

	return newAchievements, nil
}

// BatchCheckAchievements checks achievements for multiple users efficiently
func (am *AchievementManager) BatchCheckAchievements(users []*User) (map[int64][]*Achievement, error) {
	if DB == nil || len(users) == 0 {
		return nil, nil
	}

	// Collect user IDs for batch query
	userIDs := make([]int64, len(users))
	userMap := make(map[int64]*User)
	for i, user := range users {
		userIDs[i] = user.UserID
		userMap[user.UserID] = user
	}

	// Batch query for all user achievements
	earnedAchievementsMap, err := am.GetBatchUserAchievements(userIDs)
	if err != nil {
		return nil, err
	}

	results := make(map[int64][]*Achievement)
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	// Process each user
	for _, user := range users {
		earnedAchievements := earnedAchievementsMap[user.UserID]

		// Create map for quick lookup
		earnedMap := make(map[int]bool)
		for _, ua := range earnedAchievements {
			earnedMap[ua.AchievementID] = true
		}

		var newAchievements []*Achievement

		// Check each achievement for this user
		for _, achievement := range am.achievements {
			if earnedMap[achievement.ID] {
				continue // Already earned
			}

			if am.checker.Check(user, achievement) {
				newAchievements = append(newAchievements, achievement)
			}
		}

		if len(newAchievements) > 0 {
			results[user.UserID] = newAchievements
		}
	}

	// Batch award achievements and apply rewards
	if len(results) > 0 {
		am.BatchAwardAchievements(results)
	}

	return results, nil
}

// GetBatchUserAchievements retrieves achievements for multiple users in a single query
func (am *AchievementManager) GetBatchUserAchievements(userIDs []int64) (map[int64][]*UserAchievement, error) {
	if DB == nil || len(userIDs) == 0 {
		return nil, nil
	}

	ctx := context.Background()

	// Build parameterized query for batch operation
	placeholders := make([]string, len(userIDs))
	args := make([]interface{}, len(userIDs))
	for i, userID := range userIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = userID
	}

	query := fmt.Sprintf(`
		SELECT id, user_id, achievement_id, earned_at
		FROM user_achievements 
		WHERE user_id IN (%s)
		ORDER BY user_id, earned_at DESC`,
		strings.Join(placeholders, ","))

	rows, err := DB.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to batch query user achievements: %w", err)
	}
	defer rows.Close()

	results := make(map[int64][]*UserAchievement)
	for rows.Next() {
		var ua UserAchievement
		err := rows.Scan(&ua.ID, &ua.UserID, &ua.AchievementID, &ua.EarnedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user achievement: %w", err)
		}
		results[ua.UserID] = append(results[ua.UserID], &ua)
	}

	return results, nil
}

// BatchAwardAchievements awards achievements to multiple users efficiently
func (am *AchievementManager) BatchAwardAchievements(achievementsByUser map[int64][]*Achievement) error {
	if DB == nil || len(achievementsByUser) == 0 {
		return nil
	}

	ctx := context.Background()

	// Use pgx.Batch for efficient bulk operations
	batch := &pgx.Batch{}
	rewardBatch := &pgx.Batch{}

	for userID, achievements := range achievementsByUser {
		for _, achievement := range achievements {
			// Add achievement award to batch
			batch.Queue(
				"INSERT INTO user_achievements (user_id, achievement_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
				userID, achievement.ID)

			// Add reward application to batch if there are rewards
			if achievement.ChipsReward > 0 || achievement.XPReward > 0 {
				rewardBatch.Queue(
					"UPDATE users SET chips = chips + $1, total_xp = total_xp + $2 WHERE user_id = $3",
					achievement.ChipsReward, achievement.XPReward, userID)
			}
		}
	}

	// Execute achievement awards batch
	if batch.Len() > 0 {
		results := DB.SendBatch(ctx, batch)
		if err := results.Close(); err != nil {
			return fmt.Errorf("failed to batch award achievements: %w", err)
		}
	}

	// Execute rewards batch
	if rewardBatch.Len() > 0 {
		rewardResults := DB.SendBatch(ctx, rewardBatch)
		if err := rewardResults.Close(); err != nil {
			return fmt.Errorf("failed to batch apply achievement rewards: %w", err)
		}
	}

	return nil
}

// GetUserAchievements retrieves all achievements earned by a user
func (am *AchievementManager) GetUserAchievements(userID int64) ([]*UserAchievement, error) {
	if DB == nil {
		return nil, nil
	}

	ctx := context.Background()
	query := `
		SELECT id, user_id, achievement_id, earned_at
		FROM user_achievements 
		WHERE user_id = $1 
		ORDER BY earned_at DESC`

	rows, err := DB.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user achievements: %w", err)
	}
	defer rows.Close()

	var achievements []*UserAchievement
	for rows.Next() {
		var ua UserAchievement
		err := rows.Scan(&ua.ID, &ua.UserID, &ua.AchievementID, &ua.EarnedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user achievement: %w", err)
		}
		achievements = append(achievements, &ua)
	}

	return achievements, nil
}

// AwardAchievement awards an achievement to a user
func (am *AchievementManager) AwardAchievement(userID int64, achievementID int) error {
	if DB == nil {
		return nil
	}

	ctx := context.Background()
	query := `
		INSERT INTO user_achievements (user_id, achievement_id, earned_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, achievement_id) DO NOTHING`

	_, err := DB.Exec(ctx, query, userID, achievementID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to award achievement: %w", err)
	}

	return nil
}

// GetAchievement returns an achievement by ID
func (am *AchievementManager) GetAchievement(id int) *Achievement {
	am.mutex.RLock()
	defer am.mutex.RUnlock()
	return am.achievements[id]
}

// GetAllAchievements returns all achievements
func (am *AchievementManager) GetAllAchievements() []*Achievement {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	achievements := make([]*Achievement, 0, len(am.achievements))
	for _, achievement := range am.achievements {
		achievements = append(achievements, achievement)
	}
	return achievements
}

// GetAchievementsByCategory returns achievements filtered by category
func (am *AchievementManager) GetAchievementsByCategory(category AchievementCategory) []*Achievement {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	var achievements []*Achievement
	for _, achievement := range am.achievements {
		if achievement.Category == string(category) {
			achievements = append(achievements, achievement)
		}
	}
	return achievements
}

// SendAchievementNotification sends an ephemeral notification to the user about achieved achievements
func SendAchievementNotification(session *discordgo.Session, interaction *discordgo.InteractionCreate, achievements []*Achievement) error {
	if len(achievements) == 0 {
		return nil
	}

	embed := CreateAchievementNotificationEmbed(achievements)
	if embed == nil {
		return fmt.Errorf("failed to create achievement notification embed")
	}

	// Send as ephemeral followup message
	params := &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
		Flags:  discordgo.MessageFlagsEphemeral,
	}

	_, err := session.FollowupMessageCreate(interaction.Interaction, true, params)
	if err != nil {
		return fmt.Errorf("failed to send achievement notification: %w", err)
	}

	// Log the notification

	return nil
}
