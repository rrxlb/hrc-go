package models

import (
	"time"
	"github.com/bwmarrin/discordgo"
)

// GameState represents the current state of any casino game
type GameState struct {
	GameID       string                 `json:"game_id"`
	UserID       int64                  `json:"user_id"`
	GameType     string                 `json:"game_type"`
	Bet          int64                  `json:"bet"`
	Status       GameStatus             `json:"status"`
	Data         map[string]interface{} `json:"data"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	ExpiresAt    time.Time              `json:"expires_at"`
	ChannelID    string                 `json:"channel_id"`
	MessageID    string                 `json:"message_id"`
}

// GameStatus represents the current status of a game
type GameStatus string

const (
	GameStatusWaiting    GameStatus = "waiting"    // Waiting for player action
	GameStatusInProgress GameStatus = "in_progress" // Game is active
	GameStatusCompleted  GameStatus = "completed"  // Game finished normally
	GameStatusCanceled   GameStatus = "canceled"   // Game was canceled
	GameStatusTimeout    GameStatus = "timeout"    // Game timed out
)

// InteractionContext holds Discord interaction data for games
type InteractionContext struct {
	Session     *discordgo.Session
	Interaction *discordgo.InteractionCreate
	User        *discordgo.User
	ChannelID   string
	GuildID     string
}

// GameResult represents the outcome of a completed game
type GameResult struct {
	UserID       int64     `json:"user_id"`
	GameType     string    `json:"game_type"`
	InitialBet   int64     `json:"initial_bet"`
	FinalAmount  int64     `json:"final_amount"`
	Profit       int64     `json:"profit"`
	XPGained     int64     `json:"xp_gained"`
	Duration     time.Duration `json:"duration"`
	CompletedAt  time.Time `json:"completed_at"`
	ResultData   map[string]interface{} `json:"result_data"`
}

// IsExpired checks if the game state has expired
func (gs *GameState) IsExpired() bool {
	return time.Now().After(gs.ExpiresAt)
}

// IsActive checks if the game is currently active
func (gs *GameState) IsActive() bool {
	return gs.Status == GameStatusWaiting || gs.Status == GameStatusInProgress
}

// IsCompleted checks if the game has finished
func (gs *GameState) IsCompleted() bool {
	return gs.Status == GameStatusCompleted || 
		   gs.Status == GameStatusCanceled || 
		   gs.Status == GameStatusTimeout
}

// GetProfit calculates the profit/loss from the game result
func (gr *GameResult) GetProfit() int64 {
	return gr.FinalAmount - gr.InitialBet
}

// IsWin checks if the game result was a win
func (gr *GameResult) IsWin() bool {
	return gr.GetProfit() > 0
}

// IsLoss checks if the game result was a loss
func (gr *GameResult) IsLoss() bool {
	return gr.GetProfit() < 0
}

// IsTie checks if the game result was a tie
func (gr *GameResult) IsTie() bool {
	return gr.GetProfit() == 0
}