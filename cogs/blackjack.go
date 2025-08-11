package cogs

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
)

// ActiveBlackjackGames stores currently active blackjack games
var ActiveBlackjackGames = make(map[string]*BlackjackGame)
var blackjackMutex sync.RWMutex

// BlackjackGame represents a blackjack game instance
type BlackjackGame struct {
	*utils.BaseGame
	GameID      string
	Bet         int64
	Deck        *utils.Deck
	PlayerHand  *utils.Hand
	DealerHand  *utils.Hand
	Results     GameResult
	Doubled     bool
}

// GameResult represents the result of a blackjack hand
type GameResult struct {
	Result    string
	Payout    float64
	PlayerValue int
	DealerValue int
}

// NewBlackjackGame creates a new blackjack game instance
func NewBlackjackGame(session *discordgo.Session, interaction *discordgo.InteractionCreate, bet int64) *BlackjackGame {
	baseGame := utils.NewBaseGame(session, interaction, bet, "blackjack")
	
	gameID := fmt.Sprintf("blackjack_%d_%d", baseGame.UserID, time.Now().Unix())
	
	game := &BlackjackGame{
		BaseGame:   baseGame,
		GameID:     gameID,
		Bet:        bet,
		Deck:       utils.NewDeck(utils.DeckCount, "blackjack"),
		PlayerHand: utils.NewHand("blackjack"),
		DealerHand: utils.NewHand("blackjack"),
		Doubled:    false,
	}
	
	return game
}

// StartGame initializes the game and deals initial cards
func (bg *BlackjackGame) StartGame() error {
	// Deal initial cards
	bg.PlayerHand.AddCard(bg.Deck.Deal())
	bg.DealerHand.AddCard(bg.Deck.Deal())
	bg.PlayerHand.AddCard(bg.Deck.Deal())
	bg.DealerHand.AddCard(bg.Deck.Deal())
	
	// Check for natural blackjack
	if bg.PlayerHand.IsBlackjack() {
		return bg.finishGame()
	}
	
	// Send initial game state
	embed := bg.createGameEmbed(false)
	components := bg.getGameComponents()
	
	return utils.SendInteractionResponse(bg.Session, bg.Interaction, embed, components, false)
}

// HandleHit handles the player hitting
func (bg *BlackjackGame) HandleHit() error {
	if bg.IsGameOver() {
		return fmt.Errorf("game is already over")
	}
	
	bg.PlayerHand.AddCard(bg.Deck.Deal())
	
	// Check if hand is busted or has 5 cards (Charlie rule)
	if bg.PlayerHand.IsBusted() || bg.PlayerHand.Count() >= 5 {
		return bg.finishGame()
	}
	
	return bg.updateGameState()
}

// HandleStand handles the player standing
func (bg *BlackjackGame) HandleStand() error {
	if bg.IsGameOver() {
		return fmt.Errorf("game is already over")
	}
	
	return bg.finishGame()
}

// HandleDouble handles the player doubling down
func (bg *BlackjackGame) HandleDouble() error {
	if bg.IsGameOver() {
		return fmt.Errorf("game is already over")
	}
	
	// Check if player can afford to double
	if bg.UserData.Chips < bg.Bet {
		return fmt.Errorf("insufficient chips to double down")
	}
	
	// Double the bet
	bg.Bet *= 2
	bg.Doubled = true
	
	// Deal one card and stand
	bg.PlayerHand.AddCard(bg.Deck.Deal())
	
	return bg.finishGame()
}

// finishGame completes the game and calculates results
func (bg *BlackjackGame) finishGame() error {
	// Play dealer hand
	bg.playDealerHand()
	
	// Calculate results
	result := bg.calculateResult()
	bg.Results = result
	
	// Calculate profit
	profit := int64(float64(bg.Bet) * result.Payout) - bg.Bet
	
	// End the game and update user stats
	updatedUser, err := bg.EndGame(profit)
	if err != nil {
		return fmt.Errorf("failed to end game: %w", err)
	}
	
	bg.UserData = updatedUser
	
	// Update the game state one final time
	return bg.updateGameState()
}

// playDealerHand plays out the dealer's hand according to rules
func (bg *BlackjackGame) playDealerHand() {
	// Don't play dealer hand if player busted or has blackjack
	if bg.PlayerHand.IsBusted() {
		return
	}
	
	// Dealer hits on soft 17 or less
	for bg.DealerHand.GetValue() < utils.DealerStandValue {
		bg.DealerHand.AddCard(bg.Deck.Deal())
	}
}

// calculateResult determines the outcome of the game
func (bg *BlackjackGame) calculateResult() GameResult {
	playerValue := bg.PlayerHand.GetValue()
	dealerValue := bg.DealerHand.GetValue()
	
	result := GameResult{
		PlayerValue: playerValue,
		DealerValue: dealerValue,
	}
	
	// Player busted
	if bg.PlayerHand.IsBusted() {
		result.Result = "Bust"
		result.Payout = 0
		return result
	}
	
	// Player has 5-card Charlie
	if bg.PlayerHand.Count() >= 5 && playerValue <= 21 {
		result.Result = "Five Card Charlie"
		result.Payout = utils.FiveCardCharliePayout + 1
		return result
	}
	
	// Player has blackjack
	if bg.PlayerHand.IsBlackjack() && !bg.DealerHand.IsBlackjack() {
		result.Result = "Blackjack"
		result.Payout = utils.BlackjackPayout + 1
		return result
	}
	
	// Dealer busted
	if bg.DealerHand.IsBusted() {
		result.Result = "Win"
		result.Payout = 2
		return result
	}
	
	// Both have blackjack
	if bg.PlayerHand.IsBlackjack() && bg.DealerHand.IsBlackjack() {
		result.Result = "Push"
		result.Payout = 1
		return result
	}
	
	// Compare values
	if playerValue > dealerValue {
		result.Result = "Win"
		result.Payout = 2
	} else if playerValue < dealerValue {
		result.Result = "Lose"
		result.Payout = 0
	} else {
		result.Result = "Push"
		result.Payout = 1
	}
	
	return result
}

// createGameEmbed creates the game embed
func (bg *BlackjackGame) createGameEmbed(gameOver bool) *discordgo.MessageEmbed {
	playerValue := bg.PlayerHand.GetValue()
	dealerValue := bg.DealerHand.GetValue()
	
	// Format hands
	playerCards := bg.formatHand(bg.PlayerHand)
	var dealerCards string
	
	if gameOver {
		dealerCards = bg.formatHand(bg.DealerHand)
	} else {
		// Hide dealer hole card
		if len(bg.DealerHand.Cards) >= 2 {
			dealerCards = bg.DealerHand.Cards[0].String() + " 🎴"
		} else {
			dealerCards = bg.formatHand(bg.DealerHand)
		}
	}
	
	embed := &discordgo.MessageEmbed{
		Title: "🃏 Blackjack",
		Color: utils.BotColor,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "💰 Bet",
				Value:  utils.FormatChips(bg.Bet),
				Inline: true,
			},
			{
				Name:   "🎯 Your Hand",
				Value:  fmt.Sprintf("%s\n**Value: %d**", playerCards, playerValue),
				Inline: true,
			},
			{
				Name:   "🏠 Dealer",
				Value:  fmt.Sprintf("%s", dealerCards),
				Inline: true,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
	
	if gameOver {
		embed.Fields[2].Value = fmt.Sprintf("%s\n**Value: %d**", dealerCards, dealerValue)
		
		// Add result field
		resultValue := bg.Results.Result
		if bg.Results.Payout > 1 {
			profit := int64(float64(bg.Bet) * bg.Results.Payout) - bg.Bet
			resultValue += fmt.Sprintf("\n**Payout: %s**", utils.FormatChips(profit))
		}
		
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "🎊 Result",
			Value:  resultValue,
			Inline: false,
		})
		
		// Set color based on result
		if strings.Contains(bg.Results.Result, "Win") || strings.Contains(bg.Results.Result, "Blackjack") || strings.Contains(bg.Results.Result, "Charlie") {
			embed.Color = 0x00ff00 // Green for win
		} else if bg.Results.Result == "Push" {
			embed.Color = 0xffff00 // Yellow for push
		} else {
			embed.Color = 0xff0000 // Red for loss
		}
	}
	
	return embed
}

// formatHand formats a hand for display
func (bg *BlackjackGame) formatHand(hand *utils.Hand) string {
	cards := make([]string, len(hand.Cards))
	for i, card := range hand.Cards {
		cards[i] = card.String()
	}
	return strings.Join(cards, " ")
}

// getGameComponents returns the action buttons for the game
func (bg *BlackjackGame) getGameComponents() []discordgo.MessageComponent {
	if bg.IsGameOver() {
		return []discordgo.MessageComponent{}
	}
	
	buttons := []discordgo.MessageComponent{
		&discordgo.Button{
			CustomID: fmt.Sprintf("blackjack_hit_%s", bg.GameID),
			Label:    "Hit",
			Style:    discordgo.PrimaryButton,
			Emoji:    &discordgo.ComponentEmoji{Name: "🃏"},
		},
		&discordgo.Button{
			CustomID: fmt.Sprintf("blackjack_stand_%s", bg.GameID),
			Label:    "Stand",
			Style:    discordgo.SecondaryButton,
			Emoji:    &discordgo.ComponentEmoji{Name: "✋"},
		},
	}
	
	// Double down button (only on first two cards)
	if bg.PlayerHand.Count() == 2 && !bg.Doubled {
		buttons = append(buttons, &discordgo.Button{
			CustomID: fmt.Sprintf("blackjack_double_%s", bg.GameID),
			Label:    "Double",
			Style:    discordgo.SuccessButton,
			Emoji:    &discordgo.ComponentEmoji{Name: "💰"},
		})
	}
	
	return []discordgo.MessageComponent{
		&discordgo.ActionsRow{
			Components: buttons,
		},
	}
}

// updateGameState updates the game state in Discord
func (bg *BlackjackGame) updateGameState() error {
	embed := bg.createGameEmbed(bg.IsGameOver())
	components := bg.getGameComponents()
	
	return utils.EditInteractionResponse(bg.Session, bg.Interaction, embed, components)
}

// RegisterBlackjackCommands registers the blackjack slash command
func RegisterBlackjackCommands() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "blackjack",
		Description: "Play a game of blackjack",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "bet",
				Description: "Your bet amount (supports 'all', 'half', percentages like '50%', or exact amounts)",
				Required:    true,
			},
		},
	}
}

// HandleBlackjackCommand handles the blackjack slash command
func HandleBlackjackCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
	
	// Get bet amount
	betStr := i.ApplicationCommandData().Options[0].StringValue()
	
	// Get user data
	user, err := utils.GetUser(userID)
	if err != nil {
		respondError(s, i, "Failed to get user data. Please try again.")
		return
	}
	
	// Parse bet
	bet, err := utils.ParseBet(betStr, user.Chips)
	if err != nil {
		respondError(s, i, fmt.Sprintf("Invalid bet: %v", err))
		return
	}
	
	// Validate bet
	if bet <= 0 {
		respondError(s, i, "Bet must be greater than 0.")
		return
	}
	
	if bet > user.Chips {
		respondError(s, i, fmt.Sprintf("Insufficient chips. You have %s.", utils.FormatChips(user.Chips)))
		return
	}
	
	// Check for existing game
	gameKey := fmt.Sprintf("%d", userID)
	blackjackMutex.RLock()
	if _, exists := ActiveBlackjackGames[gameKey]; exists {
		blackjackMutex.RUnlock()
		respondError(s, i, "You already have an active blackjack game!")
		return
	}
	blackjackMutex.RUnlock()
	
	// Create new game
	game := NewBlackjackGame(s, i, bet)
	
	// Validate bet
	if err := game.ValidateBet(); err != nil {
		respondError(s, i, fmt.Sprintf("Invalid bet: %v", err))
		return
	}
	
	// Store game
	blackjackMutex.Lock()
	ActiveBlackjackGames[gameKey] = game
	blackjackMutex.Unlock()
	
	// Deduct bet from user chips
	_, err = utils.UpdateUser(userID, utils.UserUpdateData{
		ChipsIncrement: -bet,
	})
	if err != nil {
		blackjackMutex.Lock()
		delete(ActiveBlackjackGames, gameKey)
		blackjackMutex.Unlock()
		respondError(s, i, "Failed to process bet. Please try again.")
		return
	}
	
	// Start the game
	if err := game.StartGame(); err != nil {
		// Refund bet on error
		utils.UpdateUser(userID, utils.UserUpdateData{
			ChipsIncrement: bet,
		})
		blackjackMutex.Lock()
		delete(ActiveBlackjackGames, gameKey)
		blackjackMutex.Unlock()
		respondError(s, i, fmt.Sprintf("Failed to start game: %v", err))
		return
	}
	
	log.Printf("Started blackjack game for user %d with bet %d", userID, bet)
}

// HandleBlackjackInteraction handles button interactions for blackjack
func HandleBlackjackInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	parts := strings.Split(customID, "_")
	
	if len(parts) < 3 {
		return
	}
	
	action := parts[1]
	gameID := parts[2]
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
	gameKey := fmt.Sprintf("%d", userID)
	
	// Get game
	blackjackMutex.RLock()
	game, exists := ActiveBlackjackGames[gameKey]
	blackjackMutex.RUnlock()
	
	if !exists {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Game not found or expired.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	
	if game.GameID != gameID {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Invalid game session.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}
	
	// Acknowledge the interaction
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
	
	// Handle the action
	var err error
	switch action {
	case "hit":
		err = game.HandleHit()
	case "stand":
		err = game.HandleStand()
	case "double":
		err = game.HandleDouble()
	default:
		return
	}
	
	if err != nil {
		log.Printf("Error handling blackjack action %s: %v", action, err)
		return
	}
	
	// Clean up game if it's over
	if game.IsGameOver() {
		blackjackMutex.Lock()
		delete(ActiveBlackjackGames, gameKey)
		blackjackMutex.Unlock()
		log.Printf("Finished blackjack game for user %d", userID)
	}
}

// respondError sends an error response
func respondError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "❌ " + message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}