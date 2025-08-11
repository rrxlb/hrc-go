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
	betStr := i.ApplicationCommandData().Options[0].StringValue()
	userID, _ := utils.ParseUserID(i.Member.User.ID)
	user, err := utils.GetCachedUser(userID)
	if err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", "Error fetching user.", 0xFF0000), nil, true)
		return
	}
	bet, err := utils.ParseBet(betStr, user.Chips)
	if err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", err.Error(), 0xFF0000), nil, true)
		return
	}
	adjusted, note := normalizeBetForPaylines(bet, user.Chips)
	if adjusted == 0 {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", fmt.Sprintf("Bet must be at least %d and divisible by %d.", minBet, payLines), 0xFF0000), nil, true)
		return
	}

	game := &Game{BaseGame: utils.NewBaseGame(s, i, adjusted, "slots"), Session: s, Phase: phaseInitial, BetNote: note, Rand: rand.New(rand.NewSource(time.Now().UnixNano()))}
	game.BaseGame.CountWinLossMinRatio = 0.20
	if err := game.ValidateBet(); err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Slots", err.Error(), 0xFF0000), nil, true)
		return
	}

	if utils.JackpotMgr != nil { // base contribution similar to Python's ensure; here use configured rate
		utils.JackpotMgr.ContributeToJackpot(utils.JackpotSlots, adjusted)
	}

	utils.DeferInteractionResponse(s, i, false)
	go game.play()
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
	embed := g.buildEmbed("", 0, 0, false, 0)
	msg, err := g.Session.FollowupMessageCreate(g.Interaction.Interaction, true, &discordgo.WebhookParams{Embeds: []*discordgo.MessageEmbed{embed}})
	if err != nil {
		return
	}
	g.MessageID = msg.ID
	g.ChannelID = msg.ChannelID
	final := g.createReels()
	g.animateSpin(final)
	g.Reels = final
	totalWinnings, jackpotLine := g.calculateResults()
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
	finalEmbed.Fields = append(finalEmbed.Fields, &discordgo.MessageEmbedField{Name: "New Balance", Value: fmt.Sprintf("%s %s", utils.FormatChips(updatedUser.Chips), utils.ChipsEmoji), Inline: false})
	components := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{utils.CreateButton("slots_spin_again_"+g.MessageID, "Spin Again", discordgo.SuccessButton, false, nil)}}}
	// Edit followup message (not original interaction)
	embeds := []*discordgo.MessageEmbed{finalEmbed}
	g.Session.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: g.MessageID, Channel: g.ChannelID, Embeds: &embeds, Components: &components})
	if g.BetNote != "" {
		g.Session.FollowupMessageCreate(g.Interaction.Interaction, true, &discordgo.WebhookParams{Content: g.BetNote, Flags: discordgo.MessageFlagsEphemeral})
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

// buildEmbed constructs embed per current phase (light parity; refine after)
func (g *Game) buildEmbed(reels string, winnings int64, xpGain int64, final bool, jackpotAmount int64) *discordgo.MessageEmbed {
	color := 0x1E5631
	if final {
		if winnings > 0 {
			color = 0xFFD700
		} else {
			color = 0xFF0000
		}
	}
	title := "🎰 Slots"
	desc := "Spinning the reels..."
	if final {
		desc = "Final Results"
	}
	embed := utils.CreateBrandedEmbed(title, desc, color)
	if reels != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Reels", Value: fmt.Sprintf("``%s``", reels), Inline: false})
	}
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Bet", Value: fmt.Sprintf("%s %s", utils.FormatChips(g.Bet), utils.ChipsEmoji), Inline: true})
	if final && winnings > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Winnings", Value: fmt.Sprintf("%s %s", utils.FormatChips(winnings), utils.ChipsEmoji), Inline: true})
	}
	if final && xpGain > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "XP Gained", Value: utils.FormatChips(xpGain) + " XP", Inline: true})
	}
	if jackpotAmount > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Jackpot", Value: fmt.Sprintf("%s %s", utils.FormatChips(jackpotAmount), utils.ChipsEmoji), Inline: true})
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
