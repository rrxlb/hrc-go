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

// Forward declarations - these will be replaced with proper imports
type User struct {
	UserID        int64
	Username      string
	Chips         int64
	Wins          int
	Losses        int
	TotalXP       int64
	CurrentXP     int64
	PrestigeLevel int
	PremiumData   JSONB
	LastDaily     *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type UserUpdateData struct {
	Username           string
	ChipsIncrement     int64
	WinsIncrement      int
	LossesIncrement    int
	TotalXPIncrement   int64
	CurrentXPIncrement int64
	PrestigeLevel      int
	LastDaily          *time.Time
}

type Achievement struct {
	ID           int
	Name         string
	Description  string
	Icon         string
	Type         string
	TargetValue  int
	ChipsReward  int64
	XPReward     int64
	IsHidden     bool
	IsDefault    bool
	CreatedAt    time.Time
}

type UserAchievement struct {
	UserID        int64
	AchievementID int
	EarnedAt      time.Time
	Progress      int
	Name          string
	Description   string
	Icon          string
	Type          string
	TargetValue   int
	ChipsReward   int64
	XPReward      int64
	IsHidden      bool
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
		return fmt.Errorf("DATABASE_URL not set in environment")
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Optimize connection pool settings
	config.MinConns = 10
	config.MaxConns = 20
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

// createTables creates all necessary database tables
func createTables() error {
	ctx := context.Background()
	
	// Users table
	usersTable := `
	CREATE TABLE IF NOT EXISTS users (
		user_id BIGINT PRIMARY KEY,
		username VARCHAR(255),
		chips BIGINT DEFAULT 1000,
		wins INTEGER DEFAULT 0,
		losses INTEGER DEFAULT 0,
		total_xp BIGINT DEFAULT 0,
		current_xp BIGINT DEFAULT 0,
		prestige_level INTEGER DEFAULT 0,
		premium_data JSONB DEFAULT '{}',
		last_daily TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`

	// Achievements table
	achievementsTable := `
	CREATE TABLE IF NOT EXISTS achievements (
		id SERIAL PRIMARY KEY,
		name VARCHAR(255) UNIQUE NOT NULL,
		description TEXT NOT NULL,
		icon VARCHAR(50) NOT NULL,
		achievement_type VARCHAR(50) NOT NULL,
		target_value INTEGER,
		chips_reward INTEGER DEFAULT 0,
		xp_reward INTEGER DEFAULT 0,
		is_hidden BOOLEAN DEFAULT FALSE,
		is_default BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`

	// User achievements table
	userAchievementsTable := `
	CREATE TABLE IF NOT EXISTS user_achievements (
		user_id BIGINT,
		achievement_id INTEGER,
		earned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		progress INTEGER DEFAULT 0,
		PRIMARY KEY (user_id, achievement_id),
		FOREIGN KEY (achievement_id) REFERENCES achievements(id) ON DELETE CASCADE
	)`

	// Execute table creation
	tables := []string{usersTable, achievementsTable, userAchievementsTable}
	for _, table := range tables {
		if _, err := DB.Exec(ctx, table); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	log.Println("Database tables created/verified successfully")
	return nil
}

// GetUser retrieves user data from the database
func GetUser(userID int64) (*User, error) {
	ctx := context.Background()
	
	user := &User{}
	query := `
	SELECT user_id, username, chips, wins, losses, total_xp, current_xp, 
	       prestige_level, premium_data, last_daily, created_at, updated_at
	FROM users WHERE user_id = $1`
	
	err := DB.QueryRow(ctx, query, userID).Scan(
		&user.UserID, &user.Username, &user.Chips, &user.Wins, &user.Losses,
		&user.TotalXP, &user.CurrentXP, &user.PrestigeLevel, &user.PremiumData,
		&user.LastDaily, &user.CreatedAt, &user.UpdatedAt,
	)
	
	if err != nil {
		if err == pgx.ErrNoRows {
			// Create new user if doesn't exist
			return CreateUser(userID, "")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	
	return user, nil
}

// CreateUser creates a new user in the database
func CreateUser(userID int64, username string) (*User, error) {
	ctx := context.Background()
	
	user := &User{
		UserID:        userID,
		Username:      username,
		Chips:         StartingChips,
		Wins:          0,
		Losses:        0,
		TotalXP:       0,
		CurrentXP:     0,
		PrestigeLevel: 0,
		PremiumData:   JSONB{},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	
	query := `
	INSERT INTO users (user_id, username, chips, wins, losses, total_xp, current_xp, 
	                  prestige_level, premium_data, created_at, updated_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	ON CONFLICT (user_id) DO UPDATE SET
		username = EXCLUDED.username,
		updated_at = CURRENT_TIMESTAMP
	RETURNING user_id, username, chips, wins, losses, total_xp, current_xp, 
	          prestige_level, premium_data, last_daily, created_at, updated_at`
	
	err := DB.QueryRow(ctx, query,
		user.UserID, user.Username, user.Chips, user.Wins, user.Losses,
		user.TotalXP, user.CurrentXP, user.PrestigeLevel, user.PremiumData,
		user.CreatedAt, user.UpdatedAt,
	).Scan(
		&user.UserID, &user.Username, &user.Chips, &user.Wins, &user.Losses,
		&user.TotalXP, &user.CurrentXP, &user.PrestigeLevel, &user.PremiumData,
		&user.LastDaily, &user.CreatedAt, &user.UpdatedAt,
	)
	
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	
	return user, nil
}

// UpdateUser updates user data in the database
func UpdateUser(userID int64, updates UserUpdateData) (*User, error) {
	ctx := context.Background()
	
	// Build dynamic query based on provided updates
	setParts := []string{"updated_at = CURRENT_TIMESTAMP"}
	args := []interface{}{userID}
	argIndex := 2
	
	if updates.ChipsIncrement != 0 {
		setParts = append(setParts, fmt.Sprintf("chips = chips + $%d", argIndex))
		args = append(args, updates.ChipsIncrement)
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

	if updates.Username != "" {
		setParts = append(setParts, fmt.Sprintf("username = $%d", argIndex))
		args = append(args, updates.Username)
		argIndex++
	}
	
	query := fmt.Sprintf(`
		UPDATE users SET %s WHERE user_id = $1
		RETURNING user_id, username, chips, wins, losses, total_xp, current_xp, 
		          prestige_level, premium_data, last_daily, created_at, updated_at`,
		strings.Join(setParts, ", "))
	
	user := &User{}
	err := DB.QueryRow(ctx, query, args...).Scan(
		&user.UserID, &user.Username, &user.Chips, &user.Wins, &user.Losses,
		&user.TotalXP, &user.CurrentXP, &user.PrestigeLevel, &user.PremiumData,
		&user.LastDaily, &user.CreatedAt, &user.UpdatedAt,
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

// Achievement-related functions
func CreateAchievement(achievement *Achievement) error {
	ctx := context.Background()
	
	query := `
	INSERT INTO achievements (name, description, icon, achievement_type, target_value,
	                         chips_reward, xp_reward, is_hidden, is_default)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	RETURNING id, created_at`
	
	return DB.QueryRow(ctx, query,
		achievement.Name, achievement.Description, achievement.Icon,
		achievement.Type, achievement.TargetValue, achievement.ChipsReward,
		achievement.XPReward, achievement.IsHidden, achievement.IsDefault,
	).Scan(&achievement.ID, &achievement.CreatedAt)
}

func GetUserAchievements(userID int64) ([]UserAchievement, error) {
	ctx := context.Background()
	
	query := `
	SELECT ua.user_id, ua.achievement_id, ua.earned_at, ua.progress,
	       a.name, a.description, a.icon, a.achievement_type, a.target_value,
	       a.chips_reward, a.xp_reward, a.is_hidden
	FROM user_achievements ua
	JOIN achievements a ON ua.achievement_id = a.id
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
		err := rows.Scan(
			&ua.UserID, &ua.AchievementID, &ua.EarnedAt, &ua.Progress,
			&ua.Name, &ua.Description, &ua.Icon, &ua.Type, &ua.TargetValue,
			&ua.ChipsReward, &ua.XPReward, &ua.IsHidden,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user achievement: %w", err)
		}
		achievements = append(achievements, ua)
	}
	
	return achievements, nil
}