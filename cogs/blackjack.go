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

// ActiveGames stores currently active blackjack games
var ActiveGames = make(map[string]*BlackjackGame)
var gamesMutex sync.RWMutex

// BlackjackGame represents a blackjack game instance (matches Python BlackjackGameLogic)
type BlackjackGame struct {
	*utils.BaseGame
	GameID        string
	Bets          []int64
	Deck          *utils.Deck
	PlayerHands   []*utils.Hand
	DealerHand    *utils.Hand
	CurrentHand   int
	InsuranceBet  int64
	Results       []string
	View          *utils.BlackjackView
	CreatedAt     time.Time
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
		Results:       make([]string, 0),
		View:          utils.NewBlackjackView(baseGame.UserID, gameID),
		CreatedAt:     time.Now(),
	}
	
	return game
}

// StartGame initializes the game and deals initial cards (matches Python start_game)
func (bg *BlackjackGame) StartGame() error {
	// Deal initial cards
	bg.PlayerHands[0].AddCard(bg.Deck.Deal())
	bg.DealerHand.AddCard(bg.Deck.Deal())
	bg.PlayerHands[0].AddCard(bg.Deck.Deal())
	bg.DealerHand.AddCard(bg.Deck.Deal())
	
	// Check for natural blackjack
	playerValue := bg.PlayerHands[0].GetValue()
	
	// Update view options
	bg.updateViewOptions()
	
	if playerValue == 21 {
		// Player has blackjack, finish the game immediately
		return bg.finishGame()
	}
	
	// Send initial game state
	embed := bg.createGameEmbed(false)
	components := bg.View.GetComponents()
	
	return utils.SendInteractionResponse(bg.Session, bg.Interaction, embed, components, false)
}

// HandleHit handles the player hitting (matches Python handle_hit)
func (bg *BlackjackGame) HandleHit() error {
	if bg.IsGameOver() {
		return fmt.Errorf("game is already over")
	}
	
	currentHand := bg.PlayerHands[bg.CurrentHand]
	currentHand.AddCard(bg.Deck.Deal())
	
	playerScore := currentHand.GetValue()
	
	// Check for split ace restriction (matches Python logic)
	if len(bg.PlayerHands) > 1 && currentHand.Cards[0].IsAce() {
		return bg.standCurrentHand()
	}
	
	// Check various end conditions
	if playerScore > 21 {
		return bg.standCurrentHand() // Bust
	} else if len(currentHand.Cards) == 5 {
		// 5-card charlie is an auto win
		return bg.handleFiveCardCharlie()
	} else if playerScore == 21 {
		return bg.standCurrentHand() // 21
	} else {
		bg.updateViewOptions()
		return bg.updateGameState()
	}
}

// HandleStand handles the player standing (matches Python handle_stand)
func (bg *BlackjackGame) HandleStand() error {
	if bg.IsGameOver() {
		return fmt.Errorf("game is already over")
	}
	
	return bg.standCurrentHand()
}

// HandleDouble handles the player doubling down (matches Python handle_double_down)
func (bg *BlackjackGame) HandleDouble() error {
	if bg.IsGameOver() {
		return fmt.Errorf("game is already over")
	}
	
	betToDouble := bg.Bets[bg.CurrentHand]
	totalBetRequired := sumBets(bg.Bets) + betToDouble
	
	// Check if player can afford to double
	if bg.UserData.Chips < totalBetRequired {
		return fmt.Errorf("insufficient chips to double down")
	}
	
	// Double the bet
	bg.Bets[bg.CurrentHand] += betToDouble
	
	// Deal one card and stand
	currentHand := bg.PlayerHands[bg.CurrentHand]
	currentHand.AddCard(bg.Deck.Deal())
	
	return bg.standCurrentHand()
}

// HandleSplit handles the player splitting (matches Python handle_split)
func (bg *BlackjackGame) HandleSplit() error {
	if bg.IsGameOver() {
		return fmt.Errorf("game is already over")
	}
	
	currentHand := bg.PlayerHands[bg.CurrentHand]
	if !currentHand.CanSplit() {
		return fmt.Errorf("cannot split this hand")
	}
	
	betCost := bg.Bets[0]
	totalBetRequired := sumBets(bg.Bets) + betCost
	
	// Check if player can afford to split
	if bg.UserData.Chips < totalBetRequired {
		return fmt.Errorf("insufficient chips to split")
	}
	
	// Split the hand (matches Python logic)
	secondCard := currentHand.Cards[1]
	currentHand.Cards = []*utils.Card{currentHand.Cards[0]} // Keep first card
	
	// Create new hand with the second card
	newHand := utils.NewHand("blackjack")
	newHand.AddCard(secondCard)
	
	// Deal new cards to both hands
	currentHand.AddCard(bg.Deck.Deal())
	newHand.AddCard(bg.Deck.Deal())
	
	// Update game state
	bg.PlayerHands = append(bg.PlayerHands, newHand)
	bg.Bets = append(bg.Bets, bg.Bets[bg.CurrentHand])
	
	bg.updateViewOptions()
	return bg.updateGameState()
}

// HandleInsurance handles the player taking insurance (matches Python handle_insurance)
func (bg *BlackjackGame) HandleInsurance() error {
	if bg.IsGameOver() {
		return fmt.Errorf("game is already over")
	}
	
	insuranceCost := bg.Bets[0] / 2
	totalBetRequired := sumBets(bg.Bets) + insuranceCost
	
	if bg.UserData.Chips < totalBetRequired {
		return fmt.Errorf("insufficient chips for insurance")
	}
	
	bg.InsuranceBet = insuranceCost
	bg.updateViewOptions()
	return bg.updateGameState()
}

// handleFiveCardCharlie handles 5-card charlie as an immediate auto win (matches Python)
func (bg *BlackjackGame) handleFiveCardCharlie() error {
	if len(bg.PlayerHands) > 1 && bg.CurrentHand < len(bg.PlayerHands)-1 {
		// If there are multiple hands, move to the next hand
		bg.CurrentHand++
		bg.updateViewOptions()
		return bg.updateGameState()
	} else {
		// End the game immediately - 5-card charlie is an auto win
		return bg.finishGame()
	}
}

// standCurrentHand moves to the next hand or finishes the game (matches Python)
func (bg *BlackjackGame) standCurrentHand() error {
	if len(bg.PlayerHands) > 1 && bg.CurrentHand < len(bg.PlayerHands)-1 {
		bg.CurrentHand++
		bg.updateViewOptions()
		return bg.updateGameState()
	} else {
		// Check if any hands are not busted before dealer plays
		if bg.anyHandNotBusted() {
			return bg.dealerTurn()
		} else {
			return bg.finishGame()
		}
	}
}

// dealerTurn plays the dealer's hand (matches Python dealer_turn)
func (bg *BlackjackGame) dealerTurn() error {
	// Update view to show dealer's hidden card
	bg.updateGameState()
	time.Sleep(500 * time.Millisecond) // Small delay like Python
	
	// Dealer plays: hit on soft 17 and below, stand on hard 17 and above
	for bg.DealerHand.GetValue() < utils.DealerStandValue {
		bg.DealerHand.AddCard(bg.Deck.Deal())
		bg.updateGameState()
		time.Sleep(300 * time.Millisecond) // Small delay for card draws
	}
	
	return bg.finishGame()
}

// finishGame completes the game and calculates results (matches Python end_game)
func (bg *BlackjackGame) finishGame() error {
	totalProfit := int64(0)
	
	// Calculate results for each hand (matches Python _get_hand_result logic)
	for i, hand := range bg.PlayerHands {
		result, payout := bg.calculateHandResult(hand, i)
		
		if len(bg.PlayerHands) > 1 {
			bg.Results = append(bg.Results, fmt.Sprintf("Hand %d: %s", i+1, result))
		} else {
			bg.Results = append(bg.Results, result)
		}
		totalProfit += payout
	}
	
	// Handle insurance (matches Python logic)
	if bg.InsuranceBet > 0 {
		isDealerBlackjack := bg.DealerHand.GetValue() == 21 && len(bg.DealerHand.Cards) == 2
		if isDealerBlackjack {
			insurancePayout := bg.InsuranceBet * 2
			totalProfit += insurancePayout
			bg.Results = append(bg.Results, fmt.Sprintf("Insurance pays out %d!", insurancePayout))
		} else {
			totalProfit -= bg.InsuranceBet
			bg.Results = append(bg.Results, "Insurance lost.")
		}
	}
	
	// End the base game (updates user data, XP, etc.)
	updatedUser, err := bg.EndGame(totalProfit)
	if err != nil {
		return fmt.Errorf("failed to end game: %w", err)
	}
	
	// Update the user data
	bg.UserData = updatedUser
	
	// Calculate XP gain (matches Python logic)
	xpGain := int64(0)
	if totalProfit > 0 {
		xpGain = int64(float64(totalProfit) * utils.XPPerProfit)
	}
	
	// Check for rank up (matches Python)
	if xpGain > 0 {
		// This would need rank checking logic similar to Python
		// For now, simplified
	}
	
	// Send final game state with all results
	outcomeText := strings.Join(bg.Results, "\n")
	embed := bg.createGameEmbed(true)
	
	// Update embed with game over info
	embed = bg.addGameOverInfo(embed, outcomeText, updatedUser.Chips, totalProfit, xpGain)
	
	components := bg.View.DisableAllButtons()
	
	// Clean up the game
	gamesMutex.Lock()
	delete(ActiveGames, bg.GameID)
	gamesMutex.Unlock()
	
	return utils.UpdateComponentInteraction(bg.Session, bg.Interaction, embed, components)
}

// calculateHandResult calculates the result for a specific hand (matches Python _get_hand_result)
func (bg *BlackjackGame) calculateHandResult(hand *utils.Hand, handIndex int) (string, int64) {
	playerValue := hand.GetValue()
	dealerValue := bg.DealerHand.GetValue()
	bet := bg.Bets[handIndex]
	
	isPlayerBlackjack := playerValue == 21 && len(hand.Cards) == 2
	isDealerBlackjack := dealerValue == 21 && len(bg.DealerHand.Cards) == 2
	
	// Check for player bust
	if playerValue > 21 {
		return "Bust! You lost.", -bet
	}
	
	// Check for five card charlie
	if len(hand.Cards) == 5 && playerValue <= 21 {
		return "5-Card Charlie! You win!", int64(float64(bet) * utils.FiveCardCharliePayout)
	}
	
	// Check for blackjack
	if isPlayerBlackjack && !isDealerBlackjack {
		return "Blackjack! You win!", int64(float64(bet) * utils.BlackjackPayout)
	}
	if isDealerBlackjack && !isPlayerBlackjack {
		return "Dealer has Blackjack. You lose.", -bet
	}
	if isPlayerBlackjack && isDealerBlackjack {
		return "Push.", 0 // Both have blackjack
	}
	
	// Check for dealer bust
	if dealerValue > 21 {
		return "Dealer busts! You win!", bet
	}
	
	// Compare values
	if playerValue > dealerValue {
		return "You win!", bet
	} else if dealerValue > playerValue {
		return "Dealer wins.", -bet
	} else {
		return "Push.", 0
	}
}

// anyHandNotBusted checks if any player hands are not busted
func (bg *BlackjackGame) anyHandNotBusted() bool {
	for _, hand := range bg.PlayerHands {
		if hand.GetValue() <= 21 {
			return true
		}
	}
	return false
}

// updateViewOptions updates the available actions based on game state (matches Python)
func (bg *BlackjackGame) updateViewOptions() {
	if bg.IsGameOver() || bg.CurrentHand >= len(bg.PlayerHands) {
		bg.View.CanHit = false
		bg.View.CanStand = false
		bg.View.CanDouble = false
		bg.View.CanSplit = false
		bg.View.CanInsure = false
		return
	}
	
	currentHand := bg.PlayerHands[bg.CurrentHand]
	playerScore := currentHand.GetValue()
	
	// Basic actions
	bg.View.CanHit = playerScore <= 21
	bg.View.CanStand = true
	
	// Double down: only on first two cards, valid scores, and if player has enough chips
	bg.View.CanDouble = len(currentHand.Cards) == 2 && 
		(playerScore == 9 || playerScore == 10 || playerScore == 11) &&
		bg.UserData.Chips >= bg.Bets[bg.CurrentHand]
	
	// Split: only on first two cards of same rank and if player has enough chips
	bg.View.CanSplit = len(currentHand.Cards) == 2 && 
		currentHand.CanSplit() && 
		len(bg.PlayerHands) == 1 && // Can only split once
		bg.UserData.Chips >= bg.Bets[bg.CurrentHand]
	
	// Insurance: dealer shows ace, player has 2 cards, no insurance yet
	bg.View.CanInsure = len(bg.DealerHand.Cards) > 0 &&
		bg.DealerHand.Cards[0].IsAce() &&
		len(currentHand.Cards) == 2 &&
		bg.InsuranceBet == 0
}

// updateGameState updates the game state display
func (bg *BlackjackGame) updateGameState() error {
	embed := bg.createGameEmbed(false)
	components := bg.View.GetComponents()
	
	return utils.UpdateComponentInteraction(bg.Session, bg.Interaction, embed, components)
}

// createGameEmbed creates the Discord embed for the game state (matches Python create_game_embed)
func (bg *BlackjackGame) createGameEmbed(gameOver bool) *discordgo.MessageEmbed {
	// Convert hands to the format expected by the embed function
	var playerHands []utils.HandData
	
	for i, hand := range bg.PlayerHands {
		handCards := make([]string, len(hand.Cards))
		for j, card := range hand.Cards {
			handCards[j] = card.String()
		}
		
		playerHands = append(playerHands, utils.HandData{
			Hand:     handCards,
			Score:    hand.GetValue(),
			IsActive: i == bg.CurrentHand && !gameOver,
		})
	}
	
	// Dealer hand
	var dealerHand []string
	dealerValue := 0
	
	if gameOver {
		// Show all dealer cards
		for _, card := range bg.DealerHand.Cards {
			dealerHand = append(dealerHand, card.String())
		}
		dealerValue = bg.DealerHand.GetValue()
	} else {
		// Show first card, hide second
		if len(bg.DealerHand.Cards) > 0 {
			dealerHand = append(dealerHand, bg.DealerHand.Cards[0].String())
			dealerHand = append(dealerHand, "??")
			dealerValue = bg.DealerHand.Cards[0].GetValue("blackjack")
		}
	}
	
	// Check if any hands have aces for the clarification text
	hasAces := false
	for _, hand := range bg.PlayerHands {
		for _, card := range hand.Cards {
			if card.IsAce() {
				hasAces = true
				break
			}
		}
		if hasAces {
			break
		}
	}
	
	totalBet := sumBets(bg.Bets)
	
	return utils.BlackjackGameEmbed(
		playerHands,
		dealerHand,
		dealerValue,
		totalBet,
		gameOver,
		"", // outcomeText - will be filled by addGameOverInfo
		0,  // newBalance - will be filled by addGameOverInfo
		0,  // profit - will be filled by addGameOverInfo
		0,  // xpGain - will be filled by addGameOverInfo
		hasAces,
	)
}

// addGameOverInfo adds game over information to the embed
func (bg *BlackjackGame) addGameOverInfo(embed *discordgo.MessageEmbed, outcomeText string, newBalance, profit, xpGain int64) *discordgo.MessageEmbed {
	// This function would modify the embed to add outcome, winnings/losses, new balance, etc.
	// For now, create a new embed with the game over info
	
	// Convert hands for display
	var playerHands []utils.HandData
	for i, hand := range bg.PlayerHands {
		handCards := make([]string, len(hand.Cards))
		for j, card := range hand.Cards {
			handCards[j] = card.String()
		}
		
		playerHands = append(playerHands, utils.HandData{
			Hand:     handCards,
			Score:    hand.GetValue(),
			IsActive: false, // Game is over
		})
	}
	
	// Dealer hand - show all cards since game is over
	var dealerHand []string
	for _, card := range bg.DealerHand.Cards {
		dealerHand = append(dealerHand, card.String())
	}
	
	hasAces := false
	for _, hand := range bg.PlayerHands {
		for _, card := range hand.Cards {
			if card.IsAce() {
				hasAces = true
				break
			}
		}
		if hasAces {
			break
		}
	}
	
	totalBet := sumBets(bg.Bets)
	
	return utils.BlackjackGameEmbed(
		playerHands,
		dealerHand,
		bg.DealerHand.GetValue(),
		totalBet,
		true, // gameOver
		outcomeText,
		newBalance,
		profit,
		xpGain,
		hasAces,
	)
}

// Helper function to sum all bets
func sumBets(bets []int64) int64 {
	total := int64(0)
	for _, bet := range bets {
		total += bet
	}
	return total
}

// RegisterBlackjackCommands returns the blackjack slash command
func RegisterBlackjackCommands() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "blackjack",
		Description: "Start a game of Blackjack.",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "bet",
				Description: "Chips to wager.",
				Required:    true,
			},
		},
	}
}

// HandleBlackjackCommand handles the /blackjack slash command
func HandleBlackjackCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Parse bet amount
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		respondWithError(s, i, "Bet amount is required")
		return
	}
	
	betStr := options[0].StringValue()
	
	userID, err := parseUserID(i.Member.User.ID)
	if err != nil {
		respondWithError(s, i, "Failed to parse user ID")
		return
	}
	
	// Check if user already has an active game
	gamesMutex.RLock()
	hasActiveGame := false
	for _, game := range ActiveGames {
		if game.UserID == userID {
			hasActiveGame = true
			break
		}
	}
	gamesMutex.RUnlock()
	
	if hasActiveGame {
		embed := utils.CreateBrandedEmbed(
			"❌ Error",
			"You already have an active game.",
			0xFF0000,
		)
		utils.SendInteractionResponse(s, i, embed, nil, true)
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
		embed := utils.InsufficientChipsEmbed(bet, user.Chips, fmt.Sprintf("that bet (%s chips)", utils.FormatChips(bet)))
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
	case "blackjack_insurance":
		actionErr = game.HandleInsurance()
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
	return strconv.ParseInt(discordID, 10, 64)
}

func respondWithError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	embed := utils.CreateBrandedEmbed(
		"❌ Error",
		message,
		0xFF0000,
	)
	
	utils.SendInteractionResponse(s, i, embed, nil, true)
}