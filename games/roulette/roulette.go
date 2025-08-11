package roulette

import (
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
)

var redNumbers = map[int]struct{}{1: {}, 3: {}, 5: {}, 7: {}, 9: {}, 12: {}, 14: {}, 16: {}, 18: {}, 19: {}, 21: {}, 23: {}, 25: {}, 27: {}, 30: {}, 32: {}, 34: {}, 36: {}}
var blackNumbers = map[int]struct{}{2: {}, 4: {}, 6: {}, 8: {}, 10: {}, 11: {}, 13: {}, 15: {}, 17: {}, 20: {}, 22: {}, 24: {}, 26: {}, 28: {}, 29: {}, 31: {}, 33: {}, 35: {}}

var activeRouletteGames = make(map[int64]*RouletteGame)

type RouletteGame struct {
	*utils.BaseGame
	Bets         map[string]int64
	ResultNumber int
	ResultColor  string
	State        string
}

func RegisterRouletteCommand() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{Name: "roulette", Description: "Play a game of Roulette"}
}

func HandleRouletteCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := utils.ParseUserID(i.Member.User.ID)
	if _, exists := activeRouletteGames[userID]; exists {
		_ = utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Roulette", "You already have an active roulette game.", 0xFF0000), nil, true)
		return
	}
	game := &RouletteGame{BaseGame: utils.NewBaseGame(s, i, 0, "roulette"), Bets: make(map[string]int64), State: "betting"}
	activeRouletteGames[userID] = game
	embed := utils.RouletteGameEmbed("betting", game.Bets, 0, "", 0, 0, 0)
	if err := utils.SendInteractionResponse(s, i, embed, game.buildComponents(), false); err != nil {
		log.Printf("roulette: failed initial InteractionRespond for user %d: %v", userID, err)
		// Clean up so user can retry
		delete(activeRouletteGames, userID)
		// Attempt a simple ephemeral fallback if interaction still valid
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: "❌ Failed to start roulette (Discord error). Please try /roulette again.", Flags: discordgo.MessageFlagsEphemeral}})
	}
}

func (rg *RouletteGame) buildComponents() []discordgo.MessageComponent {
	if rg.State == "final" {
		return nil
	}
	row1 := []discordgo.MessageComponent{
		// Even-money bets now open a modal to specify wager amount (distinct styles)
		utils.CreateButton("roulette_bet_red", "Red", discordgo.DangerButton, false, &discordgo.ComponentEmoji{Name: "🟥"}),
		utils.CreateButton("roulette_bet_black", "Black", discordgo.SecondaryButton, false, &discordgo.ComponentEmoji{Name: "⬛"}),
		utils.CreateButton("roulette_bet_odd", "Odd", discordgo.SecondaryButton, false, nil),
		utils.CreateButton("roulette_bet_even", "Even", discordgo.SecondaryButton, false, nil),
		// Removed invalid '#' emoji (caused BUTTON_COMPONENT_INVALID_EMOJI); leaving without emoji
		utils.CreateButton("roulette_bet_single", "Single", discordgo.SecondaryButton, false, nil),
	}
	row2 := []discordgo.MessageComponent{
		utils.CreateButton("roulette_bet_1-18", "1-18", discordgo.SecondaryButton, false, nil),
		utils.CreateButton("roulette_bet_19-36", "19-36", discordgo.SecondaryButton, false, nil),
		utils.CreateButton("roulette_bet_dozen1", "1-12", discordgo.SecondaryButton, false, nil),
		utils.CreateButton("roulette_bet_dozen2", "13-24", discordgo.SecondaryButton, false, nil),
		utils.CreateButton("roulette_bet_dozen3", "25-36", discordgo.SecondaryButton, false, nil),
	}
	row3 := []discordgo.MessageComponent{
		utils.CreateButton("roulette_bet_col1", "Col 1", discordgo.SecondaryButton, false, nil),
		utils.CreateButton("roulette_bet_col2", "Col 2", discordgo.SecondaryButton, false, nil),
		utils.CreateButton("roulette_bet_col3", "Col 3", discordgo.SecondaryButton, false, nil),
		utils.CreateButton("roulette_spin", "Spin", discordgo.SuccessButton, len(rg.Bets) == 0, &discordgo.ComponentEmoji{Name: "🎡"}),
	}
	return []discordgo.MessageComponent{utils.CreateActionRow(row1...), utils.CreateActionRow(row2...), utils.CreateActionRow(row3...)}
}

func HandleRouletteInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := utils.ParseUserID(i.Member.User.ID)
	game, exists := activeRouletteGames[userID]
	if !exists {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Roulette", "No active roulette game.", 0xFF0000), nil, true)
		return
	}
	cid := i.MessageComponentData().CustomID
	if cid == "roulette_spin" {
		if len(game.Bets) == 0 {
			utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Roulette", "Place at least one bet first.", 0xFF0000), nil, true)
			return
		}
		game.State = "spinning"
		utils.UpdateComponentInteraction(s, i, utils.RouletteGameEmbed("spinning", game.Bets, 0, "", 0, 0, 0), game.buildComponents())
		go game.resolveSpin(s)
		return
	}
	if strings.HasPrefix(cid, "roulette_bet_") {
		betType := strings.TrimPrefix(cid, "roulette_bet_")
		// Open modal for ALL bet types so user selects amount (and number for single)
		modalID := "roulette_bet_modal_" + betType
		components := []discordgo.MessageComponent{}
		// Wager input (common)
		wagerInput := &discordgo.TextInput{
			CustomID:    "wager",
			Label:       "Bet Amount",
			Style:       discordgo.TextInputShort,
			Placeholder: "e.g. 100, 5k, half, all",
			Required:    true,
		}
		if betType == "single" {
			// Add number input
			numberInput := &discordgo.TextInput{
				CustomID:    "number",
				Label:       "Number (0-36)",
				Style:       discordgo.TextInputShort,
				Placeholder: "e.g. 17",
				Required:    true,
			}
			components = append(components, utils.CreateActionRow(numberInput))
		}
		components = append(components, utils.CreateActionRow(wagerInput))
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID:   modalID,
				Title:      "Place " + strings.Title(strings.ReplaceAll(betType, "_", " ")) + " Bet",
				Components: components,
			},
		})
	}
}

func (rg *RouletteGame) resolveSpin(s *discordgo.Session) {
	time.Sleep(2 * time.Second)
	rand.Seed(time.Now().UnixNano())
	num := rand.Intn(37)
	color := "green"
	if num != 0 {
		if _, ok := redNumbers[num]; ok {
			color = "red"
		} else {
			color = "black"
		}
	}
	rg.ResultNumber = num
	rg.ResultColor = color
	profit := rg.calculateProfit()
	rg.BaseGame.Bet = rg.totalBet()
	updatedUser, _ := rg.BaseGame.EndGame(profit)
	newBalance := updatedUser.Chips
	var xpGain int64
	if profit > 0 {
		xpGain = profit * utils.XPPerProfit
	}
	rg.State = "final"
	utils.EditOriginalInteraction(s, rg.BaseGame.Interaction, utils.RouletteGameEmbed("final", rg.Bets, num, color, profit, newBalance, xpGain), nil)
	delete(activeRouletteGames, rg.UserID)
}

func (rg *RouletteGame) totalBet() int64 {
	var t int64
	for _, v := range rg.Bets {
		t += v
	}
	return t
}

// HandleRouletteModal processes wager inputs from modal submissions for even-money bets
func HandleRouletteModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionModalSubmit {
		return
	}
	customID := i.ModalSubmitData().CustomID
	if !strings.HasPrefix(customID, "roulette_bet_modal_") {
		return
	}
	betType := strings.TrimPrefix(customID, "roulette_bet_modal_")
	userID, _ := utils.ParseUserID(i.Member.User.ID)
	game, exists := activeRouletteGames[userID]
	if !exists {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Roulette", "No active roulette game.", 0xFF0000), nil, true)
		return
	}
	// Extract wager input value
	var wagerStr string
	var numberStr string
	for _, comp := range i.ModalSubmitData().Components { // each is an action row
		if row, ok := comp.(*discordgo.ActionsRow); ok {
			for _, inner := range row.Components {
				if ti, ok := inner.(*discordgo.TextInput); ok && ti.CustomID == "wager" {
					wagerStr = ti.Value
				} else if ti, ok := inner.(*discordgo.TextInput); ok && ti.CustomID == "number" {
					numberStr = ti.Value
				}
			}
		}
	}
	if wagerStr == "" {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "No wager provided.", 0xFF0000), nil, true)
		return
	}
	user, err := utils.GetCachedUser(userID)
	if err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "Failed to load user.", 0xFF0000), nil, true)
		return
	}
	// Remaining chips after existing bets
	remaining := user.Chips
	for _, v := range game.Bets {
		remaining -= v
	}
	betAmount, err := utils.ParseBet(wagerStr, user.Chips)
	if err != nil || betAmount <= 0 {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "Invalid bet amount.", 0xFF0000), nil, true)
		return
	}
	if betAmount > remaining {
		utils.SendInteractionResponse(s, i, utils.InsufficientChipsEmbed(betAmount, user.Chips, "that wager"), nil, true)
		return
	}
	// If single bet, validate number and adjust betType
	if betType == "single" {
		if numberStr == "" {
			utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "No number provided for single bet.", 0xFF0000), nil, true)
			return
		}
		n, err := strconv.Atoi(strings.TrimSpace(numberStr))
		if err != nil || n < 0 || n > 36 {
			utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "Invalid number (0-36).", 0xFF0000), nil, true)
			return
		}
		betType = "single_" + strconv.Itoa(n)
	}
	// Accumulate if user places same bet multiple times
	game.Bets[betType] += betAmount
	utils.UpdateComponentInteraction(s, i, utils.RouletteGameEmbed("betting", game.Bets, 0, "", 0, 0, 0), game.buildComponents())
}
func (rg *RouletteGame) calculateProfit() int64 {
	num := rg.ResultNumber
	profit := int64(0)
	if num == 0 {
		for k, v := range rg.Bets {
			if isEvenMoney(k) {
				profit -= v / 2
			} else {
				profit -= v
			}
		}
		return profit
	}
	for k, v := range rg.Bets {
		win := false
		mult := int64(1)
		switch {
		case k == "red" && rg.ResultColor == "red":
			win = true
		case k == "black" && rg.ResultColor == "black":
			win = true
		case k == "odd" && num%2 == 1:
			win = true
		case k == "even" && num%2 == 0:
			win = true
		case k == "1-18" && num >= 1 && num <= 18:
			win = true
		case k == "19-36" && num >= 19 && num <= 36:
			win = true
		case k == "dozen1" && num >= 1 && num <= 12:
			win = true
		case k == "dozen2" && num >= 13 && num <= 24:
			win = true
		case k == "dozen3" && num >= 25 && num <= 36:
			win = true
		case k == "col1" && num%3 == 1:
			win = true
		case k == "col2" && num%3 == 2:
			win = true
		case k == "col3" && num%3 == 0:
			win = true
		case strings.HasPrefix(k, "single_"):
			parts := strings.Split(k, "_")
			if len(parts) == 2 {
				if n, err := strconv.Atoi(parts[1]); err == nil && n == num {
					win = true
					mult = 35
				}
			}
		}
		if win {
			profit += v * mult
		} else {
			profit -= v
		}
	}
	return profit
}

func isEvenMoney(k string) bool {
	return k == "red" || k == "black" || k == "odd" || k == "even" || k == "1-18" || k == "19-36"
}
