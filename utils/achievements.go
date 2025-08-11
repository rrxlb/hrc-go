package utils

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
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
	RequirementChips         RequirementType = "chips"
	RequirementWins          RequirementType = "wins"
	RequirementTotalXP       RequirementType = "total_xp"
	RequirementPrestige      RequirementType = "prestige"
	RequirementGamesPlayed   RequirementType = "games_played"
	RequirementDailyBonuses  RequirementType = "daily_bonuses"
	RequirementVotes         RequirementType = "votes"
	RequirementSpecial       RequirementType = "special"
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

	log.Printf("Loaded %d achievements", len(am.achievements))
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
		{ID: 1, Name: "Welcome to the Casino", Description: "Join the casino and start your gambling journey", Icon: "🎰", Category: string(CategoryFirstSteps), RequirementType: string(RequirementSpecial), RequirementValue: 0, ChipsReward: 100, XPReward: 50, Hidden: false},
		{ID: 2, Name: "First Win", Description: "Win your first game", Icon: "🏆", Category: string(CategoryFirstSteps), RequirementType: string(RequirementWins), RequirementValue: 1, ChipsReward: 200, XPReward: 100, Hidden: false},
		{ID: 3, Name: "Getting Started", Description: "Play 5 games", Icon: "🎯", Category: string(CategoryFirstSteps), RequirementType: string(RequirementGamesPlayed), RequirementValue: 5, ChipsReward: 300, XPReward: 150, Hidden: false},
		{ID: 4, Name: "Daily Visitor", Description: "Claim your first daily bonus", Icon: "📅", Category: string(CategoryFirstSteps), RequirementType: string(RequirementDailyBonuses), RequirementValue: 1, ChipsReward: 250, XPReward: 100, Hidden: false},
		{ID: 5, Name: "Voter", Description: "Vote for the bot", Icon: "🗳️", Category: string(CategoryFirstSteps), RequirementType: string(RequirementVotes), RequirementValue: 1, ChipsReward: 500, XPReward: 200, Hidden: false},

		// Wins (6-15)
		{ID: 6, Name: "Lucky Streak", Description: "Win 10 games", Icon: "🍀", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 10, ChipsReward: 1000, XPReward: 500, Hidden: false},
		{ID: 7, Name: "Winner", Description: "Win 25 games", Icon: "🏅", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 25, ChipsReward: 2000, XPReward: 1000, Hidden: false},
		{ID: 8, Name: "Champion", Description: "Win 50 games", Icon: "🏆", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 50, ChipsReward: 5000, XPReward: 2500, Hidden: false},
		{ID: 9, Name: "Dominator", Description: "Win 100 games", Icon: "👑", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 100, ChipsReward: 10000, XPReward: 5000, Hidden: false},
		{ID: 10, Name: "Unstoppable", Description: "Win 250 games", Icon: "🔥", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 250, ChipsReward: 25000, XPReward: 12500, Hidden: false},

		// Wealth (11-20)
		{ID: 11, Name: "First Thousand", Description: "Accumulate 1,000 chips", Icon: "💰", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 1000, ChipsReward: 500, XPReward: 250, Hidden: false},
		{ID: 12, Name: "Five Grand", Description: "Accumulate 5,000 chips", Icon: "💎", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 5000, ChipsReward: 1000, XPReward: 500, Hidden: false},
		{ID: 13, Name: "Ten Thousand Club", Description: "Accumulate 10,000 chips", Icon: "🏦", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 10000, ChipsReward: 2000, XPReward: 1000, Hidden: false},
		{ID: 14, Name: "High Roller", Description: "Accumulate 50,000 chips", Icon: "🎩", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 50000, ChipsReward: 10000, XPReward: 5000, Hidden: false},
		{ID: 15, Name: "Millionaire", Description: "Accumulate 1,000,000 chips", Icon: "🏰", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 1000000, ChipsReward: 100000, XPReward: 50000, Hidden: false},

		// Experience (21-30)
		{ID: 21, Name: "Novice", Description: "Reach 1,000 XP", Icon: "🥉", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 1000, ChipsReward: 500, XPReward: 0, Hidden: false},
		{ID: 22, Name: "Apprentice", Description: "Reach 10,000 XP", Icon: "🥈", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 10000, ChipsReward: 2000, XPReward: 0, Hidden: false},
		{ID: 23, Name: "Gambler", Description: "Reach 40,000 XP", Icon: "🥇", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 40000, ChipsReward: 5000, XPReward: 0, Hidden: false},
		{ID: 24, Name: "Card Shark", Description: "Reach 350,000 XP", Icon: "🦈", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 350000, ChipsReward: 50000, XPReward: 0, Hidden: false},
		{ID: 25, Name: "Legend", Description: "Reach 2,000,000 XP", Icon: "🌟", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 2000000, ChipsReward: 200000, XPReward: 0, Hidden: false},

		// Loyalty (31-40)
		{ID: 31, Name: "Regular", Description: "Claim 7 daily bonuses", Icon: "📆", Category: string(CategoryLoyalty), RequirementType: string(RequirementDailyBonuses), RequirementValue: 7, ChipsReward: 1000, XPReward: 500, Hidden: false},
		{ID: 32, Name: "Dedicated", Description: "Claim 30 daily bonuses", Icon: "🗓️", Category: string(CategoryLoyalty), RequirementType: string(RequirementDailyBonuses), RequirementValue: 30, ChipsReward: 5000, XPReward: 2500, Hidden: false},
		{ID: 33, Name: "Loyal Customer", Description: "Claim 100 daily bonuses", Icon: "🎖️", Category: string(CategoryLoyalty), RequirementType: string(RequirementDailyBonuses), RequirementValue: 100, ChipsReward: 15000, XPReward: 7500, Hidden: false},
		{ID: 34, Name: "Supporter", Description: "Vote 10 times", Icon: "❤️", Category: string(CategoryLoyalty), RequirementType: string(RequirementVotes), RequirementValue: 10, ChipsReward: 5000, XPReward: 2500, Hidden: false},
		{ID: 35, Name: "True Fan", Description: "Vote 50 times", Icon: "⭐", Category: string(CategoryLoyalty), RequirementType: string(RequirementVotes), RequirementValue: 50, ChipsReward: 25000, XPReward: 12500, Hidden: false},

		// Gaming (41-50)
		{ID: 41, Name: "Casual Player", Description: "Play 50 games", Icon: "🎮", Category: string(CategoryGaming), RequirementType: string(RequirementGamesPlayed), RequirementValue: 50, ChipsReward: 2500, XPReward: 1250, Hidden: false},
		{ID: 42, Name: "Active Gambler", Description: "Play 200 games", Icon: "🎯", Category: string(CategoryGaming), RequirementType: string(RequirementGamesPlayed), RequirementValue: 200, ChipsReward: 7500, XPReward: 3750, Hidden: false},
		{ID: 43, Name: "Veteran", Description: "Play 500 games", Icon: "🎪", Category: string(CategoryGaming), RequirementType: string(RequirementGamesPlayed), RequirementValue: 500, ChipsReward: 20000, XPReward: 10000, Hidden: false},
		{ID: 44, Name: "Addict", Description: "Play 1000 games", Icon: "🎡", Category: string(CategoryGaming), RequirementType: string(RequirementGamesPlayed), RequirementValue: 1000, ChipsReward: 50000, XPReward: 25000, Hidden: false},

		// Special Hidden Achievements (51-52)
		{ID: 51, Name: "Lucky Number", Description: "Win with exactly 777 chips", Icon: "🍀", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 777, ChipsReward: 7777, XPReward: 3888, Hidden: true},
		{ID: 52, Name: "High Stakes", Description: "Win a bet of 100,000+ chips", Icon: "💸", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 100000, ChipsReward: 200000, XPReward: 100000, Hidden: true},
	}

	am.mutex.Lock()
	defer am.mutex.Unlock()

	for _, achievement := range defaultAchievements {
		achievement.CreatedAt = time.Now()
		am.achievements[achievement.ID] = achievement
	}

	log.Printf("Loaded %d default achievements", len(defaultAchievements))
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

	log.Printf("Saved %d achievements to database", len(am.achievements))
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
				log.Printf("Failed to award achievement %s to user %d: %v", achievement.Name, user.UserID, err)
				continue
			}
			newAchievements = append(newAchievements, achievement)

			// Apply rewards
			if achievement.ChipsReward > 0 || achievement.XPReward > 0 {
				updates := UserUpdateData{
					ChipsIncrement:   achievement.ChipsReward,
					TotalXPIncrement: achievement.XPReward,
				}
				if _, err := UpdateCachedUser(user.UserID, updates); err != nil {
					log.Printf("Failed to apply achievement rewards for %s: %v", achievement.Name, err)
				}
			}
		}
	}

	return newAchievements, nil
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

	am.mutex.RLock()
	achievement := am.achievements[achievementID]
	am.mutex.RUnlock()

	if achievement != nil {
		log.Printf("Awarded achievement '%s' to user %d", achievement.Name, userID)
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