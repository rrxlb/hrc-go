package main

import (
	"encoding/base64"
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
	roulette "hrc-go/games/roulette"
	threecardpoker "hrc-go/games/three_card_poker"
	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
)

var session *discordgo.Session
var botStatus = "starting"
var readyCh = make(chan struct{}, 1)

const devGuildID = "1262162191923023882" // fast registration guild

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

	// Initialize cache system (10 minute TTL)
	utils.InitializeCache(10 * time.Minute)
	defer utils.CloseCache()
	log.Println("Cache system initialized")

	// Initialize achievement system
	if err := utils.InitializeAchievementManager(); err != nil {
		log.Printf("Achievement manager initialization failed: %v", err)
		log.Println("Bot will continue without achievement features")
	} else {
		log.Println("Achievement system initialized")
	}

	// Initialize jackpot system (non-blocking but quieter)
	jackpotInitCh := make(chan error, 1)
	go func() { jackpotInitCh <- utils.InitializeJackpotManager() }()
	select {
	case err := <-jackpotInitCh:
		if err != nil {
			log.Printf("Jackpot manager init failed: %v", err)
		} else {
			log.Println("Jackpot system initialized")
		}
	case <-time.After(3 * time.Second):
		log.Println("Jackpot init continuing in background...")
		go func() {
			if err := <-jackpotInitCh; err != nil {
				log.Printf("Jackpot manager late init error: %v", err)
			} else {
				log.Println("Jackpot system initialized (late)")
			}
		}()
	}

	// (Game manager initialization removed; legacy interface cleanup)

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
		log.Printf("Token appears malformed (segments=%d). Recheck BOT_TOKEN value.", len(parts))
		botStatus = "invalid_token"
		select {}
	}
	if userIDPart, err := base64.RawStdEncoding.DecodeString(parts[0]); err == nil {
		// Expect numeric user ID
		if _, convErr := strconv.ParseInt(string(userIDPart), 10, 64); convErr != nil {
			log.Printf("First token segment decoded but not numeric (%q). Token likely wrong.", string(userIDPart))
		}
	} else {
		log.Printf("Failed to base64 decode first token segment: %v (segment=%s)", err, parts[0])
	}
	log.Printf("Token length=%d characters (sanitized).", len(token))

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

	log.Println("Starting Discord session...")

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
	log.Println("Waiting for READY (30s timeout)...")

	// Wait for READY or timeout to help diagnose hanging
	select {
	case <-readyCh:
		// READY received
	case <-time.After(30 * time.Second):
		log.Println("READY not received in 30s (continuing). Ensure bot is in a guild and intents enabled if interactions fail.")
	}
	defer session.Close()

	log.Println("Bot is now running. Press CTRL+C to exit.")
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
	if err := s.UpdateStatusComplex(discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{
			{
				Name: "Casino Games - Go Version",
				Type: discordgo.ActivityTypeGame,
			},
		},
		Status: "online",
	}); err != nil {
		log.Printf("Failed to update status: %v", err)
	}

	// Register slash commands
	if err := registerSlashCommands(s); err != nil {
		log.Printf("Failed to register slash commands: %v", err)
	}
}

func registerSlashCommands(s *discordgo.Session) error {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "ping",
			Description: "Check bot latency and status",
		},
		{
			Name:        "info",
			Description: "Get information about the bot",
		},
		{
			Name:        "profile",
			Description: "View your casino profile and stats",
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
		blackjack.RegisterBlackjackCommands(),
		baccarat.RegisterBaccaratCommand(),
		roulette.RegisterRouletteCommand(),
		threecardpoker.RegisterThreeCardPokerCommand(),
	}

	// Register as guild commands for instant updates
	for _, command := range commands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, devGuildID, command)
		if err != nil {
			return fmt.Errorf("failed to create guild command %s: %w", command.Name, err)
		}
	}
	log.Printf("Registered %d guild slash commands in %s", len(commands), devGuildID)
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
		case "profile":
			handleProfileCommand(s, i)
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
		case "blackjack":
			blackjack.HandleBlackjackCommand(s, i)
		case "baccarat":
			baccarat.HandleBaccaratCommand(s, i)
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

	if strings.HasPrefix(customID, "tcp_") {
		threecardpoker.HandleThreeCardPokerInteraction(s, i)
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
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
	user, err := utils.GetUser(userID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: "❌ Error accessing user data.", Flags: discordgo.MessageFlagsEphemeral}})
		return
	}
	embed := utils.UserProfileEmbed(user, i.Member.User)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}}})
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
		embed := &discordgo.MessageEmbed{
			Title:       "🎁 Claim All Bonuses",
			Description: "❌ No bonuses are currently available to claim.",
			Color:       0xff6b6b,
			Footer: &discordgo.MessageEmbedFooter{
				Text: "Bonus System",
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{embed},
			},
		})
		return
	}

	// Calculate totals
	var totalChips, totalXP int64
	bonusTypes := make([]string, 0)

	for _, bonus := range claimedBonuses {
		if bonus.Success && bonus.BonusInfo != nil {
			totalChips += bonus.BonusInfo.ActualAmount
			totalXP += bonus.BonusInfo.XPAmount
			bonusTypes = append(bonusTypes, string(bonus.BonusInfo.Type))
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🎁 Claim All Bonuses",
		Description: fmt.Sprintf("Successfully claimed %d bonuses!", len(claimedBonuses)),
		Color:       0x00ff00,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "💰 Total Chips",
				Value:  fmt.Sprintf("%d %s", totalChips, utils.ChipsEmoji),
				Inline: true,
			},
			{
				Name:   "⭐ Total XP",
				Value:  fmt.Sprintf("%d XP", totalXP),
				Inline: true,
			},
			{
				Name:   "🎯 Bonuses Claimed",
				Value:  strings.Join(bonusTypes, ", "),
				Inline: false,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Bonus System",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

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
