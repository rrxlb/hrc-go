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
	"sync"
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
	CreatedAt            time.Time
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

// JSONB represents a JSONB column type for PostgreSQL
type JSONB map[string]interface{}

// Value implements driver.Valuer interface for JSONB
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements sql.Scanner interface for JSONB
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into JSONB", value)
	}
	
	if len(bytes) == 0 {
		*j = make(JSONB)
		return nil
	}
	
	var data map[string]interface{}
	if err := json.Unmarshal(bytes, &data); err != nil {
		log.Printf("Error unmarshaling JSONB: %v, data: %s", err, string(bytes))
		// Don't return error, just set to empty map
		*j = make(JSONB)
		return nil
	}
	
	*j = JSONB(data)
	return nil
}

var (
	DB           *pgxpool.Pool
	dbInitialized = false
	dbMutex      sync.RWMutex
)

// InitializeDatabase initializes the database connection pool
func InitializeDatabase() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if dbInitialized {
		return nil
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Printf("DATABASE_URL not set, skipping database initialization")
		return nil
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test the connection
	conn, err := pool.Acquire(ctx)
	if err != nil {
		pool.Close()
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	conn.Release()

	DB = pool
	dbInitialized = true

	log.Printf("Database connection initialized successfully")
	return nil
}

// CloseDatabase closes the database connection pool
func CloseDatabase() {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if DB != nil {
		DB.Close()
		DB = nil
		dbInitialized = false
		log.Printf("Database connection closed")
	}
}

// GetUser retrieves a user from the database, creating one if it doesn't exist
func GetUser(userID int64) (*User, error) {
	if DB == nil {
		return &User{
			UserID:              userID,
			Chips:               StartingChips,
			TotalXP:             0,
			CurrentXP:           0,
			Prestige:            0,
			Wins:                0,
			Losses:              0,
			DailyBonusesClaimed: 0,
			VotesCount:          0,
			PremiumSettings:     make(JSONB),
			CreatedAt:           time.Now(),
		}, nil
	}

	ctx := context.Background()

	query := `
		SELECT user_id, chips, total_xp, current_xp, prestige, wins, losses, 
			   daily_bonuses_claimed, votes_count, last_hourly, last_daily, 
			   last_weekly, last_vote, last_bonus, premium_settings, created_at
		FROM users WHERE user_id = $1`

	var user User
	err := DB.QueryRow(ctx, query, userID).Scan(
		&user.UserID,
		&user.Chips,
		&user.TotalXP,
		&user.CurrentXP,
		&user.Prestige,
		&user.Wins,
		&user.Losses,
		&user.DailyBonusesClaimed,
		&user.VotesCount,
		&user.LastHourly,
		&user.LastDaily,
		&user.LastWeekly,
		&user.LastVote,
		&user.LastBonus,
		&user.PremiumSettings,
		&user.CreatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			// User doesn't exist, create a new one
			return CreateUser(userID)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	log.Printf("Retrieved user %d: chips=%d, xp=%d", user.UserID, user.Chips, user.TotalXP)
	return &user, nil
}

// CreateUser creates a new user in the database
func CreateUser(userID int64) (*User, error) {
	if DB == nil {
		return &User{
			UserID:              userID,
			Chips:               StartingChips,
			TotalXP:             0,
			CurrentXP:           0,
			Prestige:            0,
			Wins:                0,
			Losses:              0,
			DailyBonusesClaimed: 0,
			VotesCount:          0,
			PremiumSettings:     make(JSONB),
			CreatedAt:           time.Now(),
		}, nil
	}

	ctx := context.Background()
	now := time.Now()

	user := &User{
		UserID:              userID,
		Chips:               StartingChips,
		TotalXP:             0,
		CurrentXP:           0,
		Prestige:            0,
		Wins:                0,
		Losses:              0,
		DailyBonusesClaimed: 0,
		VotesCount:          0,
		PremiumSettings:     make(JSONB),
		CreatedAt:           now,
	}

	query := `
		INSERT INTO users (user_id, chips, total_xp, current_xp, prestige, wins, losses, 
						  daily_bonuses_claimed, votes_count, premium_settings, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := DB.Exec(ctx, query,
		user.UserID,
		user.Chips,
		user.TotalXP,
		user.CurrentXP,
		user.Prestige,
		user.Wins,
		user.Losses,
		user.DailyBonusesClaimed,
		user.VotesCount,
		user.PremiumSettings,
		user.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	log.Printf("Created new user %d with %d chips", userID, StartingChips)
	return user, nil
}

// UpdateUser updates user data in the database
func UpdateUser(userID int64, updates UserUpdateData) (*User, error) {
	if DB == nil {
		// Return dummy user for testing
		return &User{
			UserID:              userID,
			Chips:               StartingChips + updates.ChipsIncrement,
			TotalXP:             updates.TotalXPIncrement,
			CurrentXP:           updates.CurrentXPIncrement,
			Prestige:            0,
			Wins:                updates.WinsIncrement,
			Losses:              updates.LossesIncrement,
			DailyBonusesClaimed: updates.DailyBonusesClaimedIncrement,
			VotesCount:          updates.VotesCountIncrement,
			PremiumSettings:     updates.PremiumSettings,
			CreatedAt:           time.Now(),
		}, nil
	}

	ctx := context.Background()

	// Build dynamic query based on what fields need updating
	setParts := []string{}
	args := []interface{}{userID} // $1 will always be userID
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
		// No updates to make, just return current user
		return GetUser(userID)
	}

	query := fmt.Sprintf(`
		UPDATE users 
		SET %s
		WHERE user_id = $1
		RETURNING user_id, chips, total_xp, current_xp, prestige, wins, losses, 
				  daily_bonuses_claimed, votes_count, last_hourly, last_daily, 
				  last_weekly, last_vote, last_bonus, premium_settings, created_at`,
		strings.Join(setParts, ", "))

	var user User
	err := DB.QueryRow(ctx, query, args...).Scan(
		&user.UserID,
		&user.Chips,
		&user.TotalXP,
		&user.CurrentXP,
		&user.Prestige,
		&user.Wins,
		&user.Losses,
		&user.DailyBonusesClaimed,
		&user.VotesCount,
		&user.LastHourly,
		&user.LastDaily,
		&user.LastWeekly,
		&user.LastVote,
		&user.LastBonus,
		&user.PremiumSettings,
		&user.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	log.Printf("Updated user %d: chips=%d, xp=%d, wins=%d, losses=%d",
		user.UserID, user.Chips, user.TotalXP, user.Wins, user.Losses)
	return &user, nil
}

// Note: Cache functions are now in cache.go to avoid duplicates

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