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
			{Type: discordgo.ApplicationCommandOptionString, Name: "choice", Description: "Bet on player, banker, or tie", Required: true, Choices: []*discordgo.ApplicationCommandOptionChoice{{Name: "Player", Value: "player"}, {Name: "Banker", Value: "banker"}, {Name: "Tie", Value: "tie"}}},
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
	var betStr, choice string
	for _, opt := range data.Options {
		switch opt.Name {
		case "bet":
			betStr = opt.StringValue()
		case "choice":
			choice = strings.ToLower(opt.StringValue())
		}
	}
	if choice != "player" && choice != "banker" && choice != "tie" {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "Choice must be player, banker, or tie.", 0xFF0000), nil, true)
		return
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
	game := &Game{BaseGame: utils.NewBaseGame(s, i, betAmount, "baccarat"), Choice: choice, Deck: utils.NewDeck(6, "baccarat"), CreatedAt: time.Now()}
	if err := game.BaseGame.ValidateBet(); err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", err.Error(), 0xFF0000), nil, true)
		return
	}
	activeBaccaratGames[userID] = game
	game.play()
	game.finish(s)
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
		return strings.Title(winner) + " wins!"
	}()
	g.Profit = profit
}

func (g *Game) finish(s *discordgo.Session) {
	g.Finished = true
	updatedUser, _ := g.BaseGame.EndGame(g.Profit)
	var xpGain int64
	if g.Profit > 0 {
		xpGain = g.Profit * utils.XPPerProfit
	}
	embed := baccaratEmbed(g, updatedUser.Chips, xpGain)
	utils.SendInteractionResponse(s, g.Interaction, embed, nil, false)
	delete(activeBaccaratGames, g.UserID)
}

func baccaratEmbed(g *Game, newBalance int64, xpGain int64) *discordgo.MessageEmbed {
	title := "🃏 Baccarat"
	description := g.ResultText
	color := utils.BotColor
	if g.Profit > 0 {
		color = 0x2ECC71
	} else if g.Profit < 0 {
		color = 0xE74C3C
	} else {
		color = 0x95A5A6
	}
	embed := utils.CreateBrandedEmbed(title, description, color)
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1753042166/3_vxurig.png"}
	embed.Fields = []*discordgo.MessageEmbedField{{Name: "Player Hand", Value: fmt.Sprintf("`%s` (%d)", joinCards(g.PlayerHand), g.PlayerScore), Inline: true}, {Name: "Banker Hand", Value: fmt.Sprintf("`%s` (%d)", joinCards(g.BankerHand), g.BankerScore), Inline: true}}
	profitFieldName := "Result"
	profitVal := "Push"
	if g.Profit > 0 {
		profitFieldName = "Profit"
		profitVal = fmt.Sprintf("+%s %s", utils.FormatChips(g.Profit), utils.ChipsEmoji)
	} else if g.Profit < 0 {
		profitFieldName = "Loss"
		profitVal = fmt.Sprintf("%s %s", utils.FormatChips(-g.Profit), utils.ChipsEmoji)
	}
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Bet", Value: fmt.Sprintf("%s %s", utils.FormatChips(g.Bet), utils.ChipsEmoji), Inline: true}, &discordgo.MessageEmbedField{Name: profitFieldName, Value: profitVal, Inline: true}, &discordgo.MessageEmbedField{Name: "New Balance", Value: fmt.Sprintf("%s %s", utils.FormatChips(newBalance), utils.ChipsEmoji), Inline: true})
	if xpGain > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "XP Gained", Value: fmt.Sprintf("%s XP", utils.FormatChips(xpGain)), Inline: true})
	}
	embed.Footer.Text += " | Game Over"
	return embed
}

func joinCards(cards []utils.Card) string {
	parts := make([]string, len(cards))
	for i, c := range cards {
		parts[i] = c.String()
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
