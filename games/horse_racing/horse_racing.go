package horse_racing

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
)

// Constants and assets
var (
	horseNames = []string{
		"Seabiscuit", "Glue Factory", "Hoof Hearted", "Maythehorsebewithu", "Galloping Ghost",
		"Usain Colt", "Pony Soprano", "Forrest Jump", "Lil' Sebastian", "DoYouThinkHeSaurus",
		"Bet A Million", "Always Broke", "My Wife's Money", "Sofa King Fast", "Harry Trotter",
		"Blazing Saddles", "Debt Collector", "Spinning Plates", "Pixelated Steed", "Discord Nitro",
	}
	horseEmojis = []string{"🐴", "🐎", "🦄", "🏇"}
	trackLength = 20
)

var commentary = map[string][]string{
	"start": {
		"And they're off!", "A clean start for all the horses!", "The gates are open and the race has begun!",
	},
	"middle": {
		"A blistering pace is being set by the leaders!", "Down the backstretch they come!",
		"Rounding the first turn, it's still anyone's race!", "A horse is making a move on the outside!",
	},
	"end": {
		"Into the final stretch, the crowd is roaring!", "It's neck and neck as they approach the finish!",
		"One horse is pulling ahead with a burst of speed!", "This is going to be a close one!",
	},
}

type Horse struct {
	ID       int
	Name     string
	Odds     int // payout multiplier (x:1)
	Position int
	Icon     string
}

type Bet struct {
	UserID   int64
	UserName string
	HorseID  int
	Amount   int64
}

type RaceStatus string

const (
	StatusLobby     RaceStatus = "lobby"
	StatusBetting   RaceStatus = "betting"
	StatusRunning   RaceStatus = "running"
	StatusFinished  RaceStatus = "finished"
	StatusCancelled RaceStatus = "cancelled"
)

type Race struct {
	ChannelID     string
	MessageID     string
	Initiator     int64
	InitiatorName string
	Horses        []*Horse
	Bets          []Bet
	Participants  map[int64]string // userID -> display name
	Status        RaceStatus
	CreatedAt     time.Time
	mu            sync.RWMutex
}

var races = struct {
	sync.RWMutex
	byChannel map[string]*Race
}{byChannel: make(map[string]*Race)}

// Command registration
func RegisterHorseRacingCommand() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "derby",
		Description: "Start a horse race lobby in this channel.",
	}
}

// Handle slash command
func HandleHorseRacingCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := utils.DeferInteractionResponse(s, i, false); err != nil {
		return
	}

	chID := i.ChannelID
	userID, _ := utils.ParseUserID(i.Member.User.ID)

	races.Lock()
	if _, exists := races.byChannel[chID]; exists {
		races.Unlock()
		_ = utils.EditOriginalInteraction(s, i, utils.CreateBrandedEmbed("🏇 Derby", "There is already an active race in this channel.", 0xE74C3C), nil)
		return
	}
	race := &Race{ChannelID: chID, Initiator: userID, InitiatorName: i.Member.User.Username, Participants: map[int64]string{userID: i.Member.User.Mention()}, Status: StatusLobby, CreatedAt: time.Now()}
	race.Horses = pickHorses(6)
	races.byChannel[chID] = race
	races.Unlock()

	embed := lobbyEmbed(race)
	components := lobbyComponents(len(race.Participants) >= 1)
	_ = utils.EditOriginalInteraction(s, i, embed, components)
	// capture message id
	if orig, err := s.InteractionResponse(i.Interaction); err == nil && orig != nil {
		race.mu.Lock()
		race.MessageID = orig.ID
		race.mu.Unlock()
	}

	// Cleanup timer (5 minutes)
	go scheduleCleanup(s, chID, StatusLobby)
}

func pickHorses(n int) []*Horse {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	idx := r.Perm(len(horseNames))[:n]
	horses := make([]*Horse, 0, n)
	for i, j := range idx {
		name := horseNames[j]
		icon := horseEmojis[r.Intn(len(horseEmojis))]
		// Assign odds between 2 and 25 to match Python
		odds := 2 + r.Intn(24)
		horses = append(horses, &Horse{ID: i + 1, Name: name, Icon: icon, Odds: odds})
	}
	return horses
}

func lobbyEmbed(r *Race) *discordgo.MessageEmbed {
	desc := "**The lobby is open! Click 'Join Race' to enter!**\n\n"
	desc += "**Horses & Odds:**\n"
	for _, h := range r.Horses {
		desc += fmt.Sprintf("`%d.` %s **%s** `(%d:1)`\n", h.ID, h.Icon, h.Name, h.Odds)
	}
	desc += "\n**Participants:**\n"
	if len(r.Participants) == 0 {
		desc += "No one has joined yet."
	} else {
		names := make([]string, 0, len(r.Participants))
		for _, n := range r.Participants {
			names = append(names, n)
		}
		desc += strings.Join(names, ", ")
	}
	embed := utils.CreateBrandedEmbed(fmt.Sprintf("🏇 %s's Horse Race 🏇", r.InitiatorName), desc, utils.BotColor)
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1754026209/HR2_dacwe3.png"}
	embed.Footer = &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Race initiated by %s", r.InitiatorName)}
	return embed
}

func bettingEmbed(r *Race) *discordgo.MessageEmbed {
	desc := "**Place your bets now!**\n\n**Horses & Odds:**\n"
	for _, h := range r.Horses {
		desc += fmt.Sprintf("`%d.` %s **%s** `(%d:1)`\n", h.ID, h.Icon, h.Name, h.Odds)
	}
	desc += "\n**Bets Placed:**\n"
	if len(r.Bets) == 0 {
		desc += "No bets placed yet."
	} else {
		for _, b := range r.Bets {
			desc += fmt.Sprintf("• **%s** on Horse #%d\n", b.UserName, b.HorseID)
		}
	}
	embed := utils.CreateBrandedEmbed("🏇 Betting is Open! 🏇", desc, 0x3498db)
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1754026209/HR2_dacwe3.png"}
	embed.Footer = &discordgo.MessageEmbedFooter{Text: "The initiator can lock bets and start the race at any time."}
	return embed
}

func lobbyComponents(enableStart bool) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			utils.CreateButton("derby_join", "Join Race", discordgo.PrimaryButton, false, nil),
			utils.CreateButton("derby_start_betting", "Start Betting", discordgo.SuccessButton, !enableStart, nil),
			utils.CreateButton("derby_cancel", "Cancel Race", discordgo.DangerButton, false, nil),
		}},
	}
}

func bettingComponents(disableStart bool) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			utils.CreateButton("derby_place_bet", "Place Bet", discordgo.SuccessButton, false, nil),
			utils.CreateButton("derby_lock_start", "Lock Bets & Start Race", discordgo.PrimaryButton, disableStart, nil),
		}},
	}
}

// Button router
func HandleHorseRacingInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cid := i.MessageComponentData().CustomID
	chID := i.ChannelID
	races.RLock()
	race := races.byChannel[chID]
	races.RUnlock()
	if race == nil {
		_ = utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Derby", "No active race here.", 0xE74C3C), nil, true)
		return
	}

	switch cid {
	case "derby_join":
		userID, _ := utils.ParseUserID(i.Member.User.ID)
		race.mu.Lock()
		if race.Status != StatusLobby {
			race.mu.Unlock()
			_ = utils.TryEphemeralFollowup(s, i, "Lobby is closed.")
			return
		}
		if _, exists := race.Participants[userID]; exists {
			race.mu.Unlock()
			_ = utils.TryEphemeralFollowup(s, i, "You have already joined the race.")
			return
		}
		race.Participants[userID] = i.Member.User.Mention()
		race.mu.Unlock()
		_ = utils.UpdateComponentInteraction(s, i, lobbyEmbed(race), lobbyComponents(true))
	case "derby_start_betting":
		uid, _ := utils.ParseUserID(i.Member.User.ID)
		race.mu.Lock()
		if uid != race.Initiator {
			race.mu.Unlock()
			_ = utils.TryEphemeralFollowup(s, i, "Only the initiator can start betting.")
			return
		}
		race.Status = StatusBetting
		race.mu.Unlock()
		// Disable start until at least one bet exists
		_ = utils.UpdateComponentInteraction(s, i, bettingEmbed(race), bettingComponents(len(race.Bets) == 0))
		// Start betting phase cleanup timer
		go scheduleCleanup(s, chID, StatusBetting)
	case "derby_cancel":
		uid, _ := utils.ParseUserID(i.Member.User.ID)
		race.mu.Lock()
		if uid != race.Initiator {
			race.mu.Unlock()
			_ = utils.TryEphemeralFollowup(s, i, "Only the initiator can cancel.")
			return
		}
		race.Status = StatusCancelled
		race.mu.Unlock()
		_ = utils.UpdateComponentInteraction(s, i, utils.CreateBrandedEmbed("🏇 Race Cancelled 🏇", "The race was cancelled by the initiator.", 0xE74C3C), []discordgo.MessageComponent{})
		races.Lock()
		delete(races.byChannel, chID)
		races.Unlock()
	case "derby_place_bet":
		// open modal for bet
		modal := &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: &discordgo.InteractionResponseData{
			CustomID: "derby_bet_modal_" + chID,
			Title:    "Place Your Bet",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					&discordgo.TextInput{CustomID: "horse_number", Label: "Horse Number (1-6)", Style: discordgo.TextInputShort, Required: true, MinLength: 1, MaxLength: 2, Placeholder: "1-6"},
				}},
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					&discordgo.TextInput{CustomID: "bet_amount", Label: "Bet Amount", Style: discordgo.TextInputShort, Required: true, MinLength: 1, MaxLength: 10, Placeholder: "e.g., 500"},
				}},
			},
		}}
		_ = s.InteractionRespond(i.Interaction, modal)
	case "derby_lock_start":
		uid, _ := utils.ParseUserID(i.Member.User.ID)
		race.mu.Lock()
		if uid != race.Initiator {
			race.mu.Unlock()
			_ = utils.TryEphemeralFollowup(s, i, "Only the initiator can start the race.")
			return
		}
		if len(race.Bets) == 0 {
			race.mu.Unlock()
			_ = utils.TryEphemeralFollowup(s, i, "You can't start the race until at least one bet is placed.")
			return
		}
		race.Status = StatusRunning
		race.mu.Unlock()
		_ = utils.UpdateComponentInteraction(s, i, utils.CreateBrandedEmbed("🏇 Bets are Locked! 🏇", "The bets are in! The race is about to begin...", 0x8E44AD), []discordgo.MessageComponent{})
		go runRace(s, race)
	}
}

// Modal handler
func HandleHorseRacingModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !strings.HasPrefix(i.ModalSubmitData().CustomID, "derby_bet_modal_") {
		return
	}
	chID := strings.TrimPrefix(i.ModalSubmitData().CustomID, "derby_bet_modal_")
	races.RLock()
	race := races.byChannel[chID]
	races.RUnlock()
	if race == nil {
		_ = utils.TryEphemeralFollowup(s, i, "No active race.")
		return
	}
	// parse inputs
	var horseNumStr, betAmtStr string
	for _, row := range i.ModalSubmitData().Components {
		if ar, ok := row.(*discordgo.ActionsRow); ok {
			for _, c := range ar.Components {
				if ti, ok := c.(*discordgo.TextInput); ok {
					if ti.CustomID == "horse_number" {
						horseNumStr = ti.Value
					}
					if ti.CustomID == "bet_amount" {
						betAmtStr = ti.Value
					}
				}
			}
		}
	}
	// ensure betting phase
	race.mu.RLock()
	status := race.Status
	participants := make(map[int64]struct{}, len(race.Participants))
	for uid := range race.Participants {
		participants[uid] = struct{}{}
	}
	race.mu.RUnlock()
	if status != StatusBetting {
		_ = utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Betting Closed", "You can no longer place bets.", 0xE74C3C), nil, true)
		return
	}
	horseNum, _ := strconv.Atoi(strings.TrimSpace(horseNumStr))
	if horseNum < 1 || horseNum > len(race.Horses) {
		_ = utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Bet Error", "Invalid horse or amount.", 0xE74C3C), nil, true)
		return
	}
	userID, _ := utils.ParseUserID(i.Member.User.ID)
	// must be a participant
	if _, ok := participants[userID]; !ok {
		_ = utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Bet Error", "You must join the race to place a bet.", 0xE74C3C), nil, true)
		return
	}
	user, err := utils.GetCachedUser(userID)
	if err != nil {
		_ = utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "Failed to load user.", 0xE74C3C), nil, true)
		return
	}
	// parse amount using standard bet parser (supports k/m/all/half)
	betAmt, perr := utils.ParseBet(strings.TrimSpace(betAmtStr), user.Chips)
	if perr != nil || betAmt <= 0 {
		_ = utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Bet Error", "Invalid bet amount.", 0xE74C3C), nil, true)
		return
	}
	if user.Chips < betAmt {
		_ = utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Not Enough Chips", "You don't have enough chips for that bet.", 0xE74C3C), nil, true)
		return
	}
	// prevent duplicate bets by same user
	race.mu.RLock()
	alreadyBet := false
	for _, b := range race.Bets {
		if b.UserID == userID {
			alreadyBet = true
			break
		}
	}
	race.mu.RUnlock()
	if alreadyBet {
		_ = utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Bet Error", "You have already placed a bet in this race.", 0xE74C3C), nil, true)
		return
	}
	// debit
	if _, err := utils.UpdateUser(userID, utils.UserUpdateData{ChipsIncrement: -betAmt}); err != nil {
		_ = utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "Failed to place bet.", 0xE74C3C), nil, true)
		return
	}
	// record bet
	race.mu.Lock()
	race.Bets = append(race.Bets, Bet{UserID: userID, UserName: i.Member.User.Username, HorseID: horseNum, Amount: betAmt})
	race.mu.Unlock()
	_ = utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Bet Placed!", fmt.Sprintf("You bet %s on Horse #%d.", utils.FormatChips(betAmt), horseNum), 0x2ECC71), nil, true)
	// update message to show bets
	if race.MessageID != "" {
		embeds := []*discordgo.MessageEmbed{bettingEmbed(race)}
		comps := []discordgo.MessageComponent{bettingComponents(len(race.Bets) == 0)[0]}
		_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{Channel: race.ChannelID, ID: race.MessageID, Embeds: &embeds, Components: &comps})
	}
}

func runRace(s *discordgo.Session, r *Race) {
	// simulation
	winnerFound := false
	step := 0
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var winner *Horse
	phase := "start"
	// Preselect commentary strings similar to Python
	startText := commentary["start"][rng.Intn(len(commentary["start"]))]
	middleText := commentary["middle"][rng.Intn(len(commentary["middle"]))]
	endText := commentary["end"][rng.Intn(len(commentary["end"]))]
	for !winnerFound && step < 100 {
		r.mu.Lock()
		// phase selection similar to Python: start -> middle -> end when final stretch
		if phase == "start" {
			// stay start only for first tick
			phase = "middle"
		} else {
			// switch to end if any horse reaches final stretch (>= 75%)
			for _, h := range r.Horses {
				if float64(h.Position)/float64(trackLength) >= 0.75 {
					phase = "end"
					break
				}
			}
		}
		var text string
		switch phase {
		case "start":
			text = startText
		case "end":
			text = endText
		default:
			text = middleText
		}
		// advance horses using odds-influenced movement; ensure visible progress
		movedAny := false
		for _, h := range r.Horses {
			if h.Position >= trackLength-1 {
				continue
			}
			moveChance := (1.0 / float64(h.Odds)) * 0.5
			p := 0.1 + moveChance
			// Ensure a reasonable floor so movement is visible even for long odds
			if p < 0.35 {
				p = 0.35
			}
			baseMove := 0
			if rng.Float64() < p {
				baseMove = 1
			}
			bonusMove := 0
			if baseMove > 0 {
				bonusMove = rng.Intn(3) // 0-2
			}
			h.Position += baseMove + bonusMove
			if h.Position > trackLength-1 {
				h.Position = trackLength - 1
			}
			if h.Position >= trackLength-1 && winner == nil {
				winner = h
			}
			if baseMove > 0 || bonusMove > 0 {
				movedAny = true
			}
		}
		// If no horse moved this tick, randomly nudge one forward to keep the race visually active
		if !movedAny {
			idx := rng.Intn(len(r.Horses))
			if r.Horses[idx].Position < trackLength-1 {
				r.Horses[idx].Position++
			}
		}
		// build embed
		desc := fmt.Sprintf("**%s**\n\n%s", text, trackDisplay(r.Horses))
		title := "🏇 The Race is On! 🏇"
		color := utils.BotColor
		if winner != nil {
			title = "🏁 A Winner is Decided! 🏁"
			color = 0xF1C40F
			desc = fmt.Sprintf("**%s crosses the finish line first!**\n\n%s", winner.Name, trackDisplay(r.Horses))
		}
		embed := utils.CreateBrandedEmbed(title, desc, color)
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1754026209/HR2_dacwe3.png"}
		r.mu.Unlock()
		if r.MessageID != "" {
			embeds := []*discordgo.MessageEmbed{embed}
			_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{Channel: r.ChannelID, ID: r.MessageID, Embeds: &embeds})
		}
		if winner != nil {
			winnerFound = true
			break
		}
		step++
		time.Sleep(800 * time.Millisecond)
	}
	// finalize and payout
	if winner == nil { // fallback by position
		r.mu.RLock()
		hs := append([]*Horse(nil), r.Horses...)
		r.mu.RUnlock()
		sort.Slice(hs, func(i, j int) bool { return hs[i].Position > hs[j].Position })
		winner = hs[0]
	}
	payoutWinners(s, r, winner)
}

func trackDisplay(horses []*Horse) string {
	finish := "🏁"
	rows := make([]string, 0, len(horses))
	for _, h := range horses {
		pos := h.Position
		if pos < 0 {
			pos = 0
		}
		if pos > trackLength-1 {
			pos = trackLength - 1
		}
		// ASCII-only inside the code block for consistent alignment; emoji outside
		progress := strings.Repeat("=", pos)
		remain := strings.Repeat("-", trackLength-1-pos)
		rows = append(rows, fmt.Sprintf("`%2d.` %s `[%s>%s]` %s", h.ID, h.Icon, progress, remain, finish))
	}
	return strings.Join(rows, "\n")
}

func payoutWinners(s *discordgo.Session, r *Race, winner *Horse) {
	r.mu.RLock()
	horses := append([]*Horse(nil), r.Horses...)
	bets := append([]Bet(nil), r.Bets...)
	chID := r.ChannelID
	msgID := r.MessageID
	r.mu.RUnlock()

	sort.Slice(horses, func(i, j int) bool { return horses[i].Position > horses[j].Position })

	placements := map[int]string{1: "🥇", 2: "🥈", 3: "🥉"}
	results := "### 🏁 Race Results 🏁\n"
	results += fmt.Sprintf("**Winner:** %s **%s**!\n\n", winner.Icon, winner.Name)
	results += "**Final Placements:**\n"
	for i, h := range horses[:min(3, len(horses))] {
		results += fmt.Sprintf("%s **%s** (Horse #%d)\n", placements[i+1], h.Name, h.ID)
	}

	// compute payouts
	type winEntry struct {
		Payout int64
		Profit int64
		UserID int64
		Name   string
	}
	wins := []winEntry{}
	totalPaid := int64(0)
	uniqueBettors := map[int64]struct{}{}
	winnerIDs := map[int64]struct{}{}
	for _, b := range bets {
		uniqueBettors[b.UserID] = struct{}{}
		if b.HorseID == winner.ID {
			// Python credits bet*odds (stake was already debited), so payout equals winnings
			winnings := b.Amount * int64(winner.Odds)
			wins = append(wins, winEntry{Payout: winnings, Profit: winnings, UserID: b.UserID, Name: b.UserName})
			totalPaid += winnings
			winnerIDs[b.UserID] = struct{}{}
		}
	}

	// DB updates: winners get chips + XP and a win; losers get a loss
	for _, w := range wins {
		_, _ = utils.UpdateUser(w.UserID, utils.UserUpdateData{ChipsIncrement: w.Payout, TotalXPIncrement: w.Profit * utils.XPPerProfit, CurrentXPIncrement: w.Profit * utils.XPPerProfit, WinsIncrement: 1})
	}
	for uid := range uniqueBettors {
		if _, ok := winnerIDs[uid]; !ok {
			_, _ = utils.UpdateUser(uid, utils.UserUpdateData{LossesIncrement: 1})
		}
	}

	if len(wins) > 0 {
		results += "\n**🏆 Top Winners:**\n"
		sort.Slice(wins, func(i, j int) bool { return wins[i].Payout > wins[j].Payout })
		for i, w := range wins[:min(5, len(wins))] {
			medal, ok := placements[i+1]
			if !ok {
				medal = "•"
			}
			results += fmt.Sprintf("%s **%s** won **%s** chips!\n", medal, w.Name, utils.FormatChips(w.Payout))
		}
	} else {
		results += "\nNo winners this time. The house keeps the chips!"
	}

	embed := utils.CreateBrandedEmbed(fmt.Sprintf("🏇 Race Finished: %s Wins! 🏇", winner.Name), results, 0xF1C40F)
	losers := len(uniqueBettors) - len(wins)
	embed.Footer = &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("%d winners, %d losers. Total paid out: %s chips.", len(wins), losers, utils.FormatChips(totalPaid))}

	if msgID != "" {
		embeds := []*discordgo.MessageEmbed{embed}
		comps := []discordgo.MessageComponent{}
		_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{Channel: chID, ID: msgID, Embeds: &embeds, Components: &comps})
	}

	races.Lock()
	delete(races.byChannel, chID)
	races.Unlock()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// scheduleCleanup waits for a period and if the race is still in the given phase, cleans up the message with a cleanup embed
func scheduleCleanup(s *discordgo.Session, channelID string, phase RaceStatus) {
	// Lobby view 5 minutes, betting view 5 minutes similar to Python views
	time.Sleep(5 * time.Minute)
	races.Lock()
	r := races.byChannel[channelID]
	if r == nil {
		races.Unlock()
		return
	}
	// Only act if still in the same phase and not finished
	if r.Status != phase {
		races.Unlock()
		return
	}
	// If we're in betting and there are bets, auto-start the race; otherwise cleanup
	if phase == StatusBetting && len(r.Bets) > 0 {
		r.Status = StatusRunning
		msgID := r.MessageID
		races.Unlock()
		// Update message to locked & start
		embed := utils.CreateBrandedEmbed("🏇 Bets are Locked! 🏇", "The bets are in! The race is about to begin...", 0x8E44AD)
		if msgID != "" {
			embeds := []*discordgo.MessageEmbed{embed}
			comps := []discordgo.MessageComponent{}
			_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{Channel: channelID, ID: msgID, Embeds: &embeds, Components: &comps})
		}
		// Run race
		go runRace(s, r)
		return
	}

	// Otherwise, cancel and cleanup
	r.Status = StatusCancelled
	delete(races.byChannel, channelID)
	msgID := r.MessageID
	races.Unlock()

	// Calculate total bet amount forfeited
	totalBet := int64(0)
	r.mu.RLock()
	for _, b := range r.Bets {
		totalBet += b.Amount
	}
	r.mu.RUnlock()

	// Build cleanup embed using utils
	cleanup := utils.GameCleanupEmbed(totalBet)
	if msgID != "" {
		embeds := []*discordgo.MessageEmbed{cleanup}
		// Clear components
		comps := []discordgo.MessageComponent{}
		_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{Channel: channelID, ID: msgID, Embeds: &embeds, Components: &comps})
	}
}
