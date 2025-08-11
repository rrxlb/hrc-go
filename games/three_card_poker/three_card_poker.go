package threecardpoker

import (
	"sort"
	"strconv"
	"time"

	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
)

const (
	tcpGameType = "three_card_poker"
	tcpTimeout  = 90 * time.Second
)

var (
	anteBonusPayouts = map[string]int64{"Straight Flush": 5, "Three of a Kind": 4, "Straight": 1}
	pairPlusPayouts  = map[string]int64{"Straight Flush": 40, "Three of a Kind": 30, "Straight": 6, "Flush": 3, "Pair": 1}
	activeTCPGames   = make(map[int64]*TCPGame)
)

type TCPGame struct {
	*utils.BaseGame // BaseGame.Bet represents Ante
	PairPlusBet     int64
	PlayBet         int64
	Deck            *utils.Deck
	PlayerHand      []utils.Card
	DealerHand      []utils.Card
	PlayerEval      HandEval
	DealerEval      HandEval
	StartedAt       time.Time
	Finished        bool
}

type HandEval struct {
	Name     string
	Rank     int
	Tiebreak []int
}

func RegisterThreeCardPokerCommand() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{Name: "tcpoker", Description: "Play Three Card Poker.", Options: []*discordgo.ApplicationCommandOption{
		{Type: discordgo.ApplicationCommandOptionString, Name: "ante", Description: "Ante bet", Required: true},
		{Type: discordgo.ApplicationCommandOptionString, Name: "pairplus", Description: "Pair Plus bet", Required: false},
	}}
}

func HandleThreeCardPokerCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
	if _, exists := activeTCPGames[userID]; exists {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Three Card Poker", "You already have an active game.", 0xFF0000), nil, true)
		return
	}
	data := i.ApplicationCommandData()
	var anteStr, ppStr string
	for _, opt := range data.Options {
		if opt.Name == "ante" {
			anteStr = opt.StringValue()
		} else if opt.Name == "pairplus" {
			ppStr = opt.StringValue()
		}
	}
	user, err := utils.GetCachedUser(userID)
	if err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "Failed to load user.", 0xFF0000), nil, true)
		return
	}
	ante, err := utils.ParseBet(anteStr, user.Chips)
	if err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", err.Error(), 0xFF0000), nil, true)
		return
	}
	pairPlus := int64(0)
	if ppStr != "" {
		pairPlus, err = utils.ParseBet(ppStr, user.Chips-ante)
		if err != nil {
			utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", err.Error(), 0xFF0000), nil, true)
			return
		}
	}
	totalPotential := ante*2 + pairPlus
	if user.Chips < totalPotential {
		utils.SendInteractionResponse(s, i, utils.InsufficientChipsEmbed(totalPotential, user.Chips, "all bets (Ante, Pair Plus, and Play)"), nil, true)
		return
	}
	game := &TCPGame{BaseGame: utils.NewBaseGame(s, i, ante, tcpGameType), PairPlusBet: pairPlus, Deck: utils.NewDeck(1, "poker"), StartedAt: time.Now()}
	game.BaseGame.Bet = totalPotential
	if err := game.BaseGame.ValidateBet(); err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", err.Error(), 0xFF0000), nil, true)
		return
	}
	game.BaseGame.Bet = ante
	activeTCPGames[userID] = game
	utils.DeferInteractionResponse(s, i, false)
	game.start(s, i)
}

func (g *TCPGame) start(s *discordgo.Session, i *discordgo.InteractionCreate) {
	g.PlayerHand = g.Deck.DealMultiple(3)
	g.DealerHand = g.Deck.DealMultiple(3)
	g.PlayerEval = evaluateThreeCardHand(g.PlayerHand)
	g.DealerEval = evaluateThreeCardHand(g.DealerHand)
	// Pass placeholder dealer eval during initial state (will be revealed on finish)
	embed := utils.ThreeCardPokerEmbed("initial", cardsToStrings(g.PlayerHand), cardsToStrings(g.DealerHand), g.PlayerEval.Name, "Hidden", g.Bet, g.PairPlusBet, 0, "", nil, 0, 0, 0)
	utils.SendFollowupMessage(s, i, embed, g.buildComponents(), false)
}

func (g *TCPGame) buildComponents() []discordgo.MessageComponent {
	if g.Finished {
		return nil
	}
	return []discordgo.MessageComponent{utils.CreateActionRow(
		utils.CreateButton("tcp_play", "Play", discordgo.SuccessButton, false, &discordgo.ComponentEmoji{Name: "✅"}),
		utils.CreateButton("tcp_fold", "Fold", discordgo.DangerButton, false, &discordgo.ComponentEmoji{Name: "🛑"}),
	)}
}

func HandleThreeCardPokerInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cid := i.MessageComponentData().CustomID
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
	game, ok := activeTCPGames[userID]
	if !ok {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Three Card Poker", "No active game.", 0xFF0000), nil, true)
		return
	}
	if game.Finished {
		utils.AcknowledgeComponentInteraction(s, i)
		return
	}
	switch cid {
	case "tcp_play":
		game.handlePlay(s, i)
	case "tcp_fold":
		game.handleFold(s, i, false)
	}
}

func (g *TCPGame) handlePlay(s *discordgo.Session, i *discordgo.InteractionCreate) {
	totalNeeded := g.Bet + g.PairPlusBet + g.Bet
	user, _ := utils.GetCachedUser(g.UserID)
	if user.Chips < totalNeeded {
		g.handleFold(s, i, true)
		return
	}
	g.PlayBet = g.Bet
	g.finish(s, i, false)
}

func (g *TCPGame) handleFold(s *discordgo.Session, i *discordgo.InteractionCreate, forced bool) {
	g.finish(s, i, true)
}

func (g *TCPGame) finish(s *discordgo.Session, i *discordgo.InteractionCreate, folded bool) {
	if g.Finished {
		return
	}
	g.Finished = true
	profit := int64(0)
	payoutLines := []string{}
	outcome := ""
	if g.PairPlusBet > 0 {
		if !folded {
			if mult, ok := pairPlusPayouts[g.PlayerEval.Name]; ok {
				win := g.PairPlusBet * mult
				profit += win
				payoutLines = append(payoutLines, "Pair Plus: `+"+utils.FormatChips(win)+"`")
			} else {
				profit -= g.PairPlusBet
				payoutLines = append(payoutLines, "Pair Plus: `-"+utils.FormatChips(g.PairPlusBet)+"`")
			}
		} else {
			profit -= g.PairPlusBet
			payoutLines = append(payoutLines, "Pair Plus: `-"+utils.FormatChips(g.PairPlusBet)+"`")
		}
	}
	if folded {
		profit -= g.Bet
		outcome = "You folded and forfeited your Ante bet."
	} else {
		dealerQualifies := g.DealerEval.Rank > 3 || (g.DealerEval.Rank == 3 && len(g.DealerEval.Tiebreak) > 0 && g.DealerEval.Tiebreak[0] >= 12)
		playerWins := g.PlayerEval.Rank > g.DealerEval.Rank || (g.PlayerEval.Rank == g.DealerEval.Rank && compareTiebreak(g.PlayerEval.Tiebreak, g.DealerEval.Tiebreak) > 0)
		if !dealerQualifies {
			outcome = "Dealer does not qualify. Ante wins, Play pushes."
			profit += g.Bet
		} else if playerWins {
			outcome = "You win with a " + g.PlayerEval.Name + "!"
			profit += g.Bet + g.Bet
			if mult, ok := anteBonusPayouts[g.PlayerEval.Name]; ok {
				bonus := g.Bet * mult
				profit += bonus
				payoutLines = append(payoutLines, "Ante Bonus: `+"+utils.FormatChips(bonus)+"`")
			}
		} else {
			outcome = "Dealer wins with a " + g.DealerEval.Name + "."
			profit -= (g.Bet + g.Bet)
		}
	}
	updatedUser, _ := g.BaseGame.EndGame(profit)
	var xpGain int64
	if profit > 0 {
		xpGain = profit * utils.XPPerProfit
	}
	embed := utils.ThreeCardPokerEmbed("final", cardsToStrings(g.PlayerHand), cardsToStrings(g.DealerHand), g.PlayerEval.Name, g.DealerEval.Name, g.Bet, g.PairPlusBet, g.PlayBet, outcome, payoutLines, updatedUser.Chips, profit, xpGain)
	utils.UpdateComponentInteraction(s, i, embed, nil)
	delete(activeTCPGames, g.UserID)
}

func evaluateThreeCardHand(hand []utils.Card) HandEval {
	values := make([]int, 3)
	for i, c := range hand {
		values[i] = valueForCard(c)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] > values[j] })
	suits := []string{hand[0].Suit, hand[1].Suit, hand[2].Suit}
	isFlush := (suits[0] == suits[1] && suits[1] == suits[2])
	isStraight := (values[0]-1 == values[1] && values[1]-1 == values[2]) || (values[0] == 14 && values[1] == 3 && values[2] == 2)
	displayValues := append([]int{}, values...)
	if values[0] == 14 && values[1] == 3 && values[2] == 2 {
		displayValues = []int{3, 2, 1}
	}
	if isStraight && isFlush {
		return HandEval{"Straight Flush", 8, displayValues}
	}
	if values[0] == values[1] && values[1] == values[2] {
		return HandEval{"Three of a Kind", 7, values}
	}
	if isStraight {
		return HandEval{"Straight", 6, displayValues}
	}
	if isFlush {
		return HandEval{"Flush", 5, values}
	}
	if values[0] == values[1] || values[1] == values[2] {
		pairVal := values[1]
		kicker := values[0]
		if values[1] == values[2] {
			kicker = values[2]
		}
		return HandEval{"Pair", 4, []int{pairVal, kicker}}
	}
	return HandEval{"High Card", 3, values}
}

func valueForCard(c utils.Card) int { return c.GetValue("poker") }

func compareTiebreak(a, b []int) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] > b[i] {
			return 1
		} else if a[i] < b[i] {
			return -1
		}
	}
	return 0
}

func cardsToStrings(cards []utils.Card) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = c.String()
	}
	return out
}
