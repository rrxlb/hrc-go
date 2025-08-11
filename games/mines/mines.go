package mines

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
)

// PAYOUT_MULTIPLIERS baseline multipliers per mine count
var payoutMultipliers = map[int]float64{
	1: 0.03, 2: 0.07, 3: 0.11, 4: 0.15, 5: 0.20, 6: 0.25, 7: 0.30, 8: 0.36,
	9: 0.43, 10: 0.50, 11: 0.58, 12: 0.67, 13: 0.77, 14: 0.88, 15: 1.00,
	16: 1.14, 17: 1.31, 18: 1.50, 19: 1.73, 20: 2.00, 21: 2.33, 22: 2.75,
	23: 3.33, 24: 4.00,
}

// Tile represents a single cell in the grid
type Tile struct {
	Row        int
	Col        int
	IsMine     bool
	IsRevealed bool
}

// Game represents a Mines game instance
type Game struct {
	UserID    int64
	ChannelID string
	MessageID string
	Bet       int64
	MineCount int
	Grid      [][]*Tile // 4x5
	Revealed  int
	CreatedAt time.Time
	IsOver    bool
	mu        sync.RWMutex
}

var active = struct {
	sync.RWMutex
	byUser map[int64]*Game
}{byUser: make(map[int64]*Game)}

// RegisterMinesCommand registers the /mines command
func RegisterMinesCommand() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "mines",
		Description: "Play Mines and uncover gems for cash.",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "bet",
				Description: "Bet amount (e.g., 500, 10k, half, all)",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "mines",
				Description: "Number of mines (1-19)",
				Required:    true,
			},
		},
	}
}

// HandleMinesCommand handles the /mines slash command
func HandleMinesCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Prevent multiple games per user
	uid, _ := utils.ParseUserID(i.Member.User.ID)
	active.RLock()
	if _, exists := active.byUser[uid]; exists {
		active.RUnlock()
		_ = utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "You already have an active game.", 0xE74C3C), nil, true)
		return
	}
	active.RUnlock()

	if err := utils.DeferInteractionResponse(s, i, false); err != nil {
		return
	}

	// Parse inputs
	var betStr string
	var minesCount int
	for _, opt := range i.ApplicationCommandData().Options {
		switch opt.Name {
		case "bet":
			betStr = strings.TrimSpace(opt.StringValue())
		case "mines":
			minesCount = int(opt.IntValue())
		}
	}
	if minesCount < 1 || minesCount > 19 {
		_ = utils.EditOriginalInteraction(s, i, utils.CreateBrandedEmbed("Mines", "Mines count must be between 1 and 19.", 0xE74C3C), nil)
		return
	}

	user, err := utils.GetCachedUser(uid)
	if err != nil {
		_ = utils.EditOriginalInteraction(s, i, utils.CreateBrandedEmbed("Mines", "Failed to load user.", 0xE74C3C), nil)
		return
	}
	betAmt, err := utils.ParseBet(betStr, user.Chips)
	if err != nil || betAmt <= 0 {
		_ = utils.EditOriginalInteraction(s, i, utils.CreateBrandedEmbed("Mines", "Invalid bet.", 0xE74C3C), nil)
		return
	}
	if user.Chips < betAmt {
		_ = utils.EditOriginalInteraction(s, i, utils.InsufficientChipsEmbed(betAmt, user.Chips, "this bet"), nil)
		return
	}

	// Debit bet upfront (Python does this before EndGame profits)
	if _, err := utils.UpdateCachedUser(uid, utils.UserUpdateData{ChipsIncrement: -betAmt}); err != nil {
		_ = utils.EditOriginalInteraction(s, i, utils.CreateBrandedEmbed("Mines", "Could not place your bet.", 0xE74C3C), nil)
		return
	}

	// Create game and grid
	g := &Game{UserID: uid, ChannelID: i.ChannelID, Bet: betAmt, MineCount: minesCount, CreatedAt: time.Now()}
	g.Grid = make([][]*Tile, 4)
	for r := 0; r < 4; r++ {
		g.Grid[r] = make([]*Tile, 5)
		for c := 0; c < 5; c++ {
			g.Grid[r][c] = &Tile{Row: r, Col: c}
		}
	}
	placeMines(g)

	active.Lock()
	active.byUser[uid] = g
	active.Unlock()

	// Initial embed and view
	embed := createMinesEmbed(g, "playing", "", 0, 0, 0)
	comps := buildComponents(g)
	_ = utils.EditOriginalInteraction(s, i, embed, comps)

	// Capture message ID
	if resp, err := s.InteractionResponse(i.Interaction); err == nil && resp != nil {
		g.mu.Lock()
		g.MessageID = resp.ID
		g.mu.Unlock()
	}
}

// placeMines randomly marks tiles as mines
func placeMines(g *Game) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	placed := 0
	for placed < g.MineCount {
		r := rng.Intn(4)
		c := rng.Intn(5)
		if !g.Grid[r][c].IsMine {
			g.Grid[r][c].IsMine = true
			placed++
		}
	}
}

// currentMultiplier returns 1.0 + baseMultiplier*revealed
func (g *Game) currentMultiplier() float64 {
	base := payoutMultipliers[g.MineCount]
	if base <= 0 {
		base = 0.01
	}
	if g.Revealed == 0 {
		return 1.0
	}
	return round2(1.0 + base*float64(g.Revealed))
}

// currentWinnings computes bet * multiplier (integer chips)
func (g *Game) currentWinnings() int64 {
	if g.Revealed == 0 {
		return 0
	}
	return int64(float64(g.Bet) * g.currentMultiplier())
}

// HandleMinesButton routes tile/cashout button presses
func HandleMinesButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cid := i.MessageComponentData().CustomID
	if !strings.HasPrefix(cid, "mines_") {
		return
	}
	uid, _ := utils.ParseUserID(i.Member.User.ID)

	active.RLock()
	g := active.byUser[uid]
	active.RUnlock()
	if g == nil {
		_ = utils.TryEphemeralFollowup(s, i, "No active Mines game.")
		return
	}
	// Authorize
	if i.Member.User.ID != strconv.FormatInt(g.UserID, 10) {
		_ = utils.TryEphemeralFollowup(s, i, "This isn't your game.")
		return
	}

	if cid == "mines_cashout" {
		handleCashout(s, i, g)
		return
	}
	// mines_tile_r_c
	parts := strings.Split(cid, "_")
	if len(parts) == 4 && parts[1] == "tile" {
		r, _ := strconv.Atoi(parts[2])
		c, _ := strconv.Atoi(parts[3])
		handleReveal(s, i, g, r, c)
		return
	}
}

func handleReveal(s *discordgo.Session, i *discordgo.InteractionCreate, g *Game, row, col int) {
	g.mu.Lock()
	if g.IsOver {
		g.mu.Unlock()
		return
	}
	tile := g.Grid[row][col]
	if tile.IsRevealed {
		g.mu.Unlock()
		_ = utils.AcknowledgeComponentInteraction(s, i)
		return
	}
	tile.IsRevealed = true

	lost := false
	if tile.IsMine {
		lost = true
		g.IsOver = true
	} else {
		g.Revealed++
		// Win condition: all gems revealed
		if g.Revealed >= (20 - g.MineCount) {
			g.IsOver = true
		}
	}
	g.mu.Unlock()

	if g.IsOver {
		// Compute profit: lost => -bet; won => winnings - bet
		profit := int64(0)
		reason := ""
		if lost {
			profit = -g.Bet
			reason = "You hit a mine!"
		} else {
			profit = g.currentWinnings() - g.Bet
			reason = "You found all the gems!"
		}
		// Apply profit and XP
		xp := int64(0)
		if profit > 0 {
			xp = profit * utils.XPPerProfit
		}
		userAfter, _ := utils.UpdateCachedUser(g.UserID, utils.UserUpdateData{ChipsIncrement: profit, TotalXPIncrement: xp, CurrentXPIncrement: xp})
		newBal := int64(0)
		if userAfter != nil {
			newBal = userAfter.Chips
		}

		// Disable all buttons
		comps := disableAllComponents(buildComponents(g))
		// Build final embed
		embed := createMinesEmbed(g, "final", reason, profit, xp, newBal)
		_ = utils.UpdateComponentInteraction(s, i, embed, comps)

		// Clear active map
		active.Lock()
		delete(active.byUser, g.UserID)
		active.Unlock()
		return
	}

	// Update ongoing view
	embed := createMinesEmbed(g, "playing", "", 0, 0, 0)
	comps := buildComponents(g)
	_ = utils.UpdateComponentInteraction(s, i, embed, comps)
}

func handleCashout(s *discordgo.Session, i *discordgo.InteractionCreate, g *Game) {
	g.mu.Lock()
	if g.IsOver {
		g.mu.Unlock()
		return
	}
	g.IsOver = true
	g.mu.Unlock()

	winnings := g.currentWinnings()
	profit := winnings - g.Bet
	reason := "You cashed out."
	xp := int64(0)
	if profit > 0 {
		xp = profit * utils.XPPerProfit
	}
	userAfter, _ := utils.UpdateCachedUser(g.UserID, utils.UserUpdateData{ChipsIncrement: profit, TotalXPIncrement: xp, CurrentXPIncrement: xp})
	newBal := int64(0)
	if userAfter != nil {
		newBal = userAfter.Chips
	}

	comps := disableAllComponents(buildComponents(g))
	embed := createMinesEmbed(g, "final", reason, profit, xp, newBal)
	_ = utils.UpdateComponentInteraction(s, i, embed, comps)

	active.Lock()
	delete(active.byUser, g.UserID)
	active.Unlock()
}

// buildComponents constructs the 4x5 grid and cashout button
func buildComponents(g *Game) []discordgo.MessageComponent {
	rows := []discordgo.MessageComponent{}
	for r := 0; r < 4; r++ {
		btns := []discordgo.MessageComponent{}
		for c := 0; c < 5; c++ {
			t := g.Grid[r][c]
			label := "⬛"
			disabled := false
			if t.IsRevealed {
				disabled = true
				if t.IsMine {
					label = "💣"
				} else {
					label = "💎"
				}
			}
			btns = append(btns, discordgo.Button{CustomID: fmt.Sprintf("mines_tile_%d_%d", r, c), Label: label, Style: discordgo.SecondaryButton, Disabled: disabled})
		}
		rows = append(rows, discordgo.ActionsRow{Components: btns})
	}
	// Cash out row
	cashDisabled := g.Revealed == 0 || g.IsOver
	rows = append(rows, discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{CustomID: "mines_cashout", Label: "Cash Out", Style: discordgo.SuccessButton, Disabled: cashDisabled},
	}})
	return rows
}

func disableAllComponents(comps []discordgo.MessageComponent) []discordgo.MessageComponent {
	out := make([]discordgo.MessageComponent, 0, len(comps))
	for _, row := range comps {
		if ar, ok := row.(discordgo.ActionsRow); ok {
			btns := []discordgo.MessageComponent{}
			for _, c := range ar.Components {
				switch b := c.(type) {
				case discordgo.Button:
					b.Disabled = true
					btns = append(btns, b)
				default:
					btns = append(btns, c)
				}
			}
			out = append(out, discordgo.ActionsRow{Components: btns})
		} else {
			out = append(out, row)
		}
	}
	return out
}

// createMinesEmbed mirrors the Python embed states
func createMinesEmbed(g *Game, state, outcome string, profit, xp int64, newBalance int64) *discordgo.MessageEmbed {
	title := "Mines"
	desc := ""
	color := utils.BotColor
	if state == "final" {
		if profit > 0 {
			color = 0x2ECC71
		} else if profit < 0 {
			color = 0xE74C3C
		} else {
			color = 0xF39C12
		}
	} else {
		color = 0x3498DB
	}
	embed := utils.CreateBrandedEmbed(title, desc, color)
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1753328701/MN_vouwpt.jpg"}

	// Game info
	embed.Fields = []*discordgo.MessageEmbedField{
		{Name: "Initial Bet", Value: fmt.Sprintf("%s %s", utils.FormatChips(g.Bet), utils.ChipsEmoji), Inline: true},
		{Name: "Mines", Value: fmt.Sprintf("%d 💣", g.MineCount), Inline: true},
	}

	if state == "playing" {
		// Show live game info
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Revealed Gems", Value: fmt.Sprintf("%d", g.Revealed), Inline: true})
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Multiplier", Value: fmt.Sprintf("x%.2f", g.currentMultiplier()), Inline: true})
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Current Winnings", Value: fmt.Sprintf("%s %s", utils.FormatChips(g.currentWinnings()), utils.ChipsEmoji), Inline: false})
		embed.Footer = &discordgo.MessageEmbedFooter{Text: "Tap tiles to reveal. Cash out anytime."}
	} else if state == "final" {
		// Outcome
		result := ""
		if outcome != "" {
			result = outcome
		}
		if profit > 0 {
			result += fmt.Sprintf("\nYou won **%s** %s", utils.FormatChips(profit), utils.ChipsEmoji)
		} else if profit < 0 {
			result += fmt.Sprintf("\nYou lost **%s** %s", utils.FormatChips(-profit), utils.ChipsEmoji)
		} else {
			result += "\nRound ended."
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Outcome", Value: result, Inline: false})
		if newBalance != 0 {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "New Balance", Value: fmt.Sprintf("%s %s", utils.FormatChips(newBalance), utils.ChipsEmoji), Inline: true})
		}
		if xp > 0 {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "XP Gained", Value: fmt.Sprintf("+%s XP", utils.FormatChips(xp)), Inline: true})
		}
	}
	return embed
}

// Cleanup goroutine (run periodically from main registration once)
var cleanupOnce sync.Once

func StartCleanupLoop(s *discordgo.Session) {
	cleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				now := time.Now()
				active.Lock()
				for uid, g := range active.byUser {
					if now.Sub(g.CreatedAt) > 10*time.Minute {
						// Timeout: forfeit bet
						delete(active.byUser, uid)
						// Build cleanup embed
						embed := utils.GameCleanupEmbed(g.Bet)
						// Disable components and update message
						if g.MessageID != "" {
							comps := []discordgo.MessageComponent{}
							embeds := []*discordgo.MessageEmbed{embed}
							_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{Channel: g.ChannelID, ID: g.MessageID, Embeds: &embeds, Components: &comps})
						}
					}
				}
				active.Unlock()
			}
		}()
	})
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

// toPtr is a tiny helper for pointer fields in command options
func toPtr[T any](v T) *T { return &v }
