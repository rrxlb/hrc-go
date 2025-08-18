package slots

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
)

// Parity constants (mirror python slots_game.py)
var (
	symbols = map[string][]string{
		"common":   {"🍒", "🍋", "🍊", "🍉"},
		"uncommon": {"🔔", "⭐"},
		"rare":     {"💎"},
		"jackpot":  {"🎰"},
	}
	symbolWeights = map[string]float64{"common": 0.75, "uncommon": 0.10, "rare": 0.13, "jackpot": 0.02}
	payouts       = map[string]int64{"🍒": 3, "🍋": 3, "🍊": 3, "🍉": 3, "🔔": 5, "⭐": 5, "💎": 10, "🎰": 15}
)

const (
	payLines                    = 5
	minBet                      = payLines
	jackpotSymbol               = "🎰"
	jackpotLossContributionRate = 0.10 // 10% of net loss feeds jackpot
	spinFrames                  = 20
)

type phase string

const (
	phaseInitial  phase = "initial"
	phaseSpinning phase = "spinning"
	phaseFinal    phase = "final"
)

// Game represents a slots session
type Game struct {
	*utils.BaseGame
	Session      *discordgo.Session
	Reels        [][]string
	MessageID    string
	ChannelID    string
	Phase        phase
	BetNote      string
	Rand         *rand.Rand
	UsedOriginal bool // true if using original interaction message instead of followup
}

// RegisterSlotsCommand config
func RegisterSlotsCommand() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "slots",
		Description: "Play a game of slots!",
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "bet", Description: "Bet amount (k/m, all, half supported)", Required: true},
		},
	}
}

// HandleSlotsCommand handles /slots
func HandleSlotsCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Add panic recovery to prevent silent failures
	defer func() {
		if r := recover(); r != nil {
			utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", "An unexpected error occurred. Please try again.", 0xFF0000), nil, true)
		}
	}()

	// Fast validation checks before any Discord API calls
	if i == nil || i.Member == nil || i.Member.User == nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", "Invalid user data", 0xFF0000), nil, true)
		return
	}

	// Parse bet amount immediately to fail fast on invalid input
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", "No bet amount provided", 0xFF0000), nil, true)
		return
	}

	betStr := options[0].StringValue()
	userID, err := utils.ParseUserID(i.Member.User.ID)
	if err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", "Failed to parse user ID", 0xFF0000), nil, true)
		return
	}

	// Get user data
	user, err := utils.GetCachedUser(userID)
	if err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", "Error fetching user data", 0xFF0000), nil, true)
		return
	}

	// Parse and validate bet
	bet, err := utils.ParseBet(betStr, user.Chips)
	if err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", fmt.Sprintf("Bet error: %s", err.Error()), 0xFF0000), nil, true)
		return
	}

	adjusted, note := normalizeBetForPaylines(bet, user.Chips)
	if adjusted == 0 {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", fmt.Sprintf("Bet must be at least %d & divisible by %d", minBet, payLines), 0xFF0000), nil, true)
		return
	}

	// Defer only after validation passes
	if err := utils.DeferInteractionResponse(s, i, false); err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", "Failed to acknowledge command", 0xFF0000), nil, true)
		return
	}

	// Create game and start playing asynchronously
	go func() {
		// Add panic recovery for the goroutine
		defer func() {
			if r := recover(); r != nil {
				utils.UpdateInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", "An unexpected error occurred. Please try again.", 0xFF0000), nil)
			}
		}()

		game := &Game{BaseGame: utils.NewBaseGame(s, i, adjusted, "slots"), Session: s, Phase: phaseInitial, BetNote: note, Rand: rand.New(rand.NewSource(time.Now().UnixNano())), UsedOriginal: true}
		game.BaseGame.CountWinLossMinRatio = 0.20
		if err := game.ValidateBet(); err != nil {
			utils.UpdateInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", err.Error(), 0xFF0000), nil)
			return
		}

		// Contribute to jackpot asynchronously to avoid blocking main interaction flow
		if utils.JackpotMgr != nil {
			go func(b int64) {
				defer func() { recover() }()
				utils.JackpotMgr.ContributeToJackpot(utils.JackpotSlots, b)
			}(adjusted)
		}

		// Start play loop asynchronously - this will handle the initial response
		game.play()
	}()
}

// normalize bet to multiple of paylines
func normalizeBetForPaylines(bet, balance int64) (int64, string) {
	if bet <= 0 || balance <= 0 {
		return 0, ""
	}
	original := bet
	if bet > balance {
		bet = balance
	}
	adjusted := bet - (bet % payLines)
	if adjusted < minBet {
		return 0, ""
	}
	if adjusted != original {
		return adjusted, fmt.Sprintf("Adjusted bet from %s to %s to fit %d paylines.", utils.FormatChips(original), utils.FormatChips(adjusted), payLines)
	}
	return adjusted, ""
}

func (g *Game) play() {
	defer func() {
		if r := recover(); r != nil {
		}
	}()
	// play start
	// Capture pre-game rank for rank-up detection
	beforeRank := getRankForXP(func() int64 {
		if g.BaseGame.UserData != nil {
			return g.BaseGame.UserData.TotalXP
		}
		return 0
	}())
	// Handle initial deferred response with spinning state
	initial := g.buildEmbed("", 0, 0, false, 0)
	if err := utils.UpdateInteractionResponse(g.Session, g.Interaction, initial, nil); err != nil {
		// Fallback to followup message if deferred response fails
		msg, err := g.Session.FollowupMessageCreate(g.Interaction.Interaction, true, &discordgo.WebhookParams{Embeds: []*discordgo.MessageEmbed{initial}})
		if err != nil {
			return
		}
		g.MessageID = msg.ID
		g.ChannelID = msg.ChannelID
	} else {
		// Get message ID from successful deferred response
		if orig, err := g.Session.InteractionResponse(g.Interaction.Interaction); err == nil {
			g.MessageID = orig.ID
			g.ChannelID = orig.ChannelID
		}
	}
	final := g.createReels()
	g.animateSpin(final)
	g.Reels = final
	totalWinnings, jackpotLine := g.calculateResults()
	jackpotPayout := int64(0)
	if jackpotLine && utils.JackpotMgr != nil {
		won, amount, _ := utils.JackpotMgr.TryWinJackpot(utils.JackpotSlots, g.UserID, g.Bet, 1.0)
		if won {
			jackpotPayout = amount
			totalWinnings += amount
		}
	}
	profit := totalWinnings - g.Bet
	// Pre-compute new balance locally (avoid DB wait before showing user)
	preBalance := int64(0)
	if g.BaseGame.UserData != nil {
		preBalance = g.BaseGame.UserData.Chips
	}
	newBalance := preBalance + profit
	// Fetch jackpot amount BEFORE launching any async writes to avoid lock contention
	jackpotAmount := int64(0)
	if utils.JackpotMgr != nil {
		if amt, err := utils.JackpotMgr.GetJackpotAmount(utils.JackpotSlots); err == nil {
			jackpotAmount = amt
		}
	}
	// Launch loss contribution after reading jackpot amount
	if profit < 0 && utils.JackpotMgr != nil {
		loss := -profit
		go func(l int64) {
			recover()
			utils.JackpotMgr.AddJackpotAmount(utils.JackpotSlots, int64(float64(l)*jackpotLossContributionRate))
		}(loss)
	}
	xpGain := int64(0)
	if profit > 0 {
		xpGain = profit * utils.XPPerProfit
	}
	if g.BaseGame != nil && g.BaseGame.UserData != nil && !utils.ShouldShowXPGained(g.BaseGame.Interaction.Member, g.BaseGame.UserData) {
		xpGain = 0
	}
	outcome := "No wins this time. Better luck next time!"
	if totalWinnings > 0 {
		outcome = fmt.Sprintf("Congratulations! You won %s chips!", utils.FormatChips(totalWinnings))
	}
	if jackpotPayout > 0 {
		outcome = fmt.Sprintf("JACKPOT! You won %s chips! (+%s)", utils.FormatChips(totalWinnings), utils.FormatChips(jackpotPayout))
	}
	g.Phase = phaseFinal
	finalEmbed := g.buildEmbed(formatReels(g.Reels), totalWinnings, xpGain, true, jackpotAmount)
	// Profit/Loss field
	plLabel := "Profit"
	plVal := fmt.Sprintf("+%s %s", utils.FormatChips(profit), utils.ChipsEmoji)
	if profit < 0 {
		plLabel = "Loss"
		plVal = fmt.Sprintf("-%s %s", utils.FormatChips(-profit), utils.ChipsEmoji)
	}
	finalEmbed.Fields = append(finalEmbed.Fields, &discordgo.MessageEmbedField{Name: plLabel, Value: plVal, Inline: true})
	finalEmbed.Fields = append(finalEmbed.Fields, &discordgo.MessageEmbedField{Name: "Outcome", Value: outcome, Inline: false})
	finalEmbed.Fields = append(finalEmbed.Fields, &discordgo.MessageEmbedField{Name: "New Balance", Value: fmt.Sprintf("%s %s", utils.FormatChips(newBalance), utils.ChipsEmoji), Inline: false})
	if jackpotPayout > 0 {
		finalEmbed.Color = 0xFFD700
	}
	components := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{utils.CreateButton("slots_spin_again_"+g.MessageID, "Spin Again", discordgo.SuccessButton, false, nil)}}}
	embeds := []*discordgo.MessageEmbed{finalEmbed}
	if _, err := g.Session.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: g.MessageID, Channel: g.ChannelID, Embeds: &embeds, Components: &components}); err != nil {
	}
	// Fallback retry: sometimes the final edit may be lost; re-issue once after short delay if message not in final phase
	go func(msgID, channelID string, embed *discordgo.MessageEmbed, comps []discordgo.MessageComponent) {
		time.Sleep(1500 * time.Millisecond)
		defer func() { recover() }()
		m, err := g.Session.ChannelMessage(channelID, msgID)
		if err != nil {
			return
		}
		if m != nil && len(m.Embeds) > 0 && strings.Contains(strings.ToLower(m.Embeds[0].Title), "results") {
			return // already final
		}
		embedsRetry := []*discordgo.MessageEmbed{embed}
		if _, err := g.Session.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: msgID, Channel: channelID, Embeds: &embedsRetry, Components: &comps}); err != nil {
		} else {
			// fallback applied
		}
	}(g.MessageID, g.ChannelID, finalEmbed, components)
	if g.BetNote != "" {
		g.Session.FollowupMessageCreate(g.Interaction.Interaction, true, &discordgo.WebhookParams{Content: g.BetNote, Flags: discordgo.MessageFlagsEphemeral})
	}

	initialJackpot := jackpotAmount
	// Async finalize DB + potential jackpot change + rank-up
	go func(profit, xpGain int64, before utils.Rank, initialJackpot int64) {
		defer func() { recover() }()
		updatedUser, err := g.EndGame(profit)
		if err != nil {
			return
		}
		jackpotAmt := initialJackpot
		if utils.JackpotMgr != nil {
			if j, e := utils.JackpotMgr.GetJackpotAmount(utils.JackpotSlots); e == nil {
				jackpotAmt = j
			}
		}
		if jackpotAmt != initialJackpot {
			// Re-fetch message (optional)
			finalEmbed2 := g.buildEmbed(formatReels(g.Reels), totalWinnings, xpGain, true, jackpotAmt)
			finalEmbed2.Fields = append(finalEmbed2.Fields, &discordgo.MessageEmbedField{Name: "Outcome", Value: outcome, Inline: false})
			finalEmbed2.Fields = append(finalEmbed2.Fields, &discordgo.MessageEmbedField{Name: "New Balance", Value: fmt.Sprintf("%s %s", utils.FormatChips(updatedUser.Chips), utils.ChipsEmoji), Inline: false})
			if jackpotPayout > 0 {
				finalEmbed2.Color = 0xFFD700
			}
			embeds2 := []*discordgo.MessageEmbed{finalEmbed2}
			if _, e := g.Session.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: g.MessageID, Channel: g.ChannelID, Embeds: &embeds2, Components: &components}); e != nil {
			}
		}
		afterRank := getRankForXP(updatedUser.TotalXP)
		if before.Name != "" && afterRank.Name != "" && afterRank.Name != before.Name {
			g.Session.FollowupMessageCreate(g.Interaction.Interaction, true, &discordgo.WebhookParams{Embeds: []*discordgo.MessageEmbed{utils.CreateBrandedEmbed("🎊 Rank Up!", fmt.Sprintf("%s %s → %s %s", before.Icon, before.Name, afterRank.Icon, afterRank.Name), 0xFFD700)}, Flags: discordgo.MessageFlagsEphemeral})
		}
		// finalize complete
	}(profit, xpGain, beforeRank, initialJackpot)
}

func getRandomSymbol(r *rand.Rand) string {
	all := []string{}
	weights := []float64{}
	for rarity, syms := range symbols {
		per := symbolWeights[rarity] / float64(len(syms))
		for range syms {
			weights = append(weights, per)
		}
		all = append(all, syms...)
	}
	x := r.Float64()
	cumulative := 0.0
	for i, w := range weights {
		cumulative += w
		if x <= cumulative {
			return all[i]
		}
	}
	return all[len(all)-1]
}

func (g *Game) createReels() [][]string {
	reels := make([][]string, 3)
	for r := 0; r < 3; r++ {
		row := make([]string, 3)
		for c := 0; c < 3; c++ {
			row[c] = getRandomSymbol(g.Rand)
		}
		reels[r] = row
	}
	return reels
}

func formatReels(reels [][]string) string {
	rows := make([]string, len(reels))
	for i, row := range reels {
		rows[i] = strings.Join(row, " ")
	}
	return strings.Join(rows, "\n")
}

func (g *Game) calculateResults() (int64, bool) {
	betPerLine := float64(g.Bet) / float64(payLines)
	lines := [][]string{g.Reels[0], g.Reels[1], g.Reels[2], {g.Reels[0][0], g.Reels[1][1], g.Reels[2][2]}, {g.Reels[0][2], g.Reels[1][1], g.Reels[2][0]}}
	total := int64(0)
	jackpot := false
	for _, line := range lines {
		if line[0] == line[1] && line[1] == line[2] {
			sym := line[0]
			total += int64(float64(payouts[sym]) * betPerLine)
			if sym == jackpotSymbol {
				jackpot = true
			}
		}
	}
	return total, jackpot
}

// getRankForXP replicates internal rank lookup (since utils does not export a helper)
func getRankForXP(xp int64) utils.Rank {
	var current utils.Rank
	for i := 0; i < len(utils.Ranks); i++ { // map iteration order not guaranteed; gather sequentially
		if rank, ok := utils.Ranks[i]; ok {
			if xp >= int64(rank.XPRequired) {
				current = rank
			} else {
				break
			}
		}
	}
	return current
}

// buildEmbed constructs a compact embed layout
// mode: "initial", "spinning", "final"
func (g *Game) buildEmbed(reels string, winnings int64, xpGain int64, final bool, jackpotAmount int64) *discordgo.MessageEmbed {
	mode := "spinning"
	if !final {
		if reels == "" {
			mode = "initial"
		}
	} else {
		mode = "final"
	}

	title := "🎰 Slot Machine"
	if mode == "final" {
		title = "🎰 Slot Machine Results"
	}

	color := 0x3498db
	if mode == "final" {
		// Use winnings as proxy (profit determined later when fields added)
		if winnings > 0 {
			color = 0x2ecc71
		} else {
			color = 0xe74c3c
		}
	}

	// Reels inside a code block for tight spacing
	desc := ""
	switch mode {
	case "initial":
		desc = "Spinning the reels..."
	case "spinning":
		if reels != "" {
			desc = fmt.Sprintf("Spinning the reels...\n```\n%s\n```", reels)
		} else {
			desc = "Spinning the reels..."
		}
	case "final":
		desc = fmt.Sprintf("```\n%s\n```", reels)
	}

	embed := utils.CreateBrandedEmbed(title, desc, color)
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1753050867/SL_d8ophs.png"}

	if mode == "final" {
		// Current Jackpot only
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Current Jackpot", Value: fmt.Sprintf("%s %s", utils.FormatChips(jackpotAmount), utils.ChipsEmoji), Inline: false})
		// Keep Bet field (required for spin again parsing)
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Bet", Value: fmt.Sprintf("%s %s", utils.FormatChips(g.Bet), utils.ChipsEmoji), Inline: true})
		if xpGain > 0 {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "XP", Value: fmt.Sprintf("+%s", utils.FormatChips(xpGain)), Inline: true})
		}
		embed.Footer = &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("You bet %s chips.", utils.FormatChips(g.Bet))}
	}
	return embed
}

// generateReelStrip creates a realistic reel strip that ends with the final result
func (g *Game) generateReelStrip(columnIndex int, finalColumn []string, stripLength int) []string {
	strip := make([]string, stripLength)

	// Fill most of the strip with random symbols
	for i := 0; i < stripLength-3; i++ {
		strip[i] = getRandomSymbol(g.Rand)
	}

	// Place the final result at the end of the strip
	for i := 0; i < 3; i++ {
		strip[stripLength-3+i] = finalColumn[i]
	}

	return strip
}

// calculateColumnDeceleration returns the delay for a specific column at a given step
func (g *Game) calculateColumnDeceleration(column, step, totalSteps, lockStep int) time.Duration {
	baseDelay := 120 * time.Millisecond

	if step < lockStep-3 {
		// Fast spinning phase
		return baseDelay
	} else if step < lockStep {
		// Deceleration phase - progressively slower
		decelerationFactor := float64(step-(lockStep-3)) / 3.0
		delay := baseDelay + time.Duration(float64(baseDelay)*decelerationFactor*2)
		return delay
	} else if step == lockStep {
		// Anticipation pause before final stop
		return 300 * time.Millisecond
	}

	// Column is locked
	return 0
}

func (g *Game) animateSpin(final [][]string) {
	g.Phase = phaseSpinning

	// Enhanced animation parameters
	totalSteps := 20
	col1Lock := 8
	col2Lock := 14
	col3Lock := 18
	stripLen := 30

	// Generate realistic reel strips for each column
	strips := make([][]string, 3)
	for c := 0; c < 3; c++ {
		finalColumn := []string{final[0][c], final[1][c], final[2][c]}
		strips[c] = g.generateReelStrip(c, finalColumn, stripLen)
	}

	// Track position for each column
	positions := []int{0, 0, 0}
	columnLocked := []bool{false, false, false}
	lockSteps := []int{col1Lock, col2Lock, col3Lock}

	makeFrame := func(step int) [][]string {
		frame := make([][]string, 3)
		for r := 0; r < 3; r++ {
			frame[r] = make([]string, 3)
		}

		for c := 0; c < 3; c++ {
			var colSyms []string

			if columnLocked[c] {
				// Column is locked, show final result
				colSyms = []string{final[0][c], final[1][c], final[2][c]}
			} else {
				// Column is still spinning
				if step >= lockSteps[c] {
					// Time to lock this column
					columnLocked[c] = true
					colSyms = []string{final[0][c], final[1][c], final[2][c]}
				} else {
					// Show current position in the strip
					pos := positions[c]
					colSyms = []string{
						strips[c][pos%stripLen],
						strips[c][(pos+1)%stripLen],
						strips[c][(pos+2)%stripLen],
					}
				}
			}

			for r := 0; r < 3; r++ {
				frame[r][c] = colSyms[r]
			}
		}

		return frame
	}

	// Animation loop - synchronous to block game completion until animation finishes
	for step := 0; step < totalSteps; step++ {
		frame := makeFrame(step)
		embed := g.buildEmbed(formatReels(frame), 0, 0, false, 0)
		embeds := []*discordgo.MessageEmbed{embed}

		// Update message asynchronously to avoid blocking animation timing
		go func(embeds []*discordgo.MessageEmbed) {
			defer func() { recover() }()
			g.Session.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: g.MessageID, Channel: g.ChannelID, Embeds: &embeds})
		}(embeds)

		// Calculate delays for each spinning column
		maxDelay := time.Duration(0)
		for c := 0; c < 3; c++ {
			if !columnLocked[c] {
				delay := g.calculateColumnDeceleration(c, step, totalSteps, lockSteps[c])
				if delay > maxDelay {
					maxDelay = delay
				}

				// Advance position if column is still spinning
				if step < lockSteps[c]-1 {
					// Normal advancement
					positions[c]++
				} else if step == lockSteps[c]-1 {
					// Position to show final result on next frame
					positions[c] = stripLen - 3
				}
			}
		}

		// Use the maximum delay from all spinning columns
		if step < totalSteps-1 && maxDelay > 0 {
			time.Sleep(maxDelay)
		}
	}

	// Animation completed - main game flow can now continue with results
}

// HandleSlotsInteraction processes "Spin Again" button
func HandleSlotsInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}
	cid := i.MessageComponentData().CustomID
	if !strings.HasPrefix(cid, "slots_spin_again_") {
		return
	}
	if i.Message == nil || len(i.Message.Embeds) == 0 {
		return
	}
	authorID := i.Message.Interaction.User.ID
	if authorID != i.Member.User.ID {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", "This isn't your game.", 0xFF0000), nil, true)
		return
	}
	betAmount := int64(0)
	for _, f := range i.Message.Embeds[0].Fields {
		if f.Name == "Bet" {
			parts := strings.Fields(f.Value)
			if len(parts) > 0 {
				if v, err := utils.ParseUserID(strings.ReplaceAll(parts[0], ",", "")); err == nil {
					betAmount = v
				}
			}
		}
	}
	userID, _ := utils.ParseUserID(i.Member.User.ID)
	user, _ := utils.GetCachedUser(userID)
	adjusted, note := normalizeBetForPaylines(betAmount, user.Chips)
	if adjusted == 0 {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", fmt.Sprintf("Bet must be at least %d and divisible by %d.", minBet, payLines), 0xFF0000), nil, true)
		return
	}
	// Update message immediately to spinning state (component update) and reuse same message
	spinning := utils.CreateBrandedEmbed("🎰 Slot Machine", "Spinning the reels...", 0x3498db)
	if err := utils.UpdateComponentInteractionWithTimeout(s, i, spinning, []discordgo.MessageComponent{}, 3*time.Second); err != nil {
		return
	}
	// Start game asynchronously to avoid blocking the interaction
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Try to send error message if possible
				utils.TryEphemeralFollowup(s, i, "An unexpected error occurred. Please try again.")
			}
		}()

		game := &Game{BaseGame: utils.NewBaseGame(s, i, adjusted, "slots"), Session: s, Phase: phaseInitial, Rand: rand.New(rand.NewSource(time.Now().UnixNano())), MessageID: i.Message.ID, ChannelID: i.ChannelID, UsedOriginal: true}
		game.BaseGame.CountWinLossMinRatio = 0.20
		if err := game.ValidateBet(); err != nil {
			utils.TryEphemeralFollowup(s, i, err.Error())
			return
		}
		if utils.JackpotMgr != nil {
			go func() { recover(); utils.JackpotMgr.ContributeToJackpot(utils.JackpotSlots, adjusted) }()
		}
		game.play()
	}()
	if note != "" {
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: note, Flags: discordgo.MessageFlagsEphemeral})
	}
}
