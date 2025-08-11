package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	blackjack "hrc-go/games/blackjack"
	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
)

var session *discordgo.Session
var botStatus = "starting"
var readyCh = make(chan struct{}, 1)

func main() {
	// Start HTTP server for Railway health checks
	go startHealthServer()

	log.Println("[startup] Main initialization sequence starting...")

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

	// Initialize jackpot system
	jackpotInitCh := make(chan error, 1)
	log.Println("[startup] Beginning jackpot manager initialization (async)...")
	go func() {
		jackpotInitCh <- utils.InitializeJackpotManager()
	}()

	select {
	case err := <-jackpotInitCh:
		if err != nil {
			log.Printf("Jackpot manager initialization failed: %v", err)
			log.Println("Bot will continue without jackpot features")
		} else {
			log.Println("Jackpot system initialized (async)")
		}
	case <-time.After(5 * time.Second):
		log.Println("[warn] Jackpot manager initialization taking longer than 5s; continuing startup. Will log result when complete.")
		go func() { // log late result
			if err := <-jackpotInitCh; err != nil {
				log.Printf("[late] Jackpot manager initialized with error after delay: %v", err)
			} else {
				log.Println("[late] Jackpot manager initialized successfully after delay")
			}
		}()
	}

	// (Game manager initialization removed; legacy interface cleanup)

	// Get bot token from environment (support multiple common var names)
	var token string
	for _, key := range []string{"BOT_TOKEN", "DISCORD_TOKEN", "DISCORD_BOT_TOKEN", "HRC_BOT_TOKEN"} {
		val := strings.TrimSpace(os.Getenv(key))
		if val != "" {
			log.Printf("[startup] Using token from env var %s", key)
			token = val
			break
		}
	}
	if token == "" {
		log.Println("[startup] No BOT_TOKEN or DISCORD_TOKEN environment variable found – bot will not connect. Set BOT_TOKEN.")
		botStatus = "no_token"
		select {}
	}

	// Basic sanity check (Discord bot tokens have three parts separated by '.')
	parts := strings.Split(token, ".")
	if len(parts) < 3 {
		log.Println("[startup] BOT_TOKEN format looks invalid (expected 3 segments). Refusing to connect.")
		botStatus = "invalid_token"
		select {}
	}

	masked := func(t string) string {
		if len(t) <= 10 {
			return "***"
		}
		return t[:4] + "..." + t[len(t)-4:]
	}(token)
	log.Printf("[startup] Retrieved bot token (masked): %s | segments=%d", masked, len(parts))

	// Preflight network connectivity to Discord
	log.Println("[net] Starting Discord connectivity preflight (tcp + tls)...")
	if err := testDiscordConnectivity(); err != nil {
		log.Printf("[warn] Discord connectivity preflight failed: %v", err)
	} else {
		log.Println("[net] Discord connectivity preflight succeeded")
	}

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

	log.Println("[discord] Discord session object created, opening gateway connection...")

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
	log.Println("[discord] Gateway connection attempt initiated, waiting for READY event (30s timeout)...")

	// Wait for READY or timeout to help diagnose hanging
	select {
	case <-readyCh:
		log.Println("[discord] Received READY event from Discord.")
	case <-time.After(30 * time.Second):
		log.Println("[warn] Did not receive READY within 30s. Verify token, enabled intents (SERVER MEMBERS & others in Dev Portal), and that the bot is invited to at least one guild.")
	}
	defer session.Close()

	log.Println("Bot is now running. Press CTRL+C to exit.")
	botStatus = "running"

	// Heartbeat diagnostic goroutine
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			log.Printf("[heartbeat] status=%s", botStatus)
		}
	}()

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-stop

	log.Println("Gracefully shutting down...")
	botStatus = "shutting_down"
}

// testDiscordConnectivity performs a lightweight TCP + TLS dial to Discord API host
func testDiscordConnectivity() error {
	// Primary REST API host
	apiHost := "discord.com:443"
	start := time.Now()
	conn, err := net.DialTimeout("tcp", apiHost, 3*time.Second)
	if err != nil {
		return fmt.Errorf("tcp dial failed: %w", err)
	}
	defer conn.Close()
	durTCP := time.Since(start)

	// Upgrade to TLS to ensure handshake works
	startTLS := time.Now()
	client := tls.Client(conn, &tls.Config{ServerName: "discord.com"})
	if err := client.Handshake(); err != nil {
		return fmt.Errorf("tls handshake failed: %w", err)
	}
	durTLS := time.Since(startTLS)

	log.Printf("[net] Preflight TCP ok (%.0fms), TLS ok (%.0fms)", durTCP.Seconds()*1000, durTLS.Seconds()*1000)
	return nil
}

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
	}

	for _, command := range commands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, "", command)
		if err != nil {
			return fmt.Errorf("failed to create command %s: %w", command.Name, err)
		}
	}

	log.Printf("Successfully registered %d slash commands", len(commands))
	return nil
}

func onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

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
}

func handlePingCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	startTime := time.Now()

	// Calculate approximate latency
	latency := s.HeartbeatLatency()

	embed := &discordgo.MessageEmbed{
		Title: "🏓 Pong!",
		Color: 0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Latency",
				Value:  fmt.Sprintf("%dms", latency.Milliseconds()),
				Inline: true,
			},
			{
				Name:   "Status",
				Value:  "✅ Online",
				Inline: true,
			},
			{
				Name:   "Response Time",
				Value:  fmt.Sprintf("%dms", time.Since(startTime).Milliseconds()),
				Inline: true,
			},
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

func handleInfoCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "🎰 High Rollers Club Bot",
		Description: "A Discord casino bot built with Go",
		Color:       0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Version",
				Value:  "2.0.0 (Go Rewrite)",
				Inline: true,
			},
			{
				Name:   "Language",
				Value:  "Go",
				Inline: true,
			},
			{
				Name:   "Framework",
				Value:  "DiscordGo",
				Inline: true,
			},
			{
				Name:   "Features",
				Value:  "✅ User System & Profiles\n✅ Blackjack Game\n🔜 More Casino Games & Achievements",
				Inline: false,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "High Rollers Club",
		},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func handleProfileCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
	username := i.Member.User.Username

	// Get or create user
	user, err := utils.GetUser(userID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Error accessing user data. Database may be unavailable.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Get rank information
	rankName, rankIcon, rankColor, nextRankXP := utils.GetRank(user.TotalXP)

	// Calculate win rate
	totalGames := user.Wins + user.Losses
	winRate := 0.0
	if totalGames > 0 {
		winRate = float64(user.Wins) / float64(totalGames) * 100
	}

	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("🎰 %s's Casino Profile", username),
		Color: rankColor,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "💰 Chips",
				Value:  fmt.Sprintf("%d <:chips:1396988413151940629>", user.Chips),
				Inline: true,
			},
			{
				Name:   "🏆 Rank",
				Value:  fmt.Sprintf("%s %s", rankIcon, rankName),
				Inline: true,
			},
			{
				Name:   "⭐ Total XP",
				Value:  fmt.Sprintf("%d", user.TotalXP),
				Inline: true,
			},
			{
				Name:   "🎯 Games Won",
				Value:  fmt.Sprintf("%d", user.Wins),
				Inline: true,
			},
			{
				Name:   "💔 Games Lost",
				Value:  fmt.Sprintf("%d", user.Losses),
				Inline: true,
			},
			{
				Name:   "📊 Win Rate",
				Value:  fmt.Sprintf("%.1f%%", winRate),
				Inline: true,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Casino Profile",
		},
	}

	// Add prestige if > 0
	if user.Prestige > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "🌟 Prestige",
			Value:  fmt.Sprintf("Level %d", user.Prestige),
			Inline: true,
		})
	}

	// Add next rank progress if not max rank
	if nextRankXP > user.TotalXP {
		xpNeeded := nextRankXP - user.TotalXP
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "🚀 Next Rank",
			Value:  fmt.Sprintf("%d XP needed", xpNeeded),
			Inline: true,
		})
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func handleBalanceCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
	username := i.Member.User.Username

	// Get or create user
	user, err := utils.GetUser(userID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Error accessing user data. Database may be unavailable.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("💰 %s's Balance", username),
		Color:       0x5865F2,
		Description: fmt.Sprintf("You currently have **%d** <:chips:1396988413151940629> chips", user.Chips),
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
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
