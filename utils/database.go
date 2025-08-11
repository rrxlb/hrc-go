package utils

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	UserID               int64
	Chips                int64
	TotalXP              int64
	CurrentXP            int64
	Prestige             int
	Wins                 int
	Losses               int
	DailyBonusesClaimed  int
	VotesCount           int
	LastHourly           *time.Time
	LastDaily            *time.Time
	LastWeekly           *time.Time
	LastVote             *time.Time
	LastBonus            *time.Time
	PremiumSettings      JSONB
}

type UserUpdateData struct {
	ChipsIncrement              int64
	TotalXPIncrement            int64
	CurrentXPIncrement          int64
	WinsIncrement               int
	LossesIncrement             int
	DailyBonusesClaimedIncrement int
	VotesCountIncrement         int
	Prestige                    *int
	LastHourly                  *time.Time
	LastDaily                   *time.Time
	LastWeekly                  *time.Time
	LastVote                    *time.Time
	LastBonus                   *time.Time
	PremiumSettings             JSONB
}

type Achievement struct {
	ID               int
	Name             string
	Description      string
	Icon             string
	Category         string
	RequirementType  string
	RequirementValue int64
	ChipsReward      int64
	XPReward         int64
	Hidden           bool
	CreatedAt        time.Time
}

type UserAchievement struct {
	ID            int
	UserID        int64
	AchievementID int
	EarnedAt      time.Time
}

type Jackpot struct {
	ID     int
	Amount int64
}

var DB *pgxpool.Pool

// JSONB type for PostgreSQL JSONB handling
type JSONB map[string]interface{}

func (j JSONB) Value() (driver.Value, error) {
	return json.Marshal(j)
}

func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into JSONB", value)
	}
	
	return json.Unmarshal(bytes, j)
}

// SetupDatabase initializes the database connection pool
func SetupDatabase() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Println("DATABASE_URL not set - database features disabled")
		return nil
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Optimize connection pool settings
	config.MinConns = 2
	config.MaxConns = 10
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 5 * time.Minute

	DB, err = pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return fmt.Errorf("failed to create database pool: %w", err)
	}

	// Test the connection
	if err := DB.Ping(context.Background()); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Create tables if they don't exist
	if err := createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	log.Printf("Database pool created successfully with %d-%d connections", 
		config.MinConns, config.MaxConns)
	return nil
}

// CloseDatabase closes the database connection pool
func CloseDatabase() {
	if DB != nil {
		DB.Close()
		log.Println("Database connection pool closed")
	}
}

// createTables creates all necessary database tables to match existing schema
func createTables() error {
	ctx := context.Background()
	
	// Users table - matching your existing Python bot schema
	usersTable := `
	CREATE TABLE IF NOT EXISTS users (
		user_id BIGINT PRIMARY KEY,
		chips BIGINT NOT NULL DEFAULT 1000,
		total_xp BIGINT NOT NULL DEFAULT 0,
		current_xp BIGINT NOT NULL DEFAULT 0,
		prestige INTEGER NOT NULL DEFAULT 0,
		wins INTEGER NOT NULL DEFAULT 0,
		losses INTEGER NOT NULL DEFAULT 0,
		daily_bonuses_claimed INTEGER NOT NULL DEFAULT 0,
		votes_count INTEGER NOT NULL DEFAULT 0,
		last_hourly TIMESTAMPTZ,
		last_daily TIMESTAMPTZ,
		last_weekly TIMESTAMPTZ,
		last_vote TIMESTAMPTZ,
		last_bonus TIMESTAMPTZ,
		premium_settings JSONB
	)`

	// Achievements table
	achievementsTable := `
	CREATE TABLE IF NOT EXISTS achievements (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL UNIQUE,
		description TEXT NOT NULL,
		icon VARCHAR(50) NOT NULL,
		category VARCHAR(50) NOT NULL,
		requirement_type VARCHAR(50) NOT NULL,
		requirement_value BIGINT NOT NULL DEFAULT 0,
		chips_reward BIGINT NOT NULL DEFAULT 0,
		xp_reward BIGINT NOT NULL DEFAULT 0,
		hidden BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`

	// User achievements table
	userAchievementsTable := `
	CREATE TABLE IF NOT EXISTS user_achievements (
		id SERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
		achievement_id INTEGER NOT NULL REFERENCES achievements(id) ON DELETE CASCADE,
		earned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(user_id, achievement_id)
	)`

	// Jackpots table
	jackpotsTable := `
	CREATE TABLE IF NOT EXISTS jackpots (
		id SMALLINT PRIMARY KEY DEFAULT 1,
		amount BIGINT NOT NULL
	)`

	// Execute table creation
	tables := []string{usersTable, achievementsTable, userAchievementsTable, jackpotsTable}
	for _, table := range tables {
		if _, err := DB.Exec(ctx, table); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	// Create indexes for performance
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS users_chips_idx ON users (chips DESC)",
		"CREATE INDEX IF NOT EXISTS users_total_xp_idx ON users (total_xp DESC)",
		"CREATE INDEX IF NOT EXISTS users_prestige_idx ON users (prestige DESC, total_xp DESC)",
		"CREATE INDEX IF NOT EXISTS user_achievements_user_id_idx ON user_achievements (user_id)",
		"CREATE INDEX IF NOT EXISTS user_achievements_achievement_id_idx ON user_achievements (achievement_id)",
		"CREATE INDEX IF NOT EXISTS user_achievements_earned_at_idx ON user_achievements (earned_at DESC)",
	}

	for _, index := range indexes {
		if _, err := DB.Exec(ctx, index); err != nil {
			log.Printf("Warning: failed to create index: %v", err)
		}
	}

	log.Println("Database tables created/verified successfully")
	return nil
}

// GetUser retrieves user data from the database, creating if doesn't exist
func GetUser(userID int64) (*User, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not connected")
	}

	ctx := context.Background()
	
	user := &User{}
	query := `
	SELECT user_id, chips, total_xp, current_xp, prestige, wins, losses,
	       daily_bonuses_claimed, votes_count, last_hourly, last_daily, 
	       last_weekly, last_vote, last_bonus, premium_settings
	FROM users WHERE user_id = $1`
	
	err := DB.QueryRow(ctx, query, userID).Scan(
		&user.UserID, &user.Chips, &user.TotalXP, &user.CurrentXP, &user.Prestige,
		&user.Wins, &user.Losses, &user.DailyBonusesClaimed, &user.VotesCount,
		&user.LastHourly, &user.LastDaily, &user.LastWeekly, &user.LastVote,
		&user.LastBonus, &user.PremiumSettings,
	)
	
	if err != nil {
		if err == pgx.ErrNoRows {
			// Create new user if doesn't exist
			return CreateUser(userID)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	
	return user, nil
}

// CreateUser creates a new user in the database with default values
func CreateUser(userID int64) (*User, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not connected")
	}

	ctx := context.Background()
	
	user := &User{
		UserID:              userID,
		Chips:               1000,
		TotalXP:             0,
		CurrentXP:           0,
		Prestige:            0,
		Wins:                0,
		Losses:              0,
		DailyBonusesClaimed: 0,
		VotesCount:          0,
		PremiumSettings:     JSONB{},
	}
	
	query := `
	INSERT INTO users (user_id, chips, total_xp, current_xp, prestige, wins, losses,
	                  daily_bonuses_claimed, votes_count, premium_settings)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	ON CONFLICT (user_id) DO NOTHING
	RETURNING user_id, chips, total_xp, current_xp, prestige, wins, losses,
	          daily_bonuses_claimed, votes_count, last_hourly, last_daily, 
	          last_weekly, last_vote, last_bonus, premium_settings`
	
	err := DB.QueryRow(ctx, query,
		user.UserID, user.Chips, user.TotalXP, user.CurrentXP, user.Prestige,
		user.Wins, user.Losses, user.DailyBonusesClaimed, user.VotesCount,
		user.PremiumSettings,
	).Scan(
		&user.UserID, &user.Chips, &user.TotalXP, &user.CurrentXP, &user.Prestige,
		&user.Wins, &user.Losses, &user.DailyBonusesClaimed, &user.VotesCount,
		&user.LastHourly, &user.LastDaily, &user.LastWeekly, &user.LastVote,
		&user.LastBonus, &user.PremiumSettings,
	)
	
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	
	return user, nil
}

// UpdateUser updates user data in the database with dynamic field updates
func UpdateUser(userID int64, updates UserUpdateData) (*User, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not connected")
	}

	ctx := context.Background()
	
	// Build dynamic query based on provided updates
	setParts := []string{}
	args := []interface{}{userID}
	argIndex := 2
	
	if updates.ChipsIncrement != 0 {
		setParts = append(setParts, fmt.Sprintf("chips = chips + $%d", argIndex))
		args = append(args, updates.ChipsIncrement)
		argIndex++
	}
	
	if updates.TotalXPIncrement != 0 {
		setParts = append(setParts, fmt.Sprintf("total_xp = total_xp + $%d", argIndex))
		args = append(args, updates.TotalXPIncrement)
		argIndex++
	}
	
	if updates.CurrentXPIncrement != 0 {
		setParts = append(setParts, fmt.Sprintf("current_xp = current_xp + $%d", argIndex))
		args = append(args, updates.CurrentXPIncrement)
		argIndex++
	}
	
	if updates.WinsIncrement != 0 {
		setParts = append(setParts, fmt.Sprintf("wins = wins + $%d", argIndex))
		args = append(args, updates.WinsIncrement)
		argIndex++
	}
	
	if updates.LossesIncrement != 0 {
		setParts = append(setParts, fmt.Sprintf("losses = losses + $%d", argIndex))
		args = append(args, updates.LossesIncrement)
		argIndex++
	}
	
	if updates.DailyBonusesClaimedIncrement != 0 {
		setParts = append(setParts, fmt.Sprintf("daily_bonuses_claimed = daily_bonuses_claimed + $%d", argIndex))
		args = append(args, updates.DailyBonusesClaimedIncrement)
		argIndex++
	}
	
	if updates.VotesCountIncrement != 0 {
		setParts = append(setParts, fmt.Sprintf("votes_count = votes_count + $%d", argIndex))
		args = append(args, updates.VotesCountIncrement)
		argIndex++
	}
	
	if updates.Prestige != nil {
		setParts = append(setParts, fmt.Sprintf("prestige = $%d", argIndex))
		args = append(args, *updates.Prestige)
		argIndex++
	}
	
	if updates.LastHourly != nil {
		setParts = append(setParts, fmt.Sprintf("last_hourly = $%d", argIndex))
		args = append(args, *updates.LastHourly)
		argIndex++
	}
	
	if updates.LastDaily != nil {
		setParts = append(setParts, fmt.Sprintf("last_daily = $%d", argIndex))
		args = append(args, *updates.LastDaily)
		argIndex++
	}
	
	if updates.LastWeekly != nil {
		setParts = append(setParts, fmt.Sprintf("last_weekly = $%d", argIndex))
		args = append(args, *updates.LastWeekly)
		argIndex++
	}
	
	if updates.LastVote != nil {
		setParts = append(setParts, fmt.Sprintf("last_vote = $%d", argIndex))
		args = append(args, *updates.LastVote)
		argIndex++
	}
	
	if updates.LastBonus != nil {
		setParts = append(setParts, fmt.Sprintf("last_bonus = $%d", argIndex))
		args = append(args, *updates.LastBonus)
		argIndex++
	}
	
	if updates.PremiumSettings != nil {
		setParts = append(setParts, fmt.Sprintf("premium_settings = $%d", argIndex))
		args = append(args, updates.PremiumSettings)
		argIndex++
	}

	if len(setParts) == 0 {
		return GetUser(userID) // No updates to make, just return current user
	}
	
	query := fmt.Sprintf(`
		UPDATE users SET %s WHERE user_id = $1
		RETURNING user_id, chips, total_xp, current_xp, prestige, wins, losses,
		          daily_bonuses_claimed, votes_count, last_hourly, last_daily, 
		          last_weekly, last_vote, last_bonus, premium_settings`,
		strings.Join(setParts, ", "))
	
	user := &User{}
	err := DB.QueryRow(ctx, query, args...).Scan(
		&user.UserID, &user.Chips, &user.TotalXP, &user.CurrentXP, &user.Prestige,
		&user.Wins, &user.Losses, &user.DailyBonusesClaimed, &user.VotesCount,
		&user.LastHourly, &user.LastDaily, &user.LastWeekly, &user.LastVote,
		&user.LastBonus, &user.PremiumSettings,
	)
	
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}
	
	return user, nil
}

// ParseBet parses a bet string and validates it
func ParseBet(betStr string, userChips int64) (int64, error) {
	betStr = strings.TrimSpace(strings.ToLower(betStr))
	
	// Handle special cases
	switch betStr {
	case "all", "allin":
		return userChips, nil
	case "half":
		return userChips / 2, nil
	case "max":
		return userChips, nil
	}
	
	// Handle percentage
	if strings.HasSuffix(betStr, "%") {
		percentStr := strings.TrimSuffix(betStr, "%")
		percent, err := strconv.ParseFloat(percentStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid percentage: %s", betStr)
		}
		if percent < 0 || percent > 100 {
			return 0, fmt.Errorf("percentage must be between 0 and 100")
		}
		return int64(float64(userChips) * percent / 100), nil
	}
	
	// Handle multiplier suffixes
	multiplier := int64(1)
	if strings.HasSuffix(betStr, "k") {
		multiplier = 1000
		betStr = strings.TrimSuffix(betStr, "k")
	} else if strings.HasSuffix(betStr, "m") {
		multiplier = 1000000
		betStr = strings.TrimSuffix(betStr, "m")
	}
	
	// Parse the number
	bet, err := strconv.ParseInt(betStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid bet amount: %s", betStr)
	}
	
	return bet * multiplier, nil
}

// GetRank returns the user's rank based on total XP
func GetRank(totalXP int64) (string, string, int, int64) {
	ranks := []struct {
		Name       string
		Icon       string
		XPRequired int64
		Color      int
	}{
		{"Novice", "🥉", 0, 0xcd7f32},
		{"Apprentice", "🥈", 10000, 0xc0c0c0},
		{"Gambler", "🥇", 40000, 0xffd700},
		{"High Roller", "💰", 125000, 0x22a7f0},
		{"Card Shark", "🦈", 350000, 0x1f3a93},
		{"Pit Boss", "👑", 650000, 0x9b59b6},
		{"Legend", "🌟", 2000000, 0xf1c40f},
		{"Casino Elite", "💎", 4500000, 0x1abc9c},
	}
	
	for i := len(ranks) - 1; i >= 0; i-- {
		if totalXP >= ranks[i].XPRequired {
			var nextXP int64
			if i < len(ranks)-1 {
				nextXP = ranks[i+1].XPRequired
			} else {
				nextXP = totalXP // Max rank reached
			}
			return ranks[i].Name, ranks[i].Icon, ranks[i].Color, nextXP
		}
	}
	
	return ranks[0].Name, ranks[0].Icon, ranks[0].Color, ranks[1].XPRequired
}

// GetJackpot retrieves the current jackpot amount
func GetJackpot() (int64, error) {
	if DB == nil {
		return 0, fmt.Errorf("database not connected")
	}

	ctx := context.Background()
	var amount int64
	
	query := "SELECT amount FROM jackpots WHERE id = 1"
	err := DB.QueryRow(ctx, query).Scan(&amount)
	
	if err != nil {
		if err == pgx.ErrNoRows {
			// Initialize jackpot if doesn't exist
			return InitializeJackpot(100000) // Default 100k jackpot
		}
		return 0, fmt.Errorf("failed to get jackpot: %w", err)
	}
	
	return amount, nil
}

// InitializeJackpot sets the initial jackpot amount
func InitializeJackpot(amount int64) (int64, error) {
	if DB == nil {
		return 0, fmt.Errorf("database not connected")
	}

	ctx := context.Background()
	
	query := `
	INSERT INTO jackpots (id, amount) VALUES (1, $1)
	ON CONFLICT (id) DO UPDATE SET amount = EXCLUDED.amount
	RETURNING amount`
	
	var newAmount int64
	err := DB.QueryRow(ctx, query, amount).Scan(&newAmount)
	if err != nil {
		return 0, fmt.Errorf("failed to initialize jackpot: %w", err)
	}
	
	return newAmount, nil
}

// UpdateJackpot increments the jackpot by the specified amount
func UpdateJackpot(increment int64) (int64, error) {
	if DB == nil {
		return 0, fmt.Errorf("database not connected")
	}

	ctx := context.Background()
	
	query := `
	UPDATE jackpots SET amount = amount + $1 WHERE id = 1
	RETURNING amount`
	
	var newAmount int64
	err := DB.QueryRow(ctx, query, increment).Scan(&newAmount)
	if err != nil {
		return 0, fmt.Errorf("failed to update jackpot: %w", err)
	}
	
	return newAmount, nil
}

// GetUserAchievements retrieves all achievements for a user
func GetUserAchievements(userID int64) ([]UserAchievement, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not connected")
	}

	ctx := context.Background()
	
	query := `
	SELECT ua.id, ua.user_id, ua.achievement_id, ua.earned_at
	FROM user_achievements ua
	WHERE ua.user_id = $1
	ORDER BY ua.earned_at DESC`
	
	rows, err := DB.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user achievements: %w", err)
	}
	defer rows.Close()
	
	var achievements []UserAchievement
	for rows.Next() {
		var ua UserAchievement
		err := rows.Scan(&ua.ID, &ua.UserID, &ua.AchievementID, &ua.EarnedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user achievement: %w", err)
		}
		achievements = append(achievements, ua)
	}
	
	return achievements, nil
}

// GetAllAchievements retrieves all available achievements
func GetAllAchievements() ([]Achievement, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not connected")
	}

	ctx := context.Background()
	
	query := `
	SELECT id, name, description, icon, category, requirement_type, 
	       requirement_value, chips_reward, xp_reward, hidden, created_at
	FROM achievements
	ORDER BY id`
	
	rows, err := DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get achievements: %w", err)
	}
	defer rows.Close()
	
	var achievements []Achievement
	for rows.Next() {
		var a Achievement
		err := rows.Scan(
			&a.ID, &a.Name, &a.Description, &a.Icon, &a.Category,
			&a.RequirementType, &a.RequirementValue, &a.ChipsReward,
			&a.XPReward, &a.Hidden, &a.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan achievement: %w", err)
		}
		achievements = append(achievements, a)
	}
	
	return achievements, nil
}