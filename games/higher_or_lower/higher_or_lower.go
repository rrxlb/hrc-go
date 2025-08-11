package higherorlower

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
)

// Streak multipliers (index = streak-1)
var streakMultipliers = []float64{0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 4.0, 5.0, 7.0, 10.0}

// Active games (userID -> game)
var (
	activeGames   = map[int64]*Game{}
	activeGamesMu sync.RWMutex
)

const (
	gameType            = "higher_or_lower"
	inactivityThreshold = 5 * time.Minute // matches python view timeout (300s)
	checkInterval       = 15 * time.Second
)

// Game represents a Higher or Lower game state
type Game struct {
	*utils.BaseGame
	Deck           *utils.Deck
	CurrentCard    utils.Card
	NextCard       *utils.Card
	Streak         int
	MessageID      string
	CreatedAt      time.Time
	LastAction     time.Time
	Finished       bool
	cachedMult     *float64
	cachedWinnings *int64
	Phase          string // playing, result, final
}

// RegisterHigherOrLowerCommand returns slash command definition.
func RegisterHigherOrLowerCommand() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "horl",
		Description: "Play Higher or Lower.",
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "bet", Description: "Bet amount (e.g. 500, 5k, half, all)", Required: true},
		},
	}
}

// HandleHigherOrLowerCommand starts a new game.
func HandleHigherOrLowerCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := utils.ParseUserID(i.Member.User.ID)
	activeGamesMu.Lock()
	if _, exists := activeGames[userID]; exists {
		activeGamesMu.Unlock()
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Higher or Lower", "You already have an active game.", 0xFF0000), nil, true)
		return
	}
	activeGamesMu.Unlock()

	// Extract bet option
	betStr := ""
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "bet" {
			betStr = opt.StringValue()
			break
		}
	}
	user, err := utils.GetCachedUser(userID)
	if err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "Failed to load user.", 0xFF0000), nil, true)
		return
	}
	betAmt, err := utils.ParseBet(betStr, user.Chips)
	if err != nil || betAmt <= 0 {
		msg := "Invalid bet amount."
		if err != nil {
			msg = err.Error()
		}
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", msg, 0xFF0000), nil, true)
		return
	}
	game := &Game{BaseGame: utils.NewBaseGame(s, i, betAmt, gameType), Deck: utils.NewDeck(1, "poker"), CreatedAt: time.Now(), LastAction: time.Now(), Phase: "playing"}
	if err := game.BaseGame.ValidateBet(); err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", err.Error(), 0xFF0000), nil, true)
		return
	}
	// Deal first card
	game.CurrentCard = game.Deck.Deal()
	utils.DeferInteractionResponse(s, i, false)
	embed := game.buildEmbed("playing", "", false)
	msgID := game.sendInitialFollowup(s, i, embed)
	game.MessageID = msgID
	activeGamesMu.Lock()
	activeGames[userID] = game
	activeGamesMu.Unlock()
	go game.watchTimeout(s)
}

// sendInitialFollowup sends the initial followup message, returns message ID.
func (g *Game) sendInitialFollowup(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) string {
	params := &discordgo.WebhookParams{Embeds: []*discordgo.MessageEmbed{embed}, Components: g.components(false)}
	msg, err := s.FollowupMessageCreate(i.Interaction, true, params)
	if err != nil {
		return ""
	}
	return msg.ID
}

// HandleHigherOrLowerInteraction processes button presses.
func HandleHigherOrLowerInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}
	userID, _ := utils.ParseUserID(i.Member.User.ID)
	activeGamesMu.RLock()
	game, ok := activeGames[userID]
	activeGamesMu.RUnlock()
	if !ok {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Higher or Lower", "No active game.", 0xFF0000), nil, true)
		return
	}
	if game.Finished {
		utils.AcknowledgeComponentInteraction(s, i)
		return
	}
	cid := i.MessageComponentData().CustomID
	switch cid {
	case "horl_higher":
		game.handleGuess(s, i, "higher")
	case "horl_lower":
		game.handleGuess(s, i, "lower")
	case "horl_cashout":
		if game.Streak == 0 { // shouldn't happen due to disabled button
			utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Higher or Lower", "You need at least one correct guess before cashing out.", 0xFFAA00), nil, true)
			return
		}
		game.cashOut(s, i)
	}
}

// handleGuess implements guess logic similar to Python.
func (g *Game) handleGuess(s *discordgo.Session, i *discordgo.InteractionCreate, guess string) {
	if g.Finished {
		utils.AcknowledgeComponentInteraction(s, i)
		return
	}

	// Promote previous next card to current if we were in result phase
	if g.Phase == "result" && g.NextCard != nil {
		g.CurrentCard = *g.NextCard
		g.NextCard = nil
		g.Phase = "playing"
	}

	dealt := g.Deck.Deal()
	g.NextCard = &dealt
	prev := g.CurrentCard
	next := dealt
	correct := (guess == "higher" && cardValue(next) > cardValue(prev)) || (guess == "lower" && cardValue(next) < cardValue(prev))
	tie := cardValue(next) == cardValue(prev)

	if correct {
		g.Streak++
		g.cachedMult = nil
		g.cachedWinnings = nil
		g.Phase = "result"
		g.updateMessageResult(s, i, "You guessed correctly!")
		return
	}
	if tie {
		g.Phase = "result"
		g.updateMessageResult(s, i, "It's a tie! The streak continues.")
		return
	}
	// Incorrect guess ends game (lost). Provide next card for final embed.
	g.endGame(s, i, true, false, 0, &next)
}

func (g *Game) cashOut(s *discordgo.Session, i *discordgo.InteractionCreate) {
	g.endGame(s, i, false, true, 0, nil)
}

// handle timeout (cash out if streak>0 else refund?) matches python: profit_override = winnings - bet if streak>0 else 0
func (g *Game) handleTimeout(s *discordgo.Session) {
	if g.Finished {
		return
	}
	profitOverride := int64(0)
	if g.Streak > 0 {
		profitOverride = g.currentWinnings() - g.Bet
	}
	g.endGameWithOverride(s, profitOverride, "Game timed out. Cashed out.")
}

// endGame finalizes profit.
func (g *Game) endGame(s *discordgo.Session, i *discordgo.InteractionCreate, lost bool, cashedOut bool, profitOverride int64, finalCard *utils.Card) {
	if g.Finished {
		return
	}
	g.Finished = true
	g.Phase = "final"
	profit := int64(0)
	outcome := ""
	if profitOverride != 0 || (profitOverride == 0 && (lost || cashedOut)) { // explicit override or standard paths
		if profitOverride != 0 {
			profit = profitOverride
			if profit > 0 {
				outcome = fmt.Sprintf("Game timed out. Cashed out with %s.", utils.FormatChips(g.currentWinnings()))
			} else {
				outcome = "Game timed out. Your bet was refunded."
			}
		} else if lost {
			profit = -g.Bet
			if finalCard != nil {
				g.NextCard = finalCard
			}
			outcome = fmt.Sprintf("You lost! The next card was `%s`.", g.NextCard.String())
		} else if cashedOut {
			profit = g.currentWinnings() - g.Bet
			outcome = fmt.Sprintf("Cashed out with %s chips!", utils.FormatChips(g.currentWinnings()))
		}
	}
	updatedUser, _ := g.BaseGame.EndGame(profit)
	xpGain := int64(0)
	if profit > 0 {
		xpGain = profit * utils.XPPerProfit
	}
	_ = xpGain
	// Build final embed
	embed := g.buildEmbed("final", outcome, profit > 0)
	components := []discordgo.MessageComponent{} // disable buttons
	if i != nil {
		utils.UpdateComponentInteraction(s, i, embed, components)
	} else {
		g.editMessage(s, embed, components)
	}
	_ = updatedUser // embed already reflects new balance via builder
	activeGamesMu.Lock()
	delete(activeGames, g.UserID)
	activeGamesMu.Unlock()
}

func (g *Game) endGameWithOverride(s *discordgo.Session, profitOverride int64, outcome string) {
	if g.Finished {
		return
	}
	g.Finished = true
	g.Phase = "final"
	profit := profitOverride
	updatedUser, _ := g.BaseGame.EndGame(profit)
	_ = updatedUser
	embed := g.buildEmbed("final", outcome, profit > 0)
	g.editMessage(s, embed, nil)
	activeGamesMu.Lock()
	delete(activeGames, g.UserID)
	activeGamesMu.Unlock()
}

// buildEmbed recreates python create_higher_or_lower_embed function (subset used states: playing/final)
func (g *Game) buildEmbed(state string, outcomeText string, won bool) *discordgo.MessageEmbed {
	title := "Higher or Lower"
	description := ""
	color := 0x5865F2
	if state == "playing" {
		description = "Will the next card be higher or lower?"
		color = 0x1E5631
	} else if state == "result" {
		title = "Higher or Lower - Result"
		lower := strings.ToLower(outcomeText)
		if strings.Contains(lower, "correct") {
			color = 0x2ECC71
		} else if strings.Contains(lower, "tie") {
			color = 0x95A5A6
		}
	} else if state == "final" {
		title = "Higher or Lower - Game Over"
		if strings.Contains(outcomeText, "Cashed out") {
			color = 0xFFD700
		} else if won {
			color = 0x2ECC71
		} else {
			color = 0xE74C3C
		}
	}
	embed := utils.CreateBrandedEmbed(title, description, color)
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1753327146/HL2_oproic.png"}

	if state == "playing" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Current Card", Value: fmt.Sprintf("`%s`", g.CurrentCard.String()), Inline: false})
	} else if state == "result" {
		if g.NextCard != nil {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Previous Card", Value: fmt.Sprintf("`%s`", g.CurrentCard.String()), Inline: true})
			// Show newly revealed as the upcoming current card
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Current Card", Value: fmt.Sprintf("`%s`", g.NextCard.String()), Inline: true})
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "\u200b", Value: "\u200b", Inline: true})
		}
	} else if state == "final" {
		finalCard := g.CurrentCard
		if g.NextCard != nil {
			finalCard = *g.NextCard
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Final Card", Value: fmt.Sprintf("`%s`", finalCard.String()), Inline: false})
	}

	multStr := fmt.Sprintf("x%.1f", g.currentMultiplier())
	if math.Mod(g.currentMultiplier(), 1.0) == 0 {
		multStr = fmt.Sprintf("x%.0f", g.currentMultiplier())
	}
	var gameInfo string
	if state == "final" {
		gameInfo = fmt.Sprintf("**Streak:** 🔥 %d wins\n**Multiplier:** `%s`", g.Streak, multStr)
	} else {
		gameInfo = fmt.Sprintf("**Streak:** 🔥 %d wins\n**Multiplier:** `%s`\n**Current Winnings:** %s %s", g.Streak, multStr, utils.FormatChips(g.currentWinnings()), utils.ChipsEmoji)
	}
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Game Info", Value: gameInfo, Inline: false})
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Initial Bet", Value: fmt.Sprintf("%s %s", utils.FormatChips(g.Bet), utils.ChipsEmoji), Inline: false})

	if state == "result" || state == "final" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Outcome", Value: outcomeText, Inline: false})
	}
	if state == "final" {
		if g.currentWinnings() > 0 {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "💰 Total Winnings", Value: fmt.Sprintf("%s %s", utils.FormatChips(g.currentWinnings()), utils.ChipsEmoji), Inline: true})
			profit := g.currentWinnings() - g.Bet
			if profit > 0 {
				xpGain := profit * utils.XPPerProfit
				embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "XP Gained", Value: fmt.Sprintf("%s XP", utils.FormatChips(xpGain)), Inline: true})
			}
		}
		if g.UserData != nil {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "New Balance", Value: fmt.Sprintf("%s %s", utils.FormatChips(g.UserData.Chips), utils.ChipsEmoji), Inline: false})
		}
		embed.Footer.Text += " | Game Over"
	} else {
		embed.Footer.Text += " | Choose wisely!"
	}
	return embed
}

// components returns active buttons; cash out disabled until streak>0 or game finished.
func (g *Game) components(disabled bool) []discordgo.MessageComponent {
	if g.Finished {
		return nil
	}
	cashDisabled := g.Streak == 0
	if disabled {
		cashDisabled = true
	}
	return []discordgo.MessageComponent{utils.CreateActionRow(
		utils.CreateButton("horl_higher", "Higher 🔼", discordgo.SuccessButton, false, nil),
		utils.CreateButton("horl_lower", "Lower 🔽", discordgo.DangerButton, false, nil),
		utils.CreateButton("horl_cashout", "Cash Out 💰", discordgo.PrimaryButton, cashDisabled, nil),
	)}
}

// update playing state after guess
// updateMessageResult displays the reveal of the next card (result phase)
func (g *Game) updateMessageResult(s *discordgo.Session, i *discordgo.InteractionCreate, outcome string) {
	g.LastAction = time.Now()
	embed := g.buildEmbed("result", outcome, true)
	utils.UpdateComponentInteraction(s, i, embed, g.components(false))
}

func (g *Game) editMessage(s *discordgo.Session, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) {
	if g.MessageID == "" {
		return
	}
	embeds := []*discordgo.MessageEmbed{embed}
	edit := &discordgo.MessageEdit{ID: g.MessageID, Channel: g.BaseGame.Interaction.ChannelID, Embeds: &embeds}
	if components != nil {
		edit.Components = &components
	}
	s.ChannelMessageEditComplex(edit)
}

// watchTimeout monitors inactivity
func (g *Game) watchTimeout(s *discordgo.Session) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for range ticker.C {
		if g.Finished {
			return
		}
		if time.Since(g.LastAction) > inactivityThreshold {
			g.handleTimeout(s)
			return
		}
	}
}

// Utility functions
func (g *Game) currentMultiplier() float64 {
	if g.cachedMult != nil {
		return *g.cachedMult
	}
	if g.Streak == 0 {
		m := 0.0
		g.cachedMult = &m
		return 0
	}
	idx := g.Streak - 1
	if idx >= len(streakMultipliers) {
		idx = len(streakMultipliers) - 1
	}
	m := streakMultipliers[idx]
	g.cachedMult = &m
	return m
}

func (g *Game) currentWinnings() int64 {
	if g.cachedWinnings != nil {
		return *g.cachedWinnings
	}
	if g.Streak == 0 {
		w := int64(0)
		g.cachedWinnings = &w
		return 0
	}
	win := g.Bet + int64(float64(g.Bet)*g.currentMultiplier())
	g.cachedWinnings = &win
	return win
}

func cardValue(c utils.Card) int {
	// Provide poker style ranking with Ace high (14) to match python's CARD_RANKS
	switch c.Rank {
	case "A":
		return 14
	case "K":
		return 13
	case "Q":
		return 12
	case "J":
		return 11
	default:
		v, _ := strconv.Atoi(c.Rank)
		return v
	}
}
