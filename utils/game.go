package utils

import (
	"fmt"
	"log"
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

// parseUserID converts string user ID to int64
func parseUserID(userID string) (int64, error) {
	return strconv.ParseInt(userID, 10, 64)
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
	user, err := GetUser(bg.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user data: %w", err)
	}
	
	bg.UserData = user
	
	if user.Chips < bg.Bet {
		return fmt.Errorf("insufficient chips: you have %d, need %d", user.Chips, bg.Bet)
	}
	
	if bg.Bet <= 0 {
		return fmt.Errorf("bet must be greater than 0")
	}
	
	return nil
}

// EndGame handles the end of a game, updating user stats and returning updated user data
func (bg *BaseGame) EndGame(profit int64) (*User, error) {
	bg.mu.Lock()
	defer bg.mu.Unlock()
	
	if bg.IsGameOverFlag {
		return bg.UserData, fmt.Errorf("game already ended")
	}
	
	bg.IsGameOverFlag = true
	
	// Update user data
	updates := UserUpdateData{
		ChipsIncrement: profit,
	}
	
	// Determine win/loss/push
	if profit > 0 {
		updates.WinsIncrement = 1
		// Award XP based on profit
		xpGained := profit / 10 // 1 XP per 10 chips profit
		if xpGained > 0 {
			updates.TotalXPIncrement = xpGained
			updates.CurrentXPIncrement = xpGained
		}
		log.Printf("Game %s: User %d won %d chips (XP: %d)", bg.GameType, bg.UserID, profit, xpGained)
	} else if profit < 0 {
		updates.LossesIncrement = 1
		log.Printf("Game %s: User %d lost %d chips", bg.GameType, bg.UserID, -profit)
	} else {
		log.Printf("Game %s: User %d pushed (no chips change)", bg.GameType, bg.UserID)
	}
	
	// Update user in database
	updatedUser, err := UpdateUser(bg.UserID, updates)
	if err != nil {
		log.Printf("Failed to update user %d after game: %v", bg.UserID, err)
		return bg.UserData, fmt.Errorf("failed to update user data: %w", err)
	}
	
	bg.UserData = updatedUser
	
	// Update jackpot (small contribution from each game)
	if profit < 0 && -profit >= 100 { // Only on losses of 100+ chips
		jackpotContribution := (-profit) / 100 // 1% of loss goes to jackpot
		if jackpotContribution > 0 {
			if newJackpot, err := UpdateJackpot(jackpotContribution); err == nil {
				log.Printf("Jackpot increased by %d to %d", jackpotContribution, newJackpot)
			}
		}
	}
	
	return updatedUser, nil
}

// SetGameOver marks the game as over
func (bg *BaseGame) SetGameOver() {
	bg.mu.Lock()
	defer bg.mu.Unlock()
	bg.IsGameOverFlag = true
}

// SendInteractionResponse sends a response to a Discord interaction
func SendInteractionResponse(session *discordgo.Session, interaction *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, ephemeral bool) error {
	data := &discordgo.InteractionResponseData{
		Embeds: []*discordgo.MessageEmbed{embed},
	}
	
	if len(components) > 0 {
		data.Components = components
	}
	
	if ephemeral {
		data.Flags = discordgo.MessageFlagsEphemeral
	}
	
	return session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: data,
	})
}

// EditInteractionResponse edits a Discord interaction response
func EditInteractionResponse(session *discordgo.Session, interaction *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error {
	data := &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	}
	
	if len(components) > 0 {
		data.Components = &components
	}
	
	_, err := session.InteractionResponseEdit(interaction.Interaction, data)
	return err
}

// FormatChips formats chip amounts with emoji
func FormatChips(amount int64) string {
	return fmt.Sprintf("%d <:chips:1396988413151940629>", amount)
}

// CalculateXPGain calculates XP gained from profit
func CalculateXPGain(profit int64) int64 {
	if profit <= 0 {
		return 0
	}
	// 1 XP per 10 chips profit, minimum 1 XP for any profit
	xp := profit / 10
	if xp == 0 && profit > 0 {
		xp = 1
	}
	return xp
}

// GetChipsEmoji returns the chips emoji
func GetChipsEmoji() string {
	return "<:chips:1396988413151940629>"
}

// Constants for game configuration
const (
	// Blackjack constants
	DeckCount             = 6
	ShuffleThreshold      = 0.25
	DealerStandValue      = 17
	BlackjackPayout       = 1.5
	FiveCardCharliePayout = 1.75
	
	// Baccarat constants
	BaccaratPayout          = 1.0
	BaccaratTiePayout       = 8.0
	BaccaratBankerCommission = 0.05
	
	// General constants
	BotColor = 0x5865F2
)

// GameTimeouts defines timeout durations for different games
var GameTimeouts = map[string]time.Duration{
	"blackjack": 60 * time.Second,
	"baccarat":  45 * time.Second,
	"craps":     60 * time.Second,
	"roulette":  45 * time.Second,
	"slots":     30 * time.Second,
}