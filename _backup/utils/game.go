package utils

import (
	"fmt"
	"log"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Game represents the base game interface that all casino games must implement
type Game interface {
	GetUserID() int64
	GetBet() int64
	GetGameType() string
	ValidateBet() error
	EndGame(profit int64) (*User, error)
	IsGameOver() bool
	GetInteraction() *discordgo.InteractionCreate
}

// BaseGame provides common functionality for all casino games
type BaseGame struct {
	UserID               int64
	Bet                  int64
	GameType             string
	UserData             *User
	IsGameOverFlag       bool
	CountWinLossMinRatio float64 // Minimum fraction of pre-game chips required for W/L counting
	Interaction          *discordgo.InteractionCreate
	Session              *discordgo.Session
	CreatedAt            time.Time
	mu                   sync.RWMutex
}

// Achievement check debouncing
var (
	lastAchievementCheck    = make(map[int64]time.Time)
	achievementMutex        sync.RWMutex
	achievementDebounceTime = 30 * time.Second
	lastCleanup             time.Time
	cleanupInterval         = 5 * time.Minute
)

// NewBaseGame creates a new base game instance
func NewBaseGame(session *discordgo.Session, interaction *discordgo.InteractionCreate, bet int64, gameType string) *BaseGame {
	userID := interaction.Member.User.ID
	userIDInt, _ := parseUserID(userID)
	
	return &BaseGame{
		UserID:               userIDInt,
		Bet:                  bet,
		GameType:             gameType,
		IsGameOverFlag:       false,
		CountWinLossMinRatio: 0.0,
		Interaction:          interaction,
		Session:              session,
		CreatedAt:            time.Now(),
	}
}

// GetUserID returns the user ID
func (bg *BaseGame) GetUserID() int64 {
	return bg.UserID
}

// GetBet returns the bet amount
func (bg *BaseGame) GetBet() int64 {
	return bg.Bet
}

// GetGameType returns the game type
func (bg *BaseGame) GetGameType() string {
	return bg.GameType
}

// GetInteraction returns the Discord interaction
func (bg *BaseGame) GetInteraction() *discordgo.InteractionCreate {
	return bg.Interaction
}

// IsGameOver returns if the game has ended
func (bg *BaseGame) IsGameOver() bool {
	bg.mu.RLock()
	defer bg.mu.RUnlock()
	return bg.IsGameOverFlag
}

// ValidateBet checks if the player has enough chips for the bet
func (bg *BaseGame) ValidateBet() error {
	user, err := GetCachedUser(bg.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user data: %w", err)
	}
	
	bg.UserData = user
	
	if user.Chips < bg.Bet {
		return fmt.Errorf("insufficient chips: need %d, have %d", bg.Bet, user.Chips)
	}
	
	return nil
}

// EndGame finalizes the game and updates user stats
func (bg *BaseGame) EndGame(profit int64) (*User, error) {
	bg.mu.Lock()
	defer bg.mu.Unlock()
	
	if bg.IsGameOverFlag {
		return bg.UserData, nil
	}
	bg.IsGameOverFlag = true
	
	// Calculate XP gain
	var xpGain int64 = 0
	if profit > 0 {
		xpGain = profit * XPPerProfit
	}
	
	// Determine if this game should count towards wins/losses
	shouldCountWL := true
	if bg.CountWinLossMinRatio > 0.0 && bg.UserData != nil {
		requiredBet := int64(math.Ceil(float64(bg.UserData.Chips) * bg.CountWinLossMinRatio))
		shouldCountWL = bg.Bet >= requiredBet
	}
	
	// Prepare update data
	updates := UserUpdateData{
		ChipsIncrement:     profit,
		TotalXPIncrement:   xpGain,
		CurrentXPIncrement: xpGain,
	}
	
	if profit > 0 && shouldCountWL {
		updates.WinsIncrement = 1
	} else if profit < 0 && shouldCountWL {
		updates.LossesIncrement = 1
	}
	
	// Update user in database and cache
	updatedUser, err := UpdateCachedUser(bg.UserID, updates)
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}
	
	bg.UserData = updatedUser
	
	// Check achievements with debouncing
	go bg.checkAchievements(profit)
	
	return updatedUser, nil
}

// checkAchievements checks and awards achievements for the user (with debouncing)
func (bg *BaseGame) checkAchievements(profit int64) {
	currentTime := time.Now()
	
	// Periodic cleanup of achievement check cache
	achievementMutex.Lock()
	if currentTime.Sub(lastCleanup) > cleanupInterval {
		bg.cleanupAchievementCache(currentTime)
		lastCleanup = currentTime
	}
	
	lastCheck, exists := lastAchievementCheck[bg.UserID]
	if exists && currentTime.Sub(lastCheck) < achievementDebounceTime {
		achievementMutex.Unlock()
		return // Too soon since last check
	}
	
	lastAchievementCheck[bg.UserID] = currentTime
	achievementMutex.Unlock()
	
	// TODO: Implement achievement checking logic
	// This would involve checking various achievement types and awarding them
	// For now, just log that we would check achievements
	log.Printf("Would check achievements for user %d with profit %d", bg.UserID, profit)
}

// cleanupAchievementCache removes old entries from the achievement check cache
func (bg *BaseGame) cleanupAchievementCache(currentTime time.Time) {
	cutoffTime := currentTime.Add(-achievementDebounceTime * 2)
	
	for userID, timestamp := range lastAchievementCheck {
		if timestamp.Before(cutoffTime) {
			delete(lastAchievementCheck, userID)
		}
	}
	
	log.Printf("Cleaned up achievement check cache, active entries: %d", len(lastAchievementCheck))
}

// RespondWithError sends an error response to the user
func (bg *BaseGame) RespondWithError(message string) error {
	embed := &discordgo.MessageEmbed{
		Title:       "❌ Error",
		Description: message,
		Color:       0xff0000, // Red
	}
	
	response := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	}
	
	return bg.Session.InteractionRespond(bg.Interaction.Interaction, response)
}

// SendFollowup sends a followup message
func (bg *BaseGame) SendFollowup(embed *discordgo.MessageEmbed, ephemeral bool) error {
	flags := uint64(0)
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	
	_, err := bg.Session.FollowupMessageCreate(bg.Interaction.Interaction, true, &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
		Flags:  flags,
	})
	
	return err
}

// UpdateOriginalResponse updates the original interaction response
func (bg *BaseGame) UpdateOriginalResponse(embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error {
	edit := &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	}
	
	_, err := bg.Session.InteractionResponseEdit(bg.Interaction.Interaction, edit)
	return err
}

// Helper function to parse user ID from string to int64
func parseUserID(userIDStr string) (int64, error) {
	return strconv.ParseInt(userIDStr, 10, 64)
}

// GameManager manages active games
type GameManager struct {
	games map[string]Game
	mutex sync.RWMutex
}

// Global game manager instance
var Games = &GameManager{
	games: make(map[string]Game),
}

// AddGame adds a game to the active games
func (gm *GameManager) AddGame(gameID string, game Game) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	gm.games[gameID] = game
}

// GetGame retrieves a game by ID
func (gm *GameManager) GetGame(gameID string) (Game, bool) {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	game, exists := gm.games[gameID]
	return game, exists
}

// RemoveGame removes a game from active games
func (gm *GameManager) RemoveGame(gameID string) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	delete(gm.games, gameID)
}

// CleanupExpiredGames removes games that have been inactive too long
func (gm *GameManager) CleanupExpiredGames(maxAge time.Duration) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	
	now := time.Now()
	expiredGames := make([]string, 0)
	
	for gameID, game := range gm.games {
		if baseGame, ok := game.(*BaseGame); ok {
			if now.Sub(baseGame.CreatedAt) > maxAge {
				expiredGames = append(expiredGames, gameID)
			}
		}
	}
	
	for _, gameID := range expiredGames {
		delete(gm.games, gameID)
	}
	
	if len(expiredGames) > 0 {
		log.Printf("Cleaned up %d expired games", len(expiredGames))
	}
}