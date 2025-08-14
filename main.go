package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	baccarat "hrc-go/games/baccarat"
	blackjack "hrc-go/games/blackjack"
	craps "hrc-go/games/craps"
	higherorlower "hrc-go/games/higher_or_lower"
	horseracing "hrc-go/games/horse_racing"
	mines "hrc-go/games/mines"
	roulette "hrc-go/games/roulette"
	slots "hrc-go/games/slots"
	threecardpoker "hrc-go/games/three_card_poker"
	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
)

var session *discordgo.Session
var botStatus = "starting"
var readyCh = make(chan struct{}, 1)

const devGuildID = "1262162191923023882" // fast registration guild
const (
	// Admin configuration (as requested)
	AdminLogChannelID = "1262162195429724183"
	AdminGuildID      = "1262162191923023882"
	AdminRoleID       = "1333188752054681632"
)

func main() {
	// Start HTTP server for health checks
	go startHealthServer()

	// Initialize database
	if err := utils.SetupDatabase(); err != nil {
		log.Printf("Database setup failed: %v", err)
		log.Println("Bot will continue without database features")
	} else {
		log.Println("Database connected successfully")
		// Ensure database cleanup on shutdown
		defer utils.CloseDatabase()
	}

	// Initialize cache system (5 minute TTL for better responsiveness)
	utils.InitializeCache(5 * time.Minute)
	defer utils.CloseCache()

	// Initialize centralized game state management
	utils.InitializeGameManager()
	defer utils.CloseGameManager()

	// Heavy subsystems deferred until after READY to reduce startup latency

	// Get bot token from environment (common variable names)
	var token string
	for _, key := range []string{"BOT_TOKEN", "DISCORD_TOKEN", "DISCORD_BOT_TOKEN", "HRC_BOT_TOKEN"} {
		if val := strings.TrimSpace(os.Getenv(key)); val != "" {
			token = val
			break
		}
	}

	// Sanitize token (remove quotes, leading Bot prefix, accidental export, etc.)
	token = sanitizeToken(token)
	if token == "" {
		log.Println("Bot token missing (BOT_TOKEN). Exiting idle.")
		botStatus = "no_token"
		select {}
	}

	// Basic structural validation (3 segments, first decodes to numeric ID)
	parts := strings.Split(token, ".")
	if len(parts) < 3 {
		botStatus = "invalid_token"
		select {}
	}
	if _, err := base64.RawStdEncoding.DecodeString(parts[0]); err == nil {
		// Expect numeric user ID
	}
	// Token validated and sanitized

	// Create Discord session
	var err error
	session, err = discordgo.New("Bot " + token)
	if err != nil {
		log.Printf("Failed to create Discord session: %v", err)
		botStatus = "error"
		select {}
	}

	// Set up intents (broader to ensure interactions / ready received)
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildMessageReactions
	// Optional: uncomment if you later need message content
	// session.Identify.Intents |= discordgo.IntentMessageContent

	// Add event handlers
	session.AddHandler(onReady)
	session.AddHandler(onInteractionCreate)
	session.AddHandler(onButtonInteraction)

	// Open Discord connection
	if err := session.Open(); err != nil {
		log.Printf("Failed to open Discord connection: %v", err)
		botStatus = "connection_failed"
		select {}
	}

	// Wait for READY or timeout to help diagnose hanging
	select {
	case <-readyCh:
		// READY received
	case <-time.After(30 * time.Second):
		// Continue without logging timeout
	}
	defer session.Close()

	botStatus = "running"

	// (Removed verbose heartbeat logging)

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-stop

	log.Println("Gracefully shutting down...")
	botStatus = "shutting_down"
}

// (Removed network preflight test to reduce noise)

func onReady(s *discordgo.Session, event *discordgo.Ready) {
	log.Printf("✅ Discord Bot logged in as %s (ID: %s)", event.User.Username, event.User.ID)
	botStatus = "online"
	select { // non-blocking send in case already signaled
	case readyCh <- struct{}{}:
	default:
	}

	// Set bot presence
	s.UpdateStatusComplex(discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{
			{
				Name: "/help for commands",
				Type: discordgo.ActivityTypeListening,
			},
		},
		Status: "online",
	})

	// Register commands first for fastest response
	if err := registerSlashCommands(s); err != nil {
		log.Printf("Failed to register slash commands: %v", err)
	}

	// Initialize heavy systems in background
	go func() {
		// Start background cleanup loops (mines)
		mines.StartCleanupLoop(s)

		// Initialize Top.gg client for voting
		utils.InitializeTopGGClient("1396564026233983108")

		// Initialize achievement system
		if err := utils.InitializeAchievementManager(); err != nil {
			log.Printf("Achievement manager init failed: %v", err)
		}

		// Initialize jackpot system
		if err := utils.InitializeJackpotManager(); err != nil {
			log.Printf("Jackpot manager init failed: %v", err)
		} else {
			log.Println("Jackpot system initialized")
		}
	}()
}

func registerSlashCommands(s *discordgo.Session) error {
	// Base commands (global)
	globalCommands := []*discordgo.ApplicationCommand{
		{
			Name:        "ping",
			Description: "Check bot latency and status",
		},
		{
			Name:        "info",
			Description: "Get information about the bot",
		},
		{
			Name:        "help",
			Description: "Shows a list of available commands",
		},
		{
			Name:        "profile",
			Description: "View your casino profile and stats",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "User to view (defaults to yourself)",
					Required:    false,
				},
			},
		},
		{
			Name:        "chips",
			Description: "Check your chips balance",
		},
		{
			Name:        "balance",
			Description: "Check your current chip balance",
		},
		{
			Name:        "hourly",
			Description: "Claim your hourly bonus",
		},
		{
			Name:        "daily",
			Description: "Claim your daily bonus",
		},
		{
			Name:        "weekly",
			Description: "Claim your weekly bonus",
		},
		{
			Name:        "cooldowns",
			Description: "Check your bonus cooldowns",
		},
		{
			Name:        "claimall",
			Description: "Claim all available bonuses",
		},
		{
			Name:        "vote",
			Description: "Vote for the bot on Top.gg to claim your bonus",
		},
		{
			Name:        "premium",
			Description: "Manage your premium features",
		},
		{
			Name:        "prestige",
			Description: "Reset your rank to gain a prestige level",
		},
		{
			Name:        "leaderboard",
			Description: "View the server leaderboards",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "chips",
					Description: "Top 10 users by Chips",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "xp",
					Description: "Top 10 users by total XP",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "prestige",
					Description: "Top 10 users by prestige",
				},
			},
		},
		blackjack.RegisterBlackjackCommands(),
		baccarat.RegisterBaccaratCommand(),
		craps.RegisterCrapsCommand(),
		slots.RegisterSlotsCommand(),
		horseracing.RegisterHorseRacingCommand(),
		mines.RegisterMinesCommand(),
		higherorlower.RegisterHigherOrLowerCommand(),
		roulette.RegisterRouletteCommand(),
		threecardpoker.RegisterThreeCardPokerCommand(),
		// Admin commands (with runtime permission checking)
		{
			Name:        "addchips",
			Description: "Add chips to a user's balance (Admins only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "User to receive chips",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "amount",
					Description: "Amount of chips to add",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "reason",
					Description: "Reason for the addition",
					Required:    true,
				},
			},
		},
	}

	// Hash commands to skip unnecessary overwrites
	data, _ := json.Marshal(globalCommands)
	sha := sha256.Sum256(data)
	newHash := hex.EncodeToString(sha[:])
	const hashFile = ".commands.hash"
	oldHashBytes, _ := os.ReadFile(hashFile)
	oldHash := strings.TrimSpace(string(oldHashBytes))
	if oldHash == newHash {
		return nil
	}
	// Global registration only (eliminates duplicate commands)
	// Register all commands globally (may take up to an hour to propagate)
	_, err := s.ApplicationCommandBulkOverwrite(s.State.User.ID, "", globalCommands)
	if err != nil {
		return fmt.Errorf("global bulk overwrite failed: %w", err)
	}
	os.WriteFile(hashFile, []byte(newHash), 0644)
	return nil
}

func onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Slash commands
	if i.Type == discordgo.InteractionApplicationCommand {
		switch i.ApplicationCommandData().Name {
		case "ping":
			handlePingCommand(s, i)
		case "info":
			handleInfoCommand(s, i)
		case "help":
			handleHelpCommand(s, i)
		case "profile":
			handleProfileCommand(s, i)
		case "chips":
			handleChipsCommand(s, i)
		case "balance":
			handleBalanceCommand(s, i)
		case "hourly":
			handleBonusCommand(s, i, utils.BonusHourly)
		case "daily":
			handleBonusCommand(s, i, utils.BonusDaily)
		case "weekly":
			handleBonusCommand(s, i, utils.BonusWeekly)
		case "cooldowns":
			handleCooldownsCommand(s, i)
		case "claimall":
			handleClaimAllCommand(s, i)
		case "vote":
			handleVoteCommand(s, i)
		case "leaderboard":
			handleLeaderboardCommand(s, i)
		case "prestige":
			handlePrestigeCommand(s, i)
		case "premium":
			handlePremiumCommand(s, i)
		case "addchips":
			handleAddChipsCommand(s, i)
		case "blackjack":
			blackjack.HandleBlackjackCommand(s, i)
		case "baccarat":
			baccarat.HandleBaccaratCommand(s, i)
		case "craps":
			craps.HandleCrapsCommand(s, i)
		case "slots":
			slots.HandleSlotsCommand(s, i)
		case "derby":
			horseracing.HandleHorseRacingCommand(s, i)
		case "mines":
			mines.HandleMinesCommand(s, i)
		case "horl":
			higherorlower.HandleHigherOrLowerCommand(s, i)
		case "roulette":
			roulette.HandleRouletteCommand(s, i)
		case "tcpoker":
			threecardpoker.HandleThreeCardPokerCommand(s, i)
		}
		return
	}
	// Modal submissions
	if i.Type == discordgo.InteractionModalSubmit {
		if strings.HasPrefix(i.ModalSubmitData().CustomID, "roulette_bet_modal_") {
			roulette.HandleRouletteModal(s, i)
		}
		if strings.HasPrefix(i.ModalSubmitData().CustomID, "craps_bet_modal_") {
			craps.HandleCrapsModal(s, i)
		}
		if strings.HasPrefix(i.ModalSubmitData().CustomID, "derby_bet_modal_") {
			horseracing.HandleHorseRacingModal(s, i)
		}
	}
}

func onButtonInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}

	customID := i.MessageComponentData().CustomID

	// Route button interactions to appropriate handlers
	if strings.HasPrefix(customID, "blackjack_") {
		blackjack.HandleBlackjackInteraction(s, i)
	}

	if strings.HasPrefix(customID, "roulette_") {
		roulette.HandleRouletteInteraction(s, i)
	}

	if strings.HasPrefix(customID, "baccarat_") {
		baccarat.HandleBaccaratButton(s, i)
	}

	if strings.HasPrefix(customID, "craps_") {
		if customID == "craps_bet_select" { // select menu handled elsewhere
			craps.HandleCrapsSelect(s, i)
		} else {
			craps.HandleCrapsButton(s, i)
		}
	}

	if strings.HasPrefix(customID, "horl_") {
		higherorlower.HandleHigherOrLowerInteraction(s, i)
	}

	if strings.HasPrefix(customID, "tcp_") {
		threecardpoker.HandleThreeCardPokerInteraction(s, i)
	}

	if strings.HasPrefix(customID, "slots_") {
		slots.HandleSlotsInteraction(s, i)
	}

	if strings.HasPrefix(customID, "derby_") {
		horseracing.HandleHorseRacingInteraction(s, i)
	}

	if strings.HasPrefix(customID, "mines_") {
		mines.HandleMinesButton(s, i)
	}

	if strings.HasPrefix(customID, "premium_") {
		handlePremiumButton(s, i)
	}

	if strings.HasPrefix(customID, "prestige_") {
		handlePrestigeButtons(s, i)
	}

	if strings.HasPrefix(customID, "vote_") {
		handleVoteButton(s, i)
	}

	if strings.HasPrefix(customID, "profile_achievements_") {
		handleProfileAchievementsButton(s, i)
	}
}

func handlePingCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	start := time.Now()
	latency := s.HeartbeatLatency()
	embed := utils.CreateBrandedEmbed("🏓 Pong!", "", utils.BotColor)
	embed.Fields = []*discordgo.MessageEmbedField{
		{Name: "Latency", Value: fmt.Sprintf("%dms", latency.Milliseconds()), Inline: true},
		{Name: "Status", Value: "✅ Online", Inline: true},
		{Name: "Response Time", Value: fmt.Sprintf("%dms", time.Since(start).Milliseconds()), Inline: true},
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}}})
}

func handleInfoCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := utils.CreateBrandedEmbed("🎰 High Rollers Club Bot", "A Discord casino bot rewritten in Go", utils.BotColor)
	embed.Fields = []*discordgo.MessageEmbedField{
		{Name: "Version", Value: "2.0.0 (Go Rewrite)", Inline: true},
		{Name: "Language", Value: "Go", Inline: true},
		{Name: "Framework", Value: "DiscordGo", Inline: true},
		{Name: "Features", Value: "✅ User System & Profiles\n✅ Blackjack Game\n🔜 More Casino Games & Achievements", Inline: false},
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}}})
}

func handleProfileCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Determine whose profile to show
	targetUserID := i.Member.User.ID
	targetDiscordUser := i.Member.User
	if len(i.ApplicationCommandData().Options) > 0 {
		if u := i.ApplicationCommandData().Options[0].UserValue(nil); u != nil {
			targetUserID = u.ID
			targetDiscordUser = u
		}
	}
	userID, _ := strconv.ParseInt(targetUserID, 10, 64)
	user, err := utils.GetUser(userID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: "❌ Error accessing user data.", Flags: discordgo.MessageFlagsEphemeral}})
		return
	}
	// Premium: hide wins/losses unless target has premium enabled, and show premium badge
	showWinLoss := false
	hasPremiumBadge := false
	// Try to fetch member from main guild for role checks when viewing others
	var memberForPremium *discordgo.Member = i.Member
	if targetDiscordUser.ID != i.Member.User.ID {
		if m, err := s.GuildMember(i.GuildID, targetDiscordUser.ID); err == nil {
			memberForPremium = m
		}
	}
	if memberForPremium != nil && utils.HasPremiumRole(memberForPremium) {
		hasPremiumBadge = true
		showWinLoss = utils.GetPremiumSetting(user, utils.PremiumFeatureWinsLosses)
	}
	embed := utils.UserProfileEmbed(user, targetDiscordUser, showWinLoss, hasPremiumBadge)
	// Components: View Achievements button and conditional Join link
	components := []discordgo.MessageComponent{}
	// Row with View Achievements
	achBtn := discordgo.Button{CustomID: "profile_achievements_" + targetUserID, Label: "View Achievements", Style: discordgo.PrimaryButton}
	row := discordgo.ActionsRow{Components: []discordgo.MessageComponent{achBtn}}
	// Conditionally add Join for /bonus link if user not in main guild
	showJoin := true
	// Try to check membership in main guild
	mainGuildID := strconv.FormatInt(utils.GuildID, 10)
	if m, err := s.GuildMember(mainGuildID, i.Member.User.ID); err == nil && m != nil {
		showJoin = false
	}
	if showJoin {
		joinBtn := discordgo.Button{Label: "Join for /bonus", Style: discordgo.LinkButton, URL: utils.HighRollersClubLink}
		row.Components = append(row.Components, joinBtn)
	}
	components = append(components, row)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: components}})

	// After a timeout, grey out the View Achievements button
	go func(inter *discordgo.InteractionCreate) {
		// 2 minutes to keep things tidy
		time.Sleep(2 * time.Minute)
		// Rebuild row but disabled/secondary
		disabledBtn := discordgo.Button{CustomID: "profile_achievements_" + targetUserID, Label: "View Achievements", Style: discordgo.SecondaryButton, Disabled: true}
		newRow := discordgo.ActionsRow{Components: []discordgo.MessageComponent{disabledBtn}}
		if showJoin {
			joinBtn := discordgo.Button{Label: "Join for /bonus", Style: discordgo.LinkButton, URL: utils.HighRollersClubLink}
			newRow.Components = append(newRow.Components, joinBtn)
		}
		edit := &discordgo.WebhookEdit{Components: &[]discordgo.MessageComponent{newRow}}
		_, _ = s.InteractionResponseEdit(inter.Interaction, edit)
	}(i)
}

func handleBalanceCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
	user, err := utils.GetUser(userID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: "❌ Error accessing user data.", Flags: discordgo.MessageFlagsEphemeral}})
		return
	}
	embed := utils.CreateBrandedEmbed("💰 Balance", fmt.Sprintf("You have **%d** %s chips", user.Chips, utils.ChipsEmoji), utils.BotColor)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}}})
}

func handleChipsCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
	user, err := utils.GetUser(userID)
	if err != nil {
		respondWithError(s, i, "❌ Error accessing user data.")
		return
	}
	content := fmt.Sprintf("You have %s %s.", utils.FormatChips(user.Chips), utils.ChipsEmoji)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral}})
}

func handleLeaderboardCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := i.ApplicationCommandData().Options
	sub := "chips"
	if len(opts) > 0 {
		sub = opts[0].Name
	}
	// Optimized leaderboard query with prepared statements
	title := map[string]string{"chips": "High Rollers", "xp": "Total XP", "prestige": "Prestige"}[sub]
	if utils.DB == nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed(title, "Database not connected.", 0xE74C3C), nil, false)
		return
	}

	// Use optimized prepared statements for leaderboard queries
	rows, err := utils.GetLeaderboard(sub)
	if err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed(title, "Failed to load leaderboard.", 0xE74C3C), nil, false)
		return
	}
	defer rows.Close()
	lines := []string{}
	idx := 1
	for rows.Next() {
		var uid int64
		var val int64
		if sub == "prestige" {
			var pv int
			if err := rows.Scan(&uid, &pv); err == nil {
				lines = append(lines, fmt.Sprintf("%d. <@%d> — %d", idx, uid, pv))
				idx++
			}
		} else {
			if err := rows.Scan(&uid, &val); err == nil {
				if sub == "chips" {
					lines = append(lines, fmt.Sprintf("%d. <@%d> — %s %s", idx, uid, utils.FormatChips(val), utils.ChipsEmoji))
				} else {
					lines = append(lines, fmt.Sprintf("%d. <@%d> — %s XP", idx, uid, utils.FormatChips(val)))
				}
				idx++
			}
		}
	}
	if len(lines) == 0 {
		lines = []string{"No data"}
	}
	embed := utils.CreateBrandedEmbed(title, strings.Join(lines, "\n"), utils.BotColor)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}}})
}

func handlePrestigeCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
	user, err := utils.GetUser(userID)
	if err != nil {
		respondWithError(s, i, "❌ Error accessing user data.")
		return
	}
	prestigeReadyLevel := len(utils.Ranks) - 1
	requiredXP := utils.GetXPForLevel(prestigeReadyLevel, user.Prestige)
	if user.CurrentXP < requiredXP {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Prestige", fmt.Sprintf("You are not yet eligible to prestige. You need %s XP.", utils.FormatChips(requiredXP)), 0xE67E22), nil, true)
		return
	}
	embed := utils.CreateBrandedEmbed("<:chips:1396988413151940629> Prestige Confirmation", "Prestige has a price: Every chip you've collected will be reset; you'll have to rank up again to be a High Roller. Only your total XP will be unaffected.", 0xE67E22)
	components := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{CustomID: "prestige_confirm", Label: "Confirm", Style: discordgo.SuccessButton},
		discordgo.Button{CustomID: "prestige_cancel", Label: "Cancel", Style: discordgo.DangerButton},
	}}}
	utils.SendInteractionResponse(s, i, embed, components, false)
}

func handlePrestigeButtons(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cid := i.MessageComponentData().CustomID
	if cid == "prestige_cancel" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseUpdateMessage, Data: &discordgo.InteractionResponseData{Content: "Prestige canceled.", Components: []discordgo.MessageComponent{}}})
		return
	}
	if cid == "prestige_confirm" {
		userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
		// Reset chips and current XP, increment prestige
		p := utils.UserUpdateData{PremiumSettings: nil}
		// Set prestige
		newPrestige := 0
		if u, _ := utils.GetUser(userID); u != nil {
			newPrestige = u.Prestige + 1
		}
		p.Prestige = &newPrestige
		// Reset chips to starting, reset current_xp to 0
		// Use increments to set exact values by computing delta
		u, _ := utils.GetUser(userID)
		if u != nil {
			p.ChipsIncrement = utils.StartingChips - u.Chips
			p.CurrentXPIncrement = -u.CurrentXP
		}
		updated, _ := utils.UpdateCachedUser(userID, p)
		embed := utils.CreateBrandedEmbed("Prestiged!", fmt.Sprintf("You are now Prestige %d. Balance reset to %s %s.", updated.Prestige, utils.FormatChips(updated.Chips), utils.ChipsEmoji), 0xF1C40F)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseUpdateMessage, Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: []discordgo.MessageComponent{}}})
		return
	}
}

func startHealthServer() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("Discord Bot Status: %s", botStatus)))
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := fmt.Sprintf(`{"status":"healthy","service":"discord-bot","bot_status":"%s"}`, botStatus)
		w.Write([]byte(response))
	})

	log.Printf("Health server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Printf("Health server error: %v", err)
	}
}

// /help
func handleHelpCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := utils.CreateBrandedEmbed("Help", "Here is a list of available commands:", utils.BotColor)
	// Categories similar to Python
	cats := map[string][]string{
		"Casino Games":   {"blackjack", "baccarat", "craps", "horl", "mines", "derby", "roulette", "slots", "tcpoker"},
		"Bonuses":        {"hourly", "daily", "weekly", "vote", "claimall", "cooldowns"},
		"Profile / Rank": {"profile", "balance", "premium"},
		"Admin":          {"addchips"},
		"Help":           {"help"},
	}
	desc := map[string]string{
		"blackjack": "Play Blackjack",
		"baccarat":  "Play Baccarat",
		"craps":     "Play Craps",
		"horl":      "Play Higher or Lower",
		"derby":     "Bet on Horse Racing",
		"mines":     "Play Mines",
		"roulette":  "Play Roulette",
		"slots":     "Play Slots",
		"tcpoker":   "Play Three Card Poker",
		"hourly":    "Claim your hourly bonus",
		"daily":     "Claim your daily bonus",
		"weekly":    "Claim your weekly bonus",
		"vote":      "Vote on Top.gg for bonus chips",
		"claimall":  "Claim all available bonuses",
		"cooldowns": "View your bonus cooldowns",
		"profile":   "View your casino profile and stats",
		"balance":   "Check your chip balance",
		"premium":   "Manage premium feature visibility",
		"addchips":  "Add chips to a user (admins)",
		"help":      "Show this help",
	}
	for name, cmds := range cats {
		var lines []string
		for _, c := range cmds {
			lines = append(lines, fmt.Sprintf("`/%s` - %s", c, desc[c]))
		}
		if len(lines) > 0 {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: name, Value: strings.Join(lines, "\n"), Inline: false})
		}
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Flags: discordgo.MessageFlagsEphemeral}})
}

// /premium viewer + toggles
func buildPremiumEmbedAndComponents(member *discordgo.Member, user *utils.User) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	hasRole := utils.HasPremiumRole(member)
	var color int
	if hasRole {
		color = 0xFFD700
	} else {
		color = 0xE74C3C
	}
	embed := utils.CreateBrandedEmbed("💎 Premium Features", "", color)
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1753753476/PR2_oxsxaa.png"}
	var components []discordgo.MessageComponent
	if hasRole {
		embed.Description = "Toggle your premium features on or off:"
		xp := utils.GetPremiumSetting(user, utils.PremiumFeatureXPDisplay)
		wl := utils.GetPremiumSetting(user, utils.PremiumFeatureWinsLosses)
		status := func(b bool) string {
			if b {
				return "✅ Enabled"
			} else {
				return "❌ Disabled"
			}
		}
		embed.Fields = []*discordgo.MessageEmbedField{
			{Name: "XP Display", Value: status(xp) + "\nShow XP gained in game results", Inline: false},
			{Name: "Profile Stats", Value: status(wl) + "\nShow wins, losses, win% and total profit in your profile", Inline: false},
		}
		// Buttons reflect state
		btnStyle := func(b bool) discordgo.ButtonStyle {
			if b {
				return discordgo.SuccessButton
			} else {
				return discordgo.DangerButton
			}
		}
		components = []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{CustomID: "premium_" + utils.PremiumFeatureXPDisplay, Label: "XP Display", Style: btnStyle(xp)},
				discordgo.Button{CustomID: "premium_" + utils.PremiumFeatureWinsLosses, Label: "Profile Stats", Style: btnStyle(wl)},
			}},
		}
	} else {
		embed.Description = "You need to be a Patreon member to access premium features.\n\nVisit our Patreon page to subscribe and unlock these features!"
		embed.Fields = []*discordgo.MessageEmbedField{
			{Name: "Available Features", Value: "• XP Display in game results\n• Wins/Losses, Win% and Total Profit in profile\n• Future exclusive features", Inline: false},
		}
	}
	return embed, components
}

func handlePremiumCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	uid, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
	user, _ := utils.GetUser(uid)
	embed, components := buildPremiumEmbedAndComponents(i.Member, user)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: components, Flags: discordgo.MessageFlagsEphemeral},
	})
}

func handlePremiumButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Toggle the corresponding setting
	uid, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
	user, err := utils.GetUser(uid)
	if err != nil {
		return
	}
	key := strings.TrimPrefix(i.MessageComponentData().CustomID, "premium_")
	// flip current value
	current := utils.GetPremiumSetting(user, key)
	if user.PremiumSettings == nil {
		user.PremiumSettings = make(utils.JSONB)
	}
	user.PremiumSettings[key] = !current
	// persist
	_, _ = utils.UpdateCachedUser(uid, utils.UserUpdateData{PremiumSettings: user.PremiumSettings})
	// rebuild UI and update message
	embed, components := buildPremiumEmbedAndComponents(i.Member, user)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: components},
	})
}

// profile achievements button handler
func handleProfileAchievementsButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	cid := i.MessageComponentData().CustomID
	// Expected format: profile_achievements_<userID>
	parts := strings.Split(cid, "_")
	if len(parts) < 3 {
		return
	}
	targetID := parts[2]
	tid, _ := strconv.ParseInt(targetID, 10, 64)

	// Load recent achievements
	var desc string
	if utils.AchievementMgr == nil {
		desc = "Achievements system is initializing. Please try again later."
	} else {
		if achievements, err := utils.AchievementMgr.GetUserAchievements(tid); err == nil {
			if len(achievements) == 0 {
				desc = "No achievements earned yet."
			} else {
				// Show up to last 10
				max := 10
				if len(achievements) < max {
					max = len(achievements)
				}
				lines := make([]string, 0, max)
				for idx := 0; idx < max; idx++ {
					ua := achievements[idx]
					a := utils.AchievementMgr.GetAchievement(ua.AchievementID)
					if a != nil {
						lines = append(lines, fmt.Sprintf("%s **%s** — <t:%d:R>", a.Icon, a.Name, ua.EarnedAt.Unix()))
					}
				}
				desc = strings.Join(lines, "\n")
			}
		} else {
			desc = "Failed to load achievements."
		}
	}
	// Try to fetch the target user for title/avatar context
	titleName := i.Member.User.Username
	if u, err := s.User(targetID); err == nil && u != nil {
		titleName = u.Username
	}
	title := fmt.Sprintf("%s's Achievements", titleName)
	embed := utils.CreateBrandedEmbed(title, desc, utils.BotColor)
	// Ephemeral reply
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Flags: discordgo.MessageFlagsEphemeral},
	})
}

// /addchips (guild-only) with strict role check
func handleAddChipsCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Security: must be from configured guild
	if i.GuildID != AdminGuildID {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Unauthorized", "This command cannot be used in this server.", 0xE74C3C), nil, true)
		return
	}
	// Must have role
	hasRole := false
	for _, rid := range i.Member.Roles {
		if rid == AdminRoleID {
			hasRole = true
			break
		}
	}
	if !hasRole {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Forbidden", "You do not have permission to use this command.", 0xE74C3C), nil, true)
		return
	}

	data := i.ApplicationCommandData()
	var targetID string
	var amount int64
	var reason string
	for _, opt := range data.Options {
		switch opt.Name {
		case "user":
			if opt.UserValue(nil) != nil {
				targetID = opt.UserValue(nil).ID
			}
		case "amount":
			amount = int64(opt.IntValue())
		case "reason":
			reason = opt.StringValue()
		}
	}
	if amount <= 0 || targetID == "" {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Invalid Input", "Please provide a valid user and positive amount.", 0xE74C3C), nil, true)
		return
	}
	tid, _ := strconv.ParseInt(targetID, 10, 64)
	updated, err := utils.UpdateCachedUser(tid, utils.UserUpdateData{ChipsIncrement: amount})
	if err != nil {
		utils.SendInteractionResponse(s, i, utils.CreateBrandedEmbed("Error", "Failed to update user.", 0xE74C3C), nil, true)
		return
	}
	// Confirmation
	embed := utils.CreateBrandedEmbed("Chips Added", fmt.Sprintf("Successfully added %s chips to <@%s> for: %s.", utils.FormatChips(amount), targetID, reason), 0x2ECC71)
	utils.SendInteractionResponse(s, i, embed, nil, true)
	// Log
	if ch, err := s.Channel(AdminLogChannelID); err == nil && ch != nil {
		logEmbed := utils.CreateBrandedEmbed("Chip Transaction Log", "", 0x2ECC71)
		logEmbed.Fields = []*discordgo.MessageEmbedField{
			{Name: "Moderator", Value: i.Member.User.Mention(), Inline: false},
			{Name: "User", Value: "<@" + targetID + ">", Inline: false},
			{Name: "Amount Added", Value: utils.FormatChips(amount), Inline: false},
			{Name: "New Balance", Value: utils.FormatChips(updated.Chips), Inline: false},
		}
		logEmbed.Footer = &discordgo.MessageEmbedFooter{Text: "User ID: " + targetID}
		s.ChannelMessageSendEmbed(AdminLogChannelID, logEmbed)
	}
}

func handleBonusCommand(s *discordgo.Session, i *discordgo.InteractionCreate, bonusType utils.BonusType) {
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)

	// Get or create user
	user, err := utils.GetCachedUser(userID)
	if err != nil {
		respondWithError(s, i, "❌ Error accessing user data. Database may be unavailable.")
		return
	}

	// Attempt to claim bonus
	result, err := utils.BonusMgr.ClaimBonus(user, bonusType)
	if err != nil {
		respondWithError(s, i, "❌ An error occurred while claiming bonus.")
		return
	}

	// Create and send embed
	title := fmt.Sprintf("%s Bonus", string(bonusType))
	embed := utils.BonusMgr.CreateBonusEmbed(user, result, title)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func handleCooldownsCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)

	// Get or create user
	user, err := utils.GetCachedUser(userID)
	if err != nil {
		respondWithError(s, i, "❌ Error accessing user data. Database may be unavailable.")
		return
	}

	// Create cooldown embed
	embed := utils.BonusMgr.CreateCooldownEmbed(user)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func handleClaimAllCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)

	// Get or create user
	user, err := utils.GetCachedUser(userID)
	if err != nil {
		respondWithError(s, i, "❌ Error accessing user data. Database may be unavailable.")
		return
	}

	// Claim all available bonuses
	claimedBonuses, err := utils.BonusMgr.ClaimAllAvailableBonuses(user)
	if err != nil {
		respondWithError(s, i, "❌ An error occurred while claiming bonuses.")
		return
	}

	if len(claimedBonuses) == 0 {
		embed := utils.CreateBrandedEmbed("🎁 Claim All Bonuses",
			"You have no bonuses available to claim.", 0xE74C3C)

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{embed},
			},
		})
		return
	}

	// Calculate totals and build bonus list
	var totalChips, totalXP int64
	claimedList := make([]string, 0)

	for _, bonus := range claimedBonuses {
		if bonus.Success && bonus.BonusInfo != nil {
			totalChips += bonus.BonusInfo.ActualAmount
			totalXP += bonus.BonusInfo.XPAmount

			// Format bonus line like Python version
			bonusName := strings.Title(string(bonus.BonusInfo.Type))
			claimedList = append(claimedList,
				fmt.Sprintf("• **%s**: %s %s", bonusName,
					utils.FormatChips(bonus.BonusInfo.ActualAmount), utils.ChipsEmoji))
		}
	}

	// Get updated user balance for display
	updatedUser, _ := utils.GetUser(userID)

	// Create clean, user-friendly embed matching Python format
	description := fmt.Sprintf("**Bonuses Claimed:**\n%s\n\n**Total**: %s %s\n**New Balance**: %s %s",
		strings.Join(claimedList, "\n"),
		utils.FormatChips(totalChips), utils.ChipsEmoji,
		utils.FormatChips(updatedUser.Chips), utils.ChipsEmoji)

	embed := utils.CreateBrandedEmbed("All Bonuses Claimed!", description, 0x00FF00)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func respondWithError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func handleVoteCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)

	// Check if Top.gg client is available
	if utils.GlobalTopGGClient == nil {
		embed := utils.CreateBrandedEmbed("❌ Vote Unavailable",
			"This feature is currently disabled. The bot owner needs to configure the Top.gg API token.",
			0xE74C3C)
		utils.SendInteractionResponse(s, i, embed, nil, true)
		return
	}

	// Get user data
	user, err := utils.GetCachedUser(userID)
	if err != nil {
		respondWithError(s, i, "❌ Error accessing user data. Database may be unavailable.")
		return
	}

	// Check internal cooldown
	result := utils.BonusMgr.CanClaimBonus(user, utils.BonusVote)
	if !result.Success {
		// Show cooldown remaining
		hours := int(result.TimeRemaining.Hours())
		minutes := int(result.TimeRemaining.Minutes()) % 60

		embed := utils.CreateBrandedEmbed("🗳️ Vote Cooldown",
			fmt.Sprintf("You have already claimed your vote bonus. You can claim again in %dh %dm.", hours, minutes),
			0xE74C3C)
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1754610906/TU_khaw12.png",
		}
		utils.SendInteractionResponse(s, i, embed, nil, true)
		return
	}

	// Check Top.gg vote status
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hasVoted, err := utils.GlobalTopGGClient.CheckUserVote(ctx, i.Member.User.ID)
	if err != nil {
		embed := utils.CreateBrandedEmbed("❌ Vote Check Failed",
			"Could not verify your vote with Top.gg. Please try again later.",
			0xE74C3C)
		utils.SendInteractionResponse(s, i, embed, nil, true)
		return
	}

	if hasVoted {
		// Grant the vote bonus
		bonusResult, err := utils.BonusMgr.ClaimBonus(user, utils.BonusVote)
		if err != nil {
			respondWithError(s, i, "❌ An error occurred while claiming vote bonus.")
			return
		}

		// Create success embed - matching Python format
		embed := utils.CreateBrandedEmbed("🗳️ Vote Bonus Claimed!",
			fmt.Sprintf("Thank you for voting! Your vote was successfully counted and helps us a tremendous amount!\n\n"+
				"You gained **%s** %s chips.\n\n"+
				"You can vote and claim again in 12 hours.",
				utils.FormatChips(bonusResult.BonusInfo.ActualAmount), utils.ChipsEmoji),
			0x00FF00)
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1754610906/TU_khaw12.png",
		}
		utils.SendInteractionResponse(s, i, embed, nil, true)

		// Check for achievements (similar to Python version)
		go func() {
			if utils.AchievementMgr != nil {
				// Get updated user for achievement checking
				if updatedUser, err := utils.GetUser(userID); err == nil {
					newlyAwarded, err := utils.AchievementMgr.CheckUserAchievements(updatedUser)
					if err == nil && len(newlyAwarded) > 0 {
						// Could send achievement notification here if needed
					}
				}
			}
		}()

	} else {
		// Show vote button
		embed := utils.CreateBrandedEmbed("🗳️ Vote on Top.gg",
			"You have not voted for the bot in the last 12 hours. "+
				"Click the button below to vote, then run this command again to claim your reward!",
			utils.BotColor)
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1754610906/TU_khaw12.png",
		}

		// Create vote button
		components := []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label: "Vote on Top.gg",
						Style: discordgo.LinkButton,
						URL:   utils.GlobalTopGGClient.GetVoteURL(),
						Emoji: &discordgo.ComponentEmoji{Name: "🗳️"},
					},
				},
			},
		}

		utils.SendInteractionResponse(s, i, embed, components, true)
	}
}

func handleVoteButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// For future use if we need specific vote button interactions
	// Currently, the vote button is a link button that opens Top.gg directly
}

// sanitizeToken removes common accidental decorations around a token
func sanitizeToken(raw string) string {
	t := strings.TrimSpace(raw)
	// Remove surrounding quotes
	t = strings.Trim(t, "'\"")
	// Strip leading 'export ' if user copied from shell
	if strings.HasPrefix(strings.ToLower(t), "export ") {
		parts := strings.SplitN(t, "=", 2)
		if len(parts) == 2 {
			t = strings.TrimSpace(parts[1])
			t = strings.Trim(t, "'\"")
		}
	}
	// Remove leading BOT_TOKEN= if present
	if idx := strings.Index(t, "="); idx != -1 && idx < 25 { // token assignments usually small key before '='
		maybeKey := t[:idx]
		if strings.Contains(strings.ToUpper(maybeKey), "TOKEN") {
			t = strings.TrimSpace(t[idx+1:])
			t = strings.Trim(t, "'\"")
		}
	}
	// Strip leading 'Bot ' prefix if mistakenly included
	if strings.HasPrefix(strings.ToLower(t), "bot ") {
		t = strings.TrimSpace(t[4:])
	}
	return t
}
