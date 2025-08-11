package utils

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// GameAction represents an action that can be taken in a game
type GameAction string

const (
	ActionHit      GameAction = "hit"
	ActionStand    GameAction = "stand"
	ActionDouble   GameAction = "double"
	ActionSplit    GameAction = "split"
	ActionSurrender GameAction = "surrender"
	ActionInsurance GameAction = "insurance"
	ActionBet      GameAction = "bet"
	ActionCall     GameAction = "call"
	ActionFold     GameAction = "fold"
	ActionRaise    GameAction = "raise"
	ActionSpin     GameAction = "spin"
	ActionCashout  GameAction = "cashout"
	ActionReveal   GameAction = "reveal"
)

// GameStatus represents the current state of a game
type GameStatus string

const (
	StatusWaiting    GameStatus = "waiting"     // Waiting for players or input
	StatusInProgress GameStatus = "in_progress" // Game is actively being played
	StatusFinished   GameStatus = "finished"    // Game is completed
	StatusTimedOut   GameStatus = "timed_out"   // Game timed out
	StatusCancelled  GameStatus = "cancelled"   // Game was cancelled
)

// GameResult represents the outcome of a game
type GameResult struct {
	Winner        string            `json:"winner"`         // "player", "dealer", "tie", etc.
	PayoutMultiplier float64        `json:"payout_multiplier"`
	ChipsWon      int64             `json:"chips_won"`
	XPAwarded     int64             `json:"xp_awarded"`
	Details       map[string]interface{} `json:"details,omitempty"`
	JackpotWon    *JackpotWinInfo   `json:"jackpot_won,omitempty"`
}

// GameData contains the current state and information about a game
type GameData struct {
	Hand          *Hand                  `json:"hand,omitempty"`
	DealerHand    *Hand                  `json:"dealer_hand,omitempty"`
	Deck          *Deck                  `json:"deck,omitempty"`
	Bets          []int64                `json:"bets"`
	CurrentBet    int64                  `json:"current_bet"`
	Round         int                    `json:"round"`
	TurnIndex     int                    `json:"turn_index"`
	CustomData    map[string]interface{} `json:"custom_data,omitempty"`
}

// Game interface that all casino games must implement
type Game interface {
	// Core game lifecycle
	Initialize() error
	Start() error
	ProcessAction(action GameAction, data interface{}) error
	GetResult() *GameResult
	IsFinished() bool
	GetStatus() GameStatus
	Cleanup()

	// Discord integration
	GetEmbed() *discordgo.MessageEmbed
	GetComponents() []discordgo.MessageComponent
	HandleInteraction(i *discordgo.InteractionCreate) error

	// Game information
	GetGameID() string
	GetGameType() string
	GetUserID() int64
	GetBet() int64
	GetTimeRemaining() time.Duration
	
	// Game data
	GetGameData() *GameData
	SetGameData(data *GameData)
}

// BaseGameImplementation provides common functionality for all games
type BaseGameImplementation struct {
	GameID      string                 `json:"game_id"`
	GameType    string                 `json:"game_type"`
	UserID      int64                  `json:"user_id"`
	Username    string                 `json:"username"`
	Bet         int64                  `json:"bet"`
	Status      GameStatus             `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	ExpiresAt   time.Time              `json:"expires_at"`
	Session     *discordgo.Session     `json:"-"`
	Interaction *discordgo.InteractionCreate `json:"-"`
	Result      *GameResult            `json:"result,omitempty"`
	Data        *GameData              `json:"data"`
	mutex       sync.RWMutex           `json:"-"`
}

// NewBaseGameImplementation creates a new base game implementation
func NewBaseGameImplementation(session *discordgo.Session, interaction *discordgo.InteractionCreate, bet int64, gameType string) *BaseGameImplementation {
	userID := GetUserIDFromInteraction(interaction)
	username := GetUsernameFromInteraction(interaction)
	gameID := fmt.Sprintf("%s_%d_%d", gameType, userID, time.Now().Unix())

	return &BaseGameImplementation{
		GameID:      gameID,
		GameType:    gameType,
		UserID:      userID,
		Username:    username,
		Bet:         bet,
		Status:      StatusWaiting,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(15 * time.Minute), // 15 minute timeout
		Session:     session,
		Interaction: interaction,
		Data:        &GameData{Bets: []int64{bet}, CurrentBet: bet},
	}
}

// Implement basic methods that most games will use
func (b *BaseGameImplementation) GetGameID() string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.GameID
}

func (b *BaseGameImplementation) GetGameType() string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.GameType
}

func (b *BaseGameImplementation) GetUserID() int64 {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.UserID
}

func (b *BaseGameImplementation) GetBet() int64 {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.Bet
}

func (b *BaseGameImplementation) GetStatus() GameStatus {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.Status
}

func (b *BaseGameImplementation) IsFinished() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.Status == StatusFinished || b.Status == StatusTimedOut || b.Status == StatusCancelled
}

func (b *BaseGameImplementation) GetResult() *GameResult {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.Result
}

func (b *BaseGameImplementation) GetTimeRemaining() time.Duration {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	if b.ExpiresAt.Before(time.Now()) {
		return 0
	}
	return time.Until(b.ExpiresAt)
}

func (b *BaseGameImplementation) GetGameData() *GameData {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.Data
}

func (b *BaseGameImplementation) SetGameData(data *GameData) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.Data = data
	b.UpdatedAt = time.Now()
}

func (b *BaseGameImplementation) SetStatus(status GameStatus) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.Status = status
	b.UpdatedAt = time.Now()
}

func (b *BaseGameImplementation) SetResult(result *GameResult) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.Result = result
	b.UpdatedAt = time.Now()
}

func (b *BaseGameImplementation) IsExpired() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return time.Now().After(b.ExpiresAt)
}

func (b *BaseGameImplementation) ExtendTimeout(duration time.Duration) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.ExpiresAt = time.Now().Add(duration)
}

// Basic cleanup implementation
func (b *BaseGameImplementation) Cleanup() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	
	if b.Status == StatusInProgress {
		b.Status = StatusCancelled
	}
	b.Session = nil
	b.Interaction = nil
}

// GameManager manages active games across all game types
type GameManager struct {
	games   map[string]Game
	mutex   sync.RWMutex
	cleanup *time.Ticker
}

// Global game manager
var GameMgr *GameManager

// InitializeGameManager sets up the game management system
func InitializeGameManager() {
	GameMgr = &GameManager{
		games:   make(map[string]Game),
		cleanup: time.NewTicker(5 * time.Minute), // Cleanup every 5 minutes
	}
	
	// Start cleanup routine
	go GameMgr.cleanupRoutine()
}

// CloseGameManager stops the game manager
func CloseGameManager() {
	if GameMgr != nil && GameMgr.cleanup != nil {
		GameMgr.cleanup.Stop()
	}
}

// AddGame adds a game to the active games
func (gm *GameManager) AddGame(game Game) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	gm.games[game.GetGameID()] = game
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
	if game, exists := gm.games[gameID]; exists {
		game.Cleanup()
		delete(gm.games, gameID)
	}
}

// GetUserGame finds an active game for a specific user and game type
func (gm *GameManager) GetUserGame(userID int64, gameType string) (Game, bool) {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	
	for _, game := range gm.games {
		if game.GetUserID() == userID && game.GetGameType() == gameType && !game.IsFinished() {
			return game, true
		}
	}
	return nil, false
}

// GetActiveGamesCount returns the number of active games
func (gm *GameManager) GetActiveGamesCount() int {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	return len(gm.games)
}

// GetActiveGamesForUser returns all active games for a specific user
func (gm *GameManager) GetActiveGamesForUser(userID int64) []Game {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	
	var userGames []Game
	for _, game := range gm.games {
		if game.GetUserID() == userID && !game.IsFinished() {
			userGames = append(userGames, game)
		}
	}
	return userGames
}

// cleanupRoutine removes expired and finished games
func (gm *GameManager) cleanupRoutine() {
	for range gm.cleanup.C {
		gm.cleanupExpiredGames()
	}
}

// cleanupExpiredGames removes expired and finished games
func (gm *GameManager) cleanupExpiredGames() {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	
	expiredGames := make([]string, 0)
	now := time.Now()
	
	for gameID, game := range gm.games {
		// Check if game is finished or expired
		if game.IsFinished() || game.GetTimeRemaining() <= 0 {
			expiredGames = append(expiredGames, gameID)
		}
	}
	
	// Clean up expired games
	for _, gameID := range expiredGames {
		if game, exists := gm.games[gameID]; exists {
			// If game was just expired (not finished), mark as timed out
			if !game.IsFinished() && game.GetTimeRemaining() <= 0 {
				if baseGame, ok := game.(*BaseGameImplementation); ok {
					baseGame.SetStatus(StatusTimedOut)
				}
			}
			game.Cleanup()
			delete(gm.games, gameID)
		}
	}
	
	if len(expiredGames) > 0 {
		fmt.Printf("Cleaned up %d expired games", len(expiredGames))
	}
}

// Utility functions for Discord interactions
func GetUserIDFromInteraction(i *discordgo.InteractionCreate) int64 {
	var userID string
	if i.Member != nil && i.Member.User != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}
	
	if id, err := parseUserID(userID); err == nil {
		return id
	}
	return 0
}

func GetUsernameFromInteraction(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.Username
	} else if i.User != nil {
		return i.User.Username
	}
	return "Unknown"
}

func parseUserID(userID string) (int64, error) {
	return strconv.ParseInt(userID, 10, 64)
}