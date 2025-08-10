package blackjack

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
)

// ActiveGames stores currently active blackjack games
var ActiveGames = make(map[string]*BlackjackGame)
var gamesMutex sync.RWMutex

// BlackjackGame represents a blackjack game instance
type BlackjackGame struct {
	*utils.BaseGame
	GameID        string
	Bets          []int64
	Deck          *utils.Deck
	PlayerHands   []*utils.Hand
	DealerHand    *utils.Hand
	CurrentHand   int
	InsuranceBet  int64
	Results       []GameResult
	View          *utils.BlackjackView
}

// GameResult represents the result of a blackjack hand
type GameResult struct {
	HandIndex int
	Result    string
	Payout    float64
}

// NewBlackjackGame creates a new blackjack game instance
func NewBlackjackGame(session *discordgo.Session, interaction *discordgo.InteractionCreate, bet int64) *BlackjackGame {
	baseGame := utils.NewBaseGame(session, interaction, bet, "blackjack")
	
	gameID := fmt.Sprintf("blackjack_%d_%d", baseGame.UserID, time.Now().Unix())
	
	game := &BlackjackGame{
		BaseGame:      baseGame,
		GameID:        gameID,
		Bets:          []int64{bet},
		Deck:          utils.NewDeck(utils.DeckCount, "blackjack"),
		PlayerHands:   []*utils.Hand{utils.NewHand("blackjack")},
		DealerHand:    utils.NewHand("blackjack"),
		CurrentHand:   0,
		InsuranceBet:  0,
		Results:       make([]GameResult, 0),
		View:          utils.NewBlackjackView(baseGame.UserID, gameID),
	}
	
	return game
}

// StartGame initializes the game and deals initial cards
func (bg *BlackjackGame) StartGame() error {
	// Deal initial cards
	bg.PlayerHands[0].AddCard(bg.Deck.Deal())
	bg.DealerHand.AddCard(bg.Deck.Deal())
	bg.PlayerHands[0].AddCard(bg.Deck.Deal())
	bg.DealerHand.AddCard(bg.Deck.Deal())
	
	// Check for natural blackjack
	playerValue := bg.PlayerHands[0].GetValue()
	dealerUpCard := bg.DealerHand.Cards[0]
	
	// Update view options
	bg.updateViewOptions()
	
	if playerValue == 21 {
		// Player has blackjack, finish the game immediately
		return bg.finishGame()
	}
	
	// Check if dealer has potential blackjack
	if dealerUpCard.IsAce() || dealerUpCard.IsTen() {
		// In a full implementation, you might offer insurance here
		// For now, just continue with normal play
	}
	
	// Send initial game state
	embed := bg.createGameEmbed(false)
	components := bg.View.GetComponents()
	
	return utils.SendInteractionResponse(bg.Session, bg.Interaction, embed, components, false)
}

// HandleHit handles the player hitting
func (bg *BlackjackGame) HandleHit() error {
	if bg.IsGameOver() {
		return fmt.Errorf("game is already over")
	}
	
	currentHand := bg.PlayerHands[bg.CurrentHand]
	currentHand.AddCard(bg.Deck.Deal())
	
	// Check if hand is busted or has 5 cards
	if currentHand.IsBust() || currentHand.Size() >= 5 {
		return bg.standCurrentHand()
	}
	
	bg.updateViewOptions()
	return bg.updateGameState()
}

// HandleStand handles the player standing
func (bg *BlackjackGame) HandleStand() error {
	if bg.IsGameOver() {
		return fmt.Errorf("game is already over")
	}
	
	return bg.standCurrentHand()
}

// HandleDouble handles the player doubling down
func (bg *BlackjackGame) HandleDouble() error {
	if bg.IsGameOver() {
		return fmt.Errorf("game is already over")
	}
	
	// Check if player can afford to double
	if bg.UserData.Chips < bg.Bets[bg.CurrentHand] {
		return fmt.Errorf("insufficient chips to double down")
	}
	
	// Double the bet
	bg.Bets[bg.CurrentHand] *= 2
	
	// Deal one card and stand
	currentHand := bg.PlayerHands[bg.CurrentHand]
	currentHand.AddCard(bg.Deck.Deal())
	
	return bg.standCurrentHand()
}

// HandleSplit handles the player splitting
func (bg *BlackjackGame) HandleSplit() error {
	if bg.IsGameOver() {
		return fmt.Errorf("game is already over")
	}
	
	currentHand := bg.PlayerHands[bg.CurrentHand]
	if !currentHand.CanSplit() {
		return fmt.Errorf("cannot split this hand")
	}
	
	// Check if player can afford to split
	if bg.UserData.Chips < bg.Bets[bg.CurrentHand] {
		return fmt.Errorf("insufficient chips to split")
	}
	
	// Split the hand
	hand1, hand2 := currentHand.Split()
	
	// Deal one card to each new hand
	hand1.AddCard(bg.Deck.Deal())
	hand2.AddCard(bg.Deck.Deal())
	
	// Update game state
	bg.PlayerHands[bg.CurrentHand] = hand1
	bg.PlayerHands = append(bg.PlayerHands, hand2)
	bg.Bets = append(bg.Bets, bg.Bets[bg.CurrentHand])
	
	bg.updateViewOptions()
	return bg.updateGameState()
}

// standCurrentHand moves to the next hand or finishes the game
func (bg *BlackjackGame) standCurrentHand() error {
	bg.CurrentHand++
	
	if bg.CurrentHand >= len(bg.PlayerHands) {
		// All hands completed, finish the game
		return bg.finishGame()
	}
	
	// Move to next hand
	bg.updateViewOptions()
	return bg.updateGameState()
}

// finishGame completes the game and calculates results
func (bg *BlackjackGame) finishGame() error {
	// Play dealer hand
	bg.playDealerHand()
	
	// Calculate results for each hand
	totalProfit := int64(0)
	
	for i, hand := range bg.PlayerHands {
		result := bg.calculateHandResult(hand, i)
		bg.Results = append(bg.Results, result)
		
		payout := int64(float64(bg.Bets[i]) * result.Payout)
		totalProfit += payout - bg.Bets[i] // Subtract original bet
	}
	
	// End the base game
	updatedUser, err := bg.EndGame(totalProfit)
	if err != nil {
		return fmt.Errorf("failed to end game: %w", err)
	}
	
	// Update the user data
	bg.UserData = updatedUser
	
	// Send final game state
	embed := bg.createGameEmbed(true)
	components := bg.View.DisableAllButtons()
	
	// Clean up the game
	gamesMutex.Lock()
	delete(ActiveGames, bg.GameID)
	gamesMutex.Unlock()
	
	return utils.UpdateComponentInteraction(bg.Session, bg.Interaction, embed, components)
}

// playDealerHand plays the dealer's hand according to standard rules
func (bg *BlackjackGame) playDealerHand() {
	// Check if any player hands are not busted
	anyPlayerNotBusted := false
	for _, hand := range bg.PlayerHands {
		if !hand.IsBust() {
			anyPlayerNotBusted = true
			break
		}
	}
	
	// If all players are busted, dealer doesn't play
	if !anyPlayerNotBusted {
		return
	}
	
	// Dealer plays: hit on soft 17 and below, stand on hard 17 and above
	for bg.DealerHand.GetValue() < utils.DealerStandValue {
		bg.DealerHand.AddCard(bg.Deck.Deal())
	}
}

// calculateHandResult calculates the result for a specific hand
func (bg *BlackjackGame) calculateHandResult(hand *utils.Hand, handIndex int) GameResult {
	playerValue := hand.GetValue()
	dealerValue := bg.DealerHand.GetValue()
	
	// Check for player bust
	if hand.IsBust() {
		return GameResult{
			HandIndex: handIndex,
			Result:    "Bust",
			Payout:    0.0,
		}
	}
	
	// Check for five card charlie
	if hand.IsFiveCardCharlie() {
		return GameResult{
			HandIndex: handIndex,
			Result:    "Five Card Charlie",
			Payout:    1.0 + utils.FiveCardCharliePayout,
		}
	}
	
	// Check for blackjack
	if hand.IsBlackjack() {
		if bg.DealerHand.IsBlackjack() {
			return GameResult{
				HandIndex: handIndex,
				Result:    "Push (Both Blackjack)",
				Payout:    1.0, // Return bet
			}
		}
		return GameResult{
			HandIndex: handIndex,
			Result:    "Blackjack",
			Payout:    1.0 + utils.BlackjackPayout,
		}
	}
	
	// Check for dealer bust
	if bg.DealerHand.IsBust() {
		return GameResult{
			HandIndex: handIndex,
			Result:    "Win (Dealer Bust)",
			Payout:    2.0, // Return bet + winnings
		}
	}
	
	// Compare values
	if playerValue > dealerValue {
		return GameResult{
			HandIndex: handIndex,
			Result:    "Win",
			Payout:    2.0,
		}
	} else if playerValue < dealerValue {
		return GameResult{
			HandIndex: handIndex,
			Result:    "Loss",
			Payout:    0.0,
		}
	} else {
		return GameResult{
			HandIndex: handIndex,
			Result:    "Push",
			Payout:    1.0, // Return bet
		}
	}
}

// updateViewOptions updates the available actions based on game state
func (bg *BlackjackGame) updateViewOptions() {
	if bg.IsGameOver() || bg.CurrentHand >= len(bg.PlayerHands) {
		bg.View.CanHit = false
		bg.View.CanStand = false
		bg.View.CanDouble = false
		bg.View.CanSplit = false
		return
	}
	
	currentHand := bg.PlayerHands[bg.CurrentHand]
	
	// Basic actions
	bg.View.CanHit = !currentHand.IsBust()
	bg.View.CanStand = true
	
	// Double down: only on first two cards and if player has enough chips
	bg.View.CanDouble = currentHand.Size() == 2 && bg.UserData.Chips >= bg.Bets[bg.CurrentHand]
	
	// Split: only on first two cards of same rank/value and if player has enough chips
	bg.View.CanSplit = currentHand.CanSplit() && bg.UserData.Chips >= bg.Bets[bg.CurrentHand]
}

// updateGameState updates the game state display
func (bg *BlackjackGame) updateGameState() error {
	embed := bg.createGameEmbed(false)
	components := bg.View.GetComponents()
	
	return utils.UpdateComponentInteraction(bg.Session, bg.Interaction, embed, components)
}

// createGameEmbed creates the Discord embed for the game state
func (bg *BlackjackGame) createGameEmbed(gameOver bool) *discordgo.MessageEmbed {
	// Convert hands to card slices for the embed function
	var playerHands [][]utils.Card
	var playerScores []int
	
	for _, hand := range bg.PlayerHands {
		playerHands = append(playerHands, hand.Cards)
		playerScores = append(playerScores, hand.GetValue())
	}
	
	embed := utils.BlackjackGameEmbed(
		playerHands,
		bg.DealerHand.Cards,
		playerScores,
		bg.DealerHand.GetValue(),
		bg.CurrentHand,
		gameOver,
	)
	
	// Add bet information
	totalBet := int64(0)
	for _, bet := range bg.Bets {
		totalBet += bet
	}
	
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "💰 Total Bet",
		Value:  fmt.Sprintf("%s %s", utils.FormatChips(totalBet), utils.ChipsEmoji),
		Inline: true,
	})
	
	// Add results if game is over
	if gameOver && len(bg.Results) > 0 {
		resultText := ""
		totalPayout := int64(0)
		
		for _, result := range bg.Results {
			payout := int64(float64(bg.Bets[result.HandIndex]) * result.Payout)
			totalPayout += payout
			
			if len(bg.Results) > 1 {
				resultText += fmt.Sprintf("Hand %d: %s (%s %s)\n",
					result.HandIndex+1,
					result.Result,
					utils.FormatChips(payout),
					utils.ChipsEmoji,
				)
			} else {
				resultText = fmt.Sprintf("%s (%s %s)",
					result.Result,
					utils.FormatChips(payout),
					utils.ChipsEmoji,
				)
			}
		}
		
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "🎯 Result",
			Value:  resultText,
			Inline: false,
		})
		
		profit := totalPayout - totalBet
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "💵 Profit/Loss",
			Value:  fmt.Sprintf("%s%s %s", getProfitPrefix(profit), utils.FormatChips(abs(profit)), utils.ChipsEmoji),
			Inline: true,
		})
		
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "💳 New Balance",
			Value:  fmt.Sprintf("%s %s", utils.FormatChips(bg.UserData.Chips), utils.ChipsEmoji),
			Inline: true,
		})
	}
	
	return embed
}

// Helper functions
func getProfitPrefix(profit int64) string {
	if profit > 0 {
		return "+"
	}
	return ""
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// HandleBlackjackCommand handles the /blackjack slash command
func HandleBlackjackCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Parse bet amount
	betOption := i.ApplicationCommandData().Options[0]
	betStr := betOption.StringValue()
	
	userID, err := parseUserID(i.Member.User.ID)
	if err != nil {
		respondWithError(s, i, "Failed to parse user ID")
		return
	}
	
	// Get user data to validate bet
	user, err := utils.GetCachedUser(userID)
	if err != nil {
		respondWithError(s, i, "Failed to get user data")
		return
	}
	
	// Parse and validate bet
	bet, err := utils.ParseBet(betStr, user.Chips)
	if err != nil {
		respondWithError(s, i, "Invalid bet amount: "+err.Error())
		return
	}
	
	if bet <= 0 {
		respondWithError(s, i, "Bet amount must be greater than 0")
		return
	}
	
	if user.Chips < bet {
		embed := utils.InsufficientChipsEmbed(bet, user.Chips, "blackjack")
		utils.SendInteractionResponse(s, i, embed, nil, true)
		return
	}
	
	// Create and start new game
	game := NewBlackjackGame(s, i, bet)
	game.UserData = user
	
	// Validate bet using base game
	if err := game.ValidateBet(); err != nil {
		respondWithError(s, i, err.Error())
		return
	}
	
	// Store game in active games
	gamesMutex.Lock()
	ActiveGames[game.GameID] = game
	gamesMutex.Unlock()
	
	// Start the game
	if err := game.StartGame(); err != nil {
		log.Printf("Failed to start blackjack game: %v", err)
		respondWithError(s, i, "Failed to start game")
		
		// Clean up failed game
		gamesMutex.Lock()
		delete(ActiveGames, game.GameID)
		gamesMutex.Unlock()
		return
	}
	
	log.Printf("Started blackjack game %s for user %d with bet %d", game.GameID, userID, bet)
}

// HandleBlackjackInteraction handles component interactions for blackjack games
func HandleBlackjackInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	userID, err := parseUserID(i.Member.User.ID)
	if err != nil {
		respondWithError(s, i, "Failed to parse user ID")
		return
	}
	
	// Find the user's active game
	gamesMutex.RLock()
	var game *BlackjackGame
	for _, g := range ActiveGames {
		if g.UserID == userID {
			game = g
			break
		}
	}
	gamesMutex.RUnlock()
	
	if game == nil {
		respondWithError(s, i, "No active blackjack game found")
		return
	}
	
	// Update the interaction reference
	game.Interaction = i
	
	// Handle the specific action
	var actionErr error
	switch customID {
	case "blackjack_hit":
		actionErr = game.HandleHit()
	case "blackjack_stand":
		actionErr = game.HandleStand()
	case "blackjack_double":
		actionErr = game.HandleDouble()
	case "blackjack_split":
		actionErr = game.HandleSplit()
	default:
		respondWithError(s, i, "Unknown blackjack action")
		return
	}
	
	if actionErr != nil {
		log.Printf("Blackjack action error: %v", actionErr)
		respondWithError(s, i, actionErr.Error())
		return
	}
	
	log.Printf("Processed blackjack action %s for game %s", customID, game.GameID)
}

// Helper functions
func parseUserID(discordID string) (int64, error) {
	// Convert Discord user ID (string) to int64
	// Discord IDs are 64-bit integers stored as strings
	return strconv.ParseInt(discordID, 10, 64)
}

func respondWithError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	embed := utils.CreateBrandedEmbed(
		"❌ Error",
		message,
		0xFF0000, // Red
	)
	
	utils.SendInteractionResponse(s, i, embed, nil, true)
}