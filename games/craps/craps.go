package craps

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
)

// Payout ratios (approximate to python constants)
var payoutRatios = map[string]float64{
	"pass_line": 1.0, "dont_pass": 1.0, "come": 1.0, "dont_come": 1.0,
	"place_4": 9.0 / 5.0, "place_5": 7.0 / 5.0, "place_6": 7.0 / 6.0, "place_8": 7.0 / 6.0, "place_9": 7.0 / 5.0, "place_10": 9.0 / 5.0,
	"field_2": 2.0, "field_12": 3.0, "field_other": 1.0,
	"hard_4": 7.0, "hard_10": 7.0, "hard_6": 9.0, "hard_8": 9.0,
}

// Active games keyed by userID
var activeGames = map[int64]*Game{}

// Dice emojis (placeholder unicode; can be replaced with custom server emojis)
var diceEmoji = map[int]string{1: "🎲1", 2: "🎲2", 3: "🎲3", 4: "🎲4", 5: "🎲5", 6: "🎲6"}

// Game phases
const (
	phaseComeOut = "come_out"
	phasePoint   = "point"
)

// Inactivity timeout duration for craps games
const inactivityTimeout = 2 * time.Minute

// Hard termination after remaining timed out beyond this duration
const hardTimeout = 8 * time.Minute

type Game struct {
	*utils.BaseGame
	Phase            string
	Point            *int
	Bets             map[string]int64
	ComePoints       map[int]int64 // come point value -> bet amount
	SessionProfit    int64
	CreatedAt        time.Time
	MessageID        string // primary game message
	rng              *rand.Rand
	LastRollDisplay  string
	PendingDecisions map[string]int64 // betType -> winnings awaiting keep/down decision
	LastAction       time.Time
	TimedOut         bool
	Rolling          bool
	TimedOutAt       time.Time
	AutoClosed       bool
}

// RegisterCrapsCommand returns the slash command definition
func RegisterCrapsCommand() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "craps",
		Description: "Play a game of Craps (Pass Line bet to start)",
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "bet", Description: "Pass Line bet (e.g. 500, 5k, half, all)", Required: true},
		},
	}
}

// HandleCrapsCommand starts a new craps game
func HandleCrapsCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := utils.ParseUserID(i.Member.User.ID)
	if _, exists := activeGames[userID]; exists {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Craps", "You already have an active Craps game.", 0xFF0000), nil, true)
		return
	}
	betStr := ""
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "bet" {
			betStr = opt.StringValue()
		}
	}
	user, err := utils.GetCachedUser(userID)
	if err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "Failed to load user.", 0xFF0000), nil, true)
		return
	}
	betAmount, err := utils.ParseBet(betStr, user.Chips)
	if err != nil || betAmount <= 0 {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "Invalid bet amount.", 0xFF0000), nil, true)
		return
	}
	game := &Game{BaseGame: utils.NewBaseGame(s, i, betAmount, "craps"), Phase: phaseComeOut, Bets: map[string]int64{"pass_line": betAmount}, ComePoints: map[int]int64{}, CreatedAt: time.Now(), rng: rand.New(rand.NewSource(time.Now().UnixNano())), PendingDecisions: map[string]int64{}, LastAction: time.Now()}
	if err := game.BaseGame.ValidateBet(); err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", err.Error(), 0xFF0000), nil, true)
		return
	}
	activeGames[userID] = game
	embed := game.buildEmbed("Game started. Place additional bets with buttons.", "Waiting to roll...")
	components := game.components()
	utils.SendInteractionResponse(s, i, embed, components, false)
	go game.watchTimeout(s)
}

// HandleCrapsButton processes button interactions
func HandleCrapsButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := utils.ParseUserID(i.Member.User.ID)
	game, exists := activeGames[userID]
	if !exists {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Craps", "No active game.", 0xFF0000), nil, true)
		return
	}
	custom := i.MessageComponentData().CustomID
	// Decision buttons (keep / take down)
	if strings.HasPrefix(custom, "craps_decide_keep_") || strings.HasPrefix(custom, "craps_decide_take_") {
		isKeep := strings.HasPrefix(custom, "craps_decide_keep_")
		betType := strings.TrimPrefix(custom, ternary(isKeep, "craps_decide_keep_", "craps_decide_take_"))
		winnings, pending := game.PendingDecisions[betType]
		if !pending {
			utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Craps", "Decision expired or already handled.", 0xFFAA00), nil, true)
			return
		}
		if game.BaseGame.IsGameOver() {
			delete(game.PendingDecisions, betType)
			utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Craps", "Game over; decision ignored.", 0xFF0000), nil, true)
			return
		}
		game.SessionProfit += winnings
		msg := ""
		if isKeep {
			msg = fmt.Sprintf("You won %s and kept your bet on %s.", utils.FormatChips(winnings), formatBetKey(betType))
		} else {
			original := game.Bets[betType]
			delete(game.Bets, betType)
			msg = fmt.Sprintf("You won %s and took down your bet on %s (stake %s returned).", utils.FormatChips(winnings), formatBetKey(betType), utils.FormatChips(original))
		}
		delete(game.PendingDecisions, betType)
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Craps", msg, utils.BotColor), nil, true)
		game.updateOriginalAfterBet(msg)
		game.updateLastAction()
		return
	}
	if custom == "craps_resume" {
		if !game.TimedOut {
			utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Craps", "Game is already active.", 0xFFAA00), nil, true)
			return
		}
		game.TimedOut = false
		game.updateLastAction()
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Craps", "Game resumed. You may roll.", utils.BotColor), nil, true)
		game.updateOriginalAfterBet("Game resumed.")
		return
	}
	if strings.HasPrefix(custom, "craps_roll") {
		if game.TimedOut {
			utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Craps", "Game timed out. Press Resume to continue.", 0xFFAA00), nil, true)
			return
		}
		if game.Rolling {
			utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Craps", "Roll already in progress...", 0xFFAA00), nil, true)
			return
		}
		game.Rolling = true
		game.handleRoll(s, i)
		game.Rolling = false
		return
	}
	if strings.HasPrefix(custom, "craps_bet_") {
		if game.TimedOut {
			utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Craps", "Game timed out. Resume first.", 0xFFAA00), nil, true)
			return
		}
		betType := strings.TrimPrefix(custom, "craps_bet_")
		// Open modal to collect amount
		modal := &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: &discordgo.InteractionResponseData{CustomID: fmt.Sprintf("craps_bet_modal_%s", betType), Title: fmt.Sprintf("Bet Amount - %s", formatBetKey(betType)), Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{&discordgo.TextInput{CustomID: "bet_amount", Label: "Enter amount (e.g. 500, 5k, half, all)", Style: discordgo.TextInputShort, Placeholder: "Amount", Required: true}}}}}}
		_ = s.InteractionRespond(i.Interaction, modal)
		return
	}
}

// addBet adds a bet if allowed
func (g *Game) addBet(betType string, amount int64) error {
	if _, ok := g.Bets[betType]; ok {
		return fmt.Errorf("already have bet on %s", formatBetKey(betType))
	}
	// Phase restrictions similar to python
	comeOutOnly := map[string]bool{"pass_line": true, "dont_pass": true}
	pointOnly := map[string]bool{"come": true, "dont_come": true}
	if comeOutOnly[betType] && g.Phase != phaseComeOut {
		return fmt.Errorf("%s bets only on come-out", formatBetKey(betType))
	}
	if pointOnly[betType] && g.Phase != phasePoint {
		return fmt.Errorf("%s bets only after point", formatBetKey(betType))
	}
	if strings.HasPrefix(betType, "place_") && g.Phase != phasePoint {
		return fmt.Errorf("place bets only after point")
	}
	// Chips check
	totalCommitted := amount
	for _, v := range g.Bets {
		totalCommitted += v
	}
	for _, v := range g.ComePoints {
		totalCommitted += v
	}
	if g.UserData.Chips < totalCommitted {
		return fmt.Errorf("insufficient chips")
	}
	g.Bets[betType] = amount
	return nil
}

func (g *Game) rollDice() (int, int) { return g.rng.Intn(6) + 1, g.rng.Intn(6) + 1 }

// handleRoll executes a dice roll
func (g *Game) handleRoll(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if g.BaseGame.IsGameOver() {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Craps", "Game over.", 0xFF0000), nil, true)
		return
	}
	// Acknowledge interaction
	_ = utils.AcknowledgeComponentInteraction(s, i)
	d1, d2 := g.rollDice()
	total := d1 + d2
	outcome, rollProfit, placeWins := g.resolveRoll(total, d1, d2)
	g.SessionProfit += rollProfit
	// Point transitions
	if g.Phase == phaseComeOut && !isCrapsOrNatural(total) {
		g.Phase = phasePoint
		pt := total
		g.Point = &pt
		if outcome != "" {
			outcome += "\n"
		}
		outcome += fmt.Sprintf("Point is now %d.", total)
	} else if g.Phase == phasePoint {
		if g.Point != nil && total == *g.Point { // point hit
			if outcome != "" {
				outcome += "\n"
			}
			outcome += fmt.Sprintf("Point %d hit! New come-out roll.", *g.Point)
			g.Phase = phaseComeOut
			g.Point = nil
		} else if total == 7 { // seven out end game
			gameOverMsg := fmt.Sprintf("Seven out! Game over. Total %s: %s.", ternary(g.SessionProfit >= 0, "Profit", "Loss"), utils.FormatChips(abs64(g.SessionProfit)))
			if outcome != "" {
				outcome += "\n"
			}
			outcome += gameOverMsg
			g.endGame()
		}
	}
	g.LastRollDisplay = fmt.Sprintf("%s %s (Total: %d)", diceEmoji[d1], diceEmoji[d2], total)
	g.updateLastAction()
	g.updateMessage(s, i, outcome, g.LastRollDisplay, g.BaseGame.IsGameOver())

	// Create decision prompts for place/hard wins (only if game not over)
	if !g.BaseGame.IsGameOver() && len(placeWins) > 0 {
		for betType, winnings := range placeWins {
			// Record pending decision
			g.PendingDecisions[betType] = winnings
			// Build ephemeral decision message
			components := []discordgo.MessageComponent{utils.CreateActionRow(
				utils.CreateButton("craps_decide_keep_"+betType, "Keep Bet Up", discordgo.SuccessButton, false, nil),
				utils.CreateButton("craps_decide_take_"+betType, "Take Bet Down", discordgo.DangerButton, false, nil),
			)}
			params := &discordgo.WebhookParams{
				Embeds:     []*discordgo.MessageEmbed{utils.CreateBrandedEmbed("Craps Decision", fmt.Sprintf("Your %s bet won %s. What would you like to do?", formatBetKey(betType), utils.FormatChips(winnings)), utils.BotColor)},
				Components: components,
				Flags:      discordgo.MessageFlagsEphemeral,
			}
			// Send followup (ignore errors)
			_, _ = s.FollowupMessageCreate(i.Interaction, true, params)
		}
	}
	_ = placeWins // already processed
}

func isCrapsOrNatural(total int) bool {
	return total == 2 || total == 3 || total == 7 || total == 11 || total == 12
}

// resolveRoll mirrors python logic (simplified payout handling) returns outcome lines & profit delta
func (g *Game) resolveRoll(total, d1, d2 int) (string, int64, map[string]int64) {
	lines := []string{}
	profit := int64(0)
	removeBets := []string{}
	placeWins := map[string]int64{}
	// Field bet
	if amt, ok := g.Bets["field"]; ok {
		if total == 2 {
			net := int64(math.Ceil(float64(amt) * payoutRatios["field_2"]))
			profit += net
			lines = append(lines, fmt.Sprintf("Field bet wins %s.", utils.FormatChips(net)))
		} else if total == 12 {
			net := int64(math.Ceil(float64(amt) * payoutRatios["field_12"]))
			profit += net
			lines = append(lines, fmt.Sprintf("Field bet wins %s.", utils.FormatChips(net)))
		} else if contains([]int{3, 4, 9, 10, 11}, total) {
			net := int64(math.Ceil(float64(amt) * payoutRatios["field_other"]))
			profit += net
			lines = append(lines, fmt.Sprintf("Field bet wins %s.", utils.FormatChips(net)))
		} else {
			profit -= amt
			lines = append(lines, "Field bet loses.")
		}
		removeBets = append(removeBets, "field")
	}
	// Hard ways
	for bet, amt := range g.Bets {
		if strings.HasPrefix(bet, "hard_") {
			num := mustAtoi(strings.TrimPrefix(bet, "hard_"))
			if d1 == d2 && d1+d2 == num {
				win := int64(math.Ceil(float64(amt) * payoutRatios[bet]))
				placeWins[bet] = win
				lines = append(lines, fmt.Sprintf("Hard %d hits!", num))
			} else if total == 7 || (d1+d2 == num && d1 != d2) {
				profit -= amt
				lines = append(lines, fmt.Sprintf("Hard %d loses.", num))
				removeBets = append(removeBets, bet)
			}
		}
	}
	// Come-out specific
	if g.Phase == phaseComeOut {
		if amt, ok := g.Bets["pass_line"]; ok {
			if total == 7 || total == 11 {
				profit += amt
				lines = append(lines, fmt.Sprintf("Pass Line wins %s.", utils.FormatChips(amt)))
			} else if contains([]int{2, 3, 12}, total) {
				profit -= amt
				lines = append(lines, "Pass Line loses (Craps).")
			}
		}
		if amt, ok := g.Bets["dont_pass"]; ok {
			if contains([]int{2, 3}, total) {
				profit += amt
				lines = append(lines, fmt.Sprintf("Don't Pass wins %s.", utils.FormatChips(amt)))
			} else if contains([]int{7, 11}, total) {
				profit -= amt
				lines = append(lines, "Don't Pass loses.")
			} else if total == 12 {
				lines = append(lines, "Don't Pass pushes (Bar 12).")
			}
		}
	} else { // point phase
		if amt, ok := g.Bets["pass_line"]; ok {
			if g.Point != nil && total == *g.Point {
				profit += amt
				lines = append(lines, fmt.Sprintf("Point of %d hit! Pass Line wins %s.", *g.Point, utils.FormatChips(amt)))
			} else if total == 7 {
				profit -= amt
				lines = append(lines, "Seven out! Pass Line loses.")
			}
		}
		if amt, ok := g.Bets["dont_pass"]; ok {
			if total == 7 {
				profit += amt
				lines = append(lines, fmt.Sprintf("Seven out! Don't Pass wins %s.", utils.FormatChips(amt)))
			} else if g.Point != nil && total == *g.Point {
				profit -= amt
				lines = append(lines, fmt.Sprintf("Point of %d hit! Don't Pass loses.", *g.Point))
			}
		}
		// Place bets
		for bet, amt := range g.Bets {
			if strings.HasPrefix(bet, "place_") {
				num := mustAtoi(strings.TrimPrefix(bet, "place_"))
				if total == num {
					win := int64(math.Ceil(float64(amt) * payoutRatios[bet]))
					placeWins[bet] = win
					lines = append(lines, fmt.Sprintf("Place bet on %d wins!", num))
				} else if total == 7 {
					profit -= amt
					lines = append(lines, fmt.Sprintf("Place bet on %d loses (Seven out).", num))
					removeBets = append(removeBets, bet)
				}
			}
		}
	}
	// Come bet resolution
	if amt, ok := g.Bets["come"]; ok {
		if contains([]int{7, 11}, total) {
			profit += amt
			lines = append(lines, fmt.Sprintf("Come bet wins %s.", utils.FormatChips(amt)))
		} else if contains([]int{2, 3, 12}, total) {
			profit -= amt
			lines = append(lines, "Come bet loses.")
		} else {
			g.ComePoints[total] += amt
			lines = append(lines, fmt.Sprintf("Come point is now %d.", total))
		}
		removeBets = append(removeBets, "come")
	}
	if amt, ok := g.Bets["dont_come"]; ok {
		if contains([]int{2, 3}, total) {
			profit += amt
			lines = append(lines, fmt.Sprintf("Don't Come bet wins %s.", utils.FormatChips(amt)))
		} else if contains([]int{7, 11}, total) {
			profit -= amt
			lines = append(lines, "Don't Come bet loses.")
		} else if total == 12 {
			lines = append(lines, "Don't Come bet pushes.")
		} else {
			lines = append(lines, fmt.Sprintf("Don't Come point established on %d.", total))
		}
		removeBets = append(removeBets, "dont_come")
	}
	// Existing come points
	for point, amt := range g.ComePoints {
		if total == point {
			profit += amt
			lines = append(lines, fmt.Sprintf("Come point %d hit! You win %s.", point, utils.FormatChips(amt)))
			delete(g.ComePoints, point)
		} else if total == 7 {
			profit -= amt
			lines = append(lines, fmt.Sprintf("Come point %d loses (Seven out).", point))
			delete(g.ComePoints, point)
		}
	}
	// Remove consumed bets
	for _, b := range removeBets {
		delete(g.Bets, b)
	}
	return strings.Join(lines, "\n"), profit, placeWins
}

// endGame finalizes profit with BaseGame
func (g *Game) endGame() {
	_, _ = g.BaseGame.EndGame(g.SessionProfit)
}

// buildEmbed builds the main game embed
func (g *Game) buildEmbed(outcome, rollDisplay string) *discordgo.MessageEmbed {
	title := "🎲 Craps Table"
	color := utils.BotColor
	if g.BaseGame.IsGameOver() {
		color = 0xE74C3C
	} else if g.TimedOut {
		color = 0xF1C40F
	}
	layout := g.layoutString()
	embed := utils.CreateBrandedEmbed(title, layout, color)
	// Shooter field
	shooter := fmt.Sprintf("<@%d>", g.BaseGame.UserID)
	pointStr := "OFF"
	if g.Point != nil {
		pointStr = fmt.Sprintf("ON (%d)", *g.Point)
	}
	if rollDisplay != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Current Roll", Value: rollDisplay, Inline: true})
	}
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Shooter", Value: shooter, Inline: true})
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Point", Value: pointStr, Inline: true})
	// Outcome field includes profit line
	if outcome != "" {
		netWord := ternary(g.SessionProfit > 0, "Profit", ternary(g.SessionProfit < 0, "Loss", "Push"))
		profitLine := fmt.Sprintf("Total %s: %s.", netWord, utils.FormatChips(abs64(g.SessionProfit)))
		outcome = outcome + "\n" + profitLine
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Outcome", Value: outcome, Inline: false})
	}
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Your Bets", Value: g.betSummary(), Inline: false})
	if pd := g.pendingSummary(); pd != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Pending Decisions", Value: pd, Inline: false})
	}
	if g.BaseGame.IsGameOver() {
		embed.Footer.Text += " | Game Over"
	} else if g.TimedOut {
		embed.Footer.Text += " | Timed Out"
	} else {
		embed.Footer.Text += " | Active"
	}
	return embed
}

// pendingSummary returns a list of pending decision bet names
func (g *Game) pendingSummary() string {
	if len(g.PendingDecisions) == 0 {
		return ""
	}
	keys := make([]string, 0, len(g.PendingDecisions))
	for k := range g.PendingDecisions {
		keys = append(keys, formatBetKey(k))
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// components builds interactive buttons
func (g *Game) components() []discordgo.MessageComponent {
	if g.BaseGame.IsGameOver() {
		return []discordgo.MessageComponent{}
	}
	if g.TimedOut && !g.AutoClosed {
		return []discordgo.MessageComponent{utils.CreateActionRow(
			utils.CreateButton("craps_resume", "Resume", discordgo.SuccessButton, false, nil),
		)}
	}
	row1 := utils.CreateActionRow(
		utils.CreateButton("craps_roll", "Roll", discordgo.SuccessButton, false, &discordgo.ComponentEmoji{Name: "🎲"}),
		utils.CreateButton("craps_bet_field", "Field", discordgo.PrimaryButton, false, nil),
		utils.CreateButton("craps_bet_come", "Come", discordgo.SecondaryButton, g.Phase != phasePoint, nil),
		utils.CreateButton("craps_bet_dont_come", "Don't Come", discordgo.SecondaryButton, g.Phase != phasePoint, nil),
	)
	row2 := utils.CreateActionRow(
		utils.CreateButton("craps_bet_place_6", "Place 6", discordgo.SecondaryButton, g.Phase != phasePoint, nil),
		utils.CreateButton("craps_bet_place_8", "Place 8", discordgo.SecondaryButton, g.Phase != phasePoint, nil),
		utils.CreateButton("craps_bet_place_5", "Place 5", discordgo.SecondaryButton, g.Phase != phasePoint, nil),
		utils.CreateButton("craps_bet_place_9", "Place 9", discordgo.SecondaryButton, g.Phase != phasePoint, nil),
	)
	row3 := utils.CreateActionRow(
		utils.CreateButton("craps_bet_hard_4", "Hard 4", discordgo.SecondaryButton, false, nil),
		utils.CreateButton("craps_bet_hard_6", "Hard 6", discordgo.SecondaryButton, false, nil),
		utils.CreateButton("craps_bet_hard_8", "Hard 8", discordgo.SecondaryButton, false, nil),
		utils.CreateButton("craps_bet_hard_10", "Hard 10", discordgo.SecondaryButton, false, nil),
	)
	return []discordgo.MessageComponent{row1, row2, row3}
}

// updateMessage updates or edits interaction message after a roll or bet
func (g *Game) updateMessage(s *discordgo.Session, i *discordgo.InteractionCreate, outcome, rollDisplay string, gameOver bool) {
	embed := g.buildEmbed(outcome, rollDisplay)
	components := g.components()
	if gameOver {
		components = []discordgo.MessageComponent{}
	}
	if err := utils.UpdateComponentInteraction(s, i, embed, components); err != nil {
		// fallback edit
		embeds := []*discordgo.MessageEmbed{embed}
		edit := &discordgo.MessageEdit{ID: i.Message.ID, Channel: i.ChannelID, Embeds: &embeds, Components: &components}
		_, _ = s.ChannelMessageEditComplex(edit)
	}
}

// helper formatting
func (g *Game) betSummary() string {
	if len(g.Bets) == 0 && len(g.ComePoints) == 0 {
		return "No bets placed."
	}
	// sort for stable output
	keys := make([]string, 0, len(g.Bets))
	for k := range g.Bets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := []string{}
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("**%s:** %s", formatBetKey(k), utils.FormatChips(g.Bets[k])))
	}
	// come points
	cpKeys := make([]int, 0, len(g.ComePoints))
	for p := range g.ComePoints {
		cpKeys = append(cpKeys, p)
	}
	sort.Ints(cpKeys)
	for _, p := range cpKeys {
		lines = append(lines, fmt.Sprintf("**Come %d:** %s", p, utils.FormatChips(g.ComePoints[p])))
	}
	return strings.Join(lines, "\n")
}

func (g *Game) layoutString() string {
	nums := []int{4, 5, 6, 8, 9, 10}
	top := make([]string, 0, len(nums))
	for _, n := range nums {
		markers := []string{}
		if g.Point != nil && *g.Point == n {
			markers = append(markers, "POINT")
		}
		if _, ok := g.Bets[fmt.Sprintf("place_%d", n)]; ok {
			markers = append(markers, "PL")
		}
		if cpAmt, ok := g.ComePoints[n]; ok {
			markers = append(markers, "C"+shortChips(cpAmt))
		}
		if len(markers) == 0 {
			top = append(top, fmt.Sprintf("[%d]", n))
			continue
		}
		top = append(top, fmt.Sprintf("[%d %s]", n, strings.Join(markers, ":")))
	}
	line := strings.Repeat("─", 58)
	withAmt := func(key, label string) string {
		if v, ok := g.Bets[key]; ok {
			return fmt.Sprintf("%s: %s", label, utils.FormatChips(v))
		}
		return ""
	}
	passSegs := []string{}
	if s := withAmt("pass_line", "Pass Line"); s != "" {
		passSegs = append(passSegs, s)
	}
	if s := withAmt("come", "Come"); s != "" {
		passSegs = append(passSegs, s)
	}
	if s := withAmt("field", "Field"); s != "" {
		passSegs = append(passSegs, s)
	}
	dontSegs := []string{}
	if s := withAmt("dont_pass", "Don't Pass"); s != "" {
		dontSegs = append(dontSegs, s)
	}
	if s := withAmt("dont_come", "Don't Come"); s != "" {
		dontSegs = append(dontSegs, s)
	}
	hardParts := []string{}
	for _, h := range []int{4, 6, 8, 10} {
		if amt, ok := g.Bets[fmt.Sprintf("hard_%d", h)]; ok {
			hardParts = append(hardParts, fmt.Sprintf("Hard %d: %s", h, utils.FormatChips(amt)))
		}
	}
	sections := []string{strings.Join(top, " "), line}
	if len(passSegs) > 0 {
		sections = append(sections, strings.Join(passSegs, " | "), line)
	}
	if len(dontSegs) > 0 {
		sections = append(sections, strings.Join(dontSegs, " | "), line)
	}
	if len(hardParts) > 0 {
		sections = append(sections, strings.Join(hardParts, " | "))
	}
	return "`" + strings.Join(sections, "\n") + "`"
}

// shortChips returns a compact chip amount representation (e.g. 1500 -> 1.5k)
func shortChips(v int64) string {
	if v >= 1_000_000 {
		if v%1_000_000 == 0 {
			return fmt.Sprintf("%dm", v/1_000_000)
		}
		return fmt.Sprintf("%.1fm", float64(v)/1_000_000)
	}
	if v >= 1_000 {
		if v%1_000 == 0 {
			return fmt.Sprintf("%dk", v/1_000)
		}
		return fmt.Sprintf("%.1fk", float64(v)/1_000)
	}
	return fmt.Sprintf("%d", v)
}

// Utility helpers
func contains(slice []int, v int) bool {
	for _, x := range slice {
		if x == v {
			return true
		}
	}
	return false
}
func mustAtoi(s string) int {
	var n int
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}
func ternary[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}
func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
func formatBetKey(k string) string {
	parts := strings.Split(strings.ReplaceAll(k, "_", " "), " ")
	for i, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(p)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		parts[i] = string(r)
	}
	res := strings.Join(parts, " ")
	res = strings.ReplaceAll(res, "Dont", "Don't")
	return res
}

// HandleCrapsModal processes bet amount modal submissions
func HandleCrapsModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := utils.ParseUserID(i.Member.User.ID)
	game, ok := activeGames[userID]
	if !ok {
		// Respond ephemeral game missing
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Craps", "No active game.", 0xFF0000), nil, true)
		return
	}
	custom := i.ModalSubmitData().CustomID
	if !strings.HasPrefix(custom, "craps_bet_modal_") {
		return
	}
	betType := strings.TrimPrefix(custom, "craps_bet_modal_")
	var amountStr string
	for _, row := range i.ModalSubmitData().Components { // robust extraction with pointer/value handling
		var ar discordgo.ActionsRow
		switched := false
		if v, ok := row.(discordgo.ActionsRow); ok {
			ar = v
			switched = true
		} else if vp, ok := row.(*discordgo.ActionsRow); ok && vp != nil {
			ar = *vp
			switched = true
		}
		if !switched {
			continue
		}
		for _, comp := range ar.Components {
			if ti, ok := comp.(*discordgo.TextInput); ok {
				if ti.CustomID == "bet_amount" {
					amountStr = strings.TrimSpace(ti.Value)
				}
			}
		}
	}
	// Fallback: first text input if custom id mismatch
	if amountStr == "" {
		for _, row := range i.ModalSubmitData().Components {
			if ar, ok := row.(discordgo.ActionsRow); ok {
				for _, comp := range ar.Components {
					if input, ok := comp.(*discordgo.TextInput); ok {
						amountStr = strings.TrimSpace(input.Value)
						break
					}
				}
			} else if arp, ok := row.(*discordgo.ActionsRow); ok && arp != nil {
				for _, comp := range arp.Components {
					if input, ok := comp.(*discordgo.TextInput); ok {
						amountStr = strings.TrimSpace(input.Value)
						break
					}
				}
			}
			if amountStr != "" {
				break
			}
		}
	}
	original := amountStr
	if amountStr == "" {
		amountStr = "0"
	}
	// sanitize: remove commas, underscores, currency symbols/backticks
	amountStr = strings.TrimSpace(amountStr)
	amountStr = strings.ReplaceAll(amountStr, ",", "")
	amountStr = strings.ReplaceAll(amountStr, "_", "")
	amountStr = strings.Trim(amountStr, "`$")
	// Parse amount using user chips
	user, err := utils.GetCachedUser(userID)
	if err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "Failed to load user.", 0xFF0000), nil, true)
		return
	}
	amt, err := utils.ParseBet(amountStr, user.Chips)
	if err != nil || amt <= 0 {
		msg := "Invalid bet amount."
		if err != nil {
			msg = err.Error()
		}
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", msg+" ("+original+")", 0xFF0000), nil, true)
		return
	}
	if err := game.addBet(betType, amt); err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Craps", err.Error(), 0xFF0000), nil, true)
		return
	}
	// Respond ephemeral success
	utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Craps", fmt.Sprintf("Added %s bet (%s).", formatBetKey(betType), utils.FormatChips(amt)), utils.BotColor), nil, true)
	// Edit original game message via original interaction response
	// Provide outcome line only
	game.updateLastAction()
	game.updateOriginalAfterBet(fmt.Sprintf("Added %s bet (%s).", formatBetKey(betType), utils.FormatChips(amt)))
}

// updateOriginalAfterBet edits original slash command response after modal bet
func (g *Game) updateOriginalAfterBet(outcome string) {
	// Build embed using last roll display (if any)
	rollDisp := g.LastRollDisplay
	if rollDisp == "" {
		rollDisp = "Waiting to roll..."
	}
	embed := g.buildEmbed(outcome, rollDisp)
	g.BaseGame.UpdateOriginalResponse(embed, g.components())
}

// updateLastAction records latest interaction time (skip if already timed out)
func (g *Game) updateLastAction() {
	if g.TimedOut || g.BaseGame.IsGameOver() {
		return
	}
	g.LastAction = time.Now()
}

// watchTimeout monitors inactivity and marks the game as timed out
func (g *Game) watchTimeout(s *discordgo.Session) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		<-ticker.C
		if g.BaseGame.IsGameOver() {
			return
		}
		if !g.TimedOut && time.Since(g.LastAction) > inactivityTimeout {
			g.TimedOut = true
			g.TimedOutAt = time.Now()
			// Update original message to reflect timeout
			outcome := "Game timed out due to inactivity. Press Resume to continue."
			rollDisp := g.LastRollDisplay
			if rollDisp == "" {
				rollDisp = "Waiting to roll..."
			}
			embed := g.buildEmbed(outcome, rollDisp)
			g.BaseGame.UpdateOriginalResponse(embed, g.components())
		}
		if g.TimedOut && !g.AutoClosed && !g.TimedOutAt.IsZero() && time.Since(g.TimedOutAt) > hardTimeout {
			g.AutoClosed = true
			g.endGame()
			delete(activeGames, g.BaseGame.UserID)
			outcome := "Game auto-closed after extended inactivity."
			rollDisp := g.LastRollDisplay
			if rollDisp == "" {
				rollDisp = "Waiting to roll..."
			}
			embed := g.buildEmbed(outcome, rollDisp)
			g.BaseGame.UpdateOriginalResponse(embed, g.components())
			return
		}
	}
}
