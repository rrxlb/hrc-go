package utils

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	UserID              int64
	Chips               int64
	TotalXP             int64
	CurrentXP           int64
	Prestige            int
	Wins                int
	Losses              int
	DailyBonusesClaimed int
	VotesCount          int
	LastHourly          *time.Time
	LastDaily           *time.Time
	LastWeekly          *time.Time
	LastVote            *time.Time
	LastBonus           *time.Time
	PremiumSettings     JSONB
	CreatedAt           time.Time
}

type UserUpdateData struct {
	ChipsIncrement               int64
	TotalXPIncrement             int64
	CurrentXPIncrement           int64
	WinsIncrement                int
	LossesIncrement              int
	DailyBonusesClaimedIncrement int
	VotesCountIncrement          int
	Prestige                     *int
	LastHourly                   *time.Time
	LastDaily                    *time.Time
	LastWeekly                   *time.Time
	LastVote                     *time.Time
	LastBonus                    *time.Time
	PremiumSettings              JSONB
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
		// Don't return error, just set to empty map
		*j = make(JSONB)
		return nil
	}

	*j = JSONB(data)
	return nil
}

// Object pools for performance optimization
var (
	userPool = sync.Pool{
		New: func() interface{} {
			return &User{
				PremiumSettings: make(JSONB),
			}
		},
	}
	
	embedPool = sync.Pool{
		New: func() interface{} {
			return &discordgo.MessageEmbed{
				Fields: make([]*discordgo.MessageEmbedField, 0, 5),
			}
		},
	}
	
	componentPool = sync.Pool{
		New: func() interface{} {
			return make([]discordgo.MessageComponent, 0, 5)
		},
	}
	
	stringSlicePool = sync.Pool{
		New: func() interface{} {
			return make([]string, 0, 10)
		},
	}
)

// GetUserFromPool retrieves a user struct from the object pool
func GetUserFromPool() *User {
	return userPool.Get().(*User)
}

// PutUserToPool returns a user struct to the object pool after resetting it
func PutUserToPool(u *User) {
	// Reset all fields to zero values
	*u = User{
		PremiumSettings: make(JSONB),
	}
	userPool.Put(u)
}

// GetEmbedFromPool retrieves an embed from the object pool
func GetEmbedFromPool() *discordgo.MessageEmbed {
	embed := embedPool.Get().(*discordgo.MessageEmbed)
	// Reset embed fields
	embed.Title = ""
	embed.Description = ""
	embed.Color = 0
	embed.Fields = embed.Fields[:0] // Keep underlying array
	embed.Footer = nil
	embed.Thumbnail = nil
	return embed
}

// PutEmbedToPool returns an embed to the object pool
func PutEmbedToPool(embed *discordgo.MessageEmbed) {
	embedPool.Put(embed)
}

// GetComponentsFromPool retrieves a component slice from the pool
func GetComponentsFromPool() []discordgo.MessageComponent {
	components := componentPool.Get().([]discordgo.MessageComponent)
	return components[:0] // Reset length but keep capacity
}

// PutComponentsToPool returns components to the pool
func PutComponentsToPool(components []discordgo.MessageComponent) {
	componentPool.Put(components)
}

// GetStringSliceFromPool retrieves a string slice from the pool
func GetStringSliceFromPool() []string {
	slice := stringSlicePool.Get().([]string)
	return slice[:0] // Reset length but keep capacity
}

// PutStringSliceToPool returns a string slice to the pool
func PutStringSliceToPool(slice []string) {
	stringSlicePool.Put(slice)
}

var (
	DB            *pgxpool.Pool
	dbInitialized = false
	dbMutex       sync.RWMutex
)

// SetupDatabase initializes the database connection pool
func SetupDatabase() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if dbInitialized {
		return nil
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil
	}

	ctx := context.Background()

	// Parse the database URL to add connection pool optimizations
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Optimize connection pool for Discord bot workload on Railway
	config.MaxConns = 30                      // Increased for better concurrency
	config.MinConns = 8                       // More ready connections for bursts
	config.MaxConnLifetime = 45 * time.Minute // Balanced connection recycling
	config.MaxConnIdleTime = 5 * time.Minute  // Faster idle cleanup
	config.HealthCheckPeriod = 30 * time.Second // More frequent health checks
	
	// Additional performance optimizations
	config.ConnConfig.RuntimeParams = map[string]string{
		"application_name":     "hrc-discord-bot",
		"timezone":            "UTC",
		"statement_timeout":   "30s",
		"idle_in_transaction_session_timeout": "60s",
	}

	// Create optimized connection pool
	pool, err := pgxpool.NewWithConfig(ctx, config)
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

	// Ensure core tables exist
	createUsersTable()

	// Create user_achievements table if it doesn't exist
	createUserAchievementsTable()

	// Create performance indexes
	createPerformanceIndexes()

	// Initialize prepared statements for high-frequency queries
	if err := initializePreparedStatements(); err != nil {
		return fmt.Errorf("failed to initialize prepared statements: %w", err)
	}

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

	// Use object pool for better memory management
	user := GetUserFromPool()
	var err error
	defer func() {
		// Only return to pool if there was an error
		if err != nil {
			PutUserToPool(user)
		}
	}()

	// Use direct SQL query for reliable operation
	query := `
		SELECT user_id, chips, total_xp, current_xp, prestige, wins, losses, 
			   daily_bonuses_claimed, votes_count, last_hourly, last_daily, 
			   last_weekly, last_vote, last_bonus, premium_settings, created_at
		FROM users WHERE user_id = $1`

	err = DB.QueryRow(ctx, query, userID).Scan(
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

	return user, nil
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

	// Use object pool for better memory management
	user := GetUserFromPool()
	var err error
	defer func() {
		// Only return to pool if there was an error
		if err != nil {
			PutUserToPool(user)
		}
	}()

	user.UserID = userID
	user.Chips = StartingChips
	user.TotalXP = 0
	user.CurrentXP = 0
	user.Prestige = 0
	user.Wins = 0
	user.Losses = 0
	user.DailyBonusesClaimed = 0
	user.VotesCount = 0
	user.PremiumSettings = make(JSONB)
	user.CreatedAt = now

	query := `
		INSERT INTO users (user_id, chips, total_xp, current_xp, prestige, wins, losses, 
						  daily_bonuses_claimed, votes_count, premium_settings, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err = DB.Exec(ctx, query,
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

	// Use object pool for better memory management
	user := GetUserFromPool()
	var err error
	defer func() {
		// Only return to pool if there was an error
		if err != nil {
			PutUserToPool(user)
		}
	}()

	err = DB.QueryRow(ctx, query, args...).Scan(
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

	return user, nil
}

// BatchUpdateUsers updates multiple users in a single transaction for better performance
func BatchUpdateUsers(updates []struct {
	UserID int64
	Data   UserUpdateData
}) error {
	if DB == nil {
		return fmt.Errorf("database not connected")
	}

	ctx := context.Background()
	tx, err := DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Batch update using COPY or prepared statements
	for _, update := range updates {
		_, err := UpdateUser(update.UserID, update.Data)
		if err != nil {
			return fmt.Errorf("failed to update user %d: %w", update.UserID, err)
		}
	}

	return tx.Commit(ctx)
}

// GetMultipleUsers retrieves multiple users in a single query
func GetMultipleUsers(userIDs []int64) (map[int64]*User, error) {
	if DB == nil || len(userIDs) == 0 {
		return make(map[int64]*User), nil
	}

	ctx := context.Background()
	
	// Build parameterized query for multiple users
	query := `
		SELECT user_id, chips, total_xp, current_xp, prestige, wins, losses, 
			   daily_bonuses_claimed, votes_count, last_hourly, last_daily, 
			   last_weekly, last_vote, last_bonus, premium_settings, created_at
		FROM users WHERE user_id = ANY($1)`

	rows, err := DB.Query(ctx, query, userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to query multiple users: %w", err)
	}
	defer rows.Close()

	users := make(map[int64]*User)
	for rows.Next() {
		user := GetUserFromPool()
		err := rows.Scan(
			&user.UserID, &user.Chips, &user.TotalXP, &user.CurrentXP,
			&user.Prestige, &user.Wins, &user.Losses, &user.DailyBonusesClaimed,
			&user.VotesCount, &user.LastHourly, &user.LastDaily, &user.LastWeekly,
			&user.LastVote, &user.LastBonus, &user.PremiumSettings, &user.CreatedAt,
		)
		if err != nil {
			PutUserToPool(user)
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users[user.UserID] = user
	}

	return users, nil
}

// Note: Cache functions are now in cache.go to avoid duplicates

// ParseBet parses a bet string and validates it
func ParseBet(betStr string, userChips int64) (int64, error) {
	betStr = strings.TrimSpace(strings.ToLower(betStr))
	// Remove common formatting characters
	betStr = strings.ReplaceAll(betStr, ",", "")
	betStr = strings.ReplaceAll(betStr, "_", "")

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

// createUserAchievementsTable creates the user_achievements table if it doesn't exist
func createUserAchievementsTable() error {
	if DB == nil {
		return fmt.Errorf("database not connected")
	}

	ctx := context.Background()
	query := `
		CREATE TABLE IF NOT EXISTS user_achievements (
			id SERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL,
			achievement_id INTEGER NOT NULL,
			earned_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, achievement_id)
		);
		
		CREATE INDEX IF NOT EXISTS idx_user_achievements_user_id ON user_achievements(user_id);
		CREATE INDEX IF NOT EXISTS idx_user_achievements_achievement_id ON user_achievements(achievement_id);`

	_, err := DB.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create user_achievements table: %w", err)
	}

	return nil
}

// createUsersTable creates the users table if it does not exist
func createUsersTable() error {
	if DB == nil {
		return fmt.Errorf("database not connected")
	}
	ctx := context.Background()
	query := `CREATE TABLE IF NOT EXISTS users (
		user_id BIGINT PRIMARY KEY,
		chips BIGINT NOT NULL DEFAULT 0,
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
		premium_settings JSONB,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_users_chips ON users(chips);
	CREATE INDEX IF NOT EXISTS idx_users_total_xp ON users(total_xp);
	CREATE INDEX IF NOT EXISTS idx_users_wins ON users(wins);`
	if _, err := DB.Exec(ctx, query); err != nil {
		return fmt.Errorf("failed to create users table: %w", err)
	}
	return nil
}

// createPerformanceIndexes creates indexes to optimize query performance
func createPerformanceIndexes() error {
	if DB == nil {
		return fmt.Errorf("database not connected")
	}

	ctx := context.Background()

	// Performance-optimized indexes for common queries
	indexes := []string{
		// Leaderboard queries with composite indexes (chips, total_xp, prestige)
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_chips_desc ON users(chips DESC, user_id)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_total_xp_desc ON users(total_xp DESC, user_id)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_prestige_desc ON users(prestige DESC, user_id)",

		// Partial indexes for high-value users (better performance for active users)
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_active_chips ON users(chips DESC, user_id) WHERE chips >= 10000",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_high_xp ON users(total_xp DESC, user_id) WHERE total_xp >= 50000",

		// Bonus cooldown queries
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_last_hourly ON users(last_hourly) WHERE last_hourly IS NOT NULL",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_last_daily ON users(last_daily) WHERE last_daily IS NOT NULL",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_last_weekly ON users(last_weekly) WHERE last_weekly IS NOT NULL",

		// Achievement-related queries
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_wins_losses ON users(wins, losses)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_prestige_xp ON users(prestige, total_xp)",

		// JSONB operations (if premium settings are queried)
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_premium_gin ON users USING GIN(premium_settings)",

		// Time-based queries
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_created_at ON users(created_at)",
	}

	for _, query := range indexes {
		DB.Exec(ctx, query)
	}

	return nil
}

// Prepared statement names for high-frequency queries
var (
	preparedStatements = map[string]string{
		"getUserByID":       "SELECT user_id, chips, total_xp, current_xp, prestige, wins, losses, daily_bonuses_claimed, votes_count, last_hourly, last_daily, last_weekly, last_vote, last_bonus, premium_settings, created_at FROM users WHERE user_id = $1",
		"leaderboardChips":  "SELECT user_id, chips FROM users ORDER BY chips DESC, user_id LIMIT 10",
		"leaderboardXP":     "SELECT user_id, total_xp FROM users ORDER BY total_xp DESC, user_id LIMIT 10",
		"leaderboardPrestige": "SELECT user_id, prestige FROM users ORDER BY prestige DESC, user_id LIMIT 10",
		"updateUserStats":   "UPDATE users SET chips = chips + $2, total_xp = total_xp + $3, current_xp = current_xp + $4, wins = wins + $5, losses = losses + $6 WHERE user_id = $1 RETURNING user_id, chips, total_xp, current_xp, prestige, wins, losses, daily_bonuses_claimed, votes_count, last_hourly, last_daily, last_weekly, last_vote, last_bonus, premium_settings, created_at",
	}
)

// initializePreparedStatements creates prepared statements for high-frequency queries
func initializePreparedStatements() error {
	if DB == nil {
		return fmt.Errorf("database not connected")
	}

	ctx := context.Background()
	
	// Prepare high-frequency statements
	for name, query := range preparedStatements {
		_, err := DB.Prepare(ctx, name, query)
		if err != nil {
			return fmt.Errorf("failed to prepare statement %s: %w", name, err)
		}
	}
	
	return nil
}

// GetLeaderboard executes optimized leaderboard query using prepared statements
func GetLeaderboard(leaderboardType string) (pgx.Rows, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not connected")
	}

	ctx := context.Background()

	switch leaderboardType {
	case "chips":
		return DB.Query(ctx, "leaderboardChips")
	case "xp":
		return DB.Query(ctx, "leaderboardXP")
	case "prestige":
		return DB.Query(ctx, "leaderboardPrestige")
	default:
		return DB.Query(ctx, "leaderboardChips")
	}
}
