package baccarat

import (
	"fmt"
	"strings"
	"time"

	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
)

var activeBaccaratGames = make(map[int64]*Game)

type Game struct {
	*utils.BaseGame
	Choice      string
	Deck        *utils.Deck
	PlayerHand  []utils.Card
	BankerHand  []utils.Card
	PlayerScore int
	BankerScore int
	CreatedAt   time.Time
	Finished    bool
	ResultText  string
	Profit      int64
}

func RegisterBaccaratCommand() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "baccarat",
		Description: "Play a game of Baccarat",
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "bet", Description: "Bet amount (e.g. 500, 5k, half, all)", Required: true},
		},
	}
}

func HandleBaccaratCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := utils.ParseUserID(i.Member.User.ID)
	if _, exists := activeBaccaratGames[userID]; exists {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Baccarat", "You already have an active baccarat game.", 0xFF0000), nil, true)
		return
	}
	data := i.ApplicationCommandData()
	var betStr string
	for _, opt := range data.Options {
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
	game := &Game{BaseGame: utils.NewBaseGame(s, i, betAmount, "baccarat"), Deck: utils.NewDeck(6, "baccarat"), CreatedAt: time.Now()}
	if err := game.BaseGame.ValidateBet(); err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", err.Error(), 0xFF0000), nil, true)
		return
	}
	activeBaccaratGames[userID] = game
	// Send selection embed with buttons
	embed := baccaratStartEmbed(game)
	components := baccaratChoiceComponents()
	utils.SendInteractionResponse(s, i, embed, components, false)
}

// HandleBaccaratButton handles side selection via buttons
func HandleBaccaratButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := utils.ParseUserID(i.Member.User.ID)
	game, exists := activeBaccaratGames[userID]
	if !exists {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Baccarat", "No active baccarat game found.", 0xFF0000), nil, true)
		return
	}
	if game.Finished {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Baccarat", "Game already finished.", 0xFF0000), nil, true)
		return
	}
	choiceID := i.MessageComponentData().CustomID
	switch choiceID {
	case "baccarat_player":
		game.Choice = "player"
	case "baccarat_banker":
		game.Choice = "banker"
	case "baccarat_tie":
		game.Choice = "tie"
	default:
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Baccarat", "Invalid selection.", 0xFF0000), nil, true)
		return
	}
	// Play immediately then send component update
	game.play()
	game.finishViaComponentUpdate(s, i)
}

func (g *Game) baccaratValue(c utils.Card) int { return c.GetValue("baccarat") }

func (g *Game) updateScores() {
	total := 0
	for _, c := range g.PlayerHand {
		total += g.baccaratValue(c)
	}
	g.PlayerScore = total % 10
	total = 0
	for _, c := range g.BankerHand {
		total += g.baccaratValue(c)
	}
	g.BankerScore = total % 10
}

func (g *Game) deal() utils.Card { return g.Deck.Deal() }

func (g *Game) play() {
	g.PlayerHand = append(g.PlayerHand, g.deal(), g.deal())
	g.BankerHand = append(g.BankerHand, g.deal(), g.deal())
	g.updateScores()
	playerDraws := false
	if g.PlayerScore < 8 && g.BankerScore < 8 { // no naturals
		if g.PlayerScore <= 5 { // player draws
			g.PlayerHand = append(g.PlayerHand, g.deal())
			playerDraws = true
		}
		playerThirdVal := -1
		if playerDraws {
			playerThirdVal = g.baccaratValue(g.PlayerHand[2])
		}
		if !playerDraws && g.BankerScore <= 5 {
			g.BankerHand = append(g.BankerHand, g.deal())
		} else if playerDraws {
			switch g.BankerScore {
			case 0, 1, 2:
				g.BankerHand = append(g.BankerHand, g.deal())
			case 3:
				if playerThirdVal != 8 {
					g.BankerHand = append(g.BankerHand, g.deal())
				}
			case 4:
				if inIntSlice(playerThirdVal, []int{2, 3, 4, 5, 6, 7}) {
					g.BankerHand = append(g.BankerHand, g.deal())
				}
			case 5:
				if inIntSlice(playerThirdVal, []int{4, 5, 6, 7}) {
					g.BankerHand = append(g.BankerHand, g.deal())
				}
			case 6:
				if inIntSlice(playerThirdVal, []int{6, 7}) {
					g.BankerHand = append(g.BankerHand, g.deal())
				}
			}
		}
		g.updateScores()
	}
	winner := "tie"
	if g.PlayerScore > g.BankerScore {
		winner = "player"
	} else if g.BankerScore > g.PlayerScore {
		winner = "banker"
	}
	var profit int64
	switch {
	case g.Choice == winner && winner == "player":
		profit = int64(float64(g.Bet) * utils.BaccaratPayout)
	case g.Choice == winner && winner == "banker":
		profit = int64(float64(g.Bet) * utils.BaccaratPayout * (1 - utils.BaccaratBankerCommission))
	case g.Choice == winner && winner == "tie":
		profit = int64(float64(g.Bet) * utils.BaccaratTiePayout)
	case winner == "tie" && (g.Choice == "player" || g.Choice == "banker"):
		profit = 0
	default:
		profit = -g.Bet
	}
	g.ResultText = func() string {
		if winner == "tie" {
			return "It's a Tie!"
		}
		return capitalize(winner) + " wins!"
	}()
	g.Profit = profit
}

// finishViaComponentUpdate finalizes and updates via component interaction response
func (g *Game) finishViaComponentUpdate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	g.Finished = true
	updatedUser, _ := g.BaseGame.EndGame(g.Profit)
	var xpGain int64
	if g.Profit > 0 {
		xpGain = g.Profit * utils.XPPerProfit
	}
	embed := baccaratResultEmbed(g, updatedUser.Chips, xpGain)
	components := []discordgo.MessageComponent{utils.CreateActionRow(
		utils.CreateButton("baccarat_player", "Player", discordgo.SuccessButton, true, nil),
		utils.CreateButton("baccarat_banker", "Banker", discordgo.DangerButton, true, nil),
		utils.CreateButton("baccarat_tie", "Tie", discordgo.SecondaryButton, true, nil),
	)}
	if err := utils.UpdateComponentInteraction(s, i, embed, components); err != nil {
		utils.BotLogf("baccarat", "UpdateComponentInteraction failed for user %d: %v", g.UserID, err)
		// fallback channel edit
		embeds := []*discordgo.MessageEmbed{embed}
		edit := &discordgo.MessageEdit{ID: i.Message.ID, Channel: i.ChannelID, Embeds: &embeds, Components: &components}
		_, _ = s.ChannelMessageEditComplex(edit)
		// NOTE: This does not satisfy the interaction, so the client may show 'This interaction failed'.
		// As a secondary attempt, try sending an ephemeral ack if still possible.
		_ = utils.TryEphemeralFollowup(s, i, "⚠️ Display update failed, showing result above.")
	}
	delete(activeBaccaratGames, g.UserID)
}

// Start phase embed
func baccaratStartEmbed(g *Game) *discordgo.MessageEmbed {
	msg := fmt.Sprintf("You are betting %s %s.\nChoose your side.", utils.FormatChips(g.Bet), utils.ChipsEmoji)
	embed := utils.CreateBrandedEmbed("Baccarat", msg, utils.BotColor)
	return embed
}

func baccaratChoiceComponents() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{utils.CreateActionRow(
		utils.CreateButton("baccarat_player", "Player", discordgo.SuccessButton, false, nil),
		utils.CreateButton("baccarat_banker", "Banker", discordgo.DangerButton, false, nil),
		utils.CreateButton("baccarat_tie", "Tie", discordgo.SecondaryButton, false, nil),
	)}
}

func baccaratResultEmbed(g *Game, newBalance int64, xpGain int64) *discordgo.MessageEmbed {
	color := utils.BotColor
	if g.Profit > 0 {
		color = 0x2ECC71
	} else if g.Profit < 0 {
		color = 0xE74C3C
	} else {
		color = 0x95A5A6
	}
	embed := utils.CreateBrandedEmbed("Baccarat", "", color)
	embed.Fields = []*discordgo.MessageEmbedField{}
	// Hands (non-inline, with score in title)
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: fmt.Sprintf("Player's Hand - %d", g.PlayerScore), Value: joinCards(g.PlayerHand), Inline: false})
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: fmt.Sprintf("Banker's Hand - %d", g.BankerScore), Value: joinCards(g.BankerHand), Inline: false})
	// Outcome field
	betSide := capitalize(g.Choice)
	lines := []string{fmt.Sprintf("You bet on %s.", betSide), g.ResultText}
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Outcome", Value: strings.Join(lines, "\n"), Inline: false})
	// Profit / Loss field naming
	if g.Profit > 0 {
		profitVal := fmt.Sprintf("%s %s", utils.FormatChips(g.Profit), utils.ChipsEmoji)
		// Show commission detail for banker wins
		if g.Choice == "banker" && strings.Contains(strings.ToLower(g.ResultText), "banker wins") {
			commission := int64(float64(g.Bet) * utils.BaccaratBankerCommission)
			profitVal += fmt.Sprintf(" (5%% commission: -%s)", utils.FormatChips(commission))
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Winnings", Value: profitVal, Inline: false})
	} else if g.Profit < 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Losses", Value: fmt.Sprintf("%s %s", utils.FormatChips(-g.Profit), utils.ChipsEmoji), Inline: false})
	} else {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Result", Value: "Push", Inline: false})
	}
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "New Balance", Value: fmt.Sprintf("%s %s", utils.FormatChips(newBalance), utils.ChipsEmoji), Inline: false})
	embed.Footer.Text += " | Game Over"
	return embed
}

func joinCards(cards []utils.Card) string {
	parts := make([]string, len(cards))
	for i, c := range cards {
		parts[i] = "`" + c.String() + "`"
	}
	return strings.Join(parts, " ")
}

func inIntSlice(v int, list []int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	if len(s) == 1 {
		return strings.ToUpper(s)
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
