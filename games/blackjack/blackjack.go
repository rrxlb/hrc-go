package blackjack

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

// Cleanup configuration for game state management
const (
	GameTimeoutDuration = 10 * time.Minute // Games timeout after 10 minutes of inactivity
	CleanupInterval     = 2 * time.Minute  // Run cleanup every 2 minutes
)

// Initialize cleanup routine - call this from main.go during bot startup
func init() {
	go startGameCleanup()
}

// startGameCleanup runs a background cleanup routine to remove stale games
func startGameCleanup() {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		cleanupExpiredGames()
	}
}

// cleanupExpiredGames removes games that have been inactive for too long
func cleanupExpiredGames() {
	gamesMutex.Lock()
	defer gamesMutex.Unlock()

	now := time.Now()
	expiredCount := 0

	for gameID, game := range ActiveGames {
		// Check if game has been inactive for too long
		if now.Sub(game.CreatedAt) > GameTimeoutDuration {
			delete(ActiveGames, gameID)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		log.Printf("Cleaned up %d expired blackjack games", expiredCount)
	}
}

// GameState represents the current state of interaction handling
type GameState int

const (
	StateInitial   GameState = iota // Game created, no responses sent
	StateDeferred                   // Interaction deferred, waiting for initial response
	StateActive                     // Game active with initial response sent
	StateRevealing                  // Dealer cards being revealed
	StateFinished                   // Game completed, all responses sent
)

// BlackjackGame represents a blackjack game instance
type BlackjackGame struct {
	*utils.BaseGame
	GameID              string
	Bets                []int64
	Deck                *utils.Deck
	PlayerHands         []*utils.Hand
	DealerHand          *utils.Hand
	CurrentHand         int
	InsuranceBet        int64
	Results             []GameResult
	NetProfit           int64
	View                *utils.BlackjackView
	OriginalInteraction *discordgo.InteractionCreate
	State               GameState // Simplified state management
	// Fallback editing support when webhook token expires
	ChannelID string
	MessageID string
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
		BaseGame:            baseGame,
		GameID:              gameID,
		Bets:                []int64{bet},
		Deck:                utils.NewDeck(utils.DeckCount, "blackjack"),
		PlayerHands:         []*utils.Hand{utils.NewHand("blackjack")},
		DealerHand:          utils.NewHand("blackjack"),
		CurrentHand:         0,
		InsuranceBet:        0,
		Results:             make([]GameResult, 0),
		View:                utils.NewBlackjackView(baseGame.UserID, gameID),
		OriginalInteraction: interaction,
		State:               StateInitial, // Start in initial state
		ChannelID:           interaction.ChannelID,
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

	// Update view options (include insurance availability check before potential early finish)
	bg.updateViewOptions()

	if playerValue == 21 {
		// Player has natural blackjack - finish immediately without revealing dealer's second card
		// This provides the fastest possible resolution for natural blackjacks
		return bg.finishNaturalBlackjack()
	}

	// Check if dealer shows Ace to allow insurance
	if dealerUpCard.IsAce() {
		bg.View.CanInsure = true
		bg.updateViewOptions() // re-evaluate with insurance available
	}

	// Send or update initial game state based on current state
	embed := bg.createGameEmbed(false)
	components := bg.View.GetComponents()

	var err error
	if bg.State == StateDeferred {
		// Interaction was deferred; update the original response
		err = utils.UpdateInteractionResponse(bg.Session, bg.OriginalInteraction, embed, components)
		if err == nil {
			bg.State = StateActive
		}
	} else {
		// No prior response; send initial response now
		err = utils.SendInteractionResponse(bg.Session, bg.Interaction, embed, components, false)
		if err == nil {
			bg.State = StateActive
		}
	}

	if err == nil {
		// Capture original message ID for fallback edits later (async to avoid blocking)
		go func(sess *discordgo.Session, inter *discordgo.InteractionCreate) {
			time.Sleep(200 * time.Millisecond)
			if m2, e2 := utils.GetOriginalResponseMessage(sess, inter); e2 == nil && m2 != nil {
				bg.ChannelID = m2.ChannelID
				bg.MessageID = m2.ID
			}
		}(bg.Session, bg.Interaction)
	}
	return err
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

	log.Printf("Processing double down for game %s, current state: %d", bg.GameID, bg.State)

	// Check if player can afford to double
	if bg.UserData.Chips < bg.Bets[bg.CurrentHand] {
		return fmt.Errorf("insufficient chips to double down")
	}

	// Double the bet
	bg.Bets[bg.CurrentHand] *= 2

	// Deal one card and stand
	currentHand := bg.PlayerHands[bg.CurrentHand]
	currentHand.AddCard(bg.Deck.Deal())

	log.Printf("After double down card deal - hand value: %d, proceeding to stand", currentHand.GetValue())
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

// HandleInsurance handles taking insurance when dealer shows an Ace
func (bg *BlackjackGame) HandleInsurance() error {
	if bg.IsGameOver() {
		return fmt.Errorf("game is already over")
	}
	// Already took insurance
	if bg.InsuranceBet > 0 {
		return fmt.Errorf("insurance already taken")
	}
	// Conditions: dealer upcard is Ace and current hand has exactly 2 cards
	if len(bg.DealerHand.Cards) == 0 || !bg.DealerHand.Cards[0].IsAce() {
		return fmt.Errorf("insurance not available")
	}
	currentHand := bg.PlayerHands[bg.CurrentHand]
	if currentHand.Size() != 2 {
		return fmt.Errorf("insurance only available on first two cards")
	}
	// Cost is half of original bet for this hand (integer division)
	cost := bg.Bets[bg.CurrentHand] / 2
	if cost <= 0 {
		return fmt.Errorf("invalid insurance cost")
	}
	// Ensure user can afford (total committed + cost <= chips)
	totalCommitted := int64(0)
	for _, b := range bg.Bets {
		totalCommitted += b
	}
	if bg.UserData.Chips < totalCommitted+cost {
		return fmt.Errorf("insufficient chips for insurance")
	}
	bg.InsuranceBet = cost
	// Disable insurance button after taking
	bg.View.CanInsure = false
	return bg.updateGameState()
}

// standCurrentHand moves to the next hand or finishes the game
func (bg *BlackjackGame) standCurrentHand() error {
	bg.CurrentHand++

	log.Printf("standCurrentHand: CurrentHand=%d, TotalHands=%d, State=%d", bg.CurrentHand, len(bg.PlayerHands), bg.State)

	if bg.CurrentHand >= len(bg.PlayerHands) {
		// All hands completed, finish the game
		log.Printf("All hands completed for game %s, finishing game", bg.GameID)
		return bg.finishGame()
	}

	// Move to next hand
	log.Printf("Moving to next hand for game %s", bg.GameID)
	bg.updateViewOptions()
	return bg.updateGameState()
}

// finishNaturalBlackjack handles immediate natural blackjack wins without revealing dealer cards
func (bg *BlackjackGame) finishNaturalBlackjack() error {
	// For natural blackjack, we don't reveal the dealer's second card or play dealer hand
	// Check if dealer also has blackjack by peeking at hole card
	dealerHasBlackjack := bg.DealerHand.IsBlackjack()

	var result GameResult
	var totalProfit int64

	if dealerHasBlackjack {
		// Push - both have blackjack
		result = GameResult{HandIndex: 0, Result: "Push - Both have Blackjack!", Payout: 1.0}
		totalProfit = 0 // No loss, no gain
	} else {
		// Player wins with natural blackjack
		result = GameResult{HandIndex: 0, Result: "Natural Blackjack! You win!", Payout: 1.0 + utils.BlackjackPayout}
		payout := int64(float64(bg.Bets[0]) * result.Payout)
		totalProfit = payout - bg.Bets[0]
	}

	bg.Results = []GameResult{result}
	bg.NetProfit = totalProfit

	// End the base game
	updatedUser, err := bg.EndGame(totalProfit)
	if err != nil {
		return fmt.Errorf("failed to end game: %w", err)
	}

	// Update the user data
	bg.UserData = updatedUser

	// Send final game state - use special natural blackjack embed that doesn't reveal dealer second card
	embed := bg.createNaturalBlackjackEmbed(dealerHasBlackjack)
	components := bg.View.DisableAllButtons()

	// Send response - this is for natural blackjack so no initial response was sent
	err = utils.SendInteractionResponse(bg.Session, bg.OriginalInteraction, embed, components, false)
	if err != nil {
		log.Printf("Failed to send natural blackjack response: %v", err)
	}

	// Mark game as finished and clean up
	bg.State = StateFinished
	gamesMutex.Lock()
	delete(ActiveGames, bg.GameID)
	gamesMutex.Unlock()

	return nil
}

// finishGame completes the game and calculates results
func (bg *BlackjackGame) finishGame() error {
	// Play dealer hand with animation
	if err := bg.playDealerHand(); err != nil {
		log.Printf("Error during dealer hand animation: %v", err)
		// Continue with game completion even if animation fails
	}

	// Calculate results for each hand
	totalProfit := int64(0)

	for i, hand := range bg.PlayerHands {
		result := bg.calculateHandResult(hand, i)
		bg.Results = append(bg.Results, result)

		payout := int64(float64(bg.Bets[i]) * result.Payout)
		totalProfit += payout - bg.Bets[i] // profit component
	}

	// Insurance resolution (Python parity): pays 2:1 if dealer blackjack; otherwise lost
	if bg.InsuranceBet > 0 {
		if bg.DealerHand.IsBlackjack() {
			payout := bg.InsuranceBet * 2
			totalProfit += payout
			bg.Results = append(bg.Results, GameResult{HandIndex: -1, Result: fmt.Sprintf("Insurance pays out %d!", payout), Payout: 0})
		} else {
			totalProfit -= bg.InsuranceBet
			bg.Results = append(bg.Results, GameResult{HandIndex: -1, Result: "Insurance lost.", Payout: 0})
		}
	}

	bg.NetProfit = totalProfit

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

	// Send final game state based on current state
	var errUpdate error

	if bg.State != StateFinished {
		if bg.Interaction.Type == discordgo.InteractionMessageComponent {
			// Component interaction - send update (this was likely a double down button)
			log.Printf("Sending final game state via component interaction for game %s", bg.GameID)
			errUpdate = utils.UpdateComponentInteraction(bg.Session, bg.Interaction, embed, components)
		} else if bg.State == StateDeferred || bg.State == StateActive {
			// Update deferred response
			errUpdate = utils.UpdateInteractionResponse(bg.Session, bg.OriginalInteraction, embed, components)
		} else {
			// Initial response needed
			errUpdate = utils.SendInteractionResponse(bg.Session, bg.OriginalInteraction, embed, components, false)
		}

		// If update fails, try fallback edit via channel message
		if errUpdate != nil {
			log.Printf("Failed to send final game state: %v", errUpdate)
			if bg.isWebhookExpired(errUpdate) {
				if fErr := bg.fallbackEdit(embed, components); fErr == nil {
					log.Printf("Successfully used fallback edit for game %s", bg.GameID)
					errUpdate = nil // Successful fallback
				} else {
					log.Printf("Fallback edit also failed for game %s: %v", bg.GameID, fErr)
				}
			}
		}
	}

	// Mark game as finished and clean up
	bg.State = StateFinished
	gamesMutex.Lock()
	delete(ActiveGames, bg.GameID)
	gamesMutex.Unlock()

	return nil
}

// playDealerHand plays the dealer's hand according to standard rules with minimal animation
func (bg *BlackjackGame) playDealerHand() error {
	// Set revealing state to disable player actions
	bg.State = StateRevealing

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
		return nil
	}

	// Dealer plays: hit on soft 17 and below, stand on hard 17 and above
	cardCount := 0
	maxCards := 10 // Safety limit to prevent infinite loops
	for bg.DealerHand.GetValue() < utils.DealerStandValue && cardCount < maxCards {
		// Deal next card
		bg.DealerHand.AddCard(bg.Deck.Deal())
		cardCount++
	}

	if cardCount >= maxCards {
		log.Printf("Warning: dealer hit maximum card limit in blackjack game %s", bg.GameID)
	}

	// Skip intermediate update - finishGame will send the final state
	// This prevents double interaction consumption issues

	return nil
}

// calculateHandResult calculates the result for a specific hand
func (bg *BlackjackGame) calculateHandResult(hand *utils.Hand, handIndex int) GameResult {
	playerValue := hand.GetValue()
	dealerValue := bg.DealerHand.GetValue()

	// Player bust
	if hand.IsBust() {
		return GameResult{HandIndex: handIndex, Result: "Bust! You lost.", Payout: 0.0}
	}
	// Five Card Charlie
	if hand.IsFiveCardCharlie() {
		return GameResult{HandIndex: handIndex, Result: "5-Card Charlie! You win!", Payout: 1.0 + utils.FiveCardCharliePayout}
	}
	// Player blackjack scenarios
	if hand.IsBlackjack() {
		if bg.DealerHand.IsBlackjack() {
			return GameResult{HandIndex: handIndex, Result: "Push.", Payout: 1.0}
		}
		return GameResult{HandIndex: handIndex, Result: "Blackjack! You win!", Payout: 1.0 + utils.BlackjackPayout}
	}
	// Dealer bust
	if bg.DealerHand.IsBust() {
		return GameResult{HandIndex: handIndex, Result: "Dealer busts! You win!", Payout: 2.0}
	}
	// Compare values
	if playerValue > dealerValue {
		return GameResult{HandIndex: handIndex, Result: "You win!", Payout: 2.0}
	}
	if playerValue < dealerValue {
		if bg.DealerHand.IsBlackjack() {
			return GameResult{HandIndex: handIndex, Result: "Dealer has Blackjack. You lose.", Payout: 0.0}
		}
		return GameResult{HandIndex: handIndex, Result: "Dealer wins.", Payout: 0.0}
	}
	return GameResult{HandIndex: handIndex, Result: "Push.", Payout: 1.0}
}

// updateViewOptions updates the available actions based on game state
func (bg *BlackjackGame) updateViewOptions() {
	if bg.IsGameOver() || bg.CurrentHand >= len(bg.PlayerHands) || bg.State == StateRevealing || bg.State == StateFinished {
		bg.View.CanHit = false
		bg.View.CanStand = false
		bg.View.CanDouble = false
		bg.View.CanSplit = false
		bg.View.CanInsure = false
		return
	}

	currentHand := bg.PlayerHands[bg.CurrentHand]

	// Basic actions
	bg.View.CanHit = !currentHand.IsBust()
	bg.View.CanStand = true

	// Double down: only on first two cards with value 9, 10, or 11, and if player can afford doubling that specific hand
	handValue := currentHand.GetValue()
	bg.View.CanDouble = currentHand.Size() == 2 &&
		(handValue == 9 || handValue == 10 || handValue == 11) &&
		bg.UserData.Chips >= (bg.Bets[bg.CurrentHand])

	// Split: only on first two cards of same rank and only if still single original hand
	bg.View.CanSplit = currentHand.CanSplit() && len(bg.PlayerHands) == 1 && bg.UserData.Chips >= bg.Bets[bg.CurrentHand]

	// Insurance: dealer shows Ace, first hand only, first two cards, insurance not already taken
	if bg.InsuranceBet == 0 && len(bg.DealerHand.Cards) > 0 && bg.DealerHand.Cards[0].IsAce() && currentHand.Size() == 2 {
		// ensure user can afford half bet in addition to current committed bets
		cost := bg.Bets[bg.CurrentHand] / 2
		totalCommitted := int64(0)
		for _, b := range bg.Bets {
			totalCommitted += b
		}
		bg.View.CanInsure = cost > 0 && bg.UserData.Chips >= (totalCommitted+cost)
	} else {
		bg.View.CanInsure = false
	}
}

// updateGameState updates the game state display
func (bg *BlackjackGame) updateGameState() error {
	// Skip update if game is finished
	if bg.State == StateFinished {
		return nil
	}

	embed := bg.createGameEmbed(false)
	components := bg.View.GetComponents()

	var err error
	if bg.Interaction.Type == discordgo.InteractionMessageComponent {
		// Component interactions need immediate response
		err = utils.UpdateComponentInteraction(bg.Session, bg.Interaction, embed, components)
	} else {
		// Update the deferred response
		err = utils.UpdateInteractionResponse(bg.Session, bg.OriginalInteraction, embed, components)
	}

	// If update fails, try fallback edit via channel message
	if err != nil && bg.isWebhookExpired(err) {
		if fErr := bg.fallbackEdit(embed, components); fErr == nil {
			// Successful fallback
			err = nil
		}
	}

	return err
}

// updateGameStateRevealing updates the game state during dealer card reveals
func (bg *BlackjackGame) updateGameStateRevealing() error {
	// Skip update if game is finished
	if bg.State == StateFinished {
		return nil
	}

	embed := bg.createGameEmbed(true)         // Show as game over to reveal dealer cards
	components := bg.View.DisableAllButtons() // Disable all buttons during reveal

	var err error
	if bg.Interaction.Type == discordgo.InteractionMessageComponent {
		// Component interactions need immediate response
		err = utils.UpdateComponentInteraction(bg.Session, bg.Interaction, embed, components)
	} else {
		// Update the deferred response
		err = utils.UpdateInteractionResponse(bg.Session, bg.OriginalInteraction, embed, components)
	}

	// If update fails, try fallback edit via channel message
	if err != nil && bg.isWebhookExpired(err) {
		if fErr := bg.fallbackEdit(embed, components); fErr == nil {
			// Successful fallback
			err = nil
		} else {
			// Log webhook expiration for debugging
			log.Printf("Discord webhook expired for blackjack game %s (user %d)", bg.GameID, bg.UserID)
		}
	}

	return err
}

// createNaturalBlackjackEmbed creates a special embed for natural blackjack that doesn't reveal dealer's second card
func (bg *BlackjackGame) createNaturalBlackjackEmbed(dealerHasBlackjack bool) *discordgo.MessageEmbed {
	// Build player hand data
	var playerHandData []utils.HandData
	hasAces := false
	hand := bg.PlayerHands[0]
	cards := make([]string, len(hand.Cards))
	for i, c := range hand.Cards {
		cards[i] = c.String()
		if c.IsAce() {
			hasAces = true
		}
	}
	playerHandData = append(playerHandData, utils.HandData{Hand: cards, Score: hand.GetValue(), IsActive: false})

	// For natural blackjack, we only show dealer's first card unless dealer also has blackjack
	var dealerCards []string
	var dealerValue int

	if dealerHasBlackjack {
		// Reveal both dealer cards for push scenario
		for _, c := range bg.DealerHand.Cards {
			dealerCards = append(dealerCards, c.String())
		}
		dealerValue = bg.DealerHand.GetValue()
	} else {
		// Only show dealer's first card - second card remains hidden
		dealerCards = append(dealerCards, bg.DealerHand.Cards[0].String())
		dealerCards = append(dealerCards, "??")
		dealerValue = bg.DealerHand.Cards[0].GetValue("blackjack")
	}

	totalBet := bg.Bets[0]

	// Outcome text for natural blackjack
	outcomeText := bg.Results[0].Result

	// Compute premium-gated XP for display
	xpGain := int64(0)
	if bg.NetProfit > 0 {
		xpGain = bg.NetProfit * utils.XPPerProfit
		if bg.BaseGame != nil && bg.BaseGame.UserData != nil && !utils.ShouldShowXPGained(bg.BaseGame.Interaction.Member, bg.BaseGame.UserData) {
			xpGain = 0
		}
	}

	embed := utils.BlackjackGameEmbed(
		playerHandData,
		dealerCards,
		dealerValue,
		totalBet,
		true, // gameOver = true for final state
		outcomeText,
		bg.UserData.Chips,
		bg.NetProfit,
		xpGain,
		hasAces,
	)

	return embed
}

// createGameEmbed creates the Discord embed for the game state
func (bg *BlackjackGame) createGameEmbed(gameOver bool) *discordgo.MessageEmbed {
	// Build HandData slice
	var playerHandData []utils.HandData
	hasAces := false
	for idx, hand := range bg.PlayerHands {
		cards := make([]string, len(hand.Cards))
		for i, c := range hand.Cards {
			cards[i] = c.String()
			if c.IsAce() {
				hasAces = true
			}
		}
		playerHandData = append(playerHandData, utils.HandData{Hand: cards, Score: hand.GetValue(), IsActive: idx == bg.CurrentHand && !gameOver})
	}

	// Dealer hand (hide second card if not game over)
	var dealerCards []string
	dealerValue := 0
	if gameOver {
		for _, c := range bg.DealerHand.Cards {
			dealerCards = append(dealerCards, c.String())
		}
		dealerValue = bg.DealerHand.GetValue()
	} else {
		if len(bg.DealerHand.Cards) > 0 {
			dealerCards = append(dealerCards, bg.DealerHand.Cards[0].String())
			dealerCards = append(dealerCards, "??")
			dealerValue = bg.DealerHand.Cards[0].GetValue("blackjack")
		}
	}

	totalBet := int64(0)
	for _, b := range bg.Bets {
		totalBet += b
	}

	// Outcome aggregation (Python parity)
	outcomeText := ""
	if gameOver && len(bg.Results) > 0 {
		lines := []string{}
		multi := len(bg.PlayerHands) > 1
		for _, r := range bg.Results {
			if r.HandIndex >= 0 {
				if multi {
					lines = append(lines, fmt.Sprintf("Hand %d: %s", r.HandIndex+1, r.Result))
				} else {
					lines = append(lines, r.Result)
				}
			} else {
				lines = append(lines, r.Result)
			}
		}
		outcomeText = strings.TrimSpace(strings.Join(lines, "\n"))
	}
	profit := int64(0)
	if gameOver {
		profit = bg.NetProfit
	}
	// Compute premium-gated XP for display
	xpGain := int64(0)
	if gameOver && profit > 0 {
		xpGain = profit * utils.XPPerProfit
		if bg.BaseGame != nil && bg.BaseGame.UserData != nil && !utils.ShouldShowXPGained(bg.BaseGame.Interaction.Member, bg.BaseGame.UserData) {
			xpGain = 0
		}
	}

	embed := utils.BlackjackGameEmbed(
		playerHandData,
		dealerCards,
		dealerValue,
		totalBet,
		gameOver,
		outcomeText,
		bg.UserData.Chips,
		profit,
		xpGain,
		hasAces,
	)

	return embed
}

// HandleBlackjackCommand handles the /blackjack slash command
func HandleBlackjackCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Defer immediately for sub-3ms response time - critical for performance
	if err := utils.DeferInteractionResponse(s, i, false); err != nil {
		// If deferral fails, fall back to normal error response path
		respondWithError(s, i, "Failed to acknowledge command")
		return
	}

	// All heavy work moved to async goroutine for optimal responsiveness
	go func(sess *discordgo.Session, inter *discordgo.InteractionCreate) {
		start := time.Now()

		// Parse bet amount
		betOption := inter.ApplicationCommandData().Options[0]
		betStr := betOption.StringValue()

		userID, err := parseUserID(inter.Member.User.ID)
		if err != nil {
			respondWithDeferredError(sess, inter, "Failed to parse user ID")
			return
		}

		// Get user data to validate bet
		user, err := utils.GetCachedUser(userID)
		if err != nil {
			respondWithDeferredError(sess, inter, "Failed to get user data")
			return
		}

		// Parse and validate bet
		bet, err := utils.ParseBet(betStr, user.Chips)
		if err != nil {
			respondWithDeferredError(sess, inter, "Invalid bet amount: "+err.Error())
			return
		}

		if bet <= 0 {
			respondWithDeferredError(sess, inter, "Bet amount must be greater than 0")
			return
		}

		if user.Chips < bet {
			embed := utils.InsufficientChipsEmbed(bet, user.Chips, "blackjack")
			utils.UpdateInteractionResponse(sess, inter, embed, nil)
			return
		}

		// Create and start new game (user data already validated)
		game := NewBlackjackGame(sess, inter, bet)
		game.UserData = user
		// Mark that the interaction was deferred
		game.State = StateDeferred

		// Store game in active games
		gamesMutex.Lock()
		ActiveGames[game.GameID] = game
		gamesMutex.Unlock()

		// Start the game
		if err := game.StartGame(); err != nil {
			log.Printf("Failed to start blackjack game: %v", err)
			respondWithDeferredError(sess, inter, "Failed to start game")

			// Clean up failed game
			gamesMutex.Lock()
			delete(ActiveGames, game.GameID)
			gamesMutex.Unlock()
			return
		}

		// Log performance for slow operations (>500ms is concerning for async work)
		duration := time.Since(start)
		if duration > 500*time.Millisecond {
			log.Printf("Slow blackjack game initialization: %dms (user: %d)", duration.Milliseconds(), userID)
		}
	}(s, i)
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
	// Capture message/channel for fallback edits
	if i.Message != nil {
		game.MessageID = i.Message.ID
		game.ChannelID = i.ChannelID
	}

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
		// Don't send error response if the action might have already responded
		// The action methods handle their own responses
		return
	}

	// Check if game was completed by the action (e.g., double down finishing the game)
	if game.State == StateFinished {
		log.Printf("Game %s completed after %s action", game.GameID, customID)
		return
	}

}

// isWebhookExpired checks for Discord webhook expiration errors
func (bg *BlackjackGame) isWebhookExpired(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Unknown Webhook") || strings.Contains(msg, "\"code\": 10015") || strings.Contains(msg, "404")
}

// fallbackEdit attempts to update the message via channel edit when webhook expired
func (bg *BlackjackGame) fallbackEdit(embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error {
	// Ensure we have IDs; try to pull from current interaction message as last resort
	if bg.ChannelID == "" || bg.MessageID == "" {
		if bg.Interaction != nil && bg.Interaction.Message != nil {
			bg.ChannelID = bg.Interaction.ChannelID
			bg.MessageID = bg.Interaction.Message.ID
		}
	}
	if bg.ChannelID == "" || bg.MessageID == "" {
		return fmt.Errorf("missing message/channel id for fallback edit")
	}
	embeds := []*discordgo.MessageEmbed{embed}
	edit := &discordgo.MessageEdit{ID: bg.MessageID, Channel: bg.ChannelID, Embeds: &embeds, Components: &components}
	_, err := bg.Session.ChannelMessageEditComplex(edit)
	return err
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

// respondWithDeferredError sends an error response for already-deferred interactions
func respondWithDeferredError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	embed := utils.CreateBrandedEmbed(
		"❌ Error",
		message,
		0xFF0000, // Red
	)

	utils.UpdateInteractionResponse(s, i, embed, nil)
}

// RegisterBlackjackCommands returns the slash command definition for blackjack
func RegisterBlackjackCommands() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "blackjack",
		Description: "Start a game of Blackjack",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "bet",
				Description: "Chips to wager (e.g. 500, 10k, 50%)",
				Required:    true,
			},
		},
	}
}
