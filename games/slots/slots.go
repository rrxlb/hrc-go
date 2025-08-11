package slots

import (
	"fmt"
	"log"
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
	Session   *discordgo.Session
	Reels     [][]string
	MessageID string
	ChannelID string
	Phase     phase
	BetNote   string
	Rand      *rand.Rand
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
	start := time.Now()
	log.Printf("[slots] command invoked by user=%s", i.Member.User.ID)
	betStr := i.ApplicationCommandData().Options[0].StringValue()
	// Defer IMMEDIATELY to avoid 3s timeout regardless of downstream latency
	if err := utils.DeferInteractionResponse(s, i, false); err != nil {
		log.Printf("[slots] failed immediate defer: %v", err)
		return
	}
	log.Printf("[slots] deferred in %dms", time.Since(start).Milliseconds())

	userID, _ := utils.ParseUserID(i.Member.User.ID)
	user, err := utils.GetCachedUser(userID)
	if err != nil {
		utils.TryEphemeralFollowup(s, i, "Error fetching user data.")
		return
	}
	bet, err := utils.ParseBet(betStr, user.Chips)
	if err != nil {
		utils.TryEphemeralFollowup(s, i, fmt.Sprintf("Bet error: %s", err.Error()))
		return
	}
	adjusted, note := normalizeBetForPaylines(bet, user.Chips)
	if adjusted == 0 {
		utils.TryEphemeralFollowup(s, i, fmt.Sprintf("Bet must be at least %d and divisible by %d.", minBet, payLines))
		return
	}

	game := &Game{BaseGame: utils.NewBaseGame(s, i, adjusted, "slots"), Session: s, Phase: phaseInitial, BetNote: note, Rand: rand.New(rand.NewSource(time.Now().UnixNano()))}
	game.BaseGame.CountWinLossMinRatio = 0.20
	if err := game.ValidateBet(); err != nil {
		utils.TryEphemeralFollowup(s, i, err.Error())
		return
	}

	if utils.JackpotMgr != nil {
		utils.JackpotMgr.ContributeToJackpot(utils.JackpotSlots, adjusted)
	}

	// Run play asynchronously so we return control; animation + followups use webhook token
	go game.play()
	log.Printf("[slots] launched play goroutine user=%s bet=%d", i.Member.User.ID, adjusted)
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
			log.Printf("[slots] panic recovered: %v", r)
		}
	}()
	playStart := time.Now()
	log.Printf("[slots] play() start user=%d bet=%d", g.UserID, g.Bet)
	// Capture pre-game rank for rank-up detection
	beforeRank := getRankForXP(func() int64 {
		if g.BaseGame.UserData != nil {
			return g.BaseGame.UserData.TotalXP
		}
		return 0
	}())

	embed := g.buildEmbed("", 0, 0, false, 0)
	msg, err := g.Session.FollowupMessageCreate(g.Interaction.Interaction, true, &discordgo.WebhookParams{Embeds: []*discordgo.MessageEmbed{embed}})
	if err != nil {
		log.Printf("[slots] followup create failed: %v (attempting edit original)", err)
		// Fallback: edit original interaction response
		if _, editErr := g.Session.InteractionResponseEdit(g.Interaction.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}}); editErr != nil {
			log.Printf("[slots] edit original also failed: %v", editErr)
			return
		} else {
			log.Printf("[slots] fallback edit original succeeded")
			// Try to fetch original to get ID (not strictly needed for animation; skip animation if no ID)
			orig, getErr := g.Session.InteractionResponse(g.Interaction.Interaction)
			if getErr == nil {
				g.MessageID = orig.ID
				g.ChannelID = orig.ChannelID
			} else {
				log.Printf("[slots] could not fetch original response: %v", getErr)
			}
		}
	} else {
		g.MessageID = msg.ID
		g.ChannelID = msg.ChannelID
		log.Printf("[slots] followup message created id=%s channel=%s", g.MessageID, g.ChannelID)
	}

	if g.MessageID == "" || g.ChannelID == "" {
		log.Printf("[slots] missing message identifiers; aborting animation")
		return
	}
	final := g.createReels()
	log.Printf("[slots] reels generated user=%d symbols=%s", g.UserID, formatReels(final))
	g.animateSpin(final)
	g.Reels = final
	totalWinnings, jackpotLine := g.calculateResults()
	log.Printf("[slots] results calculated winnings=%d jackpotLine=%v", totalWinnings, jackpotLine)
	jackpotPayout := int64(0)
	if jackpotLine && utils.JackpotMgr != nil {
		won, amount, _ := utils.JackpotMgr.TryWinJackpot(utils.JackpotSlots, g.UserID, g.Bet, 1.0) // guarantee on line
		if won {
			jackpotPayout = amount
			totalWinnings += amount
		}
	}
	profit := totalWinnings - g.Bet
	if profit < 0 && utils.JackpotMgr != nil {
		loss := -profit
		utils.JackpotMgr.AddJackpotAmount(utils.JackpotSlots, int64(float64(loss)*jackpotLossContributionRate))
	}
	updatedUser, _ := g.EndGame(profit)
	xpGain := int64(0)
	if profit > 0 {
		xpGain = profit * utils.XPPerProfit
	}
	jackpotAmount := int64(0)
	if utils.JackpotMgr != nil {
		if j, err := utils.JackpotMgr.GetJackpotAmount(utils.JackpotSlots); err == nil {
			jackpotAmount = j
		}
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
	finalEmbed.Fields = append(finalEmbed.Fields, &discordgo.MessageEmbedField{Name: "Outcome", Value: outcome, Inline: false})
	// Append New Balance at end
	finalEmbed.Fields = append(finalEmbed.Fields, &discordgo.MessageEmbedField{Name: "New Balance", Value: fmt.Sprintf("%s %s", utils.FormatChips(updatedUser.Chips), utils.ChipsEmoji), Inline: false})
	// Jackpot color override
	if jackpotPayout > 0 {
		finalEmbed.Color = 0xFFD700
	}
	components := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{utils.CreateButton("slots_spin_again_"+g.MessageID, "Spin Again", discordgo.SuccessButton, false, nil)}}}
	// Edit followup message (not original interaction)
	embeds := []*discordgo.MessageEmbed{finalEmbed}
	if _, err := g.Session.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: g.MessageID, Channel: g.ChannelID, Embeds: &embeds, Components: &components}); err != nil {
		log.Printf("[slots] final edit failed: %v", err)
	} else {
		log.Printf("[slots] final edit success user=%d duration=%dms", g.UserID, time.Since(playStart).Milliseconds())
	}
	if g.BetNote != "" {
		g.Session.FollowupMessageCreate(g.Interaction.Interaction, true, &discordgo.WebhookParams{Content: g.BetNote, Flags: discordgo.MessageFlagsEphemeral})
	}

	// Rank-up ephemeral notice
	afterRank := getRankForXP(updatedUser.TotalXP)
	if beforeRank.Name != "" && afterRank.Name != "" && afterRank.Name != beforeRank.Name {
		g.Session.FollowupMessageCreate(g.Interaction.Interaction, true, &discordgo.WebhookParams{Embeds: []*discordgo.MessageEmbed{utils.CreateBrandedEmbed("🎊 Rank Up!", fmt.Sprintf("%s %s → %s %s", beforeRank.Icon, beforeRank.Name, afterRank.Icon, afterRank.Name), 0xFFD700)}, Flags: discordgo.MessageFlagsEphemeral})
	}
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

// buildEmbed constructs embed with parity to Python create_slots_embed
func (g *Game) buildEmbed(reels string, winnings int64, xpGain int64, final bool, jackpotAmount int64) *discordgo.MessageEmbed {
	var title, description string
	var color int

	if !final {
		if reels == "" { // initial
			title = "Slot Machine"
			description = "Spinning the reels..."
			color = 0x3498db // blue
		} else { // spinning frame
			title = "Slot Machine"
			description = fmt.Sprintf("Spinning the reels...\n\n%s", reels)
			color = 0x3498db
		}
	} else { // final
		title = "Slot Machine Results"
		description = reels
		if winnings > 0 {
			color = 0x2ecc71 // green for win
		} else {
			color = 0xe74c3c // red for loss
		}
	}

	embed := utils.CreateBrandedEmbed(title, description, color)
	// Thumbnail parity (slots icon)
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1753050867/SL_d8ophs.png"}

	if final {
		// Spacer field
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "\u200b", Value: "\u200b", Inline: false})
		// Jackpot field (always show current amount for visibility)
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Jackpot", Value: fmt.Sprintf("%s %s", utils.FormatChips(jackpotAmount), utils.ChipsEmoji), Inline: true})
		// Bet field (needed for Spin Again parsing)
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Bet", Value: fmt.Sprintf("%s %s", utils.FormatChips(g.Bet), utils.ChipsEmoji), Inline: true})
		if winnings > 0 {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Winnings", Value: fmt.Sprintf("%s %s", utils.FormatChips(winnings), utils.ChipsEmoji), Inline: true})
			if xpGain > 0 { // XP gating simplified: always show
				embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "XP Gained", Value: fmt.Sprintf("+%s XP", utils.FormatChips(xpGain)), Inline: true})
			}
		}
		embed.Footer = &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("You bet %s chips.", utils.FormatChips(g.Bet))}
	}

	return embed
}

func (g *Game) animateSpin(final [][]string) {
	g.Phase = phaseSpinning
	stripLen := 21
	strips := make([][]string, 3)
	for c := 0; c < 3; c++ {
		col := make([]string, stripLen)
		for i := 0; i < stripLen; i++ {
			col[i] = getRandomSymbol(g.Rand)
		}
		strips[c] = col
	}
	idx := []int{0, 0, 0}
	stopSteps := []int{int(spinFrames * 6 / 10), int(spinFrames * 8 / 10), spinFrames}
	for step := 1; step <= spinFrames; step++ {
		frame := make([][]string, 3)
		for r := 0; r < 3; r++ {
			frame[r] = make([]string, 3)
		}
		for c := 0; c < 3; c++ {
			var colSyms []string
			if step < stopSteps[c] {
				colSyms = []string{strips[c][(idx[c])%stripLen], strips[c][(idx[c]+1)%stripLen], strips[c][(idx[c]+2)%stripLen]}
				idx[c] = (idx[c] + 1) % stripLen
			} else {
				colSyms = []string{final[0][c], final[1][c], final[2][c]}
			}
			for r := 0; r < 3; r++ {
				frame[r][c] = colSyms[r]
			}
		}
		reelsTxt := formatReels(frame)
		embed := g.buildEmbed(reelsTxt, 0, 0, false, 0)
		embeds := []*discordgo.MessageEmbed{embed}
		g.Session.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: g.MessageID, Channel: g.ChannelID, Embeds: &embeds})
		t := float64(step) / float64(spinFrames)
		delay := 0.06 + (0.18-0.06)*(t*t)
		time.Sleep(time.Duration(delay*1000) * time.Millisecond)
	}
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
	// Acknowledge and remove buttons
	utils.AcknowledgeComponentInteraction(s, i)
	s.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: i.Message.ID, Channel: i.ChannelID, Components: &[]discordgo.MessageComponent{}})
	game := &Game{BaseGame: utils.NewBaseGame(s, i, adjusted, "slots"), Session: s, Phase: phaseInitial, Rand: rand.New(rand.NewSource(time.Now().UnixNano()))}
	game.BaseGame.CountWinLossMinRatio = 0.20
	if err := game.ValidateBet(); err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", err.Error(), 0xFF0000), nil, true)
		return
	}
	if utils.JackpotMgr != nil {
		utils.JackpotMgr.ContributeToJackpot(utils.JackpotSlots, adjusted)
	}
	go game.play()
	if note != "" {
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: note, Flags: discordgo.MessageFlagsEphemeral})
	}
}
