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
	if err := AchievementMgr.LoadAchievements(); err != nil {
		return err
	}
	
	// Refresh achievements to ensure database has latest reward values
	return AchievementMgr.RefreshAchievementsFromDefaults()
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
		// First Steps (1-5) - Small rewards to get started without snowballing
		{ID: 1, Name: "First Blood", Description: "Win your first game", Icon: "🎯", Category: string(CategoryFirstSteps), RequirementType: string(RequirementWins), RequirementValue: 1, ChipsReward: 50, XPReward: 25, Hidden: false},
		{ID: 2, Name: "Getting Started", Description: "Reach 7,500 chips", Icon: "💰", Category: string(CategoryFirstSteps), RequirementType: string(RequirementChips), RequirementValue: 7500, ChipsReward: 150, XPReward: 50, Hidden: false},
		{ID: 3, Name: "Beginner's Luck", Description: "Win 10 games", Icon: "🍀", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 10, ChipsReward: 200, XPReward: 75, Hidden: false},
		{ID: 4, Name: "Lucky Streak", Description: "Win 50 games", Icon: "🏅", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 75, ChipsReward: 800, XPReward: 200, Hidden: false},
		{ID: 5, Name: "Seasoned Player", Description: "Win 100 games", Icon: "<:dicesixfacesthree:1396630430136139907>", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 150, ChipsReward: 1500, XPReward: 400, Hidden: false},
		{ID: 6, Name: "Gambling Master", Description: "Win 500 games", Icon: "👑", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 750, ChipsReward: 5000, XPReward: 1000, Hidden: false},
		{ID: 7, Name: "Small Fortune", Description: "Accumulate 35,000 chips", Icon: "💎", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 35000, ChipsReward: 750, XPReward: 150, Hidden: false},
		{ID: 8, Name: "Big Money", Description: "Accumulate 150,000 chips", Icon: "💸", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 150000, ChipsReward: 2500, XPReward: 500, Hidden: false},
		{ID: 9, Name: "Millionaire", Description: "Accumulate 1,000,000 chips", Icon: "🏰", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 1000000, ChipsReward: 15000, XPReward: 2500, Hidden: false},
		{ID: 10, Name: "Rising Star", Description: "Reach 10,000 total XP", Icon: "⭐", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 10000, ChipsReward: 1000, XPReward: 300, Hidden: false},
		{ID: 11, Name: "Veteran", Description: "Reach 100,000 total XP", Icon: "🏅", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 100000, ChipsReward: 8000, XPReward: 1500, Hidden: false},
		{ID: 12, Name: "Legend", Description: "Reach 500,000 total XP", Icon: "🌟", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 500000, ChipsReward: 25000, XPReward: 5000, Hidden: false},
		{ID: 13, Name: "Fresh Start", Description: "Reach Prestige Level 1", Icon: "🔄", Category: string(CategoryPrestige), RequirementType: string(RequirementPrestige), RequirementValue: 1, ChipsReward: 2500, XPReward: 500, Hidden: false},
		{ID: 14, Name: "Second Wind", Description: "Reach Prestige Level 3", Icon: "🌪️", Category: string(CategoryPrestige), RequirementType: string(RequirementPrestige), RequirementValue: 3, ChipsReward: 6000, XPReward: 1200, Hidden: false},
		{ID: 15, Name: "Prestige Master", Description: "Reach Prestige Level 5", Icon: "👑", Category: string(CategoryPrestige), RequirementType: string(RequirementPrestige), RequirementValue: 5, ChipsReward: 12000, XPReward: 2500, Hidden: true},
		{ID: 16, Name: "Century Club", Description: "Play 100 total games", Icon: "💯", Category: string(CategoryGaming), RequirementType: string(RequirementGamesPlayed), RequirementValue: 150, ChipsReward: 1200, XPReward: 300, Hidden: false},
		{ID: 17, Name: "Dedication", Description: "Play 500 total games", Icon: "🎮", Category: string(CategoryGaming), RequirementType: string(RequirementGamesPlayed), RequirementValue: 750, ChipsReward: 4000, XPReward: 800, Hidden: false},
		{ID: 18, Name: "Addiction", Description: "Play 1,000 total games", Icon: "🎪", Category: string(CategoryGaming), RequirementType: string(RequirementGamesPlayed), RequirementValue: 1000, ChipsReward: 8000, XPReward: 1500, Hidden: false},
		{ID: 19, Name: "Big Winner", Description: "Win 50,000 chips in a single game", Icon: "💸", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 50000, ChipsReward: 5000, XPReward: 1000, Hidden: false},
		{ID: 20, Name: "Whale", Description: "Win 100,000 chips in a single game", Icon: "🐋", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 100000, ChipsReward: 12000, XPReward: 2000, Hidden: false},
		{ID: 21, Name: "Regular Visitor", Description: "Claim 50 daily bonuses", Icon: "📅", Category: string(CategoryLoyalty), RequirementType: string(RequirementDailyBonuses), RequirementValue: 50, ChipsReward: 3000, XPReward: 600, Hidden: false},
		{ID: 22, Name: "Supporter", Description: "Vote for the bot 25 times", Icon: "💝", Category: string(CategoryLoyalty), RequirementType: string(RequirementVotes), RequirementValue: 25, ChipsReward: 2000, XPReward: 400, Hidden: false},

		// Additional early milestone achievements
		{ID: 61, Name: "Baby Steps", Description: "Accumulate 2,500 chips", Icon: "👶", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 2500, ChipsReward: 50, XPReward: 25, Hidden: false},
		{ID: 62, Name: "Pocket Money", Description: "Accumulate 15,000 chips", Icon: "🪙", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 15000, ChipsReward: 300, XPReward: 100, Hidden: false},

		// Additional achievements from CSV to reach the full set
		{ID: 274, Name: "Novice", Description: "Reach 1,000 total XP", Icon: "🥉", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 1000, ChipsReward: 100, XPReward: 50, Hidden: false},
		{ID: 275, Name: "Learner", Description: "Reach 5,000 total XP", Icon: "📚", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 5000, ChipsReward: 300, XPReward: 150, Hidden: false},
		{ID: 277, Name: "Adept", Description: "Reach 25,000 total XP", Icon: "⚡", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 25000, ChipsReward: 1200, XPReward: 600, Hidden: false},
		{ID: 278, Name: "Skilled", Description: "Reach 50,000 total XP", Icon: "🧠", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 50000, ChipsReward: 2000, XPReward: 1000, Hidden: false},
		{ID: 280, Name: "Elite", Description: "Reach 250,000 total XP", Icon: "💠", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 250000, ChipsReward: 10000, XPReward: 3000, Hidden: false},
		{ID: 282, Name: "Mythic", Description: "Reach 1,000,000 total XP", Icon: "🔮", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 1000000, ChipsReward: 30000, XPReward: 8000, Hidden: false},
		{ID: 283, Name: "Ascendant", Description: "Reach 2,500,000 total XP", Icon: "🚀", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 2500000, ChipsReward: 60000, XPReward: 15000, Hidden: false},
		{ID: 284, Name: "Transcendent", Description: "Reach 5,000,000 total XP", Icon: "🌌", Category: string(CategoryExperience), RequirementType: string(RequirementTotalXP), RequirementValue: 5000000, ChipsReward: 100000, XPReward: 25000, Hidden: true},

		// First Steps - Additional beginner achievements
		{ID: 23, Name: "Welcome Bonus", Description: "Claim your first hourly bonus", Icon: "⏰", Category: string(CategoryFirstSteps), RequirementType: string(RequirementDailyBonuses), RequirementValue: 1, ChipsReward: 25, XPReward: 15, Hidden: false},
		{ID: 24, Name: "Early Bird", Description: "Play your first game within 1 hour of joining", Icon: "🐦", Category: string(CategoryFirstSteps), RequirementType: string(RequirementSpecial), RequirementValue: 1, ChipsReward: 50, XPReward: 25, Hidden: false},
		{ID: 25, Name: "Curious Cat", Description: "Check your balance for the first time", Icon: "🐱", Category: string(CategoryFirstSteps), RequirementType: string(RequirementSpecial), RequirementValue: 1, ChipsReward: 25, XPReward: 10, Hidden: false},

		// Wins - More win-based achievements
		{ID: 26, Name: "Hot Streak", Description: "Win 25 games", Icon: "🔥", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 40, ChipsReward: 500, XPReward: 150, Hidden: false},
		{ID: 27, Name: "Unstoppable Force", Description: "Win 250 games", Icon: "⚡", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 375, ChipsReward: 3500, XPReward: 800, Hidden: false},
		{ID: 28, Name: "Casino Royale", Description: "Win 1,000 games", Icon: "🃏", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 1000, ChipsReward: 12000, XPReward: 2500, Hidden: false},
		{ID: 29, Name: "Living Legend", Description: "Win 2,500 games", Icon: "🏛️", Category: string(CategoryWins), RequirementType: string(RequirementWins), RequirementValue: 2500, ChipsReward: 25000, XPReward: 5000, Hidden: false},

		// Wealth - More chip-based milestones
		{ID: 30, Name: "Pocket Change", Description: "Accumulate 1,000 chips", Icon: "🪙", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 1000, ChipsReward: 25, XPReward: 15, Hidden: false},
		{ID: 31, Name: "Shopping Spree", Description: "Accumulate 10,000 chips", Icon: "🛍️", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 10000, ChipsReward: 200, XPReward: 75, Hidden: false},
		{ID: 32, Name: "High Roller", Description: "Accumulate 500,000 chips", Icon: "🎩", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 500000, ChipsReward: 8000, XPReward: 1500, Hidden: false},
		{ID: 33, Name: "Billionaire Club", Description: "Accumulate 10,000,000 chips", Icon: "🏦", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 10000000, ChipsReward: 80000, XPReward: 15000, Hidden: false},
		{ID: 34, Name: "Dragon's Hoard", Description: "Accumulate 100,000,000 chips", Icon: "🐉", Category: string(CategoryWealth), RequirementType: string(RequirementChips), RequirementValue: 100000000, ChipsReward: 200000, XPReward: 40000, Hidden: true},

		// Gaming - Play frequency achievements
		{ID: 35, Name: "Casual Gambler", Description: "Play 25 total games", Icon: "<:dicesixfacesfive:1396630450667262138>", Category: string(CategoryGaming), RequirementType: string(RequirementGamesPlayed), RequirementValue: 25, ChipsReward: 300, XPReward: 100, Hidden: false},
		{ID: 36, Name: "Weekend Warrior", Description: "Play 250 total games", Icon: "⚔️", Category: string(CategoryGaming), RequirementType: string(RequirementGamesPlayed), RequirementValue: 250, ChipsReward: 3000, XPReward: 700, Hidden: false},
		{ID: 37, Name: "No Life", Description: "Play 5,000 total games", Icon: "💀", Category: string(CategoryGaming), RequirementType: string(RequirementGamesPlayed), RequirementValue: 5000, ChipsReward: 35000, XPReward: 8000, Hidden: false},
		{ID: 38, Name: "Eternal Player", Description: "Play 10,000 total games", Icon: "♾️", Category: string(CategoryGaming), RequirementType: string(RequirementGamesPlayed), RequirementValue: 10000, ChipsReward: 75000, XPReward: 18000, Hidden: true},

		// Loyalty - Community engagement
		{ID: 39, Name: "Daily Grinder", Description: "Claim 7 daily bonuses", Icon: "⚙️", Category: string(CategoryLoyalty), RequirementType: string(RequirementDailyBonuses), RequirementValue: 7, ChipsReward: 500, XPReward: 150, Hidden: false},
		{ID: 40, Name: "Dedicated Member", Description: "Claim 100 daily bonuses", Icon: "🏅", Category: string(CategoryLoyalty), RequirementType: string(RequirementDailyBonuses), RequirementValue: 100, ChipsReward: 6000, XPReward: 1200, Hidden: false},
		{ID: 41, Name: "Cult Member", Description: "Claim 365 daily bonuses", Icon: "🗓️", Category: string(CategoryLoyalty), RequirementType: string(RequirementDailyBonuses), RequirementValue: 365, ChipsReward: 20000, XPReward: 4000, Hidden: false},
		{ID: 42, Name: "True Believer", Description: "Vote for the bot 100 times", Icon: "🙏", Category: string(CategoryLoyalty), RequirementType: string(RequirementVotes), RequirementValue: 100, ChipsReward: 15000, XPReward: 3000, Hidden: false},

		// Special - Hidden fun achievements
		{ID: 43, Name: "Lucky 7s", Description: "Win exactly 777 chips in a single game", Icon: "🍀", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 777, ChipsReward: 1500, XPReward: 300, Hidden: true},
		{ID: 44, Name: "Jackpot Hunter", Description: "Hit any jackpot or max win", Icon: "🎰", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 1, ChipsReward: 10000, XPReward: 2000, Hidden: true},
		{ID: 45, Name: "Degen Gambler", Description: "Lose 50 games in a row", Icon: "📉", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 50, ChipsReward: 2000, XPReward: 400, Hidden: true},
		{ID: 46, Name: "Bankruptcy Expert", Description: "Go broke 10 times", Icon: "💸", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 10, ChipsReward: 3000, XPReward: 600, Hidden: true},
		{ID: 47, Name: "Double or Nothing", Description: "Double your chips in a single game", Icon: "🔁", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 1, ChipsReward: 4000, XPReward: 800, Hidden: true},
		{ID: 48, Name: "All In", Description: "Bet all your chips and win", Icon: "🎯", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 1, ChipsReward: 6000, XPReward: 1200, Hidden: true},
		{ID: 49, Name: "House Always Wins", Description: "Lose 1,000 games", Icon: "🏠", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 1000, ChipsReward: 15000, XPReward: 3000, Hidden: true},
		{ID: 50, Name: "Miracle Worker", Description: "Win when you had less than 100 chips", Icon: "✨", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 1, ChipsReward: 1500, XPReward: 300, Hidden: true},

		// Prestige - More prestige levels
		{ID: 51, Name: "Renaissance", Description: "Reach Prestige Level 10", Icon: "🎭", Category: string(CategoryPrestige), RequirementType: string(RequirementPrestige), RequirementValue: 10, ChipsReward: 30000, XPReward: 6000, Hidden: true},
		{ID: 52, Name: "Ascension", Description: "Reach Prestige Level 25", Icon: "👼", Category: string(CategoryPrestige), RequirementType: string(RequirementPrestige), RequirementValue: 25, ChipsReward: 75000, XPReward: 15000, Hidden: true},

		// Time-based achievements (Special category)
		{ID: 53, Name: "Night Owl", Description: "Play a game after midnight", Icon: "🦉", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 1, ChipsReward: 300, XPReward: 100, Hidden: true},
		{ID: 54, Name: "Early Riser", Description: "Play a game before 6 AM", Icon: "🌅", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 1, ChipsReward: 300, XPReward: 100, Hidden: true},
		{ID: 55, Name: "Marathon Session", Description: "Play for 6 hours straight", Icon: "🏃", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 1, ChipsReward: 5000, XPReward: 1000, Hidden: true},

		// Game-specific achievements (if you have specific games)
		{ID: 56, Name: "Blackjack Master", Description: "Win 100 blackjack games", Icon: "🃏", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 100, ChipsReward: 4000, XPReward: 800, Hidden: false},
		{ID: 57, Name: "Slot Machine Addict", Description: "Play slots 500 times", Icon: "🎰", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 500, ChipsReward: 5000, XPReward: 1000, Hidden: false},
		{ID: 58, Name: "Roulette Roller", Description: "Play roulette 200 times", Icon: "🎡", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 200, ChipsReward: 3000, XPReward: 600, Hidden: false},

		// Social achievements
		{ID: 59, Name: "Show Off", Description: "Use profile command 50 times", Icon: "🤳", Category: string(CategorySpecial), RequirementType: string(RequirementSpecial), RequirementValue: 50, ChipsReward: 1500, XPReward: 300, Hidden: true},
		{ID: 60, Name: "Helping Hand", Description: "Help another player (future feature)", Icon: "🤝", Category: string(CategoryLoyalty), RequirementType: string(RequirementSpecial), RequirementValue: 1, ChipsReward: 2500, XPReward: 500, Hidden: true},
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
			ON CONFLICT (name) DO UPDATE SET
				description = EXCLUDED.description,
				icon = EXCLUDED.icon,
				category = EXCLUDED.category,
				requirement_type = EXCLUDED.requirement_type,
				requirement_value = EXCLUDED.requirement_value,
				chips_reward = EXCLUDED.chips_reward,
				xp_reward = EXCLUDED.xp_reward,
				hidden = EXCLUDED.hidden`

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

// RefreshAchievementsFromDefaults forces a refresh of achievements from default values
// This is useful when achievement rewards have been updated in code
func (am *AchievementManager) RefreshAchievementsFromDefaults() error {
	if DB == nil {
		// Just reload in-memory for offline mode
		am.loadDefaultAchievements()
		return nil
	}

	// Load default achievements into memory
	am.loadDefaultAchievements()
	
	// Save them to database (will update existing ones due to ON CONFLICT DO UPDATE)
	return am.saveDefaultAchievements()
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
	if len(achievements) == 1 {
		fmt.Printf("Achievement unlocked for user: %s - %s\n", getInteractionUserID(interaction), achievements[0].Name)
	} else {
		fmt.Printf("Multiple achievements (%d) unlocked for user: %s\n", len(achievements), getInteractionUserID(interaction))
	}

	return nil
}

// SendAchievementNotificationDM sends achievement notification via DM (fallback when no interaction)
func SendAchievementNotificationDM(session *discordgo.Session, userID string, achievements []*Achievement) error {
	if len(achievements) == 0 {
		return nil
	}

	embed := CreateAchievementNotificationEmbed(achievements)
	if embed == nil {
		return fmt.Errorf("failed to create achievement notification embed")
	}

	// Try to create DM channel with user
	dmChannel, err := session.UserChannelCreate(userID)
	if err != nil {
		return fmt.Errorf("failed to create DM channel: %w", err)
	}

	// Send the embed
	_, err = session.ChannelMessageSendEmbed(dmChannel.ID, embed)
	if err != nil {
		return fmt.Errorf("failed to send DM notification: %w", err)
	}

	// Log the notification
	if len(achievements) == 1 {
		fmt.Printf("Achievement DM sent to user: %s - %s\n", userID, achievements[0].Name)
	} else {
		fmt.Printf("Multiple achievements DM (%d) sent to user: %s\n", len(achievements), userID)
	}

	return nil
}

// Helper function to get user ID from interaction
func getInteractionUserID(interaction *discordgo.InteractionCreate) string {
	if interaction.Member != nil && interaction.Member.User != nil {
		return interaction.Member.User.ID
	}
	if interaction.User != nil {
		return interaction.User.ID
	}
	return "unknown"
}
